package wayland

import (
	"sync"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/rajveermalviya/go-wayland/wayland/client"
)

type WaylandInput struct {
	ctx      *client.Context
	keyboard *client.Keyboard
	pointer  *client.Pointer

	keyCh   chan platform.KeyEvent
	mouseCh chan platform.MouseEvent

	mu   sync.Mutex
	mods platform.Modifiers
	posX int
	posY int

	keymap keymap

	done chan struct{}
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
		if e.Fd >= 0 {
			km, err := parseKeymap(e.Fd, e.Size)
			if err == nil {
				i.mu.Lock()
				i.keymap = km
				i.mu.Unlock()
			}
		}
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

		if mod, ok := modifierKeys[code]; ok {
			if state {
				mods |= mod
			} else {
				mods &^= mod
			}
			i.mu.Lock()
			i.mods = mods
			i.mu.Unlock()

			i.keyCh <- platform.KeyEvent{
				Rune:  0,
				Code:  code,
				Mods:  mods,
				State: state,
			}
			return
		}

		var r rune
		if km != nil {
			r = km.lookup(e.Key, mods)
		} else {
			r = fallbackKeyRune(e.Key, mods)
		}

		i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: state,
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

		i.mouseCh <- platform.MouseEvent{
			X:      posX,
			Y:      posY,
			Button: btn,
			State:  platform.KeyState(e.State == 1),
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
	close(i.done)
	if i.keyboard != nil {
		i.keyboard.Release()
	}
	if i.pointer != nil {
		i.pointer.Release()
	}
	return nil
}

var modifierKeys = map[uint16]platform.Modifiers{
	42:  platform.ModShift,
	54:  platform.ModShift,
	29:  platform.ModCtrl,
	97:  platform.ModCtrl,
	56:  platform.ModAlt,
	100: platform.ModAlt,
	125: platform.ModSuper,
}

var usKeyMap = map[uint32]rune{
	2: '1', 3: '2', 4: '3', 5: '4', 6: '5', 7: '6', 8: '7', 9: '8', 10: '9', 11: '0',
	12: '-', 13: '=', 14: 0x08,
	15: '\t',
	16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p',
	26: '[', 27: ']', 28: '\r',
	30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l',
	39: ';', 40: '\'', 41: '`',
	42: 0,
	43: '\\', 44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm',
	51: ',', 52: '.', 53: '/',
	54: 0,
	56: 0,
	57: ' ',
	58: 0,
	59: 0, 60: 0, 61: 0, 62: 0, 63: 0, 64: 0, 65: 0, 66: 0, 67: 0, 68: 0,
	97: 0,
	100: 0,
	103: 0,
	105: 0,
	106: 0,
	108: 0,
	125: 0,
	29: 0,
}

func fallbackKeyRune(key uint32, mods platform.Modifiers) rune {
	r, ok := usKeyMap[key]
	if !ok {
		return 0
	}
	if mods&platform.ModShift != 0 && r != 0 {
		r = shiftRune(r)
	}
	if mods&platform.ModCtrl != 0 && r >= 'a' && r <= 'z' {
		r = r - 'a' + 1
	}
	return r
}

func shiftRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	switch r {
	case '1':
		return '!'
	case '2':
		return '@'
	case '3':
		return '#'
	case '4':
		return '$'
	case '5':
		return '%'
	case '6':
		return '^'
	case '7':
		return '&'
	case '8':
		return '*'
	case '9':
		return '('
	case '0':
		return ')'
	case '-':
		return '_'
	case '=':
		return '+'
	case '[':
		return '{'
	case ']':
		return '}'
	case '\\':
		return '|'
	case ';':
		return ':'
	case '\'':
		return '"'
	case ',':
		return '<'
	case '.':
		return '>'
	case '/':
		return '?'
	case '`':
		return '~'
	}
	return r
}

var _ platform.InputSource = (*WaylandInput)(nil)
