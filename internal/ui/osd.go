package ui

import (
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/runeutil"
)

type Tab struct {
	Title  string
	Active bool
}

// 图元类型常量，与 plugins.PrimText/PrimRect 对应。
const (
	primText = 0
	primRect = 1
)

var (
	barBg      = [3]uint8{24, 24, 24}
	activeBg   = [3]uint8{56, 56, 56}
	inactiveBg = [3]uint8{32, 32, 32}
	activeFg   = [3]uint8{230, 230, 230}
	inactiveFg = [3]uint8{150, 150, 150}
)

// OSD 顶部标签栏默认开启；底/左/右边面板由插件系统
// vistty.ui.enable / on_render 驱动，无需配置项。
type OSD struct {
	face     font.Face
	metrics  font.Metrics
	tabs     []Tab
	active   int
	gp       render.GlyphProvider
	uploader render.GPUGlyphUploader

	pluginPanels map[string][]PanelPrimitive
	panelLines   map[string]int
}

// PanelPrimitive 是插件面板图元的 UI 包本地定义，
// 与 plugins.Primitive 字段一致。由 session 包做转换，
// 避免 ui → plugins 的循环依赖。
type PanelPrimitive struct {
	Kind int // 0=text, 1=rect
	X, Y int
	W, H int
	Text string
	Fg   [4]uint8
	Bg   [4]uint8
	Bold bool
}

type osdCell struct {
	x             int
	r             rune
	bgR, bgG, bgB uint8
	fgR, fgG, fgB uint8
}

