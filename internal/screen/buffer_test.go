package screen

import (
	"sync"
	"testing"
)

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(80, 24)
	if b.Cols() != 80 || b.Rows() != 24 {
		t.Errorf("expected 80x24, got %dx%d", b.Cols(), b.Rows())
	}
	if b.scrollTop != 0 || b.scrollBot != 23 {
		t.Errorf("expected scroll region [0,23], got [%d,%d]", b.scrollTop, b.scrollBot)
	}
}

func TestBufferCell(t *testing.T) {
	b := NewBuffer(10, 5)
	c := b.Cell(0, 0)
	if c == nil {
		t.Fatal("Cell(0,0) returned nil")
	}
	if c.Rune != ' ' {
		t.Error("expected default cell")
	}
	c.Rune = 'Z'
	if b.Cell(0, 0).Rune != 'Z' {
		t.Error("cell modification not reflected")
	}
	if b.Cell(-1, 0) != nil || b.Cell(5, 0) != nil || b.Cell(0, 10) != nil {
		t.Error("out-of-bounds Cell should return nil")
	}
}

func TestBufferLine(t *testing.T) {
	b := NewBuffer(10, 5)
	if b.Line(-1) != nil || b.Line(5) != nil {
		t.Error("out-of-bounds Line should return nil")
	}
	if b.Line(0) == nil {
		t.Error("Line(0) should not be nil")
	}
}

func TestBufferScrollUp(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		b.Cell(i, 0).Rune = rune('0' + i)
	}
	h := NewHistory(100)
	for i := 0; i < 3; i++ {
		h.Push(b.Line(i))
	}
	b.ScrollUp(3)
	for i := 0; i < 2; i++ {
		if b.Cell(i, 0).Rune != rune('3'+i) {
			t.Errorf("row %d: expected '%c', got '%c'", i, '3'+i, b.Cell(i, 0).Rune)
		}
	}
	for i := 2; i < 5; i++ {
		if b.Cell(i, 0).Rune != ' ' {
			t.Errorf("row %d: expected blank, got '%c'", i, b.Cell(i, 0).Rune)
		}
	}
	if h.Len() != 3 {
		t.Errorf("expected 3 history lines, got %d", h.Len())
	}
}

func TestBufferScrollDown(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		b.Cell(i, 0).Rune = rune('0' + i)
	}
	b.ScrollDown(2)
	if b.Cell(0, 0).Rune != ' ' || b.Cell(1, 0).Rune != ' ' {
		t.Error("top rows should be blank after ScrollDown")
	}
	for i := 2; i < 5; i++ {
		if b.Cell(i, 0).Rune != rune('0'+(i-2)) {
			t.Errorf("row %d: expected '%c', got '%c'", i, '0'+(i-2), b.Cell(i, 0).Rune)
		}
	}
}

func TestBufferScrollRegion(t *testing.T) {
	b := NewBuffer(10, 24)
	for i := 0; i < 24; i++ {
		b.Cell(i, 0).Rune = rune('a' + i%26)
	}
	b.SetScrollRegion(5, 20)
	b.ScrollUp(1)
	for i := 0; i < 5; i++ {
		if b.Cell(i, 0).Rune != rune('a'+i%26) {
			t.Errorf("row %d above scroll region should be unchanged", i)
		}
	}
	for i := 5; i < 20; i++ {
		if b.Cell(i, 0).Rune != rune('a'+(i+1)%26) {
			t.Errorf("row %d in scroll region: expected '%c', got '%c'", i, 'a'+(i+1)%26, b.Cell(i, 0).Rune)
		}
	}
	if b.Cell(20, 0).Rune != ' ' {
		t.Error("row 20 (scrollBot) should be blank after ScrollUp")
	}
	for i := 21; i < 24; i++ {
		if b.Cell(i, 0).Rune != rune('a'+i%26) {
			t.Errorf("row %d below scroll region should be unchanged", i)
		}
	}
}

