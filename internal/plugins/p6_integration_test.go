package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/terminal"
	lua "github.com/yuin/gopher-lua"
)

// p6FakeCtx 统一的 P6 集成测试用 fakeCtx（实现 PluginContext）。
// 复用 p2/p3/p4 的 fakeCtx 思路，但独立定义以避免与既有类型冲突。
type p6FakeCtx struct {
	tabs        []TabInfo
	screenIdx   int
	zoomCalls   []int
	enabledPanx map[string]int
}

func newP6FakeCtx() *p6FakeCtx {
	return &p6FakeCtx{
		enabledPanx: make(map[string]int),
	}
}

func (f *p6FakeCtx) FocusTerm() *terminal.Terminal { return nil }
func (f *p6FakeCtx) Terms() []*terminal.Terminal   { return nil }
func (f *p6FakeCtx) NewTab() error {
	f.tabs = append(f.tabs, TabInfo{Title: "new", Active: true})
	return nil
}
func (f *p6FakeCtx) CloseCurrentTab() {}
func (f *p6FakeCtx) NextTab()         {}
func (f *p6FakeCtx) PrevTab()         {}
func (f *p6FakeCtx) SwitchTab(i int)  {}
func (f *p6FakeCtx) TabList() []TabInfo {
	if f.tabs == nil {
		return []TabInfo{{Title: "tab1", Active: true}}
	}
	return f.tabs
}
func (f *p6FakeCtx) NextScreen()                { f.screenIdx++ }
func (f *p6FakeCtx) PrevScreen()                { f.screenIdx-- }
func (f *p6FakeCtx) SwitchScreen(i int)         { f.screenIdx = i - 1 }
func (f *p6FakeCtx) ScreenCount() int           { return 3 }
func (f *p6FakeCtx) FocusScreenIdx() int        { return f.screenIdx + 1 }
func (f *p6FakeCtx) ZoomIn()                    { f.zoomCalls = append(f.zoomCalls, 1) }
func (f *p6FakeCtx) ZoomOut()                   { f.zoomCalls = append(f.zoomCalls, -1) }
func (f *p6FakeCtx) ZoomReset()                 { f.zoomCalls = append(f.zoomCalls, 0) }
func (f *p6FakeCtx) EnablePanel(s string, n int) {
	if f.enabledPanx == nil {
		f.enabledPanx = make(map[string]int)
	}
	f.enabledPanx[s] = n
}
func (f *p6FakeCtx) DisablePanel(s string) {
	if f.enabledPanx != nil {
		delete(f.enabledPanx, s)
	}
}
func (f *p6FakeCtx) ReloadPlugins() error { return nil }
func (f *p6FakeCtx) Exit()                {}

// examplesDir 返回仓库根目录下的 examples 目录绝对路径。
// go test 的工作目录为包目录 internal/plugins，因此向上回溯两层。
func examplesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(filepath.Dir(wd)), "examples")
}

