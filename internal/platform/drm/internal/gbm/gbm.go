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
	lib                    uintptr
	createDevice           func(fd int) uintptr
	deviceDestroy          func(dev uintptr)
	surfaceCreate          func(dev uintptr, w, h, format, flags uint32) uintptr
	surfaceDestroy         func(surf uintptr)
	surfaceLockFrontBuffer func(surf uintptr) uintptr
	surfaceReleaseBuffer   func(surf, bo uintptr)
	boCreate               func(dev uintptr, w, h, format, flags uint32) uintptr
	boDestroy              func(bo uintptr)
	boGetHandle            func(bo uintptr) uint32
	boGetStride            func(bo uintptr) uint32
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
		name string
		fptr any
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

	var errs []error
	for _, s := range syms {
		addr, err := purego.Dlsym(lib, s.name)
		if err != nil {
			errs = append(errs, fmt.Errorf("missing GBM symbol %s: %w", s.name, err))
			continue
		}
		purego.RegisterFunc(s.fptr, addr)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("GBM symbol resolution: %w", errors.Join(errs...))
	}

	return l, nil
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
