package ui

import (
	"strings"

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
	face        font.Face
	metrics     font.Metrics
	gp          render.GlyphProvider
	uploader    render.GPUGlyphUploader
	title       string
	content     string
	contentRows []string
	input       *InputField
	buttons     []string
	result      DialogResult
	bgAlpha     float32
	zOrder      int
	closed      bool
	callbackFired bool
	selectedBtn int
	OnClose     func(result DialogResult, text string)
}

func NewDialog(face7 font.Face, title, content string, input *InputField, buttons []string) *Dialog {
	m := font.Metrics{Width: 8, Height: 16, Ascent: 12}
	if face7 != nil {
		m = face7.Metrics()
	}
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	d := &Dialog{
		face:    face7,
		metrics: m,
		title:   title,
		content: content,
		input:   input,
		buttons: buttons,
		bgAlpha: 0.92,
		zOrder:  200,
	}
	if content != "" {
		d.contentRows = strings.Split(content, "\n")
	}
	return d
}

func (d *Dialog) SetGlyphProvider(gp render.GlyphProvider)      { d.gp = gp }
func (d *Dialog) SetGPUGlyphUploader(u render.GPUGlyphUploader) { d.uploader = u }
func (d *Dialog) ZOrder() int                                    { return d.zOrder }
func (d *Dialog) Close()                                         { d.closed = true }

func (d *Dialog) Result() DialogResult  { return d.result }
func (d *Dialog) InputField() *InputField { return d.input }
func (d *Dialog) Closed() bool           { return d.closed }

// CloseAndCallback 在移除 Dialog 前收集结果并触发 OnClose 回调。
// 幂等：多次调用只触发一次回调。
func (d *Dialog) CloseAndCallback() {
	if d.callbackFired {
		return
	}
	d.callbackFired = true
	var text string
	if d.input != nil {
		text = d.input.Text()
	}
	if d.OnClose != nil {
		d.OnClose(d.result, text)
	}
}

func (d *Dialog) SetFace(face font.Face) {
	d.face = face
	d.metrics = face.Metrics()
}

