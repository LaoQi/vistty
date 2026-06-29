package plugins

import (
	"github.com/LaoQi/vistty/ime"
	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/debug"
)

// registerIME 注册 vistty.ime 命名空间。
// 提供 Lua 注册输入法、激活/去激活、按键路由、preedit/candidates 查询等能力。
// 所有方法在 pm.registry == nil 时安全返回零值。
func registerIME(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	imeT := L.NewTable()
	vt.RawSetString("ime", imeT)

	// vistty.ime.register(name, hooks_table)
	// hooks_table 含 on_activate/on_deactivate/process_key/preedit/candidates/reset 回调函数。
	imeT.RawSetString("register", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			return 0
		}
		name := L.CheckString(1)
		hooksT := L.CheckTable(2)

		hooks := ime.LuaIMMHooks{}

		if fn, ok := getLuaFn(L, hooksT, "on_activate"); ok {
			hooks.OnActivate = pm.makeVoidHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "on_deactivate"); ok {
			hooks.OnDeactivate = pm.makeVoidHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "process_key"); ok {
			hooks.ProcessKey = pm.makeProcessKeyHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "preedit"); ok {
			hooks.Preedit = pm.makeStringHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "candidates"); ok {
			hooks.Candidates = pm.makeCandidatesHook(fn)
		}
		if fn, ok := getLuaFn(L, hooksT, "reset"); ok {
			hooks.OnReset = pm.makeVoidHook(fn)
		}

		m := ime.NewLuaIMM(name, hooks)
		pm.registry.Register(m)
		return 0
	}))

	// vistty.ime.list() → {"pinyin", ...}
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

	// vistty.ime.activate(name) → bool
	imeT.RawSetString("activate", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(lua.LFalse)
			return 1
		}
		name := L.CheckString(1)
		L.Push(lua.LBool(pm.registry.Activate(name)))
		return 1
	}))

	// vistty.ime.deactivate()
	imeT.RawSetString("deactivate", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			return 0
		}
		pm.registry.Deactivate()
		return 0
	}))

	// vistty.ime.active() → string / nil
	imeT.RawSetString("active", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(lua.LNil)
			return 1
		}
		if m := pm.registry.Active(); m != nil {
			L.Push(lua.LString(m.Name()))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}))

	// vistty.ime.process_key(ev) → Response table
	// ev = {rune=,code=,mods=,state=}
	// Response table = {consumed=,commit=,preedit=,candidates={{word=,code=},...}}
	imeT.RawSetString("process_key", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(emptyResponseTable(L))
			return 1
		}
		evT := L.CheckTable(1)
		ev := keyEventFromTable(L, evT)
		resp := pm.registry.ProcessKey(ev)
		L.Push(responseToTable(L, resp))
		return 1
	}))

	// vistty.ime.preedit() → string
	imeT.RawSetString("preedit", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(lua.LString(""))
			return 1
		}
		if m := pm.registry.Active(); m != nil {
			L.Push(lua.LString(m.Preedit()))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	}))

	// vistty.ime.candidates() → {{word=,code=},...}
	imeT.RawSetString("candidates", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			L.Push(L.NewTable())
			return 1
		}
		var cands []ime.Candidate
		if m := pm.registry.Active(); m != nil {
			cands = m.Candidates()
		}
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

	// vistty.ime.reset()
	imeT.RawSetString("reset", L.NewFunction(func(L *lua.LState) int {
		if pm.registry == nil {
			return 0
		}
		if m := pm.registry.Active(); m != nil {
			m.Reset()
		}
		return 0
	}))
}

// getLuaFn 从 table 中读取名为 key 的函数字段。
// 若字段不是函数或不存在，返回 ok=false。
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

// makeVoidHook 构造无参无返回值的 Lua 回调闭包。
func (pm *PluginManager) makeVoidHook(fn *lua.LFunction) func() {
	return func() {
		pm.L.Push(fn)
		if err := pm.L.PCall(0, 0, nil); err != nil {
			pm.logHookError(err)
		}
	}
}

