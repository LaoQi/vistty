package wayland

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/LaoQi/vistty/internal/platform"
	"golang.org/x/sys/unix"
)

func TestSurfaceDecoModeInitial(t *testing.T) {
	s := &WaylandSurface{
		decoMode: decoModeServerSide,
	}
	if mode := s.DecoMode(); mode != decoModeServerSide {
		t.Errorf("initial DecoMode = %d, want %d (server-side)", mode, decoModeServerSide)
	}
}

func TestSurfaceDecoModeNoDecoMgr(t *testing.T) {
	s := &WaylandSurface{}
	if mode := s.DecoMode(); mode != 0 {
		t.Errorf("DecoMode without decoMgr = %d, want 0", mode)
	}
}

func TestSurfaceDecoModeCallbackUpdates(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	s := &WaylandSurface{
		backend: &WaylandBackend{c: c, shmFormat: wlFmtXRGB8888},
		decoMode: decoModeServerSide,
		mu:       sync.Mutex{},
	}

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
		s.mu.Lock()
		s.decoMode = mode
		s.mu.Unlock()
	}

	if mode := s.DecoMode(); mode != decoModeServerSide {
		t.Errorf("before event: DecoMode = %d, want %d", mode, decoModeServerSide)
	}

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, decoModeClientSide)
	writeEvent(t, peer, deco.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if mode := s.DecoMode(); mode != decoModeClientSide {
		t.Errorf("after CSD event: DecoMode = %d, want %d (client-side)", mode, decoModeClientSide)
	}

	payload2 := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload2, decoModeServerSide)
	writeEvent(t, peer, deco.id, 0, payload2)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if mode := s.DecoMode(); mode != decoModeServerSide {
		t.Errorf("after SSD event: DecoMode = %d, want %d (server-side)", mode, decoModeServerSide)
	}
}

func TestSurfaceDecoModeConcurrent(t *testing.T) {
	s := &WaylandSurface{
		decoMode: decoModeServerSide,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.DecoMode()
		}()
	}
	wg.Wait()
}

func TestSurfaceResizeEvent(t *testing.T) {
	s := &WaylandSurface{
		resizeCh: make(chan platform.ResizeEvent, 4),
	}

	select {
	case <-s.ResizeEvents():
	default:
	}

	s.resizeCh <- platform.ResizeEvent{Width: 1024, Height: 768}

	select {
	case ev := <-s.ResizeEvents():
		if ev.Width != 1024 || ev.Height != 768 {
			t.Errorf("resize event = (%d,%d), want (1024,768)", ev.Width, ev.Height)
		}
	default:
		t.Error("expected resize event")
	}
}

func TestSurfaceResizeChOverflow(t *testing.T) {
	s := &WaylandSurface{
		resizeCh: make(chan platform.ResizeEvent, 2),
	}

	for i := 0; i < 5; i++ {
		select {
		case s.resizeCh <- platform.ResizeEvent{Width: i, Height: i}:
		default:
		}
	}

	count := 0
	for {
		select {
		case <-s.ResizeEvents():
			count++
		default:
			goto done
		}
	}
done:
	if count > 2 {
		t.Errorf("received %d events, expected at most 2 (channel capacity)", count)
	}
}

func TestSurfaceOutputID(t *testing.T) {
	s := &WaylandSurface{}
	if id := s.OutputID(); id != 0 {
		t.Errorf("OutputID = %d, want 0 for wayland backend", id)
	}
}

func TestSurfaceDirectRender(t *testing.T) {
	s := &WaylandSurface{}
	if !s.DirectRender() {
		t.Error("DirectRender should return true for wayland backend")
	}
}

func TestDecorationManagerDestroy(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	mgr := &zxdgDecorationManagerV1{c: c, id: c.newID()}
	c.addObject(mgr.id, nil)

	mgr.destroy()

	objID, opcode, payload := readMsg(t, peer)
	if objID != mgr.id {
		t.Errorf("objID = %d, want %d", objID, mgr.id)
	}
	if opcode != 0 {
		t.Errorf("opcode = %d, want 0 (destroy)", opcode)
	}
	if len(payload) != 0 {
		t.Errorf("payload len = %d, want 0", len(payload))
	}
	if _, ok := c.objects[mgr.id]; ok {
		t.Error("object should be removed after destroy")
	}
}

func TestDecorationConfigureNoCallbackNoPanic(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 {
			mode := binary.LittleEndian.Uint32(msg[0:4])
			if deco.onConfigure != nil {
				deco.onConfigure(mode)
			}
		}
	})

	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, decoModeClientSide)
	writeEvent(t, peer, deco.id, 0, payload)

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func TestDecorationConfigureShortPayload(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	deco := &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(deco.id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 {
			mode := binary.LittleEndian.Uint32(msg[0:4])
			if deco.onConfigure != nil {
				deco.onConfigure(mode)
			}
		}
	})

	writeEvent(t, peer, deco.id, 0, []byte{0x01, 0x00})

	if err := c.dispatch(); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

func TestSurfaceCloseOrder(t *testing.T) {
	c, peer := newTestConn(t)
	defer unix.Close(peer)
	defer c.close()

	s := &WaylandSurface{
		backend: &WaylandBackend{c: c, shmFormat: wlFmtXRGB8888},
	}

	s.wlSurface = &wlSurface{c: c, id: c.newID()}
	c.addObject(s.wlSurface.id, nil)
	s.xdgSurface = &wlXdgSurface{c: c, id: c.newID()}
	c.addObject(s.xdgSurface.id, nil)
	s.toplevel = &wlXdgToplevel{c: c, id: c.newID()}
	c.addObject(s.toplevel.id, nil)
	s.toplevelDeco = &zxdgToplevelDecorationV1{c: c, id: c.newID()}
	c.addObject(s.toplevelDeco.id, nil)

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	unix.SetNonblock(peer, true)

	type msgInfo struct {
		objID   uint32
		opcode  uint16
	}
	msgs := make([]msgInfo, 0, 4)
	buf := make([]byte, 8192)
	for {
		n, err := unix.Read(peer, buf)
		if err != nil || n == 0 {
			break
		}
		off := 0
		for off < n {
			if off+8 > n {
				break
			}
			objID := binary.LittleEndian.Uint32(buf[off : off+4])
			opcode := uint16(binary.LittleEndian.Uint32(buf[off+4 : off+8]) & 0xffff)
			size := int(binary.LittleEndian.Uint32(buf[off+4 : off+8]) >> 16)
			msgs = append(msgs, msgInfo{objID, opcode})
			off += size
		}
	}

	if len(msgs) < 4 {
		t.Fatalf("expected 4 destroy messages, got %d", len(msgs))
	}

	destroyOps := make([]uint32, 0, 4)
	for _, m := range msgs {
		if m.opcode == 0 {
			destroyOps = append(destroyOps, m.objID)
		}
	}
	if len(destroyOps) < 4 {
		t.Errorf("expected 4 destroy opcodes, got %d", len(destroyOps))
	}
}