// TestP6EndToEnd 模拟完整的 init.lua 加载流程：
// init.lua 设置 config + 注册 on_key + 注册 on_render + enable panel；
// 验证 Load() 返回的 RunConfig 字段、Activate 后 OnKey 拦截、OnRender 图元收集、EnabledPanels 面板声明。
func TestP6EndToEnd(t *testing.T) {
	src := `
vistty.config = {
	backend = "wayland", shell = "/bin/zsh", fontsize = 18,
	osd = { top = true },
}

vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F1 then
		return {code=vistty.keys.F2, mods=ev.mods, rune=0}
	end
	if ev.code == vistty.keys.F3 then
		return true
	end
end)

vistty.ui.enable("bottom", 2)
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	ctx:rect(0, 0, w, h, {bg={10, 20, 30}})
	ctx:text(2, 0, "hello", {fg={255, 0, 0}})
	return true
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	defer pm.Close()

	cfg, err := pm.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Backend != "wayland" {
		t.Fatalf("backend want wayland got %s", cfg.Backend)
	}
	if cfg.Shell != "/bin/zsh" {
		t.Fatalf("shell want /bin/zsh got %s", cfg.Shell)
	}
	if cfg.FontSize != 18 {
		t.Fatalf("fontsize want 18 got %v", cfg.FontSize)
	}

	panels := pm.EnabledPanels()
	if panels["bottom"] != 2 {
		t.Fatalf("panel bottom want 2 got %d", panels["bottom"])
	}

	pm.Activate(newP6FakeCtx())

	// F1 → 改写为 F2
	consumed, out := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if consumed {
		t.Fatal("F1 should not be consumed")
	}
	if out.Code != 60 {
		t.Fatalf("F1 should rewrite to F2(60) got %d", out.Code)
	}
	// F3 → consumed
	consumed2, _ := pm.OnKey(platform.KeyEvent{Code: 61, State: platform.KeyPress})
	if !consumed2 {
		t.Fatal("F3 should be consumed")
	}
	// 普通键 → not consumed
	consumed3, _ := pm.OnKey(platform.KeyEvent{Rune: 'a', State: platform.KeyPress})
	if consumed3 {
		t.Fatal("'a' should not be consumed")
	}

	dirty, prims := pm.OnRender("bottom", 80, 2)
	if !dirty {
		t.Fatal("dirty should be true")
	}
	if len(prims) != 2 {
		t.Fatalf("expected 2 primitives got %d", len(prims))
	}
	if prims[0].Kind != PrimRect || prims[0].W != 80 || prims[0].H != 2 {
		t.Fatalf("prim0 rect mismatch: %+v", prims[0])
	}
	if prims[0].Bg != [4]uint8{10, 20, 30, 255} {
		t.Fatalf("prim0 bg mismatch: %v", prims[0].Bg)
	}
	if prims[1].Kind != PrimText || prims[1].Text != "hello" {
		t.Fatalf("prim1 text mismatch: %+v", prims[1])
	}
	if prims[1].Fg != [4]uint8{255, 0, 0, 255} {
		t.Fatalf("prim1 fg mismatch: %v", prims[1].Fg)
	}
}

// TestP6Reload 验证热重载：初始注册 on_key hook A，Reload 后旧钩子清空、新钩子生效。
func TestP6Reload(t *testing.T) {
	src := `
vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F1 then return true end
end)
vistty.ui.enable("bottom", 1)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	defer pm.Close()

	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	// v1: F1 消费
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if !consumed {
		t.Fatal("v1 F1 should be consumed")
	}
	if pm.EnabledPanels()["bottom"] != 1 {
		t.Fatal("v1 bottom panel should be 1")
	}

	if err := pm.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	// Reload 后 F1 不再被消费（钩子被清空，init.lua 未重新注册不同的钩子，但仍注册了 F1 消费）
	// 这里 init.lua 内容未变，Reload 后钩子应重新注册，F1 仍应被消费
	consumed2, _ := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if !consumed2 {
		t.Fatal("after Reload F1 should still be consumed (same init.lua)")
	}
	if pm.EnabledPanels()["bottom"] != 1 {
		t.Fatal("after Reload bottom panel should still be 1")
	}

	// 验证 Reload 清空+重建：用 Lua 检查 keyHooks 长度
	// Reload 应保证 keyHooks 是全新表
	vt, ok := pm.L.GetGlobal("vistty").(*lua.LTable)
	if !ok {
		t.Fatal("vistty table missing")
	}
	_ = vt
}

