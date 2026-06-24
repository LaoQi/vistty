package terminal

import "unicode/utf8"

type Charset byte

const (
	CharsetUS  Charset = 'B'
	CharsetDEC Charset = '0'
	CharsetUK  Charset = 'A'
)

var decSpecialGraphic = map[rune]rune{
	'`': '\u25C6',
	'a': '\u2592',
	'b': '\u2409',
	'c': '\u240C',
	'd': '\u240D',
	'e': '\u240A',
	'f': '\u00B0',
	'g': '\u00B1',
	'h': '\u2424',
	'i': '\u240B',
	'j': '\u2518',
	'k': '\u2510',
	'l': '\u250C',
	'm': '\u2514',
	'n': '\u253C',
	'o': '\u23BA',
	'p': '\u23BB',
	'q': '\u2500',
	'r': '\u23BC',
	's': '\u23BD',
	't': '\u251C',
	'u': '\u2524',
	'v': '\u2534',
	'w': '\u252C',
	'x': '\u2502',
	'y': '\u2264',
	'z': '\u2265',
	'{': '\u03C0',
	'|': '\u2260',
	'}': '\u00A3',
	'~': '\u00B7',
}

func (cs Charset) Translate(r rune) rune {
	if cs != CharsetDEC {
		return r
	}
	if mapped, ok := decSpecialGraphic[r]; ok {
		return mapped
	}
	return r
}

type charsetState struct {
	g0     Charset
	g1     Charset
	glIsG1 bool
}

func newCharsetState() charsetState {
	return charsetState{g0: CharsetUS, g1: CharsetUS}
}

func (cs *charsetState) designateG0(c byte) { cs.g0 = Charset(c) }
func (cs *charsetState) designateG1(c byte) { cs.g1 = Charset(c) }
func (cs *charsetState) shiftOut()          { cs.glIsG1 = true }
func (cs *charsetState) shiftIn()           { cs.glIsG1 = false }

func (cs *charsetState) current() Charset {
	if cs.glIsG1 {
		return cs.g1
	}
	return cs.g0
}

func (cs *charsetState) translate(data []byte) []rune {
	var runes []rune
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == 0xFFFD && size == 1 {
			runes = append(runes, r)
			i++
			continue
		}
		runes = append(runes, cs.current().Translate(r))
		i += size
	}
	return runes
}
