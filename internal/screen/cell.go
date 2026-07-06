package screen

type Color struct {
	R, G, B    uint8
	IsDefault bool
}

type Attributes uint16

const (
	AttrBold Attributes = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrBlink
	AttrReverse
	AttrCrossedOut
	AttrClean
)

func (c *Cell) Clean() bool { return c.Attr&AttrClean != 0 }
func (c *Cell) SetClean()   { c.Attr |= AttrClean }
func (c *Cell) SetDirty()   { c.Attr &^= AttrClean }

type Cell struct {
	Rune  rune
	Width uint8
	Fg    Color
	Bg    Color
	Attr  Attributes
}

func NewCell() Cell {
	return Cell{
		Rune:  ' ',
		Width: 1,
		Fg:    Color{IsDefault: true},
		Bg:    Color{IsDefault: true},
	}
}

func (c *Cell) Clear() {
	*c = NewCell()
}

func (c *Cell) Erase(fg, bg Color, attr Attributes) {
	c.Rune = ' '
	c.Width = 1
	c.Fg = fg
	c.Bg = bg
	c.Attr = attr
}

func (c *Cell) Equal(other Cell) bool {
	return c.Rune == other.Rune &&
		c.Width == other.Width &&
		c.Fg == other.Fg &&
		c.Bg == other.Bg &&
		c.Attr == other.Attr
}
