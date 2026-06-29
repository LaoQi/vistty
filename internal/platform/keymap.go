package platform

import "strings"

var usKeyMap = map[uint32]rune{
	1: 0x1b,
	2: '1', 3: '2', 4: '3', 5: '4', 6: '5', 7: '6', 8: '7', 9: '8', 10: '9', 11: '0',
	12: '-', 13: '=', 14: 0x08,
	15: '\t',
	16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p',
	26: '[', 27: ']', 28: '\r',
	30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l',
	39: ';', 40: '\'', 41: '`',
	42: 0,
	43: '\\', 44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm',
	51: ',', 52: '.', 53: '/',
	54: 0,
	56: 0,
	57: ' ',
	58: 0,
	59: 0, 60: 0, 61: 0, 62: 0, 63: 0, 64: 0, 65: 0, 66: 0, 67: 0, 68: 0,
	97: 0,
	100: 0,
	103: 0,
	105: 0,
	106: 0,
	108: 0,
	125: 0,
	29: 0,
}

var modifierKeys = map[uint32]Modifiers{
	42:  ModShift,
	54:  ModShift,
	29:  ModCtrl,
	97:  ModCtrl,
	56:  ModAlt,
	100: ModAlt,
	125: ModSuper,
	126: ModSuper,
}

func LookupModifier(key uint32) (Modifiers, bool) {
	m, ok := modifierKeys[key]
	return m, ok
}

func LookupModifierCode(code uint16) bool {
	_, ok := modifierKeys[uint32(code)]
	return ok
}

func IsMappedKey(key uint32) bool {
	_, ok := usKeyMap[key]
	return ok
}

func FallbackKeyRune(key uint32, mods Modifiers) rune {
	r, ok := usKeyMap[key]
	if !ok {
		return 0
	}
	if mods&ModShift != 0 && r != 0 {
		r = ShiftRune(r)
	}
	if mods&ModCtrl != 0 && r >= 'a' && r <= 'z' {
		r = r - 'a' + 1
	}
	return r
}

func ShiftRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	switch r {
	case '1':
		return '!'
	case '2':
		return '@'
	case '3':
		return '#'
	case '4':
		return '$'
	case '5':
		return '%'
	case '6':
		return '^'
	case '7':
		return '&'
	case '8':
		return '*'
	case '9':
		return '('
	case '0':
		return ')'
	case '-':
		return '_'
	case '=':
		return '+'
	case '[':
		return '{'
	case ']':
		return '}'
	case '\\':
		return '|'
	case ';':
		return ':'
	case '\'':
		return '"'
	case ',':
		return '<'
	case '.':
		return '>'
	case '/':
		return '?'
	case '`':
		return '~'
	}
	return r
}

var keyNameMap = map[string]struct {
	r    rune
	code uint16
}{
	"equal":     {'=', 0},
	"minus":     {'-', 0},
	"0":         {'0', 0},
	"1":         {'1', 2},
	"2":         {'2', 3},
	"3":         {'3', 4},
	"4":         {'4', 5},
	"5":         {'5', 6},
	"6":         {'6', 7},
	"7":         {'7', 8},
	"8":         {'8', 9},
	"9":         {'9', 10},
	"t":         {'t', 0},
	"w":         {'w', 0},
	"Tab":       {0, 15},
	"Left":      {0, 105},
	"Right":     {0, 106},
	"Up":        {0, 103},
	"Down":      {0, 108},
	"Return":    {0, 28},
	"Backspace": {0, 14},
	"Escape":    {0, 1},
	"Space":     {' ', 57},
	"Page_Up":   {0, 104},
	"Page_Down": {0, 109},
	"Home":      {0, 102},
	"End":       {0, 107},
	"Delete":    {0, 111},
	"Insert":    {0, 110},
}

func ParseModKey(s string) Modifiers {
	switch strings.ToLower(s) {
	case "alt":
		return ModAlt
	case "ctrl", "control":
		return ModCtrl
	case "shift":
		return ModShift
	case "super", "win", "meta":
		return ModSuper
	default:
		return ModSuper
	}
}

func ParseKey(s string) (rune, uint16) {
	if entry, ok := keyNameMap[s]; ok {
		return entry.r, entry.code
	}
	if len(s) == 1 {
		return rune(s[0]), 0
	}
	return 0, 0
}
