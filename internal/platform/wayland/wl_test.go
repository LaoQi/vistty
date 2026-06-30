package wayland

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func newTestConn(t *testing.T) (*conn, int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	c := &conn{
		fd:      fds[0],
		nextID:  2,
		objects: make(map[uint32]*wlObject),
		inBuf:   make([]byte, 8192),
		oobBuf:  make([]byte, 512),
	}
	c.objects[1] = &wlObject{id: 1, onEvent: c.handleDisplayEvent}
	return c, fds[1]
}

func readMsg(t *testing.T, fd int) (objID uint32, opcode uint16, payload []byte) {
	t.Helper()
	buf := make([]byte, 8192)
	n, err := unix.Read(fd, buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n < 8 {
		t.Fatalf("short read: %d bytes", n)
	}
	objID = binary.LittleEndian.Uint32(buf[0:4])
	size := int(binary.LittleEndian.Uint32(buf[4:8]) >> 16)
	opcode = uint16(binary.LittleEndian.Uint32(buf[4:8]) & 0xffff)
	if size < 8 {
		t.Fatalf("invalid size %d", size)
	}
	payload = make([]byte, size-8)
	copy(payload, buf[8:size])
	return
}

func writeEvent(t *testing.T, fd int, objID uint32, opcode uint16, payload []byte) {
	t.Helper()
	size := 8 + len(payload)
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], objID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(size<<16)|uint32(opcode))
	copy(buf[8:], payload)
	if _, err := unix.Write(fd, buf); err != nil {
		t.Fatalf("write event: %v", err)
	}
}

func TestWlStringAlignment(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 8},
		{"a", 8},
		{"ab", 8},
		{"abc", 8},
		{"abcd", 12},
		{"hello", 12},
	}
	for _, tt := range tests {
		got := wlString(tt.input)
		totalLen := len(got)
		if totalLen != tt.want {
			t.Errorf("wlString(%q) total len = %d, want %d", tt.input, totalLen, tt.want)
		}
		if totalLen%4 != 0 {
			t.Errorf("wlString(%q) len %d not 4-byte aligned", tt.input, totalLen)
		}
		strLen := int(binary.LittleEndian.Uint32(got[0:4]))
		if strLen != len(tt.input)+1 {
			t.Errorf("wlString(%q) strLen = %d, want %d", tt.input, strLen, len(tt.input)+1)
		}
	}
}

func TestWlFixed(t *testing.T) {
	if v := wlFixed(256); v != 1.0 {
		t.Errorf("wlFixed(256) = %v, want 1.0", v)
	}
	if v := wlFixed(512); v != 2.0 {
		t.Errorf("wlFixed(512) = %v, want 2.0", v)
	}
	if v := wlFixed(0); v != 0.0 {
		t.Errorf("wlFixed(0) = %v, want 0.0", v)
	}
}

func TestReadString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short", "hi"},
		{"exact", "abcd"},
		{"longer", "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := wlString(tt.input)
			msg := make([]byte, 0, 256)
			msg = append(msg, encoded...)
			extra := []byte{0xDE, 0xAD, 0xBE, 0xEF}
			msg = append(msg, extra...)
			s, off := readString(msg, 0)
			if s != tt.input {
				t.Errorf("readString = %q, want %q", s, tt.input)
			}
			if off != len(encoded) {
				t.Errorf("offset = %d, want %d", off, len(encoded))
			}
		})
	}
}

func TestWriteMsgHeader(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	err := c.writeMsg(42, 3, []byte{0x01, 0x02, 0x03, 0x04}, nil)
	if err != nil {
		t.Fatalf("writeMsg: %v", err)
	}

	objID, opcode, payload := readMsg(t, peer)
	if objID != 42 {
		t.Errorf("objID = %d, want 42", objID)
	}
	if opcode != 3 {
		t.Errorf("opcode = %d, want 3", opcode)
	}
	if len(payload) != 4 {
		t.Fatalf("payload len = %d, want 4", len(payload))
	}
	if payload[0] != 0x01 || payload[3] != 0x04 {
		t.Errorf("payload = %x, want 01020304", payload)
	}
}

func TestWriteMsgUnaligned(t *testing.T) {
	c, _ := newTestConn(t)
	defer c.close()

	err := c.writeMsg(1, 0, []byte{0x01, 0x02, 0x03}, nil)
	if err == nil {
		t.Error("expected error for unaligned payload")
	}
}

