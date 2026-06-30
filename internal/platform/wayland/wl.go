package wayland

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// 纯 Go Wayland 客户端协议层（最小子集）。
// 替代 rajveermalviya/go-wayland，仅实现 vistty 窗口后端所需协议对象。
// Wire 格式: header = object_id(u32) | opcode(u16) | size(u16)（小端，size 含 8 字节头）

// ---------- 连接核心 ----------

type conn struct {
	fd     int
	nextID uint32

	mu       sync.Mutex // 保护 fd 写入、objects、nextID
	objects  map[uint32]*wlObject
	errFunc  func(objID uint32, code uint32, msg string)

	inBuf  []byte
	inLen  int
	oobBuf []byte
	fds    []int // 待消费的 fd
}

type wlObject struct {
	id      uint32
	onEvent func(opcode uint16, payload []byte, fds []int)
}

func dial() (*conn, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return nil, fmt.Errorf("XDG_RUNTIME_DIR not set")
	}
	wd := os.Getenv("WAYLAND_DISPLAY")
	if wd == "" {
		wd = "wayland-0"
	}
	path := wd
	if wd[0] != '/' {
		path = runtimeDir + "/" + wd
	}

	// 用 unix.Socket 直接创建 fd，避免 net.DialUnix + File() 的 GC fd 回收问题
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("wayland: socket: %w", err)
	}
	sa := &unix.SockaddrUnix{Name: path}
	if err := unix.Connect(fd, sa); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("wayland: connect %s: %w", path, err)
	}

	c := &conn{
		fd:      fd,
		nextID:  2, // display=1, 客户端对象从 2 开始
		objects: make(map[uint32]*wlObject),
		inBuf:   make([]byte, 8192),
		oobBuf:  make([]byte, 512),
	}
	// display 对象（id=1）
	c.objects[1] = &wlObject{id: 1, onEvent: c.handleDisplayEvent}
	return c, nil
}

func (c *conn) setErrorHandler(f func(objID, code uint32, msg string)) {
	c.mu.Lock()
	c.errFunc = f
	c.mu.Unlock()
}

func (c *conn) newID() uint32 {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()
	return id
}

func (c *conn) addObject(id uint32, h func(opcode uint16, payload []byte, fds []int)) *wlObject {
	obj := &wlObject{id: id, onEvent: h}
	c.mu.Lock()
	c.objects[id] = obj
	c.mu.Unlock()
	return obj
}

func (c *conn) removeObject(id uint32) {
	c.mu.Lock()
	delete(c.objects, id)
	c.mu.Unlock()
}

// writeMsg 发送一条 Wayland 请求。data 为 payload（不含头），oob 为辅助 fd。
func (c *conn) writeMsg(objID uint32, opcode uint16, payload []byte, fds []int) error {
	size := 8 + len(payload)
	if size%4 != 0 {
		return fmt.Errorf("wayland: unaligned message size %d", size)
	}
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], objID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(size<<16)|uint32(opcode))
	copy(buf[8:], payload)

	var oob []byte
	if len(fds) > 0 {
		oob = unix.UnixRights(fds...)
	}
	c.mu.Lock()
	err := unix.Sendmsg(c.fd, buf, oob, nil, 0)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("wayland: sendmsg: %w", err)
	}
	return nil
}