func TestBufferSetScrollRegionInvalid(t *testing.T) {
	b := NewBuffer(10, 24)
	b.SetScrollRegion(10, 5)
	if b.scrollTop != 0 || b.scrollBot != 23 {
		t.Error("invalid scroll region should not change settings")
	}
}

func TestBufferResizeShrink(t *testing.T) {
	b := NewBuffer(80, 24)
	for i := 0; i < 24; i++ {
		b.Cell(i, 0).Rune = rune('A' + i%26)
	}
	b.Resize(40, 12)
	if b.Cols() != 40 || b.Rows() != 12 {
		t.Errorf("expected 40x12, got %dx%d", b.Cols(), b.Rows())
	}
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("existing content should be preserved after shrink")
	}
	if b.scrollTop != 0 || b.scrollBot != 11 {
		t.Error("scroll region should be reset after resize")
	}
}

func TestBufferResizeGrow(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'X'
	b.Resize(20, 10)
	if b.Cols() != 20 || b.Rows() != 10 {
		t.Errorf("expected 20x10, got %dx%d", b.Cols(), b.Rows())
	}
	if b.Cell(0, 0).Rune != 'X' {
		t.Error("existing content should be preserved after grow")
	}
	if b.Cell(5, 0).Rune != ' ' {
		t.Error("new rows should be default")
	}
}

func TestBufferClear(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'Z'
	b.Clear()
	if b.Cell(0, 0).Rune != ' ' {
		t.Error("Clear should reset all cells")
	}
}

func TestBufferClearRect(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		for j := 0; j < 10; j++ {
			b.Cell(i, j).Rune = 'X'
		}
	}
	b.ClearRect(Rect{X: 2, Y: 1, W: 3, H: 2})
	for y := 1; y < 3; y++ {
		for x := 2; x < 5; x++ {
			if b.Cell(y, x).Rune != ' ' {
				t.Errorf("cell (%d,%d) should be cleared", y, x)
			}
		}
	}
	if b.Cell(0, 0).Rune != 'X' {
		t.Error("cell outside rect should be unchanged")
	}
}

func TestBufferScrollUpZero(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'A'
	b.ScrollUp(0)
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("ScrollUp(0) should not change buffer")
	}
}

func TestBufferScrollDownZero(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'A'
	b.ScrollDown(0)
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("ScrollDown(0) should not change buffer")
	}
}

func TestScrollUpPushesToHistory(t *testing.T) {
	buf := NewBuffer(10, 5)
	for col := 0; col < 10; col++ {
		cell := buf.Cell(0, col)
		if cell != nil {
			cell.Rune = 'A'
		}
	}
	buf.ScrollUp(1)
	hist := buf.History()
	if hist.Len() != 1 {
		t.Fatalf("expected history length 1, got %d", hist.Len())
	}
	histLine := hist.Line(0)
	if histLine == nil {
		t.Fatal("expected history line not nil")
	}
	if histLine.Cell(0).Rune != 'A' {
		t.Error("expected 'A' in history")
	}
}

func TestScrollUpMultipleLines(t *testing.T) {
	buf := NewBuffer(10, 5)
	for row := 0; row < 5; row++ {
		for col := 0; col < 10; col++ {
			cell := buf.Cell(row, col)
			if cell != nil {
				cell.Rune = rune('0' + row)
			}
		}
	}
	buf.ScrollUp(3)
	hist := buf.History()
	if hist.Len() != 3 {
		t.Fatalf("expected history length 3, got %d", hist.Len())
	}
}

func TestHistoryPushTruncate(t *testing.T) {
	h := NewHistory(3)
	for i := 0; i < 5; i++ {
		line := NewLine(10)
		line.Cell(0).Rune = rune('0' + i)
		h.Push(line)
	}
	if h.Len() != 3 {
		t.Fatalf("expected len 3, got %d", h.Len())
	}
	if h.Line(0).Cell(0).Rune != '2' {
		t.Errorf("expected '2', got %c", h.Line(0).Cell(0).Rune)
	}
	if h.Line(2).Cell(0).Rune != '4' {
		t.Errorf("expected '4', got %c", h.Line(2).Cell(0).Rune)
	}
}

