//go:build linux && !android

package linux

// Window tests of platform.CanvasGL (canvas_gl_linux.go) on a real GTK.
// They need a display: run them under Xvfb, e.g.
//
//	xvfb-run -a -s "-screen 0 1280x800x24" go test ./internal/platform/linux/ -run GLArea
//
// Without DISPLAY / WAYLAND_DISPLAY they skip. TestMain initializes the
// backend on the main thread and runs gtk_main there; the tests run on
// other goroutines and reach GTK through onMain (Wake + Drain).

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() } // TestMain runs GTK on the main thread

// glTestEnv is the GL subprocess marker (TestUseGLAreaUnavailable).
const glTestEnv = "TANGO_GLAREA_SUBPROCESS"

var gtkTest struct {
	b    *backend
	host *gtkTestHost
	err  error // why window tests skip
}

type gtkTestHost struct {
	mu    sync.Mutex
	queue []func()
}

func (h *gtkTestHost) Drain() {
	for {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

func (h *gtkTestHost) OnLifecycle(platform.LifecycleEvent)                      {}
func (h *gtkTestHost) ShouldClose(platform.SurfaceID) bool                      { return true }
func (h *gtkTestHost) OnSurfaceEvent(platform.SurfaceID, platform.SurfaceEvent) {}
func (h *gtkTestHost) OnMessage(platform.SurfaceID, string, string)             {}
func (h *gtkTestHost) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, _ *http.Request) {
	http.NotFound(w, nil)
}

func TestMain(m *testing.M) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		gtkTest.err = errors.New("no DISPLAY / WAYLAND_DISPLAY")
		os.Exit(m.Run())
	}
	h := &gtkTestHost{}
	b := New().(*backend)
	if err := b.Init(h, platform.AppOptions{Name: "tango-linux-test"}); err != nil {
		gtkTest.err = err
		os.Exit(m.Run())
	}
	gtkTest.b, gtkTest.host = b, h
	code := make(chan int, 1)
	go func() {
		c := m.Run()
		code <- c
		h.mu.Lock()
		h.queue = append(h.queue, b.Quit)
		h.mu.Unlock()
		b.Wake()
	}()
	b.Run() //nolint:errcheck // returns when Quit runs
	os.Exit(<-code)
}

// needGTK skips t without a usable display.
func needGTK(t *testing.T) {
	t.Helper()
	if gtkTest.b == nil {
		t.Skipf("no GTK display: %v", gtkTest.err)
	}
}

// onMain runs fn on the GTK main thread and waits for it.
func onMain(fn func()) {
	done := make(chan struct{})
	h := gtkTest.host
	h.mu.Lock()
	h.queue = append(h.queue, func() { defer close(done); fn() })
	h.mu.Unlock()
	gtkTest.b.Wake()
	<-done
}

