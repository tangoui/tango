//go:build linux

package gl

import (
	"errors"
	"fmt"
	"image"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/devkit"
	"github.com/tangoui/tango/ui/paint"
)

// Presentation to a GTK 3 GtkGLArea.
//
// The Renderer keeps drawing into the Device's offscreen GL_RGBA8 target,
// exactly as without a window (so ReadPixels and the parity tests see
// what is shown), with the GLArea's own GL context: the presenter's
// Device runs every call on the caller's thread — uihost calls on the GTK
// main thread, inside CanvasFrame — after gtk_gl_area_make_current. The
// function table is loaded through libepoxy while that context is
// current. The GLArea is the platform's (platform.CanvasGL.UseGLArea:
// realized, OpenGL 3.3 core, auto-render off, no alpha, depth or
// stencil); the presenter holds a reference to it until Close, so its
// pointer stays valid even if GTK destroys the widget first.
//
// present(size, scale) does not draw: it calls gtk_gl_area_queue_render
// (the render handler sizes the blit from the area's buffer). GTK then emits the area's "render" signal
// while painting (the frame clock's paint phase, after this cycle's
// CanvasFrame), with the context current and the area's framebuffer bound
// as the draw framebuffer. The presenter's handler (connected in
// NewPresenter, before the first present; it returns TRUE) blits the
// target into that framebuffer — glBlitFramebuffer from the target's
// framebuffer, source (0, 0, w, h) to destination (0, H, w, H-h) with
// GL_NEAREST: one y flip, since the target stores its top row first and
// the window framebuffer its bottom row — where H is the buffer's height
// (allocation × scale factor, as GTK sizes it). When the buffer's size
// differs from the target's (after a resize CanvasFrame may run before
// the GLArea has its new allocation, and the target is ceil(Size×Scale)
// while the buffer is the allocation in device pixels) the buffer is
// first cleared to the frame's Frame.Clear and the target blitted top-left
// aligned, its excess cropped. Before the first frame, and after a failed
// one, the handler clears to that color (opaque black before any frame):
// the area never shows an uninitialized buffer. So the presenter's
// back-pressure is GTK's: at most one render per paint, queue_render
// coalescing repeated presents; present never waits for the GPU.
//
// Shared context. GTK uses the GLArea's context too (its framebuffer and
// renderbuffer, the resize handler's glViewport, and the read-back of the
// buffer when it is painted without GL compositing), and it is the
// toolkit's, not the Device's. Before Device work and before the blit
// the presenter resets the state that would change them and that GTK may
// leave set: GL_PIXEL_UNPACK_BUFFER and GL_PIXEL_PACK_BUFFER unbound, the
// pack and unpack SKIP_PIXELS / SKIP_ROWS / ROW_LENGTH 0 and alignment 4,
// GL_RASTERIZER_DISCARD and GL_SAMPLE_ALPHA_TO_COVERAGE disabled,
// glPolygonMode GL_FILL; the Device sets everything else it draws with on
// every frame. For the blit it also disables the scissor test and sRGB
// writes and enables every color channel (all three affect a blit). After
// the blit the handler binds the area's framebuffer as the read
// framebuffer too, leaving both bindings as GTK had them.
//
// Errors. gtk_gl_area_make_current failing (the area unrealized or
// destroyed, no context), gtk_gl_area_get_error set, or the area's
// GdkGLContext no longer being the one NewPresenter created the Device on
// (the area was unrealized and realized again: the Device's objects are
// not in the new context, or not alive) loses the device:
// r.Render and present return an error wrapping gpu.ErrDeviceLost from
// then on, and the caller goes back to the CPU. A GL error in the render
// handler is reported by the next present; maxFailures in a row, or a
// GL_OUT_OF_MEMORY, lose the device. Device errors are the Device's (see
// Device).
//
// Close (main thread) disconnects the handler and drops the reference to
// the area. When the area is still realized with the Device's context
// (uihost closed the renderer while the window lives: a fallback to the
// CPU, a software renderer rejected) it deletes every GL object with that
// context made current. When the window is closed it is not: GTK 3.24's
// gtk_widget_dispose hides and unrealizes the toplevel, recursively,
// before it emits "destroy", so by the time the surface's destroy reports
// SurfaceClosed and uihost closes the renderer the GLArea is unrealized
// and its GdkGLContext finalized (TestPresenterSurfaceClose checks this
// order on the real Linux canvas). Close then issues no GL call (no
// context is made current, nothing touches the dead context) and skips
// the deletes: the objects went with the context. They live in the share
// group GDK creates the area's context in (with the toplevel GdkWindow's
// paint context), which the same destroy tears down when it destroys the
// toplevel's GdkWindow, so nothing outlives the window (the sharing is
// GTK 3.24's gdk_window_create_gl_context, read from its source, not
// measured). The presenter tracks the context with a GObject weak
// reference, so it never compares or uses a finalized context's address.

