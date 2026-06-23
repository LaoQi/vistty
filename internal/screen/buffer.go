package screen

type Rect struct {
	X, Y, W, H int
}

type Buffer struct {
	lines     []*Line
	cols      int
	rows      int
	scrollTop int
	scrollBot int
	cursor    *Cursor
	history   *History
}

func NewBuffer(cols, rows int) *Buffer {
	b := &Buffer{
		cols:      cols,
		rows:      rows,
		scrollTop: 0,
		scrollBot: rows - 1,
		cursor:    NewCursor(),
		history:   NewHistory(1000),
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

func (b *Buffer) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	if n > b.scrollBot-b.scrollTop+1 {
		n = b.scrollBot - b.scrollTop + 1
	}
	for i := b.scrollTop; i < b.scrollTop+n; i++ {
		if b.lines[i] != nil {
			b.history.Push(b.lines[i])
		}
	}
	copy(b.lines[b.scrollTop:], b.lines[b.scrollTop+n:b.scrollBot+1])
	for i := b.scrollBot - n + 1; i <= b.scrollBot; i++ {
		b.lines[i] = NewLine(b.cols)
	}
	for i := b.scrollTop; i <= b.scrollBot; i++ {
		b.lines[i].dirty = true
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
	}
	for i := b.scrollTop; i <= b.scrollBot; i++ {
		b.lines[i].dirty = true
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
		newLines[i].dirty = true
	}
	b.lines = newLines
	b.cols = cols
	b.rows = rows
	b.scrollTop = 0
	b.scrollBot = rows - 1
}

func (b *Buffer) Clear() {
	for i := range b.lines {
		b.lines[i].Clear()
	}
}

func (b *Buffer) DirtyRegions() []Rect {
	var regions []Rect
	for y := 0; y < b.rows; y++ {
		line := b.lines[y]
		if line == nil {
			continue
		}
		if !line.IsDirty() {
			continue
		}
		if line.dirty {
			regions = append(regions, Rect{X: 0, Y: y, W: b.cols, H: 1})
			continue
		}
		startX := -1
		for x := 0; x < b.cols; x++ {
			cell := line.Cell(x)
			if cell != nil && cell.Dirty {
				if startX == -1 {
					startX = x
				}
			} else if startX != -1 {
				regions = append(regions, Rect{X: startX, Y: y, W: x - startX, H: 1})
				startX = -1
			}
		}
		if startX != -1 {
			regions = append(regions, Rect{X: startX, Y: y, W: b.cols - startX, H: 1})
		}
	}
	return mergeRects(regions)
}

func (b *Buffer) ClearDirty() {
	for _, l := range b.lines {
		l.ClearDirty()
	}
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
				cell.Clear()
				cell.MarkDirty()
			}
		}
	}
}

func mergeRects(rects []Rect) []Rect {
	if len(rects) <= 1 {
		return rects
	}
	merged := []Rect{rects[0]}
	for i := 1; i < len(rects); i++ {
		last := &merged[len(merged)-1]
		curr := rects[i]
		if curr.X == last.X && curr.W == last.W && curr.Y == last.Y+last.H {
			last.H++
		} else {
			merged = append(merged, curr)
		}
	}
	return merged
}
