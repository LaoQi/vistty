package ui

import (
	"image"

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

const csdBtnCount = 3

type CsdButton int

const (
	CsdBtnMinimize CsdButton = iota
	CsdBtnMaximize
	CsdBtnClose
)

// OSD 顶部标签栏默认开启；底/左/右边面板由插件系统
// vistty.ui.enable / on_render 驱动，无需配置项。
type OSD struct {
	face     font.Face
	metrics  font.Metrics
	tabs     []Tab
	active   int
	scroll   int // 标签栏水平滚动偏移（像素，对齐 tab 边界）
	gp       render.GlyphProvider
	uploader render.GPUGlyphUploader

	pluginPanels map[string][]PanelPrimitive
	panelLines   map[string]int
	csdMode      bool
	theme        OSDTheme
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
	w             int // 占用列数（1 或 2，双宽 CJK 为 2）
	r             rune
	bgR, bgG, bgB uint8
	fgR, fgG, fgB uint8
}

// maxTabTitleCols 限制单个标签标题的显示列宽（含省略号）。
// 超出时截断并以 …（单宽）结尾，实际字符最多 maxTabTitleCols-1 列。
const maxTabTitleCols = 16

// truncateTabTitle 将标题截断到 maxTabTitleCols 列宽，超出加省略号。
func truncateTabTitle(title string) string {
	totalW := runeutil.StringWidth(title)
	if totalW <= maxTabTitleCols {
		return title
	}
	limit := maxTabTitleCols - 1
	if limit <= 0 {
		return "…"
	}
	out := make([]rune, 0, limit+1)
	w := 0
	for _, ch := range title {
		rw := runeutil.RuneWidth(ch)
		if w+rw > limit {
			break
		}
		out = append(out, ch)
		w += rw
	}
	out = append(out, '…')
	return string(out)
}

// layoutTabs 布局顶部标签栏。scroll 为当前水平滚动偏移（像素，对齐 tab 边界），
// 返回可见 cells 与调整后的滚动偏移。
// 滚动策略：active tab 完全可见时保持偏移；否则滚动使 active 可见
// （左边超出→左对齐；右边超出→靠右对齐到最近的 tab 边界）。
func (o *OSD) layoutTabs(tabs []Tab, active, cellW, width, csdWidth, scroll int) ([]osdCell, int) {
	if cellW <= 0 || width <= 0 {
		return nil, scroll
	}
	tabWidth := width - csdWidth
	if tabWidth <= 0 {
		return nil, scroll
	}

	// 1. 截断标题 + 计算每个 tab 占用列宽（pad + title + pad）
	type tinfo struct {
		title string
		cols  int
	}
	infos := make([]tinfo, len(tabs))
	for i := range tabs {
		t := truncateTabTitle(tabs[i].Title)
		infos[i].title = t
		infos[i].cols = 1 + runeutil.StringWidth(t) + 1
	}

	// 2. 全局起始 x
	tabStarts := make([]int, len(tabs))
	totalW := 0
	for i := range infos {
		tabStarts[i] = totalW
		totalW += infos[i].cols * cellW
	}

	// 3. 调整 scroll 使 active 可见
	if totalW <= tabWidth {
		scroll = 0
	} else if active >= 0 && active < len(tabs) {
		aStart := tabStarts[active]
		aEnd := aStart + infos[active].cols*cellW
		switch {
		case aStart >= scroll && aEnd <= scroll+tabWidth:
			// active 可见，保持
		case aStart < scroll:
			scroll = aStart
		default: // aEnd > scroll+tabWidth，靠右对齐到最近 tab 边界
			target := aEnd - tabWidth
			best := -1
			for _, ts := range tabStarts {
				if ts <= target && ts > best {
					best = ts
				}
			}
			if best >= 0 {
				scroll = best
			} else {
				scroll = 0
			}
		}
	}
	if scroll < 0 {
		scroll = 0
	}

	// 4. 生成窗口 [scroll, scroll+tabWidth] 内的 cells
	viewEnd := scroll + tabWidth
	var cells []osdCell
	endX := 0
	for i := range tabs {
		tStart := tabStarts[i]
		tEnd := tStart + infos[i].cols*cellW
		if tEnd <= scroll || tStart >= viewEnd {
			continue
		}
		var tabBg, tabFg [3]uint8
		if i == active {
			tabBg = o.theme.ActiveBg
			tabFg = o.theme.ActiveFg
		} else {
			tabBg = o.theme.InactiveBg
			tabFg = o.theme.InactiveFg
		}
		rx := tStart - scroll
		if rx >= tabWidth {
			break
		}
		// pad before
		cells = append(cells, osdCell{x: rx, w: 1, r: 0, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
		rx += cellW
		for _, ch := range infos[i].title {
			if rx >= tabWidth {
				break
			}
			rw := 1
			if runeutil.IsWide(ch) {
				rw = 2
			}
			cells = append(cells, osdCell{x: rx, w: rw, r: ch, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
			rx += rw * cellW
		}
		if rx < tabWidth {
			cells = append(cells, osdCell{x: rx, w: 1, r: 0, bgR: tabBg[0], bgG: tabBg[1], bgB: tabBg[2], fgR: tabFg[0], fgG: tabFg[1], fgB: tabFg[2]})
			rx += cellW
		}
		endX = rx
	}
	// 5. 尾部 bar 填充
	for endX+cellW <= tabWidth {
		cells = append(cells, osdCell{x: endX, w: 1, r: 0, bgR: o.theme.BarBg[0], bgG: o.theme.BarBg[1], bgB: o.theme.BarBg[2], fgR: o.theme.InactiveFg[0], fgG: o.theme.InactiveFg[1], fgB: o.theme.InactiveFg[2]})
		endX += cellW
	}
	return cells, scroll
}

func NewOSD(face font.Face, osdTheme OSDTheme) *OSD {
	if osdTheme == (OSDTheme{}) {
		osdTheme = DefaultOSDTheme
	}
	return &OSD{
		face:    face,
		metrics: face.Metrics(),
		theme:   osdTheme,
	}
}

func (o *OSD) SetTheme(t OSDTheme) {
	o.theme = t
}

func (o *OSD) SetCSDMode(csd bool) {
	o.csdMode = csd
}

func (o *OSD) CsdEnabled() bool {
	return o.csdMode
}

func (o *OSD) csdButtonsWidth() int {
	if !o.csdMode || o.metrics.Width <= 0 {
		return 0
	}
	return csdBtnCount * o.metrics.Width
}

func (o *OSD) layoutCsdButtons(cellW, width int) []osdCell {
	if cellW <= 0 || width <= 0 {
		return nil
	}
	syms := []rune{'─', '□', '✕'}
	bgs := [3][3]uint8{o.theme.CsdBtnBg, o.theme.CsdBtnBg, o.theme.CsdCloseBg}
	fgs := [3][3]uint8{o.theme.CsdBtnFg, o.theme.CsdBtnFg, o.theme.CsdBtnFg}
	var cells []osdCell
	for i := 0; i < csdBtnCount; i++ {
		x := width - (csdBtnCount-i)*cellW
		cells = append(cells, osdCell{
			x: x, w: 1, r: syms[i],
			bgR: bgs[i][0], bgG: bgs[i][1], bgB: bgs[i][2],
			fgR: fgs[i][0], fgG: fgs[i][1], fgB: fgs[i][2],
		})
	}
	return cells
}

func (o *OSD) CsdButtonRects(width int) [csdBtnCount]image.Rectangle {
	var rects [csdBtnCount]image.Rectangle
	if !o.csdMode || o.metrics.Width <= 0 || o.metrics.Height <= 0 {
		return rects
	}
	for i := 0; i < csdBtnCount; i++ {
		x := width - (csdBtnCount-i)*o.metrics.Width
		rects[i] = image.Rect(x, 0, x+o.metrics.Width, o.metrics.Height)
	}
	return rects
}

type TabBarHit int

const (
	TabBarMiss TabBarHit = iota
	TabBarArea
	TabBarCsdMin
	TabBarCsdMax
	TabBarCsdClose
)

func (o *OSD) HitTestTabBar(x, y, width int) TabBarHit {
	if o.metrics.Height <= 0 || y >= o.metrics.Height {
		return TabBarMiss
	}
	if o.csdMode {
		rects := o.CsdButtonRects(width)
		for i, r := range rects {
			if r.Min.X <= x && x < r.Max.X && r.Min.Y <= y && y < r.Max.Y {
				switch i {
				case 0:
					return TabBarCsdMin
				case 1:
					return TabBarCsdMax
				case 2:
					return TabBarCsdClose
				}
			}
		}
	}
	return TabBarArea
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
	csdW := o.csdButtonsWidth()
	cells, sc := o.layoutTabs(o.tabs, o.active, o.metrics.Width, width, csdW, o.scroll)
	o.scroll = sc
	for _, c := range cells {
		render.FillRect(buf, stride, c.x, 0, c.w*o.metrics.Width, o.metrics.Height, c.bgR, c.bgG, c.bgB)
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
	if csdW > 0 {
		csdCells := o.layoutCsdButtons(o.metrics.Width, width)
		for _, c := range csdCells {
			render.FillRect(buf, stride, c.x, 0, c.w*o.metrics.Width, o.metrics.Height, c.bgR, c.bgG, c.bgB)
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
	if o.metrics.Height <= 0 {
		return
	}
	csdW := o.csdButtonsWidth()
	cells, sc := o.layoutTabs(o.tabs, o.active, o.metrics.Width, width, csdW, o.scroll)
	o.scroll = sc
	cellW := float32(o.metrics.Width)
	cellH := float32(o.metrics.Height)
	for _, c := range cells {
		inst := platform.CellInstance{
			X:         float32(c.x),
			Y:         0,
			CellW:     float32(c.w) * cellW,
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
	if csdW > 0 {
		csdCells := o.layoutCsdButtons(o.metrics.Width, width)
		for _, c := range csdCells {
			inst := platform.CellInstance{
				X:         float32(c.x),
				Y:         0,
				CellW:     float32(c.w) * cellW,
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
