package ui

import (
	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/panel"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/runeutil"
)

type StatusBarTheme struct {
	Bg [3]uint8
}

var DefaultStatusBarTheme = StatusBarTheme{
	Bg: [3]uint8{24, 24, 24},
}

type StatusBar struct {
	face       font.Face
	metrics    font.Metrics
	primitives []panel.Primitive
	lines      int
	theme      StatusBarTheme
	gp         render.GlyphProvider
	uploader   render.GPUGlyphUploader
}

func NewStatusBar(face font.Face, theme StatusBarTheme) *StatusBar {
	if theme.Bg == [3]uint8{} {
		theme = DefaultStatusBarTheme
	}
	sb := &StatusBar{
		face:  face,
		theme: theme,
	}
	if face != nil {
		sb.metrics = face.Metrics()
	}
	return sb
}

func (sb *StatusBar) SetTheme(t StatusBarTheme) {
	sb.theme = t
}

func (sb *StatusBar) SetPrimitives(prims []panel.Primitive) {
	sb.primitives = prims
}

func (sb *StatusBar) SetLines(n int) {
	sb.lines = n
}

func (sb *StatusBar) Lines() int {
	return sb.lines
}

func (sb *StatusBar) Insets() (top, bottom, left, right int) {
	if sb.metrics.Height <= 0 {
		return 0, 0, 0, 0
	}
	return 0, sb.lines * sb.metrics.Height, 0, 0
}

func (sb *StatusBar) SetGlyphProvider(gp render.GlyphProvider) {
	sb.gp = gp
}

func (sb *StatusBar) SetGPUGlyphUploader(u render.GPUGlyphUploader) {
	sb.uploader = u
}

func (sb *StatusBar) UpdateFace(face font.Face) {
	sb.face = face
	if face != nil {
		sb.metrics = face.Metrics()
	}
}

func (sb *StatusBar) RenderCPU(buf []byte, stride, width, height int) {
	if sb.primitives == nil || sb.metrics.Height <= 0 {
		return
	}
	bottom := sb.lines * sb.metrics.Height
	yOff := height - bottom
	clipX := 0
	clipY := yOff
	clipW := width
	clipH := bottom
	for _, p := range sb.primitives {
		sb.drawPrimitiveCPU(buf, stride, width, height, p, 0, yOff, clipX, clipY, clipW, clipH)
	}
}

func (sb *StatusBar) drawPrimitiveCPU(buf []byte, stride, frameW, frameH int, p panel.Primitive, xOff, yOff, clipX, clipY, clipW, clipH int) {
	if sb.metrics.Width <= 0 || sb.metrics.Height <= 0 {
		return
	}
	absX := xOff + p.X*sb.metrics.Width
	absY := yOff + p.Y*sb.metrics.Height
	if p.Kind == panel.PrimRect {
		w := p.W * sb.metrics.Width
		h := p.H * sb.metrics.Height
		if w <= 0 || h <= 0 {
			return
		}
		if absX < clipX {
			dx := clipX - absX
			w -= dx
			absX = clipX
		}
		if absX+w > clipX+clipW {
			w = clipX + clipW - absX
		}
		if absY < clipY {
			dy := clipY - absY
			h -= dy
			absY = clipY
		}
		if absY+h > clipY+clipH {
			h = clipY + clipH - absY
		}
		if w <= 0 || h <= 0 {
			return
		}
		render.FillRectBlend(buf, stride, absX, absY, w, h, p.Bg[0], p.Bg[1], p.Bg[2], p.Bg[3])
		return
	}
	xpos := 0
	for _, ch := range p.Text {
		if ch == 0 || sb.gp == nil {
			xpos++
			continue
		}
		g := sb.gp.OverlayGlyph(ch)
		if g == nil {
			xpos++
			continue
		}
		rw := 1
		if runeutil.IsWide(ch) {
			rw = 2
		}
		cellX := absX + xpos*sb.metrics.Width
		if cellX >= clipX+clipW {
			break
		}
		gx := cellX + g.XOffset
		gy := absY + sb.metrics.Ascent + g.YOffset
		render.BlendGlyphAlpha(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, p.Fg[0], p.Fg[1], p.Fg[2], p.Fg[3])
		xpos += rw
	}
}

func (sb *StatusBar) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if sb.primitives == nil || sb.metrics.Height <= 0 || sb.metrics.Width <= 0 {
		return
	}
	bottom := sb.lines * sb.metrics.Height
	yOff := float32(height - bottom)
	cellW := float32(sb.metrics.Width)
	cellH := float32(sb.metrics.Height)
	clipX := float32(0)
	clipY := yOff
	clipW := float32(width)
	clipH := float32(bottom)

	drawPrimGPU := func(p panel.Primitive, xOff, yOff, clipX, clipY, clipW, clipH float32) {
		bgA := float32(p.Bg[3]) / 255
		fgA := float32(p.Fg[3]) / 255
		if p.Kind == panel.PrimRect {
			for j := 0; j < p.H; j++ {
				for i := 0; i < p.W; i++ {
					px := xOff + float32(p.X+i)*cellW
					py := yOff + float32(p.Y+j)*cellH
					if px < clipX || px >= clipX+clipW || py < clipY || py >= clipY+clipH {
						continue
					}
					inst := platform.CellInstance{
						X:     px,
						Y:     py,
						CellW: cellW, CellH: cellH,
				BgR:   float32(p.Bg[0]) / 255,
				BgG:   float32(p.Bg[1]) / 255,
				BgB:   float32(p.Bg[2]) / 255,
				BgA: bgA,
			}
			*instances = append(*instances, inst)
		}
	}
	return
}
xpos := p.X
for _, ch := range p.Text {
	rw := 1
	if runeutil.IsWide(ch) {
		rw = 2
	}
	cellX := xOff + float32(xpos)*cellW
	if cellX >= clipX+clipW {
		break
	}
	inst := platform.CellInstance{
		X:     cellX,
		Y:     yOff + float32(p.Y)*cellH,
		CellW: float32(rw) * cellW, CellH: cellH,
		FgR:   float32(p.Fg[0]) / 255 * fgA,
		FgG:   float32(p.Fg[1]) / 255 * fgA,
		FgB:   float32(p.Fg[2]) / 255 * fgA,
		BgR:   float32(p.Bg[0]) / 255,
		BgG:   float32(p.Bg[1]) / 255,
		BgB:   float32(p.Bg[2]) / 255,
				BgA: bgA,
			}
			if ch != 0 && sb.uploader != nil {
				u0, v0, u1, v1, gw, gh, xoff, yoff, ok := sb.uploader.OverlayUploadGlyph(ch)
				if ok {
					inst.GlyphU0 = u0
					inst.V0 = v0
					inst.GlyphU1 = u1
					inst.V1 = v1
					inst.GlyphOffX = float32(xoff)
					inst.GlyphOffY = float32(sb.metrics.Ascent + yoff)
					inst.GlyphW = float32(gw)
					inst.GlyphH = float32(gh)
				}
			}
			*instances = append(*instances, inst)
			xpos += rw
		}
	}

	for _, p := range sb.primitives {
		drawPrimGPU(p, 0, yOff, clipX, clipY, clipW, clipH)
	}
}
