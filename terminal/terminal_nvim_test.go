package terminal

import (
	"os"
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

// TestNvimCursorUpRewrite reproduces the nvim cursor-up rendering bug.
// nvim uses CUU (CSI A) to move cursor up then rewrites only the target rows.
// If the rewrite sequence is mishandled, the highlighted row shows stale content.
func TestNvimCursorUpRewrite(t *testing.T) {
	// nvim alt-screen: 80 cols x 54 rows (from debug log)
	term, _ := newTerminalForTest(80, 54)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	// Initial full redraw: fill rows 1-3 with distinct content
	// Row 1: "AAA", Row 2: "BBB", Row 3: "CCC"
	initial := []byte("\x1b[H\x1b[2J")
	for _, r := range []string{"AAA", "BBB", "CCC"} {
		initial = append(initial, []byte(r)...)
		initial = append(initial, '\r', '\n')
	}
	term.feedBytes(initial)

	// Verify initial content
	checkCell := func(row, col int, want rune) {
		cell := term.screen.Cell(row, col)
		if cell == nil {
			t.Fatalf("cell(%d,%d) is nil", row, col)
		}
		if cell.Rune != want {
			t.Errorf("cell(%d,%d) = %q, want %q", row, col, cell.Rune, want)
		}
	}
	checkCell(0, 0, 'A')
	checkCell(1, 0, 'B')
	checkCell(2, 0, 'C')

	// Simulate nvim cursor-up rewrite: move to row 54, CUU(52) to row 2,
	// then rewrite rows 2-3 with new content.
	// This is the exact pattern from the debug log.
	upSeq := []byte("\x1b[54;1H\x1b[52A") // CUP to last row, then CUU 52 -> row 2 (0-indexed row 1)
	term.feedBytes(upSeq)

	// Cursor should be at row 1 (0-indexed), col 0
	if term.cursor.Row != 1 {
		t.Fatalf("after CUU(52): cursor.Row = %d, want 1", term.cursor.Row)
	}
	if term.cursor.Col != 0 {
		t.Fatalf("after CUU(52): cursor.Col = %d, want 0", term.cursor.Col)
	}

	// Rewrite row 2 with "XXX" + EL, then CRLF, then row 3 with "YYY"
	rewrite := []byte("XXX\x1b[K\r\nYYY\x1b[K")
	term.feedBytes(rewrite)

	// Row 2 (0-indexed 1) should now be "XXX"
	checkCell(1, 0, 'X')
	checkCell(1, 1, 'X')
	checkCell(1, 2, 'X')
	// Row 3 (0-indexed 2) should now be "YYY"
	checkCell(2, 0, 'Y')
	checkCell(2, 1, 'Y')
	checkCell(2, 2, 'Y')
	// Row 1 (0-indexed 0) should be untouched: "AAA"
	checkCell(0, 0, 'A')
}

// TestNvimCursorUpWithSpaces tests the case where nvim rewrites the number
// column and content — the exact scenario from the debug log where
// "  2 " prefix is written before EL.
func TestNvimCursorUpWithPrefix(t *testing.T) {
	term, _ := newTerminalForTest(80, 54)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	// Fill rows 1-3: "  1 line1", "  2 line2", "  3 line3"
	rows := []string{"  1 line1content", "  2 line2content", "  3 line3content"}
	term.feedBytes([]byte("\x1b[H\x1b[2J"))
	for _, r := range rows {
		term.feedBytes([]byte(r + "\r\n"))
	}

	// Move cursor to row 2 (0-indexed 1), rewrite "  2 " + EL
	term.feedBytes([]byte("\x1b[2;1H"))
	term.feedBytes([]byte("  2 \x1b[K"))

	// After "  2 " (5 chars) + EL(0), the rest of row 2 should be cleared
	cell := term.screen.Cell(1, 0)
	if cell == nil || cell.Rune != ' ' {
		t.Errorf("row2 col0: want space, got %v", cell)
	}
	// col 5 should be cleared (was 'l' of "line2content")
	cell = term.screen.Cell(1, 5)
	if cell == nil {
		t.Fatal("cell(1,5) is nil")
	}
	if cell.Rune != ' ' {
		t.Errorf("row2 col5 after EL: want space, got %q", cell.Rune)
	}
}

// TestNvimRealSequence replays the exact byte stream captured from a real
// nvim session (alt-screen entry, full redraw, cursor down, cursor up) and
// checks that the screen buffer matches the expected nvim output.
func TestNvimRealSequence(t *testing.T) {
	data, err := os.ReadFile("/tmp/nvim_full.bin")
	if err != nil {
		t.Skipf("skip: cannot read /tmp/nvim_full.bin: %v", err)
	}

	term, _ := newTerminalForTest(80, 54)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	term.feedBytes(data)

	// Dump rows 0-6 for inspection
	for row := 0; row < 7; row++ {
		line := term.screen.Line(row)
		if line == nil {
			continue
		}
		s := ""
		for col := 0; col < line.Width() && col < 40; col++ {
			cell := line.Cell(col)
			if cell != nil && cell.Rune != 0 && cell.Rune != ' ' {
				s += string(cell.Rune)
			} else {
				s += "."
			}
		}
		t.Logf("row %2d: %s", row, s)
	}
	t.Logf("cursor: row=%d col=%d wrap=%v", term.cursor.Row, term.cursor.Col, term.cursor.WrapPending)

	// After the full sequence (initial redraw + down j + up k):
	// row 0 should be "  1 # Vistty修复..." (preserved, not scrolled away)
	cell := term.screen.Cell(0, 4)
	if cell == nil {
		t.Fatal("cell(0,4) is nil")
	}
	if cell.Rune != '#' {
		t.Errorf("cell(0,4) = %q, want '#' (row 0 should preserve title)", cell.Rune)
	}
}

// TestSGRDoesNotResetWrapPending verifies that SGR sequences between a
// wrap-pending char and the next print char do NOT reset deferred wrap.
// This is the root cause of nvim rendering glitches: nvim writes a char at
// the last column, then sends SGR (color changes), then writes the next char.
// xterm-compatible behavior: SGR preserves pending wrap, so the next char
// wraps to the next line instead of overwriting the last column.
func TestSGRDoesNotResetWrapPending(t *testing.T) {
	term, _ := newTerminalForTest(5, 3)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	term.feedBytes([]byte("ABCDE"))
	if !term.cursor.WrapPending {
		t.Fatal("after filling row: WrapPending should be true")
	}

	term.feedBytes([]byte("\x1b[m\x1b[31m"))
	if !term.cursor.WrapPending {
		t.Fatal("after SGR: WrapPending should still be true")
	}

	term.feedBytes([]byte("F"))
	if term.cursor.Row != 1 {
		t.Fatalf("after print 'F': cursor.Row = %d, want 1 (wrapped)", term.cursor.Row)
	}
	cell := term.screen.Cell(0, 4)
	if cell == nil || cell.Rune != 'E' {
		t.Errorf("cell(0,4) = %v, want 'E' (not overwritten)", cell)
	}
	cell = term.screen.Cell(1, 0)
	if cell == nil || cell.Rune != 'F' {
		t.Errorf("cell(1,0) = %v, want 'F'", cell)
	}
}

// TestCharsetDesignateDoesNotResetWrapPending verifies that ESC ( B
// (designate G0 charset) does not reset deferred wrap.
func TestCharsetDesignateDoesNotResetWrapPending(t *testing.T) {
	term, _ := newTerminalForTest(5, 3)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	term.feedBytes([]byte("ABCDE"))
	term.feedBytes([]byte("\x1b(B"))
	if !term.cursor.WrapPending {
		t.Fatal("after ESC ( B: WrapPending should still be true")
	}

	term.feedBytes([]byte("F"))
	if term.cursor.Row != 1 {
		t.Fatalf("after print 'F': cursor.Row = %d, want 1 (wrapped)", term.cursor.Row)
	}
}

// TestCursorMoveResetsWrapPending verifies that cursor positioning commands
// DO reset deferred wrap (contrast with SGR/charset).
func TestCursorMoveResetsWrapPending(t *testing.T) {
	term, _ := newTerminalForTest(5, 3)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	term.feedBytes([]byte("ABCDE"))
	term.feedBytes([]byte("\x1b[1;1H"))
	if term.cursor.WrapPending {
		t.Fatal("after CUP: WrapPending should be false")
	}

	term.feedBytes([]byte("X"))
	cell := term.screen.Cell(0, 0)
	if cell == nil || cell.Rune != 'X' {
		t.Errorf("cell(0,0) = %v, want 'X' (overwritten at CUP position)", cell)
	}
}

// TestEraseUsesCurrentBg verifies that EL (Erase in Line) fills erased cells
// with the current SGR background color, not the default black background.
func TestEraseUsesCurrentBg(t *testing.T) {
	term, _ := newTerminalForTest(10, 3)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	// Set custom background via SGR 48;2;R;G;B
	term.feedBytes([]byte("\x1b[48;2;46;50;60m"))
	// Fill row 0 with "AAAAAAAAAA"
	term.feedBytes([]byte("AAAAAAAAAA"))
	// Move to col 3, erase to end of line (EL 0)
	term.feedBytes([]byte("\x1b[1;4H\x1b[K"))

	// Cells 3-9 should be spaces with Bg = {46,50,60}
	wantBg := screen.Color{R: 46, G: 50, B: 60}
	for col := 3; col < 10; col++ {
		cell := term.screen.Cell(0, col)
		if cell == nil {
			t.Fatalf("cell(0,%d) is nil", col)
		}
		if cell.Bg != wantBg {
			t.Errorf("cell(0,%d).Bg = %+v, want {46,50,60}", col, cell.Bg)
		}
		if cell.Rune != ' ' {
			t.Errorf("cell(0,%d).Rune = %q, want space", col, cell.Rune)
		}
	}
	// Cells 0-2 should be 'A' (untouched)
	for col := 0; col < 3; col++ {
		cell := term.screen.Cell(0, col)
		if cell == nil || cell.Rune != 'A' {
			t.Errorf("cell(0,%d) = %v, want 'A'", col, cell)
		}
	}
}

// TestScrollUpNewLineUsesCurrentBg verifies that new lines created by
// scroll-up are filled with the current SGR background color.
func TestScrollUpNewLineUsesCurrentBg(t *testing.T) {
	term, _ := newTerminalForTest(5, 3)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	// Fill all 3 rows with 'X'
	term.feedBytes([]byte("XXXXX\r\nXXXXX\r\nXXXXX"))
	// Set custom background, then scroll up 1 line
	term.feedBytes([]byte("\x1b[48;2;46;50;60m\x1b[S"))

	// Bottom row (row 2) should be blank cells with custom Bg
	wantBg := screen.Color{R: 46, G: 50, B: 60}
	for col := 0; col < 5; col++ {
		cell := term.screen.Cell(2, col)
		if cell == nil {
			t.Fatalf("cell(2,%d) is nil", col)
		}
		if cell.Bg != wantBg {
			t.Errorf("cell(2,%d).Bg = %+v, want {46,50,60}", col, cell.Bg)
		}
	}
}

// TestEraseCharsUsesCurrentBg verifies that ECH (Erase Characters) fills
// erased cells with the current SGR background color.
func TestEraseCharsUsesCurrentBg(t *testing.T) {
	term, _ := newTerminalForTest(10, 1)
	term.screen = term.altBuf
	term.cursor = term.altBuf.Cursor()
	term.altBuf.ClearAll()

	// Fill row with 'A', set custom bg, erase 3 chars at col 2
	term.feedBytes([]byte("AAAAAAAAAA"))
	term.feedBytes([]byte("\x1b[48;2;46;50;60m\x1b[2G\x1b[3X"))

	wantBg := screen.Color{R: 46, G: 50, B: 60}
	for col := 1; col < 4; col++ {
		cell := term.screen.Cell(0, col)
		if cell == nil {
			t.Fatalf("cell(0,%d) is nil", col)
		}
		if cell.Bg != wantBg {
			t.Errorf("cell(0,%d).Bg = %+v, want {46,50,60}", col, cell.Bg)
		}
	}
}

