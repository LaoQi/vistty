package wayland

import (
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
)

// usKeymapFixture 是一个最小但结构完整的 XKB keymap 文本样本，
// 覆盖 keycodes/types/compat/symbols/geometry 全部段。types/compat 段内部
// 含嵌套块（`type "X" { ... };` / `interpret X { ... };`），其结束 `};`
// 与段结束 `};` 同为独立成行，用于回归保护：简单的行首 `}` 判定会误把
// 嵌套块结束当作段结束，导致状态机提前跳过 symbols 段。symbols 段含 3 个
// `key <X> { [...] };` 行。其中 <AC01> 在 keycodes 段默认为 'a'，在 symbols
// 段被覆盖为 'x'，用于明确区分 keycodes 默认值与 symbols 覆盖值。
const usKeymapFixture = `xkb_keymap {
    xkb_keycodes {
        <ESC> = 9;
        <AE01> = 10;
        <AE06> = 15;
        <AE07> = 16;
        <BKSP> = 22;
        <TAB> = 23;
        <AD01> = 24;
        <AC01> = 38;
        <SPCE> = 65;
    };
    xkb_types {
        type "ONE_LEVEL" {
            modifiers = Shift;
            map[Shift] = Level2;
        };
    };
    xkb_compat {
        interpret Shift_L {
            action = SetMods(modifiers=Shift);
        };
    };
    xkb_symbols {
        name[Group1]= "English (US)";
        key <AD01> { [ q, Q ] };
        key <AC01> { [ x, X ] };
        key <AE01> { [ 1, ! ] };
    };
    xkb_geometry { include "pc(pc104)" };
};
`

