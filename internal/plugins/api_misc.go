package plugins

import (
	"fmt"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
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
	Fg, Bg [3]uint8
	Bold bool
}

// 图元类型常量。
const (
	PrimText = 0
	PrimRect = 1
)

// keysTable 是 vistty.keys 常量表，值为 evdev scancode。
var keysTable = map[string]int{
	"ESCAPE": 1, "BACKSPACE": 14, "TAB": 15, "ENTER": 28, "RETURN": 28,
	"LEFT": 105, "RIGHT": 106, "UP": 103, "DOWN": 108,
	"HOME": 102, "END": 107, "PAGE_UP": 104, "PAGE_DOWN": 109,
	"INSERT": 110, "DELETE": 111, "SPACE": 57,
	"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68, "F11": 87, "F12": 88,
	"LEFT_CTRL": 29, "RIGHT_CTRL": 97, "LEFT_ALT": 56, "RIGHT_ALT": 100,
	"LEFT_SHIFT": 42, "RIGHT_SHIFT": 54, "LEFT_SUPER": 125, "RIGHT_SUPER": 126,
}

// modsTable 是 vistty.mods 常量表，值为 platform.Modifiers 位值。
var modsTable = map[string]int{
	"CTRL":  int(platform.ModCtrl),
	"ALT":   int(platform.ModAlt),
	"SHIFT": int(platform.ModShift),
	"SUPER": int(platform.ModSuper),
}

// registerAPIs 在给定的 LState 上注册全部 vistty.* 命名空间。
// 由 NewPluginManager 与 Reload 调用，确保每次状态重建后 API 就位。
func registerAPIs(L *lua.LState, pm *PluginManager) {
	registerMisc(L, pm)
	registerTerm(L, pm)
	registerTab(L, pm)
	registerScreen(L, pm)
	registerZoom(L, pm)
	registerUI(L, pm)
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
// 以及 vistty.log / vistty.reload / vistty.on_key 函数。
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

	// vistty.log(msg)
	vt.RawSetString("log", L.NewFunction(luaVisttyLog))
	// vistty.reload() — 闭包捕获 pm
	vt.RawSetString("reload", L.NewFunction(func(L *lua.LState) int {
		if err := pm.Reload(); err != nil {
			L.RaiseError("vistty.reload: %v", err)
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