// dispatch 读取并分发一个事件。
func (c *conn) dispatch() error {
	// 确保至少有一个完整消息
	for {
		if c.inLen >= 8 {
			size := int(binary.LittleEndian.Uint32(c.inBuf[4:8]) >> 16)
			if c.inLen >= size {
				break
			}
		}
		n, oobn, _, _, err := unix.Recvmsg(c.fd, c.inBuf[c.inLen:cap(c.inBuf)], c.oobBuf, 0)
		if err != nil {
			return fmt.Errorf("wayland: recvmsg: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("wayland: connection closed")
		}
		c.inLen += n
		c.parseFds(oobn)
	}

	size := int(binary.LittleEndian.Uint32(c.inBuf[4:8]) >> 16)
	objID := binary.LittleEndian.Uint32(c.inBuf[0:4])
	opcode := uint16(binary.LittleEndian.Uint32(c.inBuf[4:8]) & 0xffff)

	msg := make([]byte, size-8)
	copy(msg, c.inBuf[8:size])
	// 移动剩余字节
	copy(c.inBuf, c.inBuf[size:c.inLen])
	c.inLen -= size

	// 取出当前累积的 fd 交给 handler（Wayland fd 与消息 payload 对应）
	fds := c.fds
	c.fds = nil

	c.mu.Lock()
	obj := c.objects[objID]
	c.mu.Unlock()
	if obj != nil && obj.onEvent != nil {
		obj.onEvent(opcode, msg, fds)
	}
	return nil
}

func (c *conn) parseFds(oobn int) {
	if oobn == 0 {
		return
	}
	cmsgs, err := unix.ParseSocketControlMessage(c.oobBuf[:oobn])
	if err != nil {
		return
	}
	for _, cm := range cmsgs {
		fds, err := unix.ParseUnixRights(&cm)
		if err != nil {
			continue
		}
		c.fds = append(c.fds, fds...)
	}
}

func (c *conn) close() error {
	return unix.Close(c.fd)
}

// display 事件处理: error(opcode 0) / delete_id(opcode 1)
func (c *conn) handleDisplayEvent(opcode uint16, msg []byte, fds []int) {
	switch opcode {
	case 0: // error: object_id(u32), code(u32), message(string)
		if len(msg) < 12 {
			return
		}
		objID := binary.LittleEndian.Uint32(msg[0:4])
		code := binary.LittleEndian.Uint32(msg[4:8])
		strLen := int(binary.LittleEndian.Uint32(msg[8:12]))
		ml := strLen
		if ml > 0 {
			ml-- // 去掉末尾 NUL
		}
		if 12+strLen > len(msg) {
			ml = len(msg) - 12
			if ml > 0 {
				ml--
			}
		}
		m := string(msg[12 : 12+ml])
		c.mu.Lock()
		f := c.errFunc
		c.mu.Unlock()
		if f != nil {
			f(objID, code, m)
		}
	case 1: // delete_id: id(u32)
		if len(msg) >= 4 {
			c.removeObject(binary.LittleEndian.Uint32(msg[0:4]))
		}
	}
}

// roundtrip 执行一次 sync，等待 callback done。
func (c *conn) roundtrip() error {
	cb := c.sync()
	defer cb.destroy()
	for !cb.done {
		if err := c.dispatch(); err != nil {
			return err
		}
	}
	return nil
}

// ---------- wire 编码辅助 ----------

func wlString(s string) []byte {
	strLen := len(s) + 1 // 含 NUL
	padded := (strLen + 3) &^ 3
	buf := make([]byte, 4+padded)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(strLen))
	copy(buf[4:], s)
	return buf
}

func wlFixed(f uint32) float64 {
	return float64(int32(f)) / 256.0
}

func putU32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

// ---------- Display / Registry / Callback ----------

func (c *conn) sync() *wlCallback {
	id := c.newID()
	cb := &wlCallback{c: c, id: id}
	c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 { // done: callback_data(u32)
			cb.done = true
			if cb.onDone != nil {
				cb.onDone(binary.LittleEndian.Uint32(msg[0:4]))
			}
		}
	})
	// wl_display.sync: opcode 0, arg: new_id callback
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = c.writeMsg(1, 0, payload, nil) // display id=1
	return cb
}

type wlCallback struct {
	c      *conn
	id     uint32
	done   bool
	onDone func(data uint32)
}

func (cb *wlCallback) destroy() {
	// wl_callback 无 destroy 请求。done 事件后合成器自动删除该对象并发 delete_id。
	// 仅从本地对象表移除，不发送任何请求。
	cb.c.removeObject(cb.id)
}

func (c *conn) getRegistry() *wlRegistry {
	id := c.newID()
	reg := &wlRegistry{c: c, id: id}
	c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 { // global: name(u32), interface(string), version(u32)
			if len(msg) < 4 {
				return
			}
			name := binary.LittleEndian.Uint32(msg[0:4])
			iface, off := readString(msg, 4)
			if off+4 > len(msg) {
				return
			}
			version := binary.LittleEndian.Uint32(msg[off : off+4])
			if reg.onGlobal != nil {
				reg.onGlobal(name, iface, version)
			}
		}
	})
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = c.writeMsg(1, 1, payload, nil) // wl_display.get_registry: opcode 1
	return reg
}

