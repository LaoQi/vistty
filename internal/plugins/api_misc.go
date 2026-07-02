package plugins

import (
	"fmt"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/runeutil"
	lua "github.com/yuin/gopher-lua"
)

// Primitive 是 OnRender 钩子中由 Lua 端 ctx:text / ctx:rect
// 产生的图元描述。P1 阶段先定义结构骨架，P4 阶段由 ctx 的
// text/rect metatable 方法填充并收集。
type Primitive struct {
	Kind int // 0=text, 1=rect
	X, Y int
	W, H int
	Text string
	Fg, Bg [4]uint8 // RGBA
	Bold bool
}

// 图元类型常量。
const (
	PrimText = 0
	PrimRect = 1
)

// keysTable 是 vistty.keys 常量表，值为 evdev scancode（Linux input-event-codes.h）。
// 覆盖修饰键/系统键/数字键/符号键/字母键/功能键/方向编辑键/小键盘全系列。
var keysTable = map[string]int{
	// 修饰键
	"LEFT_CTRL": 29, "RIGHT_CTRL": 97,
	"LEFT_ALT": 56, "RIGHT_ALT": 100,
	"LEFT_SHIFT": 42, "RIGHT_SHIFT": 54,
	"LEFT_SUPER": 125, "RIGHT_SUPER": 126,
	// 系统键
	"CAPSLOCK": 58, "NUMLOCK": 69, "SCROLLLOCK": 70,
	"SYSRQ": 99, "PAUSE": 119, "PRINT": 210, "MENU": 139,
	// 转义键（ESCAPE 别名 ESC）
	"ESCAPE": 1, "ESC": 1,
	// 数字键（主键盘区）
	"NUM1": 2, "NUM2": 3, "NUM3": 4, "NUM4": 5, "NUM5": 6,
	"NUM6": 7, "NUM7": 8, "NUM8": 9, "NUM9": 10, "NUM0": 11,
	// 符号键（Linux input-event-codes.h 标准命名）
	"MINUS": 12, "EQUAL": 13, "BACKSPACE": 14,
	"TAB": 15,
	"LEFTBRACE": 26, "RIGHTBRACE": 27, "ENTER": 28, "RETURN": 28,
	"SEMICOLON": 39, "APOSTROPHE": 40, "GRAVE": 41, "BACKSLASH": 43,
	"COMMA": 51, "DOT": 52, "SLASH": 53,
	"SPACE": 57,
	// 字母键（QWERTY 布局，值为 evdev scancode）
	"Q": 16, "W": 17, "E": 18, "R": 19, "T": 20, "Y": 21, "U": 22,
	"I": 23, "O": 24, "P": 25,
	"A": 30, "S": 31, "D": 32, "F": 33, "G": 34, "H": 35, "J": 36,
	"K": 37, "L": 38,
	"Z": 44, "X": 45, "C": 46, "V": 47, "B": 48, "N": 49, "M": 50,
	// 功能键
	"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68, "F11": 87, "F12": 88,
	// 方向 / 编辑键
	"UP": 103, "DOWN": 108, "LEFT": 105, "RIGHT": 106,
	"HOME": 102, "END": 107, "PAGE_UP": 104, "PAGE_DOWN": 109,
	"INSERT": 110, "DELETE": 111,
	// 小键盘
	"KP7": 71, "KP8": 72, "KP9": 73,
	"KP4": 75, "KP5": 76, "KP6": 77, "KPPLUS": 78,
	"KP1": 79, "KP2": 80, "KP3": 81, "KP0": 82, "KPDOT": 83,
	"KPENTER": 96, "KPSLASH": 98, "KPASTERISK": 55, "KPMINUS": 74, "KPEQUAL": 117,
}

// modsTable 是 vistty.mods 常量表，值为 platform.Modifiers 位值。
var modsTable = map[string]int{
	"CTRL":  int(platform.ModCtrl),
	"ALT":   int(platform.ModAlt),
	"SHIFT": int(platform.ModShift),
	"SUPER": int(platform.ModSuper),
}

// stateTable 是 vistty.state 常量表，值为 newKeyEventTable 中 state 字段的字符串值，
// 便于插件端用 ev.state == vistty.state.PRESS 直接比较，避免写字符串字面量。
var stateTable = map[string]string{
	"PRESS":   "press",
	"RELEASE": "release",
}

