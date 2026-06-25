package gbm

import (
	"fmt"
	"unsafe"
)

const (
	EGL_NONE                   = 0x3038
	EGL_RED_SIZE               = 0x3024
	EGL_GREEN_SIZE             = 0x3023
	EGL_BLUE_SIZE              = 0x3022
	EGL_ALPHA_SIZE             = 0x3021
	EGL_SURFACE_TYPE           = 0x3033
	EGL_WINDOW_BIT             = 0x0004
	EGL_RENDERABLE_TYPE        = 0x3040
	EGL_OPENGL_ES2_BIT         = 0x0004
	EGL_OPENGL_BIT             = 0x0008
	EGL_CONTEXT_CLIENT_VERSION = 0x3098
	EGL_OPENGL_ES_API          = 0x30A0
	EGL_OPENGL_API             = 0x30A2
	EGL_CONFIG_CAVEAT          = 0x3030
	EGL_CONFIG_ID              = 0x3028
	EGL_NATIVE_VISUAL_ID       = 0x3032
	EGL_DEPTH_SIZE             = 0x3025

	EGL_PLATFORM_GBM_KHR       = 0x31D7
	EGL_PLATFORM_WAYLAND_KHR   = 0x31D8

	EGL_DEFAULT_DISPLAY uintptr = 0
	EGL_NO_DISPLAY      uintptr = 0
	EGL_NO_CONTEXT      uintptr = 0
	EGL_NO_SURFACE      uintptr = 0
	EGL_FALSE           uintptr = 0
	EGL_TRUE            uintptr = 1
)

type EGLLoader struct {
	lib                  *LibHandle
	getDisplay           uintptr
	getPlatformDisplay   uintptr
	initialize           uintptr
	terminate            uintptr
	chooseConfig         uintptr
	bindAPI              uintptr
	createContext        uintptr
	destroyContext       uintptr
	createWindowSurface  uintptr
	destroySurface       uintptr
	makeCurrent          uintptr
	swapBuffers          uintptr
	querySurface         uintptr
	getConfigAttrib      uintptr
}

func LoadEGL() (*EGLLoader, error) {
	lib, err := OpenLib("libEGL.so.1")
	if err != nil {
		lib, err = OpenLib("libEGL.so")
		if err != nil {
			return nil, fmt.Errorf("load libEGL: %w", err)
		}
	}

	l := &EGLLoader{lib: lib}

	type symDef struct {
		name    string
		ptr     *uintptr
		optional bool
	}
	syms := []symDef{
		{"eglGetDisplay", &l.getDisplay, false},
		{"eglGetPlatformDisplay", &l.getPlatformDisplay, true},
		{"eglInitialize", &l.initialize, false},
		{"eglTerminate", &l.terminate, false},
		{"eglChooseConfig", &l.chooseConfig, false},
		{"eglBindAPI", &l.bindAPI, false},
		{"eglCreateContext", &l.createContext, false},
		{"eglDestroyContext", &l.destroyContext, false},
		{"eglCreateWindowSurface", &l.createWindowSurface, false},
		{"eglDestroySurface", &l.destroySurface, false},
		{"eglMakeCurrent", &l.makeCurrent, false},
		{"eglSwapBuffers", &l.swapBuffers, false},
		{"eglQuerySurface", &l.querySurface, true},
		{"eglGetConfigAttrib", &l.getConfigAttrib, true},
	}

	var me multiError
	for _, s := range syms {
		addr, err := lib.Sym(s.name)
		if err != nil {
			if !s.optional {
				me.add(fmt.Errorf("missing EGL symbol %s: %w", s.name, err))
			}
			continue
		}
		*s.ptr = addr
	}
	if me.hasErrors() {
		return nil, fmt.Errorf("EGL symbol resolution: %w", me.asError())
	}

	return l, nil
}

func (l *EGLLoader) GetDisplay(nativeDisplay uintptr) uintptr {
	return ccall1(l.getDisplay, nativeDisplay)
}

func (l *EGLLoader) GetPlatformDisplay(platform uintptr, nativeDisplay uintptr) uintptr {
	if l.getPlatformDisplay == 0 {
		return 0
	}
	return ccall3(l.getPlatformDisplay, platform, nativeDisplay, 0)
}

func (l *EGLLoader) Initialize(display uintptr) (int32, int32, error) {
	var major, minor int32
	ret := ccall3(l.initialize, display, uintptr(unsafe.Pointer(&major)), uintptr(unsafe.Pointer(&minor)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("eglInitialize failed")
	}
	return major, minor, nil
}

func (l *EGLLoader) Terminate(display uintptr) {
	ccall1(l.terminate, display)
}

func (l *EGLLoader) ChooseConfig(display uintptr, attribs []int32) (uintptr, error) {
	var configs [1]uintptr
	var numConfig int32

	var attribsPtr unsafe.Pointer
	if len(attribs) > 0 {
		attribsPtr = unsafe.Pointer(&attribs[0])
	}

	ret := ccall5(l.chooseConfig,
		display,
		uintptr(attribsPtr),
		uintptr(unsafe.Pointer(&configs[0])),
		1,
		uintptr(unsafe.Pointer(&numConfig)))

	if ret == 0 || numConfig == 0 {
		return 0, fmt.Errorf("eglChooseConfig: no matching config")
	}
	return configs[0], nil
}

func (l *EGLLoader) BindAPI(api uintptr) error {
	ret := ccall1(l.bindAPI, api)
	if ret == 0 {
		return fmt.Errorf("eglBindAPI failed")
	}
	return nil
}

func (l *EGLLoader) CreateContext(display, config, shareContext uintptr, attribs []int32) uintptr {
	var attribsPtr unsafe.Pointer
	if len(attribs) > 0 {
		attribsPtr = unsafe.Pointer(&attribs[0])
	}
	return ccall4(l.createContext, display, config, shareContext, uintptr(attribsPtr))
}

func (l *EGLLoader) DestroyContext(display, context uintptr) {
	ccall2(l.destroyContext, display, context)
}

func (l *EGLLoader) CreateWindowSurface(display, config, nativeWindow uintptr) uintptr {
	return ccall4(l.createWindowSurface, display, config, nativeWindow, 0)
}

func (l *EGLLoader) DestroySurface(display, surface uintptr) {
	ccall2(l.destroySurface, display, surface)
}

func (l *EGLLoader) MakeCurrent(display, draw, read, context uintptr) error {
	ret := ccall4(l.makeCurrent, display, draw, read, context)
	if ret == 0 {
		return fmt.Errorf("eglMakeCurrent failed")
	}
	return nil
}

func (l *EGLLoader) SwapBuffers(display, surface uintptr) error {
	ret := ccall2(l.swapBuffers, display, surface)
	if ret == 0 {
		return fmt.Errorf("eglSwapBuffers failed")
	}
	return nil
}

func (l *EGLLoader) GetConfigAttrib(display, config uintptr, attribute int32) (int32, error) {
	var value int32
	ret := ccall4(l.getConfigAttrib, display, config, uintptr(attribute), uintptr(unsafe.Pointer(&value)))
	if ret == 0 {
		return 0, fmt.Errorf("eglGetConfigAttrib failed for attr 0x%x", attribute)
	}
	return value, nil
}

func EGLAttribList(pairs ...int32) []int32 {
	result := make([]int32, 0, len(pairs)+1)
	result = append(result, pairs...)
	result = append(result, EGL_NONE)
	return result
}
