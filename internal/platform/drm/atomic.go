package drm

import (
	"runtime"
	"unsafe"
)

const (
	AtomicFlagPageFlipEvent uint32 = 0x01
	AtomicFlagPageFlipAsync uint32 = 0x02
	AtomicFlagTestOnly      uint32 = 0x0100
	AtomicFlagNonBlock      uint32 = 0x0200
	AtomicFlagAllowModeset  uint32 = 0x0400
)

type AtomicProp struct {
	ID    uint32
	Value uint64
}

type AtomicObject struct {
	ID    uint32
	Props []AtomicProp
}

func AtomicCommit(fd int, flags uint32, objects []AtomicObject, userData uint64) error {
	nObjs := len(objects)
	if nObjs == 0 {
		req := AtomicReq{
			Flags:     flags,
			CountObjs: 0,
			UserData:  userData,
		}
		return drmIoctl(fd, DRM_IOCTL_MODE_ATOMIC, unsafe.Pointer(&req), "DRM_IOCTL_MODE_ATOMIC")
	}

	totalProps := 0
	for _, obj := range objects {
		totalProps += len(obj.Props)
	}

	objIDs := make([]uint32, nObjs)
	countPropsPerObj := make([]uint32, nObjs)
	propIDs := make([]uint32, 0, totalProps)
	propValues := make([]uint64, 0, totalProps)

	for i, obj := range objects {
		objIDs[i] = obj.ID
		countPropsPerObj[i] = uint32(len(obj.Props))
		for _, p := range obj.Props {
			propIDs = append(propIDs, p.ID)
			propValues = append(propValues, p.Value)
		}
	}

	req := AtomicReq{
		Flags:         flags,
		CountObjs:     uint32(nObjs),
		ObjsPtr:       uint64(uintptr(unsafe.Pointer(&objIDs[0]))),
		CountPropsPtr: uint64(uintptr(unsafe.Pointer(&countPropsPerObj[0]))),
		UserData:      userData,
	}
	if totalProps > 0 {
		req.PropsPtr = uint64(uintptr(unsafe.Pointer(&propIDs[0])))
		req.PropValuesPtr = uint64(uintptr(unsafe.Pointer(&propValues[0])))
	}

	err := drmIoctl(fd, DRM_IOCTL_MODE_ATOMIC, unsafe.Pointer(&req), "DRM_IOCTL_MODE_ATOMIC")
	runtime.KeepAlive(objIDs)
	runtime.KeepAlive(countPropsPerObj)
	runtime.KeepAlive(propIDs)
	runtime.KeepAlive(propValues)
	return err
}

func HasAtomic(fd int) bool {
	return SetClientCap(fd, DRM_CLIENT_CAP_ATOMIC, 1) == nil
}
