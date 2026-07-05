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
		term, osd := parseLuaTheme(L, t)
		pm.currentTheme = &term
		pm.currentOSDTheme = &osd
		if pm.ctx != nil {
			pm.ctx.ApplyTheme(term, osd)
		}
		return 0
	}))

	themeT.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		var term terminal.Theme
		var osd ui.OSDTheme
		if pm.currentTheme != nil {
			term = *pm.currentTheme
		} else {
			term = terminal.DefaultTheme
		}
		if pm.currentOSDTheme != nil {
			osd = *pm.currentOSDTheme
		} else {
			osd = ui.DefaultOSDTheme
		}
		L.Push(themeToLuaTable(L, term, osd))
		return 1
	}))

	themeT.RawSetString("default", L.NewFunction(func(L *lua.LState) int {
		term := terminal.DefaultTheme
		osd := ui.DefaultOSDTheme
		L.Push(themeToLuaTable(L, term, osd))
		return 1
	}))
}

func themeToLuaTable(L *lua.LState, term terminal.Theme, osd ui.OSDTheme) *lua.LTable {
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
	osdT.RawSetString("bar_bg", lua.LString(array3ToHex(osd.BarBg)))
	osdT.RawSetString("active_bg", lua.LString(array3ToHex(osd.ActiveBg)))
	osdT.RawSetString("inactive_bg", lua.LString(array3ToHex(osd.InactiveBg)))
	osdT.RawSetString("active_fg", lua.LString(array3ToHex(osd.ActiveFg)))
	osdT.RawSetString("inactive_fg", lua.LString(array3ToHex(osd.InactiveFg)))
	osdT.RawSetString("csd_btn_bg", lua.LString(array3ToHex(osd.CsdBtnBg)))
	osdT.RawSetString("csd_close_bg", lua.LString(array3ToHex(osd.CsdCloseBg)))
	osdT.RawSetString("csd_btn_fg", lua.LString(array3ToHex(osd.CsdBtnFg)))
	t.RawSetString("osd", osdT)
	return t
}

func colorToHex(c screen.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func array3ToHex(a [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", a[0], a[1], a[2])
}
