package screen

type Rect struct {
	X, Y, W, H int
}

type Buffer struct {
	lines     []*Line
	cols      int
	rows      int
	capacity  int
	mask      int
	offset    int
	scrollTop int
	scrollBot int
	cursor    *Cursor
	history   *History
	altScreen bool
	eraseCell Cell
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	v := uint(n - 1)
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	return int(v) + 1
}

func NewBuffer(cols, rows int) *Buffer {
	capacity := nextPow2(rows)
	b := &Buffer{
		cols:      cols,
		rows:      rows,
		capacity:  capacity,
		mask:      capacity - 1,
		scrollTop: 0,
		scrollBot: rows - 1,
		cursor:    NewCursor(),
		history:   NewHistory(1000),
		eraseCell: NewCell(),
	}
	b.lines = make([]*Line, capacity)
	for i := 0; i < rows; i++ {
		b.lines[i] = NewLine(cols)
	}
	return b
}

func (b *Buffer) lineAt(row int) *Line {
	return b.lines[(b.offset+row)&b.mask]
}

func (b *Buffer) physRow(row int) int {
	return (b.offset + row) & b.mask
}

func (b *Buffer) Cell(row, col int) *Cell {
	if row < 0 || row >= b.rows || col < 0 || col >= b.cols {
		return nil
	}
	line := b.lineAt(row)
	if line == nil {
		return nil
	}
	return line.Cell(col)
}

func (b *Buffer) Line(row int) *Line {
	if row < 0 || row >= b.rows {
		return nil
	}
	return b.lineAt(row)
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
		Attr:  attr &^ (AttrBold | AttrDim | AttrItalic | AttrUnderline | AttrBlink | AttrReverse | AttrCrossedOut | AttrClean),
	}
}

func (b *Buffer) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	if n > b.scrollBot-b.scrollTop+1 {
		n = b.scrollBot - b.scrollTop + 1
	}
	if b.scrollTop == 0 && b.scrollBot == b.rows-1 {
		for i := 0; i < n; i++ {
			pr := b.physRow(i)
			if b.lines[pr] != nil && !b.altScreen {
				b.history.Push(b.lines[pr])
			}
			b.lines[pr] = nil
		}
		b.offset = (b.offset + n) & b.mask
		for i := b.rows - n; i < b.rows; i++ {
			pr := b.physRow(i)
			if b.lines[pr] == nil {
				b.lines[pr] = NewLine(b.cols)
			}
			b.lines[pr].Fill(b.eraseCell)
		}
	} else {
		for i := b.scrollTop; i < b.scrollTop+n; i++ {
			pr := b.physRow(i)
			b.lines[pr] = nil
		}
		for i := b.scrollTop; i <= b.scrollBot-n; i++ {
			b.lines[b.physRow(i)] = b.lines[b.physRow(i+n)]
		}
		for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
			pr := b.physRow(i)
			b.lines[pr] = NewLine(b.cols)
			b.lines[pr].Fill(b.eraseCell)
		}
	}
	b.DamageRows(b.scrollTop, b.scrollBot)
}

func (b *Buffer) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	if n > b.scrollBot-b.scrollTop+1 {
		n = b.scrollBot - b.scrollTop + 1
	}
	if b.scrollTop == 0 && b.scrollBot == b.rows-1 {
		for i := 0; i < n; i++ {
			pr := b.physRow(b.rows - 1 - i)
			b.lines[pr] = nil
		}
		b.offset = (b.offset - n) & b.mask
		for i := 0; i < n; i++ {
			pr := b.physRow(i)
			if b.lines[pr] == nil {
				b.lines[pr] = NewLine(b.cols)
			}
			b.lines[pr].Fill(b.eraseCell)
		}
	} else {
		for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
			pr := b.physRow(i)
			b.lines[pr] = nil
		}
		for i := b.scrollBot; i >= b.scrollTop+n; i-- {
			b.lines[b.physRow(i)] = b.lines[b.physRow(i-n)]
		}
		for i := b.scrollTop; i < b.scrollTop+n; i++ {
			pr := b.physRow(i)
			b.lines[pr] = NewLine(b.cols)
			b.lines[pr].Fill(b.eraseCell)
		}
	}
	b.DamageRows(b.scrollTop, b.scrollBot)
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
	newCap := nextPow2(rows)
	newLines := make([]*Line, newCap)
	copyLen := rows
	if copyLen > b.rows {
		copyLen = b.rows
	}
	for i := 0; i < copyLen; i++ {
		line := b.lineAt(i)
		if line == nil {
			line = NewLine(cols)
		} else {
			line.Resize(cols)
		}
		newLines[i] = line
	}
	for i := copyLen; i < rows; i++ {
		newLines[i] = NewLine(cols)
	}
	b.lines = newLines
	b.capacity = newCap
	b.mask = newCap - 1
	b.offset = 0
	b.cols = cols
	b.rows = rows
	b.scrollTop = 0
	b.scrollBot = rows - 1
}

