package render

import (
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
)

type defaultColor struct {
	bg screen.Color
	fg screen.Color
}

type Compositor struct {
	surface    platform.Surface
	face       font.Face
	atlas      *font.Atlas
	metrics    font.Metrics
	cols       int
	rows       int
	defColor   defaultColor
	backBuf    []byte
	backStride int
	backWidth  int
	backHeight int
	prevCursor cursorState
}

type cursorState struct {
	row     int
	col     int
	visible bool
}

func NewCompositor(surface platform.Surface, face font.Face) *Compositor {
	m := face.Metrics()
	w, h := surface.Size()
	cols := w / m.Width
	rows := 0
	if m.Height > 0 {
		rows = h / m.Height
	}
	stride := w * 4
	c := &Compositor{
		surface:    surface,
		face:       face,
		atlas:      font.NewAtlas(4096),
		metrics:    m,
		cols:       cols,
		rows:       rows,
		backBuf:    make([]byte, stride*h),
		backStride: stride,
		backWidth:  w,
		backHeight: h,
		defColor: defaultColor{
			bg: screen.Color{R: 0, G: 0, B: 0},
			fg: screen.Color{R: 204, G: 204, B: 204},
		},
	}
	bg := c.defColor.bg
	fillRect(c.backBuf, c.backStride, 0, 0, w, h, bg.R, bg.G, bg.B)
	return c
}

func (c *Compositor) getGlyph(r rune) (*font.Glyph, error) {
	if g := c.atlas.Get(r); g != nil {
		return g, nil
	}
	g, err := c.face.Glyph(r)
	if err != nil || g == nil {
		return nil, err
	}
	c.atlas.Put(r, g)
	return g, nil
}

func (c *Compositor) Render(buf *screen.Buffer) error {
	regions := buf.DirtyRegions()
	cursor := buf.Cursor()

	cursorMoved := false
	if cursor != nil {
		cursorMoved = cursor.Row != c.prevCursor.row ||
			cursor.Col != c.prevCursor.col ||
			cursor.Visible != c.prevCursor.visible
	}

	if len(regions) == 0 && !cursorMoved {
		return nil
	}

	if cursorMoved && c.prevCursor.visible {
		regions = append(regions, screen.Rect{
			X: c.prevCursor.col, Y: c.prevCursor.row,
			W: 1, H: 1,
		})
	}

	for _, r := range regions {
		for row := r.Y; row < r.Y+r.H && row < buf.Rows(); row++ {
			for col := r.X; col < r.X+r.W && col < buf.Cols(); col++ {
				cell := buf.Cell(row, col)
				if cell == nil {
					continue
				}
				px := col * c.metrics.Width
				py := row * c.metrics.Height

				bg := c.resolveBg(cell.Bg)
				fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, bg.R, bg.G, bg.B)

				if cell.Rune != 0 && cell.Rune != ' ' {
					glyph, err := c.getGlyph(cell.Rune)
					if err != nil || glyph == nil {
						continue
					}
					fg := c.resolveFg(cell.Fg)
					if cell.Attr&screen.AttrReverse != 0 {
						fg, bg = bg, fg
						fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, bg.R, bg.G, bg.B)
					}
					gx := px + glyph.XOffset
					gy := py + c.metrics.Ascent + glyph.YOffset
					blendGlyph(c.backBuf, c.backStride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fg.R, fg.G, fg.B)
				} else if cell.Attr&screen.AttrReverse != 0 {
					fg := c.resolveFg(cell.Fg)
					cellBg := c.resolveBg(cell.Bg)
					fg, cellBg = cellBg, fg
					fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
				}
			}
		}
	}

	if cursor != nil && cursor.Visible {
		cx := cursor.Col * c.metrics.Width
		cy := cursor.Row * c.metrics.Height
		fg := c.resolveFg(screen.Color{IsDefault: true})
		drawCursor(c.backBuf, c.backStride, cx, cy, c.metrics.Width, c.metrics.Height, toCursorStyle(cursor.Style), fg.R, fg.G, fg.B)
	}

	c.copyDirtyToSurface(regions, cursor)

	if err := c.surface.Swap(); err != nil {
		return err
	}

	if cursor != nil {
		c.prevCursor = cursorState{
			row:     cursor.Row,
			col:     cursor.Col,
			visible: cursor.Visible,
		}
	}
	buf.ClearDirty()
	return nil
}