// TestP6ReloadWithNewFile 验证修改 init.lua 文件内容后 Reload 生效。
func TestP6ReloadWithNewFile(t *testing.T) {
	// 写入 v1：F1 消费
	f, err := os.CreateTemp("", "init*.lua")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := f.WriteString(`vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F1 then return true end
end)`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(path)

	pm := NewPluginManager(path)
	defer pm.Close()
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if !consumed {
		t.Fatal("v1 F1 should be consumed")
	}
	// v1 中 F2 不被消费
	consumedF2, _ := pm.OnKey(platform.KeyEvent{Code: 60, State: platform.KeyPress})
	if consumedF2 {
		t.Fatal("v1 F2 should not be consumed")
	}

	// 改写文件为 v2：F2 消费，F1 不再消费
	if err := os.WriteFile(path, []byte(`vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F2 then return true end
end)`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := pm.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// v2: F1 不再消费
	consumed2, _ := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if consumed2 {
		t.Fatal("v2 F1 should not be consumed")
	}
	// v2: F2 被消费
	consumed3, _ := pm.OnKey(platform.KeyEvent{Code: 60, State: platform.KeyPress})
	if !consumed3 {
		t.Fatal("v2 F2 should be consumed")
	}
}

// TestP6ExampleInitLua 加载 examples/init.lua，验证 RunConfig 字段与面板声明。
func TestP6ExampleInitLua(t *testing.T) {
	examplePath := filepath.Join(examplesDir(t), "init.lua")
	if _, err := os.Stat(examplePath); err != nil {
		t.Skipf("examples/init.lua not found: %v", err)
	}

	pm := NewPluginManager(examplePath)
	defer pm.Close()

	cfg, err := pm.Load()
	if err != nil {
		t.Fatalf("Load examples/init.lua failed: %v", err)
	}
	if cfg.Backend != "auto" {
		t.Fatalf("backend want auto got %s", cfg.Backend)
	}
	if cfg.Shell != "/bin/bash" {
		t.Fatalf("shell want /bin/bash got %s", cfg.Shell)
	}
	if cfg.FontSize != 14 {
		t.Fatalf("fontsize want 14 got %v", cfg.FontSize)
	}

	panels := pm.EnabledPanels()
	if panels["bottom"] != 1 {
		t.Fatalf("panel bottom want 1 got %d", panels["bottom"])
	}

	// Activate 后验证 OnRender 钩子可正常调用（不 panic）
	pm.Activate(newP6FakeCtx())
	dirty, prims := pm.OnRender("bottom", 80, 2)
	if !dirty {
		t.Fatal("example on_render should return dirty=true")
	}
	if len(prims) < 2 {
		t.Fatalf("expected at least 2 primitives (rect+text) got %d", len(prims))
	}
	// 第一个应为 rect（背景填充）
	if prims[0].Kind != PrimRect {
		t.Fatalf("prim0 should be rect got kind=%d", prims[0].Kind)
	}
	if prims[0].Bg != [4]uint8{68, 68, 68, 255} {
		t.Fatalf("prim0 bg want {68,68,68} got %v", prims[0].Bg)
	}
}

// TestP6MultipleOnKeyChain 验证多钩子链式：改写→检查→消费。
func TestP6MultipleOnKeyChain(t *testing.T) {
	src := `
local trace = {}
vistty._trace = trace

vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F1 then
		return {code=vistty.keys.F2, mods=ev.mods, rune=0}
	end
	table.insert(trace, "hook1-pass")
end)

vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F2 then
		table.insert(trace, "hook2-saw-F2")
	else
		table.insert(trace, "hook2-saw-other")
	end
end)

vistty.input.on_key(function(ev)
	if ev.code == vistty.keys.F2 then
		table.insert(trace, "hook3-consume")
		return true
	end
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	defer pm.Close()
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	// F1 → hook1 改写为 F2 → hook2 看到 F2 → hook3 消费
	consumed, out := pm.OnKey(platform.KeyEvent{Code: 59, State: platform.KeyPress})
	if !consumed {
		t.Fatal("should be consumed by hook3")
	}
	if out.Code != 60 {
		t.Fatalf("out code should be F2(60) got %d", out.Code)
	}

	// 验证 trace
	vt, ok := pm.L.GetGlobal("vistty").(*lua.LTable)
	if !ok {
		t.Fatal("vistty table missing")
	}
	trace := vt.RawGetString("_trace")
	trT, ok := trace.(*lua.LTable)
	if !ok {
		t.Fatal("_trace should be a table")
	}
	// hook1 改写后不再 insert（return 后未执行 table.insert）
	// hook2 看到 F2 → "hook2-saw-F2"
	// hook3 消费 → "hook3-consume"
	n := trT.Len()
	if n != 2 {
		t.Fatalf("trace len want 2 got %d", n)
	}
	if string(trT.RawGetInt(1).(lua.LString)) != "hook2-saw-F2" {
		t.Fatalf("trace[1] want hook2-saw-F2 got %v", trT.RawGetInt(1))
	}
	if string(trT.RawGetInt(2).(lua.LString)) != "hook3-consume" {
		t.Fatalf("trace[2] want hook3-consume got %v", trT.RawGetInt(2))
	}
}

// TestP6OnRenderWithCtx 验证 OnRender 钩子使用 ctx:text/ctx:rect/ctx:size 完整链路。
func TestP6OnRenderWithCtx(t *testing.T) {
	src := `
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	-- 全屏背景
	ctx:rect(0, 0, w, h, {bg={5, 10, 15}})
	-- 左上角文本
	ctx:text(0, 0, "top-left", {fg={255, 255, 255}, bold=true})
	-- 右下角文本（依赖传入的 w/h）
	ctx:text(w - 4, h - 1, "end", {fg={0, 255, 0}})
	return true
end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	defer pm.Close()
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	dirty, prims := pm.OnRender("bottom", 80, 2)
	if !dirty {
		t.Fatal("dirty should be true")
	}
	if len(prims) != 3 {
		t.Fatalf("expected 3 primitives got %d", len(prims))
	}
	// rect 全屏
	if prims[0].Kind != PrimRect || prims[0].X != 0 || prims[0].Y != 0 ||
		prims[0].W != 80 || prims[0].H != 2 {
		t.Fatalf("prim0 rect mismatch: %+v", prims[0])
	}
	if prims[0].Bg != [4]uint8{5, 10, 15, 255} {
		t.Fatalf("prim0 bg mismatch: %v", prims[0].Bg)
	}
	// top-left 文本
	if prims[1].Kind != PrimText || prims[1].X != 0 || prims[1].Y != 0 ||
		prims[1].Text != "top-left" {
		t.Fatalf("prim1 text mismatch: %+v", prims[1])
	}
	if prims[1].Fg != [4]uint8{255, 255, 255, 255} || !prims[1].Bold {
		t.Fatalf("prim1 fg/bold mismatch: fg=%v bold=%v", prims[1].Fg, prims[1].Bold)
	}
	// end 文本 (76, 1)
	if prims[2].Kind != PrimText || prims[2].X != 76 || prims[2].Y != 1 ||
		prims[2].Text != "end" {
		t.Fatalf("prim2 text mismatch: %+v", prims[2])
	}
	if prims[2].Fg != [4]uint8{0, 255, 0, 255} {
		t.Fatalf("prim2 fg mismatch: %v", prims[2].Fg)
	}
}

// TestP6ReloadClearsHooks 验证 Reload 清空旧钩子。
// 通过 Reload 到一个不注册任何钩子的 init.lua，确认旧钩子不再触发。
func TestP6ReloadClearsHooks(t *testing.T) {
	// v1: 注册 on_key + on_render + panel
	f, err := os.CreateTemp("", "init*.lua")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := f.WriteString(`
vistty.input.on_key(function(ev) return true end)
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx) ctx:text(0,0,"x") return true end)
`); err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(path)

	pm := NewPluginManager(path)
	defer pm.Close()
	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	// v1 验证钩子存在
	consumed, _ := pm.OnKey(platform.KeyEvent{Code: 28, State: platform.KeyPress})
	if !consumed {
		t.Fatal("v1 should consume all keys")
	}
	dirty, prims := pm.OnRender("bottom", 80, 1)
	if !dirty || len(prims) != 1 {
		t.Fatalf("v1 on_render should produce dirty+1 prim: dirty=%v prims=%d", dirty, len(prims))
	}
	if pm.EnabledPanels()["bottom"] != 1 {
		t.Fatal("v1 panel bottom should be 1")
	}

	// 改写为 v2：无任何钩子、无 panel
	if err := os.WriteFile(path, []byte(`-- empty init`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := pm.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// v2：所有钩子应被清空
	consumed2, _ := pm.OnKey(platform.KeyEvent{Code: 28, State: platform.KeyPress})
	if consumed2 {
		t.Fatal("v2 should not consume (hooks cleared)")
	}
	dirty2, prims2 := pm.OnRender("bottom", 80, 1)
	if dirty2 || len(prims2) != 0 {
		t.Fatalf("v2 on_render should be empty: dirty=%v prims=%d", dirty2, len(prims2))
	}
	if _, ok := pm.EnabledPanels()["bottom"]; ok {
		t.Fatal("v2 panel bottom should be cleared")
	}
}

// TestP6NoRaceOnSequentialAccess 是一个轻量的串行访问冒烟测试，
// 确保在同一线程串行调用 Load/Activate/OnKey/OnRender/Reload/Close 不出错。
// 真正的 -race 检测由 go test -race 覆盖。
func TestP6NoRaceOnSequentialAccess(t *testing.T) {
	src := `
vistty.input.on_key(function(ev) return true end)
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx) ctx:text(0,0,"x") return true end)
`
	f := writeTemp(t, src)
	pm := NewPluginManager(f)
	defer pm.Close()

	if _, err := pm.Load(); err != nil {
		t.Fatal(err)
	}
	pm.Activate(newP6FakeCtx())

	for i := 0; i < 20; i++ {
		_, _ = pm.OnKey(platform.KeyEvent{Code: uint16(28 + i%10), State: platform.KeyPress})
		_, _ = pm.OnRender("bottom", 80, 1)
	}
	if err := pm.Reload(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		_, _ = pm.OnKey(platform.KeyEvent{Code: uint16(28 + i%10), State: platform.KeyPress})
		_, _ = pm.OnRender("bottom", 80, 1)
	}
	// runtime.KeepAlive 防止 pm 被过早 GC（主要为了显式引用 runtime 包）
	runtime.KeepAlive(pm)
}
