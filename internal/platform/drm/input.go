package drm

import (
	"fmt"
	"sync"

	"github.com/holoplot/go-evdev"
	"github.com/LaoQi/vistty/internal/platform"
)

var usKeyMap = map[uint16]rune{
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

var modifierKeys = map[uint16]platform.Modifiers{
	42:  platform.ModShift,
	54:  platform.ModShift,
	29:  platform.ModCtrl,
	97:  platform.ModCtrl,
	56:  platform.ModAlt,
	100: platform.ModAlt,
	125: platform.ModSuper,
}

type DRMInput struct {
	keyCh   chan platform.KeyEvent
	mouseCh chan platform.MouseEvent
	devices []*evdev.InputDevice
	done    chan struct{}
	mods    platform.Modifiers
	mu      sync.Mutex
}

func newDRMInput() (*DRMInput, error) {
	i := &DRMInput{
		keyCh:   make(chan platform.KeyEvent, 64),
		mouseCh: make(chan platform.MouseEvent, 16),
		done:    make(chan struct{}),
	}

	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, fmt.Errorf("list evdev devices: %w", err)
	}

	for _, p := range paths {
		dev, err := evdev.Open(p.Path)
		if err != nil {
			continue
		}

		hasKey := false
		for _, t := range dev.CapableTypes() {
			if t == evdev.EV_KEY {
				hasKey = true
				break
			}
		}
		if !hasKey {
			dev.Close()
			continue
		}

		if err := dev.Grab(); err != nil {
			dev.Close()
			continue
		}

		i.devices = append(i.devices, dev)
		go i.readLoop(dev)
	}

	return i, nil
}

func (i *DRMInput) readLoop(dev *evdev.InputDevice) {
	for {
		ev, err := dev.ReadOne()
		if err != nil {
			select {
			case <-i.done:
				return
			default:
				return
			}
		}

		if ev.Type != evdev.EV_KEY {
			continue
		}

		code := uint16(ev.Code)

		i.mu.Lock()
		if mod, ok := modifierKeys[code]; ok {
			if ev.Value != 0 {
				i.mods |= mod
			} else {
				i.mods &^= mod
			}
			i.mu.Unlock()

			i.keyCh <- platform.KeyEvent{
				Rune:  0,
				Code:  code,
				Mods:  i.mods,
				State: platform.KeyState(ev.Value != 0),
			}
			continue
		}
		mods := i.mods
		i.mu.Unlock()

		r, ok := usKeyMap[code]
		if !ok {
			continue
		}

		if mods&platform.ModShift != 0 && r != 0 {
			r = shiftRune(r)
		}
		if mods&platform.ModCtrl != 0 && r >= 'a' && r <= 'z' {
			r = r - 'a' + 1
		}

		i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: platform.KeyState(ev.Value != 0),
		}
	}
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

func (i *DRMInput) KeyEvents() <-chan platform.KeyEvent {
	return i.keyCh
}

func (i *DRMInput) MouseEvents() <-chan platform.MouseEvent {
	return i.mouseCh
}

func (i *DRMInput) Close() error {
	close(i.done)
	for _, dev := range i.devices {
		dev.Ungrab()
		dev.Close()
	}
	return nil
}

var _ platform.InputSource = (*DRMInput)(nil)
