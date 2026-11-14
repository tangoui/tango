//go:build linux && !android

package gl

// The close order on the real Linux canvas: a canvas surface of the Linux
// backend (internal/platform/linux), switched to its GtkGLArea
// (platform.CanvasGL) with a presenter on it, closed the way a window
// closes (Surface.Close: gtk_widget_destroy of the toplevel). uihost
// closes the renderer when SurfaceClosed arrives; the test does the same
// from the host's OnSurfaceEvent and records which path Close took.
//
// The backend runs gtk_main, so it gets a process of its own: the parent
// test re-runs the test binary with presenterCloseEnv set, and the child
// runs the backend on its (locked) test goroutine.

import (
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/linux"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// presenterCloseEnv marks the child of TestPresenterSurfaceClose.
const presenterCloseEnv = "TANGO_GL_PRESENTER_CLOSE_SUBPROCESS"

// TestPresenterSurfaceClose: closing a window whose canvas presents with
// the GL presenter. GTK 3.24 unrealizes the toplevel (and so the GLArea,
// dropping its GdkGLContext) before it emits "destroy", so when
// SurfaceClosed arrives the area is unrealized and its context finalized:
// Close issues no GL call and skips the deletes (the objects went with the
// context). The child asserts exactly that and prints what it saw.
func TestPresenterSurfaceClose(t *testing.T) {
	if os.Getenv(presenterCloseEnv) != "" {
		t.Skip("in the subprocess")
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no DISPLAY / WAYLAND_DISPLAY")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestPresenterSurfaceCloseChild$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), presenterCloseEnv+"=1")
	out, err := cmd.CombinedOutput()
	t.Logf("subprocess:\n%s", out)
	if err != nil {
		t.Fatalf("subprocess: %v", err)
	}
	if strings.Contains(string(out), "--- SKIP: TestPresenterSurfaceCloseChild") {
		t.Skip("subprocess skipped")
	}
	if !strings.Contains(string(out), "--- PASS: TestPresenterSurfaceCloseChild") {
		t.Fatal("subprocess did not run TestPresenterSurfaceCloseChild")
	}
}

// closeHost is a minimal platform.Host: Drain runs queued work on the
// GTK thread; SurfaceClosed calls onClosed there.
type closeHost struct {
	mu       sync.Mutex
	queue    []func()
	onClosed func()
}

func (h *closeHost) Drain() {
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

func (h *closeHost) OnLifecycle(platform.LifecycleEvent) {}
func (h *closeHost) ShouldClose(platform.SurfaceID) bool { return true }
func (h *closeHost) OnSurfaceEvent(_ platform.SurfaceID, ev platform.SurfaceEvent) {
	if ev.Kind == platform.SurfaceClosed && h.onClosed != nil {
		h.onClosed()
	}
}
func (h *closeHost) OnMessage(platform.SurfaceID, string, string) {}
func (h *closeHost) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, _ *http.Request) {
	http.NotFound(w, nil)
}

// post queues fn for the GTK thread.
func (h *closeHost) post(b platform.Backend, fn func()) {
	h.mu.Lock()
	h.queue = append(h.queue, fn)
	h.mu.Unlock()
	b.Wake()
}

// do runs fn on the GTK thread and waits for it.
func (h *closeHost) do(b platform.Backend, fn func()) {
	done := make(chan struct{})
	h.post(b, func() { defer close(done); fn() })
	<-done
}

// closeSink renders a solid frame with the presenter on every
// CanvasFrame (GTK thread).
type closeSink struct {
	cv     platform.Canvas
	p      *presenter
	frames int
	err    error
}

func (s *closeSink) CanvasFrame() {
	s.frames++
	if s.p == nil {
		return
	}
	w, h := s.cv.Size()
	scale := s.cv.Scale()
	l := &paint.List{Background: red}
	l.Fill(geom.Rect{W: w / 2, H: h / 2}, geom.Radii{}, green)
	if err := s.p.r.Render(l, geom.Size{W: w, H: h}, scale); err != nil {
		s.err = err
		return
	}
	px := image.Pt(int(w*scale+0.5), int(h*scale+0.5))
	if err := s.p.present(px, scale); err != nil {
		s.err = err
		return
	}
	s.err = s.cv.Present(nil)
}
func (s *closeSink) CanvasInput(input.Event)           {}
func (s *closeSink) CanvasResized(w, h, scale float32) {}
func (s *closeSink) CanvasFocus(bool)                  {}

// closeResult is what the child saw.
type closeResult struct {
	skip                          string // why the child skips
	err                           error
	rendered                      int  // render handlers before the close
	closedEvents                  int  // SurfaceClosed events
	realizedAtClosed, ctxAtClosed bool // the GLArea when SurfaceClosed arrived
	ctxDeadAtClosed               bool // the weak notify had run (context finalized)
	deleted                       bool // Close deleted the objects with the context current
	closedAfter                   bool
}

func TestPresenterSurfaceCloseChild(t *testing.T) {
	if os.Getenv(presenterCloseEnv) == "" {
		t.Skip("only in TestPresenterSurfaceClose's subprocess")
	}
	runtime.LockOSThread() // GTK's thread: Init and Run below
	h := &closeHost{}
	b := linux.New()
	if err := b.Init(h, platform.AppOptions{Name: "tango-gl-close-test"}); err != nil {
		t.Skipf("no GTK display: %v", err)
	}
	res := make(chan closeResult, 1)
	go func() {
		r := driveSurfaceClose(b, h)
		res <- r
		h.post(b, b.Quit)
	}()
	b.Run() //nolint:errcheck // returns when Quit runs
	r := <-res
	if r.skip != "" {
		t.Skip(r.skip)
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	fmt.Printf("CLOSE-ORDER rendered=%d closedEvents=%d realizedAtSurfaceClosed=%v contextAtSurfaceClosed=%v contextFinalizedAtSurfaceClosed=%v closeDeletedWithContext=%v\n",
		r.rendered, r.closedEvents, r.realizedAtClosed, r.ctxAtClosed, r.ctxDeadAtClosed, r.deleted)
	if r.rendered < 1 || r.closedEvents != 1 || !r.closedAfter {
		t.Fatalf("rendered %d, SurfaceClosed %d, presenter closed %v", r.rendered, r.closedEvents, r.closedAfter)
	}
	// GTK 3.24: dispose unrealizes before "destroy".
	if r.realizedAtClosed || r.ctxAtClosed || !r.ctxDeadAtClosed {
		t.Errorf("at SurfaceClosed the GLArea was realized %v, had a context %v, its context finalized %v; want false, false, true",
			r.realizedAtClosed, r.ctxAtClosed, r.ctxDeadAtClosed)
	}
	if r.deleted {
		t.Error("Close made a context current and deleted the objects; want the skipped path")
	}
}

// driveSurfaceClose opens the canvas surface, presents a frame through the
// GLArea and closes the surface; on a goroutine other than GTK's.
func driveSurfaceClose(b platform.Backend, h *closeHost) (r closeResult) {
	sink := &closeSink{}
	var (
		s    platform.Surface
		area uintptr
	)
	h.do(b, func() {
		cc, ok := b.(platform.CanvasCreator)
		if !ok {
			r.skip = "the Linux backend has no canvas surfaces"
			return
		}
		var cv platform.Canvas
		var err error
		s, cv, err = cc.NewCanvasSurface(1, platform.SurfaceOptions{
			Title: "gl close", Width: 160, Height: 100, Visible: true,
		}, platform.SurfaceExtras{}, sink)
		if err != nil {
			r.err = fmt.Errorf("NewCanvasSurface: %w", err)
			return
		}
		sink.cv = cv
		cg, ok := cv.(platform.CanvasGL)
		if !ok {
			r.skip = "the canvas has no CanvasGL"
			return
		}
		if area, err = cg.UseGLArea(); err != nil {
			r.skip = fmt.Sprintf("UseGLArea: %v", err)
			return
		}
		p, err := newPresenter(area)
		if errors.Is(err, ErrNoDevice) {
			r.skip = fmt.Sprintf("no GL device: %v", err)
			return
		}
		if err != nil {
			r.err = fmt.Errorf("newPresenter: %w", err)
			return
		}
		sink.p = p
		h.onClosed = func() {
			r.closedEvents++
			r.realizedAtClosed = gtkGL.getRealized(area) != 0
			r.ctxAtClosed = gtkGL.getContext(area) != 0
			r.ctxDeadAtClosed = p.ctxDead
			p.Close() // what uihost does on SurfaceClosed
			r.deleted = p.closeDeleted
			r.closedAfter = p.closed
		}
	})
	if r.skip != "" || r.err != nil {
		if s != nil {
			h.do(b, s.Close)
		}
		return r
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		var err error
		h.do(b, func() { n, err = sink.p.rendered, sink.err })
		if err != nil {
			r.err = fmt.Errorf("frame: %w", err)
			h.do(b, s.Close)
			return r
		}
		if n > 0 {
			r.rendered = n
			break
		}
		if time.Now().After(deadline) {
			r.err = errors.New("the GLArea's render signal did not run")
			h.do(b, s.Close)
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.do(b, s.Close) // gtk_widget_destroy of the toplevel; SurfaceClosed from its "destroy"
	// Let GTK run on: nothing may reach the closed presenter or its context.
	for i := 0; i < 10; i++ {
		h.do(b, func() {})
		time.Sleep(10 * time.Millisecond)
	}
	return r
}
