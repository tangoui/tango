//go:build linux && !android

package gl

// Window tests of the GtkGLArea presenter on a real GTK 3. They need a
// display and GL: run them under Xvfb, e.g.
//
//	xvfb-run -a -s "-screen 0 1280x800x24" go test ./ui/gpu/gl/ -run Presenter
//
// Without DISPLAY / WAYLAND_DISPLAY, libgtk-3 or GL they skip. GTK runs
// on one goroutine locked to its OS thread (a glThread), initialized by
// the first test that needs it; every GTK and presenter call goes through
// onGTK, and test assertions stay on the test goroutine.

import (
	"errors"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/paint"
)

// presenterSubprocessEnv marks the GDK_SCALE=2 child of TestPresenterScale2.
const presenterSubprocessEnv = "TANGO_GL_PRESENTER_SUBPROCESS"

var gtkTest struct {
	once sync.Once
	th   *glThread
	err  error // why window tests skip

	initCheck     func(argc, argv uintptr) int32
	windowNew     func(typ int32) uintptr
	setDefault    func(w uintptr, width, height int32)
	windowResize  func(w uintptr, width, height int32)
	glAreaNew     func() uintptr
	setRequired   func(area uintptr, major, minor int32)
	setAutoRender func(area uintptr, on int32)
	setHasAlpha   func(area uintptr, on int32)
	setUseES      func(area uintptr, on int32)
	attachBuffers func(area uintptr)
	containerAdd  func(c, w uintptr)
	showAll       func(w uintptr)
	destroy       func(w uintptr)
	unrealize     func(w uintptr)
	realize       func(w uintptr)
	iteration     func(ctx uintptr, mayBlock int32) int32
}

// needGTK skips t without a display or GTK and returns the GTK thread.
func needGTK(t *testing.T) {
	t.Helper()
	gtkTest.once.Do(func() {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			gtkTest.err = errors.New("no DISPLAY / WAYLAND_DISPLAY")
			return
		}
		if err := loadGTK(); err != nil {
			gtkTest.err = err
			return
		}
		lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			gtkTest.err = err
			return
		}
		glib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			gtkTest.err = err
			return
		}
		g := &gtkTest
		if err := bindLib(lib, "libgtk-3.so.0", map[string]any{
			"gtk_init_check":                   &g.initCheck,
			"gtk_window_new":                   &g.windowNew,
			"gtk_window_set_default_size":      &g.setDefault,
			"gtk_window_resize":                &g.windowResize,
			"gtk_gl_area_new":                  &g.glAreaNew,
			"gtk_gl_area_set_required_version": &g.setRequired,
			"gtk_gl_area_set_auto_render":      &g.setAutoRender,
			"gtk_gl_area_set_has_alpha":        &g.setHasAlpha,
			"gtk_gl_area_set_use_es":           &g.setUseES,
			"gtk_gl_area_attach_buffers":       &g.attachBuffers,
			"gtk_container_add":                &g.containerAdd,
			"gtk_widget_show_all":              &g.showAll,
			"gtk_widget_destroy":               &g.destroy,
			"gtk_widget_unrealize":             &g.unrealize,
			"gtk_widget_realize":               &g.realize,
		}); err != nil {
			gtkTest.err = err
			return
		}
		if err := bindLib(glib, "libglib-2.0.so.0", map[string]any{"g_main_context_iteration": &g.iteration}); err != nil {
			gtkTest.err = err
			return
		}
		g.th = newGLThread()
		ok := false
		g.th.do(func() { ok = g.initCheck(0, 0) != 0 })
		if !ok {
			gtkTest.err = errors.New("gtk_init_check failed")
		}
	})
	if gtkTest.err != nil {
		t.Skipf("no GTK window: %v", gtkTest.err)
	}
}

// onGTK runs fn on the GTK thread. fn must not call t.Fatal.
func onGTK(fn func()) { gtkTest.th.do(fn) }

