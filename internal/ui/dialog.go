package ui

import (
	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/runeutil"
)

type DialogResult int

const (
	DialogNone DialogResult = iota
	DialogOK
	DialogCancel
)

type Dialog struct {
	face     font.Face
	metrics  font.Metrics
	gp       render.GlyphProvider
	uploader render.GPUGlyphUploader
	title    string
	input    *InputField
	buttons  []string
	result   DialogResult
	bgAlpha  float32
	zOrder   int
	closed   bool
}

func NewDialog(face7 font.Face, title string, input *InputField, buttons []string) *Dialog {
	m := font.Metrics{Width: 8, Height: 16, Ascent: 12}
	if face7 != nil {
		m = face7.Metrics()
	}
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	return &Dialog{
		face:    face7,
		metrics: m,
		title:   title,
		input:   input,
		buttons: buttons,
		bgAlpha: 0.92,
		zOrder:  200,
	}
}

func (d *Dialog) SetGlyphProvider(gp render.GlyphProvider)      { d.gp = gp }
func (d *Dialog) SetGPUGlyphUploader(u render.GPUGlyphUploader) { d.uploader = u }
func (d *Dialog) ZOrder() int                                    { return d.zOrder }
func (d *Dialog) Close()                                         { d.closed = true }

func (d *Dialog) Result() DialogResult  { return d.result }
func (d *Dialog) InputField() *InputField { return d.input }
func (d *Dialog) Closed() bool           { return d.closed }

func (d *Dialog) SetFace(face font.Face) {
	d.face = face
	d.metrics = face.Metrics()
}

func (d *Dialog) HandleKey(ev platform.KeyEvent) {
	if ev.State != platform.KeyPress {
		return
	}
	if ev.Code == 1 {
		d.result = DialogCancel
		d.closed = true
		return
	}
	if ev.Rune != 0 && ev.Mods == 0 && d.input != nil {
		d.input.InsertText(string(ev.Rune))
		return
	}
	if ev.Code == 14 && d.input != nil {
		d.input.DeleteBackward()
		return
	}
	if ev.Code == 28 {
		d.result = DialogOK
		d.closed = true
		return
	}
}

func (d *Dialog) CommitText(text string) {
	if d.input != nil {
		d.input.InsertText(text)
	}
}

func (d *Dialog) layout(width, height int) (x, y, w, h int) {
	desiredW := 40 * d.metrics.Width
	if desiredW > width*3/4 {
		desiredW = width * 3 / 4
	}
	rows := 2
	if d.input != nil {
		rows++
	}
	if len(d.buttons) > 0 {
		rows++
	}
	desiredH := rows * d.metrics.Height
	x = (width - desiredW) / 2
	y = (height - desiredH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, desiredW, desiredH
}

func (d *Dialog) RenderCPU(buf []byte, stride, width, height int) {
	if d.metrics.Height <= 0 || d.bgAlpha <= 0 {
		return
	}
	x, y, w, h := d.layout(width, height)
	a := uint8(d.bgAlpha * 255)
	render.FillRectBlend(buf, stride, x, y, w, h, 30, 30, 30, a)
	if d.gp == nil {
		return
	}
	d.renderTextCPU(buf, stride, x, y, w, width, height)
}

func (d *Dialog) renderTextCPU(buf []byte, stride, x, y, w, frameW, frameH int) {
	fgR, fgG, fgB := uint8(230), uint8(230), uint8(230)
	if d.title != "" {
		xpos := 0
		for _, ch := range d.title {
			g := d.gp.OverlayGlyph(ch)
			if g == nil {
				rw := 1
				if runeutil.IsWide(ch) {
					rw = 2
				}
				xpos += rw
				continue
			}
			gx := x + xpos*d.metrics.Width + g.XOffset
			gy := y + d.metrics.Ascent + g.YOffset
			render.BlendGlyphAlpha(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, fgR, fgG, fgB, 255)
			rw := 1
			if runeutil.IsWide(ch) {
				rw = 2
			}
			xpos += rw
			if xpos*d.metrics.Width >= w {
				break
			}
		}
	}
	inputY := y + d.metrics.Height
	if d.input != nil {
		text := d.input.DisplayText()
		if text != "" {
			xpos := 0
			for _, ch := range text {
				g := d.gp.OverlayGlyph(ch)
				if g == nil {
					rw := 1
					if runeutil.IsWide(ch) {
						rw = 2
					}
					xpos += rw
					continue
				}
				gx := x + xpos*d.metrics.Width + g.XOffset
				gy := inputY + d.metrics.Ascent + g.YOffset
				render.BlendGlyphAlpha(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, fgR, fgG, fgB, 255)
				rw := 1
				if runeutil.IsWide(ch) {
					rw = 2
				}
				xpos += rw
				if xpos*d.metrics.Width >= w {
					break
				}
			}
		}
	}
}

func (d *Dialog) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if d.metrics.Height <= 0 || d.metrics.Width <= 0 || d.bgAlpha <= 0 {
		return
	}
	x, y, w, h := d.layout(width, height)
	cellW := float32(d.metrics.Width)
	cellH := float32(d.metrics.Height)
	cols := w / d.metrics.Width
	rows := h / d.metrics.Height
	bgR := 30.0 / 255 * d.bgAlpha
	bgG := 30.0 / 255 * d.bgAlpha
	bgB := 30.0 / 255 * d.bgAlpha
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			inst := platform.CellInstance{
				X:     float32(x) + float32(col)*cellW,
				Y:     float32(y) + float32(row)*cellH,
				CellW: cellW,
				CellH: cellH,
				BgR:   bgR,
				BgG:   bgG,
				BgB:   bgB,
				BgA:   d.bgAlpha,
			}
			*instances = append(*instances, inst)
		}
	}
	if d.uploader == nil {
		return
	}
	fgR := float32(230.0 / 255)
	fgG := float32(230.0 / 255)
	fgB := float32(230.0 / 255)
	if d.title != "" {
		xpos := 0
		for _, ch := range d.title {
			u0, v0, u1, v1, gw, gh, xoff, yoff, ok := d.uploader.OverlayUploadGlyph(ch)
			if ok {
				inst := platform.CellInstance{
					X:         float32(x) + float32(xpos)*cellW,
					Y:         float32(y),
					CellW:     cellW,
					CellH:     cellH,
					FgR:       fgR,
					FgG:       fgG,
					FgB:       fgB,
					BgA:       d.bgAlpha,
					GlyphU0:   u0,
					V0:        v0,
					GlyphU1:   u1,
					V1:        v1,
					GlyphOffX: float32(xoff),
					GlyphOffY: float32(d.metrics.Ascent + yoff),
					GlyphW:    float32(gw),
					GlyphH:    float32(gh),
				}
				*instances = append(*instances, inst)
			}
			rw := 1
			if runeutil.IsWide(ch) {
				rw = 2
			}
			xpos += rw
			if xpos*d.metrics.Width >= w {
				break
			}
		}
	}
}
