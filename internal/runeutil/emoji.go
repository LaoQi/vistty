package runeutil

func IsEmojiRune(r rune) bool {
	switch {
	case 0x1F600 <= r && r <= 0x1F64F:
		return true
	case 0x1F300 <= r && r <= 0x1F5FF:
		return true
	case 0x1F680 <= r && r <= 0x1F6FF:
		return true
	case 0x1F1E6 <= r && r <= 0x1F1FF:
		return true
	case 0x2600 <= r && r <= 0x26FF:
		return true
	case r == 0x2712 || r == 0x2714 || r == 0x2716 || r == 0x271D ||
		r == 0x2721 || r == 0x2728:
		return true
	case 0x2733 <= r && r <= 0x2734:
		return true
	case r == 0x2744 || r == 0x2747 || r == 0x274C || r == 0x274E:
		return true
	case 0x2753 <= r && r <= 0x2755:
		return true
	case r == 0x2757 || r == 0x2763 || r == 0x2764:
		return true
	case 0x2795 <= r && r <= 0x2797:
		return true
	case r == 0x27A1 || r == 0x27B0 || r == 0x27BF:
		return true
	case 0xFE00 <= r && r <= 0xFE0F:
		return true
	case 0x1F900 <= r && r <= 0x1F9FF:
		return true
	case 0x1FA00 <= r && r <= 0x1FA6F:
		return true
	case 0x1FA70 <= r && r <= 0x1FAFF:
		return true
	case 0x231A <= r && r <= 0x231B:
		return true
	case 0x23E9 <= r && r <= 0x23F3:
		return true
	case 0x23F8 <= r && r <= 0x23FA:
		return true
	case 0x25AA <= r && r <= 0x25FE:
		return true
	case 0x2614 <= r && r <= 0x2615:
		return true
	case 0x2648 <= r && r <= 0x2653:
		return true
	case 0x267F <= r && r <= 0x267F:
		return true
	case 0x2693 <= r && r <= 0x2693:
		return true
	case 0x26A1 <= r && r <= 0x26A1:
		return true
	case 0x26AA <= r && r <= 0x26AB:
		return true
	case 0x26BD <= r && r <= 0x26BE:
		return true
	case 0x26C4 <= r && r <= 0x26C5:
		return true
	case 0x26CE <= r && r <= 0x26CE:
		return true
	case 0x26D4 <= r && r <= 0x26D4:
		return true
	case 0x26EA <= r && r <= 0x26EA:
		return true
	case 0x26F2 <= r && r <= 0x26F3:
		return true
	case 0x26F5 <= r && r <= 0x26F5:
		return true
	case 0x26FA <= r && r <= 0x26FA:
		return true
	case 0x26FD <= r && r <= 0x26FD:
		return true
	case 0x2702 <= r && r <= 0x2702:
		return true
	case 0x2705 <= r && r <= 0x2705:
		return true
	case 0x2708 <= r && r <= 0x270D:
		return true
	case 0x270F <= r && r <= 0x270F:
		return true
	case 0x2B50 <= r && r <= 0x2B50:
		return true
	case 0x2B55 <= r && r <= 0x2B55:
		return true
	case 0x200D == r:
		return true
	case 0x20E3 == r:
		return true
	case 0xE0020 <= r && r <= 0xE007F:
		return true
	}
	return false
}

func isEmojiModifier(r rune) bool {
	switch {
	case r == 0x200D:
		return true
	case r == 0x20E3:
		return true
	case 0xFE00 <= r && r <= 0xFE0F:
		return true
	case 0xE0020 <= r && r <= 0xE007F:
		return true
	case 0x1F3FB <= r && r <= 0x1F3FF:
		return true
	}
	return false
}
