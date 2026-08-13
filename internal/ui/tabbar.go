package ui

import (
	"image"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/runeutil"
)

type Tab struct {
	Title  string
	Active bool
}

type CsdButton int

const (
	CsdBtnMinimize CsdButton = iota
	CsdBtnMaximize
	CsdBtnClose
)

const csdBtnCount = 3

// csdBtnCellSpan 是每个 CSD 按钮占用的 cell 宽度（4 cell @14pt ≈ 40px），
// 随字号缩放自适应，避免 1 cell 过窄导致点击目标小、符号拥挤。
const csdBtnCellSpan = 4

type TabBarHit int

const (
	TabBarMiss TabBarHit = iota
	TabBarArea
	TabBarCsdMin
	TabBarCsdMax
	TabBarCsdClose
)

const maxTabTitleCols = 16

type osdCell struct {
	x             int
	w             int
	r             rune
	bgR, bgG, bgB uint8
	fgR, fgG, fgB uint8
}

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

type TabBarTheme struct {
	BarBg      [3]uint8
	ActiveBg   [3]uint8
	InactiveBg [3]uint8
	ActiveFg   [3]uint8
	InactiveFg [3]uint8
	CsdBtnBg   [3]uint8
	CsdCloseBg [3]uint8
	CsdBtnFg   [3]uint8
}

var DefaultTabBarTheme = TabBarTheme{
	BarBg:      [3]uint8{24, 24, 24},
	ActiveBg:   [3]uint8{56, 56, 56},
	InactiveBg: [3]uint8{32, 32, 32},
	ActiveFg:   [3]uint8{230, 230, 230},
	InactiveFg: [3]uint8{150, 150, 150},
	CsdBtnBg:   [3]uint8{40, 40, 40},
	CsdCloseBg: [3]uint8{200, 50, 50},
	CsdBtnFg:   [3]uint8{200, 200, 200},
}

type TabBar struct {
	face     font.Face
	metrics  font.Metrics
	tabs     []Tab
	active   int
	scroll   int
	csdMode  bool
	theme    TabBarTheme
	gp       render.GlyphProvider
	uploader render.GPUGlyphUploader
}

func NewTabBar(face font.Face, theme TabBarTheme) *TabBar {
	if theme == (TabBarTheme{}) {
		theme = DefaultTabBarTheme
	}
	return &TabBar{
		face:    face,
		metrics: face.Metrics(),
		theme:   theme,
	}
}

func (t *TabBar) SetTheme(theme TabBarTheme) {
	t.theme = theme
}

func (t *TabBar) SetCSDMode(csd bool) {
	t.csdMode = csd
}

func (t *TabBar) CsdEnabled() bool {
	return t.csdMode
}

func (t *TabBar) csdButtonsWidth() int {
	if !t.csdMode || t.metrics.Width <= 0 {
		return 0
	}
	return csdBtnCount * csdBtnCellSpan * t.metrics.Width
}

func (t *TabBar) layoutCsdButtons(cellW, width int) []osdCell {
	if cellW <= 0 || width <= 0 {
		return nil
	}
	syms := []rune{font.CsdBtnMinRune, font.CsdBtnMaxRune, font.CsdBtnCloseRune}
	btnW := csdBtnCellSpan * cellW
	bgs := [3][3]uint8{t.theme.CsdBtnBg, t.theme.CsdBtnBg, t.theme.CsdCloseBg}
	fgs := [3][3]uint8{t.theme.CsdBtnFg, t.theme.CsdBtnFg, t.theme.CsdBtnFg}
	var cells []osdCell
	for i := 0; i < csdBtnCount; i++ {
		x := width - (csdBtnCount-i)*btnW
		cells = append(cells, osdCell{
			x: x, w: csdBtnCellSpan, r: syms[i],
			bgR: bgs[i][0], bgG: bgs[i][1], bgB: bgs[i][2],
			fgR: fgs[i][0], fgG: fgs[i][1], fgB: fgs[i][2],
		})
	}
	return cells
}

func (t *TabBar) CsdButtonRects(width int) [csdBtnCount]image.Rectangle {
	var rects [csdBtnCount]image.Rectangle
	if !t.csdMode || t.metrics.Width <= 0 || t.metrics.Height <= 0 {
		return rects
	}
	btnW := csdBtnCellSpan * t.metrics.Width
	for i := 0; i < csdBtnCount; i++ {
		x := width - (csdBtnCount-i)*btnW
		rects[i] = image.Rect(x, 0, x+btnW, t.metrics.Height)
	}
	return rects
}