// makeStringHook 构造 () → string 的 Lua 回调闭包。
func (pm *PluginManager) makeStringHook(fn *lua.LFunction) func() string {
	return func() string {
		pm.L.Push(fn)
		if err := pm.L.PCall(0, 1, nil); err != nil {
			pm.logHookError(err)
			return ""
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		if s, ok := ret.(lua.LString); ok {
			return string(s)
		}
		return ""
	}
}

// makeCandidatesHook 构造 () → []ime.Candidate 的 Lua 回调闭包。
// Lua 端返回 {{word=,code=},...} 数组。
func (pm *PluginManager) makeCandidatesHook(fn *lua.LFunction) func() []ime.Candidate {
	return func() []ime.Candidate {
		pm.L.Push(fn)
		if err := pm.L.PCall(0, 1, nil); err != nil {
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

// makeProcessKeyHook 构造 (KeyEvent) → Response 的 Lua 回调闭包。
// Lua 端接收 ev table，返回 {consumed=,commit=,preedit=,candidates=...}。
func (pm *PluginManager) makeProcessKeyHook(fn *lua.LFunction) func(ime.KeyEvent) ime.Response {
	return func(ev ime.KeyEvent) ime.Response {
		pm.L.Push(fn)
		pm.L.Push(keyEventToTable(pm.L, ev))
		if err := pm.L.PCall(1, 1, nil); err != nil {
			pm.logHookError(err)
			return ime.Response{}
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		t, ok := ret.(*lua.LTable)
		if !ok {
			return ime.Response{}
		}
		return responseFromTable(pm.L, t)
	}
}

// keyEventToTable 把 ime.KeyEvent 转为 Lua table {rune=,code=,mods=,state=}。
func keyEventToTable(L *lua.LState, ev ime.KeyEvent) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("rune", lua.LNumber(int(ev.Rune)))
	t.RawSetString("code", lua.LNumber(ev.Code))
	t.RawSetString("mods", lua.LNumber(int(ev.Mods)))
	if ev.State {
		t.RawSetString("state", lua.LString("press"))
	} else {
		t.RawSetString("state", lua.LString("release"))
	}
	return t
}

// keyEventFromTable 从 Lua table 读取 ime.KeyEvent。
func keyEventFromTable(L *lua.LState, t *lua.LTable) ime.KeyEvent {
	var ev ime.KeyEvent
	if v := t.RawGetString("rune"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			ev.Rune = rune(n)
		}
	}
	if v := t.RawGetString("code"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			ev.Code = uint16(n)
		}
	}
	if v := t.RawGetString("mods"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			ev.Mods = uint8(n)
		}
	}
	if v := t.RawGetString("state"); v != lua.LNil {
		if s, ok := v.(lua.LString); ok {
			ev.State = string(s) == "press"
		}
	}
	return ev
}

// responseToTable 把 ime.Response 转为 Lua table {consumed=,commit=,preedit=,candidates=...}。
func responseToTable(L *lua.LState, resp ime.Response) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("consumed", lua.LBool(resp.Consumed))
	t.RawSetString("commit", lua.LString(resp.Commit))
	t.RawSetString("preedit", lua.LString(resp.Preedit))
	candsT := L.NewTable()
	for i, c := range resp.Candidates {
		ct := L.NewTable()
		ct.RawSetString("word", lua.LString(c.Word))
		ct.RawSetString("code", lua.LString(c.Code))
		candsT.RawSetInt(i+1, ct)
	}
	t.RawSetString("candidates", candsT)
	return t
}

// responseFromTable 从 Lua table 读取 ime.Response。
func responseFromTable(L *lua.LState, t *lua.LTable) ime.Response {
	var resp ime.Response
	if v := t.RawGetString("consumed"); v != lua.LNil {
		if b, ok := v.(lua.LBool); ok {
			resp.Consumed = bool(b)
		}
	}
	if v := t.RawGetString("commit"); v != lua.LNil {
		if s, ok := v.(lua.LString); ok {
			resp.Commit = string(s)
		}
	}
	if v := t.RawGetString("preedit"); v != lua.LNil {
		if s, ok := v.(lua.LString); ok {
			resp.Preedit = string(s)
		}
	}
	if v := t.RawGetString("candidates"); v != lua.LNil {
		if ct, ok := v.(*lua.LTable); ok {
			n := ct.Len()
			resp.Candidates = make([]ime.Candidate, 0, n)
			for i := 1; i <= n; i++ {
				item := ct.RawGetInt(i)
				it, ok := item.(*lua.LTable)
				if !ok {
					continue
				}
				var c ime.Candidate
				if w := it.RawGetString("word"); w != lua.LNil {
					if s, ok := w.(lua.LString); ok {
						c.Word = string(s)
					}
				}
				if cd := it.RawGetString("code"); cd != lua.LNil {
					if s, ok := cd.(lua.LString); ok {
						c.Code = string(s)
					}
				}
				resp.Candidates = append(resp.Candidates, c)
			}
		}
	}
	return resp
}

// emptyResponseTable 返回空 Response 对应的 Lua table。
func emptyResponseTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("consumed", lua.LFalse)
	t.RawSetString("commit", lua.LString(""))
	t.RawSetString("preedit", lua.LString(""))
	t.RawSetString("candidates", L.NewTable())
	return t
}

// logHookError 记录 Lua 钩子 PCall 错误日志。
func (pm *PluginManager) logHookError(err error) {
	debug.Errorf("plugin ime hook error: %v", err)
}
