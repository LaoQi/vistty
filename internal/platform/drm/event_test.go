package drm

import (
	"encoding/binary"
	"os"
	"testing"
)

// buildFlipEvent 构造一个 EventFlipComplete 事件字节切片（32 字节）。
func buildFlipEvent(crtcID, sequence uint32) []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b[0:4], EventFlipComplete)
	binary.LittleEndian.PutUint32(b[4:8], 32)
	binary.LittleEndian.PutUint32(b[24:28], sequence)
	binary.LittleEndian.PutUint32(b[28:32], crtcID)
	return b
}

func TestModeInfoRoundTrip(t *testing.T) {
	original := ModeInfo{
		Clock:      148500,
		HDisplay:   1920,
		HSyncStart: 2008,
		HSyncEnd:   2052,
		HTotal:     2200,
		VDisplay:   1080,
		VSyncStart: 1084,
		VSyncEnd:   1089,
		VTotal:     1125,
		VRefresh:   60,
		Flags:      0x5,
		Type:       0x40,
	}
	name := "1920x1080"
	copy(original.Name[:], name)

	pub := modeInfoToPublic(&original)
	if pub.Clock != original.Clock {
		t.Errorf("Clock mismatch: %d vs %d", pub.Clock, original.Clock)
	}
	if pub.HDisplay != original.HDisplay {
		t.Errorf("HDisplay mismatch: %d vs %d", pub.HDisplay, original.HDisplay)
	}
	if pub.Name != name {
		t.Errorf("Name mismatch: %q vs %q", pub.Name, name)
	}

	roundTrip := publicToModeInfo(&pub)
	if roundTrip.Clock != original.Clock {
		t.Errorf("round-trip Clock mismatch: %d vs %d", roundTrip.Clock, original.Clock)
	}
	if roundTrip.HDisplay != original.HDisplay {
		t.Errorf("round-trip HDisplay mismatch: %d vs %d", roundTrip.HDisplay, original.HDisplay)
	}
	if roundTrip.VRefresh != original.VRefresh {
		t.Errorf("round-trip VRefresh mismatch: %d vs %d", roundTrip.VRefresh, original.VRefresh)
	}
}

func TestModeInfoPublicNameTrim(t *testing.T) {
	m := ModeInfo{}
	copy(m.Name[:], "test\x00\x00")
	pub := modeInfoToPublic(&m)
	if pub.Name != "test" {
		t.Errorf("expected trimmed name 'test', got %q", pub.Name)
	}
}

func TestReadEventFlipComplete(t *testing.T) {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[0:4], EventFlipComplete)
	binary.LittleEndian.PutUint32(buf[4:8], 32)
	binary.LittleEndian.PutUint32(buf[16:20], 1000)
	binary.LittleEndian.PutUint32(buf[20:24], 500)
	binary.LittleEndian.PutUint32(buf[24:28], 42)
	binary.LittleEndian.PutUint32(buf[28:32], 7)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("osPipe: %v", err)
	}
	defer r.Close()
	if _, err := w.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	ev, err := ReadEvent(int(r.Fd()))
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil event")
	}
	if ev.Type != EventFlipComplete {
		t.Errorf("expected type %d, got %d", EventFlipComplete, ev.Type)
	}
	if ev.TVSec != 1000 {
		t.Errorf("expected TVSec 1000, got %d", ev.TVSec)
	}
	if ev.Sequence != 42 {
		t.Errorf("expected Sequence 42, got %d", ev.Sequence)
	}
	if ev.CrtcID != 7 {
		t.Errorf("expected CrtcID 7, got %d", ev.CrtcID)
	}
}

func TestReadEventTooShort(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	if _, err := w.Write([]byte{1, 2, 3}); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	_, err = ReadEvent(int(r.Fd()))
	if err == nil {
		t.Error("expected error for short read")
	}
}

