package plugins

import (
	"testing"

	"github.com/LaoQi/vistty/terminal"
	lua "github.com/yuin/gopher-lua"
)

// p4FakeCtx 用于 P4 OnRender 测试
type p4FakeCtx struct{}

func (f *p4FakeCtx) FocusTerm() *terminal.Terminal { return nil }
func (f *p4FakeCtx) Terms() []*terminal.Terminal   { return nil }
func (f *p4FakeCtx) NewTab() error                 { return nil }
func (f *p4FakeCtx) CloseCurrentTab()              {}
func (f *p4FakeCtx) NextTab()                      {}
func (f *p4FakeCtx) PrevTab()                      {}
func (f *p4FakeCtx) SwitchTab(i int)               {}
func (f *p4FakeCtx) TabList() []TabInfo            { return nil }
func (f *p4FakeCtx) NextScreen()                   {}
func (f *p4FakeCtx) PrevScreen()                   {}
func (f *p4FakeCtx) SwitchScreen(i int)            {}
func (f *p4FakeCtx) ScreenCount() int              { return 1 }
func (f *p4FakeCtx) FocusScreenIdx() int           { return 1 }
func (f *p4FakeCtx) ZoomIn()                       {}
func (f *p4FakeCtx) ZoomOut()                      {}
func (f *p4FakeCtx) ZoomReset()                    {}
func (f *p4FakeCtx) EnablePanel(s string, n int)   {}
func (f *p4FakeCtx) DisablePanel(s string)         {}
func (f *p4FakeCtx) ReloadPlugins() error          { return nil }
func (f *p4FakeCtx) Exit()                          {}

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
