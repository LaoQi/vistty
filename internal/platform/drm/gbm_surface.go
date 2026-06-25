package drm

import (
	"fmt"
	"sync"
	"time"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform"
)

type GBMSurface struct {
	device      *GBMDevice
	commitor    *AtomicCommitor
	info        *surfaceAtomicInfo
	gbmSurface  uintptr
	eglSurface  uintptr
	width       int
	height      int
	crtcID      uint32
	connectorID uint32

	mu          sync.Mutex
	active      bool
	flipCh      chan struct{}

	currentBO   uintptr
	currentFB    uint32
	currentStride uint32
	closed       bool
}

func (s *GBMSurface) Size() (int, int) {
	return s.width, s.height
}

func (s *GBMSurface) Data() []byte {
	return nil
}

func (s *GBMSurface) Stride() int {
	return int(s.currentStride)
}

func (s *GBMSurface) Swap() error {
	s.mu.Lock()
	if !s.active || s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if err := s.device.eglLoader.SwapBuffers(s.device.eglDisplay, s.eglSurface); err != nil {
		return fmt.Errorf("eglSwapBuffers: %w", err)
	}

	bo := s.device.gbmLoader.SurfaceLockFrontBuffer(s.gbmSurface)
	if bo == 0 {
		return fmt.Errorf("gbm_surface_lock_front_buffer returned NULL")
	}

	handle := s.device.gbmLoader.BOGetHandle(bo)
	stride := s.device.gbmLoader.BOGetStride(bo)

	fbID, err := drminternal.AddFB(
		s.device.fd,
		uint16(s.width), uint16(s.height),
		24, 32,
		stride, handle,
	)
	if err != nil {
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
		return fmt.Errorf("drmModeAddFB: %w", err)
	}

	modeset := !s.info.modesetDone
	if err := s.commitor.CommitSingle(s.info, fbID, modeset); err != nil {
		drminternal.RmFB(s.device.fd, fbID)
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
		return fmt.Errorf("atomic commit: %w", err)
	}

	select {
	case <-s.flipCh:
	case <-time.After(200 * time.Millisecond):
	}

	oldBO := s.currentBO
	oldFB := s.currentFB

	s.mu.Lock()
	s.currentBO = bo
	s.currentFB = fbID
	s.currentStride = stride
	s.mu.Unlock()

	if oldBO != 0 {
		if oldFB != 0 {
			drminternal.RmFB(s.device.fd, oldFB)
		}
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, oldBO)
	}

	return nil
}

func (s *GBMSurface) notifyFlip() {
	select {
	case s.flipCh <- struct{}{}:
	default:
	}
}

func (s *GBMSurface) SetActive(active bool) {
	s.mu.Lock()
	s.active = active
	s.mu.Unlock()
}

func (s *GBMSurface) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.active = false
	s.mu.Unlock()

	if s.currentBO != 0 {
		if s.currentFB != 0 {
			drminternal.RmFB(s.device.fd, s.currentFB)
			s.currentFB = 0
		}
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, s.currentBO)
		s.currentBO = 0
	}

	if s.eglSurface != 0 {
		s.device.eglLoader.DestroySurface(s.device.eglDisplay, s.eglSurface)
		s.eglSurface = 0
	}
	if s.gbmSurface != 0 {
		s.device.gbmLoader.SurfaceDestroy(s.gbmSurface)
		s.gbmSurface = 0
	}

	return nil
}

func (s *GBMSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return nil
}

func (s *GBMSurface) OutputID() uint32 {
	return s.connectorID
}

func (s *GBMSurface) CrtcID() uint32 {
	return s.crtcID
}

var _ platform.Surface = (*GBMSurface)(nil)
