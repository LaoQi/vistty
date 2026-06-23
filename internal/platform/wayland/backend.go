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

	mu     sync.Mutex
	closed bool
}

func NewWaylandBackend() (*WaylandBackend, error) {
	display, err := client.Connect("")
	if err != nil {
		return nil, fmt.Errorf("connect to wayland: %w", err)
	}

	b := &WaylandBackend{
		ctx:     display.Context(),
		display: display,
	}

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

	registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "wl_compositor":
			compositorName = e.Name
		case "wl_shm":
			shmName = e.Name
		case "xdg_wm_base":
			wmBaseName = e.Name
		case "wl_seat":
			seatName = e.Name
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
			return nil, fmt.Errorf("dispatch: %w", err)
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

	compositor := client.NewCompositor(b.ctx)
	if err := registry.Bind(compositorName, "wl_compositor", 4, compositor); err != nil {
		b.close()
		return nil, fmt.Errorf("bind compositor: %w", err)
	}
	b.compositor = compositor

	shm := client.NewShm(b.ctx)
	if err := registry.Bind(shmName, "wl_shm", 1, shm); err != nil {
		b.close()
		return nil, fmt.Errorf("bind shm: %w", err)
	}
	b.shm = shm

	wmBaseProxy := xdg_shell.NewWmBase(b.ctx)
	if err := registry.Bind(wmBaseName, "xdg_wm_base", 1, wmBaseProxy); err != nil {
		b.close()
		return nil, fmt.Errorf("bind wm_base: %w", err)
	}
	b.wmBase = wmBaseProxy

	wmBaseProxy.SetPingHandler(func(e xdg_shell.WmBasePingEvent) {
		_ = wmBaseProxy.Pong(e.Serial)
	})

	seat := client.NewSeat(b.ctx)
	if err := registry.Bind(seatName, "wl_seat", 5, seat); err != nil {
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
	return newWaylandSurface(b.ctx, b.compositor, b.shm, b.wmBase, width, height)
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
			return
		}
	}
}

func (b *WaylandBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return b.close()
}

func (b *WaylandBackend) close() error {
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
	if b.ctx != nil {
		return b.ctx.Close()
	}
	return nil
}

var _ platform.Backend = (*WaylandBackend)(nil)
