package platform

var usKeyMap = map[uint32]rune{
	1: 0x1b,
	2: '1', 3: '2', 4: '3', 5: '4', 6: '5', 7: '6', 8: '7', 9: '8', 10: '9', 11: '0',
	12: '-', 13: '=', 14: 0x08,
	15: '\t',
	16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p',
	26: '[', 27: ']', 28: '\r',
	29:  0, // LeftCtrl
	30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l',
	39: ';', 40: '\'', 41: '`',
	42: 0, // LeftShift
	43: '\\', 44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm',
	51: ',', 52: '.', 53: '/',
	54:  0,  // RightShift
	55:  '*', // KP asterisk
	56:  0,  // LeftAlt
	57: ' ',
	58:  0, // CapsLock
	59: 0, 60: 0, 61: 0, 62: 0, 63: 0, 64: 0, 65: 0, 66: 0, 67: 0, 68: 0, // F1-F10
	69:  0,  // NumLock
	70:  0,  // ScrollLock
	71:  '7', 72: '8', 73: '9', // KP7 KP8 KP9
	74:  '-', // KP minus
	75:  '4', 76: '5', 77: '6', // KP4 KP5 KP6
	78:  '+', // KP plus
	79:  '1', 80: '2', 81: '3', // KP1 KP2 KP3
	82:  '0', 83: '.', // KP0 KPDOT
	87: 0, 88: 0, // F11 F12
	96:  '\r', // KP enter
	97:  0,   // RightCtrl
	98:  '/', // KP slash
	99:  0,   // SysRq
	100: 0,   // RightAlt
	102: 0,   // Home
	103: 0,   // Up
	104: 0,   // PageUp
	105: 0,   // Left
	106: 0,   // Right
	107: 0,   // End
	108: 0,   // Down
	109: 0,   // PageDown
	110: 0,   // Insert
	111: 0,   // Delete
	117: '=', // KP equal
	119: 0,   // Pause
	125: 0,   // LeftSuper
	126: 0,   // RightSuper
	139: 0,   // Menu
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