func (b *Buffer) Clear() {
	for i := 0; i < b.rows; i++ {
		pr := b.physRow(i)
		if b.lines[pr] == nil {
			b.lines[pr] = NewLine(b.cols)
		}
		b.lines[pr].Fill(b.eraseCell)
	}
	b.DamageAll()
}

func (b *Buffer) ClearAll() {
	for i := 0; i < b.rows; i++ {
		pr := b.physRow(i)
		if b.lines[pr] == nil {
			b.lines[pr] = NewLine(b.cols)
		}
		b.lines[pr].Fill(b.eraseCell)
	}
	b.history.Clear()
	b.DamageAll()
}

func (b *Buffer) ClearRect(r Rect) {
	for y := r.Y; y < r.Y+r.H && y < b.rows; y++ {
		if y < 0 {
			continue
		}
		line := b.lineAt(y)
		if line != nil {
			line.SetDirty(true)
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

func (b *Buffer) DamageRows(start, end int) {
	if start < 0 {
		start = 0
	}
	if end >= b.rows {
		end = b.rows - 1
	}
	for r := start; r <= end; r++ {
		line := b.lineAt(r)
		if line == nil {
			continue
		}
		line.SetDirty(true)
		for c := 0; c < b.cols; c++ {
			cell := line.Cell(c)
			if cell != nil {
				cell.SetDirty()
			}
		}
	}
}

func (b *Buffer) DamageView() { b.DamageRows(0, b.rows-1) }
func (b *Buffer) DamageAll()  { b.DamageRows(0, b.rows-1) }

func (b *Buffer) DamageLine(row int) {
	if row < 0 || row >= b.rows {
		return
	}
	line := b.lineAt(row)
	if line == nil {
		return
	}
	line.SetDirty(true)
	for c := 0; c < b.cols; c++ {
		cell := line.Cell(c)
		if cell != nil {
			cell.SetDirty()
		}
	}
}

func (b *Buffer) DamageCell(row, col int) {
	if row < 0 || row >= b.rows {
		return
	}
	line := b.lineAt(row)
	if line == nil {
		return
	}
	line.SetDirty(true)
	cell := line.Cell(col)
	if cell != nil {
		cell.SetDirty()
	}
}

func (b *Buffer) DamageCursor(row, col int) {
	b.DamageCell(row, col)
}

// InsertLines 在 row 行插入 n 个新空行，[row, scrollBot] 区间内容下移 n 行，
// 底部 n 行挤出丢失。不进 history（与 xterm IL 语义一致）。
// 光标在 scroll region 外时为 no-op。
func (b *Buffer) InsertLines(row, n int) {
	if row < b.scrollTop || row > b.scrollBot {
		return
	}
	if n <= 0 {
		return
	}
	if n > b.scrollBot-row+1 {
		n = b.scrollBot - row + 1
	}
	for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
		pr := b.physRow(i)
		b.lines[pr] = nil
	}
	for i := b.scrollBot; i >= row+n; i-- {
		b.lines[b.physRow(i)] = b.lines[b.physRow(i-n)]
	}
	for i := row; i < row+n; i++ {
		pr := b.physRow(i)
		b.lines[pr] = NewLine(b.cols)
		b.lines[pr].Fill(b.eraseCell)
	}
	b.DamageRows(row, b.scrollBot)
}

// DeleteLines 删除 row 行起的 n 行，[row+n, scrollBot] 区间内容上移 n 行，
// 底部 n 行填新空行。不进 history（与 xterm DL 语义一致）。
// 光标在 scroll region 外时为 no-op。
func (b *Buffer) DeleteLines(row, n int) {
	if row < b.scrollTop || row > b.scrollBot {
		return
	}
	if n <= 0 {
		return
	}
	if n > b.scrollBot-row+1 {
		n = b.scrollBot - row + 1
	}
	for i := row; i < row+n; i++ {
		pr := b.physRow(i)
		b.lines[pr] = nil
	}
	for i := row; i <= b.scrollBot-n; i++ {
		b.lines[b.physRow(i)] = b.lines[b.physRow(i+n)]
	}
	for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
		pr := b.physRow(i)
		b.lines[pr] = NewLine(b.cols)
		b.lines[pr].Fill(b.eraseCell)
	}
	b.DamageRows(row, b.scrollBot)
}
