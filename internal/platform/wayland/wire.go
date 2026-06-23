package wayland

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/rajveermalviya/go-wayland/wayland/stable/xdg-shell"
)

// This file provides corrected Wayland wire format implementations.
// The go-wayland library has a bug in PutString: it writes the padded
// string length instead of the actual length (including null terminator)
// in the uint32 length field, which violates the Wayland protocol spec.

func paddedLen(l int) int {
	return (l + 3) &^ 3
}

func registryBind(ctx *client.Context, registryID, name uint32, iface string, version uint32, newID uint32) error {
	const opcode = 0
	strLen := len(iface) + 1
	paddedStr := paddedLen(strLen)
	msgSize := 8 + 4 + 4 + paddedStr + 4 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], registryID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], name)
	binary.LittleEndian.PutUint32(buf[12:16], uint32(strLen))
	copy(buf[16:], iface)
	off := 16 + paddedStr
	binary.LittleEndian.PutUint32(buf[off:off+4], version)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], newID)

	return ctx.WriteMsg(buf, nil)
}

func toplevelSetTitle(t *xdg_shell.Toplevel, title string) error {
	const opcode = 2
	strLen := len(title) + 1
	paddedStr := paddedLen(strLen)
	msgSize := 8 + 4 + paddedStr

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], t.ID())
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(strLen))
	copy(buf[12:], title)

	return t.Context().WriteMsg(buf, nil)
}

func toplevelSetAppId(t *xdg_shell.Toplevel, appID string) error {
	const opcode = 3
	strLen := len(appID) + 1
	paddedStr := paddedLen(strLen)
	msgSize := 8 + 4 + paddedStr

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], t.ID())
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(strLen))
	copy(buf[12:], appID)

	return t.Context().WriteMsg(buf, nil)
}

func shmCreatePool(ctx *client.Context, shmID uint32, fd int, size int32) (*client.ShmPool, error) {
	pool := client.NewShmPool(ctx)
	const opcode = 0
	const msgSize = 8 + 4 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], shmID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], pool.ID())
	binary.LittleEndian.PutUint32(buf[12:16], uint32(size))

	oob := unix.UnixRights(fd)
	if err := ctx.WriteMsg(buf, oob); err != nil {
		pool.Destroy()
		return nil, fmt.Errorf("write create_pool: %w", err)
	}
	return pool, nil
}

func shmPoolCreateBuffer(ctx *client.Context, poolID uint32, offset, width, height, stride int32, format uint32) (*client.Buffer, error) {
	buffer := client.NewBuffer(ctx)
	const opcode = 0
	const msgSize = 8 + 4 + 4 + 4 + 4 + 4 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], poolID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], buffer.ID())
	binary.LittleEndian.PutUint32(buf[12:16], uint32(offset))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(width))
	binary.LittleEndian.PutUint32(buf[20:24], uint32(height))
	binary.LittleEndian.PutUint32(buf[24:28], uint32(stride))
	binary.LittleEndian.PutUint32(buf[28:32], format)

	if err := ctx.WriteMsg(buf, nil); err != nil {
		buffer.Destroy()
		return nil, fmt.Errorf("write create_buffer: %w", err)
	}
	return buffer, nil
}

func compositorCreateSurface(ctx *client.Context, compositorID uint32) (*client.Surface, error) {
	surface := client.NewSurface(ctx)
	const opcode = 0
	const msgSize = 8 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], compositorID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], surface.ID())

	if err := ctx.WriteMsg(buf, nil); err != nil {
		surface.Destroy()
		return nil, err
	}
	return surface, nil
}

func xdgWmBaseGetXdgSurface(ctx *client.Context, wmBaseID uint32, wlSurfaceID uint32) (*xdg_shell.Surface, error) {
	xdgSurface := xdg_shell.NewSurface(ctx)
	const opcode = 2
	const msgSize = 8 + 4 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], wmBaseID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], xdgSurface.ID())
	binary.LittleEndian.PutUint32(buf[12:16], wlSurfaceID)

	if err := ctx.WriteMsg(buf, nil); err != nil {
		xdgSurface.Destroy()
		return nil, err
	}
	return xdgSurface, nil
}

func xdgSurfaceGetToplevel(ctx *client.Context, xdgSurfaceID uint32) (*xdg_shell.Toplevel, error) {
	toplevel := xdg_shell.NewToplevel(ctx)
	const opcode = 1
	const msgSize = 8 + 4

	buf := make([]byte, msgSize)
	binary.LittleEndian.PutUint32(buf[0:4], xdgSurfaceID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(msgSize<<16|opcode))
	binary.LittleEndian.PutUint32(buf[8:12], toplevel.ID())

	if err := ctx.WriteMsg(buf, nil); err != nil {
		toplevel.Destroy()
		return nil, err
	}
	return toplevel, nil
}
