package vte

type ControlCode int

const (
	ControlNUL ControlCode = iota
	ControlBEL
	ControlBS
	ControlHT
	ControlLF
	ControlVT
	ControlFF
	ControlCR
	ControlSO
	ControlSI
	ControlCAN
	ControlSUB
	ControlDEL
)

func ParseControl(b byte) (ControlCode, bool) {
	switch b {
	case 0x00:
		return ControlNUL, true
	case 0x07:
		return ControlBEL, true
	case 0x08:
		return ControlBS, true
	case 0x09:
		return ControlHT, true
	case 0x0A:
		return ControlLF, true
	case 0x0B:
		return ControlVT, true
	case 0x0C:
		return ControlFF, true
	case 0x0D:
		return ControlCR, true
	case 0x0E:
		return ControlSO, true
	case 0x0F:
		return ControlSI, true
	case 0x18:
		return ControlCAN, true
	case 0x1A:
		return ControlSUB, true
	case 0x7F:
		return ControlDEL, true
	default:
		return 0, false
	}
}
