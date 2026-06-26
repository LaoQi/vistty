package drm

import (
	"fmt"
	"sync"
	"time"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform"
)

type drmbuf struct {
	fbID   uint32
	handle uint32
	data   []byte
	stride int
	size   uint64
}

type DRMSurface struct {
	fd          int
	width       int
	height      int
	crtcID      uint32
	connectorID uint32
	bufs        [2]drmbuf
	current     int
	mu          sync.Mutex
	active      bool
	flipCh      chan struct{}
}

func newDRMSurface(fd int, width, height int, crtcID, connectorID uint32) (*DRMSurface, error) {
	s := &DRMSurface{
		fd:          fd,
		width:       width,
		height:      height,
		crtcID:      crtcID,
		connectorID: connectorID,
		active:      true,
		flipCh:      make(chan struct{}, 1),
	}

	for i := 0; i < 2; i++ {
		db, err := drminternal.CreateDumbBuffer(fd, uint32(width), uint32(height), 32)
		if err != nil {
			s.closeBufs(i)
			return nil, fmt.Errorf("create dumb buffer %d: %w", i, err)
		}

		fbID, err := drminternal.AddFB(fd, uint16(width), uint16(height), 24, 32, db.Pitch, db.Handle)
		if err != nil {
			drminternal.DestroyDumbBuffer(fd, db.Handle)
			s.closeBufs(i)
			return nil, fmt.Errorf("addfb %d: %w", i, err)
		}

		offset, err := drminternal.MapDumbBuffer(fd, db.Handle)
		if err != nil {
			drminternal.RmFB(fd, fbID)
			drminternal.DestroyDumbBuffer(fd, db.Handle)
			s.closeBufs(i)
			return nil, fmt.Errorf("map dumb buffer %d: %w", i, err)
		}

		data, err := drminternal.Mmap(fd, offset, db.Size)
		if err != nil {
			drminternal.RmFB(fd, fbID)
			drminternal.DestroyDumbBuffer(fd, db.Handle)
			s.closeBufs(i)
			return nil, fmt.Errorf("mmap dumb buffer %d: %w", i, err)
		}

		s.bufs[i] = drmbuf{
			fbID:   fbID,
			handle: db.Handle,
			data:   data,
			stride: int(db.Pitch),
			size:   db.Size,
		}
	}

	return s, nil
}

func (s *DRMSurface) Size() (int, int) {
	return s.width, s.height
}

func (s *DRMSurface) Data() []byte {
	backIdx := s.current ^ 1
	return s.bufs[backIdx].data
}

func (s *DRMSurface) DirectRender() bool {
	return false
}

func (s *DRMSurface) Stride() int {
	backIdx := s.current ^ 1
	return s.bufs[backIdx].stride
}

func (s *DRMSurface) Swap() error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	backIdx := s.current ^ 1
	if err := drminternal.DoPageFlip(s.fd, s.crtcID, s.bufs[backIdx].fbID, drminternal.FlipEvent, 0); err != nil {
		return fmt.Errorf("page flip: %w", err)
	}
	s.current = backIdx

	select {
	case <-s.flipCh:
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

func (s *DRMSurface) notifyFlip() {
	select {
	case s.flipCh <- struct{}{}:
	default:
	}
}

func (s *DRMSurface) SetActive(active bool) {
	s.mu.Lock()
	s.active = active
	s.mu.Unlock()
}

func (s *DRMSurface) Close() error {
	s.closeBufs(2)
	return nil
}

func (s *DRMSurface) closeBufs(upTo int) {
	for i := 0; i < upTo; i++ {
		b := &s.bufs[i]
		if b.fbID != 0 {
			drminternal.RmFB(s.fd, b.fbID)
			b.fbID = 0
		}
		if b.data != nil {
			drminternal.Munmap(b.data)
			b.data = nil
		}
		if b.handle != 0 {
			drminternal.DestroyDumbBuffer(s.fd, b.handle)
			b.handle = 0
		}
	}
}

func (s *DRMSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return nil
}

func (s *DRMSurface) OutputID() uint32 {
	return s.connectorID
}

var _ platform.Surface = (*DRMSurface)(nil)
