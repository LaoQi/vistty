package vte

import (
	"unicode"
	"unicode/utf8"
)

type Action int

const (
	ActionPrint Action = iota
	ActionExecute
	ActionCSI
	ActionOSC
	ActionESC
	ActionDCS
	ActionIgnore
)

type state int

const (
	stateGround state = iota
	stateEscape
	stateEscapeIntermediate
	stateCSIEntry
	stateCSIParameter
	stateCSIIntermediate
	stateOSCString
	stateDCSString
	stateUTF8
)

type Sequence struct {
	Action   Action
	Command  byte
	Params   [16]int
	NParams  int
	Intermed []byte
	Data     []byte
	Rune     rune
}

type Parser struct {
	state       state
	params      []int
	curParam    int
	hasParam    bool
	intermed    []byte
	data        []byte
	private       bool
	privateMarker byte
	pendingOSC    bool
	pendingDCS  bool
	utf8Buf     [4]byte
	utf8Len     int
	utf8Total   int
	seqs        []Sequence
}

func NewParser() *Parser {
	p := &Parser{
		seqs: make([]Sequence, 0, 256),
	}
	p.Reset()
	return p
}

func (p *Parser) Reset() {
	p.state = stateGround
	p.params = p.params[:0]
	p.curParam = 0
	p.hasParam = false
	p.intermed = p.intermed[:0]
	p.data = p.data[:0]
	p.private = false
	p.privateMarker = 0
	p.pendingOSC = false
	p.pendingDCS = false
	p.seqs = p.seqs[:0]
}

func (p *Parser) Feed(b byte) []Sequence {
	start := len(p.seqs)
	p.dispatch(b)
	if len(p.seqs) == start {
		return nil
	}
	result := make([]Sequence, len(p.seqs)-start)
	copy(result, p.seqs[start:])
	return result
}

func (p *Parser) dispatch(b byte) {
	switch p.state {
	case stateGround:
		p.feedGround(b)
	case stateEscape:
		p.feedEscape(b)
	case stateEscapeIntermediate:
		p.feedEscapeIntermediate(b)
	case stateCSIEntry:
		p.feedCSIEntry(b)
	case stateCSIParameter:
		p.feedCSIParameter(b)
	case stateCSIIntermediate:
		p.feedCSIIntermediate(b)
	case stateOSCString:
		p.feedOSCString(b)
	case stateDCSString:
		p.feedDCSString(b)
	case stateUTF8:
		p.feedUTF8(b)
	default:
		p.state = stateGround
	}
}

func (p *Parser) feedGround(b byte) {
	if b == 0x1B {
		p.state = stateEscape
		return
	}
	if b < 0x20 || b == 0x7F {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	if b >= 0x80 {
		p.utf8Total = utf8Len(b)
		if p.utf8Total < 2 || p.utf8Total > 4 {
			p.utf8Total = 0
			p.seqs = append(p.seqs, Sequence{Action: ActionPrint, Rune: unicode.ReplacementChar})
			return
		}
		p.utf8Buf[0] = b
		p.utf8Len = 1
		p.state = stateUTF8
		return
	}
	p.seqs = append(p.seqs, Sequence{Action: ActionPrint, Rune: rune(b)})
}

func (p *Parser) feedEscape(b byte) {
	if p.pendingOSC {
		p.pendingOSC = false
		if b == '\\' {
			p.state = stateGround
			p.seqs = append(p.seqs, Sequence{Action: ActionOSC, Data: copyBytes(p.data)})
			return
		}
		p.seqs = append(p.seqs, Sequence{Action: ActionOSC, Data: copyBytes(p.data)})
		p.state = stateGround
		p.dispatch(b)
		return
	}
	if p.pendingDCS {
		p.pendingDCS = false
		if b == '\\' {
			p.state = stateGround
			p.seqs = append(p.seqs, Sequence{Action: ActionDCS, Data: copyBytes(p.data), Command: 0})
			return
		}
		p.seqs = append(p.seqs, Sequence{Action: ActionDCS, Data: copyBytes(p.data), Command: 0})
		p.state = stateGround
		p.dispatch(b)
		return
	}
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	switch b {
	case '[':
		p.resetSequence()
		p.state = stateCSIEntry
		return
	case ']':
		p.resetSequence()
		p.state = stateOSCString
		return
	case 'P':
		p.resetSequence()
		p.state = stateDCSString
		return
	case 'X', '^', '_':
		p.data = p.data[:0]
		p.state = stateDCSString
		return
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateEscapeIntermediate
		return
	}
	if b >= 0x30 && b <= 0x7E {
		p.state = stateGround
		p.seqs = append(p.seqs, Sequence{Action: ActionESC, Command: b, Intermed: copyBytes(p.intermed)})
		return
	}
	p.state = stateGround
}

func (p *Parser) feedEscapeIntermediate(b byte) {
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		return
	}
	if b >= 0x30 && b <= 0x7E {
		p.state = stateGround
		p.seqs = append(p.seqs, Sequence{Action: ActionESC, Command: b, Intermed: copyBytes(p.intermed)})
		return
	}
	p.state = stateGround
}