func (t *TabBar) HitTestTabBar(x, y, width int) TabBarHit {
	if t.metrics.Height <= 0 || y >= t.metrics.Height {
		return TabBarMiss
	}
	if t.csdMode {
		rects := t.CsdButtonRects(width)
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

func (t *TabBar) Insets() (top, bottom, left, right int) {
	if t.metrics.Height > 0 {
		top = t.metrics.Height
	}
	return
}

func (t *TabBar) SetTabs(tabs []Tab, active int) {
	t.tabs = tabs
	t.active = active
}

func (t *TabBar) SetGlyphProvider(gp render.GlyphProvider) {
	t.gp = gp
}

func (t *TabBar) SetGPUGlyphUploader(u render.GPUGlyphUploader) {
	t.uploader = u
}

func (t *TabBar) UpdateFace(face font.Face) {
	t.face = face
	t.metrics = face.Metrics()
}

func (t *TabBar) layoutTabs(tabs []Tab, active, cellW, width, csdWidth, scroll int) ([]osdCell, int) {
	if cellW <= 0 || width <= 0 {
		return nil, scroll
	}
	tabWidth := width - csdWidth
	if tabWidth <= 0 {
		return nil, scroll
	}

	type tinfo struct {
		title string
		cols  int
	}
	infos := make([]tinfo, len(tabs))
	for i := range tabs {
		tr := truncateTabTitle(tabs[i].Title)
		infos[i].title = tr
		infos[i].cols = 1 + runeutil.StringWidth(tr) + 1
	}

	tabStarts := make([]int, len(tabs))
	totalW := 0
	for i := range infos {
		tabStarts[i] = totalW
		totalW += infos[i].cols * cellW
	}

	if totalW <= tabWidth {
		scroll = 0
	} else if active >= 0 && active < len(tabs) {
		aStart := tabStarts[active]
		aEnd := aStart + infos[active].cols*cellW
		switch {
		case aStart >= scroll && aEnd <= scroll+tabWidth:
		case aStart < scroll:
			scroll = aStart
		default:
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
			tabBg = t.theme.ActiveBg
			tabFg = t.theme.ActiveFg
		} else {
			tabBg = t.theme.InactiveBg
			tabFg = t.theme.InactiveFg
		}
		rx := tStart - scroll
		if rx >= tabWidth {
			break
		}
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
	for endX+cellW <= tabWidth {
		cells = append(cells, osdCell{x: endX, w: 1, r: 0, bgR: t.theme.BarBg[0], bgG: t.theme.BarBg[1], bgB: t.theme.BarBg[2], fgR: t.theme.InactiveFg[0], fgG: t.theme.InactiveFg[1], fgB: t.theme.InactiveFg[2]})
		endX += cellW
	}
	return cells, scroll
}

func (t *TabBar) RenderCPU(buf []byte, stride, width, height int) {
	if t.metrics.Height <= 0 || len(buf) == 0 {
		return
	}
	csdW := t.csdButtonsWidth()
	cells, sc := t.layoutTabs(t.tabs, t.active, t.metrics.Width, width, csdW, t.scroll)
	t.scroll = sc
	for _, c := range cells {
		render.FillRect(buf, stride, c.x, 0, c.w*t.metrics.Width, t.metrics.Height, c.bgR, c.bgG, c.bgB)
		if c.r != 0 && t.gp != nil {
			g := t.gp.OverlayGlyph(c.r)
			if g == nil {
				continue
			}
			gx := c.x + g.XOffset
			gy := 0 + t.metrics.Ascent + g.YOffset
			render.BlendGlyph(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, c.fgR, c.fgG, c.fgB)
		}
	}
	if csdW > 0 {
		csdCells := t.layoutCsdButtons(t.metrics.Width, width)
		for _, c := range csdCells {
			render.FillRect(buf, stride, c.x, 0, c.w*t.metrics.Width, t.metrics.Height, c.bgR, c.bgG, c.bgB)
			if c.r != 0 && t.gp != nil {
				g := t.gp.OverlayGlyph(c.r)
				if g == nil {
					continue
				}
				gx := c.x + (c.w*t.metrics.Width-g.Width)/2
				gy := 0 + t.metrics.Ascent + g.YOffset
				render.BlendGlyph(buf, stride, gx, gy, g.Bitmap, g.Width, g.Height, c.fgR, c.fgG, c.fgB)
			}
		}
	}
}

func (t *TabBar) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if t.metrics.Height <= 0 {
		return
	}
	csdW := t.csdButtonsWidth()
	cells, sc := t.layoutTabs(t.tabs, t.active, t.metrics.Width, width, csdW, t.scroll)
	t.scroll = sc
	cellW := float32(t.metrics.Width)
	cellH := float32(t.metrics.Height)
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
			BgA:       1,
			GlyphOffY: 0,
		}
		if c.r != 0 && t.uploader != nil {
			u0, v0, u1, v1, gw, gh, xoff, yoff, ok := t.uploader.OverlayUploadGlyph(c.r)
			if ok {
				inst.GlyphU0 = u0
				inst.V0 = v0
				inst.GlyphU1 = u1
				inst.V1 = v1
				inst.GlyphOffX = float32(xoff)
				inst.GlyphOffY = float32(t.metrics.Ascent + yoff)
				inst.GlyphW = float32(gw)
				inst.GlyphH = float32(gh)
			}
		}
		*instances = append(*instances, inst)
	}
	if csdW > 0 {
		csdCells := t.layoutCsdButtons(t.metrics.Width, width)
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
				BgA:       1,
				GlyphOffY: 0,
			}
			if c.r != 0 && t.uploader != nil {
				u0, v0, u1, v1, gw, gh, _, yoff, ok := t.uploader.OverlayUploadGlyph(c.r)
				if ok {
					inst.GlyphU0 = u0
					inst.V0 = v0
					inst.GlyphU1 = u1
					inst.V1 = v1
					inst.GlyphOffX = (float32(c.w)*cellW - float32(gw)) / 2
					inst.GlyphOffY = float32(t.metrics.Ascent + yoff)
					inst.GlyphW = float32(gw)
					inst.GlyphH = float32(gh)
				}
			}
			*instances = append(*instances, inst)
		}
	}
}