// gtkGL holds the GTK, GDK and GObject entry points the presenter calls,
// bound from the toolkit's libraries (libgtk-3.so.0 and what it loads).
var gtkGL struct {
	once sync.Once
	err  error

	glAreaType    func() uintptr // GType
	makeCurrent   func(area uintptr)
	getError      func(area uintptr) uintptr // const GError*
	getContext    func(area uintptr) uintptr
	queueRender   func(area uintptr)
	getRealized   func(widget uintptr) int32
	allocW        func(widget uintptr) int32
	allocH        func(widget uintptr) int32
	scaleFactor   func(widget uintptr) int32
	currentCtx    func() uintptr // gdk_gl_context_get_current
	connect       func(instance uintptr, signal string, handler, data, destroy uintptr, flags int32) uintptr
	disconnect    func(instance, id uintptr)
	isConnected   func(instance, id uintptr) int32
	isA           func(instance, typ uintptr) int32
	ref           func(object uintptr) uintptr
	unref         func(object uintptr)
	weakRef       func(object, notify, data uintptr)
	weakUnref     func(object, notify, data uintptr)
	renderHandler uintptr // the "render" signal callback (one for every presenter)
	ctxGone       uintptr // the GWeakNotify on the area's GdkGLContext (one for every presenter)
}

// loadGTK binds gtkGL. The libraries are the toolkit's: the process
// normally has them loaded already (the Linux platform backend), and
// dlopen returns the same handles. Errors wrap ErrNoDevice.
func loadGTK() error {
	gtkGL.once.Do(func() {
		libs := map[string]map[string]any{
			"libgtk-3.so.0": {
				"gtk_gl_area_get_type":            &gtkGL.glAreaType,
				"gtk_gl_area_make_current":        &gtkGL.makeCurrent,
				"gtk_gl_area_get_error":           &gtkGL.getError,
				"gtk_gl_area_get_context":         &gtkGL.getContext,
				"gtk_gl_area_queue_render":        &gtkGL.queueRender,
				"gtk_widget_get_realized":         &gtkGL.getRealized,
				"gtk_widget_get_allocated_width":  &gtkGL.allocW,
				"gtk_widget_get_allocated_height": &gtkGL.allocH,
				"gtk_widget_get_scale_factor":     &gtkGL.scaleFactor,
			},
			"libgdk-3.so.0": {
				"gdk_gl_context_get_current": &gtkGL.currentCtx,
			},
			"libgobject-2.0.so.0": {
				"g_signal_connect_data":         &gtkGL.connect,
				"g_signal_handler_disconnect":   &gtkGL.disconnect,
				"g_signal_handler_is_connected": &gtkGL.isConnected,
				"g_type_check_instance_is_a":    &gtkGL.isA,
				"g_object_ref":                  &gtkGL.ref,
				"g_object_unref":                &gtkGL.unref,
				"g_object_weak_ref":             &gtkGL.weakRef,
				"g_object_weak_unref":           &gtkGL.weakUnref,
			},
		}
		for _, name := range []string{"libgtk-3.so.0", "libgdk-3.so.0", "libgobject-2.0.so.0"} {
			lib, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				gtkGL.err = fmt.Errorf("gl: %w: %s: %v", ErrNoDevice, name, err)
				return
			}
			if err := bindLib(lib, name, libs[name]); err != nil {
				gtkGL.err = fmt.Errorf("%w (%w)", err, ErrNoDevice)
				return
			}
		}
		// gboolean (*)(GtkGLArea *area, GdkGLContext *context, gpointer data)
		gtkGL.renderHandler = purego.NewCallback(func(area, ctx, data uintptr) uintptr {
			if p := presenters.get(data); p != nil {
				p.render(ctx)
			}
			return 1 // TRUE: handled
		})
		// void (*GWeakNotify)(gpointer data, GObject *where_the_object_was)
		gtkGL.ctxGone = purego.NewCallback(func(data, obj uintptr) uintptr {
			if p := presenters.get(data); p != nil {
				p.ctxDead = true
			}
			return 0
		})
	})
	return gtkGL.err
}

