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
	surface  platform.Surface
	face     font.Face
	atlas    *font.Atlas
	metrics  font.Metrics
	cols     int
	rows     int
	defColor defaultColor
}

func NewCompositor(surface platform.Surface, face font.Face) *Compositor {
	m := face.Metrics()
	w, _ := surface.Size()
	cols := w / m.Width
	rows := 0
	if m.Height > 0 {
		_, h := surface.Size()
		rows = h / m.Height
	}
	return &Compositor{
		surface: surface,
		face:    face,
		atlas:   font.NewAtlas(4096),
		metrics: m,
		cols:    cols,
		rows:    rows,
		defColor: defaultColor{
			bg: screen.Color{R: 0, G: 0, B: 0},
			fg: screen.Color{R: 204, G: 204, B: 204},
		},
	}
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
	hasCursorChange := cursor != nil && cursor.Visible

	if len(regions) == 0 && !hasCursorChange {
		return nil
	}

	data := c.surface.Data()
	stride := c.surface.Stride()

	if cursor != nil && cursor.Visible {
		cx := cursor.Col * c.metrics.Width
		cy := cursor.Row * c.metrics.Height
		fg := c.resolveFg(screen.Color{IsDefault: true})
		drawCursor(data, stride, cx, cy, c.metrics.Width, c.metrics.Height, toCursorStyle(cursor.Style), fg.R, fg.G, fg.B)
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
				fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, bg.R, bg.G, bg.B)

				if cell.Rune != 0 && cell.Rune != ' ' {
					glyph, err := c.getGlyph(cell.Rune)
					if err != nil || glyph == nil {
						continue
					}
					fg := c.resolveFg(cell.Fg)
					if cell.Attr&screen.AttrReverse != 0 {
						fg, bg = bg, fg
						fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, bg.R, bg.G, bg.B)
					}
					gx := px + glyph.XOffset
					gy := py + c.metrics.Ascent + glyph.YOffset
					blendGlyph(data, stride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fg.R, fg.G, fg.B)
				} else if cell.Attr&screen.AttrReverse != 0 {
					fg := c.resolveFg(cell.Fg)
					cellBg := c.resolveBg(cell.Bg)
					fg, cellBg = cellBg, fg
					fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
				}
			}
		}
	}

	if err := c.surface.Swap(); err != nil {
		return err
	}
	buf.ClearDirty()
	return nil
}

func (c *Compositor) RenderAll(buf *screen.Buffer) error {
	data := c.surface.Data()
	stride := c.surface.Stride()

	bg := c.defColor.bg
	surfW, surfH := c.surface.Size()
	fillRect(data, stride, 0, 0, surfW, surfH, bg.R, bg.G, bg.B)

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
				fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
			}

			if cell.Rune != 0 && cell.Rune != ' ' {
				glyph, err := c.getGlyph(cell.Rune)
				if err != nil || glyph == nil {
					continue
				}
				fg := c.resolveFg(cell.Fg)
				if cell.Attr&screen.AttrReverse != 0 {
					fg, cellBg = cellBg, fg
					fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
				}
				gx := px + glyph.XOffset
				gy := py + c.metrics.Ascent + glyph.YOffset
				blendGlyph(data, stride, gx, gy, glyph.Bitmap, glyph.Width, glyph.Height, fg.R, fg.G, fg.B)
			} else if cell.Attr&screen.AttrReverse != 0 {
				fg := c.resolveFg(cell.Fg)
				cellBg := c.resolveBg(cell.Bg)
				fg, cellBg = cellBg, fg
				fillRect(data, stride, px, py, c.metrics.Width, c.metrics.Height, cellBg.R, cellBg.G, cellBg.B)
			}
		}
	}

	cursor := buf.Cursor()
	if cursor != nil && cursor.Visible {
		cx := cursor.Col * c.metrics.Width
		cy := cursor.Row * c.metrics.Height
		fg := c.resolveFg(screen.Color{IsDefault: true})
		drawCursor(data, stride, cx, cy, c.metrics.Width, c.metrics.Height, toCursorStyle(cursor.Style), fg.R, fg.G, fg.B)
	}

	if err := c.surface.Swap(); err != nil {
		return err
	}
	buf.ClearDirty()
	return nil
}

func (c *Compositor) Resize(cols, rows int) {
	c.cols = cols
	c.rows = rows
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
