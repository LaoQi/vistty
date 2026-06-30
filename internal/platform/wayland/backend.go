package wayland

import (
	"fmt"
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
)

// FourCC 格式码（Wayland wl_shm 规范值）
const (
	wlFmtXRGB8888 uint32 = 0x34325258
	wlFmtARGB8888 uint32 = 0x34325241
	wlFmtXBGR8888 uint32 = 0x34324258
	wlFmtABGR8888 uint32 = 0x34324241
	wlFmtBGRX8888 uint32 = 0x34325842
	wlFmtBGRA8888 uint32 = 0x34324142

	// niri 对 XRGB8888/ARGB8888 广播枚举索引(1/0)而非 FourCC 码。
	// 需同时匹配两种值，create_buffer 发送时用合成器广播的原值。
	wlEnumARGB8888 uint32 = 0
	wlEnumXRGB8888 uint32 = 1
)

type WaylandBackend struct {
	c          *conn
	registry   *wlRegistry
	compositor *wlCompositor
	shm        *wlShm
	wmBase     *wlXdgWmBase
	seat       *wlSeat
	decoMgr    *zxdgDecorationManagerV1
	shmFormat  uint32
	swapBR     bool
	savedCap   uint32
	input      *WaylandInput

	mu        sync.Mutex
	closed    bool
	doneCh    chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
	lastErr   string
}

func NewWaylandBackend() (*WaylandBackend, error) {
	c, err := dial()
	if err != nil {
		return nil, fmt.Errorf("connect to wayland: %w", err)
	}

	b := &WaylandBackend{
		c:      c,
		doneCh: make(chan struct{}),
	}

	c.setErrorHandler(func(objID, code uint32, msg string) {
		b.mu.Lock()
		b.lastErr = fmt.Sprintf("object %d code %d: %s", objID, code, msg)
		b.mu.Unlock()
		b.notifyClose()
	})

	registry := c.getRegistry()
	b.registry = registry

	var compositorName, shmName, wmBaseName, seatName, decoMgrName uint32
	var compositorVersion, seatVersion, wmBaseVersion, decoMgrVersion uint32

	registry.onGlobal = func(name uint32, iface string, version uint32) {
		switch iface {
		case "wl_compositor":
			compositorName = name
			compositorVersion = version
		case "wl_shm":
			shmName = name
		case "xdg_wm_base":
			wmBaseName = name
			wmBaseVersion = version
		case "wl_seat":
			seatName = name
			seatVersion = version
		case "zxdg_decoration_manager_v1":
			decoMgrName = name
			decoMgrVersion = version
		}
	}

	if err := c.roundtrip(); err != nil {
		b.close()
		return nil, fmt.Errorf("registry roundtrip: %w", err)
	}

	if compositorName == 0 {
		b.close()
		return nil, fmt.Errorf("wl_compositor not available")
	}
	if shmName == 0 {
		b.close()
		return nil, fmt.Errorf("wl_shm not available")
	}
	if wmBaseName == 0 {
		b.close()
		return nil, fmt.Errorf("xdg_wm_base not available")
	}
	if seatName == 0 {
		b.close()
		return nil, fmt.Errorf("wl_seat not available")
	}

	if compositorVersion > 4 {
		compositorVersion = 4
	}
	if seatVersion > 5 {
		seatVersion = 5
	}
	if wmBaseVersion > 1 {
		wmBaseVersion = 1
	}

	b.compositor = c.bindCompositor(registry, compositorName, compositorVersion)
	b.shm = c.bindShm(registry, shmName)

	formats := make(map[uint32]bool)
	b.shm.onFormat = func(format uint32) {
		formats[format] = true
	}

	if err := c.roundtrip(); err != nil {
		b.close()
		return nil, b.wrapErr("shm roundtrip", err)
	}

	// 优先 RGB 序格式以避免 Surface.Swap 中的逐像素 B/R 交换。
	// XRGB8888 是所有 Wayland 合成器的必备格式，必然可用。
	// niri 对 XRGB/ARGB 广播枚举索引(1/0)，需同时匹配；create_buffer 用原值发送。
	hasFmt := func(vs ...uint32) bool {
		for _, v := range vs {
			if formats[v] {
				return true
			}
		}
		return false
	}
	switch {
	case hasFmt(wlFmtXRGB8888, wlEnumXRGB8888):
		b.shmFormat = wlEnumXRGB8888 // 用枚举值发送
	case hasFmt(wlFmtARGB8888, wlEnumARGB8888):
		b.shmFormat = wlEnumARGB8888
	case formats[wlFmtXBGR8888]:
		b.shmFormat = wlFmtXBGR8888
		b.swapBR = true
	case formats[wlFmtABGR8888]:
		b.shmFormat = wlFmtABGR8888
		b.swapBR = true
	case formats[wlFmtBGRX8888]:
		b.shmFormat = wlFmtBGRX8888
		b.swapBR = true
	case formats[wlFmtBGRA8888]:
		b.shmFormat = wlFmtBGRA8888
		b.swapBR = true
	default:
		b.shmFormat = wlFmtXRGB8888
	}

	b.wmBase = c.bindWmBase(registry, wmBaseName, wmBaseVersion)
	b.seat = c.bindSeat(registry, seatName, seatVersion)
	b.seat.onCapabilities = func(cap uint32) {
		b.savedCap = cap
		if b.input != nil {
			b.input.HandleCapabilities(cap)
		}
	}

	if decoMgrName != 0 {
		if decoMgrVersion > 2 {
			decoMgrVersion = 2
		}
		b.decoMgr = c.bindDecoManager(registry, decoMgrName, decoMgrVersion)
	}

	return b, nil
}

