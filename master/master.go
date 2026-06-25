package master

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/slave"
	"github.com/LaoQi/vistty/terminal"
)

type scaleReq struct {
	delta int
}

type Master struct {
	backend         platform.Backend
	opts            terminal.Options
	outputs         []platform.Output
	primaryIdx      int
	slaves          []*slave.Slave
	input           platform.InputSource

	face            font.Face
	faceCache       *font.FaceCache
	compositor      *render.Compositor
	fontData        []byte
	initialFontSize float64

	terms           []*terminal.Terminal
	focusIdx        int

	scaleReqCh      chan scaleReq
	renderReqCh     chan struct{}
	fpsLogging      bool

	done            chan struct{}
	closeOnce       sync.Once
	wg              sync.WaitGroup
	cleanupOnce     sync.Once
}

func New(backend platform.Backend, opts terminal.Options) (*Master, error) {
	outputs, err := backend.ListOutputs()
	if err != nil {
		return nil, fmt.Errorf("list outputs: %w", err)
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("no outputs available")
	}

	primaryIdx := 0
	if opts.Primary != "" {
		found := false
		for i, o := range outputs {
			if o.Name() == opts.Primary {
				primaryIdx = i
				found = true
				break
			}
		}
		if !found {
			if idx, err := strconv.Atoi(opts.Primary); err == nil && idx >= 0 && idx < len(outputs) {
				primaryIdx = idx
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("primary output not found: %s", opts.Primary)
		}
	}

	var fontData []byte
	if opts.FontPath != "" {
		fontData, err = os.ReadFile(opts.FontPath)
		if err != nil {
			return nil, fmt.Errorf("read font file: %w", err)
		}
	} else {
		fontData = font.EmbeddedFontData()
	}

	var slaves []*slave.Slave
	for _, out := range outputs {
		surf, err := backend.CreateSurfaceFor(out)
		if err != nil {
			for _, s := range slaves {
				s.Close()
			}
			return nil, fmt.Errorf("create surface for %s: %w", out.Name(), err)
		}
		slaves = append(slaves, slave.New(out, surf))
	}

	m := &Master{
		backend:         backend,
		opts:            opts,
		outputs:         outputs,
		primaryIdx:      primaryIdx,
		slaves:          slaves,
		fontData:        fontData,
		initialFontSize: opts.FontSize,
		focusIdx:        0,
		scaleReqCh:      make(chan scaleReq, 1),
		renderReqCh:     make(chan struct{}, 1),
		done:            make(chan struct{}),
	}

	if opts.Mode == "independent" {
		if err := m.initIndependent(); err != nil {
			for _, s := range slaves {
				s.Close()
			}
			return nil, err
		}
	} else {
		if err := m.initMirror(); err != nil {
			for _, s := range slaves {
				s.Close()
			}
			return nil, err
		}
	}

	input, err := backend.CreateInputSource()
	if err != nil {
		m.cleanup()
		return nil, fmt.Errorf("create input: %w", err)
	}
	m.input = input

	return m, nil
}

func (m *Master) initMirror() error {
	faceCache, err := font.NewFaceCache(m.fontData, 72)
	if err != nil {
		return fmt.Errorf("load font: %w", err)
	}

	face, err := faceCache.Get(m.opts.FontSize)
	if err != nil {
		faceCache.Close()
		return fmt.Errorf("get face: %w", err)
	}

	primarySurf := m.slaves[m.primaryIdx].Surface()
	met := face.Metrics()
	w, h := primarySurf.Size()
	cols := w / met.Width
	rows := 0
	if met.Height > 0 {
		rows = h / met.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	term, err := terminal.New(m.opts, cols, rows)
	if err != nil {
		faceCache.Close()
		return fmt.Errorf("create terminal: %w", err)
	}
	term.SetOnDefaultColor(func(fg, bg screen.Color) {
		m.compositor.SetDefaultColors(fg, bg)
	})

	for _, s := range m.slaves {
		s.BindTerminal(term)
	}

	m.face = face
	m.faceCache = faceCache
	m.compositor = render.NewCompositor(primarySurf, face)
	m.terms = []*terminal.Terminal{term}
	return nil
}

func (m *Master) initIndependent() error {
	for _, s := range m.slaves {
		if err := s.InitIndependent(m.fontData, m.opts.FontSize); err != nil {
			return fmt.Errorf("init independent slave %s: %w", s.Output().Name(), err)
		}
		met := s.Face().Metrics()
		w, h := s.Surface().Size()
		cols := w / met.Width
		rows := 0
		if met.Height > 0 {
			rows = h / met.Height
		}
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}
		term, err := terminal.New(m.opts, cols, rows)
		if err != nil {
			return fmt.Errorf("create terminal: %w", err)
		}
		slaveComp := s.Compositor()
		term.SetOnDefaultColor(func(fg, bg screen.Color) {
			slaveComp.SetDefaultColors(fg, bg)
		})
		s.BindTerminal(term)
		m.terms = append(m.terms, term)
	}
	return nil
}

func (m *Master) focusTerm() *terminal.Terminal {
	if m.focusIdx < len(m.terms) {
		return m.terms[m.focusIdx]
	}
	return nil
}

func (m *Master) EnableFPSLogging() {
	m.fpsLogging = true
}

func (m *Master) Close() error {
	m.signalClose()
	m.wg.Wait()
	if m.backend != nil {
		m.backend.Stop()
	}
	if m.input != nil {
		m.input.Close()
	}
	m.cleanup()
	return nil
}

func (m *Master) signalClose() {
	m.closeOnce.Do(func() {
		close(m.done)
		for _, t := range m.terms {
			t.SignalClose()
		}
	})
}

func (m *Master) cleanup() {
	m.cleanupOnce.Do(func() {
		for _, t := range m.terms {
			t.Close()
		}
		if m.compositor != nil {
			m.compositor.Close()
		}
		if m.faceCache != nil {
			m.faceCache.Close()
		}
		for _, s := range m.slaves {
			s.Close()
		}
	})
}
