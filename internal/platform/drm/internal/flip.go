package internal

import "unsafe"

const (
	FlipEvent = 1
	FlipAsync = 2
)

func DoPageFlip(fd int, crtcID, fbID uint32, flags uint32, userData uint64) error {
	pf := PageFlip{
		CrtcID:   crtcID,
		FbID:     fbID,
		Flags:    flags,
		UserData: userData,
	}
	return drmIoctl(fd, DRM_IOCTL_MODE_PAGE_FLIP, unsafe.Pointer(&pf), "DRM_IOCTL_MODE_PAGE_FLIP")
}
