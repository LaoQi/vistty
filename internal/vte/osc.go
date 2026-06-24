package vte

import "strings"

type OSCCommand int

const (
	OSCSetWindowTitle OSCCommand = iota
	OSCSetIconTitle
	OSCSetClipboard
	OSCSetWorkingDir
	OSCHyperlink
	OSCColorQuery
	OSCUnknown
)

type OSCSequence struct {
	Command OSCCommand
	Data    string
}

func ParseOSC(seq Sequence) OSCSequence {
	if seq.Action != ActionOSC {
		return OSCSequence{Command: OSCUnknown}
	}

	data := string(seq.Data)

	idx := strings.IndexByte(data, ';')
	if idx < 0 {
		return OSCSequence{Command: OSCUnknown, Data: data}
	}

	cmdStr := data[:idx]
	payload := data[idx+1:]

	if cmdStr == "" {
		return OSCSequence{Command: OSCUnknown, Data: data}
	}

	var cmd int
	for _, ch := range cmdStr {
		if ch >= '0' && ch <= '9' {
			cmd = cmd*10 + int(ch-'0')
		} else {
			return OSCSequence{Command: OSCUnknown, Data: data}
		}
	}

	switch cmd {
	case 0:
		return OSCSequence{Command: OSCSetWindowTitle, Data: payload}
	case 2:
		return OSCSequence{Command: OSCSetWindowTitle, Data: payload}
	case 1:
		return OSCSequence{Command: OSCSetIconTitle, Data: payload}
	case 7:
		return OSCSequence{Command: OSCSetWorkingDir, Data: payload}
	case 8:
		return OSCSequence{Command: OSCHyperlink, Data: payload}
	case 10, 11:
		return OSCSequence{Command: OSCColorQuery, Data: payload}
	case 52:
		return OSCSequence{Command: OSCSetClipboard, Data: payload}
	default:
		return OSCSequence{Command: OSCUnknown, Data: data}
	}
}
