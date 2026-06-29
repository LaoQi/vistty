package plugins

import (
	"os"
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
	lua "github.com/yuin/gopher-lua"
)

func TestP1DefaultConfig(t *testing.T) {
	pm := NewPluginManager("/nonexistent/init.lua")
	cfg, err := pm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != "auto" || cfg.Shell != "/bin/bash" || cfg.FontSize != 14 || cfg.ModKey != "super" {
		t.Fatalf("default config mismatch: %+v", cfg)
	}
	if !cfg.OSD.Top.Enabled || cfg.OSD.Bottom.Enabled {
		t.Fatalf("default osd mismatch: %+v", cfg.OSD)
	}
	if len(cfg.Keybindings) < 17 {
		t.Fatalf("default keybindings count %d < 17", len(cfg.Keybindings))
	}
	pm.Close()
}

func TestP1LuaConfig(t *testing.T) {
	src := `
vistty.config = {
	backend = "wayland", shell = "/bin/zsh", fontsize = 18,
	osd = { top = false, bottom = true, left = true, right = false },
	keybindings = { zoom_in = {key="=", mod="ctrl"}, new_tab = {key="t", mod="alt"} },
}
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	cfg, err := pm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != "wayland" || cfg.Shell != "/bin/zsh" || cfg.FontSize != 18 {
		t.Fatalf("lua config mismatch: %+v", cfg)
	}
	if cfg.OSD.Top.Enabled || !cfg.OSD.Bottom.Enabled || !cfg.OSD.Left.Enabled || cfg.OSD.Right.Enabled {
		t.Fatalf("lua osd mismatch: %+v", cfg.OSD)
	}
	if cfg.Keybindings["zoom_in"].Mod != platform.ModCtrl {
		t.Fatalf("zoom_in mod != ctrl: %+v", cfg.Keybindings["zoom_in"])
	}
	if cfg.Keybindings["new_tab"].Mod != platform.ModAlt {
		t.Fatalf("new_tab mod != alt: %+v", cfg.Keybindings["new_tab"])
	}
	if cfg.Keybindings["zoom_out"].Mod != platform.ModSuper {
		t.Fatalf("zoom_out default mod != super: %+v", cfg.Keybindings["zoom_out"])
	}
	if cfg.Keybindings["switch_n1"].Rune != '1' {
		t.Fatalf("switch_n1 rune != 1: %+v", cfg.Keybindings["switch_n1"])
	}
	pm.Close()
}

func TestP1OnKey(t *testing.T) {
	src := `
vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.SPACE and (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL then
		return true
	end
	if ev.code == vistty.keys.F1 then
		return {rune=0, code=vistty.keys.F2, mods=ev.mods}
	end
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(nil)

	consumed, _ := pm.OnKey(platform.KeyEvent{Code: uint16(57), Mods: platform.ModCtrl, State: platform.KeyPress})
	if !consumed {
		t.Fatal("Ctrl+Space should be consumed")
	}
	consumed2, out := pm.OnKey(platform.KeyEvent{Code: uint16(59), State: platform.KeyPress})
	if consumed2 {
		t.Fatal("F1 should not be consumed")
	}
	if out.Code != uint16(60) {
		t.Fatalf("F1 should rewrite to F2 (60), got %d", out.Code)
	}
	consumed3, _ := pm.OnKey(platform.KeyEvent{Rune: 'a', State: platform.KeyPress})
	if consumed3 {
		t.Fatal("letter 'a' should not be consumed")
	}
	pm.Close()
}

func TestP1Panels(t *testing.T) {
	src := `
vistty.ui.enable("bottom", 2)
vistty.ui.enable("left", 1)
vistty.ui.disable("left")
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	panels := pm.EnabledPanels()
	if panels["bottom"] != 2 {
		t.Fatalf("panel bottom != 2: %+v", panels)
	}
	if _, ok := panels["left"]; ok {
		t.Fatal("panel left should be disabled")
	}
	pm.Close()
}

func TestP1Constants(t *testing.T) {
	src := `vistty.input.on_key(function(ev) end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	L := pm.L
	vt := L.GetGlobal("vistty").(*lua.LTable)
	keys := L.GetField(vt, "keys").(*lua.LTable)
	if int(keys.RawGetString("ENTER").(lua.LNumber)) != 28 {
		t.Fatal("keys.ENTER != 28")
	}
	if int(keys.RawGetString("F1").(lua.LNumber)) != 59 {
		t.Fatal("keys.F1 != 59")
	}
	mods := L.GetField(vt, "mods").(*lua.LTable)
	if int(mods.RawGetString("CTRL").(lua.LNumber)) != int(platform.ModCtrl) {
		t.Fatal("mods.CTRL != 1")
	}
	if int(mods.RawGetString("SUPER").(lua.LNumber)) != int(platform.ModSuper) {
		t.Fatal("mods.SUPER != 8")
	}
	pm.Close()
}

func TestP1DefaultInitPath(t *testing.T) {
	p := DefaultInitPath()
	if p == "" {
		t.Fatal("DefaultInitPath empty")
	}
	t.Log("DefaultInitPath:", p)
}

func writeTemp(t *testing.T, content string) string {
	f, err := os.CreateTemp("", "init*.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}
