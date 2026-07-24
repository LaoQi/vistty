package ui

import (
	"testing"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/panel"
)

func TestStatusBarInsets(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}
	sb := NewStatusBar(face, StatusBarTheme{})
	sb.SetLines(2)
	top, bottom, left, right := sb.Insets()
	if top != 0 || bottom != 40 || left != 0 || right != 0 {
		t.Fatalf("expected 0,40,0,0 got %d,%d,%d,%d", top, bottom, left, right)
	}
}

func TestStatusBarSetPrimitives(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 10, Height: 20, Ascent: 16}}
	sb := NewStatusBar(face, StatusBarTheme{})
	sb.SetLines(1)
	sb.SetPrimitives([]panel.Primitive{
		{Kind: panel.PrimRect, X: 0, Y: 0, W: 5, H: 1, Bg: [4]uint8{1, 2, 3, 255}},
	})
	if len(sb.primitives) != 1 {
		t.Fatal("SetPrimitives did not store primitive")
	}
	sb.SetPrimitives(nil)
	if len(sb.primitives) != 0 {
		t.Fatal("SetPrimitives nil should clear")
	}
}
