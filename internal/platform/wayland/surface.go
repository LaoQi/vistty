package wayland

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/LaoQi/vistty/internal/platform"
	"golang.org/x/sys/unix"
)

type shmBuf struct {
	fd     int
	data   []byte
	size   int
	pool   *wlShmPool
	buffer *wlBuffer
}

type WaylandSurface struct {
	backend      *WaylandBackend
	wlSurface    *wlSurface
	xdgSurface   *wlXdgSurface
	toplevel     *wlXdgToplevel
	toplevelDeco *zxdgToplevelDecorationV1

	mu       sync.Mutex
	width    int
	height   int
	stride   int
	bufs     [2]shmBuf
	front    int
	swapBR   bool
	decoMode uint32

	resizeCh chan platform.ResizeEvent
}

func newWaylandSurface(backend *WaylandBackend, width, height int) (*WaylandSurface, error) {
	s := &WaylandSurface{
		backend:  backend,
		width:    width,
		height:   height,
		stride:   width * 4,
		resizeCh: make(chan platform.ResizeEvent, 4),
		swapBR:   backend.swapBR,
	}

	s.wlSurface = backend.compositor.createSurface()

	s.xdgSurface = backend.wmBase.getXdgSurface(s.wlSurface)

	s.toplevel = s.xdgSurface.getToplevel()
	s.toplevel.onClose = func() {
		s.backend.notifyClose()
	}

	s.toplevel.setTitle("vistty")
	s.toplevel.setAppId("github.com.LaoQi.vistty")

	if s.backend.decoMgr != nil {
		s.toplevelDeco = s.backend.decoMgr.getToplevelDecoration(s.toplevel)
		s.decoMode = decoModeServerSide
		s.toplevelDeco.onConfigure = func(mode uint32) {
			s.mu.Lock()
			s.decoMode = mode
			s.mu.Unlock()
		}
		s.toplevelDeco.setMode(decoModeServerSide)
	}

	bufSize := s.stride * height
	for i := 0; i < 2; i++ {
		buf, err := createShmBuf(backend.shm, bufSize, width, height, s.stride, backend.shmFormat)
		if err != nil {
			s.closeBufs(i)
			s.toplevel.destroy()
			s.xdgSurface.destroy()
			s.wlSurface.destroy()
			return nil, fmt.Errorf("create shm buffer %d: %w", i, err)
		}
		s.bufs[i] = buf
	}

	configureCh := make(chan uint32, 1)

	s.xdgSurface.onConfigure = func(serial uint32) {
		select {
		case configureCh <- serial:
		default:
		}
	}

	s.toplevel.onConfigure = func(w, h int32) {
		if w > 0 && h > 0 && (int(w) != s.width || int(h) != s.height) {
			s.resize(int(w), int(h))
		}
	}

	s.wlSurface.commit()

	for {
		if err := backend.c.dispatch(); err != nil {
			s.closeBufs(2)
			s.toplevel.destroy()
			s.xdgSurface.destroy()
			s.wlSurface.destroy()
			return nil, backend.wrapErr("dispatch waiting for configure", err)
		}
		select {
		case serial := <-configureCh:
			s.xdgSurface.ackConfigure(serial)
			goto configured
		default:
		}
	}
configured:

	s.xdgSurface.onConfigure = func(serial uint32) {
		s.xdgSurface.ackConfigure(serial)
	}

	s.xdgSurface.setWindowGeometry(0, 0, int32(width), int32(height))

	return s, nil
}

func (s *WaylandSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.width, s.height
}

func (s *WaylandSurface) Data() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	backIdx := s.front ^ 1
	return s.bufs[backIdx].data
}

func (s *WaylandSurface) DirectRender() bool {
	return true
}

func (s *WaylandSurface) DecoMode() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decoMode
}

func (s *WaylandSurface) Stride() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	backIdx := s.front ^ 1
	return s.bufs[backIdx].size / s.height
}

func (s *WaylandSurface) Swap() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backIdx := s.front ^ 1
	buf := &s.bufs[backIdx]

	if s.swapBR {
		data := buf.data
		for i := 0; i+4 <= len(data); i += 4 {
			v := *(*uint32)(unsafe.Pointer(&data[i]))
			v = (v & 0xFF00FF00) | ((v >> 16) & 0xFF) | ((v & 0xFF) << 16)
			*(*uint32)(unsafe.Pointer(&data[i])) = v
		}
	}

	s.wlSurface.attach(buf.buffer, 0, 0)
	s.wlSurface.damage(0, 0, int32(s.width), int32(s.height))
	s.wlSurface.commit()

	s.front = backIdx
	return nil
}

func (s *WaylandSurface) Close() error {
	s.closeBufs(2)
	if s.toplevelDeco != nil {
		s.toplevelDeco.destroy()
	}
	if s.toplevel != nil {
		s.toplevel.destroy()
	}
	if s.xdgSurface != nil {
		s.xdgSurface.destroy()
	}
	if s.wlSurface != nil {
		s.wlSurface.destroy()
	}
	return nil
}

func (s *WaylandSurface) closeBufs(upTo int) {
	for i := 0; i < upTo; i++ {
		b := &s.bufs[i]
		if b.data != nil {
			unix.Munmap(b.data)
			b.data = nil
		}
		if b.buffer != nil {
			b.buffer.destroy()
			b.buffer = nil
		}
		if b.pool != nil {
			b.pool.destroy()
			b.pool = nil
		}
		if b.fd >= 0 {
			unix.Close(b.fd)
			b.fd = -1
		}
	}
}

func (s *WaylandSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return s.resizeCh
}

func (s *WaylandSurface) OutputID() uint32 {
	return 0
}

func (s *WaylandSurface) resize(newWidth, newHeight int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newWidth <= 0 || newHeight <= 0 {
		return
	}

	oldBufs := s.bufs
	s.width = newWidth
	s.height = newHeight
	s.stride = newWidth * 4

	bufSize := s.stride * newHeight
	for i := 0; i < 2; i++ {
		buf, err := createShmBuf(s.backend.shm, bufSize, newWidth, newHeight, s.stride, s.backend.shmFormat)
		if err != nil {
			s.bufs = oldBufs
			return
		}
		s.bufs[i] = buf
	}

	for i := 0; i < 2; i++ {
		if oldBufs[i].data != nil {
			unix.Munmap(oldBufs[i].data)
		}
		if oldBufs[i].buffer != nil {
			oldBufs[i].buffer.destroy()
		}
		if oldBufs[i].pool != nil {
			oldBufs[i].pool.destroy()
		}
		if oldBufs[i].fd >= 0 {
			unix.Close(oldBufs[i].fd)
		}
	}

	s.xdgSurface.setWindowGeometry(0, 0, int32(newWidth), int32(newHeight))

	select {
	case s.resizeCh <- platform.ResizeEvent{Width: newWidth, Height: newHeight}:
	default:
	}
}

func createShmBuf(shm *wlShm, size, width, height, stride int, format uint32) (shmBuf, error) {
	fd, err := unix.MemfdCreate("vistty-wl-shm", unix.MFD_CLOEXEC)
	if err != nil {
		return shmBuf{}, fmt.Errorf("memfd_create: %w", err)
	}

	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("ftruncate: %w", err)
	}

	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("mmap: %w", err)
	}

	pool := shm.createPool(fd, int32(size))
	buffer := pool.createBuffer(0, int32(width), int32(height), int32(stride), format)

	return shmBuf{
		fd:     fd,
		data:   data,
		size:   size,
		pool:   pool,
		buffer: buffer,
	}, nil
}

var _ platform.Surface = (*WaylandSurface)(nil)
