package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/ime"
	"github.com/LaoQi/vistty/ime/pinyin"
	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
)

type keyBinding struct {
	codes   []uint16
	indexed bool
	fn      *lua.LFunction
}

type PluginManager struct {
	L        *lua.LState
	ctx      PluginContext
	initPath string
	registry    *ime.Registry
	keyHooks    *lua.LTable
	renderHooks *lua.LTable
	panels      map[string]int
	bindings    []keyBinding
	pressedKeys map[uint16]bool
	active      bool
}

func NewPluginManager(initPath string) *PluginManager {
	L := lua.NewState()
	pm := &PluginManager{
		L:           L,
		initPath:    initPath,
		registry:    ime.NewRegistry(),
		keyHooks:    L.NewTable(),
		renderHooks: L.NewTable(),
		panels:      make(map[string]int),
		active:      false,
	}
	pm.registry.Register(pinyin.New())
	registerAPIs(L, pm)
	if initPath != "" {
		if dir, err := filepath.Abs(filepath.Dir(initPath)); err == nil {
			pkg := L.GetField(L.Get(lua.EnvironIndex), "package")
			if t, ok := pkg.(*lua.LTable); ok {
				cur := L.GetField(t, "path")
				curStr := ""
				if s, ok := cur.(lua.LString); ok {
					curStr = string(s)
				}
				extra := dir + string(os.PathSeparator) + "?.lua;" +
					dir + string(os.PathSeparator) + "?" + string(os.PathSeparator) + "init.lua"
				L.SetField(t, "path", lua.LString(extra+";"+curStr))
			}
		}
	}
	return pm
}

func (pm *PluginManager) Load() (*RunConfig, error) {
	if pm.initPath == "" {
		return DefaultRunConfig(), nil
	}
	if _, err := os.Stat(pm.initPath); err != nil {
		if os.IsNotExist(err) {
			debug.Debugf("plugins: init.lua not found at %s, using defaults", pm.initPath)
			return DefaultRunConfig(), nil
		}
		return nil, fmt.Errorf("stat init.lua: %w", err)
	}
	if err := pm.L.DoFile(pm.initPath); err != nil {
		return nil, fmt.Errorf("exec init.lua: %w", err)
	}
	return pm.readConfig()
}

func (pm *PluginManager) Activate(ctx PluginContext) {
	pm.ctx = ctx
	pm.active = true
}

func (pm *PluginManager) Reload() error {
	pm.keyHooks = pm.L.NewTable()
	pm.renderHooks = pm.L.NewTable()
	pm.panels = make(map[string]int)
	pm.bindings = nil
	pm.pressedKeys = make(map[uint16]bool)
	pm.active = false

	pm.registry.ClearLuaMethods()

	registerAPIs(pm.L, pm)

	if _, err := pm.Load(); err != nil {
		return err
	}
	if pm.ctx != nil {
		pm.Activate(pm.ctx)
	}
	return nil
}

func (pm *PluginManager) Close() {
	if pm.L != nil {
		pm.L.Close()
		pm.L = nil
	}
}

func (pm *PluginManager) EnabledPanels() map[string]int {
	out := make(map[string]int, len(pm.panels))
	for k, v := range pm.panels {
		out[k] = v
	}
	return out
}

func (pm *PluginManager) SetPanel(side string, lines int) {
	if pm.panels == nil {
		pm.panels = make(map[string]int)
	}
	if lines <= 0 {
		delete(pm.panels, side)
		return
	}
	pm.panels[side] = lines
}