// gerrorMessage returns a GError's message (GError: GQuark domain, gint
// code, gchar *message).
func gerrorMessage(e uintptr) string {
	ge := *(*unsafe.Pointer)(unsafe.Pointer(&e)) // C memory GTK owns: not a Go pointer
	msg := *(**byte)(unsafe.Add(ge, 8))
	if msg == nil {
		return "unknown GL error"
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(msg), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(msg, n))
}

// presenters maps the render handler's user data to its presenter: the
// callback is shared, and a closed presenter's key finds nothing.
var presenters = presenterRegistry{m: map[uintptr]*presenter{}}

type presenterRegistry struct {
	mu   sync.Mutex
	next uintptr
	m    map[uintptr]*presenter
}

func (r *presenterRegistry) add(p *presenter) uintptr {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	r.m[r.next] = p
	return r.next
}

func (r *presenterRegistry) get(key uintptr) *presenter {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[key]
}

func (r *presenterRegistry) remove(key uintptr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, key)
}

// NewPresenter attaches a GPU renderer to a GtkGLArea obtained from
// platform.CanvasGL.UseGLArea. Main thread (the GTK thread) only. On
// other platforms it returns an error wrapping ErrUnsupported.
//
// It makes the area's context current, loads its GL functions, creates a
// Device on it (failing with an error wrapping ErrNoDevice without
// libgtk-3, libepoxy or OpenGL 3.3, and with the driver's info log when
// the shader does not compile) and connects the area's "render" signal.
// r is a gpu.Renderer over the Device; it also implements
// interface{ Close() error }, which disconnects the signal, deletes the
// GL objects when the area still has the Device's context (see above)
// and drops the reference to the area (main thread, idempotent; r and
// present fail with ErrClosed afterwards), and
// interface{ GLRenderer() string }, which returns the context's
// GL_RENDERER string as read in NewPresenter (e.g. "llvmpipe (LLVM 15.0.6,
// 128 bits)" for Mesa's software rasterizer; uihost keeps windows on the
// CPU on software renderers).
//
// present(size, scale) shows the frame r rendered last: size is the
// area's size in device pixels, round(Size*Scale) (scale is
// informational). It returns nil without presenting when size is empty;
// otherwise it queues a render of the area, in which the frame is
// blitted (see above). It returns the error of a failed blit, and an
// error wrapping gpu.ErrDeviceLost once the device is unusable — after
// which r.Render fails with gpu.ErrDeviceLost too, and the caller should
// go back to the CPU renderer.
func NewPresenter(glarea uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	p, err := newPresenter(glarea)
	if err != nil {
		return nil, nil, err
	}
	return p.r, p.present, nil
}