// pump runs GTK's main loop on the GTK thread until cond (evaluated there)
// holds or 5 s pass, and reports whether it held.
func pump(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		ok := false
		onGTK(func() {
			for gtkTest.iteration(0, 0) != 0 {
			}
			ok = cond()
		})
		if ok {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// testArea is a shown toplevel holding one GtkGLArea configured like the
// Linux canvas's (GL 3.3 core, no auto-render, no alpha).
type testArea struct {
	win, area uintptr
	scale     int // the area's scale factor (GDK_SCALE)
}

// dev returns the logical size w×h in the area's device pixels.
func (a *testArea) dev(w, h int) image.Point { return image.Pt(w*a.scale, h*a.scale) }

// newTestArea opens a w×h (logical) window with a realized, allocated
// GLArea, destroyed at the end of the test.
func newTestArea(t *testing.T, w, h int) *testArea {
	t.Helper()
	needGTK(t)
	a := &testArea{}
	g := &gtkTest
	onGTK(func() {
		a.win = g.windowNew(0) // GTK_WINDOW_TOPLEVEL
		g.setDefault(a.win, int32(w), int32(h))
		a.area = g.glAreaNew()
		g.setRequired(a.area, 3, 3)
		g.setUseES(a.area, 0)
		g.setAutoRender(a.area, 0)
		g.setHasAlpha(a.area, 0)
		g.containerAdd(a.win, a.area)
		g.showAll(a.win)
	})
	t.Cleanup(func() { a.close() })
	if !pump(func() bool {
		return gtkGL.getRealized(a.area) != 0 && gtkGL.allocW(a.area) == int32(w) && gtkGL.allocH(a.area) == int32(h)
	}) {
		var rw, rh int32
		onGTK(func() { rw, rh = gtkGL.allocW(a.area), gtkGL.allocH(a.area) })
		t.Fatalf("GLArea not realized at %d×%d (allocation %d×%d)", w, h, rw, rh)
	}
	var gerr error
	onGTK(func() {
		if e := gtkGL.getError(a.area); e != 0 {
			gerr = errors.New(gerrorMessage(e))
		}
	})
	if gerr != nil {
		t.Skipf("GtkGLArea has no GL context: %v", gerr)
	}
	onGTK(func() { a.scale = int(gtkGL.scaleFactor(a.area)) })
	return a
}

func (a *testArea) close() {
	if a.win == 0 {
		return
	}
	onGTK(func() { gtkTest.destroy(a.win) })
	a.win = 0
	pump(func() bool { return true })
}

// startTestPresenter runs newPresenter on a's GLArea and closes it at the
// end of the test (before the window is destroyed).
func startTestPresenter(t *testing.T, a *testArea) *presenter {
	t.Helper()
	var (
		p   *presenter
		err error
	)
	onGTK(func() { p, err = newPresenter(a.area) })
	if errors.Is(err, ErrNoDevice) {
		t.Skipf("no GL device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { onGTK(p.Close) })
	return p
}

// renderSolid renders a frame of size (logical) at a's scale cleared to
// bg with a fill of fg covering its top-left quarter.
func renderSolid(p *presenter, a *testArea, size geom.Size, bg, fg color.NRGBA) error {
	scale := float32(a.scale)
	l := &paint.List{Background: bg}
	l.Fill(geom.Rect{W: size.W / 2, H: size.H / 2}, geom.Radii{}, fg)
	var err error
	onGTK(func() { err = p.r.Render(l, size, scale) })
	return err
}

// presentAndWait presents the logical size w×h (in a's device pixels)
// and runs GTK until the GLArea's render signal has run for it.
func presentAndWait(t *testing.T, p *presenter, a *testArea, w, h int) {
	t.Helper()
	size := a.dev(w, h)
	var err error
	var before int
	onGTK(func() {
		before = p.rendered
		err = p.present(size, float32(a.scale))
	})
	if err != nil {
		t.Fatalf("present(%v): %v", size, err)
	}
	if !pump(func() bool { return p.rendered > before }) {
		t.Fatalf("present(%v): the GLArea's render signal did not run", size)
	}
}

// readArea reads the GLArea framebuffer's pixel at the logical point
// (x, y) (top-left coordinates, times a's scale), outside the render
// signal: the area's context made current and its buffers attached, as
// GTK does.
func readArea(p *presenter, a *testArea, x, y int) color.RGBA {
	x, y = x*a.scale, y*a.scale
	var px [4]byte
	onGTK(func() {
		gtkGL.makeCurrent(a.area)
		gtkTest.attachBuffers(a.area)
		f := p.f
		h := int(gtkGL.allocH(a.area) * gtkGL.scaleFactor(a.area))
		f.ReadPixels(int32(x), int32(h-1-y), 1, 1, glRGBA, glUNSIGNED_BYTE, unsafe.Pointer(&px[0]))
	})
	return color.RGBA{px[0], px[1], px[2], px[3]}
}

var (
	red    = color.NRGBA{200, 30, 60, 255}
	green  = color.NRGBA{20, 180, 40, 255}
	redPx  = color.RGBA{200, 30, 60, 255}
	greenP = color.RGBA{20, 180, 40, 255}
)

// TestPresenterBasic: three frames rendered and presented without error;
// the GLArea's framebuffer holds the frame, upright (the top-left quarter
// is the fill), and the Renderer is closable.
func TestPresenterBasic(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	if _, ok := p.r.(interface{ Close() error }); !ok {
		t.Fatal("the presenter's Renderer has no Close")
	}
	for i := range 3 {
		if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		presentAndWait(t, p, a, 160, 100)
	}
	if p.bufSize != a.dev(160, 100) {
		t.Fatalf("GLArea buffer %v, want %v", p.bufSize, a.dev(160, 100))
	}
	if c := readArea(p, a, 80, 50); c != redPx {
		t.Errorf("center pixel %v, want %v", c, redPx)
	}
	if c := readArea(p, a, 10, 10); c != greenP {
		t.Errorf("top-left pixel %v, want %v (frame upside down?)", c, greenP)
	}
	if c := readArea(p, a, 150, 90); c != redPx {
		t.Errorf("bottom-right pixel %v, want %v", c, redPx)
	}
}

// TestPresenterBeforeFirstFrame: a render before any frame shows a
// defined color (opaque black), never an uninitialized buffer.
func TestPresenterBeforeFirstFrame(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	presentAndWait(t, p, a, 160, 100)
	if c := readArea(p, a, 80, 50); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("pixel before the first frame %v, want opaque black", c)
	}
}

// TestPresenterResize: after the window grows the frame follows; a frame
// whose size differs from the GLArea's buffer (rendered before the area
// got its new allocation) is shown top-left aligned over Frame.Clear.
func TestPresenterResize(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)

	onGTK(func() { gtkTest.windowResize(a.win, 240, 150) })
	if !pump(func() bool { return gtkGL.allocW(a.area) == 240 && gtkGL.allocH(a.area) == 150 }) {
		t.Fatal("window did not resize to 240×150")
	}
	// The old frame (160×100) on the new 240×150 buffer: top-left aligned,
	// the rest cleared to the frame's clear color.
	presentAndWait(t, p, a, 240, 150)
	if p.bufSize != a.dev(240, 150) {
		t.Fatalf("GLArea buffer %v, want %v", p.bufSize, a.dev(240, 150))
	}
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{{10, 10, greenP}, {70, 40, greenP}, {90, 60, redPx}, {200, 20, redPx}, {20, 130, redPx}, {230, 140, redPx}} {
		if got := readArea(p, a, c.x, c.y); got != c.want {
			t.Errorf("mismatched sizes: pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// A frame at the new size fills it.
	if err := renderSolid(p, a, geom.Size{W: 240, H: 150}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 240, 150)
	if got := readArea(p, a, 110, 70); got != greenP {
		t.Errorf("after the resize: pixel 110,70 = %v, want %v", got, greenP)
	}
	if got := readArea(p, a, 130, 80); got != redPx {
		t.Errorf("after the resize: pixel 130,80 = %v, want %v", got, redPx)
	}
	// Shrinking: a larger frame is cropped at the right and bottom.
	if err := renderSolid(p, a, geom.Size{W: 240, H: 150}, red, green); err != nil {
		t.Fatal(err)
	}
	onGTK(func() { gtkTest.windowResize(a.win, 100, 60) })
	if !pump(func() bool { return gtkGL.allocW(a.area) == 100 && gtkGL.allocH(a.area) == 60 }) {
		t.Fatal("window did not resize to 100×60")
	}
	presentAndWait(t, p, a, 100, 60)
	if got := readArea(p, a, 10, 10); got != greenP {
		t.Errorf("cropped: pixel 10,10 = %v, want %v", got, greenP)
	}
	if got := readArea(p, a, 99, 59); got != greenP {
		t.Errorf("cropped: pixel 99,59 = %v, want %v (the frame's 240×150 top-left part)", got, greenP)
	}
}

// TestPresenterZeroSize: an empty size (minimized window) queues no
// render and returns nil.
func TestPresenterZeroSize(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)
	var errs []error
	var queued int
	onGTK(func() {
		queued = p.queued
		for _, size := range []image.Point{{}, {0, 100}, {160, 0}, {-1, 5}} { // device pixels
			errs = append(errs, p.present(size, 1))
		}
	})
	for i, err := range errs {
		if err != nil {
			t.Errorf("empty size %d: %v", i, err)
		}
	}
	if p.queued != queued {
		t.Errorf("empty sizes queued %d renders", p.queued-queued)
	}
}

// TestPresenterScale2 runs TestPresenterScale2Child with GDK_SCALE=2: the
// GLArea's buffer is in device pixels, twice the logical size, and a
// frame rendered at scale 2 fills it.
func TestPresenterScale2(t *testing.T) {
	if os.Getenv(presenterSubprocessEnv) != "" {
		t.Skip("in the subprocess")
	}
	needGTK(t)
	cmd := exec.Command(os.Args[0], "-test.run", "^TestPresenterScale2Child$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), presenterSubprocessEnv+"=1", "GDK_SCALE=2")
	out, err := cmd.CombinedOutput()
	t.Logf("subprocess:\n%s", out)
	if err != nil {
		t.Fatalf("subprocess: %v", err)
	}
	if strings.Contains(string(out), "--- SKIP: TestPresenterScale2Child") {
		t.Skip("subprocess skipped")
	}
	if !strings.Contains(string(out), "--- PASS: TestPresenterScale2Child") {
		t.Fatal("subprocess did not run TestPresenterScale2Child")
	}
}

func TestPresenterScale2Child(t *testing.T) {
	if os.Getenv(presenterSubprocessEnv) == "" {
		t.Skip("only in TestPresenterScale2's subprocess")
	}
	a := newTestArea(t, 160, 100)
	var scale int32
	onGTK(func() { scale = gtkGL.scaleFactor(a.area) })
	if scale != 2 {
		t.Fatalf("scale factor %d under GDK_SCALE=2", scale)
	}
	p := startTestPresenter(t, a)
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)
	if p.bufSize != image.Pt(320, 200) {
		t.Fatalf("GLArea buffer %v, want 320×200 (device pixels)", p.bufSize)
	}
	if p.dev.target.size != image.Pt(320, 200) {
		t.Fatalf("target %v, want 320×200", p.dev.target.size)
	}
	// readArea takes logical points: device 160,100 (center), 150,90
	// (inside the 160×100 device-pixel fill) and 170,110 (outside it).
	if c := readArea(p, a, 80, 50); c != redPx {
		t.Errorf("center pixel %v, want %v", c, redPx)
	}
	if c := readArea(p, a, 75, 45); c != greenP {
		t.Errorf("device pixel 150,90 %v, want %v (frame not at full resolution?)", c, greenP)
	}
	if c := readArea(p, a, 85, 55); c != redPx {
		t.Errorf("device pixel 170,110 %v, want %v", c, redPx)
	}
}

// TestPresenterClose: Close is idempotent, disconnects the render
// handler, and afterwards present and Render fail with ErrClosed.
func TestPresenterClose(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)
	handler := p.handler
	var errs [2]error
	connected := int32(1)
	onGTK(func() {
		c := p.r.(interface{ Close() error })
		errs[0] = c.Close()
		errs[1] = c.Close()
		connected = gtkGL.isConnected(a.area, handler)
	})
	for i, err := range errs {
		if err != nil {
			t.Errorf("Close %d: %v", i+1, err)
		}
	}
	if !p.closeDeleted {
		t.Error("Close on a live GLArea did not delete the objects with its context current")
	}
	if connected != 0 {
		t.Error("the render handler is still connected after Close")
	}
	var perr error
	onGTK(func() { perr = p.present(a.dev(160, 100), 1) })
	if !errors.Is(perr, ErrClosed) {
		t.Errorf("present after Close: %v, want ErrClosed", perr)
	}
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); !errors.Is(err, ErrClosed) {
		t.Errorf("Render after Close: %v, want ErrClosed", err)
	}
	// GTK keeps drawing the area: no callback reaches the closed presenter.
	pump(func() bool { return true })
}

// TestPresenterBadArea: no GLArea, or a widget that is not one, is an
// error.
func TestPresenterBadArea(t *testing.T) {
	if _, _, err := NewPresenter(0); err == nil {
		t.Fatal("NewPresenter(0) succeeded")
	}
	a := newTestArea(t, 160, 100)
	var err error
	onGTK(func() { _, _, err = NewPresenter(a.win) })
	if err == nil || !strings.Contains(err.Error(), "GtkGLArea") {
		t.Fatalf("NewPresenter(GtkWindow): %v", err)
	}
}

// TestPresenterSharedState: GL state GTK (or anyone sharing the context)
// may leave behind does not change the frame: pixel buffer bindings,
// skip offsets, rasterizer discard, alpha to coverage, polygon mode, a
// scissor box.
func TestPresenterSharedState(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	var buf uint32
	onGTK(func() {
		gtkGL.makeCurrent(a.area)
		f := p.f
		f.GenBuffers(1, &buf)
		f.BindBuffer(glPIXEL_UNPACK_BUFFER, buf)
		f.BufferData(glPIXEL_UNPACK_BUFFER, 4096, nil, glSTREAM_DRAW)
		f.BindBuffer(glPIXEL_PACK_BUFFER, buf)
		f.PixelStorei(glUNPACK_SKIP_PIXELS, 3)
		f.PixelStorei(glUNPACK_SKIP_ROWS, 2)
		f.PixelStorei(glPACK_SKIP_PIXELS, 3)
		f.PixelStorei(glPACK_SKIP_ROWS, 2)
		f.Enable(glRASTERIZER_DISCARD)
		f.Enable(glSAMPLE_ALPHA_TO_COVERAGE)
		if p.polygonMode != nil {
			p.polygonMode(glFRONT_AND_BACK, glLINE)
		}
	})
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)
	if c := readArea(p, a, 80, 50); c != redPx {
		t.Errorf("center pixel %v, want %v", c, redPx)
	}
	if c := readArea(p, a, 40, 25); c != greenP {
		t.Errorf("fill pixel %v, want %v", c, greenP)
	}
	// On the GTK thread like every presenter call: a GLX context made
	// current on another thread and left there makes later headless EGL
	// contexts fail eglMakeCurrent with EGL_BAD_ACCESS under libglvnd.
	var (
		img *image.RGBA
		err error
	)
	onGTK(func() { img, err = p.dev.ReadPixels() })
	if err != nil {
		t.Fatal(err)
	}
	if c := img.RGBAAt(40*a.scale, 25*a.scale); c != greenP {
		t.Errorf("ReadPixels: fill pixel %v, want %v", c, greenP)
	}
	onGTK(func() {
		gtkGL.makeCurrent(a.area)
		p.f.DeleteBuffers(1, &buf)
	})
}

// TestPresenterAreaDestroyed: the surface destroyed its GLArea before
// the presenter was closed (not the platform's order, but must not
// crash): Render reports a lost device, Close still succeeds.
func TestPresenterAreaDestroyed(t *testing.T) {
	a := newTestArea(t, 160, 100)
	var (
		p   *presenter
		err error
	)
	onGTK(func() { p, err = newPresenter(a.area) })
	if errors.Is(err, ErrNoDevice) {
		t.Skipf("no GL device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	a.close()
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); !errors.Is(err, gpu.ErrDeviceLost) {
		t.Errorf("Render on a destroyed GLArea: %v, want ErrDeviceLost", err)
	}
	var perr error
	onGTK(func() { perr = p.present(a.dev(160, 100), 1) })
	if !errors.Is(perr, gpu.ErrDeviceLost) {
		t.Errorf("present on a destroyed GLArea: %v, want ErrDeviceLost", perr)
	}
	var cerr error
	onGTK(func() { cerr = p.r.(interface{ Close() error }).Close() })
	if cerr != nil {
		t.Errorf("Close: %v", cerr)
	}
	if p.closeDeleted {
		t.Error("Close on a destroyed GLArea made a context current")
	}
}

// TestPresenterContextChanged: the GLArea unrealized (its context
// finalized) and realized again with a new context loses the device: no
// Device object is used in the new context, and Close does not delete
// names in it.
func TestPresenterContextChanged(t *testing.T) {
	a := newTestArea(t, 160, 100)
	var (
		p   *presenter
		err error
	)
	onGTK(func() { p, err = newPresenter(a.area) })
	if errors.Is(err, ErrNoDevice) {
		t.Skipf("no GL device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); err != nil {
		t.Fatal(err)
	}
	presentAndWait(t, p, a, 160, 100)
	var dead bool
	var oldCtx, newCtx uintptr
	onGTK(func() {
		oldCtx = p.ctx
		gtkTest.unrealize(a.area)
		dead = p.ctxDead
		gtkTest.realize(a.area)
		newCtx = gtkGL.getContext(a.area)
	})
	if !dead {
		t.Error("the weak notify did not run when the GLArea dropped its context")
	}
	if newCtx == 0 {
		t.Skip("the GLArea has no context after realizing it again")
	}
	t.Logf("context %#x, after unrealize/realize %#x", oldCtx, newCtx)
	if err := renderSolid(p, a, geom.Size{W: 160, H: 100}, red, green); !errors.Is(err, gpu.ErrDeviceLost) {
		t.Errorf("Render after the context changed: %v, want ErrDeviceLost", err)
	}
	var perr error
	onGTK(func() { perr = p.present(a.dev(160, 100), 1) })
	if !errors.Is(perr, gpu.ErrDeviceLost) {
		t.Errorf("present after the context changed: %v, want ErrDeviceLost", perr)
	}
	onGTK(p.Close)
	if p.closeDeleted {
		t.Error("Close deleted names in the area's new context")
	}
	pump(func() bool { return true })
}

// TestPresenterGLRenderer: the renderer reports the context's GL_RENDERER.
func TestPresenterGLRenderer(t *testing.T) {
	a := newTestArea(t, 160, 100)
	p := startTestPresenter(t, a)
	var want string
	onGTK(func() {
		gtkGL.makeCurrent(a.area)
		want = p.f.GetString(glRENDERER)
	})
	got := p.r.(interface{ GLRenderer() string }).GLRenderer()
	if got == "" || got != want {
		t.Fatalf("GLRenderer() = %q, want %q", got, want)
	}
	t.Logf("GL_RENDERER %q", got)
}
