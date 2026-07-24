package plugins

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

func registerTheme(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	themeT := L.NewTable()
	vt.RawSetString("theme", themeT)

	themeT.RawSetString("apply", L.NewFunction(func(L *lua.LState) int {
		t := L.CheckTable(1)
		term, tb, sb := parseLuaTheme(L, t)
		pm.currentTheme = &term
		pm.currentTabBarTheme = &tb
		pm.currentStatusBarTheme = &sb
		if pm.ctx != nil {
			pm.ctx.ApplyTheme(term, tb, sb)
		}
		return 0
	}))

	themeT.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		var term terminal.Theme
		var tb ui.TabBarTheme
		var sb ui.StatusBarTheme
		if pm.currentTheme != nil {
			term = *pm.currentTheme
		} else {
			term = terminal.DefaultTheme
		}
		if pm.currentTabBarTheme != nil {
			tb = *pm.currentTabBarTheme
		} else {
			tb = ui.DefaultTabBarTheme
		}
		if pm.currentStatusBarTheme != nil {
			sb = *pm.currentStatusBarTheme
		} else {
			sb = ui.DefaultStatusBarTheme
		}
		L.Push(themeToLuaTable(L, term, tb, sb))
		return 1
	}))

	themeT.RawSetString("default", L.NewFunction(func(L *lua.LState) int {
		term := terminal.DefaultTheme
		tb := ui.DefaultTabBarTheme
		sb := ui.DefaultStatusBarTheme
		L.Push(themeToLuaTable(L, term, tb, sb))
		return 1
	}))
}

func themeToLuaTable(L *lua.LState, term terminal.Theme, tb ui.TabBarTheme, sb ui.StatusBarTheme) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("fg", lua.LString(colorToHex(term.DefFg)))
	t.RawSetString("bg", lua.LString(colorToHex(term.DefBg)))
	t.RawSetString("cursor", lua.LString(colorToHex(term.CursorColor)))
	palette := L.NewTable()
	for i, c := range term.Palette {
		palette.RawSetInt(i+1, lua.LString(colorToHex(c)))
	}
	t.RawSetString("palette", palette)
	osdT := L.NewTable()
	osdT.RawSetString("bar_bg", lua.LString(array3ToHex(tb.BarBg)))
	osdT.RawSetString("active_bg", lua.LString(array3ToHex(tb.ActiveBg)))
	osdT.RawSetString("inactive_bg", lua.LString(array3ToHex(tb.InactiveBg)))
	osdT.RawSetString("active_fg", lua.LString(array3ToHex(tb.ActiveFg)))
	osdT.RawSetString("inactive_fg", lua.LString(array3ToHex(tb.InactiveFg)))
	osdT.RawSetString("csd_btn_bg", lua.LString(array3ToHex(tb.CsdBtnBg)))
	osdT.RawSetString("csd_close_bg", lua.LString(array3ToHex(tb.CsdCloseBg)))
	osdT.RawSetString("csd_btn_fg", lua.LString(array3ToHex(tb.CsdBtnFg)))
	t.RawSetString("osd", osdT)
	sbT := L.NewTable()
	sbT.RawSetString("bg", lua.LString(array3ToHex(sb.Bg)))
	t.RawSetString("statusbar", sbT)
	return t
}

func colorToHex(c screen.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func array3ToHex(a [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", a[0], a[1], a[2])
}
