//go:build android

package gl

import (
	"errors"
	"fmt"
	"image"
	"log"
	"sync"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/gl/nativewindow"
	"github.com/tangoui/tango/ui/gpu/internal/devkit"
	"github.com/tangoui/tango/ui/paint"
)

// Presentation to an Android native window with EGL and OpenGL ES 3.0
// (docs/specs/mobile-canvas.md §1.3.5).
//
// The target is a nativewindow.Holder's handle
// (platform.CanvasNativeWindow.UseNativeWindow), not a window: the canvas
// page's ANativeWindow comes and goes with its Surface (background,
// covered pages, the page closing), and the Holder always has the current
// one. The presenter owns one EGL context for the canvas's life, on the
// default display, made current on the caller's thread — uihost renders
// and presents on the Android main thread, inside the canvas's
// Choreographer callback, so no thread of its own is needed. Next to it
// lives a 1×1 pbuffer, current whenever there is no window surface (the
// Device's work and Close need a current context, a window does not).
//
// The Renderer draws into the Device's offscreen GL_RGBA8 target exactly
// as on Linux. present(size, scale) reads the Holder's window: none (in
// the background, before the first Surface) skips the frame; a window
// without an EGL surface gets one (eglCreateWindowSurface, the config
// chosen by esConfig: RGB8, preferably RGBX_8888 buffers to match the
// SurfaceView's format) and the presenter becomes the Holder's user, so
// the canvas's surfaceDestroyed destroys the surface (DetachWindow)
// before the window is released. The target is then blitted into the
// window's back buffer — glBlitFramebuffer, GL_NEAREST, flipped once,
// top-left aligned, the rest cleared to Frame.Clear when the sizes differ
// (as the Linux presenter) — and eglSwapBuffers posts it. Swapping waits
// for a free buffer (swap interval 1), the back-pressure ANativeWindow_lock
// gives the CPU path. EGL resizes the surface with the window (rotation),
// so the buffer's size is read from EGL_WIDTH/EGL_HEIGHT on every present.
//
// A window locked for a CPU frame (an oversized frame uihost draws on the
// CPU: nativewindow.Holder.UseCPU) cannot take an EGL surface again until
// it is replaced: the next present loses the device and uihost stays on
// the CPU.
//
// Context loss. Android drivers may lose the context, typically while the
// app is in the background: eglMakeCurrent or eglSwapBuffers fail with
// EGL_CONTEXT_LOST (or EGL_BAD_CONTEXT), and GL calls start failing. The
// presenter then rebuilds instead of giving up: it forgets the Device
// without a GL call (its objects died with the context), destroys the
// surfaces and the context, creates a new context and pbuffer, a new
// Device on it (newDevice) and a new gpu.Renderer over that, so every
// atlas page, ramp row and image is uploaded again by the next frame. A
// loss found in Render rebuilds and renders the frame again; one found in
// present rebuilds and returns a plain error, after which uihost asks for
// a new frame. A Device lost through its own failure count (GL errors,
// GL_OUT_OF_MEMORY) is rebuilt the same way. maxRebuilds rebuilds in a row
// without a frame presented in between lose the device for good: Render
// and present return errors wrapping gpu.ErrDeviceLost and uihost
// switches to the CPU.
//
// Close (main thread, idempotent) destroys the Device's objects, the
// surfaces and the context, and leaves the Holder.

// maxRebuilds context rebuilds in a row, with no frame presented in
// between, lose the device for good.
const maxRebuilds = 3

