package wayland

import (
	"fmt"
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/rajveermalviya/go-wayland/wayland/stable/xdg-shell"
	"golang.org/x/sys/unix"
)

type shmBuf struct {
	fd     int
	data   []byte
	size   int
	pool   *client.ShmPool
	buffer *client.Buffer
}

type WaylandSurface struct {
	ctx        *client.Context
	compositor *client.Compositor
	shm        *client.Shm
	wmBase     *xdg_shell.WmBase
	backend    *WaylandBackend

	wlSurface  *client.Surface
	xdgSurface *xdg_shell.Surface
	toplevel   *xdg_shell.Toplevel

	mu     sync.Mutex
	width  int
	height int
	stride int
	bufs   [2]shmBuf
	front  int

	resizeCh chan platform.ResizeEvent
}

func newWaylandSurface(backend *WaylandBackend, ctx *client.Context, compositor *client.Compositor, shm *client.Shm, wmBase *xdg_shell.WmBase, width, height int) (*WaylandSurface, error) {
	s := &WaylandSurface{
		ctx:        ctx,
		compositor: compositor,
		shm:        shm,
		wmBase:     wmBase,
		backend:    backend,
		width:      width,
		height:     height,
		stride:     width * 4,
		resizeCh:   make(chan platform.ResizeEvent, 4),
	}

	wlSurface, err := compositorCreateSurface(ctx, compositor.ID())
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}
	s.wlSurface = wlSurface

	xdgSurface, err := xdgWmBaseGetXdgSurface(ctx, wmBase.ID(), wlSurface.ID())
	if err != nil {
		wlSurface.Destroy()
		return nil, fmt.Errorf("get xdg surface: %w", err)
	}
	s.xdgSurface = xdgSurface

	toplevel, err := xdgSurfaceGetToplevel(ctx, xdgSurface.ID())
	if err != nil {
		xdgSurface.Destroy()
		wlSurface.Destroy()
		return nil, fmt.Errorf("get toplevel: %w", err)
	}
	s.toplevel = toplevel

	toplevel.SetCloseHandler(func(xdg_shell.ToplevelCloseEvent) {
		s.backend.notifyClose()
	})

	_ = toplevelSetTitle(toplevel, "vistty")
	_ = toplevelSetAppId(toplevel, "github.com.LaoQi.vistty")

	bufSize := s.stride * height

	for i := 0; i < 2; i++ {
		buf, err := createShmBuf(shm, bufSize, width, height, s.stride, backend.shmFormat)
		if err != nil {
			s.closeBufs(i)
			s.toplevel.Destroy()
			s.xdgSurface.Destroy()
			s.wlSurface.Destroy()
			return nil, fmt.Errorf("create shm buffer %d: %w", i, err)
		}
		s.bufs[i] = buf
	}

	configureCh := make(chan uint32, 1)

	xdgSurface.SetConfigureHandler(func(e xdg_shell.SurfaceConfigureEvent) {
		select {
		case configureCh <- e.Serial:
		default:
		}
	})

	toplevel.SetConfigureHandler(func(e xdg_shell.ToplevelConfigureEvent) {
		if e.Width > 0 && e.Height > 0 && (int(e.Width) != s.width || int(e.Height) != s.height) {
			s.resize(int(e.Width), int(e.Height))
		}
	})

	_ = wlSurface.Commit()

	for {
		if err := ctx.Dispatch(); err != nil {
			s.closeBufs(2)
			s.toplevel.Destroy()
			s.xdgSurface.Destroy()
			s.wlSurface.Destroy()
			return nil, backend.wrapDispatchErr("dispatch waiting for configure", err)
		}
		select {
		case serial := <-configureCh:
			_ = xdgSurface.AckConfigure(serial)
			goto configured
		default:
		}
	}
configured:

	xdgSurface.SetConfigureHandler(func(e xdg_shell.SurfaceConfigureEvent) {
		_ = xdgSurface.AckConfigure(e.Serial)
	})

	_ = xdgSurface.SetWindowGeometry(0, 0, int32(width), int32(height))

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

	if err := s.wlSurface.Attach(buf.buffer, 0, 0); err != nil {
		return fmt.Errorf("attach buffer: %w", err)
	}
	if err := s.wlSurface.Damage(0, 0, int32(s.width), int32(s.height)); err != nil {
		return fmt.Errorf("damage: %w", err)
	}
	if err := s.wlSurface.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	s.front = backIdx
	return nil
}

func (s *WaylandSurface) Close() error {
	s.closeBufs(2)
	if s.toplevel != nil {
		s.toplevel.Destroy()
	}
	if s.xdgSurface != nil {
		s.xdgSurface.Destroy()
	}
	if s.wlSurface != nil {
		s.wlSurface.Destroy()
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
			b.buffer.Destroy()
			b.buffer = nil
		}
		if b.pool != nil {
			b.pool.Destroy()
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
		buf, err := createShmBuf(s.shm, bufSize, newWidth, newHeight, s.stride, s.backend.shmFormat)
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
			oldBufs[i].buffer.Destroy()
		}
		if oldBufs[i].pool != nil {
			oldBufs[i].pool.Destroy()
		}
		if oldBufs[i].fd >= 0 {
			unix.Close(oldBufs[i].fd)
		}
	}

	_ = s.xdgSurface.SetWindowGeometry(0, 0, int32(newWidth), int32(newHeight))

	select {
	case s.resizeCh <- platform.ResizeEvent{Width: newWidth, Height: newHeight}:
	default:
	}
}

func createShmBuf(shm *client.Shm, size, width, height, stride int, format uint32) (shmBuf, error) {
	fd, err := unix.MemfdCreate("vistty-wl-shm", unix.MFD_CLOEXEC)
	if err != nil {
		return shmBuf{}, fmt.Errorf("memfd_create: %w", err)
	}

	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("ftruncate: %w", err)
	}

	// No sealing - some compositors may have issues with sealed memfds

	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("mmap: %w", err)
	}

	pool, err := shmCreatePool(shm.Context(), shm.ID(), fd, int32(size))
	if err != nil {
		unix.Munmap(data)
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("create pool: %w", err)
	}

	buffer, err := shmPoolCreateBuffer(shm.Context(), pool.ID(), 0, int32(width), int32(height), int32(stride), format)
	if err != nil {
		pool.Destroy()
		unix.Munmap(data)
		unix.Close(fd)
		return shmBuf{}, fmt.Errorf("create buffer: %w", err)
	}

	return shmBuf{
		fd:     fd,
		data:   data,
		size:   size,
		pool:   pool,
		buffer: buffer,
	}, nil
}

var _ platform.Surface = (*WaylandSurface)(nil)
