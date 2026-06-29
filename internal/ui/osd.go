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

func (o *OSD) RenderCPU(buf []byte, stride, width, height int) {
	if !o.cfg.Top.Enabled || o.metrics.Height <= 0 || len(buf) == 0 {
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
}

func (o *OSD) RenderGPU(instances *[]platform.CellInstance, width, height int) {
	if !o.cfg.Top.Enabled || o.metrics.Height <= 0 {
		return
	}
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
