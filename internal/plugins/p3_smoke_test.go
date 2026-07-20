package plugins

import (
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

// p3FakeCtx 用于测试 OnKey 集成，不依赖 terminal
type p3FakeCtx struct {
	sent []byte
}

func (f *p3FakeCtx) FocusTerm() *terminal.Terminal                   { return nil }
func (f *p3FakeCtx) Terms() []*terminal.Terminal                     { return nil }
func (f *p3FakeCtx) NewTab() error                                   { return nil }
func (f *p3FakeCtx) CloseCurrentTab()                                {}
func (f *p3FakeCtx) NextTab()                                        {}
func (f *p3FakeCtx) PrevTab()                                        {}
func (f *p3FakeCtx) SwitchTab(i int)                                 {}
func (f *p3FakeCtx) TabList() []TabInfo                              { return nil }
func (f *p3FakeCtx) NextScreen()                                     {}
func (f *p3FakeCtx) PrevScreen()                                     {}
func (f *p3FakeCtx) SwitchScreen(i int)                              {}
func (f *p3FakeCtx) ScreenCount() int                                { return 1 }
func (f *p3FakeCtx) FocusScreenIdx() int                             { return 1 }
func (f *p3FakeCtx) ZoomIn()                                         {}
func (f *p3FakeCtx) ZoomOut()                                        {}
func (f *p3FakeCtx) ZoomReset()                                      {}
func (f *p3FakeCtx) EnablePanel(s string, n int)                     {}
func (f *p3FakeCtx) DisablePanel(s string)                           {}
func (f *p3FakeCtx) ReloadPlugins() error                            { return nil }
func (f *p3FakeCtx) Exit()                                           {}
func (f *p3FakeCtx) ApplyTheme(term terminal.Theme, osd ui.OSDTheme) {}

// TestP3OnKeyConsume 验证插件消费事件后 consumed=true
func TestP3OnKeyConsume(t *testing.T) {
	src := `vistty.input.on_key(function(ev)
		if ev.code == vistty.keys.SPACE and (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL then
			return true
		end
	end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p3FakeCtx{})

	// Ctrl+Space → consumed
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 57, Mods: platform.ModCtrl, State: platform.KeyPress})
	if !consumed {
		t.Fatal("Ctrl+Space should be consumed")
	}
	// 普通键 → not consumed
	consumed2, _ := pm.OnKey(platform.KeyEvent{Rune: 'a', State: platform.KeyPress})
	if consumed2 {
		t.Fatal("letter 'a' should not be consumed")
	}
	pm.Close()
}

// TestP3OnKeyRewrite 验证插件改写事件
func TestP3OnKeyRewrite(t *testing.T) {
	src := `vistty.input.on_key(function(ev)
		if ev.code == vistty.keys.F1 then
			return {code=vistty.keys.F2, mods=ev.mods, rune=0}
		end
	end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p3FakeCtx{})

	consumed, out := pm.OnKey(platform.KeyEvent{Code: 59, Mods: 0, State: platform.KeyPress})
	if consumed {
		t.Fatal("F1 should not be consumed")
	}
	if out.Code != 60 {
		t.Fatalf("F1 should rewrite to F2 (60), got %d", out.Code)
	}
	pm.Close()
}

// TestP3OnKeyMultipleHooks 验证多钩子链式调用
func TestP3OnKeyMultipleHooks(t *testing.T) {
	src := `
	local log = {}
	vistty.input.on_key(function(ev)
		table.insert(log, "hook1")
		if ev.code == vistty.keys.F1 then
			return true
		end
	end)
	vistty.input.on_key(function(ev)
		table.insert(log, "hook2")
	end)
	`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p3FakeCtx{})

	// F1 → hook1 消费，hook2 不应执行
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if !consumed {
		t.Fatal("F1 should be consumed by hook1")
	}
	// 普通键 → 两个 hook 都执行
	consumed2, _ := pm.OnKey(platform.KeyEvent{Rune: 'x', State: platform.KeyPress})
	if consumed2 {
		t.Fatal("'x' should not be consumed")
	}
	pm.Close()
}

// TestP3OnKeyNotActive 验证未 Activate 时 OnKey 返回 false
func TestP3OnKeyNotActive(t *testing.T) {
	src := `vistty.input.on_key(function(ev) return true end)`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 未 Activate
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 28, State: platform.KeyPress})
	if consumed {
		t.Fatal("should not be consumed when not active")
	}
	pm.Close()
}

// TestP3OnKeyEmptyHooks 验证无钩子时快速返回
func TestP3OnKeyEmptyHooks(t *testing.T) {
	pm := NewPluginManager("")
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(&p3FakeCtx{})
	consumed, out := pm.OnKey(platform.KeyEvent{Code: 28, State: platform.KeyPress})
	if consumed {
		t.Fatal("should not be consumed with no hooks")
	}
	if out.Code != 28 {
		t.Fatal("event should pass through unchanged")
	}
	pm.Close()
}
