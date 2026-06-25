package gbm

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
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

	EGL_PLATFORM_GBM_KHR     = 0x31D7
	EGL_PLATFORM_WAYLAND_KHR = 0x31D8

	EGL_DEFAULT_DISPLAY uintptr = 0
	EGL_NO_DISPLAY      uintptr = 0
	EGL_NO_CONTEXT      uintptr = 0
	EGL_NO_SURFACE      uintptr = 0
	EGL_FALSE           uintptr = 0
	EGL_TRUE            uintptr = 1
)

type EGLLoader struct {
	lib                 uintptr
	getDisplay          func(nativeDisplay uintptr) uintptr
	getPlatformDisplay  func(platform, nativeDisplay, attribs uintptr) uintptr
	initialize          func(display uintptr, major, minor unsafe.Pointer) uintptr
	terminate           func(display uintptr)
	chooseConfig        func(display uintptr, attribs, configs unsafe.Pointer, configSize uintptr, numConfig unsafe.Pointer) uintptr
	bindAPI             func(api uintptr) uintptr
	createContext       func(display, config, shareContext uintptr, attribs unsafe.Pointer) uintptr
	destroyContext      func(display, context uintptr)
	createWindowSurface func(display, config, nativeWindow, attribs uintptr) uintptr
	destroySurface      func(display, surface uintptr)
	makeCurrent         func(display, draw, read, context uintptr) uintptr
	swapBuffers         func(display, surface uintptr) uintptr
	querySurface        func(display, surface uintptr, attribute uintptr, value unsafe.Pointer) uintptr
	getConfigAttrib     func(display, config uintptr, attribute uintptr, value unsafe.Pointer) uintptr
}

func LoadEGL() (*EGLLoader, error) {
	lib, err := purego.Dlopen("libEGL.so.1", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		lib, err = purego.Dlopen("libEGL.so", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("load libEGL: %w", err)
		}
	}

	l := &EGLLoader{lib: lib}

	type symDef struct {
		name     string
		fptr     any
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

	var errs []error
	for _, s := range syms {
		addr, err := purego.Dlsym(lib, s.name)
		if err != nil {
			if !s.optional {
				errs = append(errs, fmt.Errorf("missing EGL symbol %s: %w", s.name, err))
			}
			continue
		}
		purego.RegisterFunc(s.fptr, addr)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("EGL symbol resolution: %w", errors.Join(errs...))
	}

	return l, nil
}

func (l *EGLLoader) GetDisplay(nativeDisplay uintptr) uintptr {
	return l.getDisplay(nativeDisplay)
}

func (l *EGLLoader) GetPlatformDisplay(platform uintptr, nativeDisplay uintptr) uintptr {
	if l.getPlatformDisplay == nil {
		return 0
	}
	return l.getPlatformDisplay(platform, nativeDisplay, 0)
}

func (l *EGLLoader) Initialize(display uintptr) (int32, int32, error) {
	var major, minor int32
	ret := l.initialize(display, unsafe.Pointer(&major), unsafe.Pointer(&minor))
	if ret == 0 {
		return 0, 0, fmt.Errorf("eglInitialize failed")
	}
	return major, minor, nil
}

func (l *EGLLoader) Terminate(display uintptr) {
	l.terminate(display)
}

func (l *EGLLoader) ChooseConfig(display uintptr, attribs []int32) (uintptr, error) {
	var configs [1]uintptr
	var numConfig int32

	var attribsPtr unsafe.Pointer
	if len(attribs) > 0 {
		attribsPtr = unsafe.Pointer(&attribs[0])
	}

	ret := l.chooseConfig(
		display,
		attribsPtr,
		unsafe.Pointer(&configs[0]),
		1,
		unsafe.Pointer(&numConfig))

	if ret == 0 || numConfig == 0 {
		return 0, fmt.Errorf("eglChooseConfig: no matching config")
	}
	return configs[0], nil
}

func (l *EGLLoader) BindAPI(api uintptr) error {
	ret := l.bindAPI(api)
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
	return l.createContext(display, config, shareContext, attribsPtr)
}

func (l *EGLLoader) DestroyContext(display, context uintptr) {
	l.destroyContext(display, context)
}

func (l *EGLLoader) CreateWindowSurface(display, config, nativeWindow uintptr) uintptr {
	return l.createWindowSurface(display, config, nativeWindow, 0)
}

func (l *EGLLoader) DestroySurface(display, surface uintptr) {
	l.destroySurface(display, surface)
}

func (l *EGLLoader) MakeCurrent(display, draw, read, context uintptr) error {
	ret := l.makeCurrent(display, draw, read, context)
	if ret == 0 {
		return fmt.Errorf("eglMakeCurrent failed")
	}
	return nil
}

func (l *EGLLoader) SwapBuffers(display, surface uintptr) error {
	ret := l.swapBuffers(display, surface)
	if ret == 0 {
		return fmt.Errorf("eglSwapBuffers failed")
	}
	return nil
}

func (l *EGLLoader) GetConfigAttrib(display, config uintptr, attribute int32) (int32, error) {
	if l.getConfigAttrib == nil {
		return 0, fmt.Errorf("eglGetConfigAttrib not available")
	}
	var value int32
	ret := l.getConfigAttrib(display, config, uintptr(attribute), unsafe.Pointer(&value))
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