// colorsTable 是 vistty.colors 常量表，值为 "#RRGGBB" 字符串。
// 可直接传给 ctx:text / ctx:rect 的 fg/bg 参数，也可追加 AA 通道：
//   vistty.colors.RED            → "#FF0000"
//   vistty.colors.RED .. "FF"    → "#FF0000FF"（半透明红）
var colorsTable = map[string]string{
	"BLACK":   "#000000",
	"WHITE":   "#FFFFFF",
	"RED":     "#FF0000",
	"GREEN":   "#00FF00",
	"BLUE":    "#0000FF",
	"YELLOW":  "#FFFF00",
	"CYAN":    "#00FFFF",
	"MAGENTA": "#FF00FF",
	"ORANGE":  "#FF8800",
	"PURPLE":  "#8844CC",
	"PINK":    "#FF8888",
	"LIME":    "#88FF00",
	"TEAL":    "#008888",
	"GRAY":    "#888888",
	"GREY":    "#888888",
	"DARKGRAY":   "#444444",
	"DARKGREY":   "#444444",
	"LIGHTGRAY":  "#CCCCCC",
	"LIGHTGREY":  "#CCCCCC",
	"BROWN":   "#884400",
	"NAVY":    "#000088",
	"MAROON":  "#880000",
	"OLIVE":   "#888800",
	"SILVER":  "#C0C0C0",
	"GOLD":    "#FFD700",
	"CORAL":   "#FF7F50",
	"SALMON":  "#FA8072",
	"KHAKI":   "#F0E68C",
	"IVORY":   "#FFFFF0",
	"INDIGO":  "#4B0082",
}

// registerAPIs 在给定的 LState 上注册全部 vistty.* 命名空间。
// 由 NewPluginManager 与 Reload 调用，确保每次状态重建后 API 就位。
func registerAPIs(L *lua.LState, pm *PluginManager) {
	registerMisc(L, pm)
	registerKeybind(L, pm)
	registerTerm(L, pm)
	registerTab(L, pm)
	registerScreen(L, pm)
	registerZoom(L, pm)
	registerUI(L, pm)
	registerPinyin(L, pm)
	registerEnv(L, pm)
	registerLifecycle(L, pm)
}

// ensureVisttyTable 确保全局 vistty 表存在并返回它。
func ensureVisttyTable(L *lua.LState) *lua.LTable {
	if t, ok := L.GetGlobal("vistty").(*lua.LTable); ok {
		return t
	}
	t := L.NewTable()
	L.SetGlobal("vistty", t)
	return t
}

// registerMisc 注册 vistty.keys / vistty.mods 常量表，
// 以及 vistty.log / vistty.reload / vistty.exit / vistty.on_key 函数。
func registerMisc(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	keys := L.NewTable()
	for k, v := range keysTable {
		keys.RawSetString(k, lua.LNumber(v))
	}
	vt.RawSetString("keys", keys)

	mods := L.NewTable()
	for k, v := range modsTable {
		mods.RawSetString(k, lua.LNumber(v))
	}
	vt.RawSetString("mods", mods)

	// vistty.state — PRESS/RELEASE 字符串常量，与 ev.state 字段直接比较
	state := L.NewTable()
	for k, v := range stateTable {
		state.RawSetString(k, lua.LString(v))
	}
	vt.RawSetString("state", state)

	// vistty.colors — 常用颜色 "#RRGGBB" 字符串，可直接作为 fg/bg 参数
	colors := L.NewTable()
	for k, v := range colorsTable {
		colors.RawSetString(k, lua.LString(v))
	}
	vt.RawSetString("colors", colors)

	vt.RawSetString("display_width", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		L.Push(lua.LNumber(runeutil.StringWidth(s)))
		return 1
	}))

	// vistty.log(msg)
	vt.RawSetString("log", L.NewFunction(luaVisttyLog))
	// vistty.reload() — 闭包捕获 pm
	vt.RawSetString("reload", L.NewFunction(func(L *lua.LState) int {
		if err := pm.Reload(); err != nil {
			L.RaiseError("vistty.reload: %v", err)
		}
		return 0
	}))
	// vistty.exit() — 请求退出主循环（幂等）
	vt.RawSetString("exit", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx != nil {
			pm.ctx.Exit()
		}
		return 0
	}))
	// vistty.input.on_key(fn) — 闭包捕获 pm
	inputT := L.NewTable()
	vt.RawSetString("input", inputT)
	inputT.RawSetString("on_key", L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		pm.keyHooks.Append(fn)
		return 0
	}))
}

// luaVisttyLog: vistty.log(msg) — 将消息写入调试日志。
func luaVisttyLog(L *lua.LState) int {
	msg := L.CheckString(1)
	debug.Debugf("plugin: %s", msg)
	return 0
}

// formatKeyError 统一格式化按键事件构造错误（P1 暂未使用，预留）。
func formatKeyError(field string, got lua.LValue) error {
	return fmt.Errorf("invalid %s: %v", field, got)
}
