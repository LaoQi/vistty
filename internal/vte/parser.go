package vte

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
)

type Sequence struct {
	Action   Action
	Command  byte
	Params   []int
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
	private     bool
	pendingOSC  bool
	pendingDCS  bool
}

func NewParser() *Parser {
	p := &Parser{}
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
	p.pendingOSC = false
	p.pendingDCS = false
}

func (p *Parser) Feed(b byte) []Sequence {
	switch p.state {
	case stateGround:
		return p.feedGround(b)
	case stateEscape:
		return p.feedEscape(b)
	case stateEscapeIntermediate:
		return p.feedEscapeIntermediate(b)
	case stateCSIEntry:
		return p.feedCSIEntry(b)
	case stateCSIParameter:
		return p.feedCSIParameter(b)
	case stateCSIIntermediate:
		return p.feedCSIIntermediate(b)
	case stateOSCString:
		return p.feedOSCString(b)
	case stateDCSString:
		return p.feedDCSString(b)
	default:
		p.state = stateGround
		return nil
	}
}

func (p *Parser) feedGround(b byte) []Sequence {
	if b == 0x1B {
		p.state = stateEscape
		return nil
	}
	if b < 0x20 || b == 0x7F {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	return []Sequence{{Action: ActionPrint, Rune: rune(b)}}
}

func (p *Parser) feedEscape(b byte) []Sequence {
	if p.pendingOSC {
		p.pendingOSC = false
		if b == '\\' {
			p.state = stateGround
			return []Sequence{{Action: ActionOSC, Data: copyBytes(p.data)}}
		}
		var seqs []Sequence
		seqs = append(seqs, Sequence{Action: ActionOSC, Data: copyBytes(p.data)})
		p.state = stateGround
		seqs = append(seqs, p.Feed(b)...)
		return seqs
	}
	if p.pendingDCS {
		p.pendingDCS = false
		if b == '\\' {
			p.state = stateGround
			return []Sequence{{Action: ActionDCS, Data: copyBytes(p.data), Command: 0}}
		}
		var seqs []Sequence
		seqs = append(seqs, Sequence{Action: ActionDCS, Data: copyBytes(p.data), Command: 0})
		p.state = stateGround
		seqs = append(seqs, p.Feed(b)...)
		return seqs
	}
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	switch b {
	case '[':
		p.resetSequence()
		p.state = stateCSIEntry
		return nil
	case ']':
		p.resetSequence()
		p.state = stateOSCString
		return nil
	case 'P':
		p.resetSequence()
		p.state = stateDCSString
		return nil
	case 'X', '^', '_':
		p.resetSequence()
		p.state = stateGround
		return nil
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateEscapeIntermediate
		return nil
	}
	if b >= 0x30 && b <= 0x7E {
		p.state = stateGround
		return []Sequence{{Action: ActionESC, Command: b, Intermed: copyBytes(p.intermed)}}
	}
	p.state = stateGround
	return nil
}

func (p *Parser) feedEscapeIntermediate(b byte) []Sequence {
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		return nil
	}
	if b >= 0x30 && b <= 0x7E {
		p.state = stateGround
		return []Sequence{{Action: ActionESC, Command: b, Intermed: copyBytes(p.intermed)}}
	}
	p.state = stateGround
	return nil
}

func (p *Parser) feedCSIEntry(b byte) []Sequence {
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	if b >= 0x30 && b <= 0x39 {
		p.curParam = int(b - '0')
		p.hasParam = true
		p.state = stateCSIParameter
		return nil
	}
	if b == ';' {
		p.params = append(p.params, p.curParam)
		p.curParam = 0
		p.hasParam = false
		p.state = stateCSIParameter
		return nil
	}
	if b == '?' || b == '>' || b == '=' || b == '<' {
		p.private = true
		p.state = stateCSIParameter
		return nil
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateCSIIntermediate
		return nil
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		return []Sequence{{
			Action:   ActionCSI,
			Command:  b,
			Params:   copyInts(p.params),
			Intermed: intermed,
		}}
	}
	p.state = stateGround
	return nil
}

func (p *Parser) feedCSIParameter(b byte) []Sequence {
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	if b >= 0x30 && b <= 0x39 {
		p.curParam = p.curParam*10 + int(b-'0')
		p.hasParam = true
		return nil
	}
	if b == ';' {
		p.params = append(p.params, p.curParam)
		p.curParam = 0
		p.hasParam = false
		return nil
	}
	if b == '?' || b == '>' || b == '=' || b == '<' {
		p.private = true
		return nil
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		p.state = stateCSIIntermediate
		return nil
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		return []Sequence{{
			Action:   ActionCSI,
			Command:  b,
			Params:   copyInts(p.params),
			Intermed: intermed,
		}}
	}
	p.state = stateGround
	return nil
}

func (p *Parser) feedCSIIntermediate(b byte) []Sequence {
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	if b >= 0x20 && b <= 0x2F {
		p.intermed = append(p.intermed, b)
		return nil
	}
	if b >= 0x40 && b <= 0x7E {
		p.finalizeParam()
		intermed := p.csiIntermed()
		p.state = stateGround
		return []Sequence{{
			Action:   ActionCSI,
			Command:  b,
			Params:   copyInts(p.params),
			Intermed: intermed,
		}}
	}
	p.state = stateGround
	return nil
}

func (p *Parser) feedOSCString(b byte) []Sequence {
	if b == 0x07 {
		p.state = stateGround
		return []Sequence{{Action: ActionOSC, Data: copyBytes(p.data)}}
	}
	if b == 0x1B {
		p.pendingOSC = true
		p.state = stateEscape
		return nil
	}
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	p.data = append(p.data, b)
	return nil
}

func (p *Parser) feedDCSString(b byte) []Sequence {
	if b == 0x07 {
		p.state = stateGround
		return []Sequence{{Action: ActionDCS, Data: copyBytes(p.data), Command: 0}}
	}
	if b == 0x1B {
		p.pendingDCS = true
		p.state = stateEscape
		return nil
	}
	if b < 0x20 {
		return []Sequence{{Action: ActionExecute, Command: b}}
	}
	p.data = append(p.data, b)
	return nil
}

func (p *Parser) FeedAll(data []byte) []Sequence {
	var result []Sequence
	for _, b := range data {
		seqs := p.Feed(b)
		result = append(result, seqs...)
	}
	return result
}

func (p *Parser) resetSequence() {
	p.params = p.params[:0]
	p.curParam = 0
	p.hasParam = false
	p.intermed = p.intermed[:0]
	p.data = p.data[:0]
	p.private = false
	p.pendingOSC = false
	p.pendingDCS = false
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
	buf = append(buf, '?')
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

func copyInts(s []int) []int {
	if len(s) == 0 {
		return nil
	}
	cp := make([]int, len(s))
	copy(cp, s)
	return cp
}
