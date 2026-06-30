package drm

import (
	"fmt"
	"os"
	"sync"

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

	gbmProvider platform.GBMProvider
}

func NewDRMBackend(ttyPath string) (*DRMBackend, error) {
	cards := ListDevices()
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

	if err := SetMaster(int(fd.Fd())); err != nil {
		fd.Close()
		return nil, fmt.Errorf("set master: %w", err)
	}

	if !HasDumbBuffer(int(fd.Fd())) {
		DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("device does not support dumb buffers")
	}

	outputs, err := findOutputs(int(fd.Fd()))
	if err != nil {
		DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, err
	}

	b := &DRMBackend{
		fd:        fd,
		display:   outputs[0],
		outputs:   outputs,
		surfaces:  make(map[uint32]*DRMSurface),
		doneCh:    make(chan struct{}),
		eventDone: make(chan struct{}),
	}

	vt, err := newVTManager(VTCallbacks{
		OnActivate: func() {
			SetMaster(int(fd.Fd()))
			if b.surface != nil {
				b.surface.SetActive(true)
			}
			if b.gbmProvider != nil {
				b.gbmProvider.SetActive(true)
			}
		},
		OnDeactivate: func() {
			if b.surface != nil {
				b.surface.SetActive(false)
			}
			if b.gbmProvider != nil {
				b.gbmProvider.SetActive(false)
			}
			DropMaster(int(fd.Fd()))
		},
	}, ttyPath)
	if err != nil {
		DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("vt manager: %w", err)
	}
	b.vt = vt

	if vt != nil {
		if err := vt.SetGraphicsMode(); err != nil {
			vt.Close()
			DropMaster(int(fd.Fd()))
			fd.Close()
			return nil, fmt.Errorf("set graphics mode: %w", err)
		}
	}

	return b, nil
}

func (b *DRMBackend) CreateSurface(width, height int) (platform.Surface, error) {
	width = int(b.display.mode.HDisplay)
	height = int(b.display.mode.VDisplay)

	if b.gbmProvider != nil {
		return b.gbmProvider.CreateSurfaceForOutput(b.display)
	}

	surf, err := newDRMSurface(int(b.fd.Fd()), width, height, b.display.crtcID, b.display.connID)
	if err != nil {
		return nil, err
	}
	b.surface = surf
	b.surfaces[b.display.crtcID] = surf

	connIDs := []uint32{b.display.connID}
	if err := SetCrtc(int(b.fd.Fd()), b.display.crtcID, surf.bufs[surf.current].fbID, 0, 0, &b.display.mode, connIDs); err != nil {
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

	if b.gbmProvider != nil {
		return b.gbmProvider.CreateSurfaceForOutput(out)
	}

	width := int(di.mode.HDisplay)
	height := int(di.mode.VDisplay)

	surf, err := newDRMSurface(int(b.fd.Fd()), width, height, di.crtcID, di.connID)
	if err != nil {
		return nil, err
	}
	b.surfaces[di.crtcID] = surf
	if b.surface == nil {
		b.surface = surf
	}

	connIDs := []uint32{di.connID}
	if err := SetCrtc(int(b.fd.Fd()), di.crtcID, surf.bufs[surf.current].fbID, 0, 0, &di.mode, connIDs); err != nil {
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
	reader := NewEventReader(int(b.fd.Fd()))
	for {
		ev, err := reader.ReadEvent()
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
		if ev != nil && ev.Type == EventFlipComplete {
			if b.gbmProvider != nil {
				b.gbmProvider.HandleFlipEvent(ev.CrtcID)
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
		if b.gbmProvider != nil {
			b.gbmProvider.Close()
			b.gbmProvider = nil
		}
		for _, out := range b.outputs {
			if out.savedCrtc != nil {
				saved := out.savedCrtc
				var mode *ModeInfoPublic
				if saved.ModeValid {
					mode = &saved.Mode
				}
				SetCrtc(int(b.fd.Fd()), saved.ID, saved.FbID, saved.X, saved.Y, mode, nil)
			}
		}
		if b.vt != nil {
			b.vt.Close()
		}
		DropMaster(int(b.fd.Fd()))
		b.fd.Close()
	})
	return nil
}

func (b *DRMBackend) Done() <-chan struct{} {
	return b.doneCh
}

func (b *DRMBackend) SetGBMProvider(p platform.GBMProvider) {
	b.gbmProvider = p
}

func (b *DRMBackend) FD() int {
	return int(b.fd.Fd())
}

var _ platform.Backend = (*DRMBackend)(nil)

func Probe() bool {
	fd, err := ProbeDetailed()
	if err != nil {
		return false
	}
	fd.Close()
	return true
}

type ProbeResult struct {
	FD        *os.File
	HasDumb   bool
	Outputs   []*DisplayInfo
}

func ProbeDetailed() (*os.File, error) {
	cards := ListDevices()
	if len(cards) == 0 {
		return nil, fmt.Errorf("no DRM device found")
	}

	for _, card := range cards {
		fd, err := os.OpenFile(card, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if SetMaster(int(fd.Fd())) != nil {
			fd.Close()
			continue
		}
		if !HasDumbBuffer(int(fd.Fd())) {
			DropMaster(int(fd.Fd()))
			fd.Close()
			continue
		}
		return fd, nil
	}
	return nil, fmt.Errorf("no usable DRM device")
}

func NewDRMBackendFromFD(fd *os.File, ttyPath string) (*DRMBackend, error) {
	if fd == nil {
		return nil, fmt.Errorf("fd is nil")
	}

	outputs, err := findOutputs(int(fd.Fd()))
	if err != nil {
		DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, err
	}

	b := &DRMBackend{
		fd:       fd,
		display:  outputs[0],
		outputs:  outputs,
		surfaces: make(map[uint32]*DRMSurface),
		doneCh:   make(chan struct{}),
		eventDone: make(chan struct{}),
	}

	vt, err := newVTManager(VTCallbacks{
		OnActivate: func() {
			SetMaster(int(fd.Fd()))
			if b.surface != nil {
				b.surface.SetActive(true)
			}
			if b.gbmProvider != nil {
				b.gbmProvider.SetActive(true)
			}
		},
		OnDeactivate: func() {
			if b.surface != nil {
				b.surface.SetActive(false)
			}
			if b.gbmProvider != nil {
				b.gbmProvider.SetActive(false)
			}
			DropMaster(int(fd.Fd()))
		},
	}, ttyPath)
	if err != nil {
		DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("vt manager: %w", err)
	}
	b.vt = vt

	if vt != nil {
		if err := vt.SetGraphicsMode(); err != nil {
			vt.Close()
			DropMaster(int(fd.Fd()))
			fd.Close()
			return nil, fmt.Errorf("set graphics mode: %w", err)
		}
	}

	return b, nil
}
