package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/vte"
)

var seqPool = sync.Pool{
	New: func() any { return make([]vte.Sequence, 0, 4096) },
}

type savedCursorState struct {
	row     int
	col     int
	fg      screen.Color
	bg      screen.Color
	attr    screen.Attributes
	charset charsetState
}

type Terminal struct {
	mu              sync.RWMutex
	screen          *screen.Buffer
	cursor          *screen.Cursor
	parser          *vte.Parser
	pty             *os.File
	ptyCmd          *os.Process
	hostWriter      io.Writer
	mainBuf         *screen.Buffer
	altBuf          *screen.Buffer
	done            chan struct{}
	closeOnce       sync.Once
	seqCh           chan []vte.Sequence
	eofCh           chan struct{}
	cleanupOnce     sync.Once
	opts            Options
	curFg           screen.Color
	curBg           screen.Color
	curAttr         screen.Attributes
	defFg           screen.Color
	defBg           screen.Color
	saved           savedCursorState
	scrollOffset    int
	autoWrap        bool
	cursorKeysApp  bool
	bracketedPaste  bool
	focusReporting  bool
	title           string
	charset         charsetState
	tabStops        []bool
	active          bool
	cols            int
	rows            int
}

func New(opts Options, cols, rows int) (*Terminal, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	buf := screen.NewBuffer(cols, rows)
	altBuf := screen.NewBuffer(cols, rows)
	altBuf.SetAltScreen(true)
	parser := vte.NewParser()

	ptyFile, cmdProc, err := startPty(opts.Shell, rows, cols)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	term := &Terminal{
		screen:     buf,
		cursor:     buf.Cursor(),
		parser:     parser,
		pty:        ptyFile,
		ptyCmd:     cmdProc,
		hostWriter: ptyFile,
		done:       make(chan struct{}),
		seqCh:      make(chan []vte.Sequence, 64),
		eofCh:      make(chan struct{}, 1),
		opts:        opts,
		mainBuf:     buf,
		altBuf:      altBuf,
		curFg:       screen.Color{IsDefault: true},
		curBg:       screen.Color{IsDefault: true},
		defFg:       screen.Color{R: 204, G: 204, B: 204},
		defBg:       screen.Color{R: 0, G: 0, B: 0},
		autoWrap:    true,
		charset:     newCharsetState(),
		active:      true,
		cols:        cols,
		rows:        rows,
	}
	term.initTabStops()
	return term, nil
}

func (t *Terminal) Apply(seqs []vte.Sequence) {
	t.mu.Lock()
	t.executeSequences(seqs)
	t.mu.Unlock()
}

func (t *Terminal) FeedBytes(data []byte) {
	seqs := t.parser.FeedAll(data)
	t.mu.Lock()
	t.executeSequences(seqs)
	t.mu.Unlock()
}

func (t *Terminal) Screen() *screen.Buffer {
	return t.screen
}

// ReadCells 返回当前屏幕可见区域的纯字符快照（rows 行 × cols 列）。
// 供插件层（vistty.term.read_screen）在不依赖 screen 包类型的前提下
// 读取终端字符内容。加读锁保证快照一致性。
func (t *Terminal) ReadCells() [][]rune {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rows := t.rows
	cols := t.cols
	result := make([][]rune, rows)
	buf := t.screen
	for r := 0; r < rows; r++ {
		result[r] = make([]rune, cols)
		line := buf.Line(r)
		if line == nil {
			continue
		}
		for c := 0; c < cols; c++ {
			cell := line.Cell(c)
			if cell != nil {
				result[r][c] = cell.Rune
			}
		}
	}
	return result
}

func (t *Terminal) Cursor() *screen.Cursor {
	return t.cursor
}

func (t *Terminal) SeqCh() <-chan []vte.Sequence {
	return t.seqCh
}

func ReturnSeqPool(seqs []vte.Sequence) {
	seqPool.Put(seqs[:0])
}

func (t *Terminal) EofCh() <-chan struct{} {
	return t.eofCh
}

func (t *Terminal) Done() <-chan struct{} {
	return t.done
}

func (t *Terminal) ScrollOffset() int {
	return t.scrollOffset
}

func (t *Terminal) SetScrollOffset(n int) {
	t.scrollOffset = n
}

func (t *Terminal) Active() bool {
	return t.active
}

func (t *Terminal) Cols() int {
	return t.cols
}

func (t *Terminal) Rows() int {
	return t.rows
}

func (t *Terminal) CursorKeysApp() bool {
	return t.cursorKeysApp
}

func (t *Terminal) Lock() {
	t.mu.Lock()
}

func (t *Terminal) Unlock() {
	t.mu.Unlock()
}

func (t *Terminal) RLock() {
	t.mu.RLock()
}

