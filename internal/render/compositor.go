package render

import (
	"time"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
)

type defaultColor struct {
	bg screen.Color
	fg screen.Color
}

type Compositor struct {
	surface      platform.Surface
	face         font.Face
	atlas        *font.Atlas
	metrics      font.Metrics
	cols         int
	rows         int
	defColor     defaultColor
	backBuf      []byte
	backStride   int
	backWidth    int
	backHeight   int
	frameCount   uint64
	blinkOn      bool
	lastBlink    time.Time
	directRender bool
	gpu          platform.GPURenderer
	instances    []platform.CellInstance
}

func NewCompositor(surface platform.Surface, face font.Face) *Compositor {
	m := face.Metrics()
	w, h := surface.Size()
	cols := w / m.Width
	rows := 0
	if m.Height > 0 {
		rows = h / m.Height
	}
	c := &Compositor{
		surface:    surface,
		face:       face,
		atlas:      font.NewAtlas(8192),
		metrics:    m,
		cols:       cols,
		rows:       rows,
		backWidth:  w,
		backHeight: h,
		defColor: defaultColor{
			bg: screen.Color{R: 0, G: 0, B: 0},
			fg: screen.Color{R: 204, G: 204, B: 204},
		},
		blinkOn: true,
	}
	c.directRender = surface.DirectRender()
	if !c.directRender {
		stride := w * 4
		c.backBuf = make([]byte, stride*h)
		c.backStride = stride
		bg := c.defColor.bg
		fillRect(c.backBuf, c.backStride, 0, 0, w, h, bg.R, bg.G, bg.B)
	}
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
	if c.gpu == nil {
		c.gpu, _ = c.surface.(platform.GPURenderer)
	}
	if c.gpu != nil {
		c.gpu.ResetAtlas()
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

// cursorVisible 返回光标是否应当绘制。闪烁基于真实时间戳，
// 与 frameCount 解耦，使 dirty 跳帧时光标仍能正确闪烁。
func (c *Compositor) cursorVisible(cursor *screen.Cursor) bool {
	visible := cursor.Visible
	if cursor.Blinking {
		if time.Since(c.lastBlink) >= 500*time.Millisecond {
			c.blinkOn = !c.blinkOn
			c.lastBlink = time.Now()
		}
		if !c.blinkOn {
			visible = false
		}
	}
	return visible
}

func (c *Compositor) Render(buf *screen.Buffer, scrollOffset int) error {
	c.frameCount++

	// GPU instanced draw 路径
	if c.gpu == nil {
		c.gpu, _ = c.surface.(platform.GPURenderer)
	}
	if c.gpu != nil {
		return c.renderGPU(buf, scrollOffset)
	}

	history := buf.History()
	histLen := history.Len()
	offset := scrollOffset
	if offset > histLen {
		offset = histLen
	}

	cursor := buf.Cursor()

	c.frameCount++

	if c.directRender {
		c.backBuf = c.surface.Data()
		c.backStride = c.surface.Stride()
		if c.backBuf == nil {
			return nil
		}
	}

	defBg := c.defColor.bg
	fillRect(c.backBuf, c.backStride, 0, 0, c.backWidth, c.backHeight, defBg.R, defBg.G, defBg.B)

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

			if bgR != defBg.R || bgG != defBg.G || bgB != defBg.B {
				fillRect(c.backBuf, c.backStride, px, py, cellW, c.metrics.Height, bgR, bgG, bgB)
			}

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
		visible := c.cursorVisible(cursor)
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

	if !c.directRender {
		c.copyAllToSurface()
	}

	if err := c.surface.Swap(); err != nil {
		return err
	}

	return nil
}

func (c *Compositor) renderGPU(buf *screen.Buffer, scrollOffset int) error {
	if err := c.gpu.BeginFrame(); err != nil {
		return err
	}
	history := buf.History()
	histLen := history.Len()
	offset := scrollOffset
	if offset > histLen {
		offset = histLen
	}
	cursor := buf.Cursor()

	defBg := c.defColor.bg

	// 预分配 instance buffer（复用底层数组）
	maxCells := c.cols * c.rows
	if cap(c.instances) < maxCells {
		c.instances = make([]platform.CellInstance, 0, maxCells)
	}
	c.instances = c.instances[:0]

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
			if cell == nil || cell.Width == 0 {
				continue
			}

			px := float32(col * c.metrics.Width)
			py := float32(row * c.metrics.Height)
			cellW := float32(int(cell.Width) * c.metrics.Width)
			cellH := float32(c.metrics.Height)

			fg := c.resolveFg(cell.Fg)
			bg := c.resolveBg(cell.Bg)
			fgR, fgG, fgB := float32(fg.R)/255, float32(fg.G)/255, float32(fg.B)/255
			bgR, bgG, bgB := float32(bg.R)/255, float32(bg.G)/255, float32(bg.B)/255
			hasBg := float32(0)

			if cell.Attr&screen.AttrReverse != 0 {
				fgR, fgG, fgB, bgR, bgG, bgB = bgR, bgG, bgB, fgR, fgG, fgB
			}
			if cell.Attr&screen.AttrDim != 0 {
				fgR /= 2
				fgG /= 2
				fgB /= 2
			}
			if bgR != float32(defBg.R)/255 || bgG != float32(defBg.G)/255 || bgB != float32(defBg.B)/255 {
				hasBg = 1
			}

			inst := platform.CellInstance{
				X:        px,
				Y:        py,
				CellW:    cellW,
				CellH:    cellH,
				GlyphOffX: 0,
				GlyphOffY: float32(c.metrics.Ascent),
				GlyphW:   float32(c.metrics.Width),
				GlyphH:   float32(c.metrics.Height),
				FgR:      fgR,
				FgG:      fgG,
				FgB:      fgB,
				BgR:      bgR,
				BgG:      bgG,
				BgB:      bgB,
				HasBg:    hasBg,
			}
			if cell.Attr&screen.AttrUnderline != 0 {
				inst.AttrFlags += 1
			}
			if cell.Attr&screen.AttrCrossedOut != 0 {
				inst.AttrFlags += 2
			}
			if cell.Attr&screen.AttrItalic != 0 {
				inst.AttrFlags += 4
			}

			if cell.Rune != 0 {
				glyph, err := c.getGlyph(cell.Rune)
				if err == nil && glyph != nil {
					u0, v0, u1, v1, ok := c.gpu.UploadGlyph(cell.Rune, glyph.Bitmap, glyph.Width, glyph.Height)
					if ok {
						inst.GlyphU0 = u0
						inst.V0 = v0
						inst.GlyphU1 = u1
						inst.V1 = v1
						inst.GlyphOffX = float32(glyph.XOffset)
						inst.GlyphOffY = float32(c.metrics.Ascent + glyph.YOffset)
						inst.GlyphW = float32(glyph.Width)
						inst.GlyphH = float32(glyph.Height)
						if cell.Attr&screen.AttrBold != 0 {
							inst.GlyphOffX += 1
						}
					}
				}
			}
			c.instances = append(c.instances, inst)
		}
	}

	// 光标处理（简化：反转光标 cell 的 fg/bg）
	if cursor != nil && offset == 0 {
		visible := c.cursorVisible(cursor)
		if visible {
			cx := cursor.Col
			cy := cursor.Row
			if cx < c.cols && cy < c.rows {
				targetX := float32(cx * c.metrics.Width)
				targetY := float32(cy * c.metrics.Height)
				for i := range c.instances {
					if c.instances[i].X == targetX && c.instances[i].Y == targetY {
						c.instances[i].FgR, c.instances[i].BgR = c.instances[i].BgR, c.instances[i].FgR
						c.instances[i].FgG, c.instances[i].BgG = c.instances[i].BgG, c.instances[i].FgG
						c.instances[i].FgB, c.instances[i].BgB = c.instances[i].BgB, c.instances[i].FgB
						c.instances[i].HasBg = 1
						break
					}
				}
			}
		}
	}

	bgColor := [3]float32{float32(defBg.R) / 255, float32(defBg.G) / 255, float32(defBg.B) / 255}
	if err := c.gpu.DrawInstances(c.instances, c.backWidth, c.backHeight, bgColor); err != nil {
		return err
	}

	return c.surface.Swap()
}

func (c *Compositor) Resize(cols, rows int) {
	c.cols = cols
	c.rows = rows
	w, h := c.surface.Size()
	c.backWidth = w
	c.backHeight = h
	if !c.directRender {
		stride := w * 4
		c.backBuf = make([]byte, stride*h)
		c.backStride = stride
		bg := c.defColor.bg
		fillRect(c.backBuf, c.backStride, 0, 0, w, h, bg.R, bg.G, bg.B)
	}
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

func (c *Compositor) SetDefaultColors(fg, bg screen.Color) {
	c.defColor.fg = fg
	c.defColor.bg = bg
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
