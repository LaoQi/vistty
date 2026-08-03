package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

// registerScreenshot 注册 vistty.screenshot(path?)。
// path 为空时使用默认路径（$XDG_PICTURES_DIR → ~/Pictures → $HOME → /tmp）。
// 仅截取焦点屏幕。返回 (true, path)；失败返回 (false, err)。
// 实际抓取与编码在下一帧渲染完成后执行，成功/失败通过 toast 提示。
func registerScreenshot(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	vt.RawSetString("screenshot", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		path := L.OptString(1, "")
		resolved, err := pm.ctx.Screenshot(path)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		L.Push(lua.LString(resolved))
		return 2
	}))
}