// presenter is NewPresenter's state. Main thread only.
type presenter struct {
	area        uintptr // GtkGLArea*, referenced until Close
	f           *glFuncs
	polygonMode func(face, mode uint32) // glPolygonMode; nil if the context lacks it
	dev         *Device
	r           paint.Renderer
	key         uintptr // the render handler's and weak notify's user data (presenters)
	handler     uintptr // the render signal's handler id
	closed      bool
	ctx         uintptr // the area's GdkGLContext the Device was created on (weakly referenced)
	ctxDead     bool    // ctx was finalized (the weak notify ran): its address means nothing
	glRenderer  string  // GL_RENDERER of ctx

	runErr      error // make_current failed inside the Device's run
	presentErr  error // a failed render handler, reported by the next present
	blitFailure int   // render handlers in a row with a GL error

	queued, rendered int         // renders queued, render handlers run (tests)
	bufSize          image.Point // the area's buffer at the last render handler (tests)
	closeDeleted     bool        // Close deleted the GL objects with ctx current (tests)
}

// presenterRenderer is the Renderer NewPresenter returns: a gpu.Renderer
// that makes the area's context current first and can be closed.
type presenterRenderer struct {
	*gpu.Renderer
	p *presenter
}

// Render makes the area's context current (a failure loses the device)
// and renders l into the Device's target.
func (r *presenterRenderer) Render(l *paint.List, size geom.Size, scale float32) error {
	p := r.p
	if p.closed {
		return ErrClosed
	}
	if err := p.lostErr(); err != nil {
		return err
	}
	if err := p.makeCurrent(); err != nil {
		return p.lose(err)
	}
	p.runErr = nil
	err := r.Renderer.Render(l, size, scale)
	if p.runErr != nil {
		return p.lose(p.runErr)
	}
	return err
}

// Close disconnects the render handler, deletes the Device's GL objects
// (when the area still has the Device's context) and releases the area.
func (r *presenterRenderer) Close() error {
	r.p.Close()
	return nil
}

// GLRenderer returns the GL_RENDERER string of the area's context, read
// when the presenter was created ("" if the driver returned none).
func (r *presenterRenderer) GLRenderer() string { return r.p.glRenderer }

func newPresenter(glarea uintptr) (p *presenter, err error) {
	if glarea == 0 {
		return nil, errors.New("gl: NewPresenter: no GtkGLArea (glarea 0)")
	}
	if err := loadGTK(); err != nil {
		return nil, err
	}
	if gtkGL.isA(glarea, gtkGL.glAreaType()) == 0 {
		return nil, errors.New("gl: NewPresenter: not a GtkGLArea")
	}
	p = &presenter{area: glarea}
	if err := p.makeCurrent(); err != nil {
		return nil, fmt.Errorf("gl: NewPresenter: %w (%w)", err, ErrNoDevice)
	}
	if p.f, err = loadCurrentFuncs(); err != nil {
		return nil, err
	}
	p.glRenderer = p.f.GetString(glRENDERER)
	if proc, err := glProc(); err == nil {
		if a := proc("glPolygonMode"); a != 0 { // desktop GL (always, as GTK was asked for 3.3 core)
			purego.RegisterFunc(&p.polygonMode, a)
		}
	}
	gtkGL.ref(glarea)
	// From here on makeCurrent also requires this context (a weak
	// reference: the notify sets ctxDead when GTK finalizes it).
	p.ctx = gtkGL.getContext(glarea)
	p.key = presenters.add(p)
	gtkGL.weakRef(p.ctx, gtkGL.ctxGone, p.key)
	p.dev, err = newDevice(p.run, p.f)
	if err == nil && p.runErr != nil { // the context was lost in between: nothing was created
		p.dev.closed = true
		err = fmt.Errorf("gl: NewPresenter: %w (%w)", p.runErr, ErrNoDevice)
	}
	if err != nil {
		p.dropContext()
		presenters.remove(p.key)
		gtkGL.unref(glarea)
		return nil, err
	}
	p.handler = gtkGL.connect(glarea, "render", gtkGL.renderHandler, p.key, 0, 0)
	if p.handler == 0 {
		p.Close()
		return nil, errors.New("gl: NewPresenter: cannot connect the GtkGLArea's render signal")
	}
	p.r = &presenterRenderer{Renderer: gpu.NewRenderer(p.dev), p: p}
	return p, nil
}

