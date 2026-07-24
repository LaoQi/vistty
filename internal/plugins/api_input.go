package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

func registerInputAPI(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	if inputT, ok := vt.RawGetString("input").(*lua.LTable); ok {
		inputT.RawSetString("commit", L.NewFunction(func(L *lua.LState) int {
			if pm.ctx == nil {
				return 0
			}
			text := L.CheckString(1)
			pm.ctx.CommitText(text)
			return 0
		}))
	}
}
