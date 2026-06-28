package drm

import "unsafe"

func SetMaster(fd int) error {
	return drmIoctl(fd, DRM_IOCTL_SET_MASTER, unsafe.Pointer(nil), "DRM_IOCTL_SET_MASTER")
}

func DropMaster(fd int) error {
	return drmIoctl(fd, DRM_IOCTL_DROP_MASTER, unsafe.Pointer(nil), "DRM_IOCTL_DROP_MASTER")
}