func (p *Parser) feedCSIEntry(b byte) {
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	if b >= 0x30 && b <= 0x39 {
		p.curParam = int(b - '0')
		p.hasParam = true
		p.state = stateCSIParameter
		return
	}
	if b == ';' {
		p.params = append(p.params, p.curParam)
		p.curParam = 0
		p.hasParam = false
		p.state = stateCSIParameter
		return
	}
	if b == '?' || b == '>' || b == '=' || b == '<' {
		p.private = true
		p.privateMarker = b
		p.state = stateCSIParameter
		return
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateCSIIntermediate
		return
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		params, n := p.copyParams()
		p.seqs = append(p.seqs, Sequence{
			Action:   ActionCSI,
			Command:  b,
			Params:   params,
			NParams:  n,
			Intermed: intermed,
		})
		return
	}
	p.state = stateGround
}

func (p *Parser) feedCSIParameter(b byte) {
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	if b >= 0x30 && b <= 0x39 {
		p.curParam = p.curParam*10 + int(b-'0')
		p.hasParam = true
		return
	}
	if b == ';' {
		p.params = append(p.params, p.curParam)
		p.curParam = 0
		p.hasParam = false
		return
	}
	if b == '?' || b == '>' || b == '=' || b == '<' {
		p.private = true
		p.privateMarker = b
		return
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateCSIIntermediate
		return
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		params, n := p.copyParams()
		p.seqs = append(p.seqs, Sequence{
			Action:   ActionCSI,
			Command:  b,
			Params:   params,
			NParams:  n,
			Intermed: intermed,
		})
		return
	}
	p.state = stateGround
}

func (p *Parser) feedCSIIntermediate(b byte) {
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		return
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		params, n := p.copyParams()
		p.seqs = append(p.seqs, Sequence{
			Action:   ActionCSI,
			Command:  b,
			Params:   params,
			NParams:  n,
			Intermed: intermed,
		})
		return
	}
	p.state = stateGround
}

func (p *Parser) feedOSCString(b byte) {
	if b == 0x07 {
		p.state = stateGround
		p.seqs = append(p.seqs, Sequence{Action: ActionOSC, Data: copyBytes(p.data)})
		return
	}
	if b == 0x1B {
		p.pendingOSC = true
		p.state = stateEscape
		return
	}
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	p.data = append(p.data, b)
}

func (p *Parser) feedDCSString(b byte) {
	if b == 0x07 {
		p.state = stateGround
		p.seqs = append(p.seqs, Sequence{Action: ActionDCS, Data: copyBytes(p.data), Command: 0})
		return
	}
	if b == 0x1B {
		p.pendingDCS = true
		p.state = stateEscape
		return
	}
	if b < 0x20 {
		p.seqs = append(p.seqs, Sequence{Action: ActionExecute, Command: b})
		return
	}
	p.data = append(p.data, b)
}

func (p *Parser) feedUTF8(b byte) {
	if b&0xC0 != 0x80 {
		p.state = stateGround
		p.utf8Total = 0
		p.seqs = append(p.seqs, Sequence{Action: ActionPrint, Rune: unicode.ReplacementChar})
		return
	}
	p.utf8Buf[p.utf8Len] = b
	p.utf8Len++
	if p.utf8Len >= p.utf8Total {
		r, _ := utf8.DecodeRune(p.utf8Buf[:p.utf8Total])
		if r == unicode.ReplacementChar && p.utf8Total > 1 {
			r = unicode.ReplacementChar
		}
		p.state = stateGround
		p.utf8Total = 0
		p.seqs = append(p.seqs, Sequence{Action: ActionPrint, Rune: r})
		return
	}
}

func utf8Len(b byte) int {
	switch {
	case b&0x80 == 0:
		return 1
	case b&0xE0 == 0xC0:
		return 2
	case b&0xF0 == 0xE0:
		return 3
	case b&0xF8 == 0xF0:
		return 4
	default:
		return 0
	}
}

func (p *Parser) FeedAll(data []byte) []Sequence {
	p.seqs = p.seqs[:0]
	for _, b := range data {
		p.dispatch(b)
	}
	if len(p.seqs) == 0 {
		return nil
	}
	result := make([]Sequence, len(p.seqs))
	copy(result, p.seqs)
	return result
}

func (p *Parser) resetSequence() {
	p.params = p.params[:0]
	p.curParam = 0
	p.hasParam = false
	p.intermed = p.intermed[:0]
	p.data = p.data[:0]
	p.private = false
	p.privateMarker = 0
	p.pendingOSC = false
	p.pendingDCS = false
	p.utf8Total = 0
	p.utf8Len = 0
}

func (p *Parser) finalizeParam() {
	if p.hasParam || len(p.params) > 0 {
		p.params = append(p.params, p.curParam)
	}
	p.curParam = 0
	p.hasParam = false
}

func (p *Parser) csiIntermed() []byte {
	if !p.private {
		return copyBytes(p.intermed)
	}
	buf := make([]byte, 0, 1+len(p.intermed))
	buf = append(buf, p.privateMarker)
	buf = append(buf, p.intermed...)
	return buf
}

func copyBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

func (p *Parser) copyParams() ([16]int, int) {
	var arr [16]int
	n := copy(arr[:], p.params)
	return arr, n
}
