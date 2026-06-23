package wayland

import (
	"fmt"
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/rajveermalviya/go-wayland/wayland/stable/xdg-shell"
)

type WaylandBackend struct {
	ctx        *client.Context
	display    *client.Display
	registry   *client.Registry
	compositor *client.Compositor
	shm        *client.Shm
	wmBase     *xdg_shell.WmBase
	seat       *client.Seat
	shmFormat  uint32

	mu        sync.Mutex
	closed    bool
	doneCh    chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
	lastErr   string
}

func NewWaylandBackend() (*WaylandBackend, error) {
	display, err := client.Connect("")
	if err != nil {
		return nil, fmt.Errorf("connect to wayland: %w", err)
	}

	b := &WaylandBackend{
		ctx:        display.Context(),
		display:    display,
		doneCh:     make(chan struct{}),
		shmFormat:  0xFFFFFFFF,
	}

	display.SetErrorHandler(func(e client.DisplayErrorEvent) {
		b.mu.Lock()
		b.lastErr = e.Message
		b.mu.Unlock()
		b.notifyClose()
	})

	registry, err := display.GetRegistry()
	if err != nil {
		b.close()
		return nil, fmt.Errorf("get registry: %w", err)
	}
	b.registry = registry

	var compositorName uint32
	var shmName uint32
	var wmBaseName uint32
	var seatName uint32
	var compositorVersion uint32
	var seatVersion uint32

	registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "wl_compositor":
			compositorName = e.Name
			compositorVersion = e.Version
		case "wl_shm":
			shmName = e.Name
		case "xdg_wm_base":
			wmBaseName = e.Name
		case "wl_seat":
			seatName = e.Name
			seatVersion = e.Version
		}
	})

	callback, err := display.Sync()
	if err != nil {
		b.close()
		return nil, fmt.Errorf("sync: %w", err)
	}

	done := false
	callback.SetDoneHandler(func(client.CallbackDoneEvent) {
		done = true
	})

	for !done {
		if err := b.ctx.Dispatch(); err != nil {
			b.close()
			return nil, b.wrapDispatchErr("dispatch", err)
		}
	}
	callback.Destroy()

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

	compositor := client.NewCompositor(b.ctx)
	if err := registryBind(b.ctx, registry.ID(), compositorName, "wl_compositor", compositorVersion, compositor.ID()); err != nil {
		b.close()
		return nil, fmt.Errorf("bind compositor: %w", err)
	}
	b.compositor = compositor

	shm := client.NewShm(b.ctx)
	if err := registryBind(b.ctx, registry.ID(), shmName, "wl_shm", 1, shm.ID()); err != nil {
		b.close()
		return nil, fmt.Errorf("bind shm: %w", err)
	}
	b.shm = shm

	shm.SetFormatHandler(func(e client.ShmFormatEvent) {
		if b.shmFormat == 0xFFFFFFFF {
			b.shmFormat = e.Format
		}
	})

	// Roundtrip after binding wl_shm to dispatch format events before buffer creation
	syncCb, err := display.Sync()
	if err != nil {
		b.close()
		return nil, fmt.Errorf("shm sync: %w", err)
	}
	shmDone := false
	syncCb.SetDoneHandler(func(client.CallbackDoneEvent) {
		shmDone = true
	})
	for !shmDone {
		if err := b.ctx.Dispatch(); err != nil {
			b.close()
			return nil, b.wrapDispatchErr("shm dispatch", err)
		}
	}
	syncCb.Destroy()

	wmBaseProxy := xdg_shell.NewWmBase(b.ctx)
	if err := registryBind(b.ctx, registry.ID(), wmBaseName, "xdg_wm_base", 1, wmBaseProxy.ID()); err != nil {
		b.close()
		return nil, fmt.Errorf("bind wm_base: %w", err)
	}
	b.wmBase = wmBaseProxy

	wmBaseProxy.SetPingHandler(func(e xdg_shell.WmBasePingEvent) {
		_ = wmBaseProxy.Pong(e.Serial)
	})

	seat := client.NewSeat(b.ctx)
	if err := registryBind(b.ctx, registry.ID(), seatName, "wl_seat", seatVersion, seat.ID()); err != nil {
		b.close()
		return nil, fmt.Errorf("bind seat: %w", err)
	}
	b.seat = seat

	return b, nil
}

func (b *WaylandBackend) CreateSurface(width, height int) (platform.Surface, error) {
	if width <= 0 || height <= 0 {
		width = 800
		height = 600
	}
	return newWaylandSurface(b, b.ctx, b.compositor, b.shm, b.wmBase, width, height)
}

func (b *WaylandBackend) CreateInputSource() (platform.InputSource, error) {
	return newWaylandInput(b.ctx, b.seat)
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

		if err := b.ctx.Dispatch(); err != nil {
			b.notifyClose()
			return
		}
	}
}

func (b *WaylandBackend) Stop() error {
	b.stopOnce.Do(func() {
		if b.ctx != nil {
			b.ctx.Close()
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

func (b *WaylandBackend) wrapDispatchErr(prefix string, err error) error {
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
	var lastErr error
	b.closeOnce.Do(func() {
		if b.seat != nil {
			b.seat.Release()
		}
		if b.wmBase != nil {
			b.wmBase.Destroy()
		}
		if b.shm != nil {
			b.shm.Destroy()
		}
		if b.compositor != nil {
			b.compositor.Destroy()
		}
		if b.registry != nil {
			b.registry.Destroy()
		}
		if b.display != nil {
			b.display.Destroy()
		}
	})
	return lastErr
}

var _ platform.Backend = (*WaylandBackend)(nil)

func Probe() bool {
	display, err := client.Connect("")
	if err != nil {
		return false
	}
	display.Destroy()
	return true
}
