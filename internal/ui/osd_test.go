package ui

import (
	"testing"

	"github.com/LaoQi/vistty/font"
)

type fakeFace struct {
	m font.Metrics
}

func (f *fakeFace) Metrics() font.Metrics             { return f.m }
func (f *fakeFace) Glyph(r rune) (*font.Glyph, error) { return nil, nil }
func (f *fakeFace) Close() error                      { return nil }

func TestInsets(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}

	o := NewOSD(face, OSDTheme{})
	top, bottom, left, right := o.Insets()
	if top != 20 || bottom != 0 || left != 0 || right != 0 {
		t.Fatalf("default top: expected 20,0,0,0 got %d,%d,%d,%d", top, bottom, left, right)
	}
}

func TestLayoutTabs(t *testing.T) {
	tabs := []Tab{
		{Title: "a", Active: true},
		{Title: "bb", Active: false},
	}
	o := NewOSD(&fakeFace{}, OSDTheme{})
	cells, sc := o.layoutTabs(tabs, 0, 10, 100, 0, 0)
	if sc != 0 {
		t.Fatalf("scroll: expected 0, got %d", sc)
	}
	if len(cells) != 10 {
		t.Fatalf("expected 10 cells, got %d", len(cells))
	}
	if cells[0].x != 0 || cells[0].r != 0 {
		t.Errorf("cell 0: expected x=0 r=0, got x=%d r=%q", cells[0].x, cells[0].r)
	}
	if cells[1].x != 10 || cells[1].r != 'a' {
		t.Errorf("cell 1: expected x=10 r='a', got x=%d r=%q", cells[1].x, cells[1].r)
	}
	if cells[2].x != 20 || cells[2].r != 0 {
		t.Errorf("cell 2: expected x=20 r=0, got x=%d r=%q", cells[2].x, cells[2].r)
	}
	if cells[3].x != 30 || cells[3].r != 0 {
		t.Errorf("cell 3: expected x=30 r=0, got x=%d r=%q", cells[3].x, cells[3].r)
	}
	if cells[4].x != 40 || cells[4].r != 'b' {
		t.Errorf("cell 4: expected x=40 r='b', got x=%d r=%q", cells[4].x, cells[4].r)
	}
	if cells[5].x != 50 || cells[5].r != 'b' {
		t.Errorf("cell 5: expected x=50 r='b', got x=%d r=%q", cells[5].x, cells[5].r)
	}
	if cells[6].x != 60 || cells[6].r != 0 {
		t.Errorf("cell 6: expected x=60 r=0, got x=%d r=%q", cells[6].x, cells[6].r)
	}
	if cells[0].bgR != DefaultOSDTheme.ActiveBg[0] {
		t.Errorf("active tab pad bgR: expected %d, got %d", DefaultOSDTheme.ActiveBg[0], cells[0].bgR)
	}
	if cells[1].bgR != DefaultOSDTheme.ActiveBg[0] {
		t.Errorf("active tab char bgR: expected %d, got %d", DefaultOSDTheme.ActiveBg[0], cells[1].bgR)
	}
	if cells[7].r != 0 || cells[7].bgR != DefaultOSDTheme.BarBg[0] {
		t.Errorf("bar fill cell 7: expected r=0 bgR=%d, got r=%q bgR=%d", DefaultOSDTheme.BarBg[0], cells[7].r, cells[7].bgR)
	}
}

