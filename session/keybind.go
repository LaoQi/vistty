package session

import (
	"github.com/LaoQi/vistty/internal/platform"
)

type ResolvedKeybind struct {
	Mod  platform.Modifiers
	Rune rune
	Code uint16
}

type KeybindTable map[string]ResolvedKeybind

func (t KeybindTable) Match(ev platform.KeyEvent) (string, bool) {
	for action, kb := range t {
		if ev.Mods&kb.Mod == 0 {
			continue
		}
		if kb.Code != 0 {
			if ev.Code == kb.Code {
				return action, true
			}
			continue
		}
		if kb.Rune != 0 && ev.Rune == kb.Rune {
			return action, true
		}
	}
	return "", false
}