// waitMain polls cond on the main thread until it holds or d passes.
func waitMain(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(d)
	for {
		var ok bool
		onMain(func() { ok = cond() })
		if ok {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// glSink records canvas callbacks; render is called (main thread) for
// each CanvasFrame.
type glSink struct {
	mu       sync.Mutex
	frames   int
	pointers []input.Pointer
	render   func()
}

func (s *glSink) CanvasFrame() {
	s.mu.Lock()
	s.frames++
	r := s.render
	s.mu.Unlock()
	if r != nil {
		r()
	}
}

func (s *glSink) CanvasInput(ev input.Event) {
	if p, ok := ev.(input.Pointer); ok {
		s.mu.Lock()
		s.pointers = append(s.pointers, p)
		s.mu.Unlock()
	}
}
func (s *glSink) CanvasResized(w, h, scale float32) {}
func (s *glSink) CanvasFocus(bool)                  {}

func (s *glSink) frameCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames
}

func (s *glSink) presses() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, p := range s.pointers {
		if p.Kind == input.Press {
			n++
		}
	}
	return n
}

var glSurfaceID platform.SurfaceID = 100

// newGLTestCanvas opens a visible 320×240 canvas surface whose frames are
// solid CPU images until the test changes sink.render.
func newGLTestCanvas(t *testing.T) (platform.Surface, *canvas, *glSink) {
	t.Helper()
	return newGLTestCanvasEx(t, platform.SurfaceExtras{})
}

// newGLTestCanvasEx is newGLTestCanvas with surface extras.
func newGLTestCanvasEx(t *testing.T, ex platform.SurfaceExtras) (platform.Surface, *canvas, *glSink) {
	t.Helper()
	sink := &glSink{}
	var (
		s   platform.Surface
		cv  platform.Canvas
		err error
	)
	onMain(func() {
		glSurfaceID++
		s, cv, err = gtkTest.b.NewCanvasSurface(glSurfaceID, platform.SurfaceOptions{
			Title: "glarea", Width: 320, Height: 240, Visible: true, Resizable: true,
		}, ex, sink)
	})
	if err != nil {
		t.Fatalf("NewCanvasSurface: %v", err)
	}
	c := cv.(*canvas)
	sink.render = func() { c.Present(solidImage(c, color.RGBA{0, 0, 255, 255})) } //nolint:errcheck
	t.Cleanup(func() { onMain(s.Close) })
	if !waitMain(5*time.Second, func() bool { return sink.frames > 0 }) {
		t.Fatal("no first frame")
	}
	return s, c, sink
}

// solidImage is a frame of the canvas's device-pixel size filled with col.
func solidImage(c *canvas, col color.RGBA) *image.RGBA {
	w, h := int(c.w*c.scale+0.5), int(c.h*c.scale+0.5)
	img := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = col.R, col.G, col.B, col.A
	}
	return img
}

func TestUseGLArea(t *testing.T) {
	needGTK(t)
	s, c, sink := newGLTestCanvas(t)
	var (
		a1, a2        uintptr
		err1, err2    error
		realized, vis bool
		autoRender    bool
		alpha, depth  bool
		stencil       bool
		before        int
	)
	onMain(func() {
		before = sink.frames
		a1, err1 = c.UseGLArea()
		a2, err2 = c.UseGLArea()
		if a1 != 0 && loadGLTestFFI() == nil {
			f := &glTestFFI
			realized = canvasFFI.widgetGetRealized(a1)
			vis = f.widgetGetVisible(a1)
			autoRender = f.glAreaGetAutoRender(a1)
			alpha = f.glAreaGetHasAlpha(a1)
			depth = f.glAreaGetHasDepth(a1)
			stencil = f.glAreaGetHasStencil(a1)
		}
	})
	if err1 != nil || a1 == 0 {
		t.Fatalf("UseGLArea: %#x, %v", a1, err1)
	}
	if err2 != nil || a2 != a1 {
		t.Fatalf("second UseGLArea: %#x, %v; want %#x", a2, err2, a1)
	}
	var major, minor int32
	onMain(func() { canvasGLFFI.glContextGetVersion(canvasGLFFI.glAreaGetContext(a1), &major, &minor) })
	t.Logf("GLArea context: OpenGL %d.%d", major, minor)
	if !realized || !vis {
		t.Fatalf("GLArea realized %v visible %v", realized, vis)
	}
	if autoRender || alpha || depth || stencil {
		t.Fatalf("GLArea auto-render %v alpha %v depth %v stencil %v", autoRender, alpha, depth, stencil)
	}
	// The switch schedules one CanvasFrame without an Invalidate; frames
	// on the GL path present with Present(nil).
	var presentErr error
	sink.mu.Lock()
	sink.render = func() { presentErr = c.Present(nil) }
	sink.mu.Unlock()
	if !waitMain(5*time.Second, func() bool { return sink.frames > before }) {
		t.Fatal("no CanvasFrame after the switch")
	}
	onMain(func() {})
	if presentErr != nil {
		t.Fatalf("Present(nil) on the GL path: %v", presentErr)
	}
	// Invalidate and a resize still produce frames.
	n := sink.frameCount()
	c.Invalidate()
	if !waitMain(5*time.Second, func() bool { return sink.frames > n }) {
		t.Fatal("no CanvasFrame after Invalidate on the GL path")
	}
	n = sink.frameCount()
	onMain(func() { s.SetSize(400, 300) })
	if !waitMain(5*time.Second, func() bool { return sink.frames > n && c.w == 400 && c.h == 300 }) {
		t.Fatalf("no CanvasFrame after a resize on the GL path (size %vx%v)", c.w, c.h)
	}
	// After the surface closed: an error, not the old area.
	onMain(s.Close)
	var err error
	onMain(func() { _, err = c.UseGLArea() })
	if err == nil {
		t.Fatal("UseGLArea after close: no error")
	}
}

