package gbm

import (
	"fmt"
	"sync"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/gl"
)

type GBMDevice struct {
	fd             int
	gbmLoader      *GBMLoader
	eglLoader      *gl.EGLLoader
	glesLoader     *gl.GLESLoader
	gbmDev         uintptr
	eglDisplay     uintptr
	eglConfig      uintptr
	nativeVisualID uint32

	commitor *AtomicCommitor
	surfaces map[uint32]*GBMSurface
	mu       sync.Mutex
}

func NewGBMDevice(fd int) (*GBMDevice, error) {
	if err := drm.SetClientCap(fd, drm.DRM_CLIENT_CAP_ATOMIC, 1); err != nil {
		return nil, fmt.Errorf("set DRM_CLIENT_CAP_ATOMIC: %w", err)
	}
	if err := drm.SetClientCap(fd, drm.DRM_CLIENT_CAP_UNIVERSAL_PLANES, 1); err != nil {
		return nil, fmt.Errorf("set DRM_CLIENT_CAP_UNIVERSAL_PLANES: %w", err)
	}

	gbmLoader, err := LoadGBM()
	if err != nil {
		return nil, fmt.Errorf("load GBM: %w", err)
	}

	eglLoader, err := gl.LoadEGL()
	if err != nil {
		return nil, fmt.Errorf("load EGL: %w", err)
	}

	gbmDev := gbmLoader.CreateDevice(fd)
	if gbmDev == 0 {
		return nil, fmt.Errorf("gbm_create_device failed")
	}

	var eglDisplay uintptr
	eglDisplay = eglLoader.GetPlatformDisplay(gl.EGL_PLATFORM_GBM_KHR, gbmDev)
	if eglDisplay == 0 || eglDisplay == gl.EGL_NO_DISPLAY {
		eglDisplay = eglLoader.GetDisplay(gbmDev)
	}
	if eglDisplay == 0 || eglDisplay == gl.EGL_NO_DISPLAY {
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglGetDisplay failed")
	}

	if _, _, err := eglLoader.Initialize(eglDisplay); err != nil {
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglInitialize: %w", err)
	}

	if err := eglLoader.BindAPI(gl.EGL_OPENGL_ES_API); err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglBindAPI: %w", err)
	}

	configAttribs := gl.EGLAttribList(
		gl.EGL_SURFACE_TYPE, gl.EGL_WINDOW_BIT,
		gl.EGL_RED_SIZE, 8,
		gl.EGL_GREEN_SIZE, 8,
		gl.EGL_BLUE_SIZE, 8,
		gl.EGL_ALPHA_SIZE, 8,
		gl.EGL_RENDERABLE_TYPE, gl.EGL_OPENGL_ES2_BIT,
	)

	eglConfig, err := eglLoader.ChooseConfig(eglDisplay, configAttribs)
	if err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("eglChooseConfig: %w", err)
	}

	nativeVisual, err := eglLoader.GetConfigAttrib(eglDisplay, eglConfig, gl.EGL_NATIVE_VISUAL_ID)
	if err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("query EGL_NATIVE_VISUAL_ID: %w", err)
	}

	rSize, _ := eglLoader.GetConfigAttrib(eglDisplay, eglConfig, gl.EGL_RED_SIZE)
	gSize, _ := eglLoader.GetConfigAttrib(eglDisplay, eglConfig, gl.EGL_GREEN_SIZE)
	bSize, _ := eglLoader.GetConfigAttrib(eglDisplay, eglConfig, gl.EGL_BLUE_SIZE)
	aSize, _ := eglLoader.GetConfigAttrib(eglDisplay, eglConfig, gl.EGL_ALPHA_SIZE)
	debug.Debugf("GBM: config RGBA=%d%d%d%d nativeVisual=0x%x (%s)\n",
		rSize, gSize, bSize, aSize, uint32(nativeVisual), visualName(uint32(nativeVisual)))

	glesLoader, err := gl.LoadGLES()
	if err != nil {
		eglLoader.Terminate(eglDisplay)
		gbmLoader.DeviceDestroy(gbmDev)
		return nil, fmt.Errorf("load GLES: %w", err)
	}

	return &GBMDevice{
		fd:             fd,
		gbmLoader:      gbmLoader,
		eglLoader:      eglLoader,
		glesLoader:     glesLoader,
		gbmDev:         gbmDev,
		eglDisplay:     eglDisplay,
		eglConfig:      eglConfig,
		nativeVisualID: uint32(nativeVisual),
		commitor:       NewAtomicCommitor(fd),
		surfaces:       make(map[uint32]*GBMSurface),
	}, nil
}

