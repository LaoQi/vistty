package drm

import (
	"fmt"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform/drm/internal/gbm"
)

type GBMDevice struct {
	fd         int
	gbmLoader  *gbm.GBMLoader
	eglLoader  *gbm.EGLLoader
	gbmDev     uintptr
	eglDisplay uintptr
	eglContext uintptr
	eglConfig  uintptr
}

func NewGBMDevice(fd int) (*GBMDevice, error) {
	if err := drminternal.SetClientCap(fd, drminternal.DRM_CLIENT_CAP_ATOMIC, 1); err != nil {
		return nil, fmt.Errorf("set DRM_CLIENT_CAP_ATOMIC: %w", err)
	}
	if err := drminternal.SetClientCap(fd, drminternal.DRM_CLIENT_CAP_UNIVERSAL_PLANES, 1); err != nil {
		return nil, fmt.Errorf("set DRM_CLIENT_CAP_UNIVERSAL_PLANES: %w", err)
	}

	gbmLoader, err := gbm.LoadGBM()
	if err != nil {
		return nil, fmt.Errorf("load GBM: %w", err)
	}

	eglLoader, err := gbm.LoadEGL()
	if err != nil {
		return nil, fmt.Errorf("load EGL: %w", err)
	}

	gbmDev := gbmLoader.CreateDevice(fd)
	if gbmDev == 0 {
		return nil, fmt.Errorf("gbm_create_device failed")
	}

	var eglDisplay uintptr
	eglDisplay = eglLoader.GetPlatformDisplay(gbm.EGL_PLATFORM_GBM_KHR, gbmDev)
	if eglDisplay == 0 || eglDisplay == gbm.EGL_NO_DISPLAY {
		eglDisplay = eglLoader.GetDisplay(gbmDev)
	}
	if eglDisplay == 0 || eglDisplay == gbm.EGL_NO_DISPLAY {
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglGetDisplay failed")
	}

	if _, _, err := eglLoader.Initialize(eglDisplay); err != nil {
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglInitialize: %w", err)
	}

	if err := eglLoader.BindAPI(gbm.EGL_OPENGL_ES_API); err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglBindAPI: %w", err)
	}

	configAttribs := gbm.EGLAttribList(
		gbm.EGL_SURFACE_TYPE, gbm.EGL_WINDOW_BIT,
		gbm.EGL_RED_SIZE, 8,
		gbm.EGL_GREEN_SIZE, 8,
		gbm.EGL_BLUE_SIZE, 8,
		gbm.EGL_ALPHA_SIZE, 0,
		gbm.EGL_RENDERABLE_TYPE, gbm.EGL_OPENGL_ES2_BIT,
	)

	eglConfig, err := eglLoader.ChooseConfig(eglDisplay, configAttribs)
	if err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglChooseConfig: %w", err)
	}

	ctxAttribs := gbm.EGLAttribList(
		gbm.EGL_CONTEXT_CLIENT_VERSION, 2,
	)
	eglContext := eglLoader.CreateContext(eglDisplay, eglConfig, gbm.EGL_NO_CONTEXT, ctxAttribs)
	if eglContext == 0 || eglContext == gbm.EGL_NO_CONTEXT {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglCreateContext failed")
	}

	return &GBMDevice{
		fd:         fd,
		gbmLoader:  gbmLoader,
		eglLoader:  eglLoader,
		gbmDev:     gbmDev,
		eglDisplay: eglDisplay,
		eglContext: eglContext,
		eglConfig:  eglConfig,
	}, nil
}

func (d *GBMDevice) CreateSurface(width, height int, crtcID, connectorID uint32, mode *drminternal.ModeInfoPublic, commitor *AtomicCommitor) (*GBMSurface, error) {
	gbmSurface := d.gbmLoader.SurfaceCreate(
		d.gbmDev,
		uint32(width), uint32(height),
		gbm.GBM_FORMAT_XRGB8888,
		gbm.GBM_BO_USE_SCANOUT|gbm.GBM_BO_USE_RENDERING,
	)
	if gbmSurface == 0 {
		return nil, fmt.Errorf("gbm_surface_create failed")
	}

	eglSurface := d.eglLoader.CreateWindowSurface(d.eglDisplay, d.eglConfig, gbmSurface)
	if eglSurface == 0 || eglSurface == gbm.EGL_NO_SURFACE {
		d.gbmLoader.SurfaceDestroy(gbmSurface)
		return nil, fmt.Errorf("eglCreateWindowSurface failed")
	}

	info, err := commitor.Register(crtcID, connectorID, width, height, mode)
	if err != nil {
		d.eglLoader.DestroySurface(d.eglDisplay, eglSurface)
		d.gbmLoader.SurfaceDestroy(gbmSurface)
		return nil, fmt.Errorf("commitor register: %w", err)
	}

	s := &GBMSurface{
		device:      d,
		commitor:    commitor,
		info:        info,
		gbmSurface:  gbmSurface,
		eglSurface:  eglSurface,
		width:       width,
		height:      height,
		crtcID:      crtcID,
		connectorID: connectorID,
		active:      true,
		flipCh:      make(chan struct{}, 1),
	}

	return s, nil
}

func (d *GBMDevice) Close() {
	if d.eglContext != 0 {
		d.eglLoader.DestroyContext(d.eglDisplay, d.eglContext)
		d.eglContext = 0
	}
	if d.eglDisplay != 0 {
		d.eglLoader.Terminate(d.eglDisplay)
		d.eglDisplay = 0
	}
	if d.gbmDev != 0 {
		d.gbmLoader.DeviceDestroy(d.gbmDev)
		d.gbmDev = 0
	}
}

func (d *GBMDevice) GBMLoader() *gbm.GBMLoader  { return d.gbmLoader }
func (d *GBMDevice) EGLLoader() *gbm.EGLLoader  { return d.eglLoader }
func (d *GBMDevice) EGLDisplay() uintptr         { return d.eglDisplay }
func (d *GBMDevice) EGLContext() uintptr          { return d.eglContext }
func (d *GBMDevice) FD() int                      { return d.fd }