func (pm *PluginManager) OnKey(ev platform.KeyEvent) (consumed bool, out platform.KeyEvent) {
	out = ev
	if pm.pressedKeys == nil {
		pm.pressedKeys = make(map[uint16]bool)
	}
	pm.pressedKeys[ev.Code] = ev.State == platform.KeyPress

	if ev.State != platform.KeyPress {
		return false, out
	}

	if pm.active {
		for i := range pm.bindings {
			b := &pm.bindings[i]
			idx := -1
			for j, c := range b.codes {
				if ev.Code == c {
					idx = j
					break
				}
			}
			if idx < 0 {
				continue
			}
			pm.L.Push(b.fn)
			nargs := 0
			if b.indexed {
				pm.L.Push(lua.LNumber(idx + 1))
				nargs = 1
			}
			if err := pm.L.PCall(nargs, 1, nil); err != nil {
				debug.Errorf("plugin bind error: %v", err)
			}
			ret := pm.L.Get(-1)
			pm.L.Pop(1)
			if ret == lua.LTrue || (ret.Type() == lua.LTBool && bool(ret.(lua.LBool))) {
				return true, ev
			}
		}
	}

	if !pm.active || pm.keyHooks == nil || pm.keyHooks.Len() == 0 {
		return false, out
	}
	tbl := pm.newKeyEventTable(ev)
	pm.keyHooks.ForEach(func(_, fn lua.LValue) {
		if consumed {
			return
		}
		lfn, ok := fn.(*lua.LFunction)
		if !ok {
			return
		}
		pm.L.Push(lfn)
		pm.L.Push(tbl)
		if err := pm.L.PCall(1, 1, nil); err != nil {
			debug.Errorf("plugin on_key error: %v", err)
			return
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		if ret == lua.LNil || ret == lua.LFalse {
			return
		}
		if b, ok := ret.(lua.LBool); ok {
			if bool(b) {
				consumed = true
			}
			return
		}
		if s, ok := ret.(lua.LString); ok {
			if string(s) == "consume" {
				consumed = true
			}
			return
		}
		if rt, ok := ret.(*lua.LTable); ok {
			pm.mergeKeyEventTable(tbl, rt)
		}
	})
	if v := pm.L.GetField(tbl, "rune"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			out.Rune = rune(n)
		}
	}
	if v := pm.L.GetField(tbl, "code"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			out.Code = uint16(n)
		}
	}
	if v := pm.L.GetField(tbl, "mods"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			out.Mods = platform.Modifiers(n)
		}
	}
	return consumed, out
}

func (pm *PluginManager) newKeyEventTable(ev platform.KeyEvent) *lua.LTable {
	t := pm.L.NewTable()
	t.RawSetString("rune", lua.LNumber(int(ev.Rune)))
	t.RawSetString("code", lua.LNumber(ev.Code))
	t.RawSetString("mods", lua.LNumber(int(ev.Mods)))
	if ev.State == platform.KeyPress {
		t.RawSetString("state", lua.LString("press"))
	} else {
		t.RawSetString("state", lua.LString("release"))
	}
	return t
}

func (pm *PluginManager) mergeKeyEventTable(dst, src *lua.LTable) {
	if v := pm.L.GetField(src, "rune"); v != lua.LNil {
		dst.RawSetString("rune", v)
	}
	if v := pm.L.GetField(src, "code"); v != lua.LNil {
		dst.RawSetString("code", v)
	}
	if v := pm.L.GetField(src, "mods"); v != lua.LNil {
		dst.RawSetString("mods", v)
	}
}

func (pm *PluginManager) OnRender(side string, width, height int) (dirty bool, primitives []Primitive) {
	if !pm.active || pm.renderHooks == nil || pm.renderHooks.Len() == 0 {
		return false, nil
	}
	if side == "top" {
		return false, nil
	}
	ud := pm.newRenderContextUserdata(side, width, height)
	st, _ := ud.Value.(*renderContextState)
	pm.renderHooks.ForEach(func(_, fn lua.LValue) {
		lfn, ok := fn.(*lua.LFunction)
		if !ok {
			return
		}
		pm.L.Push(lfn)
		pm.L.Push(ud)
		if err := pm.L.PCall(1, 1, nil); err != nil {
			debug.Errorf("plugin on_render error: %v", err)
			return
		}
		ret := pm.L.Get(-1)
		pm.L.Pop(1)
		if b, ok := ret.(lua.LBool); ok && bool(b) {
			dirty = true
		}
	})
	if st != nil {
		primitives = st.primitives
	}
	return dirty, primitives
}

func (pm *PluginManager) newRenderContextUserdata(side string, width, height int) *lua.LUserData {
	ud := pm.L.NewUserData()
	ud.Value = &renderContextState{width: width, height: height}
	vt, _ := pm.L.GetGlobal("vistty").(*lua.LTable)
	if vt != nil {
		if uiT, ok := vt.RawGetString("ui").(*lua.LTable); ok {
			if mt := uiT.RawGetString("_ctx_mt"); mt != lua.LNil {
				ud.Metatable = mt
			}
		}
	}
	return ud
}

const renderContextUserDataName = "vistty.render_context"
