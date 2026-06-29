package session

import (
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
)

func defaultTestKeybinds(mod platform.Modifiers) KeybindTable {
	table := KeybindTable{
		"zoom_in":     {Mod: mod, Rune: '='},
		"zoom_out":    {Mod: mod, Rune: '-'},
		"zoom_reset":  {Mod: mod, Rune: '0'},
		"new_tab":     {Mod: mod, Rune: 't'},
		"close_tab":   {Mod: mod, Rune: 'w'},
		"next_screen": {Mod: mod, Code: 15},
	}
	for i := 1; i <= 9; i++ {
		digit := rune('0' + i)
		r, code := platform.ParseKey(string(digit))
		table["switch_n"+string(digit)] = ResolvedKeybind{Mod: mod, Rune: r, Code: code}
	}
	return table
}

func TestMatchRune(t *testing.T) {
	table := defaultTestKeybinds(platform.ModSuper)
	ev := platform.KeyEvent{Rune: '=', Mods: platform.ModSuper, State: platform.KeyPress}
	action, ok := table.Match(ev)
	if !ok {
		t.Error("expected match")
	}
	if action != "zoom_in" {
		t.Errorf("expected zoom_in, got %s", action)
	}
}

func TestMatchCode(t *testing.T) {
	table := defaultTestKeybinds(platform.ModSuper)
	ev := platform.KeyEvent{Code: 15, Mods: platform.ModSuper, State: platform.KeyPress}
	action, ok := table.Match(ev)
	if !ok {
		t.Error("expected match")
	}
	if action != "next_screen" {
		t.Errorf("expected next_screen, got %s", action)
	}
}

func TestMatchSwitchN(t *testing.T) {
	table := defaultTestKeybinds(platform.ModSuper)
	ev := platform.KeyEvent{Code: 5, Mods: platform.ModSuper, State: platform.KeyPress}
	action, ok := table.Match(ev)
	if !ok {
		t.Error("expected match for switch_n4")
	}
	if action != "switch_n4" {
		t.Errorf("expected switch_n4, got %s", action)
	}
}

func TestNoMatchNoMod(t *testing.T) {
	table := defaultTestKeybinds(platform.ModSuper)
	ev := platform.KeyEvent{Rune: '=', Mods: 0, State: platform.KeyPress}
	_, ok := table.Match(ev)
	if ok {
		t.Error("should not match without mod")
	}
}

func TestNoMatchWrongKey(t *testing.T) {
	table := defaultTestKeybinds(platform.ModSuper)
	ev := platform.KeyEvent{Rune: 'x', Mods: platform.ModSuper, State: platform.KeyPress}
	_, ok := table.Match(ev)
	if ok {
		t.Error("should not match wrong key")
	}
}
