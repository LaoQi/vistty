package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/vte"
)

type Terminal struct {
	screen     *screen.Buffer
	cursor     *screen.Cursor
	parser     *vte.Parser
	pty        *os.File
	ptyCmd     *os.Process
	compositor *render.Compositor
	surface    platform.Surface
	input      platform.InputSource
	backend    platform.Backend
	face       font.Face
	done       chan struct{}
	closeOnce  sync.Once
	opts       Options
	curFg      screen.Color
	curBg      screen.Color
	curAttr    screen.Attributes
	savedRow   int
	savedCol   int
}

func New(backend platform.Backend, opts Options) (*Terminal, error) {
	surface, err := backend.CreateSurface(opts.Width, opts.Height)
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}

	input, err := backend.CreateInputSource()
	if err != nil {
		surface.Close()
		return nil, fmt.Errorf("create input source: %w", err)
	}

	fontPath := opts.FontPath
	if fontPath == "" {
		fontPath = defaultFontPath()
	}
	face, err := font.NewOpenTypeFaceFromFile(fontPath, opts.FontSize, 72)
	if err != nil {
		input.Close()
		surface.Close()
		return nil, fmt.Errorf("load font: %w", err)
	}

	m := face.Metrics()
	w, h := surface.Size()
	cols := w / m.Width
	rows := 0
	if m.Height > 0 {
		rows = h / m.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	buf := screen.NewBuffer(cols, rows)
	compositor := render.NewCompositor(surface, face)
	parser := vte.NewParser()

	ptyFile, cmdProc, err := startPty(opts.Shell, rows, cols)
	if err != nil {
		face.Close()
		input.Close()
		surface.Close()
		return nil, fmt.Errorf("start pty: %w", err)
	}

	return &Terminal{
		screen:     buf,
		cursor:     buf.Cursor(),
		parser:     parser,
		pty:        ptyFile,
		ptyCmd:     cmdProc,
		compositor: compositor,
		surface:    surface,
		input:      input,
		backend:    backend,
		face:       face,
		done:       make(chan struct{}),
		opts:       opts,
		curFg:      screen.Color{IsDefault: true},
		curBg:      screen.Color{IsDefault: true},
	}, nil
}

func (t *Terminal) Run() error {
	go t.ptyReadLoop()
	go t.inputLoop()
	go t.renderLoop()
	go t.signalLoop()
	go t.backend.Run(func() {})

	select {
	case <-t.done:
	case <-t.backend.Done():
		t.Close()
		<-t.done
	}
	return nil
}

func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		close(t.done)

		if t.pty != nil {
			t.pty.Close()
		}
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
				t.ptyCmd.Signal(syscall.SIGKILL)
				t.ptyCmd.Wait()
			}
		}
		if t.input != nil {
			t.input.Close()
		}
		if t.compositor != nil {
			t.compositor.Close()
		}
		if t.face != nil {
			t.face.Close()
		}
	})
	return nil
}

func (t *Terminal) ptyReadLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-t.done:
			return
		default:
		}
		n, err := t.pty.Read(buf)
		if err != nil {
			close(t.done)
			return
		}
		seqs := t.parser.FeedAll(buf[:n])
		t.executeSequences(seqs)
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
	cell := t.screen.Cell(t.cursor.Row, t.cursor.Col)
	if cell != nil {
		cell.Rune = seq.Rune
		cell.Fg = t.curFg
		cell.Bg = t.curBg
		cell.Attr = t.curAttr
		cell.MarkDirty()
	}
	t.cursor.Col++
	if t.cursor.Col >= t.screen.Cols() {
		t.cursor.Col = 0
		t.cursor.Row++
		if t.cursor.Row > t.screen.Rows()-1 {
			t.screen.ScrollUp(1)
			t.cursor.Row = t.screen.Rows() - 1
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
		t.cursor.Row++
		if t.cursor.Row > t.screen.Rows()-1 {
			t.screen.ScrollUp(1)
			t.cursor.Row = t.screen.Rows() - 1
		}
	case vte.ControlCR:
		t.cursor.Col = 0
	case vte.ControlBS:
		if t.cursor.Col > 0 {
			t.cursor.Col--
		}
	case vte.ControlHT:
		tabStop := 8
		nextCol := ((t.cursor.Col / tabStop) + 1) * tabStop
		if nextCol >= t.screen.Cols() {
			nextCol = t.screen.Cols() - 1
		}
		t.cursor.Col = nextCol
	case vte.ControlBEL:
	case vte.ControlNUL, vte.ControlSO, vte.ControlSI,
		vte.ControlCAN, vte.ControlSUB, vte.ControlDEL:
	}
}

