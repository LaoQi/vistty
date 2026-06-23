package platform

type KeyState bool

const (
	KeyRelease KeyState = false
	KeyPress   KeyState = true
)

type Modifiers uint8

const (
	ModCtrl  Modifiers = 1 << iota
	ModAlt
	ModShift
	ModSuper
)

type KeyEvent struct {
	Rune  rune
	Code  uint16
	Mods  Modifiers
	State KeyState
}

type MouseEvent struct {
	X, Y   int
	Button uint8
	State  KeyState
}

type InputSource interface {
	KeyEvents() <-chan KeyEvent
	MouseEvents() <-chan MouseEvent
	Close() error
}
