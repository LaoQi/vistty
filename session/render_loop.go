package session

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/vte"
	"github.com/LaoQi/vistty/terminal"
)

type seqMsg struct {
	term *terminal.Terminal
	seqs []vte.Sequence
}

func (m *Master) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const maxRenderErrors = 10
	var renderErrCount int

	if m.focusTerm() == nil {
		return fmt.Errorf("no terminal session")
	}

	backendDone := make(chan struct{})
	go func() {
		m.backend.Run(func() {})
		close(backendDone)
	}()

	if err := m.renderFrame(); err != nil {
		return fmt.Errorf("initial render: %w", err)
	}
	m.dirty = false

	usc := make(chan seqMsg, 16)
	m.seqRelay = usc
	tec := make(chan *terminal.Terminal, 16)
	m.exitCh = tec
	for _, t := range m.terms {
		m.startTerminalGoroutines(t)
	}

	m.wg.Add(2)
	go m.inputLoop()
	go m.signalLoop()

	ticker := time.NewTicker(m.frameInterval)
	defer ticker.Stop()
	resizeCh := m.slaves[m.primaryIdx].Surface().ResizeEvents()

	for {
		select {
		case msg := <-usc:
			msg.term.Apply(msg.seqs)
			terminal.ReturnSeqPool(msg.seqs)
			m.dirty = true
		case <-ticker.C:
			m.tickCount++
			if m.osdDirty {
				m.refreshOSD()
				m.osdDirty = false
				m.dirty = true
			}
			// 无变化时跳过渲染以省 CPU；每 15 tick（~250ms）兜底渲染一次，
			// 保证光标闪烁（光标 500ms 翻转，250ms 兜底足以捕捉）。
			if !m.dirty && m.tickCount%15 != 0 {
				continue
			}
			var frameStart time.Time
			if m.fpsLogging {
				frameStart = time.Now()
			}
			if err := m.renderFrame(); err != nil {
				renderErrCount++
				debug.Errorf("Run: render error (%d/%d): %v\n", renderErrCount, maxRenderErrors, err)
				if renderErrCount >= maxRenderErrors {
					m.signalClose()
					goto exit
				}
			} else {
				renderErrCount = 0
				m.dirty = false
			}
			if m.fpsLogging {
				fmt.Fprintf(os.Stderr, "frame: %v\n", time.Since(frameStart))
			}
		case ev := <-resizeCh:
			m.handleResize(ev)
		case req := <-m.scaleReqCh:
			m.handleScale(req)
		case <-m.renderReqCh:
			if err := m.renderFrame(); err != nil {
				renderErrCount++
				debug.Errorf("Run: render request error (%d/%d): %v\n", renderErrCount, maxRenderErrors, err)
				if renderErrCount >= maxRenderErrors {
					m.signalClose()
					goto exit
				}
			} else {
				renderErrCount = 0
			}
			m.dirty = false
		case req := <-m.tabReqCh:
			m.handleTabRequest(req)
		case exited := <-tec:
			m.handleTermExit(exited)
		case <-m.done:
			goto exit
		case <-m.backend.Done():
			m.signalClose()
			goto exit
		}
	}

exit:
	debug.Debugf("Run: wg.Wait() starting\n")
	m.wg.Wait()
	debug.Debugf("Run: wg.Wait() done, calling backend.Stop()\n")
	m.backend.Stop()
	debug.Debugf("Run: backend.Stop() done, waiting for backendDone\n")
	<-backendDone
	debug.Debugf("Run: backendDone, closing input\n")
	m.input.Close()
	m.cleanup()
	debug.Debugf("Run: cleanup done\n")
	return nil
}

func (m *Master) renderFrame() error {
	return m.renderIndependent()
}