func readString(msg []byte, off int) (string, int) {
	if off+4 > len(msg) {
		return "", off
	}
	strLen := int(binary.LittleEndian.Uint32(msg[off : off+4]))
	off += 4
	if off+strLen > len(msg) {
		return "", off
	}
	s := string(msg[off : off+strLen-1]) // 去 NUL
	padded := (strLen + 3) &^ 3
	off += padded
	return s, off
}

type wlRegistry struct {
	c        *conn
	id       uint32
	onGlobal func(name uint32, iface string, version uint32)
}

func (r *wlRegistry) bind(name uint32, iface string, version uint32) uint32 {
	id := r.c.newID()
	// wl_registry.bind: opcode 0, args: name(u32), interface(string), version(u32), new_id(u32)
	payload := make([]byte, 0, 16+((len(iface)+1+3)&^3))
	p := make([]byte, 4)
	putU32(p, name)
	payload = append(payload, p...)
	payload = append(payload, wlString(iface)...)
	p = make([]byte, 4)
	putU32(p, version)
	payload = append(payload, p...)
	p = make([]byte, 4)
	putU32(p, id)
	payload = append(payload, p...)
	_ = r.c.writeMsg(r.id, 0, payload, nil)
	return id
}

func (r *wlRegistry) destroy() {
	// wl_registry v1 无 destroy 请求，仅从本地对象表移除。
	r.c.removeObject(r.id)
}

// ---------- Compositor / Surface ----------

type wlCompositor struct{ c *conn; id uint32 }

func (c *conn) bindCompositor(reg *wlRegistry, name, version uint32) *wlCompositor {
	id := reg.bind(name, "wl_compositor", min(version, 4))
	return &wlCompositor{c: c, id: id}
}

func (comp *wlCompositor) createSurface() *wlSurface {
	id := comp.c.newID()
	s := &wlSurface{c: comp.c, id: id}
	comp.c.addObject(id, nil)
	// wl_compositor.create_surface: opcode 0, arg: new_id
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = comp.c.writeMsg(comp.id, 0, payload, nil)
	return s
}

func (comp *wlCompositor) destroy() {
	_ = comp.c.writeMsg(comp.id, 1, nil, nil) // wl_compositor.destroy: opcode 1
	comp.c.removeObject(comp.id)
}

type wlSurface struct {
	c  *conn
	id uint32
}

func (s *wlSurface) attach(buf *wlBuffer, x, y int32) {
	payload := make([]byte, 12)
	putU32(payload[0:4], buf.id)
	putU32(payload[4:8], uint32(x))
	putU32(payload[8:12], uint32(y))
	_ = s.c.writeMsg(s.id, 1, payload, nil) // attach: opcode 1
}

func (s *wlSurface) damage(x, y, w, h int32) {
	payload := make([]byte, 16)
	putU32(payload[0:4], uint32(x))
	putU32(payload[4:8], uint32(y))
	putU32(payload[8:12], uint32(w))
	putU32(payload[12:16], uint32(h))
	_ = s.c.writeMsg(s.id, 2, payload, nil) // damage: opcode 2
}

func (s *wlSurface) commit() {
	_ = s.c.writeMsg(s.id, 6, nil, nil) // commit: opcode 6
}

func (s *wlSurface) destroy() {
	_ = s.c.writeMsg(s.id, 0, nil, nil) // destroy: opcode 0
	s.c.removeObject(s.id)
}

// ---------- Shm / ShmPool / Buffer ----------

type wlShm struct {
	c        *conn
	id       uint32
	onFormat func(format uint32)
}

func (c *conn) bindShm(reg *wlRegistry, name uint32) *wlShm {
	id := reg.bind(name, "wl_shm", 1)
	shm := &wlShm{c: c, id: id}
	c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 { // format: format(u32)
			if shm.onFormat != nil {
				shm.onFormat(binary.LittleEndian.Uint32(msg[0:4]))
			}
		}
	})
	return shm
}

func (shm *wlShm) destroy() {
	// wl_shm v1 无 destroy 请求，仅从本地对象表移除。
	shm.c.removeObject(shm.id)
}

type wlShmPool struct {
	c  *conn
	id uint32
}

