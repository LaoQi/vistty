package internal

import "unsafe"

const (
	DRM_CAP_DUMB_BUFFER     uint64 = 2
	DRM_CAP_VBLANK_HIGH_CRTC uint64 = 3
	DRM_CAP_PRIME           uint64 = 5
	DRM_CAP_ASYNC_PAGE_FLIP uint64 = 7
	DRM_CAP_CURSOR_WIDTH    uint64 = 8
	DRM_CAP_CURSOR_HEIGHT   uint64 = 9
)

func GetCap(fd int, cap uint64) (uint64, error) {
	c := Capability{
		Capability: cap,
	}
	if err := drmIoctl(fd, DRM_IOCTL_GET_CAP, unsafe.Pointer(&c), "DRM_IOCTL_GET_CAP"); err != nil {
		return 0, err
	}
	return c.Value, nil
}

func HasDumbBuffer(fd int) bool {
	_, err := GetCap(fd, DRM_CAP_DUMB_BUFFER)
	return err == nil
}