func TestUseGLAreaTransparent(t *testing.T) {
	needGTK(t)
	s, c, _ := newGLTestCanvas(t)
	var err error
	onMain(func() {
		s.(platform.Styled).SetAppearance(platform.Appearance{Transparent: true})
		_, err = c.UseGLArea()
	})
	if !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("transparent canvas: err = %v, want ErrUnsupported", err)
	}
}

// TestUseGLAreaUnavailable runs itself again with GDK_GL=disable (GDK
// reads it once, at gdk_init): GTK then cannot create a GL context, so
// UseGLArea must fail and the canvas keep presenting with cairo.
func TestUseGLAreaUnavailable(t *testing.T) {
	needGTK(t)
	if os.Getenv(glTestEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestUseGLAreaUnavailable$", "-test.v")
		cmd.Env = append(os.Environ(), glTestEnv+"=1", "GDK_GL=disable")
		out, err := cmd.CombinedOutput()
		t.Logf("subprocess:\n%s", out)
		if err != nil {
			t.Fatalf("subprocess: %v", err)
		}
		return
	}
	_, c, sink := newGLTestCanvas(t)
	var (
		area     uintptr
		err      error
		presErr  error
		children int
	)
	onMain(func() {
		area, err = c.UseGLArea()
		presErr = c.Present(solidImage(c, color.RGBA{255, 0, 0, 255}))
		children = canvasGLTestOverlayChildren(c)
	})
	if err == nil || area != 0 {
		t.Fatalf("UseGLArea with GDK_GL=disable: %#x, %v; want an error", area, err)
	}
	t.Logf("UseGLArea error: %v", err)
	if errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("GL failure reported as ErrUnsupported: %v", err)
	}
	if presErr != nil {
		t.Fatalf("Present(img) after the failed switch: %v", presErr)
	}
	if c.gl.on || c.glArea != 0 || children != 1 {
		t.Fatalf("canvas left switched: on %v area %#x overlay children %d", c.gl.on, c.glArea, children)
	}
	// Still the cairo path: Present(nil) is refused, frames keep coming.
	onMain(func() { presErr = c.Present(nil) })
	if presErr == nil {
		t.Fatal("Present(nil) accepted on the cairo path")
	}
	n := sink.frameCount()
	c.Invalidate()
	if !waitMain(5*time.Second, func() bool { return sink.frames > n }) {
		t.Fatal("no frame after the failed switch")
	}
}

// TestGLAreaInputPassThrough clicks through the X server (XTest), so
// GDK's own window picking decides who gets the press: with the GLArea's
// overlay child window pass-through, the drawing area below must.
// (gdk_test_simulate_button sends synthetic events that GDK 3.24 does not
// deliver as presses under Xvfb; the attach smoke test sees the same.)
func TestGLAreaInputPassThrough(t *testing.T) {
	needGTK(t)
	if err := loadAttachSmoke(); err != nil {
		t.Skipf("smoke hooks: %v", err)
	}
	var x11 bool
	onMain(func() { x11 = attachSmokeX11() })
	if !x11 {
		t.Skip("XTest needs X11")
	}
	if err := loadXTest(); err != nil {
		t.Skipf("XTest: %v", err)
	}
	_, c, sink := newGLTestCanvas(t)
	gpuFrames(c, sink)
	var err error
	onMain(func() { _, err = c.UseGLArea() })
	if err != nil {
		t.Fatalf("UseGLArea: %v", err)
	}
	if !waitMain(time.Second, func() bool {
		return glTestFFI.widgetGetMapped(c.glArea) && glTestFFI.widgetGetVisible(c.glArea)
	}) {
		t.Fatal("GLArea not mapped")
	}
	onMain(func() { err = xtestClick(c, 100, 80) })
	if err != nil {
		t.Fatal(err)
	}
	if !waitMain(3*time.Second, func() bool { return sink.presses() > 0 }) {
		t.Fatal("a click over the GLArea did not reach the canvas sink")
	}
	sink.mu.Lock()
	var p input.Pointer
	for _, q := range sink.pointers {
		if q.Kind == input.Press {
			p = q
			break
		}
	}
	sink.mu.Unlock()
	if p.Pos.X < 99 || p.Pos.X > 101 || p.Pos.Y < 79 || p.Pos.Y > 81 {
		t.Fatalf("press at %v, want (100, 80)", p.Pos)
	}
}