// NewPresenterAndroid attaches an OpenGL ES 3.0 renderer to the canvas
// window holder target (a nativewindow.Holder handle from
// platform.CanvasNativeWindow.UseNativeWindow). Android main thread only.
//
// It creates the EGL context and a Device on it, failing with an error
// wrapping ErrNoDevice without EGL, OpenGL ES 3 or a usable config, and
// with the driver's info log when the shader does not compile. It does
// not need the window to exist yet. r is a gpu.Renderer over the Device;
// it also implements interface{ Close() error } and
// interface{ GLRenderer() string } (GL_RENDERER of the context), as
// NewPresenter's. present(size, scale) shows the frame r rendered last
// (see above); size is the window's size in device pixels as the canvas
// computes it (round(Size*Scale)), used only to skip empty windows.
func NewPresenterAndroid(target uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	h := nativewindow.Lookup(target)
	if h == nil {
		return nil, nil, fmt.Errorf("gl: NewPresenterAndroid: no native window holder %d", target)
	}
	p := &androidPresenter{h: h}
	if err := p.open(); err != nil {
		p.destroyEGL()
		return nil, nil, err
	}
	p.r = &androidRenderer{Renderer: gpu.NewRenderer(p.dev), p: p}
	infoOnce.Do(func() {
		ramp := "GL_RGBA16 (EXT_texture_norm16)"
		if p.dev.formats[gpu.TexRamp].half {
			ramp = "GL_RGBA16F (no EXT_texture_norm16)"
		}
		log.Printf("tango: gl: OpenGL ES presenter: %s, %s, window format %d, ramp %s",
			p.f.GetString(glVERSION), p.glRenderer, p.visual, ramp)
	})
	return p.r, p.present, nil
}

// infoOnce logs the context's version and renderer once per process.
var infoOnce sync.Once

// androidPresenter is NewPresenterAndroid's state. Main thread only.
type androidPresenter struct {
	h   *nativewindow.Holder
	e   *eglFuncs
	dpy uintptr // EGLDisplay (the process's, never terminated)
	cfg uintptr // EGLConfig
	ctx uintptr // EGLContext; 0 between a loss and the rebuild
	pb  uintptr // 1×1 pbuffer, current while there is no window surface

	surf    uintptr // EGL window surface of the Holder's window; 0: none
	surfGen uint64  // the Holder generation surf was made for

	f          *glFuncs
	dev        *Device
	r          *androidRenderer
	glRenderer string
	visual     int32 // the config's native visual (window format)

	closed      bool
	lost        error // non-nil (wrapping gpu.ErrDeviceLost): for good
	runErr      error // making the context current failed inside the Device's run
	rebuilds    int   // rebuilds since the last frame presented
	blitFailure int   // presents in a row with a GL error
	surfFailure int   // window surfaces in a row that could not be created
	presented   int   // frames swapped (tests, logs)
}

// androidRenderer is the Renderer NewPresenterAndroid returns: the
// current gpu.Renderer (replaced by a rebuild) behind a make-current.
type androidRenderer struct {
	*gpu.Renderer
	p *androidPresenter
}

// Render makes the context current and renders l into the Device's
// target. A lost context is rebuilt and the frame rendered again on the
// new one.
func (r *androidRenderer) Render(l *paint.List, size geom.Size, scale float32) error {
	p := r.p
	err := r.render(l, size, scale)
	if err == nil || p.lost != nil || !p.isLoss(err) {
		return err
	}
	if rerr := p.rebuild(err); rerr != nil {
		return rerr
	}
	if err = r.render(l, size, scale); err == nil || p.lost != nil || !p.isLoss(err) {
		return err
	}
	if rerr := p.rebuild(err); rerr != nil {
		return rerr
	}
	// Not wrapping the loss: uihost asks for another frame instead of
	// leaving the GPU.
	return fmt.Errorf("gl: render: %v; the context was recreated, redrawing", err)
}

// render renders once on the current context.
func (r *androidRenderer) render(l *paint.List, size geom.Size, scale float32) error {
	p := r.p
	if p.closed {
		return ErrClosed
	}
	if p.lost != nil {
		return p.lost
	}
	if err := p.makeCurrent(); err != nil {
		return err
	}
	p.runErr = nil
	err := r.Renderer.Render(l, size, scale)
	if p.runErr != nil {
		err = p.runErr
	}
	return err
}

// Close releases everything (see the file comment).
func (r *androidRenderer) Close() error {
	r.p.Close()
	return nil
}

// GLRenderer returns the context's GL_RENDERER string.
func (r *androidRenderer) GLRenderer() string { return r.p.glRenderer }

// eglError is a failed EGL call.
type eglError struct {
	op   string
	code int32
}

