package plugins

import (
	"fmt"
	"os"

	lua "github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
)

// PluginManager 是插件系统的核心。它持有一个独立的 gopher-lua
// LState，加载 init.lua 配置，并管理 on_key / on_render 钩子。
//
// 生命周期：
//   1. NewPluginManager(initPath)  — 创建 LState、注册 API、不执行 init.lua
//   2. Load()                      — 执行 init.lua，返回 RunConfig（不存在则默认）
//   3. Activate(ctx)               — 注入 PluginContext，启用 term.* API
//   4. OnKey / OnRender            — 由主渲染线程串行调用
//   5. Reload()                    — 清状态重新 Load+Activate（保留 ctx）
//   6. Close()                     — 关闭 LState
//
// gopher-lua 非线程安全。所有 LState 操作必须在同一线程（主渲染线程）
// 串行执行，因此本结构不加锁——调用方负责串行性保证。
type PluginManager struct {
	L        *lua.LState
	ctx      PluginContext // Activate 前为 nil
	initPath string
	// keyHooks 暂存 on_key 注册的回调函数列表（LFunction 数组表）。
	keyHooks *lua.LTable
	// renderHooks 暂存 on_render 注册的回调函数列表。
	renderHooks *lua.LTable
	// panels 记录插件声明的面板开关，side → 行数。
	// key 为 "top"/"bottom"/"left"/"right"。
	panels map[string]int
	// active 标记是否已 Activate。未 active 时 OnKey/OnRender 直接返回。
	active bool
}

// NewPluginManager 创建插件管理器：初始化 LState、注册全部 vistty.*
// 命名空间、初始化钩子表与 panels。不执行 init.lua。
func NewPluginManager(initPath string) *PluginManager {
	L := lua.NewState()
	pm := &PluginManager{
		L:           L,
		initPath:    initPath,
		keyHooks:    L.NewTable(),
		renderHooks: L.NewTable(),
		panels:      make(map[string]int),
		active:      false,
	}
	registerAPIs(L, pm)
	return pm
}

// Load 执行 init.lua。文件不存在时静默回退，返回 DefaultRunConfig。
// 执行成功后读取 vistty.config 表并返回 RunConfig。
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

// Activate 注入 PluginContext 并标记管理器为活跃状态。
// 此后 vistty.term.* 等 API 可被 Lua 调用。
func (pm *PluginManager) Activate(ctx PluginContext) {
	pm.ctx = ctx
	pm.active = true
}

// Reload 清空钩子与面板状态，重新执行 Load + Activate。
// 保留 pm.ctx（若已注入），使插件能在运行期热更新。
func (pm *PluginManager) Reload() error {
	pm.keyHooks = pm.L.NewTable()
	pm.renderHooks = pm.L.NewTable()
	pm.panels = make(map[string]int)
	pm.active = false

	// 重新注册 API 以重置 vistty 表上的函数引用（reload 闭包等）。
	registerAPIs(pm.L, pm)

	if _, err := pm.Load(); err != nil {
		return err
	}
	if pm.ctx != nil {
		pm.Activate(pm.ctx)
	}
	return nil
}

// Close 关闭 LState，释放所有 Lua 资源。幂等。
func (pm *PluginManager) Close() {
	if pm.L != nil {
		pm.L.Close()
		pm.L = nil
	}
}

// EnabledPanels 返回 panels 的副本，供 OSD 读取插件声明的面板布局。
func (pm *PluginManager) EnabledPanels() map[string]int {
	out := make(map[string]int, len(pm.panels))
	for k, v := range pm.panels {
		out[k] = v
	}
	return out
}

// SetPanel 由 session 层 EnablePanel/DisablePanel 调用，运行期动态修改
// 插件声明的面板布局。lines<=0 表示禁用该边。
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

// OnKey 在主渲染线程被调用，依次执行 keyHooks 中的回调。
// ev 被构造为 Lua 表 {rune=, code=, mods=, state=}。
// 返回值语义：
//   - true / "consume"        → consumed=true，停止遍历
//   - nil / false             → 继续
//   - table{rune,code,mods}   → 改写 ev 继续传递给后续 hook
//
// 返回 (consumed, out)：
//   - consumed=true 时 out 无意义（不传给 handleKey），但仍返回 ev 保持接口一致
//   - consumed=false 时 out 为（可能被 hook 改写后的）新事件，传给 handleKey
//
// 未 active 或 keyHooks 为空时返回 (false, ev)。
func (pm *PluginManager) OnKey(ev platform.KeyEvent) (consumed bool, out platform.KeyEvent) {
	out = ev
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
		// 返回 table：改写当前 ev 继续传递后续 hook。
		if rt, ok := ret.(*lua.LTable); ok {
			pm.mergeKeyEventTable(tbl, rt)
		}
	})
	// 遍历结束后从 tbl 读回改写值（State 保持 ev.State，Lua 端不修改 state）。
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

// OnRender 在主渲染线程被调用，依次执行 renderHooks 中的回调。
// side 为 "top"/"bottom"/"left"/"right"。width/height 为该面板的 cell 尺寸，
// 供 ctx:size() 返回。top 面板暂不支持（与标签栏冲突），直接返回空。
// 返回 dirty=true 表示任一 hook 请求了重绘。
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

// newRenderContextUserdata 创建 OnRender 传给 Lua 的 ctx 对象。
// metatable 从 vistty._ctx_mt 读取（由 registerUI 写入）。
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

// renderContextUserDataName 保留作为内部标识（P4 扩展用）。
const renderContextUserDataName = "vistty.render_context"
