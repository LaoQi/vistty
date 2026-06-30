package wayland

import (
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
	"golang.org/x/sys/unix"
)

type WaylandInput struct {
	c        *conn
	seat     *wlSeat
	keyboard *wlKeyboard
	pointer  *wlPointer

	keyCh   chan platform.KeyEvent
	mouseCh chan platform.MouseEvent

	mu   sync.Mutex
	mods platform.Modifiers
	posX int
	posY int

	keymap keymap

	done      chan struct{}
	closeOnce sync.Once
}

func newWaylandInput(c *conn, seat *wlSeat) (*WaylandInput, error) {
	return &WaylandInput{
		c:       c,
		seat:    seat,
		keyCh:   make(chan platform.KeyEvent, 64),
		mouseCh: make(chan platform.MouseEvent, 16),
		done:    make(chan struct{}),
	}, nil
}

func (i *WaylandInput) registerKeyboardCallbacks() {
	i.keyboard.onKeymap = func(format uint32, fd int, size uint32) {
		if format == 1 && size > 0 {
			if km, err := parseKeymap(fd, size); err == nil {
				i.mu.Lock()
				i.keymap = km
				i.mu.Unlock()
			}
		}
		unix.Close(fd)
	}

	i.keyboard.onModifiers = func(serial, depressed, latched, locked, group uint32) {
		i.mu.Lock()
		i.mods = 0
		if depressed&0x01 != 0 || latched&0x01 != 0 || locked&0x01 != 0 {
			i.mods |= platform.ModShift
		}
		if depressed&0x04 != 0 || latched&0x04 != 0 || locked&0x04 != 0 {
			i.mods |= platform.ModCtrl
		}
		if depressed&0x08 != 0 || latched&0x08 != 0 || locked&0x08 != 0 {
			i.mods |= platform.ModAlt
		}
		if depressed&0x40 != 0 || latched&0x40 != 0 || locked&0x40 != 0 {
			i.mods |= platform.ModSuper
		}
		i.mu.Unlock()
	}

	i.keyboard.onKey = func(serial, time, key, state uint32) {
		i.mu.Lock()
		mods := i.mods
		km := i.keymap
		i.mu.Unlock()

		code := uint16(key)
		st := platform.KeyState(state == 1)

		if mod, ok := platform.LookupModifier(key); ok {
			if st {
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
				State: st,
			}:
			case <-i.done:
			default:
			}
			return
		}

		var r rune
		if km != nil {
			r = km.lookup(key, mods)
		} else {
			r = platform.FallbackKeyRune(key, mods)
		}

		select {
		case i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: st,
		}:
		case <-i.done:
		default:
		}
	}
}

func (i *WaylandInput) registerPointerCallbacks() {
	i.pointer.onMotion = func(time uint32, x, y float64) {
		i.mu.Lock()
		i.posX = int(x)
		i.posY = int(y)
		px := i.posX
		py := i.posY
		i.mu.Unlock()

		select {
		case i.mouseCh <- platform.MouseEvent{X: px, Y: py}:
		default:
		}
	}

	i.pointer.onButton = func(serial, time, button, state uint32) {
		i.mu.Lock()
		px := i.posX
		py := i.posY
		i.mu.Unlock()

		var btn uint8
		switch button {
		case 0x110:
			btn = 1
		case 0x111:
			btn = 3
		case 0x112:
			btn = 2
		default:
			btn = uint8(button)
		}

		select {
		case i.mouseCh <- platform.MouseEvent{
			X:      px,
			Y:      py,
			Button: btn,
			State:  platform.KeyState(state == 1),
		}:
		case <-i.done:
		default:
		}
	}
}

func (i *WaylandInput) HandleCapabilities(cap uint32) {
	const (
		WL_SEAT_CAPABILITY_POINTER  = 1
		WL_SEAT_CAPABILITY_KEYBOARD = 2
	)

	if cap&WL_SEAT_CAPABILITY_KEYBOARD != 0 && i.keyboard == nil {
		i.keyboard = i.seat.getKeyboard()
		i.registerKeyboardCallbacks()
	} else if cap&WL_SEAT_CAPABILITY_KEYBOARD == 0 && i.keyboard != nil {
		i.keyboard.release()
		i.keyboard = nil
		i.mu.Lock()
		i.mods = 0
		i.mu.Unlock()
	}

	if cap&WL_SEAT_CAPABILITY_POINTER != 0 && i.pointer == nil {
		i.pointer = i.seat.getPointer()
		i.registerPointerCallbacks()
	} else if cap&WL_SEAT_CAPABILITY_POINTER == 0 && i.pointer != nil {
		i.pointer.release()
		i.pointer = nil
	}
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
			i.keyboard.release()
		}
		if i.pointer != nil {
			i.pointer.release()
		}
	})
	return nil
}

var _ platform.InputSource = (*WaylandInput)(nil)
