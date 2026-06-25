package render

import (
	"testing"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
)

type testSurface struct {
	data   []byte
	stride int
	width  int
	height int
}

func (s *testSurface) Size() (int, int)                          { return s.width, s.height }
func (s *testSurface) Data() []byte                               { return s.data }
func (s *testSurface) Stride() int                                { return s.stride }
func (s *testSurface) Swap() error                                { return nil }
func (s *testSurface) Close() error                               { return nil }
func (s *testSurface) ResizeEvents() <-chan platform.ResizeEvent  { return nil }
func (s *testSurface) OutputID() uint32                           { return 0 }

type testFace struct{}

func (f *testFace) Glyph(r rune) (*font.Glyph, error) {
	return &font.Glyph{
		Rune:    r,
		Bitmap:  make([]byte, 8*16),
		Width:   8,
		Height:  16,
		XOffset: 0,
		YOffset: 0,
		Advance: 8,
	}, nil
}
func (f *testFace) Metrics() font.Metrics {
	return font.Metrics{Width: 8, Height: 16, Ascent: 12, Descent: 4}
}
func (f *testFace) Close() error { return nil }

func TestCompositorRenderNoDirty(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 800*600*4),
		stride: 800 * 4,
		width:  800,
		height: 600,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	err := c.Render(buf, 0)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestCompositorRenderWithScrollOffset(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 800*600*4),
		stride: 800 * 4,
		width:  800,
		height: 600,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	err := c.Render(buf, 0)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}