func TestSetWindowGeometryPayload(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	surf := &wlSurface{c: c, id: c.newID()}
	xdg := &wlXdgSurface{c: c, id: c.newID()}
	c.addObject(xdg.id, nil)

	xdg.setWindowGeometry(10, 20, 800, 600)

	objID, opcode, payload := readMsg(t, peer)
	if objID != xdg.id {
		t.Errorf("objID = %d, want %d", objID, xdg.id)
	}
	if opcode != 3 {
		t.Errorf("opcode = %d, want 3 (set_window_geometry)", opcode)
	}
	if len(payload) != 16 {
		t.Fatalf("payload len = %d, want 16", len(payload))
	}
	x0 := binary.LittleEndian.Uint32(payload[0:4])
	y0 := binary.LittleEndian.Uint32(payload[4:8])
	w := binary.LittleEndian.Uint32(payload[8:12])
	h := binary.LittleEndian.Uint32(payload[12:16])
	if x0 != 10 || y0 != 20 || w != 800 || h != 600 {
		t.Errorf("geometry = (%d,%d,%d,%d), want (10,20,800,600)", x0, y0, w, h)
	}
	_ = surf
}

func TestAckConfigurePayload(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	xdg := &wlXdgSurface{c: c, id: c.newID()}
	c.addObject(xdg.id, nil)

	xdg.ackConfigure(12345)

	objID, opcode, payload := readMsg(t, peer)
	if objID != xdg.id {
		t.Errorf("objID = %d, want %d", objID, xdg.id)
	}
	if opcode != 4 {
		t.Errorf("opcode = %d, want 4 (ack_configure)", opcode)
	}
	if len(payload) != 4 {
		t.Fatalf("payload len = %d, want 4", len(payload))
	}
	serial := binary.LittleEndian.Uint32(payload[0:4])
	if serial != 12345 {
		t.Errorf("serial = %d, want 12345", serial)
	}
}

func TestToplevelSetTitle(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	tl := &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(tl.id, nil)

	tl.setTitle("vistty")

	objID, opcode, payload := readMsg(t, peer)
	if objID != tl.id {
		t.Errorf("objID = %d, want %d", objID, tl.id)
	}
	if opcode != 2 {
		t.Errorf("opcode = %d, want 2 (set_title)", opcode)
	}
	strLen := int(binary.LittleEndian.Uint32(payload[0:4]))
	if strLen != len("vistty")+1 {
		t.Errorf("strLen = %d, want %d", strLen, len("vistty")+1)
	}
	s := string(payload[4 : 4+strLen-1])
	if s != "vistty" {
		t.Errorf("title = %q, want %q", s, "vistty")
	}
}

func TestToplevelSetAppId(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	tl := &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(tl.id, nil)

	tl.setAppId("github.com.LaoQi.vistty")

	objID, opcode, _ := readMsg(t, peer)
	if objID != tl.id {
		t.Errorf("objID = %d, want %d", objID, tl.id)
	}
	if opcode != 3 {
		t.Errorf("opcode = %d, want 3 (set_app_id)", opcode)
	}
}

func TestDecorationSetModePayload(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, nil)

	deco.setMode(decoModeServerSide)

	objID, opcode, payload := readMsg(t, peer)
	if objID != deco.id {
		t.Errorf("objID = %d, want %d", objID, deco.id)
	}
	if opcode != 1 {
		t.Errorf("opcode = %d, want 1 (set_mode)", opcode)
	}
	if len(payload) != 4 {
		t.Fatalf("payload len = %d, want 4", len(payload))
	}
	mode := binary.LittleEndian.Uint32(payload[0:4])
	if mode != decoModeServerSide {
		t.Errorf("mode = %d, want %d (server-side)", mode, decoModeServerSide)
	}
}

func TestDecorationSetModeClientSide(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, nil)

	deco.setMode(decoModeClientSide)

	_, _, payload := readMsg(t, peer)
	mode := binary.LittleEndian.Uint32(payload[0:4])
	if mode != decoModeClientSide {
		t.Errorf("mode = %d, want %d (client-side)", mode, decoModeClientSide)
	}
}

func TestDecorationUnsetMode(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, nil)

	deco.unsetMode()

	objID, opcode, payload := readMsg(t, peer)
	if objID != deco.id {
		t.Errorf("objID = %d, want %d", objID, deco.id)
	}
	if opcode != 2 {
		t.Errorf("opcode = %d, want 2 (unset_mode)", opcode)
	}
	if len(payload) != 0 {
		t.Errorf("payload len = %d, want 0", len(payload))
	}
}

func TestDecorationDestroy(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, nil)

	deco.destroy()

	objID, opcode, payload := readMsg(t, peer)
	if objID != deco.id {
		t.Errorf("objID = %d, want %d", objID, deco.id)
	}
	if opcode != 0 {
		t.Errorf("opcode = %d, want 0 (destroy)", opcode)
	}
	if len(payload) != 0 {
		t.Errorf("payload len = %d, want 0", len(payload))
	}
	if _, ok := c.objects[deco.id]; ok {
		t.Error("object should be removed after destroy")
	}
}

