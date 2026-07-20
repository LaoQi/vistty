package drm

import (
	"fmt"
	"sync"
	"time"

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
	flipPending bool
	done        chan struct{}
	closeOnce   sync.Once
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
		done:        make(chan struct{}),
	}

	for i := 0; i < 2; i++ {
		db, err := CreateDumbBuffer(fd, uint32(width), uint32(height), 32)
		if err != nil {
			s.closeBufs(i)
			return nil, fmt.Errorf("create dumb buffer %d: %w", i, err)
		}

		fbID, err := AddFB(fd, uint16(width), uint16(height), 24, 32, db.Pitch, db.Handle)
		if err != nil {
			DestroyDumbBuffer(fd, db.Handle)
			s.closeBufs(i)
			return nil, fmt.Errorf("addfb %d: %w", i, err)
		}

		offset, err := MapDumbBuffer(fd, db.Handle)
		if err != nil {
			RmFB(fd, fbID)
			DestroyDumbBuffer(fd, db.Handle)
			s.closeBufs(i)
			return nil, fmt.Errorf("map dumb buffer %d: %w", i, err)
		}

		data, err := Mmap(fd, offset, db.Size)
		if err != nil {
			RmFB(fd, fbID)
			DestroyDumbBuffer(fd, db.Handle)
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

func (s *DRMSurface) DecoMode() uint32 {
	return 2
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

	if s.flipPending {
		s.waitForFlip()
		s.flipPending = false
	}

	backIdx := s.current ^ 1
	var flipErr error
	for attempt := 0; attempt < 5; attempt++ {
		flipErr = DoPageFlip(s.fd, s.crtcID, s.bufs[backIdx].fbID, FlipEvent, 0)
		if flipErr == nil {
			break
		}
		if attempt < 4 {
			s.waitForFlip()
			s.flipPending = false
		}
	}
	if flipErr != nil {
		return fmt.Errorf("page flip: %w", flipErr)
	}

	s.current = backIdx
	s.flipPending = true
	s.waitForFlip()
	s.flipPending = false

	return nil
}

func (s *DRMSurface) waitForFlip() {
	select {
	case <-s.flipCh:
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
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
	if !active && s.flipPending {
		// VT 切走时 DropMaster 会取消内核 pending page flip 且不再发送
		// flip 事件。若不清 flipPending，切回后首次 Swap 的 waitForFlip 必然
		// 走完 5s 超时。排空 flipCh 防止残留信号误唤醒。
		s.flipPending = false
		select {
		case <-s.flipCh:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *DRMSurface) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		s.closeBufs(2)
	})
	return nil
}

func (s *DRMSurface) closeBufs(upTo int) {
	for i := 0; i < upTo; i++ {
		b := &s.bufs[i]
		if b.fbID != 0 {
			RmFB(s.fd, b.fbID)
			b.fbID = 0
		}
		if b.data != nil {
			Munmap(b.data)
			b.data = nil
		}
		if b.handle != 0 {
			DestroyDumbBuffer(s.fd, b.handle)
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
