package ui

import (
	"time"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/runeutil"
)

type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastWarn
	ToastError
)

type Toast struct {
	face     font.Face
	metrics  font.Metrics
	gp       render.GlyphProvider
	uploader render.GPUGlyphUploader
	message  string
	level    ToastLevel
	bgAlpha  float32
	deadline time.Time
	zOrder   int
	closed   bool
}

func NewToast(face7 font.Face, message string, level ToastLevel, duration time.Duration) *Toast {
	m := font.Metrics{Width: 8, Height: 16, Ascent: 12}
	if face7 != nil {
		m = face7.Metrics()
	}
	return &Toast{
		face:     face7,
		metrics:  m,
		message:  message,
		level:    level,
		bgAlpha:  0.9,
		deadline: time.Now().Add(duration),
		zOrder:   100,
	}
}

func (t *Toast) SetGlyphProvider(gp render.GlyphProvider)       { t.gp = gp }
func (t *Toast) SetGPUGlyphUploader(u render.GPUGlyphUploader)  { t.uploader = u }
func (t *Toast) ZOrder() int                                     { return t.zOrder }
func (t *Toast) Close()                                          { t.closed = true }
func (t *Toast) Expired() bool                                   { return time.Now().After(t.deadline) || t.closed }

func (t *Toast) SetFace(face font.Face) {
	t.face = face
	t.metrics = face.Metrics()
}

func (t *Toast) RenderCPU(buf []byte, stride, width, height int) {
	if t.message == "" || t.metrics.Height <= 0 || t.bgAlpha <= 0 {
		return
	}
	lineH := t.metrics.Height
	yOff := height - lineH - t.metrics.Height
	if yOff < 0 {
		yOff = 0
	}
	var bgR, bgG, bgB uint8
	switch t.level {
	case ToastWarn:
		bgR, bgG, bgB = 180, 140, 20
	case ToastError:
		bgR, bgG, bgB = 180, 40, 40
	default:
		bgR, bgG, bgB = 40, 40, 40
	}
	a := uint8(t.bgAlpha * 255)
	render.FillRectBlend(buf, stride, 0, yOff, width, lineH, bgR, bgG, bgB, a)
	if t.gp == nil {
		return
	}
	fgR, fgG, fgB := uint8(230), uint8(230), uint8(230)
	xpos := 0
	for _, ch := range t.message {
		g := t.gp.OverlayGlyph(ch)
		if g == nil {
			rw := 1
			if runeutil.IsWide(ch) {
				rw = 2
			}
			xpos += rw
			continue
		}
		gx := xpos*t.metrics.Width + g.XOffset
		gy := yOff + t.metrics.Ascent + g.YOffset
		render.BlendGlyphAlpha(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, fgR, fgG, fgB, 255)
		rw := 1
		if runeutil.IsWide(ch) {
			rw = 2
		}
		xpos += rw
		if xpos*t.metrics.Width >= width {
			break
		}
	}
}

func (t *Toast) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if t.message == "" || t.metrics.Height <= 0 || t.metrics.Width <= 0 || t.bgAlpha <= 0 {
		return
	}
	lineH := float32(t.metrics.Height)
	yOff := float32(height) - lineH - float32(t.metrics.Height)
	if yOff < 0 {
		yOff = 0
	}
	var bgR, bgG, bgB float32
	switch t.level {
	case ToastWarn:
		bgR, bgG, bgB = 180.0/255, 140.0/255, 20.0/255
	case ToastError:
		bgR, bgG, bgB = 180.0/255, 40.0/255, 40.0/255
	default:
		bgR, bgG, bgB = 40.0/255, 40.0/255, 40.0/255
	}
	cellW := float32(t.metrics.Width)
	cellH := float32(t.metrics.Height)
	cols := width / t.metrics.Width
	for i := 0; i < cols; i++ {
		inst := platform.CellInstance{
			X:     float32(i) * cellW,
			Y:     yOff,
			CellW: cellW,
			CellH: cellH,
			BgR:   bgR,
			BgG:   bgG,
			BgB:   bgB,
			BgA:   t.bgAlpha,
		}
		*instances = append(*instances, inst)
	}
	if t.uploader == nil {
		return
	}
	xpos := 0
	for _, ch := range t.message {
		rw := 1
		if runeutil.IsWide(ch) {
			rw = 2
		}
		cellX := float32(xpos) * cellW
		if cellX >= float32(width) {
			break
		}
		u0, v0, u1, v1, gw, gh, xoff, yoff, ok := t.uploader.OverlayUploadGlyph(ch)
		if ok {
			inst := platform.CellInstance{
				X:         cellX,
				Y:         yOff,
				CellW:     float32(rw) * cellW,
				CellH:     cellH,
				FgR:       230.0 / 255,
				FgG:       230.0 / 255,
				FgB:       230.0 / 255,
				BgA:       0,
				GlyphU0:   u0,
				V0:        v0,
				GlyphU1:   u1,
				V1:        v1,
				GlyphOffX: float32(xoff),
				GlyphOffY: float32(t.metrics.Ascent + yoff),
				GlyphW:    float32(gw),
				GlyphH:    float32(gh),
			}
			*instances = append(*instances, inst)
		}
		xpos += rw
	}
}