// dropContext removes the weak reference on ctx, unless GTK finalized
// it already.
func (p *presenter) dropContext() {
	if p.ctx != 0 && !p.ctxDead {
		gtkGL.weakUnref(p.ctx, gtkGL.ctxGone, p.key)
	}
	p.ctx = 0
}

// makeCurrent makes the area's context current on the calling thread.
// Once the Device exists the context must be the one it was created on.
// It checks the area is realized (and the recorded context alive) before
// any GL or make-current call: an unrealized area has no context.
func (p *presenter) makeCurrent() error {
	if gtkGL.getRealized(p.area) == 0 {
		return errors.New("gl: the GtkGLArea is not realized")
	}
	if p.ctxDead {
		return errors.New("gl: the GtkGLArea's GL context was destroyed (the area was unrealized)")
	}
	if p.ctx != 0 && gtkGL.getContext(p.area) != p.ctx {
		return errors.New("gl: the GtkGLArea's GL context changed (the area was unrealized and realized again)")
	}
	gtkGL.makeCurrent(p.area)
	if e := gtkGL.getError(p.area); e != 0 {
		return fmt.Errorf("gl: GtkGLArea: %s", gerrorMessage(e))
	}
	ctx := gtkGL.getContext(p.area)
	if ctx == 0 {
		return errors.New("gl: the GtkGLArea has no GL context")
	}
	if gtkGL.currentCtx() != ctx {
		return errors.New("gl: gtk_gl_area_make_current did not make the area's context current")
	}
	return nil
}

// run is the Device's run function: fn on the calling thread with the
// area's context current and the shared state reset. When the context
// cannot be made current (or is no longer the Device's) fn does not run
// and runErr records why (Render turns it into a loss; Close skips the
// deletes: the objects went with the Device's context, see the file
// comment).
func (p *presenter) run(fn func()) {
	if err := p.makeCurrent(); err != nil {
		p.runErr = err
		return
	}
	p.resetShared()
	fn()
}

// resetShared puts the context state the Device relies on but does not
// set every frame, and that GTK may leave set, back to GL's defaults.
func (p *presenter) resetShared() {
	f := p.f
	f.BindBuffer(glPIXEL_UNPACK_BUFFER, 0)
	f.BindBuffer(glPIXEL_PACK_BUFFER, 0)
	for _, s := range [...]struct {
		pname uint32
		v     int32
	}{
		{glUNPACK_SKIP_PIXELS, 0}, {glUNPACK_SKIP_ROWS, 0}, {glUNPACK_ROW_LENGTH, 0}, {glUNPACK_ALIGNMENT, 4},
		{glPACK_SKIP_PIXELS, 0}, {glPACK_SKIP_ROWS, 0}, {glPACK_ROW_LENGTH, 0}, {glPACK_ALIGNMENT, 4},
	} {
		f.PixelStorei(s.pname, s.v)
	}
	f.Disable(glRASTERIZER_DISCARD)
	f.Disable(glSAMPLE_ALPHA_TO_COVERAGE)
	if p.polygonMode != nil {
		p.polygonMode(glFRONT_AND_BACK, glFILL)
	}
}

// lostErr returns the Device's loss error, if any.
func (p *presenter) lostErr() error {
	p.dev.mu.Lock()
	defer p.dev.mu.Unlock()
	return p.dev.lost
}