func TestLayoutTabsTruncate(t *testing.T) {
	// tabWidth=25 放不下完整 tab(50px)：窗口内部分显示 pad+a+b（b 右半被 clip）
	tabs := []Tab{{Title: "abc"}}
	o := NewOSD(&fakeFace{}, OSDTheme{})
	cells, sc := o.layoutTabs(tabs, 0, 10, 25, 0, 0)
	if sc != 0 {
		t.Fatalf("scroll: expected 0, got %d", sc)
	}
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells (partial), got %d", len(cells))
	}
	if cells[0].x != 0 || cells[0].r != 0 {
		t.Errorf("cell 0: expected x=0 r=0, got x=%d r=%q", cells[0].x, cells[0].r)
	}
	if cells[1].x != 10 || cells[1].r != 'a' {
		t.Errorf("cell 1: expected x=10 r='a', got x=%d r=%q", cells[1].x, cells[1].r)
	}
	if cells[2].x != 20 || cells[2].r != 'b' {
		t.Errorf("cell 2: expected x=20 r='b', got x=%d r=%q", cells[2].x, cells[2].r)
	}
}

func TestLayoutTabsCJK(t *testing.T) {
	// 中文标题双宽字符：应步进 2*cellW，w=2
	tabs := []Tab{
		{Title: "终端", Active: true},
	}
	o := NewOSD(&fakeFace{}, OSDTheme{})
	cells, _ := o.layoutTabs(tabs, 0, 10, 100, 0, 0)
	// 布局：pad(0) + 终(10,w2) + 端(30,w2) + pad(50) + barfill(60,70,80,90) = 8 cells
	if len(cells) != 8 {
		t.Fatalf("expected 8 cells, got %d", len(cells))
	}
	if cells[0].x != 0 || cells[0].r != 0 || cells[0].w != 1 {
		t.Errorf("pad0: expected x=0 w=1 r=0, got x=%d w=%d r=%q", cells[0].x, cells[0].w, cells[0].r)
	}
	if cells[1].x != 10 || cells[1].r != '终' || cells[1].w != 2 {
		t.Errorf("终: expected x=10 w=2, got x=%d w=%d", cells[1].x, cells[1].w)
	}
	if cells[2].x != 30 || cells[2].r != '端' || cells[2].w != 2 {
		t.Errorf("端: expected x=30 w=2, got x=%d w=%d", cells[2].x, cells[2].w)
	}
	if cells[3].x != 50 || cells[3].w != 1 {
		t.Errorf("pad1: expected x=50 w=1, got x=%d w=%d", cells[3].x, cells[3].w)
	}
	if cells[4].x != 60 || cells[4].bgR != DefaultOSDTheme.BarBg[0] {
		t.Errorf("barfill: expected x=60 bgR=%d, got x=%d bgR=%d", DefaultOSDTheme.BarBg[0], cells[4].x, cells[4].bgR)
	}
}

func TestLayoutTabsCJKTruncate(t *testing.T) {
	// tabWidth=25：pad(10) + 终(20，右半被 clip)，宽字符部分显示
	tabs := []Tab{{Title: "终"}}
	o := NewOSD(&fakeFace{}, OSDTheme{})
	cells, sc := o.layoutTabs(tabs, 0, 10, 25, 0, 0)
	if sc != 0 {
		t.Fatalf("scroll: expected 0, got %d", sc)
	}
	// pad(0) + 终(10,w2) = 2 cells，宽字符部分可见
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells (wide char partial), got %d", len(cells))
	}
	if cells[0].x != 0 || cells[0].w != 1 || cells[0].r != 0 {
		t.Errorf("pad: expected x=0 w=1 r=0, got x=%d w=%d r=%q", cells[0].x, cells[0].w, cells[0].r)
	}
	if cells[1].x != 10 || cells[1].r != '终' || cells[1].w != 2 {
		t.Errorf("终: expected x=10 w=2, got x=%d w=%d r=%q", cells[1].x, cells[1].w, cells[1].r)
	}
}

func TestTruncateTabTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc", "abc"},
		{"终端", "终端"},                              // 4 列 <= 16
		{"0123456789abcdef", "0123456789abcdef"},  // 16 列正好
		{"0123456789abcdefg", "0123456789abcde…"}, // 17 列 → 15+…
		{"一二三四五六七八九", "一二三四五六七…"},                 // 18 列 → 7 双宽(14)+…
		{"", ""},
	}
	for _, c := range cases {
		got := truncateTabTitle(c.in)
		if got != c.want {
			t.Errorf("truncateTabTitle(%q): expected %q, got %q", c.in, c.want, got)
		}
	}
}

