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
	ESCDesignateG0
	ESCDesignateG1
	ESCUnknown
)

type ESCSequence struct {
	Command  ESCCommand
	Intermed []byte
	Charset  byte
}

func ParseESC(seq Sequence) ESCSequence {
	if seq.Action != ActionESC {
		return ESCSequence{Command: ESCUnknown}
	}

	if len(seq.Intermed) > 0 {
		switch seq.Intermed[0] {
		case '(', '*':
			return ESCSequence{Command: ESCDesignateG0, Intermed: seq.Intermed, Charset: seq.Command}
		case ')', '+':
			return ESCSequence{Command: ESCDesignateG1, Intermed: seq.Intermed, Charset: seq.Command}
		}
		return ESCSequence{Command: ESCUnknown, Intermed: seq.Intermed}
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
		return ESCSequence{Command: ESCUnknown}
	}
}
