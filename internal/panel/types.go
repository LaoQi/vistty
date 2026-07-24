package panel

type Primitive struct {
	Kind int
	X, Y int
	W, H int
	Text string
	Fg   [4]uint8
	Bg   [4]uint8
	Bold bool
}

const (
	PrimText = 0
	PrimRect = 1
)
