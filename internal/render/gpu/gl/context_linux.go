//go:build linux && !android

package gl

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

// EGL enums (egl.h, eglext.h).
const (
	eglNone                                = 0x3038
	eglSuccess                             = 0x3000
	eglExtensions                          = 0x3055
	eglRenderableType                      = 0x3040
	eglOpenGLBit                           = 0x0008
	eglSurfaceType                         = 0x3033
	eglPbufferBit                          = 0x0001
	eglRedSize                             = 0x3024
	eglGreenSize                           = 0x3023
	eglBlueSize                            = 0x3022
	eglAlphaSize                           = 0x3021
	eglWidth                               = 0x3057
	eglHeight                              = 0x3056
	eglOpenGLAPI                           = 0x30A2
	eglContextMajorVersion                 = 0x3098
	eglContextMinorVersion                 = 0x30FB
	eglContextOpenGLProfileMask            = 0x30FD
	eglContextOpenGLCoreProfileBit         = 0x0001
	eglPlatformSurfacelessMESA             = 0x31DD
	eglDefaultDisplay              uintptr = 0
)

// libEGL returns the handle of libEGL.so.1 (the loader's, not GL state).
var libEGL = sync.OnceValues(func() (uintptr, error) {
	lib, err := purego.Dlopen("libEGL.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("gl: %w: libEGL.so.1: %v", ErrNoDevice, err)
	}
	return lib, nil
})

type eglFuncs struct {
	GetProcAddress       func(name string) uintptr
	QueryString          func(dpy uintptr, name int32) string
	GetDisplay           func(native uintptr) uintptr
	Initialize           func(dpy uintptr, major, minor *int32) uint32
	BindAPI              func(api uint32) uint32
	ChooseConfig         func(dpy uintptr, attribs *int32, configs *uintptr, size int32, num *int32) uint32
	CreateContext        func(dpy, config, share uintptr, attribs *int32) uintptr
	CreatePbufferSurface func(dpy, config uintptr, attribs *int32) uintptr
	MakeCurrent          func(dpy, draw, read, ctx uintptr) uint32
	DestroyContext       func(dpy, ctx uintptr) uint32
	DestroySurface       func(dpy, surface uintptr) uint32
	GetError             func() int32
	ReleaseThread        func() uint32

	// eglGetPlatformDisplayEXT (EGL_EXT_platform_base), through
	// eglGetProcAddress; nil without the extension.
	GetPlatformDisplayEXT func(platform uint32, native uintptr, attribs *int32) uintptr
}

// skipSurfaceless, when set (tests), skips Mesa's surfaceless platform so
// newHeadlessContext takes the default display with a pbuffer.
var skipSurfaceless bool

// headless is an offscreen GL context with its own thread: on Linux an
// EGL OpenGL 3.3 core context on Mesa's surfaceless platform, or else on
// the default display with a 1×1 pbuffer. Render into framebuffer
// objects. The EGL display is initialized once and never terminated:
// eglTerminate would also end the other contexts of the process on it.
type headless struct {
	thread *glThread
	funcs  *glFuncs // valid on thread only

	egl     eglFuncs
	dpy     uintptr // EGLDisplay
	ctx     uintptr // EGLContext
	surface uintptr // EGLSurface; 0 on the surfaceless platform
	closed  bool
}

// newHeadlessContext creates an EGL context current on a new thread and
// loads its function table. Errors wrap ErrNoDevice.
func newHeadlessContext() (ctx *headless, err error) {
	h := &headless{thread: newGLThread()}
	h.thread.do(func() { err = h.init() })
	if err != nil {
		h.close()
		return nil, err
	}
	return h, nil
}

