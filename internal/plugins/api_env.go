package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

// registerEnv 注册运行环境查询接口：
//
//	vistty.backend_name()       → string  ("wayland"/"drm"/"drm-gbm"，Load 阶段未注入时为 "")
//	vistty.backend.is_wayland() → bool
//	vistty.backend.is_drm()     → bool    (drm 或 drm-gbm 均为 true)
//	vistty.on_activate(fn)      → 注册钩子，在 Activate 后以 backend_name 为参数回调
//
// on_activate 用于解决 auto 模式下 init.lua 加载阶段（Load）尚不知实际后端的
// 时序矛盾：backend 在 Load 之后创建，创建完成且 SetBackendName 注入后由
// Activate 触发钩子，钩子内可按后端 bind 专属快捷键。
func registerEnv(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	vt.RawSetString("backend_name", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(pm.backendName))
		return 1
	}))

	backendT := L.NewTable()
	backendT.RawSetString("is_wayland", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(pm.backendName == "wayland"))
		return 1
	}))
	backendT.RawSetString("is_drm", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(pm.backendName == "drm" || pm.backendName == "drm-gbm"))
		return 1
	}))
	vt.RawSetString("backend", backendT)

	vt.RawSetString("on_activate", L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		if pm.activateHooks == nil {
			pm.activateHooks = L.NewTable()
		}
		pm.activateHooks.Append(fn)
		return 0
	}))
}