func (c *Compositor) RenderAll(buf *screen.Buffer) error {
	bg := c.defColor.bg
	fillRect(c.backBuf, c.backStride, 0, 0, c.backWidth, c.backHeight, bg.R, bg.G, bg.B)

	for row := 0; row < buf.Rows() && row < c.rows; row++ {
		for col := 0; col < buf.Cols() && col < c.cols; col++ {
			cell := buf.Cell(row, col)
			if cell == nil {
				continue
			}
			px := col * c.metrics.Width
			py := row * c.metrics.Height

			cellBg := c.resolveBg(cell.Bg)
			if cellBg != bg {
				fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
			}

			if cell.Rune != 0 && cell.Rune != ' ' {
				glyph, err := c.getGlyph(cell.Rune)
				if err != nil || glyph == nil {
					continue
				}
				fg := c.resolveFg(cell.Fg)
				if cell.Attr&screen.AttrReverse != 0 {
					fg, cellBg = cellBg, fg
					fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
				}
				gx := px + glyph.XOffset
				gy := py + c.metrics.Ascent + glyph.YOffset
				blendGlyph(c.backBuf, c.backStride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fg.R, fg.G, fg.B)
			} else if cell.Attr&screen.AttrReverse != 0 {
				fg := c.resolveFg(cell.Fg)
				cellBg := c.resolveBg(cell.Bg)
				fg, cellBg = cellBg, fg
				fillRect(c.backBuf, c.backStride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
			}
		}
	}

	cursor := buf.Cursor()
	if cursor != nil && cursor.Visible {
		cx := cursor.Col * c.metrics.Width
		cy := cursor.Row * c.metrics.Height
		fg := c.resolveFg(screen.Color{IsDefault: true})
		drawCursor(c.backBuf, c.backStride, cx, cy, c.metrics.Width, c.metrics.Height, toCursorStyle(cursor.Style), fg.R, fg.G, fg.B)
	}

	c.copyAllToSurface()

	if err := c.surface.Swap(); err != nil {
		return err
	}

	if cursor != nil {
		c.prevCursor = cursorState{
			row:     cursor.Row,
			col:     cursor.Col,
			visible: cursor.Visible,
		}
	}
	buf.ClearDirty()
	return nil
}

func (c *Compositor) Resize(cols, rows int) {
	c.cols = cols
	c.rows = rows
	w, h := c.surface.Size()
	stride := w * 4
	c.backBuf = make([]byte, stride*h)
	c.backStride = stride
	c.backWidth = w
	c.backHeight = h
	bg := c.defColor.bg
	fillRect(c.backBuf, c.backStride, 0, 0, w, h, bg.R, bg.G, bg.B)
	c.prevCursor = cursorState{}
}

func (c *Compositor) copyDirtyToSurface(regions []screen.Rect, cursor *screen.Cursor) {
	surfData := c.surface.Data()
	surfStride := c.surface.Stride()

	if cursor != nil && cursor.Visible {
		regions = append(regions, screen.Rect{
			X: cursor.Col, Y: cursor.Row,
			W: 1, H: 1,
		})
	}

	for _, r := range regions {
		px := r.X * c.metrics.Width
		py := r.Y * c.metrics.Height
		pw := r.W * c.metrics.Width
		ph := r.H * c.metrics.Height

		if px+pw > c.backWidth {
			pw = c.backWidth - px
		}
		if py+ph > c.backHeight {
			ph = c.backHeight - py
		}
		if px < 0 {
			pw += px
			px = 0
		}
		if py < 0 {
			ph += py
			py = 0
		}
		if pw <= 0 || ph <= 0 {
			continue
		}

		for row := py; row < py+ph; row++ {
			srcOff := row*c.backStride + px*4
			dstOff := row*surfStride + px*4
			copy(surfData[dstOff:dstOff+pw*4], c.backBuf[srcOff:srcOff+pw*4])
		}
	}
}

func (c *Compositor) copyAllToSurface() {
	surfData := c.surface.Data()
	surfStride := c.surface.Stride()
	rowLen := c.backWidth * 4
	for row := 0; row < c.backHeight; row++ {
		srcOff := row * c.backStride
		dstOff := row * surfStride
		copy(surfData[dstOff:dstOff+rowLen], c.backBuf[srcOff:srcOff+rowLen])
	}
}

func (c *Compositor) Close() error {
	return c.surface.Close()
}

func (c *Compositor) resolveBg(col screen.Color) screen.Color {
	if col.IsDefault {
		return c.defColor.bg
	}
	return col
}

func (c *Compositor) resolveFg(col screen.Color) screen.Color {
	if col.IsDefault {
		return c.defColor.fg
	}
	return col
}

func toCursorStyle(s screen.CursorStyle) cursorStyle {
	switch s {
	case screen.CursorUnderline:
		return cursorUnderline
	case screen.CursorBar:
		return cursorBar
	default:
		return cursorBlock
	}
}
