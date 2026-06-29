package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

func registerKeybind(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	inputVal := vt.RawGetString("input")
	inputT, ok := inputVal.(*lua.LTable)
	if !ok {
		inputT = L.NewTable()
		vt.RawSetString("input", inputT)
	}

	inputT.RawSetString("bind", L.NewFunction(func(L *lua.LState) int {
		code := uint16(L.CheckInt(1))
		fn := L.CheckFunction(2)
		pm.bindings = append(pm.bindings, keyBinding{code: code, fn: fn})
		return 0
	}))

	inputT.RawSetString("bind_range", L.NewFunction(func(L *lua.LState) int {
		start := uint16(L.CheckInt(1))
		end := uint16(L.CheckInt(2))
		fn := L.CheckFunction(3)
		pm.bindings = append(pm.bindings, keyBinding{
			isRange:    true,
			rangeStart: start,
			rangeEnd:   end,
			fn:         fn,
		})
		return 0
	}))

	inputT.RawSetString("pressed", L.NewFunction(func(L *lua.LState) int {
		code := uint16(L.CheckInt(1))
		if pm.pressedKeys == nil {
			pm.pressedKeys = make(map[uint16]bool)
		}
		L.Push(lua.LBool(pm.pressedKeys[code]))
		return 1
	}))
}
