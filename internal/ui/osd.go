package ui

import (
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
)

type SideConfig struct {
	Enabled bool
	Lines   int
}

type Config struct {
	Top, Bottom, Left, Right SideConfig
}

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

type OSD struct {
	cfg      Config
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
	Fg   [3]uint8
	Bg   [3]uint8
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

func NewOSD(cfg Config, face font.Face) *OSD {
	return &OSD{
		cfg:     cfg,
		face:    face,
		metrics: face.Metrics(),
	}
}

func (o *OSD) Insets() (top, bottom, left, right int) {
	if o.cfg.Top.Enabled && o.metrics.Height > 0 {
		lines := o.cfg.Top.Lines
		if lines <= 0 {
			lines = 1
		}
		top = lines * o.metrics.Height
	}
	if o.cfg.Bottom.Enabled && o.metrics.Height > 0 {
		lines := o.cfg.Bottom.Lines
		if lines <= 0 {
			lines = 1
		}
		bottom = lines * o.metrics.Height
	}
	if o.cfg.Left.Enabled && o.metrics.Width > 0 {
		lines := o.cfg.Left.Lines
		if lines <= 0 {
			lines = 1
		}
		left = lines * o.metrics.Width
	}
	if o.cfg.Right.Enabled && o.metrics.Width > 0 {
		lines := o.cfg.Right.Lines
		if lines <= 0 {
			lines = 1
		}
		right = lines * o.metrics.Width
	}
	// 合并插件声明的 panelLines，取 max。
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
	if o.cfg.Top.Enabled {
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
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, left, yOff)
		}
	}
	if prims, ok := o.pluginPanels["left"]; ok && left > 0 {
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, 0, 0)
		}
	}
	if prims, ok := o.pluginPanels["right"]; ok && right > 0 {
		xOff := width - right
		for _, p := range prims {
			o.drawPrimitiveCPU(buf, stride, width, height, p, xOff, 0)
		}
	}
}

func (o *OSD) drawPrimitiveCPU(buf []byte, stride, frameW, frameH int, p PanelPrimitive, xOff, yOff int) {
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
		render.FillRect(buf, stride, absX, absY, w, h, p.Bg[0], p.Bg[1], p.Bg[2])
		return
	}
	// text
	for i, ch := range p.Text {
		if ch == 0 || o.gp == nil {
			continue
		}
		g := o.gp.OverlayGlyph(ch)
		if g == nil {
			continue
		}
		gx := absX + i*o.metrics.Width + g.XOffset
		gy := absY + o.metrics.Ascent + g.YOffset
		render.BlendGlyph(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, p.Fg[0], p.Fg[1], p.Fg[2])
	}
}

func (o *OSD) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if o.metrics.Height > 0 {
		if o.cfg.Top.Enabled {
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

	drawPrimGPU := func(p PanelPrimitive, xOff, yOff float32) {
		if p.Kind == primRect {
			for j := 0; j < p.H; j++ {
				for i := 0; i < p.W; i++ {
					inst := platform.CellInstance{
						X:     xOff + float32(p.X+i)*cellW,
						Y:     yOff + float32(p.Y+j)*cellH,
						CellW: cellW, CellH: cellH,
						BgR:   float32(p.Bg[0]) / 255,
						BgG:   float32(p.Bg[1]) / 255,
						BgB:   float32(p.Bg[2]) / 255,
						HasBg: 1,
					}
					*instances = append(*instances, inst)
				}
			}
			return
		}
		// text：每个字符一个 cell，背景填充 + 前景字形
		for i, ch := range p.Text {
			inst := platform.CellInstance{
				X:     xOff + float32(p.X+i)*cellW,
				Y:     yOff + float32(p.Y)*cellH,
				CellW: cellW, CellH: cellH,
				FgR:   float32(p.Fg[0]) / 255,
				FgG:   float32(p.Fg[1]) / 255,
				FgB:   float32(p.Fg[2]) / 255,
				BgR:   float32(p.Bg[0]) / 255,
				BgG:   float32(p.Bg[1]) / 255,
				BgB:   float32(p.Bg[2]) / 255,
				HasBg: 1,
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
		}
	}

	if prims, ok := o.pluginPanels["bottom"]; ok && bottom > 0 {
		yOff := float32(height - bottom)
		xOff := float32(left)
		for _, p := range prims {
			drawPrimGPU(p, xOff, yOff)
		}
	}
	if prims, ok := o.pluginPanels["left"]; ok && left > 0 {
		for _, p := range prims {
			drawPrimGPU(p, 0, 0)
		}
	}
	if prims, ok := o.pluginPanels["right"]; ok && right > 0 {
		xOff := float32(width - right)
		for _, p := range prims {
			drawPrimGPU(p, xOff, 0)
		}
	}
}
