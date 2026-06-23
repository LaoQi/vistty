package render

type cursorStyle int

const (
	cursorBlock cursorStyle = iota
	cursorUnderline
	cursorBar
)

func drawCursor(data []byte, stride int, x, y int, cellW, cellH int, style cursorStyle, r, g, b uint8) {
	switch style {
	case cursorBlock:
		fillRect(data, stride, x, y, cellW, cellH, r, g, b)
	case cursorUnderline:
		fillRect(data, stride, x, y+cellH-2, cellW, 2, r, g, b)
	case cursorBar:
		fillRect(data, stride, x, y, 2, cellH, r, g, b)
	}
}