// lose marks the Device lost because of err (if it is not already) and
// returns the loss error.
func (p *presenter) lose(err error) error {
	d := p.dev
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lost == nil {
		d.hasFrame = false
		d.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	return d.lost
}

func (p *presenter) present(size image.Point, scale float32) error {
	if p.closed {
		return ErrClosed
	}
	if err := p.lostErr(); err != nil {
		return err
	}
	if size.X <= 0 || size.Y <= 0 {
		return nil // minimized or empty: nothing to show
	}
	if gtkGL.getRealized(p.area) == 0 {
		return p.lose(errors.New("gl: the GtkGLArea is not realized"))
	}
	if err := p.presentErr; err != nil {
		p.presentErr = nil
		return err
	}
	gtkGL.queueRender(p.area)
	p.queued++
	return nil
}

// render is the area's "render" signal handler: GTK made ctx (the area's
// context) current and bound the area's framebuffer as the draw
// framebuffer.
func (p *presenter) render(ctx uintptr) {
	if p.closed {
		return
	}
	defer func() {
		// A panic must not unwind through GTK's C frames.
		if r := recover(); r != nil {
			p.presentErr = p.lose(fmt.Errorf("gl: render handler panicked: %v", r))
		}
	}()
	if p.ctxDead || ctx != p.ctx {
		// Realized again with a new context: the Device's objects are not
		// in it. Show black, never the uninitialized buffer, and lose the
		// device (the next present reports it).
		p.f.ClearColor(0, 0, 0, 1)
		p.f.Clear(glCOLOR_BUFFER_BIT)
		p.presentErr = p.lose(errors.New("gl: the GtkGLArea's GL context changed (the area was unrealized and realized again)"))
		return
	}
	d := p.dev
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.lost != nil {
		return
	}
	f := p.f
	p.rendered++
	var area int32
	f.GetIntegerv(glDRAW_FRAMEBUFFER_BINDING, &area)
	scale := gtkGL.scaleFactor(p.area)
	buf := image.Pt(int(gtkGL.allocW(p.area)*scale), int(gtkGL.allocH(p.area)*scale))
	p.bufSize = buf
	drainErrors(f) // GTK's, not the blit's
	p.resetShared()
	f.Disable(glSCISSOR_TEST)
	f.Disable(glFRAMEBUFFER_SRGB)
	f.ColorMask(1, 1, 1, 1)
	f.BindFramebuffer(glDRAW_FRAMEBUFFER, uint32(area))

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
		bh := int32(buf.Y)
		f.BlitFramebuffer(0, 0, w, h, 0, bh, w, bh-h, glCOLOR_BUFFER_BIT, glNEAREST)
	}
	f.BindFramebuffer(glREAD_FRAMEBUFFER, uint32(area)) // both bindings as GTK left them

	e := drainErrors(f)
	if d.resetStatus != nil {
		if rs := d.resetStatus(); rs != glNO_ERROR {
			p.presentErr = d.lose(fmt.Errorf("gl: the context was reset (glGetGraphicsResetStatus %#x)", rs))
			return
		}
	}
	switch e {
	case glNO_ERROR:
		p.blitFailure = 0
		return
	case glOUT_OF_MEMORY:
		p.presentErr = d.lose(fmt.Errorf("gl: present: GL_OUT_OF_MEMORY (%#x)", e))
		return
	}
	err := fmt.Errorf("gl: present: GL error %#x", e)
	p.blitFailure++
	if p.blitFailure >= maxFailures {
		err = d.lose(err)
	}
	p.presentErr = err
}

// Close disconnects the render handler, closes the Device (deleting its
// GL objects with the area's context current if the area still has it;
// otherwise without a GL call, see the file comment) and releases the
// area. Safe to call more than once.
func (p *presenter) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.handler != 0 && gtkGL.isConnected(p.area, p.handler) != 0 {
		gtkGL.disconnect(p.area, p.handler)
	}
	if p.dev != nil {
		p.runErr = nil
		p.dev.Close() // its run is p.run: no GL call when makeCurrent fails
		p.closeDeleted = p.runErr == nil
	}
	p.dropContext()
	presenters.remove(p.key)
	gtkGL.unref(p.area)
}
