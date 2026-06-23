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
	CSIUnknown
)

type CSISequence struct {
	Command CSICommand
	Params  []int
	Private bool
}

func param(seq Sequence, idx, def int) int {
	if idx < len(seq.Params) && seq.Params[idx] != 0 {
		return seq.Params[idx]
	}
	return def
}

func ParseCSI(seq Sequence) CSISequence {
	if seq.Action != ActionCSI {
		return CSISequence{Command: CSIUnknown}
	}

	private := false
	for _, b := range seq.Intermed {
		if b == '?' {
			private = true
		}
	}

	if private {
		return parseCSIPrivate(seq)
	}

	switch seq.Command {
	case 'A':
		return CSISequence{Command: CSICursorUp, Params: []int{param(seq, 0, 1)}}
	case 'B':
		return CSISequence{Command: CSICursorDown, Params: []int{param(seq, 0, 1)}}
	case 'C':
		return CSISequence{Command: CSICursorForward, Params: []int{param(seq, 0, 1)}}
	case 'D':
		return CSISequence{Command: CSICursorBackward, Params: []int{param(seq, 0, 1)}}
	case 'E':
		return CSISequence{Command: CSICursorNextLine, Params: []int{param(seq, 0, 1)}}
	case 'F':
		return CSISequence{Command: CSICursorPrevLine, Params: []int{param(seq, 0, 1)}}
	case 'G':
		return CSISequence{Command: CSICursorHorizontalAbsolute, Params: []int{param(seq, 0, 1)}}
	case 'H':
		return CSISequence{Command: CSICursorPosition, Params: []int{param(seq, 0, 1), param(seq, 1, 1)}}
	case 'J':
		return CSISequence{Command: CSIEraseInDisplay, Params: []int{param(seq, 0, 0)}}
	case 'K':
		return CSISequence{Command: CSIEraseInLine, Params: []int{param(seq, 0, 0)}}
	case 'L':
		return CSISequence{Command: CSIInsertLines, Params: []int{param(seq, 0, 1)}}
	case 'M':
		return CSISequence{Command: CSIDeleteLines, Params: []int{param(seq, 0, 1)}}
	case 'P':
		return CSISequence{Command: CSIDeleteChars, Params: []int{param(seq, 0, 1)}}
	case 'S':
		return CSISequence{Command: CSIScrollUp, Params: []int{param(seq, 0, 1)}}
	case 'T':
		return CSISequence{Command: CSIScrollDown, Params: []int{param(seq, 0, 1)}}
	case '@':
		return CSISequence{Command: CSIInsertChars, Params: []int{param(seq, 0, 1)}}
	case 'r':
		top := 1
		bottom := 0
		if len(seq.Params) > 0 && seq.Params[0] > 0 {
			top = seq.Params[0]
		}
		if len(seq.Params) > 1 && seq.Params[1] > 0 {
			bottom = seq.Params[1]
		}
		return CSISequence{Command: CSISetTopBottomMargin, Params: []int{top, bottom}}
	case 'm':
		return CSISequence{Command: CSISGR, Params: seq.Params}
	case 'q':
		return CSISequence{Command: CSICursorStyle, Params: []int{param(seq, 0, 0)}}
	case 'd':
		return CSISequence{Command: CSILinePositionAbsolute, Params: []int{param(seq, 0, 1)}}
	case 'I':
		return CSISequence{Command: CSICursorHorizontalTab, Params: []int{param(seq, 0, 1)}}
	case 'Z':
		return CSISequence{Command: CSICursorBackTab, Params: []int{param(seq, 0, 1)}}
	case 's':
		return CSISequence{Command: CSISaveCursor}
	case 'u':
		return CSISequence{Command: CSIRestoreCursor}
	default:
		return CSISequence{Command: CSIUnknown, Params: seq.Params}
	}
}

func parseCSIPrivate(seq Sequence) CSISequence {
	switch seq.Command {
	case 'h':
		if len(seq.Params) > 0 {
			switch seq.Params[0] {
			case 25:
				return CSISequence{Command: CSICursorShow, Private: true}
			default:
				return CSISequence{Command: CSISetMode, Params: seq.Params, Private: true}
			}
		}
		return CSISequence{Command: CSISetMode, Private: true}
	case 'l':
		if len(seq.Params) > 0 {
			switch seq.Params[0] {
			case 25:
				return CSISequence{Command: CSICursorHide, Private: true}
			default:
				return CSISequence{Command: CSIResetMode, Params: seq.Params, Private: true}
			}
		}
		return CSISequence{Command: CSIResetMode, Private: true}
	default:
		return CSISequence{Command: CSIUnknown, Params: seq.Params, Private: true}
	}
}