func TestBraceDelta(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"xkb_keycodes {", 1},
		{"};", -1},
		{"    };", -1},
		{"key <AC01> { [ x, X ] };", 0},  // 一对花括号，delta=0
		{"<ESC> = 9;", 0},
		{"name[Group1]= \"English (US)\";", 0},
		{"type \"ONE_LEVEL\" {", 1},
		{"xkb_geometry { include \"pc(pc104)\" };", 0}, // 单行段，{ } 抵消
		{"{", 1},
		{"}}", -2},
		{"{{}", 1},
		{"", 0},
	}
	for _, c := range cases {
		if got := braceDelta([]byte(c.line)); got != c.want {
			t.Errorf("braceDelta(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}

func TestParseKeymapData_KeycodesSection(t *testing.T) {
	km := parseKeymapData([]byte(usKeymapFixture))

	// keycodes 段：keyNameToKeysym 提供默认映射，按 Linux keycode 索引
	// （parseKeycodeLine 用 xkbToEvdev 将 XKB keycode 转为 Linux keycode）。
	// <AE06>=15 -> xkbToEvdev(15)=7 -> km[7].level0='6'
	if r := km.lookup(7, 0); r != '6' {
		t.Errorf("AE06: lookup(7,0) = %q, want '6'", r)
	}
	// <AE07>=16 -> xkbToEvdev(16)=8 -> km[8].level0='7'
	if r := km.lookup(8, 0); r != '7' {
		t.Errorf("AE07: lookup(8,0) = %q, want '7'", r)
	}
	// <SPCE>=65 -> xkbToEvdev(65)=57 -> km[57].level0=' '
	if r := km.lookup(57, 0); r != ' ' {
		t.Errorf("SPCE: lookup(57,0) = %q, want ' '", r)
	}
}

func TestParseKeymapData_SymbolsOverride(t *testing.T) {
	km := parseKeymapData([]byte(usKeymapFixture))

	// <AC01>=38 -> xkbToEvdev(38)=30。keycodes 段默认 km[30].level0='a'，
	// symbols 段 `key <AC01> { [ x, X ] };` 覆盖为 'x'。
	// 若 symbols 段未被解析（段定位失败），此处会得到 'a'。
	if r := km.lookup(30, 0); r != 'x' {
		t.Errorf("AC01 symbols override: lookup(30,0) = %q, want 'x' (symbols section not parsed?)", r)
	}
	if r := km.lookup(30, platform.ModShift); r != 'X' {
		t.Errorf("AC01 shift: lookup(30,ModShift) = %q, want 'X'", r)
	}

	// <AD01>=24 -> xkbToEvdev(24)=16。symbols 段 `key <AD01> { [ q, Q ] };`
	// 与 keycodes 默认一致，仍为 'q'。
	if r := km.lookup(16, 0); r != 'q' {
		t.Errorf("AD01: lookup(16,0) = %q, want 'q'", r)
	}
	if r := km.lookup(16, platform.ModShift); r != 'Q' {
		t.Errorf("AD01 shift: lookup(16,ModShift) = %q, want 'Q'", r)
	}

	// <AE01>=10 -> xkbToEvdev(10)=2。symbols 段 `key <AE01> { [ 1, ! ] };`
	// level1='!'（单字符 keysym）。
	if r := km.lookup(2, 0); r != '1' {
		t.Errorf("AE01: lookup(2,0) = %q, want '1'", r)
	}
	if r := km.lookup(2, platform.ModShift); r != '!' {
		t.Errorf("AE01 shift: lookup(2,ModShift) = %q, want '!'", r)
	}
}

func TestParseKeymapData_MultipleKeyLinesParsed(t *testing.T) {
	km := parseKeymapData([]byte(usKeymapFixture))

	// 验证 symbols 段在 types/compat 嵌套块之后被正确定位，且段内多个
	// key 行均被解析。AC01 是 symbols 段的第 2 个 key 行，keycodes 默认 'a'，
	// symbols 覆盖为 'x'。若 types/compat 的嵌套块 `};` 被误判为段结束，
	// 状态机会在到达 xkb_symbols 之前提前跳走，symbols 段永不进入，
	// 此处将得到 'a'。
	if r := km.lookup(30, 0); r != 'x' {
		t.Errorf("symbols section not located after types/compat nested blocks: lookup(30,0) = %q, want 'x'", r)
	}
}

func TestParseKeymapData_OutOfBoundsFallback(t *testing.T) {
	km := parseKeymapData([]byte(usKeymapFixture))

	// 超出 km 长度（256）-> 走 FallbackKeyRune，usKeyMap 无 300 -> 0
	if r := km.lookup(300, 0); r != 0 {
		t.Errorf("lookup(300,0) = %q, want 0 (out of bounds fallback)", r)
	}
	// km 内未设置的位置（level0==0）-> FallbackKeyRune。
	// usKeyMap[16]='q'，但 km[16] 已被 fixture 设置为 'q'，故选一个 fixture
	// 未覆盖且 usKeyMap 有的键：Linux keycode 53 ('/')。
	if r := km.lookup(53, 0); r != '/' {
		t.Errorf("unset km slot fallback: lookup(53,0) = %q, want '/' (usKeyMap[53])", r)
	}
}

// TestLookupExpectsLinuxKeycode 是 P0 回归保护：验证 km 按 Linux keycode
// 索引（而非 XKB keycode）。Wayland wl_keyboard.key 的 key 参数即 Linux
// keycode（与 /dev/input/event* 的 input_event.code 同构），应直接传入
// lookup，不做任何 -8 偏移。km[16] 对应 Linux KEY_Q=16（XKB keycode 24）。
func TestLookupExpectsLinuxKeycode(t *testing.T) {
	km := parseKeymapData([]byte(usKeymapFixture))

	// Linux keycode 16 = KEY_Q。若错误地传入 XKB keycode 24，会越界或错位。
	if r := km.lookup(16, 0); r != 'q' {
		t.Errorf("lookup expects Linux keycode: lookup(16,0) = %q, want 'q' (Linux KEY_Q=16)", r)
	}
	// 反证：传入 XKB keycode 24（即错误地不减 8 的旧路径）不应得到 'q'。
	if r := km.lookup(24, 0); r == 'q' {
		t.Errorf("lookup(24,0) = 'q' incorrectly; km is indexed by Linux keycode, 24 is out of the mapped range")
	}
}
