package render

import (
	"testing"
)

func TestDrawCursorBlock(t *testing.T) {
	data := make([]byte, 100*40*4)
	stride := 100 * 4
	glyph := make([]byte, 8*16)
	for i := range glyph {
		glyph[i] = 255
	}
	drawCursor(data, stride, 10, 10, 8, 16, cursorBlock, 255, 255, 255, 'A', glyph, 8)
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
