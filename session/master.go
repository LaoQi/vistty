package session

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/plugins"
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
	tabSwitch
)

type tabReq struct {
	action tabAction
	idx    int
}

type Master struct {
	backend    platform.Backend
	opts       terminal.Options
	outputs    []platform.Output
	primaryIdx int
	slaves     []*Slave
	input      platform.InputSource

	fontData         []byte
	fallbackFontData []byte
	initialFontSize  float64

	terms    []*terminal.Terminal
	focusIdx int

	scaleReqCh    chan scaleReq
	renderReqCh   chan struct{}
	fpsLogging    bool
	frameInterval time.Duration
	dirty         bool
	tickCount     uint64

	tabReqCh chan tabReq
	osdDirty bool

	titlePending []string

	seqRelay chan seqMsg
	exitCh   chan *terminal.Terminal

	keyEvCh   chan platform.KeyEvent
	mouseEvCh chan platform.MouseEvent
	plugins   *plugins.PluginManager

	done        chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup
	cleanupOnce sync.Once
}

func NewMaster(backend platform.Backend, opts terminal.Options) (*Master, error) {
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

	var fallbackFontData []byte
	if opts.FallbackFontPath != "" {
		fallbackFontData, err = os.ReadFile(opts.FallbackFontPath)
		if err != nil {
			return nil, fmt.Errorf("read fallback font file: %w", err)
		}
	} else {
		fallbackFontData = font.EmbeddedFallbackFontData()
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
		backend:          backend,
		opts:             opts,
		outputs:          outputs,
		primaryIdx:       primaryIdx,
		slaves:           slaves,
		fontData:         fontData,
		fallbackFontData: fallbackFontData,
		initialFontSize:  opts.FontSize,
		focusIdx:         0,
		scaleReqCh:       make(chan scaleReq, 8),
		renderReqCh:      make(chan struct{}, 1),
		frameInterval:    time.Second / 60,
		tabReqCh:         make(chan tabReq, 8),
		keyEvCh:          make(chan platform.KeyEvent, 64),
		mouseEvCh:        make(chan platform.MouseEvent, 16),
		done:             make(chan struct{}),
	}

	m.opts.OnRenderRequest = func() {
		select {
		case m.renderReqCh <- struct{}{}:
		default:
		}
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
		if err := s.InitIndependent(m.fontData, m.fallbackFontData, m.opts.FontSize); err != nil {
			return fmt.Errorf("init independent slave %s: %w", s.Output().Name(), err)
		}
		met := s.Face().Metrics()
		if met.Width <= 0 {
			met.Width = 8
		}
		if met.Height <= 0 {
			met.Height = 16
		}
		w, h := s.Surface().Size()
		top, bot, left, right := s.Insets()
		innerW := w - left - right
		innerH := h - top - bot
		cols := innerW / met.Width
		rows := innerH / met.Height
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
	term.SetOnCursorColor(func(c screen.Color) {
		slaveComp.SetCursorColor(c)
	})
	term.SetOnTitle(func(title string) {
		m.titlePending = append(m.titlePending, title)
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
			case seqs, ok := <-t.SeqCh():
				if !ok {
					return
				}
				select {
				case m.seqRelay <- seqMsg{term: t, seqs: seqs}:
				case <-m.done:
					return
				}
			case <-t.Done():
				return
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

// SetPluginManager 注入插件管理器，由 cmd/vistty 在 P5 阶段组装调用。
// 须在 Run 之前调用。nil 时禁用插件拦截，按键直接走 handleKey。
func (m *Master) SetPluginManager(pm *plugins.PluginManager) {
	m.plugins = pm
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
			old := s.activeIdx
			s.activeIdx = (s.activeIdx - 1 + len(s.terms)) % len(s.terms)
			m.damageActiveScreen(s)
			m.refreshOSD()
			m.dirty = true
			if m.plugins != nil && old != s.activeIdx {
				m.plugins.FireTabSwitch(s.activeIdx+1, old+1)
			}
		}
	case tabNext:
		if len(s.terms) > 0 {
			old := s.activeIdx
			s.activeIdx = (s.activeIdx + 1) % len(s.terms)
			m.damageActiveScreen(s)
			m.refreshOSD()
			m.dirty = true
			if m.plugins != nil && old != s.activeIdx {
				m.plugins.FireTabSwitch(s.activeIdx+1, old+1)
			}
		}
	case tabSwitch:
		idx := req.idx - 1
		if idx >= 0 && idx < len(s.terms) {
			old := s.activeIdx
			s.activeIdx = idx
			m.damageActiveScreen(s)
			m.refreshOSD()
			m.dirty = true
			if m.plugins != nil && old != idx {
				m.plugins.FireTabSwitch(idx+1, old+1)
			}
		}
	}
}

// damageActiveScreen 对 slave 当前 active terminal 的 screen 调用 DamageAll。
// 切换 tab 时新 active terminal 的 cell 在上次渲染时已被 SetClean，
// Compositor dirty 路径会跳过 Clean cell 导致 backBuf 残留前一个 terminal
// 的内容。DamageAll 清除 Clean 位 + 标记 line dirty，强制下次 Render 全量重绘。
// 在 terminal 写锁下调用，避免与 pty-read goroutine 的 executeSequences 竞争。
func (m *Master) damageActiveScreen(s *Slave) {
	t := s.ActiveTerm()
	if t == nil {
		return
	}
	t.Lock()
	t.Screen().DamageAll()
	t.Unlock()
}

func (m *Master) newTab(s *Slave) {
	metrics := s.Face().Metrics()
	if metrics.Width <= 0 {
		metrics.Width = 8
	}
	if metrics.Height <= 0 {
		metrics.Height = 16
	}
	top, bot, left, right := s.Insets()
	w, h := s.Surface().Size()
	cols := (w - left - right) / metrics.Width
	rows := (h - top - bot) / metrics.Height
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
	if m.plugins != nil {
		m.plugins.FireTabNew(s.activeIdx+1, term.Title())
	}
}

func (m *Master) closeTab(s *Slave) {
	if len(s.terms) == 0 {
		return
	}
	t := s.terms[s.activeIdx]
	t.SignalClose()
}

func (m *Master) handleTermExit(t *terminal.Terminal) {
	closeIdx := -1
	closeTitle := t.Title()
	var affectedSlave *Slave
	for _, s := range m.slaves {
		for i, term := range s.terms {
			if term == t {
				closeIdx = i
				s.terms = append(s.terms[:i], s.terms[i+1:]...)
				if s.activeIdx >= len(s.terms) && len(s.terms) > 0 {
					s.activeIdx = len(s.terms) - 1
				}
				affectedSlave = s
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
	if m.plugins != nil && closeIdx >= 0 {
		m.plugins.FireTabClose(closeIdx+1, closeTitle)
	}
	if len(m.terms) == 0 {
		m.signalClose()
		return
	}
	// 关闭 tab 后 active tab 可能切换到另一个之前渲染过的 terminal，
	// 其 cell 已 Clean，dirty 路径会跳过导致 backBuf 残留被关闭 tab 的内容。
	if affectedSlave != nil {
		m.damageActiveScreen(affectedSlave)
	}
	m.refreshOSD()
	m.dirty = true
}

// === plugins.PluginContext 实现 ===
//
// 以下方法实现 internal/plugins.PluginContext 接口。
// 标签/缩放操作通过 channel 投递主线程执行（与渲染循环串行）。
// 终端访问方法（FocusTerm/Terms/TabList）只读，假定在主线程调用；
// 若外部 goroutine 调用需调用方自行保证不与渲染循环并发。

// FocusTerm 返回当前焦点终端。未激活时返回 nil。
func (m *Master) FocusTerm() *terminal.Terminal {
	return m.focusTerm()
}

// Terms 返回所有活跃终端列表（跨屏幕）。
func (m *Master) Terms() []*terminal.Terminal {
	return m.terms
}

// NewTab 在当前焦点屏幕创建新标签。
func (m *Master) NewTab() error {
	select {
	case m.tabReqCh <- tabReq{action: tabNew}:
	case <-m.done:
		return fmt.Errorf("master closed")
	default:
		debug.Warningf("NewTab: tab request channel full, dropping")
	}
	return nil
}

// CloseCurrentTab 关闭当前焦点标签。
func (m *Master) CloseCurrentTab() {
	select {
	case m.tabReqCh <- tabReq{action: tabClose}:
	case <-m.done:
	default:
		debug.Warningf("CloseCurrentTab: tab request channel full, dropping")
	}
}

// NextTab 切换到下一个标签。
func (m *Master) NextTab() {
	select {
	case m.tabReqCh <- tabReq{action: tabNext}:
	case <-m.done:
	default:
		debug.Warningf("NextTab: tab request channel full, dropping")
	}
}

// PrevTab 切换到上一个标签。
func (m *Master) PrevTab() {
	select {
	case m.tabReqCh <- tabReq{action: tabPrev}:
	case <-m.done:
	default:
		debug.Warningf("PrevTab: tab request channel full, dropping")
	}
}

// SwitchTab 切换到指定索引的标签（1-based）。
// 索引越界时静默忽略。
func (m *Master) SwitchTab(idx int) {
	select {
	case m.tabReqCh <- tabReq{action: tabSwitch, idx: idx}:
	case <-m.done:
	default:
		debug.Warningf("SwitchTab: tab request channel full, dropping")
	}
}

// TabList 返回当前焦点屏幕的标签列表。
// 在主线程调用安全（Lua 钩子在主线程执行）。
func (m *Master) TabList() []plugins.TabInfo {
	if m.focusIdx >= len(m.slaves) {
		return nil
	}
	slave := m.slaves[m.focusIdx]
	if len(slave.terms) == 0 {
		return nil
	}
	list := make([]plugins.TabInfo, len(slave.terms))
	for i, t := range slave.terms {
		list[i] = plugins.TabInfo{
			Title:  t.Title(),
			Active: i == slave.activeIdx,
		}
	}
	return list
}

// NextScreen 切换到下一个屏幕（多屏场景）。
func (m *Master) NextScreen() {
	if len(m.slaves) == 0 {
		return
	}
	m.setFocus((m.focusIdx + 1) % len(m.slaves))
}

// PrevScreen 切换到上一个屏幕（多屏场景）。
func (m *Master) PrevScreen() {
	if len(m.slaves) == 0 {
		return
	}
	m.setFocus((m.focusIdx - 1 + len(m.slaves)) % len(m.slaves))
}

// SwitchScreen 切换到指定索引的屏幕（1-based）。
// 索引越界时静默忽略。
func (m *Master) SwitchScreen(idx int) {
	i := idx - 1
	if i >= 0 && i < len(m.slaves) {
		m.setFocus(i)
	}
}

// ScreenCount 返回屏幕数量。
func (m *Master) ScreenCount() int {
	return len(m.slaves)
}

// FocusScreenIdx 返回当前焦点屏幕索引（1-based）。
func (m *Master) FocusScreenIdx() int {
	return m.focusIdx + 1
}

// ZoomIn 放大字体。
func (m *Master) ZoomIn() {
	select {
	case m.scaleReqCh <- scaleReq{delta: 1}:
	case <-m.done:
	default:
		debug.Warningf("ZoomIn: scale request channel full, dropping")
	}
}

// ZoomOut 缩小字体。
func (m *Master) ZoomOut() {
	select {
	case m.scaleReqCh <- scaleReq{delta: -1}:
	case <-m.done:
	default:
		debug.Warningf("ZoomOut: scale request channel full, dropping")
	}
}

// ZoomReset 重置字体到初始大小。
func (m *Master) ZoomReset() {
	select {
	case m.scaleReqCh <- scaleReq{delta: 0}:
	case <-m.done:
	default:
		debug.Warningf("ZoomReset: scale request channel full, dropping")
	}
}

// EnablePanel 启用 OSD 面板（运行期动态启用插件面板）。
// 下一帧 renderPlugins 会同步到 OSD.SetPanelLines 使 Insets 扩大。
func (m *Master) EnablePanel(side string, lines int) {
	if m.plugins == nil {
		return
	}
	m.plugins.SetPanel(side, lines)
}

// DisablePanel 禁用 OSD 面板（运行期动态禁用插件面板）。
func (m *Master) DisablePanel(side string) {
	if m.plugins == nil {
		return
	}
	m.plugins.SetPanel(side, 0)
}

// ReloadPlugins 热重载插件：清空钩子与面板状态，重新执行 init.lua + Activate。
// 须在主渲染线程调用（与 OnKey/OnRender 同线程，避免 LState 并发）。
func (m *Master) ReloadPlugins() error {
	if m.plugins == nil {
		return nil
	}
	return m.plugins.Reload()
}

// Exit 请求退出主循环（幂等，signalClose 用 closeOnce 保护）。
// 供插件层通过 vistty.exit() 调用，复用现有的两阶段关闭路径。
func (m *Master) Exit() {
	m.signalClose()
}

func (m *Master) ApplyTheme(term terminal.Theme, osd ui.OSDTheme) {
	for _, t := range m.terms {
		t.SetTheme(&term)
	}
	for _, s := range m.slaves {
		if s.osd != nil {
			s.osd.SetTheme(osd)
		}
	}
	m.osdDirty = true
	m.dirty = true
}

// 编译期断言：Master 实现 plugins.PluginContext 接口。
var _ plugins.PluginContext = (*Master)(nil)