func TestScrollTop(t *testing.T) {
	buf := NewBuffer(10, 24)
	if buf.ScrollTop() != 0 {
		t.Errorf("expected scrollTop 0, got %d", buf.ScrollTop())
	}
	buf.SetScrollRegion(3, 20)
	if buf.ScrollTop() != 3 {
		t.Errorf("expected scrollTop 3, got %d", buf.ScrollTop())
	}
}

func TestBufferCellNilLine(t *testing.T) {
	buf := NewBuffer(10, 5)
	cell := buf.Cell(100, 100)
	if cell != nil {
		t.Error("expected nil for out-of-bounds")
	}
	cell = buf.Cell(-1, 0)
	if cell != nil {
		t.Error("expected nil for negative row")
	}
}

func TestBufferConcurrentIndependentInstances(t *testing.T) {
	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			b := NewBuffer(20, 10)
			for i := 0; i < 100; i++ {
				cell := b.Cell(i%10, i%20)
				if cell != nil {
					cell.Rune = 'x'
				}
				b.ScrollUp(1)
			}
		}()
	}
	wg.Wait()
}

func TestBufferScrollUpHistoryConsistency(t *testing.T) {
	buf := NewBuffer(5, 3)
	for r := 0; r < 3; r++ {
		for c := 0; c < 5; c++ {
			cell := buf.Cell(r, c)
			if cell != nil {
				cell.Rune = rune('a' + r*5 + c)
			}
		}
	}
	buf.ScrollUp(1)
	if buf.History().Len() != 1 {
		t.Errorf("expected history len 1, got %d", buf.History().Len())
	}
	histLine := buf.History().Line(0)
	if histLine == nil {
		t.Fatal("expected non-nil history line")
	}
	first := histLine.Cell(0)
	if first == nil || first.Rune != 'a' {
		t.Errorf("expected history line first rune 'a', got %v", first)
	}
}

func TestBufferScrollBot(t *testing.T) {
	buf := NewBuffer(10, 24)
	if buf.ScrollBot() != 23 {
		t.Errorf("expected scrollBot 23, got %d", buf.ScrollBot())
	}
	buf.SetScrollRegion(0, 20)
	if buf.ScrollBot() != 20 {
		t.Errorf("expected scrollBot 20, got %d", buf.ScrollBot())
	}
}

func TestLineFeedScrollsInRegion(t *testing.T) {
	buf := NewBuffer(10, 24)
	buf.SetScrollRegion(0, 22)
	for i := 0; i < 23; i++ {
		buf.Cell(i, 0).Rune = rune('a' + i%26)
	}
	buf.Cell(23, 0).Rune = 'S'
	buf.cursor.Row = 22
	buf.cursor.Col = 0
	buf.LineFeed()
	if buf.cursor.Row != 22 {
		t.Errorf("expected cursor row 22 (scrollBot), got %d", buf.cursor.Row)
	}
	if buf.Cell(0, 0).Rune != 'b' {
		t.Errorf("expected row 0 = 'b' after scroll, got %c", buf.Cell(0, 0).Rune)
	}
	if buf.Cell(22, 0).Rune != ' ' {
		t.Errorf("expected row 22 blank after scroll, got %c", buf.Cell(22, 0).Rune)
	}
	if buf.Cell(23, 0).Rune != 'S' {
		t.Errorf("status row 23 changed: expected 'S', got %c", buf.Cell(23, 0).Rune)
	}
}

