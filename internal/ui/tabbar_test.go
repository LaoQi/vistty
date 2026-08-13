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

	tb := NewTabBar(face, TabBarTheme{})
	top, bottom, left, right := tb.Insets()
	if top != 20 || bottom != 0 || left != 0 || right != 0 {
		t.Fatalf("default top: expected 20,0,0,0 got %d,%d,%d,%d", top, bottom, left, right)
	}
}

func TestLayoutTabs(t *testing.T) {
	tabs := []Tab{
		{Title: "a", Active: true},
		{Title: "bb", Active: false},
	}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	cells, sc := tb.layoutTabs(tabs, 0, 10, 100, 0, 0)
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
	if cells[0].bgR != DefaultTabBarTheme.ActiveBg[0] {
		t.Errorf("active tab pad bgR: expected %d, got %d", DefaultTabBarTheme.ActiveBg[0], cells[0].bgR)
	}
	if cells[1].bgR != DefaultTabBarTheme.ActiveBg[0] {
		t.Errorf("active tab char bgR: expected %d, got %d", DefaultTabBarTheme.ActiveBg[0], cells[1].bgR)
	}
	if cells[7].r != 0 || cells[7].bgR != DefaultTabBarTheme.BarBg[0] {
		t.Errorf("bar fill cell 7: expected r=0 bgR=%d, got r=%q bgR=%d", DefaultTabBarTheme.BarBg[0], cells[7].r, cells[7].bgR)
	}
}

func TestLayoutTabsTruncate(t *testing.T) {
	tabs := []Tab{{Title: "abc"}}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	cells, sc := tb.layoutTabs(tabs, 0, 10, 25, 0, 0)
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
	tabs := []Tab{
		{Title: "终端", Active: true},
	}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	cells, _ := tb.layoutTabs(tabs, 0, 10, 100, 0, 0)
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
	if cells[4].x != 60 || cells[4].bgR != DefaultTabBarTheme.BarBg[0] {
		t.Errorf("barfill: expected x=60 bgR=%d, got x=%d bgR=%d", DefaultTabBarTheme.BarBg[0], cells[4].x, cells[4].bgR)
	}
}

func TestLayoutTabsCJKTruncate(t *testing.T) {
	tabs := []Tab{{Title: "终"}}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	cells, sc := tb.layoutTabs(tabs, 0, 10, 25, 0, 0)
	if sc != 0 {
		t.Fatalf("scroll: expected 0, got %d", sc)
	}
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
		{"终端", "终端"},
		{"0123456789abcdef", "0123456789abcdef"},
		{"0123456789abcdefg", "0123456789abcde…"},
		{"一二三四五六七八九", "一二三四五六七…"},
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
	tabs := []Tab{
		{Title: "tab1"},
		{Title: "tab2"},
		{Title: "tab3"},
		{Title: "tab4"},
	}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	cells, sc := tb.layoutTabs(tabs, 3, 10, 100, 0, 0)
	if sc != 120 {
		t.Fatalf("scroll: expected 120, got %d", sc)
	}
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
	tabs := []Tab{{Title: "tab1"}, {Title: "tab2"}, {Title: "tab3"}, {Title: "tab4"}}
	tb := NewTabBar(&fakeFace{}, TabBarTheme{})
	_, sc := tb.layoutTabs(tabs, 2, 10, 100, 0, 60)
	if sc != 60 {
		t.Fatalf("scroll should keep 60 when active visible, got %d", sc)
	}
}

func TestCsdButtonsWidth(t *testing.T) {
	tb := NewTabBar(&fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}, TabBarTheme{})
	if w := tb.csdButtonsWidth(); w != 0 {
		t.Fatalf("non-CSD mode: width want 0 got %d", w)
	}
	tb.SetCSDMode(true)
	if w := tb.csdButtonsWidth(); w != 3*csdBtnCellSpan*10 {
		t.Fatalf("CSD width want %d got %d", 3*csdBtnCellSpan*10, w)
	}
}

func TestLayoutCsdButtons(t *testing.T) {
	tb := NewTabBar(&fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}, TabBarTheme{})
	cells := tb.layoutCsdButtons(10, 200)
	if len(cells) != csdBtnCount {
		t.Fatalf("want %d buttons got %d", csdBtnCount, len(cells))
	}
	want := []struct {
		x    int
		rune rune
	}{
		{200 - 3*csdBtnCellSpan*10, font.CsdBtnMinRune},
		{200 - 2*csdBtnCellSpan*10, font.CsdBtnMaxRune},
		{200 - 1*csdBtnCellSpan*10, font.CsdBtnCloseRune},
	}
	for i, w := range want {
		c := cells[i]
		if c.x != w.x {
			t.Errorf("btn %d: x want %d got %d", i, w.x, c.x)
		}
		if c.w != csdBtnCellSpan {
			t.Errorf("btn %d: w want %d got %d", i, csdBtnCellSpan, c.w)
		}
		if c.r != w.rune {
			t.Errorf("btn %d: rune want %U got %U", i, w.rune, c.r)
		}
	}
}

func TestCsdButtonRectsAndHit(t *testing.T) {
	tb := NewTabBar(&fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}, TabBarTheme{})
	tb.SetCSDMode(true)
	btnW := csdBtnCellSpan * 10
	width := 200
	rects := tb.CsdButtonRects(width)
	wantX := []int{200 - 3*btnW, 200 - 2*btnW, 200 - 1*btnW}
	for i, x := range wantX {
		if rects[i].Min.X != x || rects[i].Dx() != btnW || rects[i].Dy() != 20 {
			t.Errorf("rect %d: want x=%d w=%d h=20 got %+v", i, x, btnW, rects[i])
		}
	}
	if hit := tb.HitTestTabBar(wantX[0]+btnW/2, 10, width); hit != TabBarCsdMin {
		t.Errorf("min hit want TabBarCsdMin got %v", hit)
	}
	if hit := tb.HitTestTabBar(wantX[1]+btnW/2, 10, width); hit != TabBarCsdMax {
		t.Errorf("max hit want TabBarCsdMax got %v", hit)
	}
	if hit := tb.HitTestTabBar(wantX[2]+btnW/2, 10, width); hit != TabBarCsdClose {
		t.Errorf("close hit want TabBarCsdClose got %v", hit)
	}
	if hit := tb.HitTestTabBar(50, 10, width); hit != TabBarArea {
		t.Errorf("plain area want TabBarArea got %v", hit)
	}
}