func (b *WaylandBackend) CreateSurface(width, height int) (platform.Surface, error) {
	if width <= 0 || height <= 0 {
		width = 800
		height = 600
	}
	return newWaylandSurface(b, width, height)
}

type waylandOutput struct {
	id     uint32
	crtcID uint32
	name   string
	w      int
	h      int
}

func (o *waylandOutput) ID() uint32          { return o.id }
func (o *waylandOutput) ConnectorID() uint32 { return o.id }
func (o *waylandOutput) CrtcID() uint32      { return o.crtcID }
func (o *waylandOutput) Name() string        { return o.name }
func (o *waylandOutput) Size() (int, int)    { return o.w, o.h }

var _ platform.Output = (*waylandOutput)(nil)

func (b *WaylandBackend) ListOutputs() ([]platform.Output, error) {
	return []platform.Output{&waylandOutput{name: "wayland"}}, nil
}

func (b *WaylandBackend) CreateSurfaceFor(out platform.Output) (platform.Surface, error) {
	w, h := out.Size()
	return b.CreateSurface(w, h)
}

func (b *WaylandBackend) CreateInputSource() (platform.InputSource, error) {
	input, err := newWaylandInput(b.c, b.seat)
	if err != nil {
		return nil, err
	}
	b.input = input
	if b.savedCap != 0 {
		input.HandleCapabilities(b.savedCap)
	}
	return input, nil
}

func (b *WaylandBackend) Run(fn func()) {
	fn()
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()

		if err := b.c.dispatch(); err != nil {
			b.notifyClose()
			return
		}
	}
}

func (b *WaylandBackend) Stop() error {
	b.stopOnce.Do(func() {
		if b.c != nil {
			_ = b.c.close()
		}
	})
	return nil
}

func (b *WaylandBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()

	select {
	case <-b.doneCh:
	default:
		close(b.doneCh)
	}
	b.Stop()
	return b.close()
}

func (b *WaylandBackend) Done() <-chan struct{} {
	return b.doneCh
}

func (b *WaylandBackend) lastError() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastErr
}

func (b *WaylandBackend) wrapErr(prefix string, err error) error {
	if msg := b.lastError(); msg != "" {
		return fmt.Errorf("%s: %w (compositor error: %s)", prefix, err, msg)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func (b *WaylandBackend) notifyClose() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()

	select {
	case <-b.doneCh:
	default:
		close(b.doneCh)
	}
}

func (b *WaylandBackend) close() error {
	b.closeOnce.Do(func() {
		if b.seat != nil {
			b.seat.release()
		}
		if b.decoMgr != nil {
			b.decoMgr.destroy()
		}
		if b.wmBase != nil {
			b.wmBase.destroy()
		}
		if b.shm != nil {
			b.shm.destroy()
		}
		if b.compositor != nil {
			b.compositor.destroy()
		}
	})
	return nil
}

var _ platform.Backend = (*WaylandBackend)(nil)

func Probe() bool {
	c, err := dial()
	if err != nil {
		return false
	}
	_ = c.close()
	return true
}
