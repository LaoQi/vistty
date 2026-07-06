package screen

type Line struct {
	cells []Cell
	dirty bool
}

func NewLine(width int) *Line {
	l := &Line{
		cells: make([]Cell, width),
		dirty: true,
	}
	for i := range l.cells {
		l.cells[i] = NewCell()
	}
	return l
}

func (l *Line) Dirty() bool     { return l.dirty }
func (l *Line) SetDirty(v bool) { l.dirty = v }

func (l *Line) Cell(col int) *Cell {
	if col < 0 || col >= len(l.cells) {
		return nil
	}
	return &l.cells[col]
}

func (l *Line) Width() int {
	return len(l.cells)
}

func (l *Line) Resize(width int) {
	if width == len(l.cells) {
		return
	}
	if width < len(l.cells) {
		l.cells = l.cells[:width]
	} else {
		tail := make([]Cell, width-len(l.cells))
		for i := range tail {
			tail[i] = NewCell()
		}
		l.cells = append(l.cells, tail...)
	}
}

func (l *Line) Clear() {
	for i := range l.cells {
		l.cells[i] = NewCell()
	}
	l.dirty = true
}

func (l *Line) Fill(c Cell) {
	for i := range l.cells {
		l.cells[i] = c
	}
	l.dirty = true
}

func (l *Line) Clone() *Line {
	c := &Line{
		cells: make([]Cell, len(l.cells)),
	}
	copy(c.cells, l.cells)
	return c
}