func (t *Terminal) execCSI(seq vte.Sequence) {
	csi := vte.ParseCSI(seq)
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
		for i := 0; i < n; i++ {
			t.screen.ScrollDown(1)
		}
	case vte.CSIDeleteLines:
		n := csiParam(csi, 0, 1)
		for i := 0; i < n; i++ {
			t.screen.ScrollUp(1)
		}
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
		if len(csi.Params) >= 2 {
			t.screen.SetScrollRegion(csi.Params[0]-1, csi.Params[1]-1)
		} else {
			t.screen.SetScrollRegion(0, t.screen.Rows()-1)
		}
	case vte.CSICursorStyle:
		style := csiParam(csi, 0, 0)
		switch style {
		case 0, 1, 2:
			t.cursor.SetStyle(screen.CursorBlock)
		case 3, 4:
			t.cursor.SetStyle(screen.CursorUnderline)
		case 5, 6:
			t.cursor.SetStyle(screen.CursorBar)
		}
	case vte.CSICursorShow:
		t.cursor.Show()
	case vte.CSICursorHide:
		t.cursor.Hide()
	case vte.CSISaveCursor:
		t.savedRow = t.cursor.Row
		t.savedCol = t.cursor.Col
	case vte.CSIRestoreCursor:
		t.cursor.Row = t.savedRow
		t.cursor.Col = t.savedCol
	case vte.CSICursorHorizontalTab:
		n := csiParam(csi, 0, 1)
		for i := 0; i < n; i++ {
			tabStop := 8
			nextCol := ((t.cursor.Col / tabStop) + 1) * tabStop
			if nextCol >= t.screen.Cols() {
				nextCol = t.screen.Cols() - 1
				break
			}
			t.cursor.Col = nextCol
		}
	case vte.CSICursorBackTab:
		n := csiParam(csi, 0, 1)
		for i := 0; i < n; i++ {
			tabStop := 8
			prevCol := ((t.cursor.Col - 1) / tabStop) * tabStop
			if prevCol < 0 {
				prevCol = 0
			}
			t.cursor.Col = prevCol
		}
	case vte.CSISGR:
		t.applySGR(csi.Params)
	case vte.CSISetMode, vte.CSIResetMode, vte.CSIScreenMode:
	}
}

func (t *Terminal) execOSC(seq vte.Sequence) {
	osc := vte.ParseOSC(seq)
	switch osc.Command {
	case vte.OSCSetWindowTitle:
	case vte.OSCSetIconTitle:
	case vte.OSCSetClipboard:
	case vte.OSCSetWorkingDir:
	case vte.OSCHyperlink:
	case vte.OSCColorQuery:
	case vte.OSCUnknown:
	}
}

func (t *Terminal) execESC(seq vte.Sequence) {
	esc := vte.ParseESC(seq)
	switch esc.Command {
	case vte.ESCResetState:
		t.savedRow = t.cursor.Row
		t.savedCol = t.cursor.Col
	case vte.ESCRestoreState:
		t.cursor.Row = t.savedRow
		t.cursor.Col = t.savedCol
	case vte.ESCIndex:
		t.cursor.Row++
		if t.cursor.Row > t.screen.Rows()-1 {
			t.screen.ScrollUp(1)
			t.cursor.Row = t.screen.Rows() - 1
		}
	case vte.ESCNextLine:
		t.cursor.Row++
		if t.cursor.Row > t.screen.Rows()-1 {
			t.screen.ScrollUp(1)
			t.cursor.Row = t.screen.Rows() - 1
		}
		t.cursor.Col = 0
	case vte.ESCReverseIndex:
		if t.cursor.Row == 0 {
			t.screen.ScrollDown(1)
		} else {
			t.cursor.Row--
		}
	case vte.ESCTabSet:
	case vte.ESCDeckpam:
	case vte.ESCDeckpnm:
	case vte.ESCUnknown:
	}
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
				dstCell.MarkDirty()
			}
		} else {
			dstCell := row.Cell(dst)
			if dstCell != nil {
				dstCell.Clear()
				dstCell.MarkDirty()
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
				dstCell.MarkDirty()
			}
		} else {
			dstCell := row.Cell(dst)
			if dstCell != nil {
				dstCell.Clear()
				dstCell.MarkDirty()
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
}

func (t *Terminal) inputLoop() {
	for {
		select {
		case ev := <-t.input.KeyEvents():
			if ev.State != platform.KeyPress {
				continue
			}
			t.handleKey(ev)
		case <-t.done:
			return
		}
	}
}

func (t *Terminal) handleKey(ev platform.KeyEvent) {
	if ev.Mods&platform.ModCtrl != 0 {
		switch ev.Rune {
		case 'C', 'c':
			if t.ptyCmd != nil {
				t.ptyCmd.Signal(syscall.SIGINT)
			}
			return
		case 'D', 'd':
			t.pty.Write([]byte{0x04})
			return
		case 'Z', 'z':
			if t.ptyCmd != nil {
				t.ptyCmd.Signal(syscall.SIGTSTP)
			}
			return
		}
	}

	if ev.Code != 0 && ev.Rune == 0 {
		t.writeKeyEscape(ev.Code, ev.Mods)
		return
	}

	if ev.Rune != 0 {
		t.pty.Write([]byte{byte(ev.Rune)})
	}
}

func (t *Terminal) writeKeyEscape(code uint16, mods platform.Modifiers) {
	var seq string
	prefix := ""

	if mods&platform.ModAlt != 0 {
		prefix = "\x1b"
	}

	switch code {
	case 103:
		seq = "\x1b[A"
	case 108:
		seq = "\x1b[B"
	case 106:
		seq = "\x1b[C"
	case 107:
		seq = "\x1b[D"
	case 104:
		seq = "\x1b[2~"
	case 9:
		seq = "\t"
	case 36:
		seq = "\n"
	default:
		return
	}

	if prefix != "" {
		t.pty.Write([]byte(prefix))
	}
	t.pty.Write([]byte(seq))
}

func (t *Terminal) renderLoop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.compositor.Render(t.screen)
		case <-t.done:
			return
		}
	}
}

func (t *Terminal) signalLoop() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	select {
	case <-ch:
		t.Close()
	case <-t.done:
	}
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

func defaultFontPath() string {
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func csiParam(csi vte.CSISequence, idx, def int) int {
	if idx < len(csi.Params) && csi.Params[idx] != 0 {
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
