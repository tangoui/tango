//go:build android

package gl

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

// EGL and OpenGL ES 3.0 on Android, through purego (which dlopens through
// cgo on Android: the package needs CGO_ENABLED=1 there, as every Android
// build of a Tango app has). libEGL.so and libGLESv3.so are public NDK
// libraries on every Android version the host supports (API 24+).

// EGL enums (egl.h, eglext.h).
const (
	eglNone                 = 0x3038
	eglSuccess              = 0x3000
	eglBadAlloc             = 0x3003
	eglBadContext           = 0x3006
	eglBadCurrentSurf       = 0x3007
	eglBadNativeWindow      = 0x300B
	eglBadSurface           = 0x300D
	eglContextLost          = 0x300E
	eglExtensions           = 0x3055
	eglVendor               = 0x3053
	eglVersion              = 0x3054
	eglRenderableType       = 0x3040
	eglOpenGLES3Bit         = 0x0040
	eglSurfaceType          = 0x3033
	eglPbufferBit           = 0x0001
	eglWindowBit            = 0x0004
	eglRedSize              = 0x3024
	eglGreenSize            = 0x3023
	eglBlueSize             = 0x3022
	eglAlphaSize            = 0x3021
	eglDepthSize            = 0x3025
	eglStencilSize          = 0x3026
	eglNativeVisualID       = 0x302E
	eglWidth                = 0x3057
	eglHeight               = 0x3056
	eglDraw                 = 0x3059
	eglRead                 = 0x305A
	eglOpenGLESAPI          = 0x30A0
	eglContextClientVersion = 0x3098

	eglDefaultDisplay uintptr = 0
)

// AHardwareBuffer / ANativeWindow formats a config's EGL_NATIVE_VISUAL_ID
// names.
const (
	windowFormatRGBA8888 = 1
	windowFormatRGBX8888 = 2
	windowFormatRGB888   = 3
)

// eglFuncs are the EGL entry points the Android device and presenter
// call. EGL handles (EGLDisplay, EGLConfig, EGLContext, EGLSurface,
// EGLNativeWindowType) are pointers: uintptr. EGLBoolean is uint32,
// EGLint int32.
type eglFuncs struct {
	GetProcAddress       func(name string) uintptr
	QueryString          func(dpy uintptr, name int32) string
	GetDisplay           func(native uintptr) uintptr
	Initialize           func(dpy uintptr, major, minor *int32) uint32
	BindAPI              func(api uint32) uint32
	ChooseConfig         func(dpy uintptr, attribs *int32, configs *uintptr, size int32, num *int32) uint32
	GetConfigAttrib      func(dpy, config uintptr, attr int32, value *int32) uint32
	CreateContext        func(dpy, config, share uintptr, attribs *int32) uintptr
	CreatePbufferSurface func(dpy, config uintptr, attribs *int32) uintptr
	CreateWindowSurface  func(dpy, config, win uintptr, attribs *int32) uintptr
	QuerySurface         func(dpy, surface uintptr, attr int32, value *int32) uint32
	SwapBuffers          func(dpy, surface uintptr) uint32
	SwapInterval         func(dpy uintptr, interval int32) uint32
	MakeCurrent          func(dpy, draw, read, ctx uintptr) uint32
	GetCurrentContext    func() uintptr
	GetCurrentSurface    func(readdraw int32) uintptr
	DestroyContext       func(dpy, ctx uintptr) uint32
	DestroySurface       func(dpy, surface uintptr) uint32
	GetError             func() int32
	ReleaseThread        func() uint32
}

// androidEGL binds libEGL.so once per process (the loader's handles and
// function pointers, not GL state). Errors wrap ErrNoDevice.
var androidEGL = sync.OnceValues(func() (*eglFuncs, error) {
	lib, err := purego.Dlopen("libEGL.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("gl: %w: libEGL.so: %v", ErrNoDevice, err)
	}
	e := new(eglFuncs)
	if err := bindLib(lib, "libEGL.so", map[string]any{
		"eglGetProcAddress":       &e.GetProcAddress,
		"eglQueryString":          &e.QueryString,
		"eglGetDisplay":           &e.GetDisplay,
		"eglInitialize":           &e.Initialize,
		"eglBindAPI":              &e.BindAPI,
		"eglChooseConfig":         &e.ChooseConfig,
		"eglGetConfigAttrib":      &e.GetConfigAttrib,
		"eglCreateContext":        &e.CreateContext,
		"eglCreatePbufferSurface": &e.CreatePbufferSurface,
		"eglCreateWindowSurface":  &e.CreateWindowSurface,
		"eglQuerySurface":         &e.QuerySurface,
		"eglSwapBuffers":          &e.SwapBuffers,
		"eglSwapInterval":         &e.SwapInterval,
		"eglMakeCurrent":          &e.MakeCurrent,
		"eglGetCurrentContext":    &e.GetCurrentContext,
		"eglGetCurrentSurface":    &e.GetCurrentSurface,
		"eglDestroyContext":       &e.DestroyContext,
		"eglDestroySurface":       &e.DestroySurface,
		"eglGetError":             &e.GetError,
		"eglReleaseThread":        &e.ReleaseThread,
	}); err != nil {
		return nil, fmt.Errorf("%w (%w)", err, ErrNoDevice)
	}
	return e, nil
})

