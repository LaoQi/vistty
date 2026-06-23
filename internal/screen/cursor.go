package screen

type CursorStyle int

const (
	CursorBlock CursorStyle = iota
	CursorUnderline
	CursorBar
)

type Cursor struct {
	Row      int
	Col      int
	Style    CursorStyle
	Visible  bool
	Blinking bool
}

func NewCursor() *Cursor {
	return &Cursor{
		Row:      0,
		Col:      0,
		Style:    CursorBlock,
		Visible:  true,
		Blinking: true,
	}
}

func (c *Cursor) Move(row, col int) {
	c.Row = row
	c.Col = col
}

func (c *Cursor) Clamp(maxRow, maxCol int) {
	if c.Row < 0 {
		c.Row = 0
	}
	if c.Col < 0 {
		c.Col = 0
	}
	if c.Row > maxRow {
		c.Row = maxRow
	}
	if c.Col > maxCol {
		c.Col = maxCol
	}
}

func (c *Cursor) Hide() {
	c.Visible = false
}

func (c *Cursor) Show() {
	c.Visible = true
}

func (c *Cursor) SetStyle(style CursorStyle) {
	c.Style = style
}