func (d *Dialog) HandleKey(ev platform.KeyEvent) {
	if ev.State != platform.KeyPress {
		return
	}
	if d.closed {
		return
	}
	if ev.Code == 1 {
		d.result = DialogCancel
		d.closed = true
		return
	}
	if ev.Code == 105 && len(d.buttons) > 1 {
		d.selectedBtn--
		if d.selectedBtn < 0 {
			d.selectedBtn = len(d.buttons) - 1
		}
		return
	}
	if ev.Code == 106 && len(d.buttons) > 1 {
		d.selectedBtn++
		if d.selectedBtn >= len(d.buttons) {
			d.selectedBtn = 0
		}
		return
	}
	if ev.Code == 28 {
		switch d.selectedBtn {
		case 0:
			d.result = DialogOK
		default:
			d.result = DialogCancel
		}
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
}

func (d *Dialog) CommitText(text string) {
	if d.input != nil {
		d.input.InsertText(text)
	}
}

func (d *Dialog) layout(width, height int) (bgX, bgY, bgW, bgH, textX, textY int) {
	cellW := d.metrics.Width
	cellH := d.metrics.Height
	borderW := 2 * cellW // 2-cell horizontal border
	borderH := cellH
	padX := 2 * cellW // 2-cell horizontal padding
	padY := cellH
	desiredW := 40 * cellW
	if desiredW > width*3/4 {
		desiredW = width * 3 / 4
	}
	rows := 0
	if d.title != "" {
		rows++ // title
		rows++ // separator
	}
	rows += len(d.contentRows)
	if d.input != nil {
		rows++
	}
	if len(d.buttons) > 0 {
		rows++ // gap row before buttons
		rows++ // buttons
	}
	textW := desiredW
	textH := rows * cellH
	bgW = textW + (padX+borderW)*2
	bgH = textH + (padY+borderH)*2
	bgX = (width - bgW) / 2
	bgY = (height - bgH) / 2
	if bgX < 0 {
		bgX = 0
	}
	if bgY < 0 {
		bgY = 0
	}
	textX = bgX + borderW + padX
	textY = bgY + borderH + padY
	return
}

func (d *Dialog) RenderCPU(buf []byte, stride, width, height int) {
	if d.metrics.Height <= 0 || d.bgAlpha <= 0 {
		return
	}
	bgX, bgY, bgW, bgH, textX, textY := d.layout(width, height)
	cellW := d.metrics.Width
	cellH := d.metrics.Height
	a := uint8(d.bgAlpha * 255)
	borderW := 2 * cellW
	borderH := cellH
	innerX := bgX + borderW
	innerY := bgY + borderH
	innerW := bgW - 2*borderW
	innerH := bgH - 2*borderH
	render.FillRectBlend(buf, stride, bgX, bgY, bgW, borderH, 55, 55, 68, a)
	render.FillRectBlend(buf, stride, bgX, bgY+bgH-borderH, bgW, borderH, 55, 55, 68, a)
	render.FillRectBlend(buf, stride, bgX, bgY+borderH, borderW, innerH, 55, 55, 68, a)
	render.FillRectBlend(buf, stride, bgX+bgW-borderW, bgY+borderH, borderW, innerH, 55, 55, 68, a)
	render.FillRectBlend(buf, stride, innerX, innerY, innerW, innerH, 30, 30, 30, a)
	if d.gp == nil {
		return
	}
	d.renderTextCPU(buf, stride, textX, textY, innerW-4*cellW, width, height)
}

func (d *Dialog) renderLineCPU(buf []byte, stride, x, y, w int, text string, fgR, fgG, fgB uint8) {
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

func (d *Dialog) renderTextCPU(buf []byte, stride, x, y, w, frameW, frameH int) {
	curY := y
	if d.title != "" {
		d.renderLineCPU(buf, stride, x, curY, w, d.title, 255, 255, 255)
		curY += d.metrics.Height
		curY += d.metrics.Height
	}
	for _, line := range d.contentRows {
		d.renderLineCPU(buf, stride, x, curY, w, line, 215, 215, 215)
		curY += d.metrics.Height
	}
	if d.input != nil {
		text := d.input.DisplayText()
		if text != "" {
			d.renderLineCPU(buf, stride, x, curY, w, text, 230, 230, 230)
		}
		curY += d.metrics.Height
	}
	if len(d.buttons) > 0 {
		curY += d.metrics.Height
		d.renderButtonsCPU(buf, stride, x, curY, w)
	}
}

func (d *Dialog) renderButtonsCPU(buf []byte, stride, x, y, w int) {
	cellW := d.metrics.Width
	indent := 3
	bracketLen := 4
	gap := 2
	totalW := 0
	for _, btn := range d.buttons {
		bw := runeutil.StringWidth(btn) + indent*2 + bracketLen
		totalW += bw * cellW
		totalW += gap * cellW
	}
	if totalW > 0 {
		totalW -= gap * cellW
	}
	btnX := x + (w-totalW)/2
	if btnX < x {
		btnX = x
	}
	for i, btn := range d.buttons {
		btnCells := runeutil.StringWidth(btn) + indent*2 + bracketLen
		btnW := btnCells * cellW
		selected := i == d.selectedBtn
		if selected {
			render.FillRectBlend(buf, stride, btnX, y, btnW, d.metrics.Height, 60, 120, 200, 255)
		} else {
			render.FillRectBlend(buf, stride, btnX, y, btnW, d.metrics.Height, 45, 45, 52, 200)
		}
		label := "[ " + btn + " ]"
		if selected {
			d.renderLineCPU(buf, stride, btnX+indent*cellW, y, btnW, label, 255, 255, 255)
		} else {
			d.renderLineCPU(buf, stride, btnX+indent*cellW, y, btnW, label, 165, 165, 175)
		}
		btnX += btnW + gap*cellW
	}
}

func (d *Dialog) renderLineGPU(instances *[]platform.CellInstance, x, y int, w int, text string, fgR, fgG, fgB, bgA float32, cellW, cellH float32) {
	xpos := 0
	for _, ch := range text {
		rw := 1
		if runeutil.IsWide(ch) {
			rw = 2
		}
		u0, v0, u1, v1, gw, gh, xoff, yoff, ok := d.uploader.OverlayUploadGlyph(ch)
		if ok {
			inst := platform.CellInstance{
				X:         float32(x) + float32(xpos)*cellW,
				Y:         float32(y),
				CellW:     float32(rw) * cellW,
				CellH:     cellH,
				FgR:       fgR,
				FgG:       fgG,
				FgB:       fgB,
				BgA:       bgA,
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
		xpos += rw
		if xpos*d.metrics.Width >= w {
			break
		}
	}
}

func (d *Dialog) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if d.metrics.Height <= 0 || d.metrics.Width <= 0 || d.bgAlpha <= 0 {
		return
	}
	bgX, bgY, bgW, bgH, textX, textY := d.layout(width, height)
	cellW := float32(d.metrics.Width)
	cellH := float32(d.metrics.Height)
	cols := bgW / d.metrics.Width
	rows := bgH / d.metrics.Height
	bgR := 30.0 / 255 * d.bgAlpha
	bgG := 30.0 / 255 * d.bgAlpha
	bgB := 30.0 / 255 * d.bgAlpha
	borderR := 55.0 / 255 * d.bgAlpha
	borderG := 55.0 / 255 * d.bgAlpha
	borderB := 68.0 / 255 * d.bgAlpha
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			r, g, b := bgR, bgG, bgB
			if row == 0 || row == rows-1 || col < 2 || col >= cols-2 {
				r, g, b = borderR, borderG, borderB
			}
			inst := platform.CellInstance{
				X:     float32(bgX) + float32(col)*cellW,
				Y:     float32(bgY) + float32(row)*cellH,
				CellW: cellW,
				CellH: cellH,
				BgR:   r,
				BgG:   g,
				BgB:   b,
				BgA:   d.bgAlpha,
			}
			*instances = append(*instances, inst)
		}
	}
	if d.uploader == nil {
		return
	}
	textW := bgW - 8*d.metrics.Width
	curY := textY
	if d.title != "" {
		d.renderLineGPU(instances, textX, curY, textW, d.title, 1.0, 1.0, 1.0, d.bgAlpha, cellW, cellH)
		curY += d.metrics.Height
		curY += d.metrics.Height
	}
	for _, line := range d.contentRows {
		d.renderLineGPU(instances, textX, curY, textW, line, 215.0/255, 215.0/255, 215.0/255, d.bgAlpha, cellW, cellH)
		curY += d.metrics.Height
	}
	if d.input != nil {
		text := d.input.DisplayText()
		if text != "" {
			d.renderLineGPU(instances, textX, curY, textW, text, 230.0/255, 230.0/255, 230.0/255, d.bgAlpha, cellW, cellH)
		}
		curY += d.metrics.Height
	}
	if len(d.buttons) > 0 {
		curY += d.metrics.Height
		d.renderButtonsGPU(instances, textX, curY, textW, cellW, cellH)
	}
}

func (d *Dialog) renderButtonsGPU(instances *[]platform.CellInstance, x, y, w int, cellW, cellH float32) {
	indent := 3
	bracketLen := 4
	gap := 2
	icellW := int(cellW)
	totalW := 0
	for _, btn := range d.buttons {
		bw := runeutil.StringWidth(btn) + indent*2 + bracketLen
		totalW += (bw + gap) * icellW
	}
	if totalW > 0 {
		totalW -= gap * icellW
	}
	btnX := x + (w-totalW)/2
	if btnX < x {
		btnX = x
	}
	for i, btn := range d.buttons {
		btnCells := runeutil.StringWidth(btn) + indent*2 + bracketLen
		btnW := btnCells * icellW
		selected := i == d.selectedBtn
		var bgR2, bgG2, bgB2, bgA2, fgR, fgG, fgB float32
		if selected {
			bgR2 = 60.0 / 255
			bgG2 = 120.0 / 255
			bgB2 = 200.0 / 255
			bgA2 = 1.0
			fgR, fgG, fgB = 1.0, 1.0, 1.0
		} else {
			bgR2 = 45.0 / 255
			bgG2 = 45.0 / 255
			bgB2 = 52.0 / 255
			bgA2 = 0.8
			fgR, fgG, fgB = 165.0/255, 165.0/255, 175.0/255
		}
		for col := 0; col < btnCells; col++ {
			inst := platform.CellInstance{
				X:     float32(btnX) + float32(col)*cellW,
				Y:     float32(y),
				CellW: cellW,
				CellH: cellH,
				BgR:   bgR2,
				BgG:   bgG2,
				BgB:   bgB2,
				BgA:   bgA2,
			}
			*instances = append(*instances, inst)
		}
		label := "[ " + btn + " ]"
		d.renderLineGPU(instances, btnX+indent*icellW, y, btnW, label, fgR, fgG, fgB, bgA2, cellW, cellH)
		btnX += btnW + gap*icellW
	}
}