func (shm *wlShm) createPool(fd int, size int32) *wlShmPool {
	id := shm.c.newID()
	pool := &wlShmPool{c: shm.c, id: id}
	shm.c.addObject(id, nil)
	// wl_shm.create_pool: opcode 0, args: new_id, fd(oob), size
	payload := make([]byte, 8)
	putU32(payload[0:4], id)
	putU32(payload[4:8], uint32(size))
	_ = shm.c.writeMsg(shm.id, 0, payload, []int{fd})
	return pool
}

func (p *wlShmPool) createBuffer(offset, width, height, stride int32, format uint32) *wlBuffer {
	id := p.c.newID()
	buf := &wlBuffer{c: p.c, id: id}
	p.c.addObject(id, nil)
	// wl_shm_pool.create_buffer: opcode 0
	// 参数顺序与旧 wire.go 一致：new_id 在前
	payload := make([]byte, 24)
	putU32(payload[0:4], id)
	putU32(payload[4:8], uint32(offset))
	putU32(payload[8:12], uint32(width))
	putU32(payload[12:16], uint32(height))
	putU32(payload[16:20], uint32(stride))
	putU32(payload[20:24], format)
	_ = p.c.writeMsg(p.id, 0, payload, nil)
	return buf
}

func (p *wlShmPool) destroy() {
	_ = p.c.writeMsg(p.id, 1, nil, nil) // destroy: opcode 1
	p.c.removeObject(p.id)
}

type wlBuffer struct {
	c  *conn
	id uint32
}

func (b *wlBuffer) destroy() {
	_ = b.c.writeMsg(b.id, 0, nil, nil) // destroy: opcode 0
	b.c.removeObject(b.id)
}

// ---------- Seat / Keyboard / Pointer ----------

type wlSeat struct {
	c              *conn
	id             uint32
	onCapabilities func(caps uint32)
}

func (c *conn) bindSeat(reg *wlRegistry, name, version uint32) *wlSeat {
	id := reg.bind(name, "wl_seat", min(version, 5))
	seat := &wlSeat{c: c, id: id}
	c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && seat.onCapabilities != nil { // capabilities: u32
			if len(msg) >= 4 {
				seat.onCapabilities(binary.LittleEndian.Uint32(msg[0:4]))
			}
		}
	})
	return seat
}

func (s *wlSeat) getKeyboard() *wlKeyboard {
	id := s.c.newID()
	kb := &wlKeyboard{c: s.c, id: id}
	s.c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		switch opcode {
		case 0: // keymap: format(u32), fd, size(u32)
			if len(msg) >= 12 && len(fds) > 0 {
				format := binary.LittleEndian.Uint32(msg[0:4])
				size := binary.LittleEndian.Uint32(msg[8:12])
				if kb.onKeymap != nil {
					kb.onKeymap(format, fds[0], size)
				} else {
					unix.Close(fds[0])
				}
			} else {
				for _, fd := range fds {
					unix.Close(fd)
				}
			}
		case 3: // key: serial(u32), time(u32), key(u32), state(u32)
			if len(msg) >= 16 && kb.onKey != nil {
				kb.onKey(
					binary.LittleEndian.Uint32(msg[0:4]),
					binary.LittleEndian.Uint32(msg[4:8]),
					binary.LittleEndian.Uint32(msg[8:12]),
					binary.LittleEndian.Uint32(msg[12:16]),
				)
			}
		case 4: // modifiers: serial, depressed, latched, locked, group (5x u32)
			if len(msg) >= 20 && kb.onModifiers != nil {
				kb.onModifiers(
					binary.LittleEndian.Uint32(msg[0:4]),
					binary.LittleEndian.Uint32(msg[4:8]),
					binary.LittleEndian.Uint32(msg[8:12]),
					binary.LittleEndian.Uint32(msg[12:16]),
					binary.LittleEndian.Uint32(msg[16:20]),
				)
			}
		}
	})
	// wl_seat.get_keyboard: opcode 1, arg: new_id
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = s.c.writeMsg(s.id, 1, payload, nil)
	return kb
}

