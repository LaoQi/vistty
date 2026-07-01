package plugins

import (
	"github.com/LaoQi/vistty/pinyin"
	lua "github.com/yuin/gopher-lua"
)

func registerPinyin(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	pyT := L.NewTable()
	vt.RawSetString("pinyin", pyT)

	pyT.RawSetString("lookup", L.NewFunction(func(L *lua.LState) int {
		input := L.CheckString(1)
		cands := pinyin.Lookup(input)
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

	pyT.RawSetString("format_preedit", L.NewFunction(func(L *lua.LState) int {
		input := L.CheckString(1)
		L.Push(lua.LString(pinyin.FormatPreedit(input)))
		return 1
	}))

	pyT.RawSetString("split", L.NewFunction(func(L *lua.LState) int {
		input := L.CheckString(1)
		splits := pinyin.Split(input)
		t := L.NewTable()
		for i, split := range splits {
			st := L.NewTable()
			for j, s := range split {
				st.RawSetInt(j+1, lua.LString(s))
			}
			t.RawSetInt(i+1, st)
		}
		L.Push(t)
		return 1
	}))
}
