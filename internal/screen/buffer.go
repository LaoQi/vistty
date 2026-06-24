package screen

type Rect struct {
	X, Y, W, H int
}

type Buffer struct {
	lines      []*Line
	cols       int
	rows       int
	scrollTop  int
	scrollBot  int
	cursor     *Cursor
	history    *History
	altScreen  bool
	eraseCell  Cell
}

func NewBuffer(cols, rows int) *Buffer {
	b := &Buffer{
		cols:      cols,
		rows:      rows,
		scrollTop: 0,
		scrollBot: rows - 1,
		cursor:    NewCursor(),
		history:   NewHistory(1000),
		eraseCell: NewCell(),
	}
	b.lines = make([]*Line, rows)
	for i := range b.lines {
		b.lines[i] = NewLine(cols)
	}
	return b
}

func (b *Buffer) Cell(row, col int) *Cell {
	if row < 0 || row >= b.rows || col < 0 || col >= b.cols {
		return nil
	}
	return b.lines[row].Cell(col)
}

func (b *Buffer) Line(row int) *Line {
	if row < 0 || row >= b.rows {
		return nil
	}
	return b.lines[row]
}

func (b *Buffer) Cols() int {
	return b.cols
}

func (b *Buffer) Rows() int {
	return b.rows
}

func (b *Buffer) Cursor() *Cursor {
	return b.cursor
}

func (b *Buffer) History() *History {
	return b.history
}

func (b *Buffer) SetEraseCell(fg, bg Color, attr Attributes) {
	b.eraseCell = Cell{
		Rune:  ' ',
		Width: 1,
		Fg:    fg,
		Bg:    bg,
		Attr:  attr,
	}
}

func (b *Buffer) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	if n > b.scrollBot-b.scrollTop+1 {
		n = b.scrollBot - b.scrollTop + 1
	}
	for i := b.scrollTop; i < b.scrollTop+n; i++ {
		if b.lines[i] != nil && !b.altScreen {
			b.history.Push(b.lines[i])
		}
	}
	copy(b.lines[b.scrollTop:], b.lines[b.scrollTop+n:b.scrollBot+1])
	for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
		b.lines[i] = NewLine(b.cols)
		b.lines[i].Fill(b.eraseCell)
	}
}

func (b *Buffer) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	if n > b.scrollBot-b.scrollTop+1 {
		n = b.scrollBot - b.scrollTop + 1
	}
	copy(b.lines[b.scrollTop+n:b.scrollBot+1], b.lines[b.scrollTop:b.scrollBot-n+1])
	for i := b.scrollTop; i < b.scrollTop+n; i++ {
		b.lines[i] = NewLine(b.cols)
		b.lines[i].Fill(b.eraseCell)
	}
}

func (b *Buffer) SetScrollRegion(top, bot int) {
	if top < 0 {
		top = 0
	}
	if bot >= b.rows {
		bot = b.rows - 1
	}
	if top > bot {
		return
	}
	b.scrollTop = top
	b.scrollBot = bot
}

func (b *Buffer) ScrollTop() int {
	return b.scrollTop
}

func (b *Buffer) ScrollBot() int {
	return b.scrollBot
}

func (b *Buffer) SetAltScreen(alt bool) {
	b.altScreen = alt
}

func (b *Buffer) IsAltScreen() bool {
	return b.altScreen
}

func (b *Buffer) LineFeed() {
	if b.cursor.Row == b.scrollBot {
		b.ScrollUp(1)
	} else if b.cursor.Row < b.rows-1 {
		b.cursor.Row++
	}
}

func (b *Buffer) ReverseIndex() {
	if b.cursor.Row == b.scrollTop {
		b.ScrollDown(1)
	} else if b.cursor.Row > 0 {
		b.cursor.Row--
	}
}

func (b *Buffer) Resize(cols, rows int) {
	if cols == b.cols && rows == b.rows {
		return
	}
	newLines := make([]*Line, rows)
	copyLen := rows
	if copyLen > b.rows {
		copyLen = b.rows
	}
	for i := 0; i < copyLen; i++ {
		if i < b.rows {
			b.lines[i].Resize(cols)
			newLines[i] = b.lines[i]
		}
	}
	for i := copyLen; i < rows; i++ {
		newLines[i] = NewLine(cols)
	}
	b.lines = newLines
	b.cols = cols
	b.rows = rows
	b.scrollTop = 0
	b.scrollBot = rows - 1
}

func (b *Buffer) Clear() {
	for i := range b.lines {
		b.lines[i].Fill(b.eraseCell)
	}
}

func (b *Buffer) ClearAll() {
	for i := range b.lines {
		b.lines[i].Fill(b.eraseCell)
	}
	b.history.Clear()
}

func (b *Buffer) ClearRect(r Rect) {
	for y := r.Y; y < r.Y+r.H && y < b.rows; y++ {
		if y < 0 {
			continue
		}
		for x := r.X; x < r.X+r.W && x < b.cols; x++ {
			if x < 0 {
				continue
			}
			cell := b.Cell(y, x)
			if cell != nil {
				*cell = b.eraseCell
			}
		}
	}
}
