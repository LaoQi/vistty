package render

import (
	"testing"

	"github.com/LaoQi/vistty/font"
)

func TestDrawCursorBlock(t *testing.T) {
	data := make([]byte, 100*40*4)
	stride := 100 * 4
	bitmap := make([]byte, 8*16)
	for i := range bitmap {
		bitmap[i] = 255
	}
	glyph := &font.Glyph{Bitmap: bitmap, Width: 8, Height: 16}
	drawCursor(data, stride, 10, 10, 8, 16, cursorBlock, 255, 255, 255, 'A', glyph, 12)
}

func TestDrawCursorUnderline(t *testing.T) {
	data := make([]byte, 100*40*4)
	stride := 100 * 4
	drawCursor(data, stride, 10, 10, 8, 16, cursorUnderline, 255, 255, 255, 0, nil, 0)
}

func TestDrawCursorBar(t *testing.T) {
	data := make([]byte, 100*40*4)
	stride := 100 * 4
	drawCursor(data, stride, 10, 10, 8, 16, cursorBar, 255, 255, 255, 0, nil, 0)
}

func TestDrawCursorBlockNoGlyph(t *testing.T) {
	data := make([]byte, 100*40*4)
	stride := 100 * 4
	drawCursor(data, stride, 10, 10, 8, 16, cursorBlock, 255, 255, 255, 0, nil, 0)
}