func (e *eglError) Error() string { return fmt.Sprintf("gl: %s failed (EGL error %#x)", e.op, e.code) }

// contextGone reports whether an EGL error means the context is unusable.
func contextGone(code int32) bool { return code == eglContextLost || code == eglBadContext }

// isLoss reports whether err means the context or device must be rebuilt.
func (p *androidPresenter) isLoss(err error) bool {
	var ee *eglError
	if errors.As(err, &ee) {
		return contextGone(ee.code)
	}
	return errors.Is(err, gpu.ErrDeviceLost)
}

// open creates the context, the pbuffer and the Device (NewPresenterAndroid
// and rebuild).
func (p *androidPresenter) open() error {
	e, err := androidEGL()
	if err != nil {
		return err
	}
	p.e = e
	if p.dpy, err = eglDisplay(); err != nil {
		return err
	}
	if e.BindAPI(eglOpenGLESAPI) == 0 {
		return fmt.Errorf("gl: eglBindAPI(EGL_OPENGL_ES_API) failed (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	if p.cfg == 0 {
		if p.cfg, p.visual, err = esConfig(e, p.dpy); err != nil {
			return err
		}
	}
	attribs := []int32{eglContextClientVersion, 3, eglNone}
	if p.ctx = e.CreateContext(p.dpy, p.cfg, 0, &attribs[0]); p.ctx == 0 {
		return fmt.Errorf("gl: eglCreateContext (OpenGL ES 3) failed (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	pb := []int32{eglWidth, 1, eglHeight, 1, eglNone}
	if p.pb = e.CreatePbufferSurface(p.dpy, p.cfg, &pb[0]); p.pb == 0 {
		return fmt.Errorf("gl: eglCreatePbufferSurface failed (EGL error %#x): %w", e.GetError(), ErrNoDevice)
	}
	if err := p.makeCurrent(); err != nil {
		return fmt.Errorf("%w (%w)", err, ErrNoDevice)
	}
	if p.f, err = loadCurrentFuncs(); err != nil {
		return err
	}
	p.glRenderer = p.f.GetString(glRENDERER)
	p.runErr = nil
	p.dev, err = newDevice(p.run, p.f)
	if err == nil && p.runErr != nil {
		p.dev.closed = true
		err = fmt.Errorf("gl: NewPresenterAndroid: %w (%w)", p.runErr, ErrNoDevice)
	}
	return err
}

// makeCurrent makes the context current on the calling thread with the
// window surface (or the pbuffer without one) as its draw and read
// surface. A window surface that cannot be made current any more (its
// window died) is dropped for the pbuffer; a lost context is an eglError
// isLoss reports.
func (p *androidPresenter) makeCurrent() error {
	e := p.e
	if p.ctx == 0 {
		return &eglError{"eglMakeCurrent (no context)", eglBadContext}
	}
	want := p.pb
	if p.surf != 0 {
		want = p.surf
	}
	if e.GetCurrentContext() == p.ctx && e.GetCurrentSurface(eglDraw) == want {
		return nil
	}
	if e.MakeCurrent(p.dpy, want, want, p.ctx) != 0 {
		return nil
	}
	code := e.GetError()
	if want == p.surf && !contextGone(code) {
		p.dropSurface()
		if e.MakeCurrent(p.dpy, p.pb, p.pb, p.ctx) != 0 {
			return nil
		}
		code = e.GetError()
	}
	return &eglError{"eglMakeCurrent", code}
}

// run is the Device's run function: fn with the context current. When it
// cannot be made current fn does not run and runErr records why.
func (p *androidPresenter) run(fn func()) {
	if err := p.makeCurrent(); err != nil {
		p.runErr = err
		return
	}
	fn()
}

// DetachWindow implements nativewindow.User: the canvas gives the window
// up (surfaceDestroyed, a new window, a CPU frame, the page closing).
func (p *androidPresenter) DetachWindow() { p.dropSurface() }

// dropSurface destroys the window surface, first making the pbuffer
// current if the surface is (EGL would keep a current surface alive, and
// with it the window).
func (p *androidPresenter) dropSurface() {
	if p.surf == 0 {
		return
	}
	e := p.e
	if e.GetCurrentSurface(eglDraw) == p.surf {
		if p.ctx == 0 || p.pb == 0 || e.MakeCurrent(p.dpy, p.pb, p.pb, p.ctx) == 0 {
			e.MakeCurrent(p.dpy, 0, 0, 0)
		}
	}
	e.DestroySurface(p.dpy, p.surf)
	p.surf = 0
	p.h.SetUser(nil)
}

// destroyEGL destroys the surfaces and the context. The Device's objects
// go with the context.
func (p *androidPresenter) destroyEGL() {
	e := p.e
	if e == nil || p.dpy == 0 {
		return
	}
	p.dropSurface()
	if c := e.GetCurrentContext(); c != 0 && c == p.ctx {
		e.MakeCurrent(p.dpy, 0, 0, 0)
	}
	if p.pb != 0 {
		e.DestroySurface(p.dpy, p.pb)
		p.pb = 0
	}
	if p.ctx != 0 {
		e.DestroyContext(p.dpy, p.ctx)
		p.ctx = 0
	}
}

// rebuild replaces a lost context (cause) with a new context, Device and
// gpu.Renderer, or loses the device for good after maxRebuilds in a row.
// It returns nil when the new context is ready.
func (p *androidPresenter) rebuild(cause error) error {
	if p.rebuilds >= maxRebuilds {
		return p.lose(fmt.Errorf("%d context rebuilds without a frame: %w", p.rebuilds, cause))
	}
	p.rebuilds++
	log.Printf("tango: gl: OpenGL ES context lost (%v): recreating it (%d of %d)", cause, p.rebuilds, maxRebuilds)
	if p.dev != nil {
		p.dev.mu.Lock()
		p.dev.closed = true // its objects died with the context: no GL call
		p.dev.mu.Unlock()
	}
	p.destroyEGL()
	p.f, p.dev = nil, nil
	if err := p.open(); err != nil {
		p.destroyEGL()
		return p.lose(fmt.Errorf("recreating the context: %w (after %w)", err, cause))
	}
	p.r.Renderer = gpu.NewRenderer(p.dev) // empty caches: the next frame uploads everything
	p.blitFailure, p.surfFailure = 0, 0
	return nil
}

// lose loses the device for good (the caller goes back to the CPU) and
// returns the loss error.
func (p *androidPresenter) lose(err error) error {
	if p.lost == nil {
		p.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
		if p.dev != nil {
			p.dev.mu.Lock()
			p.dev.hasFrame = false
			if p.dev.lost == nil {
				p.dev.lost = p.lost
			}
			p.dev.mu.Unlock()
		}
	}
	return p.lost
}

// present shows the Device's last frame in the Holder's current window.
func (p *androidPresenter) present(size image.Point, scale float32) error {
	if p.closed {
		return ErrClosed
	}
	if p.lost != nil {
		return p.lost
	}
	if size.X <= 0 || size.Y <= 0 || p.h.Closed() {
		return nil
	}
	win, _, _, gen := p.h.Window()
	if win == 0 {
		p.dropSurface() // normally done by the Holder already
		return nil      // no Surface (background, covered): nothing to show
	}
	if p.h.CPU() {
		return p.lose(errors.New("gl: the window was locked for a CPU frame; it takes no EGL surface until it is replaced"))
	}
	if p.surf != 0 && p.surfGen != gen {
		p.dropSurface()
	}
	e := p.e
	if p.surf == 0 {
		if p.ctx == 0 {
			if err := p.rebuild(errors.New("gl: no context")); err != nil {
				return err
			}
		}
		s := e.CreateWindowSurface(p.dpy, p.cfg, win, nil)
		if s == 0 {
			code := e.GetError()
			if contextGone(code) {
				return p.presentLoss(&eglError{"eglCreateWindowSurface", code})
			}
			p.surfFailure++
			err := &eglError{"eglCreateWindowSurface", code}
			if p.surfFailure >= maxFailures {
				return p.lose(err)
			}
			return err
		}
		p.surf, p.surfGen, p.surfFailure = s, gen, 0
		p.h.SetUser(p)
	}
	if err := p.makeCurrent(); err != nil {
		if p.isLoss(err) {
			return p.presentLoss(err)
		}
		return err
	}
	if p.surf == 0 { // dropped by makeCurrent: the window died under us
		return errors.New("gl: the window surface could not be made current")
	}
	if err := p.blit(); err != nil {
		if p.isLoss(err) {
			return p.presentLoss(err)
		}
		return err
	}
	if e.SwapBuffers(p.dpy, p.surf) == 0 {
		code := e.GetError()
		err := &eglError{"eglSwapBuffers", code}
		if contextGone(code) {
			return p.presentLoss(err)
		}
		// The window went away (EGL_BAD_SURFACE, EGL_BAD_NATIVE_WINDOW)
		// or out of buffers: a new surface next time.
		p.dropSurface()
		p.surfFailure++
		if p.surfFailure >= maxFailures {
			return p.lose(err)
		}
		return err
	}
	p.presented++
	p.rebuilds = 0
	return nil
}

// presentLoss rebuilds after a loss found while presenting. The frame is
// gone: the plain error it returns makes uihost ask for another one.
func (p *androidPresenter) presentLoss(cause error) error {
	if err := p.rebuild(cause); err != nil {
		return err
	}
	// Not wrapping cause (it may wrap gpu.ErrDeviceLost): uihost asks for
	// another frame instead of leaving the GPU.
	return fmt.Errorf("gl: present: %v; the context was recreated, redrawing", cause)
}

// blit copies the Device's target into the window surface's back buffer
// (default framebuffer), with the context current on it.
func (p *androidPresenter) blit() error {
	d := p.dev
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.lost != nil {
		return d.lost
	}
	f := p.f
	var bw, bh int32
	p.e.QuerySurface(p.dpy, p.surf, eglWidth, &bw)
	p.e.QuerySurface(p.dpy, p.surf, eglHeight, &bh)
	buf := image.Pt(int(bw), int(bh))
	drainErrors(f)
	f.BindFramebuffer(glDRAW_FRAMEBUFFER, 0)
	f.Viewport(0, 0, bw, bh)
	f.Disable(glSCISSOR_TEST)
	f.ColorMask(1, 1, 1, 1)
	src := d.target.size
	frame := d.hasFrame && d.target.tex != 0
	if !frame || src != buf {
		c := devkit.ClearColor(d.clear)
		f.ClearColor(c[0], c[1], c[2], c[3])
		f.Clear(glCOLOR_BUFFER_BIT)
	}
	if frame {
		f.BindFramebuffer(glREAD_FRAMEBUFFER, d.fbo)
		f.ReadBuffer(glCOLOR_ATTACHMENT0)
		w, h := int32(src.X), int32(src.Y)
		f.BlitFramebuffer(0, 0, w, h, 0, bh, w, bh-h, glCOLOR_BUFFER_BIT, glNEAREST)
	}
	f.BindFramebuffer(glFRAMEBUFFER, 0)
	f.Enable(glSCISSOR_TEST)

	e := drainErrors(f)
	if d.resetStatus != nil {
		if rs := d.resetStatus(); rs != glNO_ERROR {
			d.hasFrame = false
			return &eglError{fmt.Sprintf("present (glGetGraphicsResetStatus %#x)", rs), eglContextLost}
		}
	}
	switch e {
	case glNO_ERROR:
		p.blitFailure = 0
		return nil
	case glOUT_OF_MEMORY:
		return d.lose(fmt.Errorf("gl: present: GL_OUT_OF_MEMORY (%#x)", e))
	}
	err := fmt.Errorf("gl: present: GL error %#x", e)
	p.blitFailure++
	if p.blitFailure >= maxFailures {
		err = d.lose(err)
	}
	return err
}

// Close destroys the Device's objects (with the context current), the
// surfaces and the context, and leaves the Holder. Idempotent.
func (p *androidPresenter) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.dev != nil {
		p.dev.Close() // its run makes the context current; no GL call if it cannot
	}
	p.destroyEGL()
	if p.e != nil {
		p.e.ReleaseThread()
	}
	p.h.SetUser(nil)
}
