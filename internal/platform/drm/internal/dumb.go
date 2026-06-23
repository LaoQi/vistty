package internal

import "unsafe"

type DumbBufferResult struct {
	Handle uint32
	Pitch  uint32
	Size   uint64
}

func CreateDumbBuffer(fd int, width, height, bpp uint32) (*DumbBufferResult, error) {
	d := CreateDumb{
		Height: height,
		Width:  width,
		BPP:    bpp,
	}
	if err := drmIoctl(fd, DRM_IOCTL_MODE_CREATE_DUMB, unsafe.Pointer(&d), "DRM_IOCTL_MODE_CREATE_DUMB"); err != nil {
		return nil, err
	}
	return &DumbBufferResult{
		Handle: d.Handle,
		Pitch:  d.Pitch,
		Size:   d.Size,
	}, nil
}

func AddFB(fd int, width, height uint16, depth, bpp uint8, stride, handle uint32) (fbID uint32, err error) {
	f := FB{
		Width:  uint32(width),
		Height: uint32(height),
		Pitch:  stride,
		BPP:    uint32(bpp),
		Depth:  uint32(depth),
		Handle: handle,
	}
	if err := drmIoctl(fd, DRM_IOCTL_MODE_ADDFB, unsafe.Pointer(&f), "DRM_IOCTL_MODE_ADDFB"); err != nil {
		return 0, err
	}
	return f.FbID, nil
}

func RmFB(fd int, fbID uint32) error {
	return drmIoctl(fd, DRM_IOCTL_MODE_RMFB, unsafe.Pointer(&fbID), "DRM_IOCTL_MODE_RMFB")
}

func MapDumbBuffer(fd int, handle uint32) (offset uint64, err error) {
	m := MapDumb{
		Handle: handle,
	}
	if err := drmIoctl(fd, DRM_IOCTL_MODE_MAP_DUMB, unsafe.Pointer(&m), "DRM_IOCTL_MODE_MAP_DUMB"); err != nil {
		return 0, err
	}
	return m.Offset, nil
}

func DestroyDumbBuffer(fd int, handle uint32) error {
	d := DestroyDumb{
		Handle: handle,
	}
	return drmIoctl(fd, DRM_IOCTL_MODE_DESTROY_DUMB, unsafe.Pointer(&d), "DRM_IOCTL_MODE_DESTROY_DUMB")
}
