package session

import (
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

type Slave struct {
	output     platform.Output
	surface    platform.Surface
	terms      []*terminal.Terminal
	activeIdx  int

	compositor *render.Compositor
	faceCache  *font.FaceCache
	face       font.Face
	osd        *ui.OSD
}

func NewSlave(output platform.Output, surface platform.Surface) *Slave {
	return &Slave{
		output:    output,
		surface:   surface,
		activeIdx: 0,
	}
}

func (s *Slave) ActiveTerm() *terminal.Terminal {
	if s.activeIdx < len(s.terms) {
		return s.terms[s.activeIdx]
	}
	return nil
}

func (s *Slave) BindTerminal(t *terminal.Terminal) {
	s.terms = append(s.terms, t)
}

func (s *Slave) Surface() platform.Surface {
	return s.surface
}

func (s *Slave) Output() platform.Output {
	return s.output
}

func (s *Slave) InitIndependent(fontData []byte, fontSize float64, osdCfg ui.Config) error {
	fc, err := font.NewFaceCache(fontData, 72)
	if err != nil {
		return err
	}
	face, err := fc.Get(fontSize)
	if err != nil {
		fc.Close()
		return err
	}
	s.faceCache = fc
	s.face = face
	s.compositor = render.NewCompositor(s.surface, face)
	s.osd = ui.NewOSD(osdCfg, face)
	s.compositor.SetOverlay(s.osd)
	s.osd.SetGlyphProvider(s.compositor)
	s.osd.SetGPUGlyphUploader(s.compositor)
	return nil
}

func (s *Slave) Compositor() *render.Compositor {
	return s.compositor
}

func (s *Slave) Face() font.Face {
	return s.face
}

func (s *Slave) SetFace(f font.Face) {
	s.face = f
	if s.osd != nil {
		s.osd.UpdateFace(f)
	}
}

func (s *Slave) FaceCache() *font.FaceCache {
	return s.faceCache
}

func (s *Slave) Insets() (top, bottom, left, right int) {
	if s.osd == nil {
		return 0, 0, 0, 0
	}
	return s.osd.Insets()
}

func (s *Slave) UpdateTabs() {
	if s.osd == nil || len(s.terms) == 0 {
		return
	}
	tabs := make([]ui.Tab, len(s.terms))
	for i, t := range s.terms {
		tabs[i] = ui.Tab{
			Title:  t.Title(),
			Active: i == s.activeIdx,
		}
	}
	s.osd.SetTabs(tabs, s.activeIdx)
}

func (s *Slave) OSD() *ui.OSD {
	return s.osd
}

func (s *Slave) Close() error {
	if s.compositor != nil {
		s.compositor.Close()
		s.compositor = nil
	} else {
		s.surface.Close()
	}
	if s.faceCache != nil {
		s.faceCache.Close()
		s.faceCache = nil
	}
	return nil
}
