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
	if cfg.Backend != "auto" || cfg.Shell != "/bin/bash" || cfg.FontSize != 14 {
		t.Fatalf("default config mismatch: %+v", cfg)
	}
	pm.Close()
}

func TestP1LuaConfig(t *testing.T) {
	src := `
vistty.config = {
	backend = "wayland", shell = "/bin/zsh", fontsize = 18,
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
	pm.Close()
}

func TestP1Bind(t *testing.T) {
	src := `
vistty.input.bind(vistty.keys.EQUAL, function()
	if vistty.input.pressed(vistty.keys.LEFT_SUPER) then
		return true
	end
end)
vistty.input.bind_range(vistty.keys.NUM1, vistty.keys.NUM9, function(n)
	if vistty.input.pressed(vistty.keys.LEFT_SUPER) then
		return true
	end
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(nil)

	pm.OnKey(platform.KeyEvent{Code: 125, State: platform.KeyPress})
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 13, Mods: platform.ModSuper, State: platform.KeyPress})
	if !consumed {
		t.Fatal("Super+Equal bind should be consumed")
	}

	pm.OnKey(platform.KeyEvent{Code: 125, State: platform.KeyRelease})
	pm.OnKey(platform.KeyEvent{Code: 13, State: platform.KeyRelease})
	consumed2, _ := pm.OnKey(platform.KeyEvent{Code: 13, State: platform.KeyPress})
	if consumed2 {
		t.Fatal("Equal without Super should not be consumed by bind")
	}

	pm.OnKey(platform.KeyEvent{Code: 125, State: platform.KeyPress})
	consumed3, _ := pm.OnKey(platform.KeyEvent{Code: 6, Mods: platform.ModSuper, State: platform.KeyPress})
	if !consumed3 {
		t.Fatal("Super+NUM5 bind_range should be consumed")
	}

	pm.OnKey(platform.KeyEvent{Code: 125, State: platform.KeyPress})
	if !pm.pressedKeys[125] {
		t.Fatal("LEFT_SUPER should be pressed")
	}
	pm.OnKey(platform.KeyEvent{Code: 125, State: platform.KeyRelease})
	if pm.pressedKeys[125] {
		t.Fatal("LEFT_SUPER should be released after KeyRelease")
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
	// 原有断言
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
	// 扩展：字母键
	if int(keys.RawGetString("A").(lua.LNumber)) != 30 {
		t.Fatal("keys.A != 30")
	}
	if int(keys.RawGetString("Q").(lua.LNumber)) != 16 {
		t.Fatal("keys.Q != 16")
	}
	if int(keys.RawGetString("M").(lua.LNumber)) != 50 {
		t.Fatal("keys.M != 50")
	}
	// 数字键（NUM1..NUM9, NUM0）
	if int(keys.RawGetString("NUM1").(lua.LNumber)) != 2 {
		t.Fatal("keys.NUM1 != 2")
	}
	if int(keys.RawGetString("NUM0").(lua.LNumber)) != 11 {
		t.Fatal("keys.NUM0 != 11")
	}
	// 符号键（Linux input-event-codes.h 标准名）
	if int(keys.RawGetString("LEFTBRACE").(lua.LNumber)) != 26 {
		t.Fatal("keys.LEFTBRACE != 26")
	}
	if int(keys.RawGetString("SEMICOLON").(lua.LNumber)) != 39 {
		t.Fatal("keys.SEMICOLON != 39")
	}
	if int(keys.RawGetString("APOSTROPHE").(lua.LNumber)) != 40 {
		t.Fatal("keys.APOSTROPHE != 40")
	}
	if int(keys.RawGetString("GRAVE").(lua.LNumber)) != 41 {
		t.Fatal("keys.GRAVE != 41")
	}
	if int(keys.RawGetString("BACKSLASH").(lua.LNumber)) != 43 {
		t.Fatal("keys.BACKSLASH != 43")
	}
	if int(keys.RawGetString("COMMA").(lua.LNumber)) != 51 {
		t.Fatal("keys.COMMA != 51")
	}
	if int(keys.RawGetString("DOT").(lua.LNumber)) != 52 {
		t.Fatal("keys.DOT != 52")
	}
	if int(keys.RawGetString("SLASH").(lua.LNumber)) != 53 {
		t.Fatal("keys.SLASH != 53")
	}
	if int(keys.RawGetString("MINUS").(lua.LNumber)) != 12 {
		t.Fatal("keys.MINUS != 12")
	}
	if int(keys.RawGetString("EQUAL").(lua.LNumber)) != 13 {
		t.Fatal("keys.EQUAL != 13")
	}
	// 系统键
	if int(keys.RawGetString("CAPSLOCK").(lua.LNumber)) != 58 {
		t.Fatal("keys.CAPSLOCK != 58")
	}
	if int(keys.RawGetString("NUMLOCK").(lua.LNumber)) != 69 {
		t.Fatal("keys.NUMLOCK != 69")
	}
	if int(keys.RawGetString("SCROLLLOCK").(lua.LNumber)) != 70 {
		t.Fatal("keys.SCROLLLOCK != 70")
	}
	if int(keys.RawGetString("PAUSE").(lua.LNumber)) != 119 {
		t.Fatal("keys.PAUSE != 119")
	}
	if int(keys.RawGetString("PRINT").(lua.LNumber)) != 210 {
		t.Fatal("keys.PRINT != 210")
	}
	if int(keys.RawGetString("MENU").(lua.LNumber)) != 139 {
		t.Fatal("keys.MENU != 139")
	}
	// ESCAPE 别名 ESC
	if int(keys.RawGetString("ESC").(lua.LNumber)) != 1 {
		t.Fatal("keys.ESC != 1")
	}
	// 小键盘
	if int(keys.RawGetString("KP1").(lua.LNumber)) != 79 {
		t.Fatal("keys.KP1 != 79")
	}
	if int(keys.RawGetString("KP0").(lua.LNumber)) != 82 {
		t.Fatal("keys.KP0 != 82")
	}
	if int(keys.RawGetString("KPDOT").(lua.LNumber)) != 83 {
		t.Fatal("keys.KPDOT != 83")
	}
	if int(keys.RawGetString("KPPLUS").(lua.LNumber)) != 78 {
		t.Fatal("keys.KPPLUS != 78")
	}
	if int(keys.RawGetString("KPENTER").(lua.LNumber)) != 96 {
		t.Fatal("keys.KPENTER != 96")
	}
	if int(keys.RawGetString("KPSLASH").(lua.LNumber)) != 98 {
		t.Fatal("keys.KPSLASH != 98")
	}
	if int(keys.RawGetString("KPASTERISK").(lua.LNumber)) != 55 {
		t.Fatal("keys.KPASTERISK != 55")
	}
	if int(keys.RawGetString("KPMINUS").(lua.LNumber)) != 74 {
		t.Fatal("keys.KPMINUS != 74")
	}
	if int(keys.RawGetString("KPEQUAL").(lua.LNumber)) != 117 {
		t.Fatal("keys.KPEQUAL != 117")
	}
	// vistty.state 常量表
	state := L.GetField(vt, "state").(*lua.LTable)
	if string(state.RawGetString("PRESS").(lua.LString)) != "press" {
		t.Fatal("state.PRESS != \"press\"")
	}
	if string(state.RawGetString("RELEASE").(lua.LString)) != "release" {
		t.Fatal("state.RELEASE != \"release\"")
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

func TestP1Colors(t *testing.T) {
	src := `vistty.input.on_key(function(ev) end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	L := pm.L
	vt := L.GetGlobal("vistty").(*lua.LTable)
	colors := L.GetField(vt, "colors").(*lua.LTable)
	if string(colors.RawGetString("RED").(lua.LString)) != "#FF0000" {
		t.Fatal("colors.RED != #FF0000")
	}
	if string(colors.RawGetString("WHITE").(lua.LString)) != "#FFFFFF" {
		t.Fatal("colors.WHITE != #FFFFFF")
	}
	if string(colors.RawGetString("BLACK").(lua.LString)) != "#000000" {
		t.Fatal("colors.BLACK != #000000")
	}
	if string(colors.RawGetString("BLUE").(lua.LString)) != "#0000FF" {
		t.Fatal("colors.BLUE != #0000FF")
	}
	if string(colors.RawGetString("GREEN").(lua.LString)) != "#00FF00" {
		t.Fatal("colors.GREEN != #00FF00")
	}
	pm.Close()
}

func TestP1ColorParsing(t *testing.T) {
	cases := []struct {
		name string
		opts string
		fg   [4]uint8
		bg   [4]uint8
	}{
		{"hex6", `{fg="#FF8800", bg="#00FF44"}`, [4]uint8{255, 136, 0, 255}, [4]uint8{0, 255, 68, 255}},
		{"hex8_alpha", `{fg="#FF880080", bg="#000000FF"}`, [4]uint8{255, 136, 0, 128}, [4]uint8{0, 0, 0, 255}},
		{"hex3", `{fg="#F80", bg="#0F0"}`, [4]uint8{255, 136, 0, 255}, [4]uint8{0, 255, 0, 255}},
		{"hex4_alpha", `{fg="#F808", bg="#000F"}`, [4]uint8{255, 136, 0, 136}, [4]uint8{0, 0, 0, 255}},
		{"table_rgb", `{fg={1,2,3}, bg={4,5,6}}`, [4]uint8{1, 2, 3, 255}, [4]uint8{4, 5, 6, 255}},
		{"table_rgba", `{fg={1,2,3,64}, bg={4,5,6,128}}`, [4]uint8{1, 2, 3, 64}, [4]uint8{4, 5, 6, 128}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `vistty.ui.on_render(function(ctx) ctx:text(0,0,"x", ` + c.opts + `) end)`
			f := writeTemp(t, src)
			pm := NewPluginManager(f)
			if _, err := pm.Load(); err != nil {
				t.Fatal(err)
			}
			pm.Activate(&p4FakeCtx{})
			_, prims := pm.OnRender("bottom", 80, 1)
			if len(prims) != 1 {
				t.Fatalf("expected 1 prim, got %d", len(prims))
			}
			if prims[0].Fg != c.fg {
				t.Fatalf("fg mismatch: got %v want %v", prims[0].Fg, c.fg)
			}
			if prims[0].Bg != c.bg {
				t.Fatalf("bg mismatch: got %v want %v", prims[0].Bg, c.bg)
			}
			pm.Close()
		})
	}
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
