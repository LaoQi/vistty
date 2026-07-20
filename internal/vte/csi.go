package vte

type CSICommand int

const (
	CSICursorUp CSICommand = iota
	CSICursorDown
	CSICursorForward
	CSICursorBackward
	CSICursorNextLine
	CSICursorPrevLine
	CSICursorHorizontalAbsolute
	CSICursorPosition
	CSIEraseInDisplay
	CSIEraseInLine
	CSIInsertLines
	CSIDeleteLines
	CSIDeleteChars
	CSIScrollUp
	CSIScrollDown
	CSIInsertChars
	CSISetTopBottomMargin
	CSICursorStyle
	CSISGR
	CSICursorShow
	CSICursorHide
	CSISaveCursor
	CSIRestoreCursor
	CSISetMode
	CSIResetMode
	CSILinePositionAbsolute
	CSICursorHorizontalTab
	CSICursorBackTab
	CSIScreenMode
	CSIEraseChars
	CSIDeviceStatusReport
	CSIDeviceAttributes
	CSIDeviceAttributes2
	CSITabClear
	CSISetCharProtection
	CSIUnknown
)

type CSISequence struct {
	Command CSICommand
	Params  [32]int
	NParams int
	Private bool
}

func param(seq Sequence, idx, def int) int {
	if idx < seq.NParams && seq.Params[idx] != 0 {
		return seq.Params[idx]
	}
	return def
}

func ParseCSI(seq Sequence) CSISequence {
	if seq.Action != ActionCSI {
		return CSISequence{Command: CSIUnknown}
	}

	privateMarker := byte(0)
	for _, b := range seq.Intermed {
		if b == '?' || b == '>' || b == '=' || b == '<' {
			privateMarker = b
			break
		}
	}

	if privateMarker != 0 {
		return parseCSIPrivate(seq, privateMarker)
	}

	var intermed byte = 0
	for _, b := range seq.Intermed {
		if b >= 0x20 && b <= 0x2F {
			intermed = b
			break
		}
	}

	switch seq.Command {
	case 'A':
		return CSISequence{Command: CSICursorUp, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'B':
		return CSISequence{Command: CSICursorDown, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'C':
		return CSISequence{Command: CSICursorForward, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'D':
		return CSISequence{Command: CSICursorBackward, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'E':
		return CSISequence{Command: CSICursorNextLine, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'F':
		return CSISequence{Command: CSICursorPrevLine, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'G':
		return CSISequence{Command: CSICursorHorizontalAbsolute, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'H':
		return CSISequence{Command: CSICursorPosition, Params: [32]int{param(seq, 0, 1), param(seq, 1, 1)}, NParams: 2}
	case 'J':
		return CSISequence{Command: CSIEraseInDisplay, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
	case 'K':
		return CSISequence{Command: CSIEraseInLine, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
	case 'L':
		return CSISequence{Command: CSIInsertLines, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'M':
		return CSISequence{Command: CSIDeleteLines, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'P':
		return CSISequence{Command: CSIDeleteChars, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'S':
		return CSISequence{Command: CSIScrollUp, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'T':
		return CSISequence{Command: CSIScrollDown, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case '@':
		return CSISequence{Command: CSIInsertChars, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'X':
		return CSISequence{Command: CSIEraseChars, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'n':
		return CSISequence{Command: CSIDeviceStatusReport, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
	case 'c':
		return CSISequence{Command: CSIDeviceAttributes, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
	case 'g':
		return CSISequence{Command: CSITabClear, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
	case 'r':
		top := 1
		bottom := 0
		if seq.NParams > 0 && seq.Params[0] > 0 {
			top = seq.Params[0]
		}
		if seq.NParams > 1 && seq.Params[1] > 0 {
			bottom = seq.Params[1]
		}
		return CSISequence{Command: CSISetTopBottomMargin, Params: [32]int{top, bottom}, NParams: seq.NParams}
	case 'm':
		return CSISequence{Command: CSISGR, Params: seq.Params, NParams: seq.NParams}
	case 'q':
		if intermed == 0x20 {
			return CSISequence{Command: CSICursorStyle, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
		}
		if intermed == 0x22 {
			return CSISequence{Command: CSISetCharProtection, Params: [32]int{param(seq, 0, 0)}, NParams: 1}
		}
		return CSISequence{Command: CSIUnknown, Params: seq.Params, NParams: seq.NParams}
	case 'd':
		return CSISequence{Command: CSILinePositionAbsolute, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'I':
		return CSISequence{Command: CSICursorHorizontalTab, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 'Z':
		return CSISequence{Command: CSICursorBackTab, Params: [32]int{param(seq, 0, 1)}, NParams: 1}
	case 's':
		return CSISequence{Command: CSISaveCursor}
	case 'u':
		return CSISequence{Command: CSIRestoreCursor}
	case 'h':
		return CSISequence{Command: CSISetMode, Params: seq.Params, NParams: seq.NParams, Private: false}
	case 'l':
		return CSISequence{Command: CSIResetMode, Params: seq.Params, NParams: seq.NParams, Private: false}
	default:
		return CSISequence{Command: CSIUnknown, Params: seq.Params, NParams: seq.NParams}
	}
}

func parseCSIPrivate(seq Sequence, marker byte) CSISequence {
	switch marker {
	case '?':
		switch seq.Command {
		case 'h':
			return CSISequence{Command: CSISetMode, Params: seq.Params, NParams: seq.NParams, Private: true}
		case 'l':
			return CSISequence{Command: CSIResetMode, Params: seq.Params, NParams: seq.NParams, Private: true}
		case 'n':
			return CSISequence{Command: CSIDeviceStatusReport, Params: [32]int{param(seq, 0, 0)}, NParams: 1, Private: true}
		case 'p':
			return CSISequence{Command: CSIDeviceStatusReport, Params: [32]int{param(seq, 0, 0)}, NParams: 1, Private: true}
		default:
			return CSISequence{Command: CSIUnknown, Params: seq.Params, NParams: seq.NParams, Private: true}
		}
	case '>':
		switch seq.Command {
		case 'c':
			return CSISequence{Command: CSIDeviceAttributes2, Private: true}
		default:
			return CSISequence{Command: CSIUnknown, Params: seq.Params, NParams: seq.NParams, Private: true}
		}
	default:
		return CSISequence{Command: CSIUnknown, Params: seq.Params, NParams: seq.NParams, Private: true}
	}
}
