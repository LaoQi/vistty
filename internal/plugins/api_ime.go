package plugins

import (
	"github.com/LaoQi/vistty/ime"
	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/debug"
)

func registerIME(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	imeT := L.NewTable()
	vt.RawSetString("ime", imeT)

	imeT.RawSetString("register", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			return 0
		}
		name := L.CheckString(1)
		hooksT := L.CheckTable(2)

		hooks := ime.LuaIMMHooks{}

		if fn, ok := getLuaFn(L, hooksT, "lookup"); ok {
			hooks.Lookup = pm.makeLookupHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "format_preedit"); ok {
			hooks.FormatPreedit = pm.makeFormatPreeditHook(fn)
		}

		m := ime.NewLuaIMM(name, hooks)
		pm.registry.Register(m)
		return 0
	}))

	imeT.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(L.NewTable())
			return 1
		}
		names := pm.registry.List()
		t := L.NewTable()
		for i, n := range names {
			t.RawSetInt(i+1, lua.LString(n))
		}
		L.Push(t)
		return 1
	}))

	imeT.RawSetString("lookup", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(L.NewTable())
			return 1
		}
		name := L.CheckString(1)
		input := L.CheckString(2)
		cands := pm.registry.Lookup(name, input)
		t := L.NewTable()
		for i, c := range cands {
			ct := L.NewTable()
			ct.RawSetString("word", lua.LString(c.Word))
			ct.RawSetString("code", lua.LString(c.Code))
			t.RawSetInt(i+1, ct)
		}
		L.Push(t)
		return 1
	}))

	imeT.RawSetString("format_preedit", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(lua.LString(""))
			return 1
		}
		name := L.CheckString(1)
		input := L.CheckString(2)
		L.Push(lua.LString(pm.registry.FormatPreedit(name, input)))
		return 1
	}))
}

func getLuaFn(L *lua.LState, t *lua.LTable, key string) (*lua.LFunction, bool) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return nil, false
	}
	fn, ok := v.(*lua.LFunction)
	if !ok {
		return nil, false
	}
	return fn, true
}

func (pm *PluginManager) makeLookupHook(fn *lua.LFunction) func(string) []ime.Candidate {
	return func(input string) []ime.Candidate {
		pm.L.Push(fn)
		pm.L.Push(lua.LString(input))
		if err := pm.L.PCall(1, 1, nil); err != nil {
			pm.logHookError(err)
			return nil
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		t, ok := ret.(*lua.LTable)
		if !ok {
			return nil
		}
		n := t.Len()
		cands := make([]ime.Candidate, 0, n)
		for i := 1; i <= n; i++ {
			v := t.RawGetInt(i)
			ct, ok := v.(*lua.LTable)
			if !ok {
				continue
			}
			var c ime.Candidate
			if w := ct.RawGetString("word"); w != lua.LNil {
				if s, ok := w.(lua.LString); ok {
					c.Word = string(s)
				}
			}
			if cd := ct.RawGetString("code"); cd != lua.LNil {
				if s, ok := cd.(lua.LString); ok {
					c.Code = string(s)
				}
			}
			cands = append(cands, c)
		}
		return cands
	}
}

func (pm *PluginManager) makeFormatPreeditHook(fn *lua.LFunction) func(string) string {
	return func(input string) string {
		pm.L.Push(fn)
		pm.L.Push(lua.LString(input))
		if err := pm.L.PCall(1, 1, nil); err != nil {
			pm.logHookError(err)
			return input
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		if s, ok := ret.(lua.LString); ok {
			return string(s)
		}
		return input
	}
}

func (pm *PluginManager) logHookError(err error) {
	debug.Errorf("plugin ime hook error: %v", err)
}