func TestLineFeedFullScreenScroll(t *testing.T) {
	buf := NewBuffer(10, 5)
	buf.Cell(0, 0).Rune = 'A'
	buf.cursor.Row = 4
	buf.LineFeed()
	if buf.cursor.Row != 4 {
		t.Errorf("expected cursor row 4, got %d", buf.cursor.Row)
	}
	if buf.History().Len() != 1 {
		t.Errorf("expected history len 1, got %d", buf.History().Len())
	}
}

func TestLineFeedMidRegionNoScroll(t *testing.T) {
	buf := NewBuffer(10, 24)
	buf.SetScrollRegion(0, 22)
	buf.cursor.Row = 10
	buf.LineFeed()
	if buf.cursor.Row != 11 {
		t.Errorf("expected cursor row 11, got %d", buf.cursor.Row)
	}
}

func TestLineFeedBelowRegionClamps(t *testing.T) {
	buf := NewBuffer(10, 24)
	buf.SetScrollRegion(0, 22)
	buf.cursor.Row = 23
	buf.LineFeed()
	if buf.cursor.Row != 23 {
		t.Errorf("cursor below region should not move, got row %d", buf.cursor.Row)
	}
}

func TestReverseIndexScrollsInRegion(t *testing.T) {
	buf := NewBuffer(10, 24)
	buf.SetScrollRegion(5, 20)
	for i := 5; i <= 20; i++ {
		buf.Cell(i, 0).Rune = rune('a' + (i-5)%26)
	}
	buf.cursor.Row = 5
	buf.ReverseIndex()
	if buf.cursor.Row != 5 {
		t.Errorf("expected cursor row 5 (scrollTop), got %d", buf.cursor.Row)
	}
	if buf.Cell(5, 0).Rune != ' ' {
		t.Errorf("expected row 5 blank after RI scroll, got %c", buf.Cell(5, 0).Rune)
	}
	if buf.Cell(6, 0).Rune != 'a' {
		t.Errorf("expected row 6 = 'a', got %c", buf.Cell(6, 0).Rune)
	}
}

func TestReverseIndexMidRegionNoScroll(t *testing.T) {
	buf := NewBuffer(10, 24)
	buf.SetScrollRegion(5, 20)
	buf.cursor.Row = 10
	buf.ReverseIndex()
	if buf.cursor.Row != 9 {
		t.Errorf("expected cursor row 9, got %d", buf.cursor.Row)
	}
}

func TestAltScreenNoHistory(t *testing.T) {
	buf := NewBuffer(10, 5)
	buf.SetAltScreen(true)
	for i := 0; i < 5; i++ {
		buf.Cell(i, 0).Rune = rune('0' + i)
	}
	buf.ScrollUp(1)
	if buf.History().Len() != 0 {
		t.Errorf("alt screen should not push history, got len %d", buf.History().Len())
	}
}

func TestMainScreenPushesHistory(t *testing.T) {
	buf := NewBuffer(10, 5)
	buf.Cell(0, 0).Rune = 'A'
	buf.ScrollUp(1)
	if buf.History().Len() != 1 {
		t.Errorf("main screen should push history, got len %d", buf.History().Len())
	}
}

func TestClearAllClearsHistory(t *testing.T) {
	buf := NewBuffer(10, 5)
	buf.Cell(0, 0).Rune = 'A'
	buf.ScrollUp(1)
	if buf.History().Len() != 1 {
		t.Fatal("expected history len 1 before ClearAll")
	}
	buf.ClearAll()
	if buf.History().Len() != 0 {
		t.Errorf("expected history cleared, got len %d", buf.History().Len())
	}
	if buf.Cell(0, 0).Rune != ' ' {
		t.Errorf("expected cells cleared, got %c", buf.Cell(0, 0).Rune)
	}
}

func TestClearKeepsHistory(t *testing.T) {
	buf := NewBuffer(10, 5)
	buf.Cell(0, 0).Rune = 'A'
	buf.ScrollUp(1)
	buf.Clear()
	if buf.History().Len() != 1 {
		t.Errorf("Clear should keep history (ED 2 semantics), got len %d", buf.History().Len())
	}
}
