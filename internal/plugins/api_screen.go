package plugins

import lua "github.com/yuin/gopher-lua"

// registerScreen 注册 vistty.screen 命名空间。
// 提供多屏切换能力：上一个/下一个/指定/查询数量/查询焦点。
// 所有方法在 pm.ctx == nil 时安全返回零值/no-op。
// 索引均为 1-based。
func registerScreen(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	screenT := L.NewTable()
	vt.RawSetString("screen", screenT)

	// vistty.screen.next() — 切换到下一个屏幕
	screenT.RawSetString("next", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.NextScreen()
		}
		return 0
	}))

	// vistty.screen.prev() — 切换到上一个屏幕
	screenT.RawSetString("prev", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.PrevScreen()
		}
		return 0
	}))

	// vistty.screen.switch(idx) — 切换到指定屏幕（1-based，越界静默忽略）
	screenT.RawSetString("switch", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			idx := L.CheckInt(1)
			pm.ctx.SwitchScreen(idx)
		}
		return 0
	}))

	// vistty.screen.count() → number
	screenT.RawSetString("count", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(pm.ctx.ScreenCount()))
		return 1
	}))

	// vistty.screen.focused_idx() → number（1-based）
	screenT.RawSetString("focused_idx", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(pm.ctx.FocusScreenIdx()))
		return 1
	}))
}
