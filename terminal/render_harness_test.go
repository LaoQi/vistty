package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
)

func TestRenderHarnessFeedBytes(t *testing.T) {
	surf := newFakeSurface(800, 600)
	opts := DefaultOptions()
	opts.FontPath = ""
	opts.FontSize = 14
	term, err := NewRenderHarness(surf, opts)
	if err != nil {
		t.Fatalf("NewRenderHarness: %v", err)
	}
	defer term.Close()

	term.FeedBytes([]byte("Hello"))

	cell := term.screen.Cell(0, 0)
	if cell == nil {
		t.Fatal("cell (0,0) is nil")
	}
	if cell.Rune != 'H' {
		t.Errorf("cell (0,0) rune = %q, want 'H'", cell.Rune)
	}
}

func TestRenderHarnessRenderFrame(t *testing.T) {
	surf := newFakeSurface(800, 600)
	opts := DefaultOptions()
	opts.FontSize = 14
	term, err := NewRenderHarness(surf, opts)
	if err != nil {
		t.Fatalf("NewRenderHarness: %v", err)
	}
	defer term.Close()

	term.FeedBytes([]byte("AB"))
	if err := term.RenderFrame(); err != nil {
		t.Fatalf("RenderFrame: %v", err)
	}

	w, h := surf.Size()
	if w <= 0 || h <= 0 {
		t.Errorf("surface size = %dx%d", w, h)
	}
	data := surf.Data()
	if len(data) == 0 {
		t.Error("surface data is empty after RenderFrame")
	}
}

func TestRenderHarnessClose(t *testing.T) {
	surf := newFakeSurface(800, 600)
	opts := DefaultOptions()
	term, err := NewRenderHarness(surf, opts)
	if err != nil {
		t.Fatalf("NewRenderHarness: %v", err)
	}
	if err := term.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

var _ platform.Surface = (*fakeSurface)(nil)
