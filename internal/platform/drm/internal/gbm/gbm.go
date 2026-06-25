package gbm

import (
	"fmt"
)

const (
	GBM_FORMAT_XRGB8888  = 0x34325258
	GBM_FORMAT_ARGB8888  = 0x34325241
	GBM_FORMAT_XRGB2101010 = 0x30335258

	GBM_BO_USE_SCANOUT   = 1 << 0
	GBM_BO_USE_RENDERING = 1 << 1
	GBM_BO_USE_LINEAR    = 1 << 4
)

type GBMLoader struct {
	lib          *LibHandle
	createDevice uintptr
	deviceDestroy uintptr
	surfaceCreate uintptr
	surfaceDestroy uintptr
	surfaceLockFrontBuffer uintptr
	surfaceReleaseBuffer uintptr
	boCreate uintptr
	boDestroy uintptr
	boGetHandle uintptr
	boGetStride uintptr
}

func LoadGBM() (*GBMLoader, error) {
	lib, err := OpenLib("libgbm.so.1")
	if err != nil {
		lib, err = OpenLib("libgbm.so")
		if err != nil {
			return nil, fmt.Errorf("load libgbm: %w", err)
		}
	}

	l := &GBMLoader{lib: lib}

	type symDef struct {
		name string
		ptr  *uintptr
	}
	syms := []symDef{
		{"gbm_create_device", &l.createDevice},
		{"gbm_device_destroy", &l.deviceDestroy},
		{"gbm_surface_create", &l.surfaceCreate},
		{"gbm_surface_destroy", &l.surfaceDestroy},
		{"gbm_surface_lock_front_buffer", &l.surfaceLockFrontBuffer},
		{"gbm_surface_release_buffer", &l.surfaceReleaseBuffer},
		{"gbm_bo_create", &l.boCreate},
		{"gbm_bo_destroy", &l.boDestroy},
		{"gbm_bo_get_handle", &l.boGetHandle},
		{"gbm_bo_get_stride", &l.boGetStride},
	}

	var me multiError
	for _, s := range syms {
		addr, err := lib.Sym(s.name)
		if err != nil {
			me.add(fmt.Errorf("missing GBM symbol %s: %w", s.name, err))
			continue
		}
		*s.ptr = addr
	}
	if me.hasErrors() {
		return nil, fmt.Errorf("GBM symbol resolution: %w", me.asError())
	}

	return l, nil
}

func (l *GBMLoader) CreateDevice(fd int) uintptr {
	return ccall1(l.createDevice, uintptr(fd))
}

func (l *GBMLoader) DeviceDestroy(dev uintptr) {
	ccall1(l.deviceDestroy, dev)
}

func (l *GBMLoader) SurfaceCreate(dev uintptr, w, h, format, flags uint32) uintptr {
	return ccall5(l.surfaceCreate, dev, uintptr(w), uintptr(h), uintptr(format), uintptr(flags))
}

func (l *GBMLoader) SurfaceDestroy(surf uintptr) {
	ccall1(l.surfaceDestroy, surf)
}

func (l *GBMLoader) SurfaceLockFrontBuffer(surf uintptr) uintptr {
	return ccall1(l.surfaceLockFrontBuffer, surf)
}

func (l *GBMLoader) SurfaceReleaseBuffer(surf, bo uintptr) {
	ccall2(l.surfaceReleaseBuffer, surf, bo)
}

func (l *GBMLoader) BOCreate(dev uintptr, w, h, format, flags uint32) uintptr {
	return ccall5(l.boCreate, dev, uintptr(w), uintptr(h), uintptr(format), uintptr(flags))
}

func (l *GBMLoader) BODestroy(bo uintptr) {
	ccall1(l.boDestroy, bo)
}

func (l *GBMLoader) BOGetHandle(bo uintptr) uint32 {
	return uint32(ccall1(l.boGetHandle, bo))
}

func (l *GBMLoader) BOGetStride(bo uintptr) uint32 {
	return uint32(ccall1(l.boGetStride, bo))
}