// gpuFrames makes the canvas's frames GPU frames (Present(nil) once
// switched), so the scheduled frames do not hide the GLArea.
func gpuFrames(c *canvas, sink *glSink) {
	sink.mu.Lock()
	sink.render = func() {
		if c.gl.on {
			c.Present(nil) //nolint:errcheck
		} else {
			c.Present(solidImage(c, color.RGBA{0, 0, 255, 255})) //nolint:errcheck
		}
	}
	sink.mu.Unlock()
}

// XTest, through GDK's own X connection.
var xtest struct {
	once sync.Once
	err  error

	xdisplay   func(gdkDisplay ptr) ptr
	fakeMotion func(dpy ptr, screen int32, x, y int32, delay uint64) int32
	fakeButton func(dpy ptr, button uint32, press bool, delay uint64) int32
	flush      func(dpy ptr) int32
}

func loadXTest() error {
	x := &xtest
	x.once.Do(func() {
		xtst, _, err := open("libXtst.so.6")
		if err != nil {
			x.err = err
			return
		}
		x11, _, err := open("libX11.so.6")
		if err != nil {
			x.err = err
			return
		}
		gdk, _, _ := open("libgdk-3.so.0")
		var bd binder
		bd.req(&x.xdisplay, "gdk_x11_display_get_xdisplay", gdk, libGTK)
		bd.req(&x.fakeMotion, "XTestFakeMotionEvent", xtst)
		bd.req(&x.fakeButton, "XTestFakeButtonEvent", xtst)
		bd.req(&x.flush, "XFlush", x11)
		if len(bd.missing) > 0 {
			x.err = fmt.Errorf("missing %v", bd.missing)
		}
	})
	return x.err
}

// xtestClick moves the pointer to canvas point (x, y) (logical) and
// clicks button 1 through the X server. Main thread.
func xtestClick(c *canvas, x, y int32) error {
	win, wx, wy, err := canvasGLTestWindowPoint(c, x, y)
	if err != nil {
		return err
	}
	var ox, oy int32
	attachSmoke.windowGetOrigin(win, &ox, &oy)
	dpy := xtest.xdisplay(attachSmoke.displayDefault())
	if dpy == 0 {
		return errors.New("no X display")
	}
	sc := max(canvasFFI.widgetGetScaleFactor(c.widget), 1) // GDK coordinates are logical, X's are device pixels
	xtest.fakeMotion(dpy, -1, (ox+wx)*sc, (oy+wy)*sc, 0)
	xtest.fakeButton(dpy, 1, true, 0)
	xtest.fakeButton(dpy, 1, false, 0)
	xtest.flush(dpy)
	return nil
}

