package plugins

import (
	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/debug"
)

func registerDialogAPI(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	uiT, ok := vt.RawGetString("ui").(*lua.LTable)
	if !ok {
		return
	}

	uiT.RawSetString("toast", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		message := L.CheckString(1)
		level := L.OptInt(2, 0)
		duration := L.OptInt(3, 3000)
		pm.ctx.ShowToast(message, level, duration)
		return 0
	}))

	uiT.RawSetString("dialog", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		title := L.CheckString(1)
		content := L.OptString(2, "")
		placeholder := L.OptString(3, "")
		buttonsT := L.OptTable(4, nil)
		var buttons []string
		if buttonsT != nil {
			n := buttonsT.Len()
			for i := 1; i <= n; i++ {
				if v := buttonsT.RawGetInt(i); v != lua.LNil {
					if s, ok := v.(lua.LString); ok {
						buttons = append(buttons, string(s))
					}
				}
			}
		}
		var onClose func(result int, text string)
		if cb := L.OptFunction(5, nil); cb != nil {
			onClose = func(result int, text string) {
				L.Push(cb)
				L.Push(lua.LNumber(result))
				L.Push(lua.LString(text))
				if err := L.PCall(2, 0, nil); err != nil {
					debug.Errorf("dialog callback error: %v", err)
				}
			}
		}
		id := pm.ctx.ShowDialog(title, content, placeholder, buttons, onClose)
		L.Push(lua.LNumber(id))
		return 1
	}))

	uiT.RawSetString("close_dialog", L.NewFunction(func(L *lua.LState) int {
		if pm.ctx == nil {
			return 0
		}
		id := L.OptInt(1, 0)
		result, text, ok := pm.ctx.CloseDialog(id)
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(result))
		L.Push(lua.LString(text))
		return 2
	}))
}
