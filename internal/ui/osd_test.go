package ui

import (
	"testing"

	"github.com/LaoQi/vistty/internal/font"
)

type fakeFace struct {
	m font.Metrics
}

func (f *fakeFace) Metrics() font.Metrics             { return f.m }
func (f *fakeFace) Glyph(r rune) (*font.Glyph, error) { return nil, nil }
func (f *fakeFace) Close() error                      { return nil }

func TestInsets(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}

	o := NewOSD(face)
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
	cells := layoutTabs(tabs, 0, 10, 100)
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
	if cells[0].bgR != activeBg[0] {
		t.Errorf("active tab pad bgR: expected %d, got %d", activeBg[0], cells[0].bgR)
	}
	if cells[1].bgR != activeBg[0] {
		t.Errorf("active tab char bgR: expected %d, got %d", activeBg[0], cells[1].bgR)
	}
	if cells[7].r != 0 || cells[7].bgR != barBg[0] {
		t.Errorf("bar fill cell 7: expected r=0 bgR=%d, got r=%q bgR=%d", barBg[0], cells[7].r, cells[7].bgR)
	}
}

func TestLayoutTabsTruncate(t *testing.T) {
	tabs := []Tab{{Title: "abc"}}
	cells := layoutTabs(tabs, 0, 10, 25)
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].x != 0 || cells[0].r != 0 {
		t.Errorf("cell 0: expected x=0 r=0, got x=%d r=%q", cells[0].x, cells[0].r)
	}
	if cells[1].x != 10 || cells[1].r != 'a' {
		t.Errorf("cell 1: expected x=10 r='a', got x=%d r=%q", cells[1].x, cells[1].r)
	}
}

func TestInsetsMergePanelLines(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}

	// 仅插件面板，无 panelLines
	o := NewOSD(face)
	o.SetPanelLines(map[string]int{"bottom": 2, "left": 3, "right": 1})
	top, bottom, left, right := o.Insets()
	if top != 20 || bottom != 40 || left != 30 || right != 10 {
		t.Fatalf("plugin insets: expected 20,40,30,10 got %d,%d,%d,%d", top, bottom, left, right)
	}

	// top 边：默认 1 行 + pluginLines 取 max
	o2 := NewOSD(face)
	o2.SetPanelLines(map[string]int{"top": 3, "bottom": 1})
	top, bottom, left, right = o2.Insets()
	if top != 60 {
		t.Fatalf("top should be max(default=20, plugin=60)=60, got %d", top)
	}
	if bottom != 20 {
		t.Fatalf("bottom should be plugin=20, got %d", bottom)
	}

	// panelLines<=0 不影响
	o3 := NewOSD(face)
	o3.SetPanelLines(map[string]int{"bottom": 0})
	_, bottom, _, _ = o3.Insets()
	if bottom != 0 {
		t.Fatalf("bottom with plugin=0 should be 0, got %d", bottom)
	}
}

func TestSetPluginPanel(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}
	o := NewOSD(face)
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
