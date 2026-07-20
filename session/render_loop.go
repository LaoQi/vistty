package session

import (
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"syscall"
	"time"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/plugins"
	"github.com/LaoQi/vistty/internal/ui"
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

	if m.plugins != nil {
		m.plugins.Activate(m)
	}

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
	cursorBlinkTicker := time.NewTicker(500 * time.Millisecond)
	defer cursorBlinkTicker.Stop()
	resizeCh := make(chan platform.ResizeEvent, 4)
	go func() {
		var cases []reflect.SelectCase
		doneCase := reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(m.done)}
		cases = append(cases, doneCase)
		for _, s := range m.slaves {
			ch := s.Surface().ResizeEvents()
			if ch == nil {
				continue
			}
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)})
		}
		for {
			chosen, v, ok := reflect.Select(cases)
			if chosen == 0 {
				if !ok {
					return
				}
				continue
			}
			if !ok {
				continue
			}
			ev := v.Interface().(platform.ResizeEvent)
			select {
			case resizeCh <- ev:
			case <-m.done:
				return
			}
		}
	}()

	for {
		select {
		case msg := <-usc:
			msg.term.Apply(msg.seqs)
			terminal.ReturnSeqPool(msg.seqs)
			if !msg.term.IsSyncUpdates() {
				m.dirty = true
			}
		case <-ticker.C:
			m.tickCount++
			if m.osdDirty {
				m.refreshOSD()
				m.osdDirty = false
				m.dirty = true
			}
			if len(m.titlePending) > 0 && m.plugins != nil {
				for _, title := range m.titlePending {
					m.plugins.FireTitleChange(title)
				}
				m.titlePending = m.titlePending[:0]
			}
			if !m.dirty {
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
		case <-cursorBlinkTicker.C:
			m.dirty = true
		case ev := <-resizeCh:
			m.handleResize(ev)
		case ev := <-m.keyEvCh:
			if m.plugins != nil {
				consumed, out := m.plugins.OnKey(ev)
				if consumed {
					continue
				}
				ev = out
			}
			m.handleKey(ev)
			m.dirty = true
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
		case ev := <-m.mouseEvCh:
			m.handleMouse(ev)
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
	if m.plugins != nil {
		m.plugins.FireExitHooks()
	}
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
	m.renderPlugins()
	return m.renderIndependent()
}

// renderPlugins 在主渲染线程调用插件 OnRender 钩子，将返回的图元转换为
// ui.PanelPrimitive 注入各 Slave 的 OSD。每帧调用一次，保证插件 UI 实时刷新。
// 同时同步 panelLines 到 OSD，使 Insets() 合并插件声明的面板尺寸。
func (m *Master) renderPlugins() {
	if m.plugins == nil {
		return
	}
	panels := m.plugins.EnabledPanels()
	for _, s := range m.slaves {
		osd := s.OSD()
		if osd == nil {
			continue
		}
		osd.SetPanelLines(panels)
		metrics := s.Face().Metrics()
		if metrics.Width <= 0 || metrics.Height <= 0 {
			continue
		}
		if s.CheckInsetsChanged() {
			top, bot, left, right := s.Insets()
			surfW, surfH := s.Surface().Size()
			innerW := surfW - left - right
			innerH := surfH - top - bot
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
			s.ResizeTerms(cols, rows)
			m.dirty = true
		}
		top, bottom, left, right := s.Insets()
		surfW, surfH := s.Surface().Size()
		for _, side := range []string{"bottom", "left", "right"} {
			w, h := pluginPanelCellSize(side, surfW, surfH, top, bottom, left, right, metrics)
			if w <= 0 || h <= 0 {
				osd.SetPluginPanel(side, nil)
				continue
			}
			dirty, prims := m.plugins.OnRender(side, w, h)
			if dirty {
				m.dirty = true
			}
			osd.SetPluginPanel(side, toUIPrimitives(prims))
		}
	}
}

// pluginPanelCellSize 计算指定 side 面板的 cell 尺寸（宽×列数，高×行数）。
func pluginPanelCellSize(side string, surfW, surfH, top, bottom, left, right int, m font.Metrics) (w, h int) {
	if m.Width <= 0 {
		m.Width = 8
	}
	if m.Height <= 0 {
		m.Height = 16
	}
	switch side {
	case "bottom":
		innerW := surfW - left - right
		w = innerW / m.Width
		h = bottom / m.Height
	case "left":
		w = left / m.Width
		h = surfH / m.Height
	case "right":
		w = right / m.Width
		h = surfH / m.Height
	}
	return
}

// toUIPrimitives 将 plugins.Primitive 转换为 ui.PanelPrimitive。
// session 包同时 import plugins 和 ui，无循环依赖。
func toUIPrimitives(prims []plugins.Primitive) []ui.PanelPrimitive {
	if len(prims) == 0 {
		return nil
	}
	out := make([]ui.PanelPrimitive, len(prims))
	for i, p := range prims {
		out[i] = ui.PanelPrimitive{
			Kind: p.Kind,
			X:    p.X,
			Y:    p.Y,
			W:    p.W,
			H:    p.H,
			Text: p.Text,
			Fg:   p.Fg,
			Bg:   p.Bg,
			Bold: p.Bold,
		}
	}
	return out
}

func (m *Master) renderIndependent() error {
	var firstErr error
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
			debug.Errorf("renderIndependent: slave %s render failed: %v\n", s.Output().Name(), err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}
	for _, s := range m.slaves {
		if err := s.Compositor().Present(); err != nil {
			debug.Errorf("renderIndependent: slave %s present failed: %v\n", s.Output().Name(), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *Master) handleResize(ev platform.ResizeEvent) {
	m.handleResizeIndependent(ev)
}

func (m *Master) handleResizeIndependent(ev platform.ResizeEvent) {
	var s *Slave
	for _, sl := range m.slaves {
		if sl.Surface().OutputID() == ev.OutputID {
			s = sl
			break
		}
	}
	if s == nil {
		s = m.slaves[m.primaryIdx]
	}
	if s == nil {
		return
	}
	metrics := s.Face().Metrics()
	if metrics.Width <= 0 {
		metrics.Width = 8
	}
	if metrics.Height <= 0 {
		metrics.Height = 16
	}
	top, bot, left, right := s.Insets()
	innerW := ev.Width - left - right
	innerH := ev.Height - top - bot
	cols := innerW / metrics.Width
	rows := innerH / metrics.Height
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	s.ResizeTerms(cols, rows)
	if m.plugins != nil {
		m.plugins.FireResize(ev.OutputID, ev.Width, ev.Height, cols, rows)
	}
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
	newFace, err := s.FaceCache().GetFace(newSize)
	if err != nil {
		debug.Errorf("handleScale: face cache get failed: %v\n", err)
		return
	}
	m.opts.FontSize = newSize
	s.SetFace(newFace)
	s.Compositor().SetFace(newFace)

	metrics := newFace.Metrics()
	if metrics.Width <= 0 {
		metrics.Width = 8
	}
	if metrics.Height <= 0 {
		metrics.Height = 16
	}
	w, h := s.Surface().Size()
	top, bot, left, right := s.Insets()
	innerW := w - left - right
	innerH := h - top - bot
	cols := innerW / metrics.Width
	rows := innerH / metrics.Height
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	s.ResizeTerms(cols, rows)
	if err := m.renderFrame(); err != nil {
		debug.Errorf("handleScale: render error: %v\n", err)
	}
	m.dirty = false
	if m.plugins != nil {
		m.plugins.FireZoom(newSize)
	}
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
		case ev, ok := <-m.input.KeyEvents():
			if !ok {
				return
			}
			stopRepeat()
			if ev.State == platform.KeyPress {
				m.dispatchKey(ev)
				if !platform.LookupModifierCode(ev.Code) {
					repeatEv = ev
					delayTimer = time.NewTimer(m.opts.RepeatDelay)
					delayCh = delayTimer.C
				}
			} else {
				m.dispatchKey(ev)
			}
		case <-delayCh:
			delayTimer = nil
			delayCh = nil
			rateTicker = time.NewTicker(m.opts.RepeatRate)
			rateCh = rateTicker.C
			m.dispatchKey(repeatEv)
		case <-rateCh:
			m.dispatchKey(repeatEv)
		case ev, ok := <-m.input.MouseEvents():
			if !ok {
				return
			}
			select {
			case m.mouseEvCh <- ev:
			case <-m.done:
				return
			}
		case <-m.done:
			return
		}
	}
}

// dispatchKey 由 inputLoop goroutine 调用，将按键事件投递到 keyEvCh，
// 由主线程 select 消费，保证 plugins.OnKey（gopher-lua VM 非线程安全）
// 与 handleKey 在主线程串行执行。
func (m *Master) dispatchKey(ev platform.KeyEvent) {
	select {
	case m.keyEvCh <- ev:
	case <-m.done:
	}
}

func (m *Master) handleKey(ev platform.KeyEvent) {
	if ev.State != platform.KeyPress {
		return
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
	if m.plugins != nil {
		m.plugins.FireScreenSwitch(idx + 1)
	}
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

func (m *Master) handleMouse(ev platform.MouseEvent) {
	if ev.State != platform.KeyPress {
		return
	}
	s := m.slaves[m.focusIdx]
	if s == nil {
		return
	}
	osd := s.OSD()
	if osd == nil {
		return
	}
	surfW, _ := s.Surface().Size()
	hit := osd.HitTestTabBar(ev.X, ev.Y, surfW)
	switch hit {
	case ui.TabBarCsdClose:
		m.signalClose()
	case ui.TabBarArea:
		if ev.Button == 1 && s.CsdMode() {
			if wm, ok := s.Surface().(platform.WindowMover); ok {
				wm.StartMove(ev.Serial)
			}
		}
	}
}
