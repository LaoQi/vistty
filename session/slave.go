package session

import (
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
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

func (s *Slave) InitIndependent(fontData []byte, fontSize float64) error {
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
}

func (s *Slave) FaceCache() *font.FaceCache {
	return s.faceCache
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