func (h *headless) init() error {
	lib, err := libEGL()
	if err != nil {
		return err
	}
	e := &h.egl
	if err := bindLib(lib, "libEGL.so.1", map[string]any{
		"eglGetProcAddress":       &e.GetProcAddress,
		"eglQueryString":          &e.QueryString,
		"eglGetDisplay":           &e.GetDisplay,
		"eglInitialize":           &e.Initialize,
		"eglBindAPI":              &e.BindAPI,
		"eglChooseConfig":         &e.ChooseConfig,
		"eglCreateContext":        &e.CreateContext,
		"eglCreatePbufferSurface": &e.CreatePbufferSurface,
		"eglMakeCurrent":          &e.MakeCurrent,
		"eglDestroyContext":       &e.DestroyContext,
		"eglDestroySurface":       &e.DestroySurface,
		"eglGetError":             &e.GetError,
		"eglReleaseThread":        &e.ReleaseThread,
	}); err != nil {
		return fmt.Errorf("%w (%w)", err, ErrNoDevice)
	}
	surfaceless := false
	if exts := e.QueryString(0, eglExtensions); !skipSurfaceless && hasExt(exts, "EGL_EXT_platform_base") && hasExt(exts, "EGL_MESA_platform_surfaceless") {
		if a := e.GetProcAddress("eglGetPlatformDisplayEXT"); a != 0 {
			purego.RegisterFunc(&e.GetPlatformDisplayEXT, a)
			h.dpy = e.GetPlatformDisplayEXT(eglPlatformSurfacelessMESA, eglDefaultDisplay, nil)
			if h.dpy != 0 && e.Initialize(h.dpy, nil, nil) != 0 {
				surfaceless = true
			} else {
				h.dpy = 0
			}
		}
	}
	if !surfaceless {
		h.dpy = e.GetDisplay(eglDefaultDisplay)
		if h.dpy == 0 || e.Initialize(h.dpy, nil, nil) == 0 {
			h.dpy = 0
			return h.eglErr("eglInitialize")
		}
	}
	if e.BindAPI(eglOpenGLAPI) == 0 {
		return h.eglErr("eglBindAPI(EGL_OPENGL_API)")
	}
	cfgAttribs := []int32{
		eglRenderableType, eglOpenGLBit,
		eglSurfaceType, eglPbufferBit,
		eglRedSize, 8, eglGreenSize, 8, eglBlueSize, 8, eglAlphaSize, 8,
		eglNone,
	}
	var cfg uintptr
	var n int32
	if e.ChooseConfig(h.dpy, &cfgAttribs[0], &cfg, 1, &n) == 0 || n == 0 {
		return h.eglErr("eglChooseConfig (OpenGL, RGBA8)")
	}
	ctxAttribs := []int32{
		eglContextMajorVersion, 3,
		eglContextMinorVersion, 3,
		eglContextOpenGLProfileMask, eglContextOpenGLCoreProfileBit,
		eglNone,
	}
	if h.ctx = e.CreateContext(h.dpy, cfg, 0, &ctxAttribs[0]); h.ctx == 0 {
		return h.eglErr("eglCreateContext (OpenGL 3.3 core)")
	}
	if !surfaceless {
		pbAttribs := []int32{eglWidth, 1, eglHeight, 1, eglNone}
		if h.surface = e.CreatePbufferSurface(h.dpy, cfg, &pbAttribs[0]); h.surface == 0 {
			return h.eglErr("eglCreatePbufferSurface")
		}
	}
	if e.MakeCurrent(h.dpy, h.surface, h.surface, h.ctx) == 0 {
		return h.eglErr("eglMakeCurrent")
	}
	return h.loadFuncs()
}

// eglErr returns the error of the failed EGL call op.
func (h *headless) eglErr(op string) error {
	return fmt.Errorf("gl: %s failed (EGL error %#x): %w", op, h.egl.GetError(), ErrNoDevice)
}

// hasExt reports whether the space-separated extension list exts has ext.
func hasExt(exts, ext string) bool {
	for _, x := range strings.Fields(exts) {
		if x == ext {
			return true
		}
	}
	return false
}

// close destroys the context and stops its thread. It is idempotent.
// When the thread died (runtime.Goexit inside do) the context cannot be
// reached any more and is left to the driver.
func (h *headless) close() {
	if h.closed {
		return
	}
	h.closed = true
	if !h.thread.alive() {
		h.thread.close()
		return
	}
	h.thread.do(func() {
		e := &h.egl
		if h.dpy == 0 {
			return
		}
		e.MakeCurrent(h.dpy, 0, 0, 0)
		if h.surface != 0 {
			e.DestroySurface(h.dpy, h.surface)
			h.surface = 0
		}
		if h.ctx != 0 {
			e.DestroyContext(h.dpy, h.ctx)
			h.ctx = 0
		}
		e.ReleaseThread()
	})
	h.thread.close()
}