func TestGLAreaCPUFallbackToggle(t *testing.T) {
	needGTK(t)
	_, c, sink := newGLTestCanvas(t)
	gpuFrames(c, sink)
	var (
		area uintptr
		err  error
	)
	onMain(func() { area, err = c.UseGLArea() })
	if err != nil {
		t.Fatalf("UseGLArea: %v", err)
	}
	if err := loadGLTestFFI(); err != nil {
		t.Skip(err)
	}
	// Each Present and its check run in one main-thread call: scheduled
	// GPU frames (Present(nil)) may come in between otherwise.
	present := func(cpu, damage bool) (visible bool, err error) {
		onMain(func() {
			var img *image.RGBA
			if cpu {
				img = solidImage(c, color.RGBA{0, 255, 0, 255})
			}
			if damage {
				err = c.PresentDamage(img, []image.Rectangle{image.Rect(0, 0, 4, 4)})
			} else {
				err = c.Present(img)
			}
			visible = glTestFFI.widgetGetVisible(area)
			if cpu && (c.surfW != img.Rect.Dx() || c.surfH != img.Rect.Dy()) {
				err = fmt.Errorf("cairo surface %dx%d, frame %v", c.surfW, c.surfH, img.Rect.Size())
			}
		})
		return visible, err
	}
	steps := []struct {
		cpu, damage bool
		visible     bool
	}{
		{false, false, true}, // GPU frame
		{true, false, false}, // CPU fallback hides the GLArea
		{true, false, false},
		{false, false, true}, // the GPU presented again
		{true, false, false},
		// PresentDamage presents whole frames on the GL path.
		{false, true, true},
		{true, true, false},
		{false, true, true},
	}
	for i, st := range steps {
		v, err := present(st.cpu, st.damage)
		if err != nil {
			t.Fatalf("step %d: %+v: %v", i, st, err)
		}
		if v != st.visible {
			t.Fatalf("step %d: %+v: GLArea visible %v", i, st, v)
		}
	}
}

// canvasGLTestWindowPoint converts canvas point (x, y) to coordinates in
// the toplevel's GdkWindow.
func canvasGLTestWindowPoint(c *canvas, x, y int32) (ptr, int32, int32, error) {
	if err := loadGLTestFFI(); err != nil {
		return 0, 0, 0, err
	}
	top := glTestFFI.widgetGetToplevel(c.widget)
	win := canvasFFI.widgetGetWindow(top)
	if win == 0 {
		return 0, 0, 0, errors.New("toplevel not realized")
	}
	var wx, wy int32
	if !attachSmoke.translate(c.widget, top, x, y, &wx, &wy) {
		return 0, 0, 0, fmt.Errorf("gtk_widget_translate_coordinates failed")
	}
	return win, wx, wy, nil
}

// TestGLAreaFileDropTarget: with AcceptFiles, a file dragged over the
// GLArea still reaches the canvas. GTK resolves the drop target by walking
// up from the widget under the pointer, which is the GLArea once it covers
// the drawing area, so the GLArea or one of its ancestors must be the drag
// destination (the drawing area, a sibling, is never asked). Before the
// fix the destination was the drawing area: a synthesized XDND drag over a
// GPU window got XdndStatus "not accepted" and no highlight (Task 7 of GPU
// milestone 4).
func TestGLAreaFileDropTarget(t *testing.T) {
	needGTK(t)
	if err := loadGLTestFFI(); err != nil {
		t.Skip(err)
	}
	_, c, _ := newGLTestCanvasEx(t, platform.SurfaceExtras{AcceptFiles: true})
	var (
		area ptr
		err  error
		dest []ptr
	)
	onMain(func() {
		area, err = c.UseGLArea()
		if err != nil {
			return
		}
		for w := area; w != 0; w = glTestFFI.widgetGetParent(w) {
			if glTestFFI.dragDestGetTargetList(w) != 0 {
				dest = append(dest, w)
			}
		}
	})
	if err != nil {
		t.Skipf("UseGLArea: %v (no GL 3.3 here)", err)
	}
	if len(dest) == 0 {
		t.Fatal("neither the GLArea nor an ancestor is a drag destination: drops over a GPU canvas are refused")
	}
	if dest[0] != c.glOverlay {
		t.Errorf("drag destination %#x, want the canvas overlay %#x", dest[0], c.glOverlay)
	}
}

// canvasGLTestOverlayChildren counts the children of the canvas's overlay.
func canvasGLTestOverlayChildren(c *canvas) int {
	if c.glOverlay == 0 {
		return 0
	}
	if loadGLTestFFI() != nil {
		return -1
	}
	f := &glTestFFI
	l := f.containerGetChildren(c.glOverlay)
	n := int(f.listLength(l))
	f.listFree(l)
	return n
}