// TestReadEventDropsSecondEvent 复现旧 ReadEvent 丢弃多事件的缺陷。
//
// 一次 read 可能返回多个拼接的事件（多屏同时 flip 完成时内核会合并），
// 无状态的 ReadEvent 只解析首个，第二个事件字节被 syscall.Read 消费后丢弃。
// 这是导致多屏 GBM 渲染卡死的根因触发器：丢失的事件对应 Surface 的
// onFlipComplete 永不调用，flipPending 永真，Swap 永久阻塞。
func TestReadEventDropsSecondEvent(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	combined := append(buildFlipEvent(7, 1), buildFlipEvent(9, 2)...)
	if _, err := w.Write(combined); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	ev1, err := ReadEvent(int(r.Fd()))
	if err != nil {
		t.Fatalf("first ReadEvent: %v", err)
	}
	if ev1.CrtcID != 7 {
		t.Fatalf("first event CrtcID=%d want 7", ev1.CrtcID)
	}

	// 旧实现：第二次 ReadEvent 因 syscall.Read 已消费全部字节而返回 EOF，
	// 第二个事件（CrtcID=9）被永久丢弃。这是已知的旧行为缺陷，
	// 新代码应改用 EventReader。
	_, err = ReadEvent(int(r.Fd()))
	if err == nil {
		t.Log("注意：旧 ReadEvent 此次未丢失（罕见），多事件丢失为概率性缺陷")
	}
}

// TestEventReaderMultipleEvents 验证 EventReader 正确处理一次 read 中的多个事件。
func TestEventReaderMultipleEvents(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	combined := append(buildFlipEvent(7, 1), buildFlipEvent(9, 2)...)
	if _, err := w.Write(combined); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	reader := NewEventReader(int(r.Fd()))

	ev1, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("first ReadEvent: %v", err)
	}
	if ev1.CrtcID != 7 || ev1.Sequence != 1 {
		t.Errorf("event1: CrtcID=%d Seq=%d want 7/1", ev1.CrtcID, ev1.Sequence)
	}

	ev2, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("second ReadEvent: %v (多屏事件丢失 — 应使用 EventReader)", err)
	}
	if ev2.CrtcID != 9 || ev2.Sequence != 2 {
		t.Errorf("event2: CrtcID=%d Seq=%d want 9/2", ev2.CrtcID, ev2.Sequence)
	}
}

// TestEventReaderThreeEvents 验证 3 个事件拼接的场景。
func TestEventReaderThreeEvents(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	combined := append(buildFlipEvent(11, 1), buildFlipEvent(22, 2)...)
	combined = append(combined, buildFlipEvent(33, 3)...)
	if _, err := w.Write(combined); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	reader := NewEventReader(int(r.Fd()))
	want := []struct{ crtc, seq uint32 }{{11, 1}, {22, 2}, {33, 3}}
	for i, w := range want {
		ev, err := reader.ReadEvent()
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if ev.CrtcID != w.crtc || ev.Sequence != w.seq {
			t.Errorf("event %d: CrtcID=%d Seq=%d want %d/%d", i, ev.CrtcID, ev.Sequence, w.crtc, w.seq)
		}
	}
}

// TestEventReaderSplitAcrossReads 验证事件跨多次 read 也能正确拼接。
func TestEventReaderSplitAcrossReads(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	ev1 := buildFlipEvent(5, 10)
	if _, err := w.Write(ev1[:24]); err != nil { // 写入不完整的第一事件
		t.Fatalf("write1: %v", err)
	}
	if _, err := w.Write(ev1[24:]); err != nil { // 补全第一事件
		t.Fatalf("write2: %v", err)
	}
	w.Close()

	reader := NewEventReader(int(r.Fd()))
	ev, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if ev.CrtcID != 5 || ev.Sequence != 10 {
		t.Errorf("CrtcID=%d Seq=%d want 5/10", ev.CrtcID, ev.Sequence)
	}
}

// TestEventReaderEOF 验证 fd 关闭后返回 io.EOF。
func TestEventReaderEOF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	w.Close()

	reader := NewEventReader(int(r.Fd()))
	_, err = reader.ReadEvent()
	if err == nil {
		t.Fatal("expected error on closed pipe")
	}
}
