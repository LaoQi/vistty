package drm

import (
	"fmt"
	"sync"

	"github.com/holoplot/go-evdev"
	"github.com/LaoQi/vistty/internal/platform"
)

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
		if ev.Value == 2 {
			continue
		}

		code := uint16(ev.Code)

		i.mu.Lock()
		if mod, ok := platform.LookupModifier(uint32(code)); ok {
			if ev.Value != 0 {
				i.mods |= mod
			} else {
				i.mods &^= mod
			}
			mods := i.mods
			i.mu.Unlock()

			select {
			case i.keyCh <- platform.KeyEvent{
				Rune:  0,
				Code:  code,
				Mods:  mods,
				State: platform.KeyState(ev.Value != 0),
			}:
			case <-i.done:
				return
			}
			continue
		}
		mods := i.mods
		i.mu.Unlock()

		r := platform.FallbackKeyRune(uint32(code), mods)
		if r == 0 {
			continue
		}

		select {
		case i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: platform.KeyState(ev.Value != 0),
		}:
		case <-i.done:
			return
		}
	}
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