func TestLayoutTabsScroll(t *testing.T) {
	// 4 个 tab 各 6 列(60px)，总 240px；tabWidth=100 仅容 1.5 个 tab。
	// active=3 (tab4, 起始 180, 结束 240) 在窗口外，应滚动使其可见。
	tabs := []Tab{
		{Title: "tab1"},
		{Title: "tab2"},
		{Title: "tab3"},
		{Title: "tab4"},
	}
	o := NewOSD(&fakeFace{}, OSDTheme{})
	cells, sc := o.layoutTabs(tabs, 3, 10, 100, 0, 0)
	// target=240-100=140，最大 tabStart<=140 为 120（tab3 起点）
	if sc != 120 {
		t.Fatalf("scroll: expected 120, got %d", sc)
	}
	// active tab4 首字符 't' 应在窗口内：rx = 180-120 = 60，pad 后 't' 在 x=70
	found := false
	for _, c := range cells {
		if c.x == 70 && c.r == 't' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("active tab4 content not visible: want cell x=70 r='t'")
	}
}

func TestLayoutTabsScrollKeepWhenVisible(t *testing.T) {
	// active 已可见时保持 scroll 不变（不抖动）
	tabs := []Tab{{Title: "tab1"}, {Title: "tab2"}, {Title: "tab3"}, {Title: "tab4"}}
	// 手动设 scroll=60（tab2 起点），active=2 (tab3, 120-180) 在窗口[60,160]内
	o := NewOSD(&fakeFace{}, OSDTheme{})
	_, sc := o.layoutTabs(tabs, 2, 10, 100, 0, 60)
	if sc != 60 {
		t.Fatalf("scroll should keep 60 when active visible, got %d", sc)
	}
}

func TestInsetsMergePanelLines(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}

	// 仅插件面板，无 panelLines
	o := NewOSD(face, OSDTheme{})
	o.SetPanelLines(map[string]int{"bottom": 2, "left": 3, "right": 1})
	top, bottom, left, right := o.Insets()
	if top != 20 || bottom != 40 || left != 30 || right != 10 {
		t.Fatalf("plugin insets: expected 20,40,30,10 got %d,%d,%d,%d", top, bottom, left, right)
	}

	// top 边：默认 1 行 + pluginLines 取 max
	o2 := NewOSD(face, OSDTheme{})
	o2.SetPanelLines(map[string]int{"top": 3, "bottom": 1})
	top, bottom, left, right = o2.Insets()
	if top != 60 {
		t.Fatalf("top should be max(default=20, plugin=60)=60, got %d", top)
	}
	if bottom != 20 {
		t.Fatalf("bottom should be plugin=20, got %d", bottom)
	}

	// panelLines<=0 不影响
	o3 := NewOSD(face, OSDTheme{})
	o3.SetPanelLines(map[string]int{"bottom": 0})
	_, bottom, _, _ = o3.Insets()
	if bottom != 0 {
		t.Fatalf("bottom with plugin=0 should be 0, got %d", bottom)
	}
}

func TestSetPluginPanel(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}
	o := NewOSD(face, OSDTheme{})
	o.SetPluginPanel("bottom", []PanelPrimitive{
		{Kind: primRect, X: 0, Y: 0, W: 5, H: 1, Bg: [4]uint8{1, 2, 3, 255}},
	})
	if len(o.pluginPanels["bottom"]) != 1 {
		t.Fatal("SetPluginPanel did not store primitive")
	}
	o.SetPluginPanel("bottom", nil)
	if len(o.pluginPanels["bottom"]) != 0 {
		t.Fatal("SetPluginPanel nil should clear")
	}
}
