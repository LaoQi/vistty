package render

import "github.com/LaoQi/vistty/font"

type cursorStyle int

const (
	cursorBlock cursorStyle = iota
	cursorUnderline
	cursorBar
)

func drawCursor(data []byte, stride int, x, y int, cellW, cellH int, style cursorStyle, r, g, b uint8, cellRune rune, glyph *font.Glyph, ascent int) {
	switch style {
	case cursorBlock:
		FillRect(data, stride, x, y, cellW, cellH, r, g, b)
		if cellRune != 0 && glyph != nil && glyph.Width > 0 && len(glyph.Bitmap) > 0 {
			gy := y + ascent + glyph.YOffset
			BlendGlyph(data, stride, x+glyph.XOffset, gy, glyph.Bitmap, glyph.Width, glyph.Height, 255-r, 255-g, 255-b)
		}
	case cursorUnderline:
		FillRect(data, stride, x, y+cellH-2, cellW, 2, r, g, b)
	case cursorBar:
		FillRect(data, stride, x, y, 2, cellH, r, g, b)
	}
}
