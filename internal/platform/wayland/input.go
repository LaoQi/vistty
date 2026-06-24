package wayland

import (
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"golang.org/x/sys/unix"
)

type WaylandInput struct {
	ctx      *client.Context
	keyboard *client.Keyboard
	pointer  *client.Pointer

	keyCh   chan platform.KeyEvent
	mouseCh chan platform.MouseEvent

	mu       sync.Mutex
	mods     platform.Modifiers
	posX     int
	posY     int

	keymap keymap

	done     chan struct{}
	closeOnce sync.Once
}

func newWaylandInput(ctx *client.Context, seat *client.Seat) (*WaylandInput, error) {
	i := &WaylandInput{
		ctx:     ctx,
		keyCh:   make(chan platform.KeyEvent, 64),
		mouseCh: make(chan platform.MouseEvent, 16),
		done:    make(chan struct{}),
	}

	keyboard, err := seat.GetKeyboard()
	if err != nil {
		return nil, err
	}
	i.keyboard = keyboard

	pointer, err := seat.GetPointer()
	if err != nil {
		keyboard.Release()
		return nil, err
	}
	i.pointer = pointer

	keyboard.SetKeymapHandler(func(e client.KeyboardKeymapEvent) {
		if e.Format == 1 && e.Size > 0 {
			if km, err := parseKeymap(e.Fd, e.Size); err == nil {
				i.mu.Lock()
				i.keymap = km
				i.mu.Unlock()
			}
		}
		unix.Close(e.Fd)
	})

	keyboard.SetModifiersHandler(func(e client.KeyboardModifiersEvent) {
		i.mu.Lock()
		i.mods = 0
		if e.ModsDepressed&0x01 != 0 || e.ModsLatched&0x01 != 0 || e.ModsLocked&0x01 != 0 {
			i.mods |= platform.ModShift
		}
		if e.ModsDepressed&0x04 != 0 || e.ModsLatched&0x04 != 0 || e.ModsLocked&0x04 != 0 {
			i.mods |= platform.ModCtrl
		}
		if e.ModsDepressed&0x08 != 0 || e.ModsLatched&0x08 != 0 || e.ModsLocked&0x08 != 0 {
			i.mods |= platform.ModAlt
		}
		if e.ModsDepressed&0x40 != 0 || e.ModsLatched&0x40 != 0 || e.ModsLocked&0x40 != 0 {
			i.mods |= platform.ModSuper
		}
		i.mu.Unlock()
	})

	keyboard.SetKeyHandler(func(e client.KeyboardKeyEvent) {
		i.mu.Lock()
		mods := i.mods
		km := i.keymap
		i.mu.Unlock()

		code := uint16(e.Key)
		state := platform.KeyState(e.State == 1)

		if mod, ok := platform.LookupModifier(uint32(code)); ok {
			if state {
				mods |= mod
			} else {
				mods &^= mod
			}
			i.mu.Lock()
			i.mods = mods
			i.mu.Unlock()

		select {
		case i.keyCh <- platform.KeyEvent{
			Rune:  0,
			Code:  code,
			Mods:  mods,
			State: state,
		}:
		case <-i.done:
		default:
		}
			return
		}

		var r rune
		if km != nil {
			r = km.lookup(e.Key, mods)
		} else {
			r = platform.FallbackKeyRune(uint32(code), mods)
		}

		select {
		case i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: state,
		}:
		case <-i.done:
		default:
		}
	})

	pointer.SetMotionHandler(func(e client.PointerMotionEvent) {
		i.mu.Lock()
		i.posX = int(e.SurfaceX)
		i.posY = int(e.SurfaceY)
		posX := i.posX
		posY := i.posY
		i.mu.Unlock()

		select {
		case i.mouseCh <- platform.MouseEvent{X: posX, Y: posY}:
		default:
		}
	})

	pointer.SetButtonHandler(func(e client.PointerButtonEvent) {
		i.mu.Lock()
		posX := i.posX
		posY := i.posY
		i.mu.Unlock()

		var btn uint8
		switch e.Button {
		case 0x110:
			btn = 1
		case 0x111:
			btn = 3
		case 0x112:
			btn = 2
		default:
			btn = uint8(e.Button)
		}

		select {
		case i.mouseCh <- platform.MouseEvent{
			X:      posX,
			Y:      posY,
			Button: btn,
			State:  platform.KeyState(e.State == 1),
		}:
		case <-i.done:
		default:
		}
	})

	return i, nil
}

func (i *WaylandInput) KeyEvents() <-chan platform.KeyEvent {
	return i.keyCh
}

func (i *WaylandInput) MouseEvents() <-chan platform.MouseEvent {
	return i.mouseCh
}

func (i *WaylandInput) Close() error {
	i.closeOnce.Do(func() {
		close(i.done)
		if i.keyboard != nil {
			i.keyboard.Release()
		}
		if i.pointer != nil {
			i.pointer.Release()
		}
	})
	return nil
}

var _ platform.InputSource = (*WaylandInput)(nil)
