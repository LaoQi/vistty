package session

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

type scaleReq struct {
	delta int
}

type tabAction int

const (
	tabNew tabAction = iota
	tabClose
	tabPrev
	tabNext
)

type tabReq struct {
	action tabAction
}

type Master struct {
	backend    platform.Backend
	opts       terminal.Options
	outputs    []platform.Output
	primaryIdx int
	slaves     []*Slave
	input      platform.InputSource

	fontData        []byte
	initialFontSize float64

	terms    []*terminal.Terminal
	focusIdx int

	scaleReqCh    chan scaleReq
	renderReqCh   chan struct{}
	fpsLogging    bool
	frameInterval time.Duration
	dirty         bool
	tickCount     uint64

	osdCfg   ui.Config
	tabReqCh chan tabReq
	osdDirty bool

	seqRelay chan seqMsg
	exitCh   chan *terminal.Terminal

	done        chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup
	cleanupOnce sync.Once
}

func NewMaster(backend platform.Backend, opts terminal.Options, osdCfg ui.Config) (*Master, error) {
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

	var slaves []*Slave
	for _, out := range outputs {
		surf, err := backend.CreateSurfaceFor(out)
		if err != nil {
			for _, s := range slaves {
				s.Close()
			}
			return nil, fmt.Errorf("create surface for %s: %w", out.Name(), err)
		}
		slaves = append(slaves, NewSlave(out, surf))
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
		frameInterval:   time.Second / 60,
		osdCfg:          osdCfg,
		tabReqCh:        make(chan tabReq, 1),
		done:            make(chan struct{}),
	}

	if err := m.initIndependent(); err != nil {
		for _, s := range slaves {
			s.Close()
		}
		return nil, err
	}

	input, err := backend.CreateInputSource()
	if err != nil {
		m.cleanup()
		return nil, fmt.Errorf("create input: %w", err)
	}
	m.input = input

	return m, nil
}

func (m *Master) initIndependent() error {
	for _, s := range m.slaves {
		if err := s.InitIndependent(m.fontData, m.opts.FontSize, m.osdCfg); err != nil {
			return fmt.Errorf("init independent slave %s: %w", s.Output().Name(), err)
		}
		met := s.Face().Metrics()
		w, h := s.Surface().Size()
		top, bot, left, right := s.Insets()
		innerW := w - left - right
		innerH := h - top - bot
		cols := innerW / met.Width
		rows := 0
		if met.Height > 0 {
			rows = innerH / met.Height
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
		m.bindTerminalCallbacks(s, term)
		s.BindTerminal(term)
		s.UpdateTabs()
		m.terms = append(m.terms, term)
	}
	return nil
}

func (m *Master) bindTerminalCallbacks(s *Slave, term *terminal.Terminal) {
	slaveComp := s.Compositor()
	term.SetOnDefaultColor(func(fg, bg screen.Color) {
		slaveComp.SetDefaultColors(fg, bg)
	})
	term.SetOnTitle(func(string) {
		m.osdDirty = true
	})
}

func (m *Master) startTerminalGoroutines(t *terminal.Terminal) {
	m.wg.Add(3)
	go func() {
		defer m.wg.Done()
		t.PtyReadLoop()
	}()
	go func() {
		defer m.wg.Done()
		for {
			select {
			case seqs := <-t.SeqCh():
				select {
				case m.seqRelay <- seqMsg{term: t, seqs: seqs}:
				case <-m.done:
					return
				}
			case <-m.done:
				return
			}
		}
	}()
	go func() {
		defer m.wg.Done()
		select {
		case <-t.EofCh():
		case <-t.Done():
		case <-m.done:
			return
		}
		select {
		case m.exitCh <- t:
		case <-m.done:
		}
	}()
}

func (m *Master) refreshOSD() {
	for _, s := range m.slaves {
		s.UpdateTabs()
	}
}

func (m *Master) focusTerm() *terminal.Terminal {
	if m.focusIdx < len(m.slaves) {
		return m.slaves[m.focusIdx].ActiveTerm()
	}
	return nil
}

func (m *Master) EnableFPSLogging() {
	m.fpsLogging = true
}

// SetFrameRate 设置渲染帧率，预留动态帧率调整。
// 注意：当前 ticker 在 Run 启动时读取 frameInterval 一次性创建，
// 运行中调用本方法不会重建已运行的 ticker；预留供后续动态帧率实现。
func (m *Master) SetFrameRate(rate int) {
	if rate <= 0 {
		rate = 60
	}
	m.frameInterval = time.Second / time.Duration(rate)
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
		for _, s := range m.slaves {
			s.Close()
		}
	})
}

func (m *Master) requestTab(action tabAction) {
	select {
	case m.tabReqCh <- tabReq{action: action}:
	case <-m.done:
	default:
	}
}

func (m *Master) handleTabRequest(req tabReq) {
	s := m.slaves[m.focusIdx]
	switch req.action {
	case tabNew:
		m.newTab(s)
	case tabClose:
		m.closeTab(s)
	case tabPrev:
		if len(s.terms) > 0 {
			s.activeIdx = (s.activeIdx - 1 + len(s.terms)) % len(s.terms)
			m.refreshOSD()
			m.dirty = true
		}
	case tabNext:
		if len(s.terms) > 0 {
			s.activeIdx = (s.activeIdx + 1) % len(s.terms)
			m.refreshOSD()
			m.dirty = true
		}
	}
}

func (m *Master) newTab(s *Slave) {
	metrics := s.Face().Metrics()
	top, bot, left, right := s.Insets()
	w, h := s.Surface().Size()
	cols := (w - left - right) / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = (h - top - bot) / metrics.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	term, err := terminal.New(m.opts, cols, rows)
	if err != nil {
		debug.Errorf("newTab: %v", err)
		return
	}
	m.bindTerminalCallbacks(s, term)
	s.BindTerminal(term)
	s.activeIdx = len(s.terms) - 1
	m.terms = append(m.terms, term)
	m.startTerminalGoroutines(term)
	m.refreshOSD()
	m.dirty = true
}

func (m *Master) closeTab(s *Slave) {
	if len(s.terms) == 0 {
		return
	}
	t := s.terms[s.activeIdx]
	t.SignalClose()
}

func (m *Master) handleTermExit(t *terminal.Terminal) {
	for _, s := range m.slaves {
		for i, term := range s.terms {
			if term == t {
				s.terms = append(s.terms[:i], s.terms[i+1:]...)
				if s.activeIdx >= len(s.terms) && len(s.terms) > 0 {
					s.activeIdx = len(s.terms) - 1
				}
				break
			}
		}
	}
	for i, term := range m.terms {
		if term == t {
			m.terms = append(m.terms[:i], m.terms[i+1:]...)
			break
		}
	}
	t.Close()
	if len(m.terms) == 0 {
		m.signalClose()
		return
	}
	m.refreshOSD()
	m.dirty = true
}
