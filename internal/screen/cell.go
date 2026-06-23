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
)

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

func (c *Cell) Equal(other Cell) bool {
	return c.Rune == other.Rune &&
		c.Width == other.Width &&
		c.Fg == other.Fg &&
		c.Bg == other.Bg &&
		c.Attr == other.Attr
}
