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
	frameCount uint64
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
		atlas:      font.NewAtlas(8192),
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

// SetFace replaces the active font face. The glyph atlas is rebuilt because
// cached bitmaps are bound to the previous size. The old face is NOT closed
// here — when a font.FaceCache owns the face it reclaims it at shutdown;
// callers owning the face directly must close it themselves.
func (c *Compositor) SetFace(face font.Face) {
	c.face = face
	c.atlas = font.NewAtlas(8192)
	c.metrics = face.Metrics()
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

func (c *Compositor) Render(buf *screen.Buffer, scrollOffset int) error {
	history := buf.History()
	histLen := history.Len()
	offset := scrollOffset
	if offset > histLen {
		offset = histLen
	}

	cursor := buf.Cursor()

	c.frameCount++

	if offset > 0 {
		bg := c.defColor.bg
		fillRect(c.backBuf, c.backStride, 0, 0, c.backWidth, c.backHeight, bg.R, bg.G, bg.B)
	}

	for row := 0; row < c.rows; row++ {
		var line *screen.Line
		isHistory := row < offset
		if isHistory {
			line = history.Line(histLen - offset + row)
		} else {
			line = buf.Line(row - offset)
		}
		if line == nil {
			continue
		}
		for col := 0; col < c.cols; col++ {
			cell := line.Cell(col)
			if cell == nil {
				continue
			}
			if cell.Width == 0 {
				continue
			}
			px := col * c.metrics.Width
			py := row * c.metrics.Height
			cellW := int(cell.Width) * c.metrics.Width

			fg := c.resolveFg(cell.Fg)
			bg := c.resolveBg(cell.Bg)
			fgR, fgG, fgB := fg.R, fg.G, fg.B
			bgR, bgG, bgB := bg.R, bg.G, bg.B

			if cell.Attr&screen.AttrReverse != 0 {
				fgR, fgG, fgB, bgR, bgG, bgB = bgR, bgG, bgB, fgR, fgG, fgB
			}
			if cell.Attr&screen.AttrDim != 0 {
				fgR = fgR / 2
				fgG = fgG / 2
				fgB = fgB / 2
			}

			fillRect(c.backBuf, c.backStride, px, py, cellW, c.metrics.Height, bgR, bgG, bgB)

			if cell.Rune != 0 {
				glyph, err := c.getGlyph(cell.Rune)
				if err != nil || glyph == nil {
					continue
				}
				gx := px + glyph.XOffset
				gy := py + c.metrics.Ascent + glyph.YOffset
				if cell.Attr&screen.AttrBold != 0 {
					blendGlyph(c.backBuf, c.backStride, px+1, gy, glyph.Bitmap, glyph.Width, glyph.Height, fgR, fgG, fgB)
				}
				if cell.Attr&screen.AttrItalic != 0 {
					blendGlyphItalic(c.backBuf, c.backStride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fgR, fgG, fgB)
				} else {
					blendGlyph(c.backBuf, c.backStride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fgR, fgG, fgB)
				}

				if cell.Attr&screen.AttrUnderline != 0 {
					underlineY := py + c.metrics.Ascent + 1
					if underlineY < py+c.metrics.Height {
						for x := px; x < px+cellW; x++ {
							off := underlineY*c.backStride + x*4
							if off+3 < len(c.backBuf) {
								c.backBuf[off] = fgB
								c.backBuf[off+1] = fgG
								c.backBuf[off+2] = fgR
								c.backBuf[off+3] = 255
							}
						}
					}
				}

				if cell.Attr&screen.AttrCrossedOut != 0 {
					midY := py + c.metrics.Height/2
					for x := px; x < px+cellW; x++ {
						off := midY*c.backStride + x*4
						if off+3 < len(c.backBuf) {
							c.backBuf[off] = fgB
							c.backBuf[off+1] = fgG
							c.backBuf[off+2] = fgR
							c.backBuf[off+3] = 255
						}
					}
				}
			}
		}
	}

	if cursor != nil && offset == 0 {
		visible := cursor.Visible
		if cursor.Blinking && c.frameCount%30 < 15 {
			visible = false
		}
		if visible {
			cx := cursor.Col * c.metrics.Width
			cy := cursor.Row * c.metrics.Height
			cursorCell := buf.Cell(cursor.Row, cursor.Col)
			var cursorRune rune
			var cursorGlyph *font.Glyph
			cursorW := c.metrics.Width
			if cursorCell != nil {
				cursorRune = cursorCell.Rune
				if cursorCell.Width == 2 {
					cursorW *= 2
				}
				cursorGlyph, _ = c.getGlyph(cursorRune)
			}
			fg := c.resolveFg(screen.Color{IsDefault: true})
			drawCursor(c.backBuf, c.backStride, cx, cy, cursorW, c.metrics.Height, toCursorStyle(cursor.Style), fg.R, fg.G, fg.B, cursorRune, cursorGlyph, c.metrics.Ascent)
		}
	}

	c.copyAllToSurface()

	if err := c.surface.Swap(); err != nil {
		return err
	}

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
}

func (c *Compositor) copyAllToSurface() {
	surfData := c.surface.Data()
	if surfData == nil {
		return
	}
	surfStride := c.surface.Stride()
	backStride := c.backStride
	if surfStride == backStride {
		totalBytes := backStride * c.backHeight
		copy(surfData[:totalBytes], c.backBuf[:totalBytes])
		return
	}
	minStride := backStride
	if surfStride < minStride {
		minStride = surfStride
	}
	for y := 0; y < c.backHeight; y++ {
		srcOff := y * backStride
		dstOff := y * surfStride
		copy(surfData[dstOff:dstOff+minStride], c.backBuf[srcOff:srcOff+minStride])
	}
}

func (c *Compositor) Close() error {
	return c.surface.Close()
}

func (c *Compositor) BackBuf() (data []byte, stride, width, height int) {
	return c.backBuf, c.backStride, c.backWidth, c.backHeight
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
