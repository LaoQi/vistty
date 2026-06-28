package drm

import "unsafe"

const (
	ModeObjectCRTC      uint32 = 0xcccccccc
	ModeObjectConnector uint32 = 0xc0c0c0c0
	ModeObjectEncoder   uint32 = 0xe0e0e0e0
	ModeObjectMode      uint32 = 0xdededede
	ModeObjectProperty  uint32 = 0xb0b0b0b0
	ModeObjectFB        uint32 = 0xfbfbfbfb
	ModeObjectBlob      uint32 = 0xbbbbbbbb
	ModeObjectPlane     uint32 = 0xeeeeeeee
	ModeObjectAny       uint32 = 0
)

const (
	ModePropRange     uint32 = 1 << 1
	ModePropImmutable uint32 = 1 << 2
	ModePropEnum      uint32 = 1 << 3
	ModePropBlob      uint32 = 1 << 4
	ModePropBitmask   uint32 = 1 << 5
	ModePropAtomic    uint32 = 0x80000000
)

const PropNameLen = 32

type PropertyResult struct {
	ID     uint32
	Name   string
	Flags  uint32
	Values []uint64
}

func GetObjectProperties(fd int, objID, objType uint32) ([]uint32, []uint64, error) {
	var op ObjProperties
	op.ObjID = objID
	op.ObjType = objType

	if err := drmIoctl(fd, DRM_IOCTL_MODE_OBJ_GETPROPERTIES, unsafe.Pointer(&op), "DRM_IOCTL_MODE_OBJ_GETPROPERTIES"); err != nil {
		return nil, nil, err
	}

	propCount := op.CountProps
	propIDs := make([]uint32, propCount)
	propValues := make([]uint64, propCount)

	if propCount > 0 {
		op.PropsPtr = uint64(uintptr(unsafe.Pointer(&propIDs[0])))
		op.PropValuesPtr = uint64(uintptr(unsafe.Pointer(&propValues[0])))
	}

	if err := drmIoctl(fd, DRM_IOCTL_MODE_OBJ_GETPROPERTIES, unsafe.Pointer(&op), "DRM_IOCTL_MODE_OBJ_GETPROPERTIES"); err != nil {
		return nil, nil, err
	}

	propIDs = propIDs[:op.CountProps]
	propValues = propValues[:op.CountProps]
	return propIDs, propValues, nil
}

func GetProperty(fd int, propID uint32) (*PropertyResult, error) {
	var p PropertyRes
	p.PropID = propID

	var values [64]uint64
	p.ValuesPtr = uint64(uintptr(unsafe.Pointer(&values[0])))
	p.CountValues = uint32(len(values))

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPROPERTY, unsafe.Pointer(&p), "DRM_IOCTL_MODE_GETPROPERTY"); err != nil {
		return nil, err
	}

	actualValues := uint32(p.CountValues)
	if actualValues > uint32(len(values)) {
		actualValues = uint32(len(values))
	}

	name := p.Name[:]
	end := 0
	for end < len(name) && name[end] != 0 {
		end++
	}

	return &PropertyResult{
		ID:     p.PropID,
		Name:   string(name[:end]),
		Flags:  p.Flags,
		Values: values[:actualValues],
	}, nil
}

func CreateBlob(fd int, data []byte) (uint32, error) {
	var blob CreateBlobReq
	blob.Length = uint32(len(data))
	if len(data) > 0 {
		blob.Data = uint64(uintptr(unsafe.Pointer(&data[0])))
	}
	if err := drmIoctl(fd, DRM_IOCTL_MODE_CREATEPROPBLOB, unsafe.Pointer(&blob), "DRM_IOCTL_MODE_CREATEPROPBLOB"); err != nil {
		return 0, err
	}
	return blob.BlobID, nil
}

func DestroyBlob(fd int, blobID uint32) error {
	d := DestroyBlobReq{BlobID: blobID}
	return drmIoctl(fd, DRM_IOCTL_MODE_DESTROYPROPBLOB, unsafe.Pointer(&d), "DRM_IOCTL_MODE_DESTROYPROPBLOB")
}

func GetBlob(fd int, blobID uint32) ([]byte, error) {
	var gb GetBlobReq
	gb.BlobID = blobID

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPROPBLOB, unsafe.Pointer(&gb), "DRM_IOCTL_MODE_GETPROPBLOB"); err != nil {
		return nil, err
	}

	data := make([]byte, gb.Length)
	if gb.Length > 0 {
		gb.Length = 0
		gb.Data = uint64(uintptr(unsafe.Pointer(&data[0])))
		if err := drmIoctl(fd, DRM_IOCTL_MODE_GETPROPBLOB, unsafe.Pointer(&gb), "DRM_IOCTL_MODE_GETPROPBLOB"); err != nil {
			return nil, err
		}
	}

	return data, nil
}
