package plugins

import (
	lua "github.com/yuin/gopher-lua"
)

// registerUI 注册 vistty.ui 命名空间：enable / disable / on_render。
// enable(side, lines) / disable(side) 写入 pm.panels 供 OSD 查询；
// on_render(fn) 将回调追加到 renderHooks，供 OnRender 触发。
//
// P1 阶段实现完整骨架（panel 声明 + render hook 注册），
// ctx:text / ctx:rect 的 Lua 绑定在 P4 阶段完善。
func registerUI(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	uiT := L.NewTable()
	vt.RawSetString("ui", uiT)

	// vistty.ui.enable(side, lines)
	uiT.RawSetString("enable", L.NewFunction(func(L *lua.LState) int {
		side := L.CheckString(1)
		lines := L.OptInt(2, 1)
		if !isValidSide(side) {
			L.RaiseError("vistty.ui.enable: invalid side %q", side)
		}
		pm.panels[side] = lines
		return 0
	}))

	// vistty.ui.disable(side)
	uiT.RawSetString("disable", L.NewFunction(func(L *lua.LState) int {
		side := L.CheckString(1)
		if !isValidSide(side) {
			L.RaiseError("vistty.ui.disable: invalid side %q", side)
		}
		delete(pm.panels, side)
		return 0
	}))

	// vistty.ui.on_render(fn)
	uiT.RawSetString("on_render", L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		pm.renderHooks.Append(fn)
		return 0
	}))

	// 注册 renderContext 的 metatable（text/rect 方法）。
	// userdata 的方法调用通过 __index 元方法转发到方法表。
	mt := L.NewTable()
	methods := L.NewTable()
	methods.RawSetString("text", L.NewFunction(luaCtxText))
	methods.RawSetString("rect", L.NewFunction(luaCtxRect))
	methods.RawSetString("size", L.NewFunction(luaCtxSize))
	mt.RawSetString("__index", methods)
	// 将 mt 存到一个固定位置供 newRenderContextUserdata 查找。
	uiT.RawSetString("_ctx_mt", mt)
}

// renderContextState 是 ctx:text / ctx:rect 调用过程中维护的图元收集状态，
// 依附于 renderContext 的生命周期。width/height 为该面板的 cell 尺寸。
type renderContextState struct {
	primitives []Primitive
	width      int
	height     int
}

// renderContextMetatableName 是 ctx userdata 的元表标识，
// 存储在 vistty._ctx_mt 字段中供 newRenderContext 查找。
const renderContextMetatableName = "vistty.render_context"