func layoutTabs(tabs []Tab, active, cellW, width int) []osdCell {
	if cellW <= 0 || width <= 0 {
		return nil
	}
	var cells []osdCell
	x := 0
loop:
	for i := range tabs {
		var tabBg, tabFg [3]uint8
		if i == active {
			tabBg = activeBg
			tabFg = activeFg
		} else {
			tabBg = inactiveBg
			tabFg = inactiveFg
		}
		if x+cellW > width {
			break loop
		}
		cells = append(cells, osdCell{x: x, r: 0, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
		x += cellW
		for _, ch := range tabs[i].Title {
			if x+cellW > width {
				break loop
			}
			cells = append(cells, osdCell{x: x, r: ch, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
			x += cellW
		}
		if x+cellW > width {
			break loop
		}
		cells = append(cells, osdCell{x: x, r: 0, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
		x += cellW
	}
	for x+cellW <= width {
		cells = append(cells, osdCell{x: x, r: 0, bgR: barBg[0], bgG: barBg[1], bgB: barBg[2], fgR: inactiveFg[0], fgG: inactiveFg[1], fgB: inactiveFg[2]})
		x += cellW
	}
	return cells
}

func NewOSD(face font.Face) *OSD {
	return &OSD{
		face:    face,
		metrics: face.Metrics(),
	}
}

func (o *OSD) Insets() (top, bottom, left, right int) {
	// 顶部标签栏默认 1 行；插件 panelLines 可扩大（取 max）。
	if o.metrics.Height > 0 {
		top = o.metrics.Height
	}
	// 底/左/右边面板完全由插件系统 panelLines 驱动（取 max 合并）。
	if o.panelLines != nil {
		if lines, ok := o.panelLines["top"]; ok && lines > 0 && o.metrics.Height > 0 {
			h := lines * o.metrics.Height
			if h > top {
				top = h
			}
		}
		if lines, ok := o.panelLines["bottom"]; ok && lines > 0 && o.metrics.Height > 0 {
			h := lines * o.metrics.Height
			if h > bottom {
				bottom = h
			}
		}
		if lines, ok := o.panelLines["left"]; ok && lines > 0 && o.metrics.Width > 0 {
			w := lines * o.metrics.Width
			if w > left {
				left = w
			}
		}
		if lines, ok := o.panelLines["right"]; ok && lines > 0 && o.metrics.Width > 0 {
			w := lines * o.metrics.Width
			if w > right {
				right = w
			}
		}
	}
	return
}

func (o *OSD) SetTabs(tabs []Tab, active int) {
	o.tabs = tabs
	o.active = active
}

func (o *OSD) SetGlyphProvider(gp render.GlyphProvider) {
	o.gp = gp
}

func (o *OSD) SetGPUGlyphUploader(u render.GPUGlyphUploader) {
	o.uploader = u
}

func (o *OSD) UpdateFace(face font.Face) {
	o.face = face
	o.metrics = face.Metrics()
}

// SetPluginPanel 更新插件面板图元缓存。side 为 "top"/"bottom"/"left"/"right"。
// primitives 为 nil 时清除该边。由 Master.renderPlugins 在主渲染线程调用。
func (o *OSD) SetPluginPanel(side string, primitives []PanelPrimitive) {
	if o.pluginPanels == nil {
		o.pluginPanels = make(map[string][]PanelPrimitive)
	}
	o.pluginPanels[side] = primitives
}

// SetPanelLines 设置插件声明的面板行数。side → lines。
// Insets() 读取本字段合并 cfg 与插件声明的面板尺寸（取 max）。
func (o *OSD) SetPanelLines(lines map[string]int) {
	o.panelLines = lines
}

func (o *OSD) RenderCPU(buf []byte, stride, width, height int) {
	if o.metrics.Height <= 0 || len(buf) == 0 {
		return
	}
	cells := layoutTabs(o.tabs, o.active, o.metrics.Width, width)
	for _, c := range cells {
		render.FillRect(buf, stride, c.x, 0, o.metrics.Width, o.metrics.Height, c.bgR, c.bgG, c.bgB)
		if c.r != 0 && o.gp != nil {
			g := o.gp.OverlayGlyph(c.r)
			if g == nil {
				continue
			}
			gx := c.x + g.XOffset
			gy := 0 + o.metrics.Ascent + g.YOffset
			render.BlendGlyph(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, c.fgR, c.fgG, c.fgB)
		}
	}
	o.renderPluginPanelsCPU(buf, stride, width, height)
}

// renderPluginPanelsCPU 在帧缓冲的 bottom/left/right 边缘绘制插件面板图元。
// top 面板暂不支持（与标签栏区域冲突）。p.X/p.Y/p.W/p.H 为 cell 单位，
// 乘以 metrics.Width/Height 转像素。xOff/yOff 为该边缘面板的像素起点偏移。
func (o *OSD) renderPluginPanelsCPU(buf []byte, stride, width, height int) {
	if o.pluginPanels == nil || o.metrics.Height <= 0 {
		return
	}
	_, bottom, left, right := o.Insets()
	if prims, ok := o.pluginPanels["bottom"]; ok && bottom > 0 {
		yOff := height - bottom
		clipX := left
		clipY := yOff
		clipW := width - left - right
		clipH := bottom
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, left, yOff, clipX, clipY, clipW, clipH)
		}
	}
	if prims, ok := o.pluginPanels["left"]; ok && left > 0 {
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, 0, 0, 0, 0, left, height)
		}
	}
	if prims, ok := o.pluginPanels["right"]; ok && right > 0 {
		xOff := width - right
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, xOff, 0, xOff, 0, right, height)
		}
	}
}

func (o *OSD) drawPrimitiveCPU(buf []byte, stride, frameW, frameH int, p PanelPrimitive, xOff, yOff, clipX, clipY, clipW, clipH int) {
	if o.metrics.Width <= 0 || o.metrics.Height <= 0 {
		return
	}
	absX := xOff + p.X*o.metrics.Width
	absY := yOff + p.Y*o.metrics.Height
	if p.Kind == primRect {
		w := p.W * o.metrics.Width
		h := p.H * o.metrics.Height
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
		if ch == 0 || o.gp == nil {
			xpos++
			continue
		}
		g := o.gp.OverlayGlyph(ch)
		if g == nil {
			xpos++
			continue
		}
		rw := 1
		if runeutil.IsWide(ch) {
			rw = 2
		}
		cellX := absX + xpos*o.metrics.Width
		if cellX >= clipX+clipW {
			break
		}
		gx := cellX + g.XOffset
		gy := absY + o.metrics.Ascent + g.YOffset
		render.BlendGlyphAlpha(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, p.Fg[0], p.Fg[1], p.Fg[2], p.Fg[3])
		xpos += rw
	}
}

