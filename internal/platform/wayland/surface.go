package wayland

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"golang.org/x/sys/unix"
)

type shmBuf struct {
	fd       int
	data     []byte
	size     int
	pool     *wlShmPool
	buffer   *wlBuffer
	released *bool // 与 buffer.onRelease 共享；true=合成器已释放，可安全覆写
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

	// pendingW/pendingH: onConfigure 记录的待应用尺寸，由 Data() 在渲染线程消费。
	// 避免 dispatch 线程直接替换 buffer 导致与渲染线程的 use-after-munmap 竞争。
	pendingW int
	pendingH int

	resizeCh chan platform.ResizeEvent
	outputID uint32
}

func newWaylandSurface(backend *WaylandBackend, width, height int, outputID uint32) (*WaylandSurface, error) {
	s := &WaylandSurface{
		backend:  backend,
		width:    width,
		height:   height,
		stride:   width * 4,
		resizeCh: make(chan platform.ResizeEvent, 4),
		swapBR:   backend.swapBR,
		outputID: outputID,
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
		buf, err := createShmBuf(backend.shm, bufSize, width, height, s.stride, backend.shmFormat, &s.mu)
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
		if w > 0 && h > 0 {
			// 两阶段 resize：dispatch 线程只记录 pending 尺寸 + 发送 ResizeEvent。
			// buffer 替换延迟到渲染线程 Data() 中执行，避免与渲染线程整帧写入的 use-after-munmap 竞争。
			s.mu.Lock()
			s.pendingW = int(w)
			s.pendingH = int(h)
			s.mu.Unlock()
			select {
			case s.resizeCh <- platform.ResizeEvent{Width: int(w), Height: int(h), OutputID: s.outputID}:
			default:
			}
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
	// 在渲染线程串行应用 pending resize：buffer 替换在此执行，
	// 旧 buffer Munmap 时无其他 goroutine 在写，消除 use-after-munmap 竞争。
	if s.pendingW > 0 && s.pendingH > 0 {
		s.applyResizeLocked(s.pendingW, s.pendingH)
		s.pendingW, s.pendingH = 0, 0
	}
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

	// 跟踪 wl_buffer.release：若合成器尚未释放 back buffer，跳过本帧 Swap，
	// 避免覆写合成器正读取的 buffer 导致撕裂。
	if buf.released == nil || !*buf.released {
		debug.Warningf("wayland: swap skipped: back buffer not released yet (%dx%d)", s.width, s.height)
		return nil
	}

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

	*buf.released = false
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
		destroyShmBuf(&s.bufs[i])
	}
}

// destroyShmBuf 释放单个 shmBuf 的全部资源（munmap + destroy buffer/pool + close fd）。
func destroyShmBuf(b *shmBuf) {
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
	b.released = nil
}

func (s *WaylandSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return s.resizeCh
}

func (s *WaylandSurface) OutputID() uint32 {
	return s.outputID
}

func (s *WaylandSurface) StartMove(serial uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toplevel != nil && s.backend.seat != nil {
		s.toplevel.move(s.backend.seat.id, serial)
	}
}

func (s *WaylandSurface) StartResize(serial uint32, edge uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toplevel != nil && s.backend.seat != nil {
		s.toplevel.resize(s.backend.seat.id, serial, edge)
	}
}

// applyResizeLocked 在渲染线程 Data() 中持 s.mu 调用，执行 buffer 替换。
// 两阶段 resize 的第二阶段：创建新 buffer -> 替换 -> 销毁旧 buffer。
// P1-20: 先创建到临时变量，任一失败则销毁已创建的，保留旧 bufs 不动。
func (s *WaylandSurface) applyResizeLocked(newWidth, newHeight int) {
	if newWidth <= 0 || newHeight <= 0 {
		return
	}
	if newWidth == s.width && newHeight == s.height && s.bufs[0].buffer != nil {
		return
	}

	newStride := newWidth * 4
	bufSize := newStride * newHeight

	var newBufs [2]shmBuf
	created := 0
	var createErr error
	for i := 0; i < 2; i++ {
		buf, err := createShmBuf(s.backend.shm, bufSize, newWidth, newHeight, newStride, s.backend.shmFormat, &s.mu)
		if err != nil {
			createErr = err
			break
		}
		newBufs[i] = buf
		created++
	}

	if createErr != nil {
		for i := 0; i < created; i++ {
			destroyShmBuf(&newBufs[i])
		}
		debug.Warningf("wayland: resize to %dx%d failed: %v (keeping %dx%d)", newWidth, newHeight, createErr, s.width, s.height)
		return
	}

	oldBufs := s.bufs
	s.width = newWidth
	s.height = newHeight
	s.stride = newStride
	s.bufs = newBufs

	for i := 0; i < 2; i++ {
		destroyShmBuf(&oldBufs[i])
	}

	s.xdgSurface.setWindowGeometry(0, 0, int32(newWidth), int32(newHeight))
}

func createShmBuf(shm *wlShm, size, width, height, stride int, format uint32, mu *sync.Mutex) (shmBuf, error) {
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

	// released 标志由 shmBuf 和 buffer.onRelease 共享（*bool 指针）。
	// 新 buffer 初始 released=true（空闲可写）。
	// onRelease 在 dispatch 线程触发，通过 mu 保护与渲染线程 Swap() 的读写同步。
	rel := new(bool)
	*rel = true
	buffer.onRelease = func() {
		mu.Lock()
		*rel = true
		mu.Unlock()
	}

	return shmBuf{
		fd:       fd,
		data:     data,
		size:     size,
		pool:     pool,
		buffer:   buffer,
		released: rel,
	}, nil
}

var _ platform.Surface = (*WaylandSurface)(nil)
var _ platform.WindowMover = (*WaylandSurface)(nil)
