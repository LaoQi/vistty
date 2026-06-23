package vte

type ESCCommand int

const (
	ESCResetState ESCCommand = iota
	ESCRestoreState
	ESCIndex
	ESCNextLine
	ESCReverseIndex
	ESCTabSet
	ESCDeckpam
	ESCDeckpnm
	ESCFullReset
	ESCUnknown
)

type ESCSequence struct {
	Command  ESCCommand
	Intermed byte
}

func ParseESC(seq Sequence) ESCSequence {
	if seq.Action != ActionESC {
		return ESCSequence{Command: ESCUnknown}
	}

	if len(seq.Intermed) > 0 {
		if seq.Intermed[0] == ' ' && seq.Command == 'F' {
			return ESCSequence{Command: ESCUnknown, Intermed: seq.Intermed[0]}
		}
	}

	switch seq.Command {
	case '7':
		return ESCSequence{Command: ESCResetState}
	case '8':
		return ESCSequence{Command: ESCRestoreState}
	case 'D':
		return ESCSequence{Command: ESCIndex}
	case 'E':
		return ESCSequence{Command: ESCNextLine}
	case 'M':
		return ESCSequence{Command: ESCReverseIndex}
	case 'H':
		return ESCSequence{Command: ESCTabSet}
	case '=':
		return ESCSequence{Command: ESCDeckpam}
	case '>':
		return ESCSequence{Command: ESCDeckpnm}
	case 'c':
		return ESCSequence{Command: ESCFullReset}
	default:
		return ESCSequence{Command: ESCUnknown, Intermed: seq.Command}
	}
}