func (m *Master) renderIndependent() error {
	for _, s := range m.slaves {
		t := s.ActiveTerm()
		if t == nil {
			continue
		}
		if !t.Active() {
			continue
		}
		t.RLock()
		err := s.Compositor().Render(t.Screen(), t.ScrollOffset())
		t.RUnlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Master) handleResize(ev platform.ResizeEvent) {
	m.handleResizeIndependent(ev)
}

func (m *Master) handleResizeIndependent(ev platform.ResizeEvent) {
	s := m.slaves[m.primaryIdx]
	ft := s.ActiveTerm()
	if ft == nil {
		return
	}
	metrics := s.Face().Metrics()
	top, bot, left, right := s.Insets()
	innerW := ev.Width - left - right
	innerH := ev.Height - top - bot
	cols := innerW / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = innerH / metrics.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	ft.Resize(cols, rows)
	s.Compositor().Resize(cols, rows)
	ft.SetPtySize(rows, cols)
}

func (m *Master) requestScale(delta int) {
	select {
	case m.scaleReqCh <- scaleReq{delta: delta}:
	case <-m.done:
	default:
	}
}

func (m *Master) handleScale(req scaleReq) {
	m.handleScaleIndependent(req)
}

func (m *Master) handleScaleIndependent(req scaleReq) {
	const minSize, maxSize = 6.0, 72.0
	s := m.slaves[m.focusIdx]
	newSize := m.opts.FontSize + float64(req.delta)
	if req.delta == 0 {
		newSize = m.initialFontSize
	}
	if newSize < minSize {
		newSize = minSize
	}
	if newSize > maxSize {
		newSize = maxSize
	}
	if newSize == m.opts.FontSize {
		return
	}
	newFace, err := s.FaceCache().Get(newSize)
	if err != nil {
		debug.Errorf("handleScale: face cache get failed: %v\n", err)
		return
	}
	m.opts.FontSize = newSize
	s.SetFace(newFace)
	s.Compositor().SetFace(newFace)

	metrics := newFace.Metrics()
	w, h := s.Surface().Size()
	top, bot, left, right := s.Insets()
	innerW := w - left - right
	innerH := h - top - bot
	cols := innerW / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = innerH / metrics.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	ft := s.ActiveTerm()
	if ft != nil {
		ft.Resize(cols, rows)
		s.Compositor().Resize(cols, rows)
		ft.SetPtySize(rows, cols)
	}
	if err := m.renderFrame(); err != nil {
		debug.Errorf("handleScale: render error: %v\n", err)
	}
	m.dirty = false
}

func (m *Master) inputLoop() {
	defer m.wg.Done()

	var repeatEv platform.KeyEvent
	var delayTimer *time.Timer
	var rateTicker *time.Ticker
	var delayCh <-chan time.Time
	var rateCh <-chan time.Time

	stopRepeat := func() {
		if delayTimer != nil {
			delayTimer.Stop()
			delayTimer = nil
			delayCh = nil
		}
		if rateTicker != nil {
			rateTicker.Stop()
			rateTicker = nil
			rateCh = nil
		}
	}
	defer stopRepeat()

	for {
		select {
		case ev := <-m.input.KeyEvents():
			stopRepeat()
			if ev.State != platform.KeyPress {
				continue
			}
			m.handleKey(ev)
			if !platform.LookupModifierCode(ev.Code) {
				repeatEv = ev
				delayTimer = time.NewTimer(m.opts.RepeatDelay)
				delayCh = delayTimer.C
			}
		case <-delayCh:
			delayTimer = nil
			delayCh = nil
			rateTicker = time.NewTicker(m.opts.RepeatRate)
			rateCh = rateTicker.C
			m.handleKey(repeatEv)
		case <-rateCh:
			m.handleKey(repeatEv)
		case <-m.done:
			return
		}
	}
}

func (m *Master) handleKey(ev platform.KeyEvent) {
	if ev.Mods&platform.ModSuper != 0 {
		switch ev.Rune {
		case '=':
			m.requestScale(1)
			return
		case '-':
			m.requestScale(-1)
			return
		case '0':
			m.requestScale(0)
			return
		case 't':
			m.requestTab(tabNew)
			return
		case 'w':
			m.requestTab(tabClose)
			return
		}
		if ev.Code >= 2 && ev.Code <= 10 {
			idx := int(ev.Code) - 2
			if idx < len(m.slaves) {
				m.setFocus(idx)
			}
			return
		}
		if ev.Code == 15 {
			m.setFocus((m.focusIdx + 1) % len(m.slaves))
			return
		}
		if ev.Code == 105 {
			m.requestTab(tabPrev)
			return
		}
		if ev.Code == 106 {
			m.requestTab(tabNext)
			return
		}
	}
	if ft := m.focusTerm(); ft != nil {
		ft.HandleKey(ev)
	}
}

func (m *Master) setFocus(idx int) {
	if idx < 0 || idx >= len(m.slaves) {
		return
	}
	if idx == m.focusIdx {
		return
	}
	m.focusIdx = idx
	select {
	case m.renderReqCh <- struct{}{}:
	case <-m.done:
	default:
	}
}

func (m *Master) signalLoop() {
	defer m.wg.Done()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(ch)
	select {
	case <-ch:
		m.signalClose()
	case <-m.done:
	}
}
