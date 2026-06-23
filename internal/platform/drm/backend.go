package drm

import (
	"fmt"
	"os"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform"
)

type DRMBackend struct {
	fd      *os.File
	display *DisplayInfo
	vt      *VTManager
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

	display, err := findDisplay(int(fd.Fd()))
	if err != nil {
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, err
	}

	vt, err := newVTManager(VTCallbacks{
		OnActivate:   func() {},
		OnDeactivate: func() {},
	})
	if err != nil {
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("vt manager: %w", err)
	}

	if err := vt.SetGraphicsMode(); err != nil {
		vt.Close()
		drminternal.DropMaster(int(fd.Fd()))
		fd.Close()
		return nil, fmt.Errorf("set graphics mode: %w", err)
	}

	return &DRMBackend{
		fd:      fd,
		display: display,
		vt:      vt,
	}, nil
}

func (b *DRMBackend) CreateSurface(width, height int) (platform.Surface, error) {
	if width <= 0 || height <= 0 {
		width = int(b.display.Mode.HDisplay)
		height = int(b.display.Mode.VDisplay)
	}

	surf, err := newDRMSurface(int(b.fd.Fd()), width, height, b.display.CrtcID)
	if err != nil {
		return nil, err
	}

	connIDs := []uint32{b.display.ConnectorID}
	if err := drminternal.SetCrtc(int(b.fd.Fd()), b.display.CrtcID, surf.bufs[surf.current].fbID, 0, 0, &b.display.Mode, connIDs); err != nil {
		surf.Close()
		return nil, fmt.Errorf("set crtc: %w", err)
	}

	return surf, nil
}

func (b *DRMBackend) CreateInputSource() (platform.InputSource, error) {
	return newDRMInput()
}

func (b *DRMBackend) Run(fn func()) {
	fn()
}

func (b *DRMBackend) Close() error {
	if b.display != nil && b.display.SavedCrtc != nil {
		saved := b.display.SavedCrtc
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
	return b.fd.Close()
}

var _ platform.Backend = (*DRMBackend)(nil)