func (s *wlSeat) getPointer() *wlPointer {
	id := s.c.newID()
	ptr := &wlPointer{c: s.c, id: id}
	s.c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		switch opcode {
		case 2: // motion: time(u32), x(fixed), y(fixed)
			if len(msg) >= 12 && ptr.onMotion != nil {
				ptr.onMotion(
					binary.LittleEndian.Uint32(msg[0:4]),
					wlFixed(binary.LittleEndian.Uint32(msg[4:8])),
					wlFixed(binary.LittleEndian.Uint32(msg[8:12])),
				)
			}
		case 3: // button: serial(u32), time(u32), button(u32), state(u32)
			if len(msg) >= 16 && ptr.onButton != nil {
				ptr.onButton(
					binary.LittleEndian.Uint32(msg[0:4]),
					binary.LittleEndian.Uint32(msg[4:8]),
					binary.LittleEndian.Uint32(msg[8:12]),
					binary.LittleEndian.Uint32(msg[12:16]),
				)
			}
		}
	})
	// wl_seat.get_pointer: opcode 0, arg: new_id
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = s.c.writeMsg(s.id, 0, payload, nil)
	return ptr
}

func (s *wlSeat) release() {
	_ = s.c.writeMsg(s.id, 3, nil, nil) // release: opcode 3 (v5+)
	s.c.removeObject(s.id)
}

type wlKeyboard struct {
	c           *conn
	id          uint32
	onKeymap    func(format uint32, fd int, size uint32)
	onKey       func(serial, time, key, state uint32)
	onModifiers func(serial, depressed, latched, locked, group uint32)
}

func (k *wlKeyboard) release() {
	_ = k.c.writeMsg(k.id, 0, nil, nil) // release: opcode 0 (v3+)
	k.c.removeObject(k.id)
}

type wlPointer struct {
	c         *conn
	id        uint32
	onMotion  func(time uint32, x, y float64)
	onButton  func(serial, time, button, state uint32)
}

func (p *wlPointer) release() {
	_ = p.c.writeMsg(p.id, 1, nil, nil) // release: opcode 1 (v3+)
	p.c.removeObject(p.id)
}

// ---------- XDG Shell ----------

type wlXdgWmBase struct {
	c  *conn
	id uint32
}

func (c *conn) bindWmBase(reg *wlRegistry, name, version uint32) *wlXdgWmBase {
	id := reg.bind(name, "xdg_wm_base", version)
	wm := &wlXdgWmBase{c: c, id: id}
	c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 { // ping: serial(u32)
			serial := binary.LittleEndian.Uint32(msg[0:4])
			payload := make([]byte, 4)
			putU32(payload, serial)
			_ = c.writeMsg(id, 3, payload, nil) // pong: opcode 3
		}
	})
	return wm
}

func (wm *wlXdgWmBase) destroy() {
	_ = wm.c.writeMsg(wm.id, 0, nil, nil) // destroy: opcode 0
	wm.c.removeObject(wm.id)
}

func (wm *wlXdgWmBase) getXdgSurface(surf *wlSurface) *wlXdgSurface {
	id := wm.c.newID()
	xdg := &wlXdgSurface{c: wm.c, id: id}
	wm.c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 { // configure: serial(u32)
			serial := binary.LittleEndian.Uint32(msg[0:4])
			if xdg.onConfigure != nil {
				xdg.onConfigure(serial)
			}
		}
	})
	// xdg_wm_base.get_xdg_surface: opcode 2
	// 参数顺序与 go-wayland 原生一致：new_id 在前，surface 在后
	payload := make([]byte, 8)
	putU32(payload[0:4], id)
	putU32(payload[4:8], surf.id)
	_ = wm.c.writeMsg(wm.id, 2, payload, nil)
	return xdg
}

type wlXdgSurface struct {
	c          *conn
	id         uint32
	onConfigure func(serial uint32)
}

func (x *wlXdgSurface) getToplevel() *wlXdgToplevel {
	id := x.c.newID()
	tl := &wlXdgToplevel{c: x.c, id: id}
	x.c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		switch opcode {
		case 0: // configure: width(i32), height(i32), states(array)
			var w, h int32
			if len(msg) >= 8 {
				w = int32(binary.LittleEndian.Uint32(msg[0:4]))
				h = int32(binary.LittleEndian.Uint32(msg[4:8]))
			}
			if tl.onConfigure != nil {
				tl.onConfigure(w, h)
			}
		case 1: // close
			if tl.onClose != nil {
				tl.onClose()
			}
		}
	})
	// xdg_surface.get_toplevel: opcode 1, arg: new_id
	payload := make([]byte, 4)
	putU32(payload, id)
	_ = x.c.writeMsg(x.id, 1, payload, nil)
	return tl
}

