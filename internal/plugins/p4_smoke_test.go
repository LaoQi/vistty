package plugins

import (
	"testing"

	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
	lua "github.com/yuin/gopher-lua"
)

// p4FakeCtx 用于 P4 OnRender 测试
type p4FakeCtx struct{}

func (f *p4FakeCtx) FocusTerm() *terminal.Terminal                   { return nil }
func (f *p4FakeCtx) Terms() []*terminal.Terminal                     { return nil }
func (f *p4FakeCtx) NewTab() error                                   { return nil }
func (f *p4FakeCtx) CloseCurrentTab()                                {}
func (f *p4FakeCtx) NextTab()                                        {}
func (f *p4FakeCtx) PrevTab()                                        {}
func (f *p4FakeCtx) SwitchTab(i int)                                 {}
func (f *p4FakeCtx) TabList() []TabInfo                              { return nil }
func (f *p4FakeCtx) NextScreen()                                     {}
func (f *p4FakeCtx) PrevScreen()                                     {}
func (f *p4FakeCtx) SwitchScreen(i int)                              {}
func (f *p4FakeCtx) ScreenCount() int                                { return 1 }
func (f *p4FakeCtx) FocusScreenIdx() int                             { return 1 }
func (f *p4FakeCtx) ZoomIn()                                         {}
func (f *p4FakeCtx) ZoomOut()                                        {}
func (f *p4FakeCtx) ZoomReset()                                      {}
func (f *p4FakeCtx) EnablePanel(s string, n int)                     {}
func (f *p4FakeCtx) DisablePanel(s string)                           {}
func (f *p4FakeCtx) ReloadPlugins() error                            { return nil }
func (f *p4FakeCtx) Exit()                                           {}
func (f *p4FakeCtx) ApplyTheme(term terminal.Theme, osd ui.OSDTheme) {}

// TestP4CtxTextRectOpts 验证 ctx:text/ctx:rect 的 opts 参数解析（fg/bg/bold）。
func TestP4CtxTextRectOpts(t *testing.T) {
	src := `
vistty.ui.on_render(function(ctx)
	ctx:rect(0, 0, 10, 1, {bg={10, 20, 30}})
	ctx:text(2, 0, "hi", {fg={255, 0, 0}, bg={0, 0, 0}, bold=true})
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})

	dirty, prims := pm.OnRender("bottom", 80, 1)
	if !dirty {
		// dirty 由 hook 返回值决定，这里 hook 无返回值，dirty 可能为 false
	}
	if len(prims) != 2 {
		t.Fatalf("expected 2 primitives, got %d", len(prims))
	}
	// rect
	if prims[0].Kind != PrimRect {
		t.Fatalf("prim0 kind != rect: %d", prims[0].Kind)
	}
	if prims[0].X != 0 || prims[0].Y != 0 || prims[0].W != 10 || prims[0].H != 1 {
		t.Fatalf("prim0 rect geom mismatch: %+v", prims[0])
	}
	if prims[0].Bg != [4]uint8{10, 20, 30, 255} {
		t.Fatalf("prim0 bg mismatch: %v", prims[0].Bg)
	}
	// text
	if prims[1].Kind != PrimText {
		t.Fatalf("prim1 kind != text: %d", prims[1].Kind)
	}
	if prims[1].X != 2 || prims[1].Y != 0 || prims[1].Text != "hi" {
		t.Fatalf("prim1 text geom/content mismatch: %+v", prims[1])
	}
	if prims[1].Fg != [4]uint8{255, 0, 0, 255} {
		t.Fatalf("prim1 fg mismatch: %v", prims[1].Fg)
	}
	if prims[1].Bg != [4]uint8{0, 0, 0, 255} {
		t.Fatalf("prim1 bg mismatch: %v", prims[1].Bg)
	}
	if !prims[1].Bold {
		t.Fatal("prim1 bold should be true")
	}
	pm.Close()
}

// TestP4CtxSize 验证 ctx:size() 返回 OnRender 传入的 width/height。
func TestP4CtxSize(t *testing.T) {
	src := `
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	vistty._w = w
	vistty._h = h
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})

	pm.OnRender("bottom", 80, 2)
	w := pm.L.GetField(pm.L.GetGlobal("vistty"), "_w")
	h := pm.L.GetField(pm.L.GetGlobal("vistty"), "_h")
	if n, ok := w.(lua.LNumber); !ok || int(n) != 80 {
		t.Fatalf("ctx:size() width expected 80, got %v", w)
	}
	if n, ok := h.(lua.LNumber); !ok || int(n) != 2 {
		t.Fatalf("ctx:size() height expected 2, got %v", h)
	}
	pm.Close()
}