func (o *OSD) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if o.metrics.Height > 0 {
		cells := layoutTabs(o.tabs, o.active, o.metrics.Width, width)
		cellW := float32(o.metrics.Width)
		cellH := float32(o.metrics.Height)
		for _, c := range cells {
			inst := platform.CellInstance{
				X:         float32(c.x),
				Y:         0,
				CellW:     cellW,
				CellH:     cellH,
				FgR:       float32(c.fgR) / 255,
				FgG:       float32(c.fgG) / 255,
				FgB:       float32(c.fgB) / 255,
				BgR:       float32(c.bgR) / 255,
				BgG:       float32(c.bgG) / 255,
				BgB:       float32(c.bgB) / 255,
				HasBg:     1,
				GlyphOffY: 0,
			}
			if c.r != 0 && o.uploader != nil {
				u0, v0, u1, v1, gw, gh, xoff, yoff, ok := o.uploader.OverlayUploadGlyph(c.r)
				if ok {
					inst.GlyphU0 = u0
					inst.V0 = v0
					inst.GlyphU1 = u1
					inst.V1 = v1
					inst.GlyphOffX = float32(xoff)
					inst.GlyphOffY = float32(o.metrics.Ascent + yoff)
					inst.GlyphW = float32(gw)
					inst.GlyphH = float32(gh)
				}
			}
			*instances = append(*instances, inst)
		}
		o.renderPluginPanelsGPU(instances, width, height)
	}
}

// renderPluginPanelsGPU 将插件面板图元转为 CellInstance 追加到 instances。
// top 面板暂不支持（与标签栏区域冲突）。
func (o *OSD) renderPluginPanelsGPU(instances *[]platform.CellInstance, width, height int) {
	if o.pluginPanels == nil || o.metrics.Height <= 0 || o.metrics.Width <= 0 {
		return
	}
	_, bottom, left, right := o.Insets()
	cellW := float32(o.metrics.Width)
	cellH := float32(o.metrics.Height)

	drawPrimGPU := func(p PanelPrimitive, xOff, yOff, clipX, clipY, clipW, clipH float32) {
		bgA := float32(p.Bg[3]) / 255
		fgA := float32(p.Fg[3]) / 255
		if p.Kind == primRect {
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
						BgR:   float32(p.Bg[0]) / 255 * bgA,
						BgG:   float32(p.Bg[1]) / 255 * bgA,
						BgB:   float32(p.Bg[2]) / 255 * bgA,
						HasBg: bgA,
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
				BgR:   float32(p.Bg[0]) / 255 * bgA,
				BgG:   float32(p.Bg[1]) / 255 * bgA,
				BgB:   float32(p.Bg[2]) / 255 * bgA,
				HasBg: bgA,
			}
			if ch != 0 && o.uploader != nil {
				u0, v0, u1, v1, gw, gh, xoff, yoff, ok := o.uploader.OverlayUploadGlyph(ch)
				if ok {
					inst.GlyphU0 = u0
					inst.V0 = v0
					inst.GlyphU1 = u1
					inst.V1 = v1
					inst.GlyphOffX = float32(xoff)
					inst.GlyphOffY = float32(o.metrics.Ascent + yoff)
					inst.GlyphW = float32(gw)
					inst.GlyphH = float32(gh)
				}
			}
			*instances = append(*instances, inst)
			xpos += rw
		}
	}

	if prims, ok := o.pluginPanels["bottom"]; ok && bottom > 0 {
		yOff := float32(height - bottom)
		xOff := float32(left)
		clipX := xOff
		clipY := yOff
		clipW := float32(width - left - right)
		clipH := float32(bottom)
		for _, p := range prims {
			drawPrimGPU(p, xOff, yOff, clipX, clipY, clipW, clipH)
		}
	}
	if prims, ok := o.pluginPanels["left"]; ok && left > 0 {
		for _, p := range prims {
			drawPrimGPU(p, 0, 0, 0, 0, float32(left), float32(height))
		}
	}
	if prims, ok := o.pluginPanels["right"]; ok && right > 0 {
		xOff := float32(width - right)
		for _, p := range prims {
			drawPrimGPU(p, xOff, 0, xOff, 0, float32(right), float32(height))
		}
	}
}
