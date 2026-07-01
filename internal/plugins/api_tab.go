package plugins

import lua "github.com/yuin/gopher-lua"

// registerTab 注册 vistty.tab 命名空间。
// 提供标签管理能力：新建/关闭/切换/查询。
// 所有方法在 pm.ctx == nil 时安全返回零值/no-op。
func registerTab(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	tabT := L.NewTable()
	vt.RawSetString("tab", tabT)

	// vistty.tab.new() — 新建标签
	tabT.RawSetString("new", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			_ = pm.ctx.NewTab()
		}
		return 0
	}))

	// vistty.tab.close() — 关闭当前标签
	tabT.RawSetString("close", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.CloseCurrentTab()
		}
		return 0
	}))

	// vistty.tab.next() — 切换到下一个标签
	tabT.RawSetString("next", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.NextTab()
		}
		return 0
	}))

	// vistty.tab.prev() — 切换到上一个标签
	tabT.RawSetString("prev", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.PrevTab()
		}
		return 0
	}))

	// vistty.tab.switch(n) — 切换到指定索引的标签（1-based，越界静默忽略）
	tabT.RawSetString("switch", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			idx := L.CheckInt(1)
			pm.ctx.SwitchTab(idx)
		}
		return 0
	}))

	// vistty.tab.count() → number
	tabT.RawSetString("count", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		list := pm.ctx.TabList()
		L.Push(lua.LNumber(len(list)))
		return 1
	}))

	// vistty.tab.list() → array of {title=, active=}
	tabT.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(L.NewTable())
			return 1
		}
		list := pm.ctx.TabList()
		result := L.NewTable()
		for i, info := range list {
			t := L.NewTable()
			t.RawSetString("title", lua.LString(info.Title))
			t.RawSetString("active", lua.LBool(info.Active))
			result.RawSetInt(i+1, t)
		}
		L.Push(result)
		return 1
	}))
}