// luaCtxText: ctx:text(x, y, str, opts?) — opts={fg={r,g,b}, bg={r,g,b}, bold=bool}。
// x/y 为 cell 单位坐标。
func luaCtxText(L *lua.LState) int {
	ud, ok := L.Get(1).(*lua.LUserData)
	if !ok {
		L.RaiseError("ctx:text: self is not userdata")
		return 0
	}
	st, ok := ud.Value.(*renderContextState)
	if !ok {
		L.RaiseError("ctx:text: invalid context state")
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	text := L.CheckString(4)
	prim := Primitive{
		Kind: PrimText,
		X:    x,
		Y:    y,
		Text: text,
	}
	if L.GetTop() >= 5 {
		parsePrimitiveOpts(L, 5, &prim)
	}
	st.primitives = append(st.primitives, prim)
	return 0
}

// luaCtxRect: ctx:rect(x, y, w, h, opts?) — opts={bg={r,g,b}, fg={r,g,b}}。
// x/y/w/h 为 cell 单位。
func luaCtxRect(L *lua.LState) int {
	ud, ok := L.Get(1).(*lua.LUserData)
	if !ok {
		L.RaiseError("ctx:rect: self is not userdata")
		return 0
	}
	st, ok := ud.Value.(*renderContextState)
	if !ok {
		L.RaiseError("ctx:rect: invalid context state")
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	w := L.CheckInt(4)
	h := L.CheckInt(5)
	prim := Primitive{
		Kind: PrimRect,
		X:    x,
		Y:    y,
		W:    w,
		H:    h,
	}
	if L.GetTop() >= 6 {
		parsePrimitiveOpts(L, 6, &prim)
	}
	st.primitives = append(st.primitives, prim)
	return 0
}

// luaCtxSize: ctx:size() → (width, height) — 返回当前面板的 cell 尺寸。
func luaCtxSize(L *lua.LState) int {
	ud, ok := L.Get(1).(*lua.LUserData)
	if !ok {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	}
	st, ok := ud.Value.(*renderContextState)
	if !ok {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	}
	L.Push(lua.LNumber(st.width))
	L.Push(lua.LNumber(st.height))
	return 2
}

// parsePrimitiveOpts 从 idx 处的 Lua table 读取 fg/bg/bold 字段写入 prim。
// fg/bg 接受：
//   - 字符串 "#RGB" / "#RGBA" / "#RRGGBB" / "#RRGGBBAA"（可省略 #）
//   - table {r,g,b} 或 {r,g,b,a}
func parsePrimitiveOpts(L *lua.LState, idx int, prim *Primitive) {
	t := L.OptTable(idx, nil)
	if t == nil {
		return
	}
	if v := L.GetField(t, "fg"); v != lua.LNil {
		prim.Fg = parseColor(L, v)
	}
	if v := L.GetField(t, "bg"); v != lua.LNil {
		prim.Bg = parseColor(L, v)
	}
	if v := L.GetField(t, "bold"); v != lua.LNil {
		if b, ok := v.(lua.LBool); ok {
			prim.Bold = bool(b)
		}
	}
}

// parseColor 将 Lua 值解析为 [4]uint8 RGBA 颜色。
// 接受字符串 "#RRGGBB"/"#RRGGBBAA"/"#RGB"/"#RGBA" 或 table {r,g,b}/{r,g,b,a}。
func parseColor(L *lua.LState, v lua.LValue) [4]uint8 {
	if s, ok := v.(lua.LString); ok {
		return parseHexColor(string(s))
	}
	if t, ok := v.(*lua.LTable); ok {
		return parseTableColor(L, t)
	}
	return [4]uint8{}
}

// parseHexColor 解析 "#RRGGBB"/"#RRGGBBAA"/"#RGB"/"#RGBA" 格式。
// # 可省略。缺省 alpha 时默认 255。
func parseHexColor(s string) [4]uint8 {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	var r, g, b, a uint8 = 0, 0, 0, 255
	switch len(s) {
	case 3: // RGB
		r = hexVal(s[0]) * 17
		g = hexVal(s[1]) * 17
		b = hexVal(s[2]) * 17
	case 4: // RGBA
		r = hexVal(s[0]) * 17
		g = hexVal(s[1]) * 17
		b = hexVal(s[2]) * 17
		a = hexVal(s[3]) * 17
	case 6: // RRGGBB
		r = hexVal(s[0])*16 + hexVal(s[1])
		g = hexVal(s[2])*16 + hexVal(s[3])
		b = hexVal(s[4])*16 + hexVal(s[5])
	case 8: // RRGGBBAA
		r = hexVal(s[0])*16 + hexVal(s[1])
		g = hexVal(s[2])*16 + hexVal(s[3])
		b = hexVal(s[4])*16 + hexVal(s[5])
		a = hexVal(s[6])*16 + hexVal(s[7])
	}
	return [4]uint8{r, g, b, a}
}

func hexVal(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// parseTableColor 从 Lua table {r,g,b} 或 {r,g,b,a} 读取颜色。
func parseTableColor(L *lua.LState, t *lua.LTable) [4]uint8 {
	var c [4]uint8
	c[3] = 255 // 默认不透明
	if v := t.RawGetInt(1); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			c[0] = uint8(n)
		}
	}
	if v := t.RawGetInt(2); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			c[1] = uint8(n)
		}
	}
	if v := t.RawGetInt(3); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			c[2] = uint8(n)
		}
	}
	if v := t.RawGetInt(4); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			c[3] = uint8(n)
		}
	}
	return c
}

func isValidSide(s string) bool {
	switch s {
	case "bottom", "left", "right":
		return true
	}
	return false
}