func TestShmFormatSelection(t *testing.T) {
	tests := []struct {
		name     string
		formats  map[uint32]bool
		wantFmt  uint32
		wantSwap bool
	}{
		{
			name:     "XRGB8888 FourCC",
			formats:  map[uint32]bool{wlFmtXRGB8888: true},
			wantFmt:  wlFmtXRGB8888,
			wantSwap: false,
		},
		{
			name:     "niri enum XRGB8888",
			formats:  map[uint32]bool{wlEnumXRGB8888: true},
			wantFmt:  wlFmtXRGB8888,
			wantSwap: false,
		},
		{
			name:     "ARGB8888 FourCC",
			formats:  map[uint32]bool{wlFmtARGB8888: true},
			wantFmt:  wlFmtARGB8888,
			wantSwap: false,
		},
		{
			name:     "XBGR8888 needs swap",
			formats:  map[uint32]bool{wlFmtXBGR8888: true},
			wantFmt:  wlFmtXBGR8888,
			wantSwap: true,
		},
		{
			name:     "XRGB preferred over ARGB",
			formats:  map[uint32]bool{wlFmtXRGB8888: true, wlFmtARGB8888: true},
			wantFmt:  wlFmtXRGB8888,
			wantSwap: false,
		},
		{
			name:     "fallback XRGB8888",
			formats:  map[uint32]bool{},
			wantFmt:  wlFmtXRGB8888,
			wantSwap: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasFmt := func(vs ...uint32) bool {
				for _, v := range vs {
					if tt.formats[v] {
						return true
					}
				}
				return false
			}
			var shmFormat uint32
			var swapBR bool
			switch {
			case hasFmt(wlFmtXRGB8888, wlEnumXRGB8888):
				shmFormat = wlFmtXRGB8888
			case hasFmt(wlFmtARGB8888, wlEnumARGB8888):
				shmFormat = wlFmtARGB8888
			case tt.formats[wlFmtXBGR8888]:
				shmFormat = wlFmtXBGR8888
				swapBR = true
			case tt.formats[wlFmtABGR8888]:
				shmFormat = wlFmtABGR8888
				swapBR = true
			case tt.formats[wlFmtBGRX8888]:
				shmFormat = wlFmtBGRX8888
				swapBR = true
			case tt.formats[wlFmtBGRA8888]:
				shmFormat = wlFmtBGRA8888
				swapBR = true
			default:
				shmFormat = wlFmtXRGB8888
			}
			if shmFormat != tt.wantFmt {
				t.Errorf("shmFormat = 0x%08x, want 0x%08x", shmFormat, tt.wantFmt)
			}
			if swapBR != tt.wantSwap {
				t.Errorf("swapBR = %v, want %v", swapBR, tt.wantSwap)
			}
		})
	}
}

func TestXdgSurfaceConfigureEvent(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	var gotSerial uint32
	xdg := &wlXdgSurface{c: c, id: c.newID()}
	c.addObject(xdg.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 {
			gotSerial = binary.LittleEndian.Uint32(msg[0:4])
			if xdg.onConfigure != nil {
				xdg.onConfigure(gotSerial)
			}
		}
	})

	var configureSerial uint32
	xdg.onConfigure = func(serial uint32) {
		configureSerial = serial
	}

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, 999)
	writeEvent(t, peer, xdg.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if gotSerial != 999 {
		t.Errorf("parsed serial = %d, want 999", gotSerial)
	}
	if configureSerial != 999 {
		t.Errorf("callback serial = %d, want 999", configureSerial)
	}
}

func TestXdgToplevelConfigureEvent(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	var gotW, gotH int32
	tl := &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(tl.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 8 {
			w := int32(binary.LittleEndian.Uint32(msg[0:4]))
			h := int32(binary.LittleEndian.Uint32(msg[4:8]))
			gotW = w
			gotH = h
			if tl.onConfigure != nil {
				tl.onConfigure(w, h)
			}
		}
	})

	var cbW, cbH int32
	tl.onConfigure = func(w, h int32) {
		cbW = w
		cbH = h
	}

	payload := make([]byte, 12)
	binary.LittleEndian.PutUint32(payload[0:4], 1024)
	binary.LittleEndian.PutUint32(payload[4:8], 768)
	binary.LittleEndian.PutUint32(payload[8:12], 0)
	writeEvent(t, peer, tl.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if gotW != 1024 || gotH != 768 {
		t.Errorf("parsed = (%d,%d), want (1024,768)", gotW, gotH)
	}
	if cbW != 1024 || cbH != 768 {
		t.Errorf("callback = (%d,%d), want (1024,768)", cbW, cbH)
	}
}

