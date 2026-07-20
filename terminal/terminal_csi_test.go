package terminal

import "testing"

func TestEraseCharsECH(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("ABCDE"))
	term.cursor.Col = 1
	term.FeedBytes([]byte("\x1b[2X"))
	cell := term.screen.Cell(0, 0)
	if cell.Rune != 'A' {
		t.Errorf("expected 'A' at col 0, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 1)
	if cell.Rune != ' ' {
		t.Errorf("expected ' ' at col 1, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 2)
	if cell.Rune != ' ' {
		t.Errorf("expected ' ' at col 2, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 3)
	if cell.Rune != 'D' {
		t.Errorf("expected 'D' at col 3, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 4)
	if cell.Rune != 'E' {
		t.Errorf("expected 'E' at col 4, got %c", cell.Rune)
	}
	if term.cursor.Col != 1 {
		t.Errorf("cursor should not move, expected col 1, got %d", term.cursor.Col)
	}
}

func TestEraseDisplayScrollback(t *testing.T) {
	term, _ := newTerminalForTest(80, 3)
	term.FeedBytes([]byte("line1\nline2\nline3\nline4"))
	if term.screen.History().Len() == 0 {
		t.Fatal("expected history to have content after scroll")
	}
	term.FeedBytes([]byte("\x1b[3J"))
	if term.screen.History().Len() != 0 {
		t.Errorf("expected history cleared, got len %d", term.screen.History().Len())
	}
}

func TestTabClearAll(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 8 {
		t.Fatalf("expected col 8 after tab, got %d", term.cursor.Col)
	}
	term.FeedBytes([]byte("\x1b[3g"))
	term.cursor.Col = 0
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 79 {
		t.Errorf("expected col 79 (no tab stops), got %d", term.cursor.Col)
	}
}

func TestTabClearCurrent(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[g"))
	term.cursor.Col = 0
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 8 {
		t.Errorf("expected col 8 (col 0 cleared but 8 remains), got %d", term.cursor.Col)
	}
}

func TestCursorClamp(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 5
	term.cursor.Col = 5
	term.FeedBytes([]byte("\x1b[999A"))
	if term.cursor.Row != 0 {
		t.Errorf("expected row 0, got %d", term.cursor.Row)
	}
	if term.cursor.Col != 5 {
		t.Errorf("col should not change, expected 5, got %d", term.cursor.Col)
	}
}

func TestScrollRegionResetNoParams(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.screen.SetScrollRegion(5, 10)
	if term.screen.ScrollTop() != 5 || term.screen.ScrollBot() != 10 {
		t.Fatalf("setup failed: expected region [5,10], got [%d,%d]", term.screen.ScrollTop(), term.screen.ScrollBot())
	}
	term.FeedBytes([]byte("\x1b[r"))
	if term.screen.ScrollTop() != 0 || term.screen.ScrollBot() != 23 {
		t.Errorf("ESC[r should reset region to full screen [0,23], got [%d,%d]", term.screen.ScrollTop(), term.screen.ScrollBot())
	}
}

func TestScrollRegionResetZeroParams(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.screen.SetScrollRegion(5, 10)
	term.FeedBytes([]byte("\x1b[0;0r"))
	if term.screen.ScrollTop() != 0 || term.screen.ScrollBot() != 23 {
		t.Errorf("ESC[0;0r should reset region to full screen [0,23], got [%d,%d]", term.screen.ScrollTop(), term.screen.ScrollBot())
	}
}

func TestScrollRegionTopOnly(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.screen.SetScrollRegion(5, 10)
	term.FeedBytes([]byte("\x1b[3r"))
	if term.screen.ScrollTop() != 2 || term.screen.ScrollBot() != 23 {
		t.Errorf("ESC[3r should set region [2,23], got [%d,%d]", term.screen.ScrollTop(), term.screen.ScrollBot())
	}
}

func TestScrollRegionBothParams(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[3;10r"))
	if term.screen.ScrollTop() != 2 || term.screen.ScrollBot() != 9 {
		t.Errorf("ESC[3;10r should set region [2,9], got [%d,%d]", term.screen.ScrollTop(), term.screen.ScrollBot())
	}
}

func TestInsertLinesCSI(t *testing.T) {
	term, _ := newTerminalForTest(10, 10)
	term.FeedBytes([]byte("0\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9"))
	term.cursor.Row = 2
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[2L"))
	want := []rune{'0', '1', ' ', ' ', '2', '3', '4', '5', '6', '7'}
	for i, w := range want {
		got := term.screen.Cell(i, 0).Rune
		if got != w {
			t.Errorf("row %d: expected '%c', got '%c'", i, w, got)
		}
	}
	if term.screen.History().Len() != 0 {
		t.Errorf("IL should not push history, got len %d", term.screen.History().Len())
	}
}

func TestDeleteLinesCSI(t *testing.T) {
	term, _ := newTerminalForTest(10, 10)
	term.FeedBytes([]byte("0\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9"))
	term.cursor.Row = 2
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[2M"))
	want := []rune{'0', '1', '4', '5', '6', '7', '8', '9', ' ', ' '}
	for i, w := range want {
		got := term.screen.Cell(i, 0).Rune
		if got != w {
			t.Errorf("row %d: expected '%c', got '%c'", i, w, got)
		}
	}
	if term.screen.History().Len() != 0 {
		t.Errorf("DL should not push history, got len %d", term.screen.History().Len())
	}
}

func TestInsertLinesCSIRespectsScrollRegion(t *testing.T) {
	term, _ := newTerminalForTest(10, 10)
	term.FeedBytes([]byte("0\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9"))
	term.FeedBytes([]byte("\x1b[3;8r"))
	term.cursor.Row = 4
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[2L"))
	want := []rune{'0', '1', '2', '3', ' ', ' ', '4', '5', '8', '9'}
	for i, w := range want {
		got := term.screen.Cell(i, 0).Rune
		if got != w {
			t.Errorf("row %d: expected '%c', got '%c'", i, w, got)
		}
	}
}

func TestDeleteLinesCSIRespectsScrollRegion(t *testing.T) {
	term, _ := newTerminalForTest(10, 10)
	term.FeedBytes([]byte("0\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9"))
	term.FeedBytes([]byte("\x1b[3;8r"))
	term.cursor.Row = 4
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[2M"))
	want := []rune{'0', '1', '2', '3', '6', '7', ' ', ' ', '8', '9'}
	for i, w := range want {
		got := term.screen.Cell(i, 0).Rune
		if got != w {
			t.Errorf("row %d: expected '%c', got '%c'", i, w, got)
		}
	}
}

func TestInsertLinesCSINoopOutsideRegion(t *testing.T) {
	term, _ := newTerminalForTest(10, 10)
	term.FeedBytes([]byte("0\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9"))
	term.FeedBytes([]byte("\x1b[3;8r"))
	term.cursor.Row = 0
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[2L"))
	for i := 0; i < 10; i++ {
		if got := term.screen.Cell(i, 0).Rune; got != rune('0'+i) {
			t.Errorf("row %d: IL above scrollTop should be no-op, got '%c'", i, got)
		}
	}
}