// eglDisplay is the default display, initialized once and never
// terminated (eglTerminate would end every context of the process on it,
// the WebView's included).
var eglDisplay = sync.OnceValues(func() (uintptr, error) {
	e, err := androidEGL()
	if err != nil {
		return 0, err
	}
	dpy := e.GetDisplay(eglDefaultDisplay)
	if dpy == 0 || e.Initialize(dpy, nil, nil) == 0 {
		return 0, fmt.Errorf("gl: eglInitialize failed (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	return dpy, nil
})

// esConfig chooses an OpenGL ES 3 config on dpy for window and pbuffer
// surfaces with 8-bit RGB, no depth or stencil. It prefers no alpha with
// RGBX_8888 buffers (the format the canvas page's SurfaceView is set to:
// opaque, so the compositor does not blend), then any alpha-less config,
// then RGBA8. It returns the config and its native visual.
func esConfig(e *eglFuncs, dpy uintptr) (cfg uintptr, visual int32, err error) {
	attribs := []int32{
		eglRenderableType, eglOpenGLES3Bit,
		eglSurfaceType, eglWindowBit | eglPbufferBit,
		eglRedSize, 8, eglGreenSize, 8, eglBlueSize, 8,
		eglDepthSize, 0, eglStencilSize, 0,
		eglNone,
	}
	var n int32
	if e.ChooseConfig(dpy, &attribs[0], nil, 0, &n) == 0 || n <= 0 {
		return 0, 0, fmt.Errorf("gl: eglChooseConfig (OpenGL ES 3, RGB8): no config (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	cfgs := make([]uintptr, n)
	if e.ChooseConfig(dpy, &attribs[0], &cfgs[0], n, &n) == 0 || n <= 0 {
		return 0, 0, fmt.Errorf("gl: eglChooseConfig failed (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	attr := func(c uintptr, a int32) int32 {
		var v int32
		e.GetConfigAttrib(dpy, c, a, &v)
		return v
	}
	best, bestRank := uintptr(0), 99
	for _, c := range cfgs[:n] {
		if attr(c, eglRedSize) != 8 || attr(c, eglGreenSize) != 8 || attr(c, eglBlueSize) != 8 {
			continue
		}
		a, vis := attr(c, eglAlphaSize), attr(c, eglNativeVisualID)
		rank := 99
		switch {
		case a == 0 && vis == windowFormatRGBX8888:
			rank = 0
		case a == 0:
			rank = 1
		case a == 8:
			rank = 2
		}
		if rank < bestRank {
			best, bestRank, visual = c, rank, vis
		}
	}
	if best == 0 {
		return 0, 0, fmt.Errorf("gl: no OpenGL ES 3 config with 8-bit RGB: %w", ErrNoDevice)
	}
	return best, visual, nil
}

// glesLib is libGLESv3.so's handle (the loader's).
var glesLib = sync.OnceValues(func() (uintptr, error) {
	lib, err := purego.Dlopen("libGLESv3.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("gl: %w: libGLESv3.so: %v", ErrNoDevice, err)
	}
	return lib, nil
})

// glProc returns the GL function lookup for glFuncs.load: the core ES
// 3.0 functions from libGLESv3.so, extension functions (the robustness
// query) through eglGetProcAddress. Android's EGL dispatches both to the
// current context's driver.
func glProc() (func(name string) uintptr, error) {
	lib, err := glesLib()
	if err != nil {
		return nil, err
	}
	e, err := androidEGL()
	if err != nil {
		return nil, err
	}
	return func(name string) uintptr {
		if a, err := purego.Dlsym(lib, name); err == nil && a != 0 {
			return a
		}
		return e.GetProcAddress(name)
	}, nil
}
