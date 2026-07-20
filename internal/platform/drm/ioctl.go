package drm

import (
	"syscall"
	"unsafe"
)

const (
	IoctlNone  uint8 = 0
	IoctlWrite uint8 = 1
	IoctlRead  uint8 = 2
	IoctlRW    uint8 = 3
)

func IoctlCode(dir uint8, typ byte, nr uint8, size uintptr) uint32 {
	return uint32(dir)<<30 | uint32(typ)<<8 | uint32(nr) | uint32(size)<<16
}

func IO(typ byte, nr uint8) uint32 {
	return IoctlCode(IoctlNone, typ, nr, 0)
}

func IOR(typ byte, nr uint8, size uintptr) uint32 {
	return IoctlCode(IoctlRead, typ, nr, size)
}

func IOW(typ byte, nr uint8, size uintptr) uint32 {
	return IoctlCode(IoctlWrite, typ, nr, size)
}

func IOWR(typ byte, nr uint8, size uintptr) uint32 {
	return IoctlCode(IoctlRW, typ, nr, size)
}

func ioctl(fd int, code uint32, arg unsafe.Pointer) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(code), uintptr(arg))
		if errno == 0 {
			return nil
		}
		if errno == syscall.EINTR {
			continue
		}
		return errno
	}
}

type drmError struct {
	ioctlName string
	errno     syscall.Errno
}

func (e *drmError) Error() string {
	return e.ioctlName + ": " + e.errno.Error()
}

func (e *drmError) Unwrap() error {
	return e.errno
}

func drmIoctl(fd int, code uint32, arg unsafe.Pointer, name string) error {
	err := ioctl(fd, code, arg)
	if err != nil {
		return &drmError{ioctlName: name, errno: err.(syscall.Errno)}
	}
	return nil
}
