package plugins

import (
	"github.com/LaoQi/vistty/internal/platform"
	lua "github.com/yuin/gopher-lua"
)

// registerTerm 注册 vistty.term 命名空间。
// 提供发送输入、滚动、查询尺寸/标题、读取屏幕内容等终端操作能力。
// 所有方法在 pm.ctx == nil（未 Activate）时安全返回零值/no-op。
func registerTerm(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)
	termT := L.NewTable()
	vt.RawSetString("term", termT)

	// vistty.term.send(s, ...) — 发送 UTF-8 字符串到焦点终端 PTY
	termT.RawSetString("send", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		var buf []byte
		n := L.GetTop()
		for i := 1; i <= n; i++ {
			s := L.CheckString(i)
			buf = append(buf, s...)
		}
		if t := pm.ctx.FocusTerm(); t != nil {
			t.PtyWrite(buf)
		}
		return 0
	}))

	// vistty.term.send_key(code, mods) — 发送功能键转义序列
	termT.RawSetString("send_key", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		code := L.CheckInt(1)
		mods := L.OptInt(2, 0)
		if t := pm.ctx.FocusTerm(); t != nil {
			t.WriteKeyEscape(uint16(code), platform.Modifiers(mods))
		}
		return 0
	}))

	// vistty.term.scroll(n) — 绝对滚动偏移
	termT.RawSetString("scroll", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		n := L.CheckInt(1)
		if t := pm.ctx.FocusTerm(); t != nil {
			t.SetScrollOffset(n)
		}
		return 0
	}))

	// vistty.term.scroll_by(delta) — 相对滚动
	termT.RawSetString("scroll_by", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		delta := L.CheckInt(1)
		if t := pm.ctx.FocusTerm(); t != nil {
			t.SetScrollOffset(t.ScrollOffset() + delta)
		}
		return 0
	}))

	// vistty.term.history_len() → number
	termT.RawSetString("history_len", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		if t := pm.ctx.FocusTerm(); t != nil {
			L.Push(lua.LNumber(t.HistoryLen()))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// vistty.term.cols() → number
	termT.RawSetString("cols", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		if t := pm.ctx.FocusTerm(); t != nil {
			L.Push(lua.LNumber(t.Cols()))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// vistty.term.rows() → number
	termT.RawSetString("rows", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		if t := pm.ctx.FocusTerm(); t != nil {
			L.Push(lua.LNumber(t.Rows()))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// vistty.term.title() → string
	termT.RawSetString("title", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(lua.LString(""))
			return 1
		}
		if t := pm.ctx.FocusTerm(); t != nil {
			L.Push(lua.LString(t.Title()))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	}))

	// vistty.term.resize(cols, rows)
	termT.RawSetString("resize", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		cols := L.CheckInt(1)
		rows := L.CheckInt(2)
		if t := pm.ctx.FocusTerm(); t != nil {
			t.Resize(cols, rows)
		}
		return 0
	}))

	// vistty.term.read_screen() → 2D table [row][col]={rune=}
	// P2 先返回 rune 字段，fg/bg/attr 后续完善
	termT.RawSetString("read_screen", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			L.Push(L.NewTable())
			return 1
		}
		t := pm.ctx.FocusTerm()
		if t == nil {
			L.Push(L.NewTable())
			return 1
		}
		cells := t.ReadCells()
		result := L.NewTable()
		for rowIdx, row := range cells {
			rowT := L.NewTable()
			for colIdx, r := range row {
				cellT := L.NewTable()
				cellT.RawSetString("rune", lua.LNumber(int(r)))
				rowT.RawSetInt(colIdx+1, cellT)
			}
			result.RawSetInt(rowIdx+1, rowT)
		}
		L.Push(result)
		return 1
	}))
}
