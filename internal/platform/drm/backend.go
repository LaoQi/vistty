package drm

import (
	"fmt"
	"os"
	"sync"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform"
)

type DRMBackend struct {
	fd        *os.File
	display   *DisplayInfo
	outputs   []*DisplayInfo
	vt        *VTManager
	surface   *DRMSurface
	surfaces  map[uint32]*DRMSurface
	doneCh    chan struct{}
	eventDone chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once

	gbmDevice   *GBMDevice
	gbmSurfaces map[uint32]*GBMSurface
	commitor    *AtomicCommitor
	useGBM      bool
}

func NewDRMBackend() (*DRMBackend, error) {
	cards := drminternal.ListDevices()
	if len(cards) == 0 {
		return nil, fmt.Errorf("no DRM device found")
	}

	var fd *os.File
	var err error
	for _, card := range cards {
		fd, err = os.OpenFile(card, os.O_RDWR, 0)
		if err == nil {
			break
		}
	}
	if fd == nil {
		return nil, fmt.Errorf("open DRM device: %w", err)
	}

	if err := drminternal.SetMaster(int(fd.Fd())); err != nil {
		fd.Close()
		return nil, fmt.Errorf("set master: %w", err)
	}

	if !drminternal.HasDumbBuffer(int(fd.Fd())) {
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("device does not support dumb buffers")
	}

	outputs, err := findOutputs(int(fd.Fd()))
	if err != nil {
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, err
	}

	b := &DRMBackend{
		fd:          fd,
		display:     outputs[0],
		outputs:     outputs,
		surfaces:    make(map[uint32]*DRMSurface),
		gbmSurfaces: make(map[uint32]*GBMSurface),
		doneCh:      make(chan struct{}),
		eventDone:   make(chan struct{}),
	}

	if drminternal.HasAtomic(int(fd.Fd())) {
		gbmDev, err := NewGBMDevice(int(fd.Fd()))
		if err == nil {
			b.gbmDevice = gbmDev
			b.commitor = NewAtomicCommitor(int(fd.Fd()))
			b.useGBM = true
		}
	}

	vt, err := newVTManager(VTCallbacks{
		OnActivate: func() {
			drminternal.SetMaster(int(fd.Fd()))
			if b.surface != nil {
				b.surface.SetActive(true)
			}
			for _, s := range b.gbmSurfaces {
				s.SetActive(true)
			}
		},
		OnDeactivate: func() {
			if b.surface != nil {
				b.surface.SetActive(false)
			}
			for _, s := range b.gbmSurfaces {
				s.SetActive(false)
			}
			drminternal.DropMaster(int(fd.Fd()))
		},
	})
	if err != nil {
		if b.commitor != nil {
			b.commitor.Close()
		}
		if b.gbmDevice != nil {
			b.gbmDevice.Close()
		}
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("vt manager: %w", err)
	}
	b.vt = vt

	if err := vt.SetGraphicsMode(); err != nil {
		vt.Close()
		if b.commitor != nil {
			b.commitor.Close()
		}
		if b.gbmDevice != nil {
			b.gbmDevice.Close()
		}
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("set graphics mode: %w", err)
	}

	return b, nil
}

func (b *DRMBackend) CreateSurface(width, height int) (platform.Surface, error) {
	width = int(b.display.mode.HDisplay)
	height = int(b.display.mode.VDisplay)

	if b.useGBM {
		surf, err := b.gbmDevice.CreateSurface(width, height, b.display.crtcID, b.display.connID, &b.display.mode, b.commitor)
		if err != nil {
			return nil, err
		}
		b.gbmSurfaces[b.display.crtcID] = surf
		return surf, nil
	}

	surf, err := newDRMSurface(int(b.fd.Fd()), width, height, b.display.crtcID, b.display.connID)
	if err != nil {
		return nil, err
	}
	b.surface = surf
	b.surfaces[b.display.crtcID] = surf

	connIDs := []uint32{b.display.connID}
	if err := drminternal.SetCrtc(int(b.fd.Fd()), b.display.crtcID, surf.bufs[surf.current].fbID, 0, 0, &b.display.mode, connIDs); err != nil {
		surf.Close()
		return nil, fmt.Errorf("set crtc: %w", err)
	}

	return surf, nil
}