// glTestFFI are GTK getters only the tests need.
var glTestFFI struct {
	once sync.Once
	err  error

	widgetGetVisible      func(w ptr) bool
	widgetGetMapped       func(w ptr) bool
	widgetGetToplevel     func(w ptr) ptr
	glAreaGetAutoRender   func(a ptr) bool
	glAreaGetHasAlpha     func(a ptr) bool
	glAreaGetHasDepth     func(a ptr) bool
	glAreaGetHasStencil   func(a ptr) bool
	containerGetChildren  func(c ptr) ptr
	widgetGetParent       func(w ptr) ptr
	dragDestGetTargetList func(w ptr) ptr
	listLength            func(l ptr) uint32
	listFree              func(l ptr)
}

func loadGLTestFFI() error {
	f := &glTestFFI
	f.once.Do(func() {
		var bd binder
		t, g := libGTK, libGLib
		bd.req(&f.widgetGetVisible, "gtk_widget_get_visible", t)
		bd.req(&f.widgetGetMapped, "gtk_widget_get_mapped", t)
		bd.req(&f.widgetGetToplevel, "gtk_widget_get_toplevel", t)
		bd.req(&f.glAreaGetAutoRender, "gtk_gl_area_get_auto_render", t)
		bd.req(&f.glAreaGetHasAlpha, "gtk_gl_area_get_has_alpha", t)
		bd.req(&f.glAreaGetHasDepth, "gtk_gl_area_get_has_depth_buffer", t)
		bd.req(&f.glAreaGetHasStencil, "gtk_gl_area_get_has_stencil_buffer", t)
		bd.req(&f.containerGetChildren, "gtk_container_get_children", t)
		bd.req(&f.widgetGetParent, "gtk_widget_get_parent", t)
		bd.req(&f.dragDestGetTargetList, "gtk_drag_dest_get_target_list", t)
		bd.req(&f.listLength, "g_list_length", g)
		bd.req(&f.listFree, "g_list_free", g)
		if len(bd.missing) > 0 {
			f.err = fmt.Errorf("test symbols missing: %v", bd.missing)
		}
	})
	return f.err
}

// TestReleaseGLArea: ReleaseGLArea (the presenter could not start, or was
// rejected) destroys the GLArea and returns the canvas to the plain cairo
// path: CPU frames from the draw handler, Present(nil) an error again,
// partial presents, no second switch.
func TestReleaseGLArea(t *testing.T) {
	needGTK(t)
	_, c, sink := newGLTestCanvas(t)
	var (
		area     ptr
		err      error
		children int
		on       bool
		before   int
	)
	onMain(func() {
		c.ReleaseGLArea() // before UseGLArea: nothing to do
		area, err = c.UseGLArea()
		if err != nil {
			return
		}
		canvasFFI.objectRefSink(area) // not floating: a plain reference, to look at it after the release
		before = sink.frames
		c.ReleaseGLArea()
		c.ReleaseGLArea() // idempotent
		children = canvasGLTestOverlayChildren(c)
		on = c.gl.on
	})
	if err != nil {
		t.Skipf("UseGLArea: %v", err)
	}
	var realized bool
	onMain(func() {
		realized = canvasFFI.widgetGetRealized(area)
		gObjectUnref(area)
	})
	if on || c.glArea != 0 || children != 1 || realized {
		t.Fatalf("after ReleaseGLArea: gl.on %v, glArea %#x, overlay children %d (want 1), area realized %v", on, c.glArea, children, realized)
	}
	if !waitMain(5*time.Second, func() bool { return sink.frames > before }) {
		t.Fatal("no CanvasFrame after ReleaseGLArea")
	}
	var nilErr, again error
	var partial bool
	onMain(func() {
		nilErr = c.Present(nil)
		_, again = c.UseGLArea()
		partial = c.gl.partial() && c.gl.drawFrames() && c.gl.paintSurface()
	})
	if nilErr == nil {
		t.Error("Present(nil) after ReleaseGLArea: no error")
	}
	if !errors.Is(again, platform.ErrUnsupported) {
		t.Errorf("UseGLArea after ReleaseGLArea: %v, want ErrUnsupported", again)
	}
	if !partial {
		t.Error("not back on the cairo path (partial presents, draw-handler frames)")
	}
	n := sink.frameCount()
	c.Invalidate()
	if !waitMain(5*time.Second, func() bool { return sink.frames > n }) {
		t.Fatal("no CanvasFrame after Invalidate")
	}
}
