package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

// registerLifecycle 注册生命周期钩子：
//
//	vistty.on_exit(fn)              — 主循环退出后触发（无参数）
//	vistty.on_tab_new(fn)           — 新标签创建后触发，fn(idx, title)
//	vistty.on_tab_close(fn)         — 标签关闭后触发，fn(idx, title)
//	vistty.on_tab_switch(fn)        — 标签切换后触发，fn(new_idx, old_idx)
//	vistty.on_screen_switch(fn)     — 屏幕焦点切换后触发，fn(idx)
//	vistty.on_title_change(fn)      — 终端标题变化后触发（经主线程缓冲），fn(title)
//	vistty.on_resize(fn)            — 窗口/尺寸变化后触发，fn(output_id, width, height, cols, rows)
//	vistty.on_zoom(fn)              — 字体缩放后触发，fn(size)
//
// 所有 idx 均为 1-based。钩子在主渲染线程触发（与 OnKey/OnRender 同线程），
// 可安全调用 vistty.* API。单个钩子出错不影响后续。
func registerLifecycle(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	vt.RawSetString("on_exit", makeHookReg(L, pm, func() *lua.LTable { return pm.exitHooks }))
	vt.RawSetString("on_tab_new", makeHookReg(L, pm, func() *lua.LTable { return pm.tabNewHooks }))
	vt.RawSetString("on_tab_close", makeHookReg(L, pm, func() *lua.LTable { return pm.tabCloseHooks }))
	vt.RawSetString("on_tab_switch", makeHookReg(L, pm, func() *lua.LTable { return pm.tabSwitchHooks }))
	vt.RawSetString("on_screen_switch", makeHookReg(L, pm, func() *lua.LTable { return pm.screenSwitchHooks }))
	vt.RawSetString("on_title_change", makeHookReg(L, pm, func() *lua.LTable { return pm.titleChangeHooks }))
	vt.RawSetString("on_resize", makeHookReg(L, pm, func() *lua.LTable { return pm.resizeHooks }))
	vt.RawSetString("on_zoom", makeHookReg(L, pm, func() *lua.LTable { return pm.zoomHooks }))
}

// makeHookReg 创建一个 vistty.on_xxx(fn) 注册函数，fn 追加到 hooksGetter 返回的表。
// 使用 getter 而非直接捕获字段，以兼容 Reload 重建表后引用新表。
func makeHookReg(L *lua.LState, pm *PluginManager, hooksGetter func() *lua.LTable) *lua.LFunction {
	return L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		hooks := hooksGetter()
		if hooks == nil {
			hooks = L.NewTable()
		}
		hooks.Append(fn)
		return 0
	})
}