func (b *DRMBackend) CreateSurfaceFor(out platform.Output) (platform.Surface, error) {
	di, ok := out.(*DisplayInfo)
	if !ok {
		return nil, fmt.Errorf("unsupported output type: %T", out)
	}

	width := int(di.mode.HDisplay)
	height := int(di.mode.VDisplay)

	if b.useGBM {
		surf, err := b.gbmDevice.CreateSurface(width, height, di.crtcID, di.connID, &di.mode, b.commitor)
		if err != nil {
			return nil, err
		}
		b.gbmSurfaces[di.crtcID] = surf
		return surf, nil
	}

	surf, err := newDRMSurface(int(b.fd.Fd()), width, height, di.crtcID, di.connID)
	if err != nil {
		return nil, err
	}
	b.surfaces[di.crtcID] = surf
	if b.surface == nil {
		b.surface = surf
	}

	connIDs := []uint32{di.connID}
	if err := drminternal.SetCrtc(int(b.fd.Fd()), di.crtcID, surf.bufs[surf.current].fbID, 0, 0, &di.mode, connIDs); err != nil {
		surf.Close()
		return nil, fmt.Errorf("set crtc: %w", err)
	}

	return surf, nil
}

func (b *DRMBackend) ListOutputs() ([]platform.Output, error) {
	outs := make([]platform.Output, len(b.outputs))
	for i, o := range b.outputs {
		outs[i] = o
	}
	return outs, nil
}

func (b *DRMBackend) CreateInputSource() (platform.InputSource, error) {
	return newDRMInput()
}

func (b *DRMBackend) Run(fn func()) {
	fn()
	go b.eventLoop()
}

func (b *DRMBackend) Stop() error {
	b.stopOnce.Do(func() {
		close(b.doneCh)
		close(b.eventDone)
	})
	return nil
}

func (b *DRMBackend) eventLoop() {
	for {
		ev, err := drminternal.ReadEvent(int(b.fd.Fd()))
		if err != nil {
			select {
			case <-b.doneCh:
				return
			case <-b.eventDone:
				return
			default:
				continue
			}
		}
		if ev != nil && (ev.Type == drminternal.EventFlipComplete || ev.Type == drminternal.EventVBlank) {
			if gbmSurf, ok := b.gbmSurfaces[ev.CrtcID]; ok {
				gbmSurf.notifyFlip()
			} else if surf, ok := b.surfaces[ev.CrtcID]; ok {
				surf.notifyFlip()
			} else if b.surface != nil {
				b.surface.notifyFlip()
			}
		}
	}
}

func (b *DRMBackend) Close() error {
	b.closeOnce.Do(func() {
		b.Stop()
		for _, s := range b.gbmSurfaces {
			s.Close()
		}
		b.gbmSurfaces = nil
		if b.commitor != nil {
			b.commitor.Close()
			b.commitor = nil
		}
		if b.gbmDevice != nil {
			b.gbmDevice.Close()
			b.gbmDevice = nil
		}
		if b.display != nil && b.display.savedCrtc != nil {
			saved := b.display.savedCrtc
			var mode *drminternal.ModeInfoPublic
			if saved.ModeValid {
				mode = &saved.Mode
			}
			drminternal.SetCrtc(int(b.fd.Fd()), saved.ID, saved.FbID, saved.X, saved.Y, mode, nil)
		}
		if b.vt != nil {
			b.vt.Close()
		}
		drminternal.DropMaster(int(b.fd.Fd()))
		b.fd.Close()
	})
	return nil
}

func (b *DRMBackend) Done() <-chan struct{} {
	return b.doneCh
}

func (b *DRMBackend) UseGBM() bool {
	return b.useGBM
}

func (b *DRMBackend) Commitor() *AtomicCommitor {
	return b.commitor
}

var _ platform.Backend = (*DRMBackend)(nil)

func Probe() bool {
	cards := drminternal.ListDevices()
	if len(cards) == 0 {
		return false
	}
	for _, card := range cards {
		fd, err := os.OpenFile(card, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if drminternal.SetMaster(int(fd.Fd())) != nil {
			fd.Close()
			continue
		}
		dumbOK := drminternal.HasDumbBuffer(int(fd.Fd()))
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return dumbOK
	}
	return false
}