func (d *GBMDevice) CreateContext() (uintptr, error) {
	ctxAttribs := gl.EGLAttribList(
		gl.EGL_CONTEXT_CLIENT_VERSION, 3,
	)
	eglContext := d.eglLoader.CreateContext(d.eglDisplay, d.eglConfig, gl.EGL_NO_CONTEXT, ctxAttribs)
	if eglContext == 0 || eglContext == gl.EGL_NO_CONTEXT {
		ctxAttribs2 := gl.EGLAttribList(
			gl.EGL_CONTEXT_CLIENT_VERSION, 2,
		)
		eglContext = d.eglLoader.CreateContext(d.eglDisplay, d.eglConfig, gl.EGL_NO_CONTEXT, ctxAttribs2)
		if eglContext == 0 || eglContext == gl.EGL_NO_CONTEXT {
			return 0, fmt.Errorf("eglCreateContext failed (tried ES3 and ES2)")
		}
		debug.Warningf("GBM: GLES 3.0 context failed, fallback to 2.0\n")
	}
	return eglContext, nil
}

func (d *GBMDevice) CreateSurface(width, height int, crtcID, connectorID uint32, mode *drm.ModeInfoPublic) (*GBMSurface, error) {
	gbmFormat := d.nativeVisualID
	if gbmFormat == 0 {
		gbmFormat = GBM_FORMAT_XRGB8888
	}

	gbmSurface := d.gbmLoader.SurfaceCreate(
		d.gbmDev,
		uint32(width), uint32(height),
		gbmFormat,
		GBM_BO_USE_SCANOUT|GBM_BO_USE_RENDERING,
	)
	if gbmSurface == 0 {
		errCode := d.eglLoader.GetError()
		return nil, fmt.Errorf("gbm_surface_create failed (eglErr=%s)", gl.EGLErrorString(errCode))
	}
	debug.Debugf("GBM: surface created %dx%d fmt=0x%x (%s)\n", width, height, gbmFormat, visualName(gbmFormat))

	eglSurface := d.eglLoader.CreateWindowSurface(d.eglDisplay, d.eglConfig, gbmSurface)
	if eglSurface == 0 || eglSurface == gl.EGL_NO_SURFACE {
		errCode := d.eglLoader.GetError()
		d.gbmLoader.SurfaceDestroy(gbmSurface)
		return nil, fmt.Errorf("eglCreateWindowSurface failed (eglErr=%s)", gl.EGLErrorString(errCode))
	}
	debug.Debugf("GBM: eglCreateWindowSurface succeeded\n")

	info, err := d.commitor.Register(crtcID, connectorID, width, height, mode)
	if err != nil {
		d.eglLoader.DestroySurface(d.eglDisplay, eglSurface)
		d.gbmLoader.SurfaceDestroy(gbmSurface)
		return nil, fmt.Errorf("commitor register: %w", err)
	}

	eglCtx, err := d.CreateContext()
	if err != nil {
		d.eglLoader.DestroySurface(d.eglDisplay, eglSurface)
		d.gbmLoader.SurfaceDestroy(gbmSurface)
		return nil, fmt.Errorf("create egl context: %w", err)
	}

	s := &GBMSurface{
		device:      d,
		commitor:    d.commitor,
		info:        info,
		gbmSurface:  gbmSurface,
		eglSurface:  eglSurface,
		eglContext:  eglCtx,
		width:       width,
		height:      height,
		crtcID:      crtcID,
		connectorID: connectorID,
		active:      true,
	}
	s.commitCond = sync.NewCond(&s.commitMu)
	s.ensureCPUBuf()

	d.mu.Lock()
	d.surfaces[crtcID] = s
	d.mu.Unlock()

	return s, nil
}

func (d *GBMDevice) Close() {
	d.mu.Lock()
	for _, s := range d.surfaces {
		s.Close()
	}
	d.surfaces = nil
	d.mu.Unlock()

	if d.commitor != nil {
		d.commitor.Close()
		d.commitor = nil
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

func (d *GBMDevice) GBMLoader() *GBMLoader      { return d.gbmLoader }
func (d *GBMDevice) EGLLoader() *gl.EGLLoader   { return d.eglLoader }
func (d *GBMDevice) GLESLoader() *gl.GLESLoader { return d.glesLoader }
func (d *GBMDevice) EGLDisplay() uintptr        { return d.eglDisplay }
func (d *GBMDevice) FD() int                    { return d.fd }

func (d *GBMDevice) CreateSurfaceForOutput(out platform.Output) (platform.Surface, error) {
	di, ok := out.(*drm.DisplayInfo)
	if !ok {
		return nil, fmt.Errorf("unsupported output type: %T", out)
	}
	mode := di.ModeInfo()
	width := int(mode.HDisplay)
	height := int(mode.VDisplay)
	return d.CreateSurface(width, height, di.CrtcID(), di.ConnectorID(), &mode)
}

func (d *GBMDevice) HandleFlipEvent(crtcID uint32) {
	d.mu.Lock()
	surf := d.surfaces[crtcID]
	d.mu.Unlock()
	if surf != nil {
		surf.onFlipComplete()
	}
}

func (d *GBMDevice) SetActive(active bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.surfaces {
		s.SetActive(active)
	}
}

func visualName(format uint32) string {
	switch format {
	case GBM_FORMAT_XRGB8888:
		return "XRGB8888"
	case GBM_FORMAT_ARGB8888:
		return "ARGB8888"
	case GBM_FORMAT_XRGB2101010:
		return "XRGB2101010"
	default:
		return fmt.Sprintf("0x%x", format)
	}
}
