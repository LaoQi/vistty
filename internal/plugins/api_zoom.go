package plugins

import lua "github.com/yuin/gopher-lua"

// registerZoom 注册 vistty.zoom 命名空间。
// 提供字体缩放能力：放大/缩小/重置。
// 所有方法在 pm.ctx == nil 时安全 no-op。
func registerZoom(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	zoomT := L.NewTable()
	vt.RawSetString("zoom", zoomT)

	zoomT.RawSetString("increase", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.ZoomIn()
		}
		return 0
	}))

	zoomT.RawSetString("decrease", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.ZoomOut()
		}
		return 0
	}))

	// vistty.zoom.reset() — 重置字体到初始大小
	zoomT.RawSetString("reset", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.ZoomReset()
		}
		return 0
	}))
}
