package drm

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func OpenCard(idx int) (*os.File, error) {
	return os.OpenFile(fmt.Sprintf("/dev/dri/card%d", idx), os.O_RDWR|syscall.O_CLOEXEC, 0)
}

func OpenRender(idx int) (*os.File, error) {
	return os.OpenFile(fmt.Sprintf("/dev/dri/renderD%d", 128+idx), os.O_RDWR|syscall.O_CLOEXEC, 0)
}

func GetVersion(fd int) (name, desc, date string, major, minor, patch int, err error) {
	const bufLen = 256
	nameBuf := make([]byte, bufLen)
	descBuf := make([]byte, bufLen)
	dateBuf := make([]byte, bufLen)

	v := Version{
		VersionMajor: 0,
		VersionMinor: 0,
		VersionPatch: 0,
		NameLen:      bufLen,
		NamePtr:      uint64(uintptr(unsafe.Pointer(&nameBuf[0]))),
		DateLen:      bufLen,
		DatePtr:      uint64(uintptr(unsafe.Pointer(&dateBuf[0]))),
		DescLen:      bufLen,
		DescPtr:      uint64(uintptr(unsafe.Pointer(&descBuf[0]))),
	}

	if err := drmIoctl(fd, DRM_IOCTL_VERSION, unsafe.Pointer(&v), "DRM_IOCTL_VERSION"); err != nil {
		return "", "", "", 0, 0, 0, err
	}

	name = string(nameBuf[:v.NameLen])
	desc = string(descBuf[:v.DescLen])
	date = string(dateBuf[:v.DateLen])
	major = int(v.VersionMajor)
	minor = int(v.VersionMinor)
	patch = int(v.VersionPatch)
	return
}

func ListDevices() []string {
	matches, _ := filepath.Glob("/dev/dri/card*")
	return matches
}
