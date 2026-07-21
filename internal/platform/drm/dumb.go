package drm

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

func AddFB2(fd int, width, height uint16, pixelFormat uint32, handle, stride uint32) (fbID uint32, err error) {
	f := FB2{
		Width:       uint32(width),
		Height:      uint32(height),
		PixelFormat: pixelFormat,
		Handles:     [4]uint32{handle, 0, 0, 0},
		Pitches:     [4]uint32{stride, 0, 0, 0},
		Offsets:     [4]uint32{0, 0, 0, 0},
	}
	if err := drmIoctl(fd, DRM_IOCTL_MODE_ADDFB2, unsafe.Pointer(&f), "DRM_IOCTL_MODE_ADDFB2"); err != nil {
		return 0, err
	}
	return f.FbID, nil
}

func AddFB2WithModifier(fd int, f *FB2) (fbID uint32, err error) {
	if err := drmIoctl(fd, DRM_IOCTL_MODE_ADDFB2, unsafe.Pointer(f), "DRM_IOCTL_MODE_ADDFB2"); err != nil {
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

func PrimeFDToHandle(fd int, primeFD int32) (uint32, error) {
	p := PrimeHandle{
		Handle: 0,
		FD:     uint32(primeFD),
	}
	if err := drmIoctl(fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, unsafe.Pointer(&p), "DRM_IOCTL_PRIME_FD_TO_HANDLE"); err != nil {
		return 0, err
	}
	return p.Handle, nil
}

func PrimeHandleToFD(fd int, handle uint32) (int32, error) {
	p := PrimeHandle{
		Handle: handle,
		FD:     0,
	}
	if err := drmIoctl(fd, DRM_IOCTL_PRIME_HANDLE_TO_FD, unsafe.Pointer(&p), "DRM_IOCTL_PRIME_HANDLE_TO_FD"); err != nil {
		return 0, err
	}
	return int32(p.FD), nil
}