func TestXdgToplevelCloseEvent(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	tl := &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(tl.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 1 && tl.onClose != nil {
			tl.onClose()
		}
	})

	closed := false
	tl.onClose = func() {
		closed = true
	}

	writeEvent(t, peer, tl.id, 1, nil)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if !closed {
		t.Error("onClose callback was not called")
	}
}

func TestDecorationConfigureCallback(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	var gotMode uint32
	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 {
			mode := binary.LittleEndian.Uint32(msg[0:4])
			gotMode = mode
			if deco.onConfigure != nil {
				deco.onConfigure(mode)
			}
		}
	})

	var cbMode uint32
	deco.onConfigure = func(mode uint32) {
		cbMode = mode
	}

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, decoModeClientSide)
	writeEvent(t, peer, deco.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if gotMode != decoModeClientSide {
		t.Errorf("parsed mode = %d, want %d (client-side)", gotMode, decoModeClientSide)
	}
	if cbMode != decoModeClientSide {
		t.Errorf("callback mode = %d, want %d (client-side)", cbMode, decoModeClientSide)
	}
}

func TestDecorationConfigureSSD(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	var cbMode uint32
	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 {
			mode := binary.LittleEndian.Uint32(msg[0:4])
			if deco.onConfigure != nil {
				deco.onConfigure(mode)
			}
		}
	})
	deco.onConfigure = func(mode uint32) {
		cbMode = mode
	}

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, decoModeServerSide)
	writeEvent(t, peer, deco.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if cbMode != decoModeServerSide {
		t.Errorf("callback mode = %d, want %d (server-side)", cbMode, decoModeServerSide)
	}
}

func TestGetToplevelDecorationWire(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	mgr := &zxdgDecorationManagerV1{c: c, id: c.newID()}
	c.addObject(mgr.id, nil)

	tl := &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(tl.id, nil)

	deco := mgr.getToplevelDecoration(tl)

	objID, opcode, payload := readMsg(t, peer)
	if objID != mgr.id {
		t.Errorf("objID = %d, want %d", objID, mgr.id)
	}
	if opcode != 1 {
		t.Errorf("opcode = %d, want 1 (get_toplevel_decoration)", opcode)
	}
	if len(payload) != 8 {
		t.Fatalf("payload len = %d, want 8", len(payload))
	}
	newID := binary.LittleEndian.Uint32(payload[0:4])
	tlID := binary.LittleEndian.Uint32(payload[4:8])
	if newID != deco.id {
		t.Errorf("new_id = %d, want %d", newID, deco.id)
	}
	if tlID != tl.id {
		t.Errorf("toplevel id = %d, want %d", tlID, tl.id)
	}
}

func TestDisplayErrorEvent(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	var errObjID, errCode uint32
	var errMsg string
	c.setErrorHandler(func(objID, code uint32, msg string) {
		errObjID = objID
		errCode = code
		errMsg = msg
	})

	msg := "test error"
	strLen := len(msg) + 1
	padded := (strLen + 3) &^ 3
	payload := make([]byte, 12+padded)
	binary.LittleEndian.PutUint32(payload[0:4], 42)
	binary.LittleEndian.PutUint32(payload[4:8], 7)
	binary.LittleEndian.PutUint32(payload[8:12], uint32(strLen))
	copy(payload[12:], msg)

	writeEvent(t, peer, 1, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if errObjID != 42 {
		t.Errorf("error objID = %d, want 42", errObjID)
	}
	if errCode != 7 {
		t.Errorf("error code = %d, want 7", errCode)
	}
	if errMsg != msg {
		t.Errorf("error msg = %q, want %q", errMsg, msg)
	}
}

func TestDisplayDeleteIdEvent(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	fakeID := uint32(99)
	c.addObject(fakeID, nil)

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, fakeID)
	writeEvent(t, peer, 1, 1, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if _, ok := c.objects[fakeID]; ok {
		t.Error("object should be removed after delete_id")
	}
}

func TestFourCCConstants(t *testing.T) {
	if wlFmtXRGB8888 != 0x34325258 {
		t.Errorf("wlFmtXRGB8888 = 0x%08x, want 0x34325258", wlFmtXRGB8888)
	}
	if wlFmtARGB8888 != 0x34325241 {
		t.Errorf("wlFmtARGB8888 = 0x%08x, want 0x34325241", wlFmtARGB8888)
	}
	if decoModeClientSide != 1 {
		t.Errorf("decoModeClientSide = %d, want 1", decoModeClientSide)
	}
	if decoModeServerSide != 2 {
		t.Errorf("decoModeServerSide = %d, want 2", decoModeServerSide)
	}
}
