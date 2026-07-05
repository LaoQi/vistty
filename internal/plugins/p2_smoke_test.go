package plugins

import (
	"testing"

	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

// fakeCtx 实现 PluginContext 接口，用于 P2 验证 vistty.term/tab/screen/zoom API
type fakeCtx struct {
	tabs      []TabInfo
	screenIdx int
	zoomCalls []int
}

func (f *fakeCtx) FocusTerm() *terminal.Terminal { return nil }
func (f *fakeCtx) Terms() []*terminal.Terminal   { return nil }
func (f *fakeCtx) NewTab() error                 { f.tabs = append(f.tabs, TabInfo{Title: "new", Active: true}); return nil }
func (f *fakeCtx) CloseCurrentTab()              {}
func (f *fakeCtx) NextTab()                      {}
func (f *fakeCtx) PrevTab()                      {}
func (f *fakeCtx) SwitchTab(i int)               {}
func (f *fakeCtx) TabList() []TabInfo            { return f.tabs }
func (f *fakeCtx) NextScreen()                   { f.screenIdx++ }
func (f *fakeCtx) PrevScreen()                   { f.screenIdx-- }
func (f *fakeCtx) SwitchScreen(i int)            { f.screenIdx = i - 1 }
func (f *fakeCtx) ScreenCount() int              { return 3 }
func (f *fakeCtx) FocusScreenIdx() int           { return f.screenIdx + 1 }
func (f *fakeCtx) ZoomIn()                       { f.zoomCalls = append(f.zoomCalls, 1) }
func (f *fakeCtx) ZoomOut()                      { f.zoomCalls = append(f.zoomCalls, -1) }
func (f *fakeCtx) ZoomReset()                    { f.zoomCalls = append(f.zoomCalls, 0) }
func (f *fakeCtx) EnablePanel(s string, n int)   {}
func (f *fakeCtx) DisablePanel(s string)         {}
func (f *fakeCtx) ReloadPlugins() error          { return nil }
func (f *fakeCtx) Exit()                          {}
func (f *fakeCtx) ApplyTheme(term terminal.Theme, osd ui.OSDTheme) {}

func TestP2TabAPI(t *testing.T) {
	src := `
vistty.tab.new()
assert(vistty.tab.count() == 1, "tab count should be 1, got " .. tostring(vistty.tab.count()))
local list = vistty.tab.list()
assert(#list == 1, "list len should be 1")
assert(list[1].title == "new", "title should be 'new'")
assert(list[1].active == true, "should be active")
vistty.tab.next()
vistty.tab.prev()
vistty.tab.close()
`
	pm := newPMWithCtx(t, src, &fakeCtx{tabs: []TabInfo{}})
	if pm == nil {
		return
	}
	defer pm.Close()
}

func TestP2ScreenAPI(t *testing.T) {
	src := `
assert(vistty.screen.count() == 3, "screen count should be 3")
assert(vistty.screen.focused_idx() == 1, "focused should be 1")
vistty.screen.next()
assert(vistty.screen.focused_idx() == 2, "after next focused should be 2")
vistty.screen.switch(3)
assert(vistty.screen.focused_idx() == 3, "after switch(3) focused should be 3")
vistty.screen.prev()
assert(vistty.screen.focused_idx() == 2, "after prev focused should be 2")
`
	pm := newPMWithCtx(t, src, &fakeCtx{})
	if pm == nil {
		return
	}
	defer pm.Close()
}

func TestP2ZoomAPI(t *testing.T) {
	src := `
vistty.zoom.increase()
vistty.zoom.decrease()
vistty.zoom.reset()
`
	fc := &fakeCtx{}
	pm := newPMWithCtx(t, src, fc)
	if pm == nil {
		return
	}
	defer pm.Close()
	if len(fc.zoomCalls) != 3 || fc.zoomCalls[0] != 1 || fc.zoomCalls[1] != -1 || fc.zoomCalls[2] != 0 {
		t.Fatalf("zoom calls mismatch: %v", fc.zoomCalls)
	}
}

func TestP2TermNoCtx(t *testing.T) {
	// 未 Activate（ctx=nil）时 term API 应安全返回零值
	src := `
assert(vistty.term.cols() == 0, "cols should be 0 when no ctx")
assert(vistty.term.rows() == 0, "rows should be 0 when no ctx")
assert(vistty.term.title() == "", "title should be empty when no ctx")
vistty.term.send("test")  -- should be no-op
vistty.term.send_key(vistty.keys.UP, 0)  -- should be no-op
vistty.term.scroll(5)  -- no-op
vistty.term.scroll_by(-2)  -- no-op
vistty.term.resize(80, 24)  -- no-op
local screen = vistty.term.read_screen()
assert(type(screen) == "table", "read_screen should return table")
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	// 不 Activate，直接执行 Lua 手动调用 API
	// 注意：Load 已执行 init.lua 顶层代码，上面的 assert 已运行
	pm.Close()
}

func TestP2TermAPINamespaces(t *testing.T) {
	// 验证所有命名空间存在
	src := `
assert(type(vistty.term) == "table", "vistty.term should be table")
assert(type(vistty.term.send) == "function", "vistty.term.send should be function")
assert(type(vistty.term.send_key) == "function")
assert(type(vistty.term.scroll) == "function")
assert(type(vistty.term.scroll_by) == "function")
assert(type(vistty.term.cols) == "function")
assert(type(vistty.term.rows) == "function")
assert(type(vistty.term.title) == "function")
assert(type(vistty.term.resize) == "function")
assert(type(vistty.term.read_screen) == "function")

assert(type(vistty.tab) == "table")
assert(type(vistty.tab.new) == "function")
assert(type(vistty.tab.close) == "function")
assert(type(vistty.tab.next) == "function")
assert(type(vistty.tab.prev) == "function")
assert(type(vistty.tab.count) == "function")
assert(type(vistty.tab.list) == "function")

assert(type(vistty.tab.switch) == "function")

assert(type(vistty.screen) == "table")
assert(type(vistty.screen.next) == "function")
assert(type(vistty.screen.prev) == "function")
assert(type(vistty.screen.switch) == "function")
assert(type(vistty.screen.count) == "function")
assert(type(vistty.screen.focused_idx) == "function")

assert(type(vistty.zoom) == "table")
	assert(type(vistty.zoom.increase) == "function")
	assert(type(vistty.zoom.decrease) == "function")
	assert(type(vistty.zoom.reset) == "function")
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Close()
}

func newPMWithCtx(t *testing.T, src string, ctx PluginContext) *PluginManager {
	f := writeTemp(t, `-- init stub`)
	pm := NewPluginManager(f)
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(ctx)
	if err := pm.L.DoString(src); err != nil {
		t.Fatal(err)
	}
	return pm
}
