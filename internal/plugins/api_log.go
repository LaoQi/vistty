package plugins

import (
	"github.com/LaoQi/vistty/internal/debug"
	lua "github.com/yuin/gopher-lua"
)

func registerLog(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	logT := L.NewTable()
	vt.RawSetString("log", logT)

	logT.RawSetString("debug", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		debug.Debugf("plugin: %s", msg)
		return 0
	}))

	logT.RawSetString("error", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		debug.Errorf("plugin: %s", msg)
		return 0
	}))

	logT.RawSetString("warning", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		debug.Warningf("plugin: %s", msg)
		return 0
	}))
}
