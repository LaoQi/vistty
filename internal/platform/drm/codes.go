package drm

import "unsafe"

const (
	drmIoctlBase byte = 'd'
)

var (
	DRM_IOCTL_VERSION           = IOWR(drmIoctlBase, 0x00, unsafe.Sizeof(Version{}))
	DRM_IOCTL_GET_CAP           = IOWR(drmIoctlBase, 0x0c, unsafe.Sizeof(Capability{}))
	DRM_IOCTL_SET_CLIENT_CAP    = IOW(drmIoctlBase, 0x0d, unsafe.Sizeof(Capability{}))
	DRM_IOCTL_SET_MASTER        = IO(drmIoctlBase, 0x1e)
	DRM_IOCTL_DROP_MASTER       = IO(drmIoctlBase, 0x1f)
	DRM_IOCTL_MODE_GETRESOURCES = IOWR(drmIoctlBase, 0xA0, unsafe.Sizeof(Resources{}))
	DRM_IOCTL_MODE_GETCRTC      = IOWR(drmIoctlBase, 0xA1, unsafe.Sizeof(Crtc{}))
	DRM_IOCTL_MODE_SETCRTC      = IOWR(drmIoctlBase, 0xA2, unsafe.Sizeof(Crtc{}))
	DRM_IOCTL_MODE_GETENCODER   = IOWR(drmIoctlBase, 0xA6, unsafe.Sizeof(Encoder{}))
	DRM_IOCTL_MODE_GETCONNECTOR = IOWR(drmIoctlBase, 0xA7, unsafe.Sizeof(Connector{}))
	DRM_IOCTL_MODE_GETPROPERTY  = IOWR(drmIoctlBase, 0xAA, unsafe.Sizeof(PropertyRes{}))
	DRM_IOCTL_MODE_GETPROPBLOB  = IOWR(drmIoctlBase, 0xAC, unsafe.Sizeof(GetBlobReq{}))
	DRM_IOCTL_MODE_ADDFB        = IOWR(drmIoctlBase, 0xAE, unsafe.Sizeof(FB{}))
	DRM_IOCTL_MODE_RMFB         = IOWR(drmIoctlBase, 0xAF, 4)
	DRM_IOCTL_MODE_PAGE_FLIP    = IOWR(drmIoctlBase, 0xB0, unsafe.Sizeof(PageFlip{}))
	DRM_IOCTL_MODE_CREATE_DUMB   = IOWR(drmIoctlBase, 0xB2, unsafe.Sizeof(CreateDumb{}))
	DRM_IOCTL_MODE_MAP_DUMB      = IOWR(drmIoctlBase, 0xB3, unsafe.Sizeof(MapDumb{}))
	DRM_IOCTL_MODE_DESTROY_DUMB  = IOWR(drmIoctlBase, 0xB4, unsafe.Sizeof(DestroyDumb{}))
	DRM_IOCTL_MODE_GETPLANERESOURCES = IOWR(drmIoctlBase, 0xB5, unsafe.Sizeof(PlaneRes{}))
	DRM_IOCTL_MODE_GETPLANE     = IOWR(drmIoctlBase, 0xB6, unsafe.Sizeof(Plane{}))
	DRM_IOCTL_MODE_OBJ_GETPROPERTIES = IOWR(drmIoctlBase, 0xB9, unsafe.Sizeof(ObjProperties{}))
	DRM_IOCTL_MODE_ATOMIC       = IOWR(drmIoctlBase, 0xBC, unsafe.Sizeof(AtomicReq{}))
	DRM_IOCTL_MODE_CREATEPROPBLOB  = IOWR(drmIoctlBase, 0xBD, unsafe.Sizeof(CreateBlobReq{}))
	DRM_IOCTL_MODE_DESTROYPROPBLOB = IOWR(drmIoctlBase, 0xBE, unsafe.Sizeof(DestroyBlobReq{}))
	DRM_IOCTL_WAIT_VBLANK       = IOWR(drmIoctlBase, 0x3a, 24)
)
