package render

type cursorStyle int

const (
	cursorBlock cursorStyle = iota
	cursorUnderline
	cursorBar
)

func drawCursor(data []byte, stride int, x, y int, cellW, cellH int, style cursorStyle, r, g, b uint8, cellRune rune, glyph []byte, glyphW int) {
	switch style {
	case cursorBlock:
		if cellRune != 0 && len(glyph) > 0 && glyphW > 0 {
			glyphH := len(glyph) / glyphW
			fillRect(data, stride, x, y, cellW, cellH, r, g, b)
			blendGlyph(data, stride, x, y, glyph, glyphW, glyphH, 255-r, 255-g, 255-b)
		} else {
			fillRect(data, stride, x, y, cellW, cellH, r, g, b)
		}
	case cursorUnderline:
		fillRect(data, stride, x, y+cellH-2, cellW, 2, r, g, b)
	case cursorBar:
		fillRect(data, stride, x, y, 2, cellH, r, g, b)
	}
}
