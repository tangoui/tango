//go:build android

package gl

import "fmt"

// skipSurfaceless exists for the Linux tests sharing this package's test
// files; Android's headless context always uses a pbuffer.
var skipSurfaceless bool

// headless is an offscreen OpenGL ES 3 context with its own thread: on
// Android an EGL context on the default display with a 1×1 pbuffer
// current. Render into framebuffer objects. It serves NewDevice (tests on
// a device or emulator, readback); windows use NewPresenterAndroid.
type headless struct {
	thread *glThread
	funcs  *glFuncs // valid on thread only

	egl     *eglFuncs
	dpy     uintptr // EGLDisplay
	ctx     uintptr // EGLContext
	surface uintptr // EGLSurface: the pbuffer
	closed  bool
}

// newHeadlessContext creates an EGL ES 3 context current on a new thread
// and loads its function table. Errors wrap ErrNoDevice.
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
	e, err := androidEGL()
	if err != nil {
		return err
	}
	h.egl = e
	if h.dpy, err = eglDisplay(); err != nil {
		return err
	}
	if e.BindAPI(eglOpenGLESAPI) == 0 {
		return h.eglErr("eglBindAPI(EGL_OPENGL_ES_API)")
	}
	cfg, _, err := esConfig(e, h.dpy)
	if err != nil {
		return err
	}
	ctxAttribs := []int32{eglContextClientVersion, 3, eglNone}
	if h.ctx = e.CreateContext(h.dpy, cfg, 0, &ctxAttribs[0]); h.ctx == 0 {
		return h.eglErr("eglCreateContext (OpenGL ES 3)")
	}
	pb := []int32{eglWidth, 1, eglHeight, 1, eglNone}
	if h.surface = e.CreatePbufferSurface(h.dpy, cfg, &pb[0]); h.surface == 0 {
		return h.eglErr("eglCreatePbufferSurface")
	}
	if e.MakeCurrent(h.dpy, h.surface, h.surface, h.ctx) == 0 {
		return h.eglErr("eglMakeCurrent")
	}
	return h.loadFuncs()
}

func (h *headless) eglErr(op string) error {
	return fmt.Errorf("gl: %s failed (EGL error %#x): %w", op, h.egl.GetError(), ErrNoDevice)
}

// close destroys the context and stops its thread. Idempotent.
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
		e := h.egl
		if e == nil || h.dpy == 0 {
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
