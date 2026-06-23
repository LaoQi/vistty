package font

import (
	"os"
	"testing"
)

func findTestFont() string {
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func TestOpenTypeFaceGlyph(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("no test font found")
	}
	face, err := NewOpenTypeFaceFromFile(fontPath, 14, 72)
	if err != nil {
		t.Fatalf("failed to load font: %v", err)
	}
	defer face.Close()

	glyph, err := face.Glyph('A')
	if err != nil {
		t.Fatalf("Glyph returned error: %v", err)
	}
	if glyph == nil {
		t.Fatal("expected non-nil glyph")
	}
	if len(glyph.Bitmap) == 0 {
		t.Error("expected non-empty glyph bitmap")
	}
	if glyph.Width <= 0 || glyph.Height <= 0 {
		t.Errorf("expected positive dimensions, got %dx%d", glyph.Width, glyph.Height)
	}
}

func TestOpenTypeFaceMetrics(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("no test font found")
	}
	face, err := NewOpenTypeFaceFromFile(fontPath, 14, 72)
	if err != nil {
		t.Fatalf("failed to load font: %v", err)
	}
	defer face.Close()

	m := face.Metrics()
	if m.Width <= 0 {
		t.Errorf("expected positive Width, got %d", m.Width)
	}
	if m.Height <= 0 {
		t.Errorf("expected positive Height, got %d", m.Height)
	}
	if m.Ascent <= 0 {
		t.Errorf("expected positive Ascent, got %d", m.Ascent)
	}
}

func TestOpenTypeFaceFileNotFound(t *testing.T) {
	_, err := NewOpenTypeFaceFromFile("/nonexistent/font.ttf", 14, 72)
	if err == nil {
		t.Error("expected error for nonexistent font")
	}
}

func TestOpenTypeFaceSpaceGlyph(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("no test font found")
	}
	face, err := NewOpenTypeFaceFromFile(fontPath, 14, 72)
	if err != nil {
		t.Fatalf("failed to load font: %v", err)
	}
	defer face.Close()

	glyph, err := face.Glyph(' ')
	if err != nil {
		t.Fatalf("Glyph returned error: %v", err)
	}
	if glyph == nil {
		t.Fatal("expected non-nil glyph for space")
	}
	if glyph.Advance <= 0 {
		t.Error("expected positive advance for space")
	}
}

func TestOpenTypeFaceClose(t *testing.T) {
	fontPath := findTestFont()
	if fontPath == "" {
		t.Skip("no test font found")
	}
	face, err := NewOpenTypeFaceFromFile(fontPath, 14, 72)
	if err != nil {
		t.Fatalf("failed to load font: %v", err)
	}
	if err := face.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