func (x *wlXdgSurface) ackConfigure(serial uint32) {
	payload := make([]byte, 4)
	putU32(payload, serial)
	_ = x.c.writeMsg(x.id, 4, payload, nil) // ack_configure: opcode 4
}

func (x *wlXdgSurface) setWindowGeometry(x0, y0, w, h int32) {
	payload := make([]byte, 16)
	putU32(payload[0:4], uint32(x0))
	putU32(payload[4:8], uint32(y0))
	putU32(payload[8:12], uint32(w))
	putU32(payload[12:16], uint32(h))
	_ = x.c.writeMsg(x.id, 3, payload, nil) // set_window_geometry: opcode 3
}

func (x *wlXdgSurface) destroy() {
	_ = x.c.writeMsg(x.id, 0, nil, nil) // destroy: opcode 0
	x.c.removeObject(x.id)
}

type wlXdgToplevel struct {
	c          *conn
	id         uint32
	onConfigure func(w, h int32)
	onClose    func()
}

func (t *wlXdgToplevel) setTitle(title string) {
	_ = t.c.writeMsg(t.id, 2, wlString(title), nil) // set_title: opcode 2
}

func (t *wlXdgToplevel) setAppId(appID string) {
	_ = t.c.writeMsg(t.id, 3, wlString(appID), nil) // set_app_id: opcode 3
}

func (t *wlXdgToplevel) destroy() {
	_ = t.c.writeMsg(t.id, 0, nil, nil) // destroy: opcode 0
	t.c.removeObject(t.id)
}

// ---------- XDG Decoration (zxdg_decoration_manager_v1) ----------

const (
	decoModeClientSide uint32 = 1
	decoModeServerSide uint32 = 2
)

type zxdgDecorationManagerV1 struct {
	c  *conn
	id uint32
}

func (c *conn) bindDecoManager(reg *wlRegistry, name, version uint32) *zxdgDecorationManagerV1 {
	id := reg.bind(name, "zxdg_decoration_manager_v1", min(version, 2))
	return &zxdgDecorationManagerV1{c: c, id: id}
}

func (m *zxdgDecorationManagerV1) getToplevelDecoration(tl *wlXdgToplevel) *zxdgToplevelDecorationV1 {
	id := m.c.newID()
	deco := &zxdgToplevelDecorationV1{c: m.c, id: id}
	m.c.addObject(id, func(opcode uint16, msg []byte, fds []int) {
		if opcode == 0 && len(msg) >= 4 { // configure: mode(u32)
			mode := binary.LittleEndian.Uint32(msg[0:4])
			if deco.onConfigure != nil {
				deco.onConfigure(mode)
			}
		}
	})
	payload := make([]byte, 8)
	putU32(payload[0:4], id)
	putU32(payload[4:8], tl.id)
	_ = m.c.writeMsg(m.id, 1, payload, nil) // get_toplevel_decoration: opcode 1
	return deco
}

func (m *zxdgDecorationManagerV1) destroy() {
	_ = m.c.writeMsg(m.id, 0, nil, nil) // destroy: opcode 0
	m.c.removeObject(m.id)
}

type zxdgToplevelDecorationV1 struct {
	c           *conn
	id          uint32
	onConfigure func(mode uint32)
}

func (d *zxdgToplevelDecorationV1) setMode(mode uint32) {
	payload := make([]byte, 4)
	putU32(payload, mode)
	_ = d.c.writeMsg(d.id, 1, payload, nil) // set_mode: opcode 1
}

func (d *zxdgToplevelDecorationV1) unsetMode() {
	_ = d.c.writeMsg(d.id, 2, nil, nil) // unset_mode: opcode 2
}

func (d *zxdgToplevelDecorationV1) destroy() {
	_ = d.c.writeMsg(d.id, 0, nil, nil) // destroy: opcode 0
	d.c.removeObject(d.id)
}
