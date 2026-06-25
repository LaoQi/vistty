package internal

import "unsafe"

type PlaneResult struct {
	PlaneID       uint32
	CrtcID        uint32
	FbID          uint32
	PossibleCrtcs uint32
	GammaSize     uint32
	Formats       []uint32
}

func GetPlaneResources(fd int) ([]uint32, error) {
	var res PlaneRes
	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPLANERESOURCES, unsafe.Pointer(&res), "DRM_IOCTL_MODE_GETPLANERESOURCES"); err != nil {
		return nil, err
	}

	planeIDs := make([]uint32, res.CountPlanes)
	if res.CountPlanes > 0 {
		res.PlaneIDPtr = uint64(uintptr(unsafe.Pointer(&planeIDs[0])))
	}

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPLANERESOURCES, unsafe.Pointer(&res), "DRM_IOCTL_MODE_GETPLANERESOURCES"); err != nil {
		return nil, err
	}

	return planeIDs, nil
}

func GetPlane(fd int, planeID uint32) (*PlaneResult, error) {
	var p Plane
	p.PlaneID = planeID

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPLANE, unsafe.Pointer(&p), "DRM_IOCTL_MODE_GETPLANE"); err != nil {
		return nil, err
	}

	formats := make([]uint32, p.CountFormatTypes)
	if p.CountFormatTypes > 0 {
		p.CountFormatTypes = 0
		p.FormatTypePtr = uint64(uintptr(unsafe.Pointer(&formats[0])))
		if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPLANE, unsafe.Pointer(&p), "DRM_IOCTL_MODE_GETPLANE"); err != nil {
			return nil, err
		}
	}

	return &PlaneResult{
		PlaneID:       p.PlaneID,
		CrtcID:        p.CrtcID,
		FbID:          p.FbID,
		PossibleCrtcs: p.PossibleCrtcs,
		GammaSize:     p.GammaSize,
		Formats:       formats,
	}, nil
}