func (t *Terminal) RUnlock() {
	t.mu.RUnlock()
}

func (t *Terminal) Close() error {
	t.SignalClose()
	t.cleanup()
	return nil
}

func (t *Terminal) SignalClose() {
	t.closeOnce.Do(func() {
		debug.Debugf("SignalClose: closing done and pty\n")
		close(t.done)
		if t.pty != nil {
			t.pty.Close()
		}
		if t.ptyCmd != nil {
			t.ptyCmd.Signal(syscall.SIGKILL)
		}
	})
}

func (t *Terminal) Resize(cols, rows int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.mainBuf.Resize(cols, rows)
	t.altBuf.Resize(cols, rows)
	if t.cursor.Row >= rows {
		t.cursor.Row = rows - 1
	}
	if t.cursor.Col >= cols {
		t.cursor.Col = cols - 1
	}
	t.scrollOffset = 0
	t.initTabStops()
	t.cols = cols
	t.rows = rows
	t.setPtySize(rows, cols)
}

func (t *Terminal) SetPtySize(rows, cols int) {
	t.setPtySize(rows, cols)
}

func (t *Terminal) setPtySize(rows, cols int) {
	if t.pty == nil {
		return
	}
	_ = pty.Setsize(t.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (t *Terminal) cleanup() {
	t.cleanupOnce.Do(func() {
		debug.Debugf("cleanup: starting\n")
		if t.ptyCmd != nil {
			t.ptyCmd.Signal(syscall.SIGTERM)
			ch := make(chan struct{})
			go func() {
				t.ptyCmd.Wait()
				close(ch)
			}()
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
				debug.Errorf("cleanup: SIGTERM timeout, sending SIGKILL\n")
				t.ptyCmd.Signal(syscall.SIGKILL)
				<-ch
			}
		}
		debug.Debugf("cleanup: done\n")
	})
}

func (t *Terminal) PtyReadLoop() {
	defer func() {
		debug.Debugf("PtyReadLoop: exiting\n")
	}()
	buf := make([]byte, 4096)
	for {
		n, err := t.pty.Read(buf)
		if err != nil {
			debug.Errorf("PtyReadLoop: read error: %v\n", err)
			select {
			case <-t.done:
			case t.eofCh <- struct{}{}:
			}
			return
		}
		debug.Debugf("PtyReadLoop: read %d bytes: %q\n", n, string(buf[:n]))
		if t.opts.RecordWriter != nil {
			t.opts.RecordWriter.Write(buf[:n])
		}
		seqs := seqPool.Get().([]vte.Sequence)
		seqs = t.parser.FeedInto(buf[:n], seqs)
		if len(seqs) > 0 {
			debug.Debugf("PtyReadLoop: parsed %d sequences\n", len(seqs))
		}
		if len(seqs) > 0 {
			select {
			case t.seqCh <- seqs:
			case <-t.done:
				seqPool.Put(seqs[:0])
				return
			}
		} else {
			seqPool.Put(seqs[:0])
		}
	}
}

func (t *Terminal) executeSequences(seqs []vte.Sequence) {
	for _, seq := range seqs {
		switch seq.Action {
		case vte.ActionPrint:
			t.execPrint(seq)
		case vte.ActionExecute:
			t.execControl(seq)
		case vte.ActionCSI:
			t.execCSI(seq)
		case vte.ActionOSC:
			t.execOSC(seq)
		case vte.ActionESC:
			t.execESC(seq)
		case vte.ActionDCS:
		case vte.ActionIgnore:
		}
	}
}

func (t *Terminal) execPrint(seq vte.Sequence) {
	r := t.charset.current().Translate(seq.Rune)
	w := runeWidth(r)

	if t.cursor.WrapPending && t.autoWrap {
		t.cursor.Col = 0
		t.screen.LineFeed()
		t.cursor.WrapPending = false
	}

	if w == 2 && t.cursor.Col+1 >= t.screen.Cols() {
		if t.autoWrap {
			t.cursor.Col = 0
			t.screen.LineFeed()
		} else {
			return
		}
	}

	cell := t.screen.Cell(t.cursor.Row, t.cursor.Col)
	if cell != nil {
		cell.Rune = r
		cell.Fg = t.curFg
		cell.Bg = t.curBg
		cell.Attr = t.curAttr
		cell.Width = uint8(w)
	}
	if w == 2 {
		next := t.screen.Cell(t.cursor.Row, t.cursor.Col+1)
		if next != nil {
			next.Rune = 0
			next.Width = 0
			next.Fg = t.curFg
			next.Bg = t.curBg
			next.Attr = t.curAttr
		}
	}
	t.cursor.Col += w
	if t.cursor.Col >= t.screen.Cols() {
		if t.autoWrap {
			t.cursor.Col = t.screen.Cols() - 1
			t.cursor.WrapPending = true
		} else {
			t.cursor.Col = t.screen.Cols() - 1
		}
	}
}

func (t *Terminal) execControl(seq vte.Sequence) {
	cc, ok := vte.ParseControl(seq.Command)
	if !ok {
		return
	}
	switch cc {
	case vte.ControlLF, vte.ControlVT, vte.ControlFF:
		t.cursor.WrapPending = false
		t.screen.LineFeed()
	case vte.ControlCR:
		t.cursor.WrapPending = false
		t.cursor.Col = 0
	case vte.ControlBS:
		t.cursor.WrapPending = false
		if t.cursor.Col > 0 {
			t.cursor.Col--
		}
	case vte.ControlHT:
		t.cursor.WrapPending = false
		t.cursor.Col = t.nextTabStop()
	case vte.ControlBEL:
	case vte.ControlSO:
		t.charset.shiftOut()
	case vte.ControlSI:
		t.charset.shiftIn()
	case vte.ControlNUL, vte.ControlCAN, vte.ControlSUB, vte.ControlDEL:
	}
}

func (t *Terminal) execCSI(seq vte.Sequence) {
	csi := vte.ParseCSI(seq)
	switch csi.Command {
	case vte.CSISGR, vte.CSICursorStyle, vte.CSIDeviceStatusReport,
		vte.CSIDeviceAttributes, vte.CSIDeviceAttributes2, vte.CSITabClear,
		vte.CSISetCharProtection, vte.CSIUnknown:
	default:
		t.cursor.WrapPending = false
	}
	switch csi.Command {
	case vte.CSICursorUp:
		n := csiParam(csi, 0, 1)
		t.cursor.Row -= n
		if t.cursor.Row < 0 {
			t.cursor.Row = 0
		}
	case vte.CSICursorDown:
		n := csiParam(csi, 0, 1)
		t.cursor.Row += n
		if t.cursor.Row >= t.screen.Rows() {
			t.cursor.Row = t.screen.Rows() - 1
		}
	case vte.CSICursorForward:
		n := csiParam(csi, 0, 1)
		t.cursor.Col += n
		if t.cursor.Col >= t.screen.Cols() {
			t.cursor.Col = t.screen.Cols() - 1
		}
	case vte.CSICursorBackward:
		n := csiParam(csi, 0, 1)
		t.cursor.Col -= n
		if t.cursor.Col < 0 {
			t.cursor.Col = 0
		}
	case vte.CSICursorNextLine:
		n := csiParam(csi, 0, 1)
		t.cursor.Row += n
		if t.cursor.Row >= t.screen.Rows() {
			t.cursor.Row = t.screen.Rows() - 1
		}
		t.cursor.Col = 0
	case vte.CSICursorPrevLine:
		n := csiParam(csi, 0, 1)
		t.cursor.Row -= n
		if t.cursor.Row < 0 {
			t.cursor.Row = 0
		}
		t.cursor.Col = 0
	case vte.CSICursorHorizontalAbsolute:
		col := csiParam(csi, 0, 1) - 1
		if col < 0 {
			col = 0
		}
		if col >= t.screen.Cols() {
			col = t.screen.Cols() - 1
		}
		t.cursor.Col = col
	case vte.CSICursorPosition:
		row := csiParam(csi, 0, 1) - 1
		col := csiParam(csi, 1, 1) - 1
		if row < 0 {
			row = 0
		}
		if col < 0 {
			col = 0
		}
		if row >= t.screen.Rows() {
			row = t.screen.Rows() - 1
		}
		if col >= t.screen.Cols() {
			col = t.screen.Cols() - 1
		}
		t.cursor.Row = row
		t.cursor.Col = col
	case vte.CSILinePositionAbsolute:
		row := csiParam(csi, 0, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= t.screen.Rows() {
			row = t.screen.Rows() - 1
		}
		t.cursor.Row = row
	case vte.CSIEraseInDisplay:
		n := csiParam(csi, 0, 0)
		t.eraseDisplay(n)
	case vte.CSIEraseInLine:
		n := csiParam(csi, 0, 0)
		t.eraseLine(n)
	case vte.CSIInsertLines:
		n := csiParam(csi, 0, 1)
		t.screen.ScrollDown(n)
	case vte.CSIDeleteLines:
		n := csiParam(csi, 0, 1)
		t.screen.ScrollUp(n)
	case vte.CSIDeleteChars:
		n := csiParam(csi, 0, 1)
		t.deleteChars(n)
	case vte.CSIInsertChars:
		n := csiParam(csi, 0, 1)
		t.insertChars(n)
	case vte.CSIScrollUp:
		n := csiParam(csi, 0, 1)
		t.screen.ScrollUp(n)
	case vte.CSIScrollDown:
		n := csiParam(csi, 0, 1)
		t.screen.ScrollDown(n)
	case vte.CSISetTopBottomMargin:
		if csi.NParams >= 2 {
			t.screen.SetScrollRegion(csi.Params[0]-1, csi.Params[1]-1)
		} else {
			t.screen.SetScrollRegion(0, t.screen.Rows()-1)
		}
		t.cursor.Row = t.screen.ScrollTop()
		t.cursor.Col = 0
	case vte.CSICursorStyle:
		style := csiParam(csi, 0, 0)
		switch style {
		case 0, 1:
			t.cursor.SetStyle(screen.CursorBlock)
			t.cursor.Blinking = true
		case 2:
			t.cursor.SetStyle(screen.CursorBlock)
			t.cursor.Blinking = false
		case 3:
			t.cursor.SetStyle(screen.CursorUnderline)
			t.cursor.Blinking = true
		case 4:
			t.cursor.SetStyle(screen.CursorUnderline)
			t.cursor.Blinking = false
		case 5:
			t.cursor.SetStyle(screen.CursorBar)
			t.cursor.Blinking = true
		case 6:
			t.cursor.SetStyle(screen.CursorBar)
			t.cursor.Blinking = false
		}
	case vte.CSICursorShow:
		t.cursor.Show()
	case vte.CSICursorHide:
		t.cursor.Hide()
	case vte.CSISaveCursor:
		t.saveCursor()
	case vte.CSIRestoreCursor:
		t.restoreCursor()
	case vte.CSICursorHorizontalTab:
		n := csiParam(csi, 0, 1)
		for i := 0; i < n; i++ {
			stop := t.nextTabStop()
			if stop <= t.cursor.Col {
				t.cursor.Col = t.screen.Cols() - 1
				break
			}
			t.cursor.Col = stop
		}
	case vte.CSICursorBackTab:
		n := csiParam(csi, 0, 1)
		for i := 0; i < n; i++ {
			t.cursor.Col = t.prevTabStop()
		}
	case vte.CSIEraseChars:
		n := csiParam(csi, 0, 1)
		t.eraseChars(n)
	case vte.CSIDeviceStatusReport:
		t.handleDSR(csi)
	case vte.CSIDeviceAttributes:
		t.PtyWrite([]byte("\x1b[?62;4c"))
	case vte.CSIDeviceAttributes2:
		t.PtyWrite([]byte("\x1b[>0;0;0c"))
	case vte.CSITabClear:
		n := csiParam(csi, 0, 0)
		if n == 0 {
			t.clearTabStop()
		} else if n == 3 {
			t.clearAllTabStops()
		}
	case vte.CSISGR:
		t.applySGR(csi.Params[:csi.NParams])
	case vte.CSISetMode, vte.CSIResetMode, vte.CSIScreenMode:
		t.handleMode(csi)
	}
}

func (t *Terminal) handleMode(csi vte.CSISequence) {
	isSet := csi.Command == vte.CSISetMode
	for _, p := range csi.Params[:csi.NParams] {
		switch p {
		case 1:
			t.cursorKeysApp = isSet
		case 7:
			t.autoWrap = isSet
		case 25:
			if isSet {
				t.cursor.Show()
			} else {
				t.cursor.Hide()
			}
		case 47, 1047:
			if isSet {
				if p == 1047 {
					t.altBuf.SetEraseCell(t.curFg, t.curBg, t.curAttr)
					t.altBuf.ClearAll()
				}
				t.screen = t.altBuf
				t.cursor = t.altBuf.Cursor()
			} else {
				if p == 1047 {
					t.altBuf.SetEraseCell(t.curFg, t.curBg, t.curAttr)
					t.altBuf.ClearAll()
				}
				t.screen = t.mainBuf
				t.cursor = t.mainBuf.Cursor()
			}
		case 1048:
			if isSet {
				t.saveCursor()
			} else {
				t.restoreCursor()
			}
		case 1049:
			if isSet {
				t.saveCursor()
				t.screen = t.altBuf
				t.cursor = t.altBuf.Cursor()
				t.altBuf.SetEraseCell(t.curFg, t.curBg, t.curAttr)
				t.altBuf.ClearAll()
				t.cursor.Row = 0
				t.cursor.Col = 0
				t.scrollOffset = 0
			} else {
				t.screen = t.mainBuf
				t.cursor = t.mainBuf.Cursor()
				t.restoreCursor()
				t.scrollOffset = 0
			}
		case 1004:
			t.focusReporting = isSet
		case 2004:
			t.bracketedPaste = isSet
		}
	}
}

func (t *Terminal) execOSC(seq vte.Sequence) {
	osc := vte.ParseOSC(seq)
	switch osc.Command {
	case vte.OSCSetWindowTitle:
		t.setTitle(osc.Data)
	case vte.OSCSetIconTitle:
	case vte.OSCSetClipboard:
	case vte.OSCSetWorkingDir:
	case vte.OSCHyperlink:
	case vte.OSCFgColor:
		t.handleOSCColor(osc.Data, true)
	case vte.OSCBgColor:
		t.handleOSCColor(osc.Data, false)
	case vte.OSCUnknown:
	}
}

func (t *Terminal) handleOSCColor(data string, isFg bool) {
	if data == "?" {
		var col screen.Color
		var cmdNum int
		if isFg {
			col = t.defFg
			cmdNum = 10
		} else {
			col = t.defBg
			cmdNum = 11
		}
		resp := fmt.Sprintf("\x1b]%d;rgb:%04x/%04x/%04x\x07", cmdNum, uint16(col.R)*0x0101, uint16(col.G)*0x0101, uint16(col.B)*0x0101)
		t.PtyWrite([]byte(resp))
		return
	}
	c, ok := parseColorSpec(data)
	if !ok {
		return
	}
	if isFg {
		t.defFg = c
	} else {
		t.defBg = c
	}
	if t.opts.OnDefaultColor != nil {
		t.opts.OnDefaultColor(t.defFg, t.defBg)
	}
}

func (t *Terminal) execESC(seq vte.Sequence) {
	esc := vte.ParseESC(seq)
	switch esc.Command {
	case vte.ESCDesignateG0, vte.ESCDesignateG1, vte.ESCTabSet,
		vte.ESCDeckpam, vte.ESCDeckpnm, vte.ESCUnknown:
	default:
		t.cursor.WrapPending = false
	}
	switch esc.Command {
	case vte.ESCResetState:
		t.saveCursor()
	case vte.ESCRestoreState:
		t.restoreCursor()
	case vte.ESCIndex:
		t.screen.LineFeed()
	case vte.ESCNextLine:
		t.screen.LineFeed()
		t.cursor.Col = 0
	case vte.ESCReverseIndex:
		t.screen.ReverseIndex()
	case vte.ESCTabSet:
		t.setTabStop()
	case vte.ESCDeckpam:
	case vte.ESCDeckpnm:
	case vte.ESCDesignateG0:
		t.charset.designateG0(esc.Charset)
	case vte.ESCDesignateG1:
		t.charset.designateG1(esc.Charset)
	case vte.ESCFullReset:
		t.fullReset()
	case vte.ESCUnknown:
	}
}

func (t *Terminal) setTitle(title string) {
	t.title = title
	if t.opts.OnTitle != nil {
		t.opts.OnTitle(title)
	}
}

func (t *Terminal) saveCursor() {
	t.saved = savedCursorState{
		row:     t.cursor.Row,
		col:     t.cursor.Col,
		fg:      t.curFg,
		bg:      t.curBg,
		attr:    t.curAttr,
		charset: t.charset,
	}
}

func (t *Terminal) restoreCursor() {
	t.cursor.Row = t.saved.row
	t.cursor.Col = t.saved.col
	t.curFg = t.saved.fg
	t.curBg = t.saved.bg
	t.curAttr = t.saved.attr
	t.charset = t.saved.charset
}

func (t *Terminal) fullReset() {
	t.curFg = screen.Color{IsDefault: true}
	t.curBg = screen.Color{IsDefault: true}
	t.curAttr = 0
	t.defFg = screen.Color{R: 204, G: 204, B: 204}
	t.defBg = screen.Color{R: 0, G: 0, B: 0}
	if t.opts.OnDefaultColor != nil {
		t.opts.OnDefaultColor(t.defFg, t.defBg)
	}
	t.screen.SetEraseCell(t.curFg, t.curBg, t.curAttr)
	t.screen.ClearAll()
	t.screen.SetScrollRegion(0, t.screen.Rows()-1)
	t.cursor.Row = 0
	t.cursor.Col = 0
	t.saved = savedCursorState{}
	t.charset = newCharsetState()
	t.autoWrap = true
	t.scrollOffset = 0
	t.initTabStops()
}

func (t *Terminal) initTabStops() {
	cols := t.screen.Cols()
	if cap(t.tabStops) >= cols {
		t.tabStops = t.tabStops[:cols]
	} else {
		t.tabStops = make([]bool, cols)
	}
	for i := range t.tabStops {
		t.tabStops[i] = i%8 == 0
	}
}

func (t *Terminal) setTabStop() {
	if t.cursor.Col < len(t.tabStops) {
		t.tabStops[t.cursor.Col] = true
	}
}

func (t *Terminal) clearTabStop() {
	if t.cursor.Col < len(t.tabStops) {
		t.tabStops[t.cursor.Col] = false
	}
}

func (t *Terminal) clearAllTabStops() {
	for i := range t.tabStops {
		t.tabStops[i] = false
	}
}

func (t *Terminal) nextTabStop() int {
	for i := t.cursor.Col + 1; i < len(t.tabStops); i++ {
		if t.tabStops[i] {
			return i
		}
	}
	return t.screen.Cols() - 1
}

func (t *Terminal) prevTabStop() int {
	for i := t.cursor.Col - 1; i >= 0; i-- {
		if t.tabStops[i] {
			return i
		}
	}
	return 0
}

func (t *Terminal) eraseDisplay(n int) {
	switch n {
	case 0:
		t.screen.ClearRect(screen.Rect{X: t.cursor.Col, Y: t.cursor.Row, W: t.screen.Cols() - t.cursor.Col, H: 1})
		if t.cursor.Row+1 < t.screen.Rows() {
			t.screen.ClearRect(screen.Rect{X: 0, Y: t.cursor.Row + 1, W: t.screen.Cols(), H: t.screen.Rows() - t.cursor.Row - 1})
		}
	case 1:
		if t.cursor.Row > 0 {
			t.screen.ClearRect(screen.Rect{X: 0, Y: 0, W: t.screen.Cols(), H: t.cursor.Row})
		}
		t.screen.ClearRect(screen.Rect{X: 0, Y: t.cursor.Row, W: t.cursor.Col + 1, H: 1})
	case 2:
		t.screen.Clear()
	case 3:
		t.screen.History().Clear()
	}
}

func (t *Terminal) eraseLine(n int) {
	switch n {
	case 0:
		t.screen.ClearRect(screen.Rect{X: t.cursor.Col, Y: t.cursor.Row, W: t.screen.Cols() - t.cursor.Col, H: 1})
	case 1:
		t.screen.ClearRect(screen.Rect{X: 0, Y: t.cursor.Row, W: t.cursor.Col + 1, H: 1})
	case 2:
		t.screen.ClearRect(screen.Rect{X: 0, Y: t.cursor.Row, W: t.screen.Cols(), H: 1})
	}
}

func (t *Terminal) eraseChars(n int) {
	for i := 0; i < n && t.cursor.Col+i < t.screen.Cols(); i++ {
		cell := t.screen.Cell(t.cursor.Row, t.cursor.Col+i)
		if cell != nil {
			cell.Erase(t.curFg, t.curBg, t.curAttr)
		}
	}
}

func (t *Terminal) handleDSR(csi vte.CSISequence) {
	n := csiParam(csi, 0, 0)
	switch n {
	case 5:
		t.PtyWrite([]byte("\x1b[0n"))
	case 6:
		row := t.cursor.Row + 1
		col := t.cursor.Col + 1
		if csi.Private {
			t.PtyWrite([]byte(fmt.Sprintf("\x1b[?%d;%d;1R", row, col)))
		} else {
			t.PtyWrite([]byte(fmt.Sprintf("\x1b[%d;%dR", row, col)))
		}
	}
}

func (t *Terminal) deleteChars(n int) {
	row := t.screen.Line(t.cursor.Row)
	if row == nil {
		return
	}
	col := t.cursor.Col
	for i := col; i < t.screen.Cols(); i++ {
		src := i + n
		dst := i
		if src < t.screen.Cols() {
			srcCell := row.Cell(src)
			dstCell := row.Cell(dst)
			if srcCell != nil && dstCell != nil {
				*dstCell = *srcCell
			}
		} else {
			dstCell := row.Cell(dst)
			if dstCell != nil {
				dstCell.Erase(t.curFg, t.curBg, t.curAttr)
			}
		}
	}
}

func (t *Terminal) insertChars(n int) {
	row := t.screen.Line(t.cursor.Row)
	if row == nil {
		return
	}
	col := t.cursor.Col
	for i := t.screen.Cols() - 1; i >= col; i-- {
		src := i - n
		dst := i
		if src >= col {
			srcCell := row.Cell(src)
			dstCell := row.Cell(dst)
			if srcCell != nil && dstCell != nil {
				*dstCell = *srcCell
			}
		} else {
			dstCell := row.Cell(dst)
			if dstCell != nil {
				dstCell.Erase(t.curFg, t.curBg, t.curAttr)
			}
		}
	}
}

func (t *Terminal) applySGR(params []int) {
	sgrs := vte.ParseSGR(params)
	for _, sgr := range sgrs {
		switch sgr.Attr {
		case vte.SGRReset:
			t.curFg = screen.Color{IsDefault: true}
			t.curBg = screen.Color{IsDefault: true}
			t.curAttr = 0
		case vte.SGRBold:
			t.curAttr |= screen.AttrBold
		case vte.SGRDim:
			t.curAttr |= screen.AttrDim
		case vte.SGRItalic:
			t.curAttr |= screen.AttrItalic
		case vte.SGRUnderline:
			t.curAttr |= screen.AttrUnderline
		case vte.SGRBlink:
			t.curAttr |= screen.AttrBlink
		case vte.SGRReverse:
			t.curAttr |= screen.AttrReverse
		case vte.SGRCrossedOut:
			t.curAttr |= screen.AttrCrossedOut
		case vte.SGRBoldOff:
			t.curAttr &^= screen.AttrBold
		case vte.SGRDimOff:
			t.curAttr &^= screen.AttrDim
		case vte.SGRItalicOff:
			t.curAttr &^= screen.AttrItalic
		case vte.SGRUnderlineOff:
			t.curAttr &^= screen.AttrUnderline
		case vte.SGRBlinkOff:
			t.curAttr &^= screen.AttrBlink
		case vte.SGRReverseOff:
			t.curAttr &^= screen.AttrReverse
		case vte.SGRCrossedOutOff:
			t.curAttr &^= screen.AttrCrossedOut
		case vte.SGRForegroundColor8:
			t.curFg = ansiColor(sgr.ColorIdx)
		case vte.SGRBackgroundColor8:
			t.curBg = ansiColor(sgr.ColorIdx)
		case vte.SGRForegroundColor256:
			t.curFg = color256(sgr.ColorIdx)
		case vte.SGRBackgroundColor256:
			t.curBg = color256(sgr.ColorIdx)
		case vte.SGRForegroundColorRGB:
			t.curFg = screen.Color{R: sgr.R, G: sgr.G, B: sgr.B}
		case vte.SGRBackgroundColorRGB:
			t.curBg = screen.Color{R: sgr.R, G: sgr.G, B: sgr.B}
		case vte.SGRForegroundColorReset:
			t.curFg = screen.Color{IsDefault: true}
		case vte.SGRBackgroundColorReset:
			t.curBg = screen.Color{IsDefault: true}
		}
	}
	t.screen.SetEraseCell(t.curFg, t.curBg, t.curAttr)
}

func (t *Terminal) HandleKey(ev platform.KeyEvent) {
	if t.scrollOffset > 0 && !platform.LookupModifierCode(ev.Code) {
		if !(ev.Mods&platform.ModShift != 0 && (ev.Code == 104 || ev.Code == 109)) {
			t.scrollOffset = 0
		}
	}
	if ev.Code != 0 && ev.Rune == 0 {
		if ev.Mods&platform.ModShift != 0 {
			switch ev.Code {
			case 104:
				histLen := t.screen.History().Len()
				if t.scrollOffset < histLen {
					t.scrollOffset++
				}
				return
			case 109:
				if t.scrollOffset > 0 {
					t.scrollOffset--
				}
				return
			}
		}
		t.WriteKeyEscape(ev.Code, ev.Mods)
		return
	}

	if ev.Mods&platform.ModCtrl != 0 {
		switch ev.Rune {
		case 'C', 'c':
			t.PtyWrite([]byte{0x03})
			return
		case 'D', 'd':
			t.PtyWrite([]byte{0x04})
			return
		case 'Z', 'z':
			t.PtyWrite([]byte{0x1a})
			return
		}
	}

	if ev.Rune != 0 {
		var buf [4]byte
		n := utf8.EncodeRune(buf[:], ev.Rune)
		t.PtyWrite(buf[:n])
	}
}

func (t *Terminal) PtyWrite(b []byte) {
	if t.hostWriter == nil {
		return
	}
	if _, err := t.hostWriter.Write(b); err != nil {
		t.SignalClose()
	}
}

func (t *Terminal) WriteKeyEscape(code uint16, mods platform.Modifiers) {
	var seq string
	prefix := ""

	if mods&platform.ModAlt != 0 {
		prefix = "\x1b"
	}

	switch code {
	case 103:
		if t.cursorKeysApp {
			seq = "\x1bOA"
		} else {
			seq = "\x1b[A"
		}
	case 108:
		if t.cursorKeysApp {
			seq = "\x1bOB"
		} else {
			seq = "\x1b[B"
		}
	case 106:
		if t.cursorKeysApp {
			seq = "\x1bOC"
		} else {
			seq = "\x1b[C"
		}
	case 105:
		if t.cursorKeysApp {
			seq = "\x1bOD"
		} else {
			seq = "\x1b[D"
		}
	case 102:
		seq = "\x1b[H"
	case 107:
		seq = "\x1b[F"
	case 104:
		seq = "\x1b[5~"
	case 109:
		seq = "\x1b[6~"
	case 110:
		seq = "\x1b[2~"
	case 111:
		seq = "\x1b[3~"
	case 14:
		seq = "\x7f"
	case 9:
		seq = "\t"
	case 36:
		seq = "\r"
	default:
		return
	}

	if prefix != "" {
		t.PtyWrite([]byte(prefix))
	}
	t.PtyWrite([]byte(seq))
}

func startPty(shell string, rows, cols int) (*os.File, *os.Process, error) {
	ws := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, nil, err
	}
	return ptmx, cmd.Process, nil
}

func csiParam(csi vte.CSISequence, idx, def int) int {
	if idx < csi.NParams && csi.Params[idx] != 0 {
		return csi.Params[idx]
	}
	return def
}

func ansiColor(idx int) screen.Color {
	palette := [16]screen.Color{
		{R: 0, G: 0, B: 0},
		{R: 205, G: 0, B: 0},
		{R: 0, G: 205, B: 0},
		{R: 205, G: 205, B: 0},
		{R: 0, G: 0, B: 238},
		{R: 205, G: 0, B: 205},
		{R: 0, G: 205, B: 205},
		{R: 229, G: 229, B: 229},
		{R: 127, G: 127, B: 127},
		{R: 255, G: 0, B: 0},
		{R: 0, G: 255, B: 0},
		{R: 255, G: 255, B: 0},
		{R: 92, G: 92, B: 255},
		{R: 255, G: 0, B: 255},
		{R: 0, G: 255, B: 255},
		{R: 255, G: 255, B: 255},
	}
	if idx >= 0 && idx < len(palette) {
		return palette[idx]
	}
	return screen.Color{IsDefault: true}
}

func color256(idx int) screen.Color {
	if idx < 16 {
		return ansiColor(idx)
	}
	if idx >= 16 && idx < 232 {
		i := idx - 16
		r, g, b := 0, 0, 0
		if i >= 36 {
			r = 55 + 40*((i/36)%6)
		}
		if i >= 6 {
			g = 55 + 40*((i/6)%6)
		}
		b = 55 + 40*(i%6)
		if r > 255 {
			r = 255
		}
		if g > 255 {
			g = 255
		}
		if b > 255 {
			b = 255
		}
		return screen.Color{R: uint8(r), G: uint8(g), B: uint8(b)}
	}
	if idx >= 232 && idx < 256 {
		v := 8 + (idx-232)*10
		if v > 255 {
			v = 255
		}
		return screen.Color{R: uint8(v), G: uint8(v), B: uint8(v)}
	}
	return screen.Color{IsDefault: true}
}

func parseColorSpec(spec string) (screen.Color, bool) {
	spec = strings.TrimSpace(spec)

	if strings.HasPrefix(spec, "rgb:") {
		parts := strings.Split(strings.TrimPrefix(spec, "rgb:"), "/")
		if len(parts) != 3 {
			return screen.Color{}, false
		}
		r, ok1 := parseHexChannel(parts[0])
		g, ok2 := parseHexChannel(parts[1])
		b, ok3 := parseHexChannel(parts[2])
		if !ok1 || !ok2 || !ok3 {
			return screen.Color{}, false
		}
		return screen.Color{R: r, G: g, B: b}, true
	}

	if strings.HasPrefix(spec, "#") {
		hex := strings.TrimPrefix(spec, "#")
		switch len(hex) {
		case 3:
			r, err1 := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
			g, err2 := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
			b, err3 := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
			if err1 != nil || err2 != nil || err3 != nil {
				return screen.Color{}, false
			}
			return screen.Color{R: uint8(r), G: uint8(g), B: uint8(b)}, true
		case 6:
			r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
			g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
			b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
			if err1 != nil || err2 != nil || err3 != nil {
				return screen.Color{}, false
			}
			return screen.Color{R: uint8(r), G: uint8(g), B: uint8(b)}, true
		}
	}

	return screen.Color{}, false
}

func parseHexChannel(s string) (uint8, bool) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	switch len(s) {
	case 1:
		return uint8(v * 0x11), true
	case 2:
		return uint8(v), true
	case 3:
		return uint8(v >> 4), true
	case 4:
		return uint8(v >> 8), true
	default:
		return 0, false
	}
}

func (t *Terminal) SetOnDefaultColor(f func(fg, bg screen.Color)) {
	t.opts.OnDefaultColor = f
}

func (t *Terminal) Title() string {
	return t.title
}

func (t *Terminal) SetOnTitle(f func(string)) {
	t.opts.OnTitle = f
}
