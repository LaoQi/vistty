package master

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/vte"
	"github.com/LaoQi/vistty/slave"
	"github.com/LaoQi/vistty/terminal"
)

type seqMsg struct {
	term *terminal.Terminal
	seqs []vte.Sequence
}

func (m *Master) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if m.focusTerm() == nil {
		return fmt.Errorf("no terminal session")
	}

	if err := m.renderFrame(); err != nil {
		return fmt.Errorf("initial render: %w", err)
	}
	m.dirty = false

	m.wg.Add(len(m.terms))
	for _, t := range m.terms {
		go func(t *terminal.Terminal) {
			defer m.wg.Done()
			t.PtyReadLoop()
		}(t)
	}

	var unifiedSeqCh <-chan seqMsg
	var termExitCh <-chan struct{}
	if m.opts.Mode == "independent" {
		usc := make(chan seqMsg, 16)
		unifiedSeqCh = usc
		m.wg.Add(len(m.terms))
		for _, t := range m.terms {
			go func(t *terminal.Terminal) {
				defer m.wg.Done()
				for {
					select {
					case seqs := <-t.SeqCh():
						select {
						case usc <- seqMsg{term: t, seqs: seqs}:
						case <-m.done:
							return
						}
					case <-m.done:
						return
					}
				}
			}(t)
		}

		tec := make(chan struct{}, 1)
		termExitCh = tec
		m.wg.Add(len(m.terms))
		for _, t := range m.terms {
			go func(t *terminal.Terminal) {
				defer m.wg.Done()
				select {
				case <-t.EofCh():
				case <-t.Done():
				case <-m.done:
					return
				}
				select {
				case tec <- struct{}{}:
				case <-m.done:
				}
			}(t)
		}
	}

	m.wg.Add(2)
	go m.inputLoop()
	go m.signalLoop()

	backendDone := make(chan struct{})
	go func() {
		m.backend.Run(func() {})
		close(backendDone)
	}()

	ticker := time.NewTicker(m.frameInterval)
	defer ticker.Stop()
	resizeCh := m.slaves[m.primaryIdx].Surface().ResizeEvents()

	var mirrorSeqCh <-chan []vte.Sequence
	var mirrorEofCh, mirrorDoneCh <-chan struct{}
	if m.opts.Mode != "independent" {
		ft := m.focusTerm()
		mirrorSeqCh = ft.SeqCh()
		mirrorEofCh = ft.EofCh()
		mirrorDoneCh = ft.Done()
	}

	for {
		select {
		case seqs := <-mirrorSeqCh:
			if ft := m.focusTerm(); ft != nil {
				ft.Apply(seqs)
			}
			terminal.ReturnSeqPool(seqs)
			m.dirty = true
		case msg := <-unifiedSeqCh:
			msg.term.Apply(msg.seqs)
			terminal.ReturnSeqPool(msg.seqs)
			m.dirty = true
		case <-ticker.C:
			m.tickCount++
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
				debug.Errorf("Run: render error: %v\n", err)
				m.signalClose()
				goto exit
			}
			m.dirty = false
			if m.fpsLogging {
				fmt.Fprintf(os.Stderr, "frame: %v\n", time.Since(frameStart))
			}
		case ev := <-resizeCh:
			m.handleResize(ev)
		case req := <-m.scaleReqCh:
			m.handleScale(req)
		case <-m.renderReqCh:
			if err := m.renderFrame(); err != nil {
				debug.Errorf("Run: render request error: %v\n", err)
				m.signalClose()
				goto exit
			}
			m.dirty = false
		case <-mirrorEofCh:
			m.signalClose()
			goto exit
		case <-mirrorDoneCh:
			goto exit
		case <-termExitCh:
			m.signalClose()
			goto exit
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
	if m.opts.Mode == "independent" {
		return m.renderIndependent()
	}
	return m.renderMirror()
}

func (m *Master) renderMirror() error {
	ft := m.focusTerm()
	ft.RLock()
	err := m.compositor.Render(ft.Screen(), ft.ScrollOffset())
	ft.RUnlock()
	if err != nil {
		return err
	}

	if len(m.slaves) <= 1 {
		return nil
	}

	srcBuf, srcStride, srcW, srcH := m.compositor.BackBuf()
	primaryID := m.outputs[m.primaryIdx].ID()
	for _, s := range m.slaves {
		if s.Output().ID() == primaryID {
			continue
		}
		m.blitToSlave(s, srcBuf, srcStride, srcW, srcH)
		s.Surface().Swap()
	}
	return nil
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

func (m *Master) blitToSlave(s *slave.Slave, src []byte, srcStride, srcW, srcH int) {
	surf := s.Surface()
	dst := surf.Data()
	if dst == nil {
		return
	}
	dstStride := surf.Stride()
	w, h := surf.Size()

	fillBlack(dst, dstStride, w, h)

	copyW := srcW
	if w < copyW {
		copyW = w
	}
	copyH := srcH
	if h < copyH {
		copyH = h
	}

	for y := 0; y < copyH; y++ {
		srcOff := y * srcStride
		dstOff := y * dstStride
		copy(dst[dstOff:dstOff+copyW*4], src[srcOff:srcOff+copyW*4])
	}
}

func (m *Master) handleResize(ev platform.ResizeEvent) {
	if m.opts.Mode == "independent" {
		m.handleResizeIndependent(ev)
		return
	}
	metrics := m.face.Metrics()
	cols := ev.Width / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = ev.Height / metrics.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	ft := m.focusTerm()
	ft.Resize(cols, rows)
	m.compositor.Resize(cols, rows)
	ft.SetPtySize(rows, cols)
}

func (m *Master) handleResizeIndependent(ev platform.ResizeEvent) {
	s := m.slaves[m.primaryIdx]
	ft := s.ActiveTerm()
	if ft == nil {
		return
	}
	metrics := s.Face().Metrics()
	cols := ev.Width / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = ev.Height / metrics.Height
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
	if m.opts.Mode == "independent" {
		m.handleScaleIndependent(req)
		return
	}
	const minSize, maxSize = 6.0, 72.0

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

	newFace, err := m.faceCache.Get(newSize)
	if err != nil {
		debug.Errorf("handleScale: face cache get failed: %v\n", err)
		return
	}

	m.opts.FontSize = newSize
	m.face = newFace
	m.compositor.SetFace(newFace)

	metrics := newFace.Metrics()
	surf := m.slaves[m.primaryIdx].Surface()
	w, h := surf.Size()
	cols := w / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = h / metrics.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	ft := m.focusTerm()
	ft.Resize(cols, rows)
	m.compositor.Resize(cols, rows)
	ft.SetPtySize(rows, cols)

	if err := m.renderFrame(); err != nil {
		debug.Errorf("handleScale: render error: %v\n", err)
	}
	m.dirty = false
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
	cols := w / metrics.Width
	rows := 0
	if metrics.Height > 0 {
		rows = h / metrics.Height
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
		}
		if m.opts.Mode == "independent" {
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

func fillBlack(data []byte, stride, w, h int) {
	pixel := uint32(255) << 24
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*stride + x*4
			if off+4 <= len(data) {
				*(*uint32)(unsafe.Pointer(&data[off])) = pixel
			}
		}
	}
}
