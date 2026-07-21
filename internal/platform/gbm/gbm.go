package gbm

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego"
)

const (
	GBM_FORMAT_XRGB8888    = 0x34325258
	GBM_FORMAT_ARGB8888    = 0x34325241
	GBM_FORMAT_XRGB2101010 = 0x30335258

	GBM_BO_USE_SCANOUT   = 1 << 0
	GBM_BO_USE_RENDERING = 1 << 1
	GBM_BO_USE_LINEAR    = 1 << 4
)

type GBMLoader struct {
	lib                     uintptr
	createDevice            func(fd int) uintptr
	deviceDestroy           func(dev uintptr)
	deviceIsFormatSupported func(dev uintptr, format, flags uint32) int32
	surfaceCreate           func(dev uintptr, w, h, format, flags uint32) uintptr
	surfaceDestroy          func(surf uintptr)
	surfaceLockFrontBuffer  func(surf uintptr) uintptr
	surfaceReleaseBuffer    func(surf, bo uintptr)
	boCreate                func(dev uintptr, w, h, format, flags uint32) uintptr
	boDestroy               func(bo uintptr)
	boGetHandle             func(bo uintptr) uint32
	boGetStride             func(bo uintptr) uint32
	boGetFormat             func(bo uintptr) uint32
	boGetModifier           func(bo uintptr) uint64
	boGetPlaneCount         func(bo uintptr) int32
	boGetHandleForPlane     func(bo uintptr, plane int32) uint32
	boGetStrideForPlane     func(bo uintptr, plane int32) uint32
	boGetFD                 func(bo uintptr) int32
}

func LoadGBM() (*GBMLoader, error) {
	lib, err := purego.Dlopen("libgbm.so.1", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		lib, err = purego.Dlopen("libgbm.so", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("load libgbm: %w", err)
		}
	}

	l := &GBMLoader{lib: lib}

	type symDef struct {
		name     string
		fptr     any
		optional bool
	}
	syms := []symDef{
		{"gbm_create_device", &l.createDevice, false},
		{"gbm_device_destroy", &l.deviceDestroy, false},
		{"gbm_device_is_format_supported", &l.deviceIsFormatSupported, true},
		{"gbm_surface_create", &l.surfaceCreate, false},
		{"gbm_surface_destroy", &l.surfaceDestroy, false},
		{"gbm_surface_lock_front_buffer", &l.surfaceLockFrontBuffer, false},
		{"gbm_surface_release_buffer", &l.surfaceReleaseBuffer, false},
		{"gbm_bo_create", &l.boCreate, false},
		{"gbm_bo_destroy", &l.boDestroy, false},
		{"gbm_bo_get_handle", &l.boGetHandle, false},
		{"gbm_bo_get_stride", &l.boGetStride, false},
		{"gbm_bo_get_format", &l.boGetFormat, true},
		{"gbm_bo_get_modifier", &l.boGetModifier, true},
		{"gbm_bo_get_plane_count", &l.boGetPlaneCount, true},
		{"gbm_bo_get_handle_for_plane", &l.boGetHandleForPlane, true},
		{"gbm_bo_get_stride_for_plane", &l.boGetStrideForPlane, true},
		{"gbm_bo_get_fd", &l.boGetFD, true},
	}

	var errs []error
	for _, s := range syms {
		addr, err := purego.Dlsym(lib, s.name)
		if err != nil {
			if !s.optional {
				errs = append(errs, fmt.Errorf("missing GBM symbol %s: %w", s.name, err))
			}
			continue
		}
		purego.RegisterFunc(s.fptr, addr)
	}
	if len(errs) > 0 {
		purego.Dlclose(lib)
		return nil, fmt.Errorf("GBM symbol resolution: %w", errors.Join(errs...))
	}

	return l, nil
}

func (l *GBMLoader) Close() error {
	if l.lib != 0 {
		purego.Dlclose(l.lib)
		l.lib = 0
	}
	return nil
}

func (l *GBMLoader) CreateDevice(fd int) uintptr {
	return l.createDevice(fd)
}

func (l *GBMLoader) DeviceDestroy(dev uintptr) {
	l.deviceDestroy(dev)
}

func (l *GBMLoader) SurfaceCreate(dev uintptr, w, h, format, flags uint32) uintptr {
	return l.surfaceCreate(dev, w, h, format, flags)
}

func (l *GBMLoader) SurfaceDestroy(surf uintptr) {
	l.surfaceDestroy(surf)
}

func (l *GBMLoader) SurfaceLockFrontBuffer(surf uintptr) uintptr {
	return l.surfaceLockFrontBuffer(surf)
}

func (l *GBMLoader) SurfaceReleaseBuffer(surf, bo uintptr) {
	l.surfaceReleaseBuffer(surf, bo)
}

func (l *GBMLoader) BOCreate(dev uintptr, w, h, format, flags uint32) uintptr {
	return l.boCreate(dev, w, h, format, flags)
}

func (l *GBMLoader) BODestroy(bo uintptr) {
	l.boDestroy(bo)
}

func (l *GBMLoader) BOGetHandle(bo uintptr) uint32 {
	return l.boGetHandle(bo)
}

func (l *GBMLoader) BOGetStride(bo uintptr) uint32 {
	return l.boGetStride(bo)
}

func (l *GBMLoader) BOGetFormat(bo uintptr) uint32 {
	if l.boGetFormat == nil {
		return 0
	}
	return l.boGetFormat(bo)
}

func (l *GBMLoader) BOGetModifier(bo uintptr) uint64 {
	if l.boGetModifier == nil {
		return 0xFFFFFFFFFFFFFFFF
	}
	return l.boGetModifier(bo)
}

func (l *GBMLoader) BOGetPlaneCount(bo uintptr) int32 {
	if l.boGetPlaneCount == nil {
		return 1
	}
	return l.boGetPlaneCount(bo)
}

func (l *GBMLoader) BOGetHandleForPlane(bo uintptr, plane int32) uint32 {
	if l.boGetHandleForPlane == nil {
		return 0
	}
	return l.boGetHandleForPlane(bo, plane)
}

func (l *GBMLoader) BOGetStrideForPlane(bo uintptr, plane int32) uint32 {
	if l.boGetStrideForPlane == nil {
		return 0
	}
	return l.boGetStrideForPlane(bo, plane)
}

func (l *GBMLoader) DeviceIsFormatSupported(dev uintptr, format, flags uint32) bool {
	if l.deviceIsFormatSupported == nil {
		return false
	}
	return l.deviceIsFormatSupported(dev, format, flags) != 0
}

func (l *GBMLoader) BOGetFD(bo uintptr) int32 {
	if l.boGetFD == nil {
		return -1
	}
	return l.boGetFD(bo)
}