// TestP4OnRenderTopSkipped 验证 top 面板被跳过（与标签栏冲突）。
func TestP4OnRenderTopSkipped(t *testing.T) {
	src := `
vistty.ui.on_render(function(ctx)
	ctx:text(0, 0, "should-not-appear")
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})

	_, prims := pm.OnRender("top", 80, 1)
	if len(prims) != 0 {
		t.Fatalf("top panel should be skipped, got %d primitives", len(prims))
	}
	pm.Close()
}

// TestP4OnRenderNotActive 验证未 Activate 时 OnRender 返回空。
func TestP4OnRenderNotActive(t *testing.T) {
	src := `vistty.ui.on_render(function(ctx) ctx:text(0,0,"x") end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 未 Activate
	_, prims := pm.OnRender("bottom", 80, 1)
	if len(prims) != 0 {
		t.Fatalf("not active should return no primitives, got %d", len(prims))
	}
	pm.Close()
}

// TestP4OnRenderNoHooks 验证无钩子时快速返回空。
func TestP4OnRenderNoHooks(t *testing.T) {
	pm := NewPluginManager("")
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	_, prims := pm.OnRender("bottom", 80, 1)
	if len(prims) != 0 {
		t.Fatalf("no hooks should return no primitives, got %d", len(prims))
	}
	pm.Close()
}

// TestP4OnRenderDirty 验证 hook 返回 true 时 dirty=true。
func TestP4OnRenderDirty(t *testing.T) {
	src := `vistty.ui.on_render(function(ctx) return true end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	dirty, _ := pm.OnRender("bottom", 80, 1)
	if !dirty {
		t.Fatal("hook returning true should set dirty=true")
	}
	pm.Close()
}

// TestP4SetPanel 验证 SetPanel 动态修改面板声明。
func TestP4SetPanel(t *testing.T) {
	pm := NewPluginManager("")
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.SetPanel("bottom", 2)
	panels := pm.EnabledPanels()
	if panels["bottom"] != 2 {
		t.Fatalf("SetPanel bottom=2 not reflected: %+v", panels)
	}
	pm.SetPanel("bottom", 0)
	panels = pm.EnabledPanels()
	if _, ok := panels["bottom"]; ok {
		t.Fatal("SetPanel bottom=0 should disable")
	}
	pm.Close()
}

// TestEnvBackendName 验证 backend_name / backend.is_wayland / backend.is_drm
// 在 SetBackendName 注入前后返回正确值。
func TestEnvBackendName(t *testing.T) {
	pm := NewPluginManager("")
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 未注入时返回空串，is_wayland/is_drm 均为 false
	if got := luaStr(t, pm, `return vistty.backend_name()`); got != "" {
		t.Fatalf("before inject, backend_name expected \"\", got %q", got)
	}
	if got := luaBool(t, pm, `return vistty.backend.is_wayland()`); got {
		t.Fatal("before inject, is_wayland should be false")
	}
	if got := luaBool(t, pm, `return vistty.backend.is_drm()`); got {
		t.Fatal("before inject, is_drm should be false")
	}
	// 注入 wayland
	pm.SetBackendName("wayland")
	if got := luaStr(t, pm, `return vistty.backend_name()`); got != "wayland" {
		t.Fatalf("after inject wayland, backend_name expected \"wayland\", got %q", got)
	}
	if got := luaBool(t, pm, `return vistty.backend.is_wayland()`); !got {
		t.Fatal("after inject wayland, is_wayland should be true")
	}
	if got := luaBool(t, pm, `return vistty.backend.is_drm()`); got {
		t.Fatal("after inject wayland, is_drm should be false")
	}
	// 注入 drm-gbm：is_drm 为 true，is_wayland 为 false
	pm.SetBackendName("drm-gbm")
	if got := luaBool(t, pm, `return vistty.backend.is_drm()`); !got {
		t.Fatal("after inject drm-gbm, is_drm should be true")
	}
	if got := luaBool(t, pm, `return vistty.backend.is_wayland()`); got {
		t.Fatal("after inject drm-gbm, is_wayland should be false")
	}
	pm.Close()
}

// TestOnActivate 验证 on_activate 钩子在 Activate 时以 backend_name 为参数被回调。
func TestOnActivate(t *testing.T) {
	src := `
vistty.on_activate(function(name)
	vistty._activated = name
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.SetBackendName("drm")
	// Activate 前钩子未触发
	if v := pm.L.GetField(pm.L.GetGlobal("vistty"), "_activated"); v != lua.LNil {
		t.Fatalf("before Activate, _activated should be nil, got %v", v)
	}
	pm.Activate(&p4FakeCtx{})
	got := pm.L.GetField(pm.L.GetGlobal("vistty"), "_activated")
	if s, ok := got.(lua.LString); !ok || string(s) != "drm" {
		t.Fatalf("after Activate, _activated expected \"drm\", got %v", got)
	}
	pm.Close()
}

// TestOnActivateReload 验证 Reload 后 activateHooks 被重置且重新 Activate 触发新钩子。
func TestOnActivateReload(t *testing.T) {
	src := `
vistty.on_activate(function(name) vistty._act = name end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.SetBackendName("wayland")
	pm.Activate(&p4FakeCtx{})
	if s := luaStr(t, pm, `return vistty._act`); s != "wayland" {
		t.Fatalf("first activate expected wayland, got %q", s)
	}
	// Reload 清空钩子并重新加载同一文件
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	pm.SetBackendName("drm")
	pm.Activate(&p4FakeCtx{})
	if s := luaStr(t, pm, `return vistty._act`); s != "drm" {
		t.Fatalf("after reload+activate expected drm, got %q", s)
	}
	pm.Close()
}

func luaStr(t *testing.T, pm *PluginManager, src string) string {
	t.Helper()
	if err := pm.L.DoString(src); err != nil {
		t.Fatalf("lua error: %v", err)
	}
	v := pm.L.Get(-1)
	pm.L.Pop(1)
	s, ok := v.(lua.LString)
	if !ok {
		t.Fatalf("expected string, got %T(%v)", v, v)
	}
	return string(s)
}

func luaBool(t *testing.T, pm *PluginManager, src string) bool {
	t.Helper()
	if err := pm.L.DoString(src); err != nil {
		t.Fatalf("lua error: %v", err)
	}
	v := pm.L.Get(-1)
	pm.L.Pop(1)
	b, ok := v.(lua.LBool)
	if !ok {
		t.Fatalf("expected bool, got %T(%v)", v, v)
	}
	return bool(b)
}

// TestOnActivateErrorIsolation 验证单个 on_activate 钩子出错不影响后续钩子执行。
func TestOnActivateErrorIsolation(t *testing.T) {
	src := `
vistty.on_activate(function(name) vistty._a1 = name end)
vistty.on_activate(function(name) error("boom") end)
vistty.on_activate(function(name) vistty._a2 = name end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.SetBackendName("drm")
	pm.Activate(&p4FakeCtx{})

	if s := luaStr(t, pm, `return vistty._a1`); s != "drm" {
		t.Fatalf("first hook expected drm, got %q", s)
	}
	if s := luaStr(t, pm, `return vistty._a2`); s != "drm" {
		t.Fatalf("third hook expected drm despite second erroring, got %q", s)
	}
	pm.Close()
}

// TestLifecycleExit 验证 on_exit 钩子在 FireExitHooks 时触发，且不依赖 active。
func TestLifecycleExit(t *testing.T) {
	src := `vistty.on_exit(function() vistty._exited = true end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 未 Activate 也能触发 on_exit
	pm.FireExitHooks()
	if v := pm.L.GetField(pm.L.GetGlobal("vistty"), "_exited"); v != lua.LTrue {
		t.Fatalf("on_exit should fire even without Activate, got %v", v)
	}
	pm.Close()
}

// TestLifecycleTabNew 验证 on_tab_new 收到 idx 和 title 参数。
func TestLifecycleTabNew(t *testing.T) {
	src := `vistty.on_tab_new(function(idx, title) vistty._idx, vistty._title = idx, title end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireTabNew(3, "bash")
	if n := luaNum(t, pm, `return vistty._idx`); n != 3 {
		t.Fatalf("on_tab_new idx expected 3, got %v", n)
	}
	if s := luaStr(t, pm, `return vistty._title`); s != "bash" {
		t.Fatalf("on_tab_new title expected bash, got %q", s)
	}
	pm.Close()
}

// TestLifecycleTabClose 验证 on_tab_close 收到 idx 和 title 参数。
func TestLifecycleTabClose(t *testing.T) {
	src := `vistty.on_tab_close(function(idx, title) vistty._ci, vistty._ct = idx, title end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireTabClose(1, "vim")
	if n := luaNum(t, pm, `return vistty._ci`); n != 1 {
		t.Fatalf("on_tab_close idx expected 1, got %v", n)
	}
	if s := luaStr(t, pm, `return vistty._ct`); s != "vim" {
		t.Fatalf("on_tab_close title expected vim, got %q", s)
	}
	pm.Close()
}

// TestLifecycleTabSwitch 验证 on_tab_switch 收到 newIdx 和 oldIdx 参数。
func TestLifecycleTabSwitch(t *testing.T) {
	src := `vistty.on_tab_switch(function(n, o) vistty._n, vistty._o = n, o end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireTabSwitch(2, 1)
	if n := luaNum(t, pm, `return vistty._n`); n != 2 {
		t.Fatalf("on_tab_switch newIdx expected 2, got %v", n)
	}
	if o := luaNum(t, pm, `return vistty._o`); o != 1 {
		t.Fatalf("on_tab_switch oldIdx expected 1, got %v", o)
	}
	pm.Close()
}

// TestLifecycleScreenSwitch 验证 on_screen_switch 收到 idx 参数。
func TestLifecycleScreenSwitch(t *testing.T) {
	src := `vistty.on_screen_switch(function(idx) vistty._si = idx end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireScreenSwitch(2)
	if n := luaNum(t, pm, `return vistty._si`); n != 2 {
		t.Fatalf("on_screen_switch idx expected 2, got %v", n)
	}
	pm.Close()
}

// TestLifecycleTitleChange 验证 on_title_change 收到 title 参数。
func TestLifecycleTitleChange(t *testing.T) {
	src := `vistty.on_title_change(function(title) vistty._t = title end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireTitleChange("user@host:~/code")
	if s := luaStr(t, pm, `return vistty._t`); s != "user@host:~/code" {
		t.Fatalf("on_title_change title expected, got %q", s)
	}
	pm.Close()
}

// TestLifecycleResize 验证 on_resize 收到 5 个参数。
func TestLifecycleResize(t *testing.T) {
	src := `vistty.on_resize(function(oid, w, h, c, r) vistty._r = w .. "x" .. h .. " " .. c .. "c" .. r .. "r" end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireResize(42, 1920, 1080, 200, 50)
	if s := luaStr(t, pm, `return vistty._r`); s != "1920x1080 200c50r" {
		t.Fatalf("on_resize args mismatch, got %q", s)
	}
	pm.Close()
}

// TestLifecycleZoom 验证 on_zoom 收到字号参数。
func TestLifecycleZoom(t *testing.T) {
	src := `vistty.on_zoom(function(size) vistty._z = size end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p4FakeCtx{})
	pm.FireZoom(16.5)
	if n := luaNum(t, pm, `return vistty._z`); n != 16.5 {
		t.Fatalf("on_zoom size expected 16.5, got %v", n)
	}
	pm.Close()
}

// TestLifecycleInactiveSkip 验证未 Activate 时生命周期钩子（除 on_exit）不触发。
func TestLifecycleInactiveSkip(t *testing.T) {
	src := `vistty.on_tab_new(function() vistty._fired = true end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 未 Activate，FireTabNew 应跳过
	pm.FireTabNew(1, "x")
	if v := pm.L.GetField(pm.L.GetGlobal("vistty"), "_fired"); v != lua.LNil {
		t.Fatal("on_tab_new should not fire when inactive")
	}
	pm.Close()
}

func luaNum(t *testing.T, pm *PluginManager, src string) float64 {
	t.Helper()
	if err := pm.L.DoString(src); err != nil {
		t.Fatalf("lua error: %v", err)
	}
	v := pm.L.Get(-1)
	pm.L.Pop(1)
	n, ok := v.(lua.LNumber)
	if !ok {
		t.Fatalf("expected number, got %T(%v)", v, v)
	}
	return float64(n)
}
