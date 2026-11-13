//go:build linux && !android

// Command smokeattach drives canvases attached to a WebView surface of the
// Linux backend (docs/specs/canvas-surface.md, step D) without the root
// tango package.
//
//	go run ./internal/platform/linux/internal/smokeattach          # interactive
//	go run ./internal/platform/linux/internal/smokeattach -smoke   # self-check, exits 0 on PASS
//
//	xvfb-run -a -s '-screen 0 1280x800x24' ./smokeattach -smoke    # X11 (all checks)
//	weston --backend=headless-backend.so --socket=tango-smoke &     # Wayland (no synthetic
//	WAYLAND_DISPLAY=tango-smoke GDK_BACKEND=wayland ./smokeattach -smoke  #  input: SKIPs)
//
// The page is blue. Canvas A sits above it at (40,30) 200×100 with
// PassThrough: its left half is transparent, its right half opaque red.
// Canvas B fills the window below the WebView and is green.
//
// With -smoke it checks: A's first frame is presented at its bounds ×
// scale; A sits at its bounds in the surface's GtkOverlay after the web
// view, its overlay child is pass-through and it has an input shape; on
// X11 (synthetic input through the X server, routed by GDK): a warp over
// the opaque half reaches A and a warp to the transparent half makes A
// report Leave; a click on the transparent half reaches the page (a JS
// message) and not A, a click on the opaque half reaches A at the right
// position and focuses it, a click on the page takes focus back; a read of
// the window shows red over A's opaque half and the page elsewhere;
// attaching B makes the web view background transparent and the window
// shows B through the transparent page; resizing the window resizes B and
// keeps A at its bounds; SetLayer moves and resizes A (and a click at the
// new place reaches it); Anchor keeps A at its distance from the
// bottom-right corner on resize; detaching B restores the web view
// background and makes B's Present fail; a still window produces no frame
// for 2 s; closing reaches the host and A's Present fails.
//
// On Wayland GDK cannot synthesize input or read the window: those checks
// are logged as SKIP and count as passed.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the overall deadlines.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/linux"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() }

const surfaceID platform.SurfaceID = 1

var smoke = flag.Bool("smoke", false, "run the automatic checks and exit")

const page = `<!doctype html><html><head><style>
html,body{margin:0;height:100%;background:rgb(0,0,255);color:#fff;font:14px sans-serif}
p{position:absolute;left:300px;top:20px;margin:0}
</style></head><body><p>page</p><script>
document.addEventListener('mousedown',e=>__tangoNative.post('down:'+e.clientX+','+e.clientY));
addEventListener('load',()=>__tangoNative.post('loaded'));
</script></body></html>`

var boundsA = platform.Rect{X: 40, Y: 30, Width: 200, Height: 100}

// hooks are the backend's test hooks (canvas_attach_smoke_linux.go).
type hooks interface {
	SmokeAttachState(cv platform.Canvas) (linux.SmokeAttachState, error)
	SmokeAttachMove(cv platform.Canvas, x, y float64) error
	SmokeAttachClick(cv platform.Canvas, x, y float64) error
	SmokeAttachPixel(cv platform.Canvas, x, y float64) (color.RGBA, error)
}

type host struct {
	b     platform.Backend
	hk    hooks
	mu    sync.Mutex
	queue []func()

	start  time.Time
	checks map[string]bool
	failed []string

	surface platform.Surface
	a, bb   *sink
	msgs    []string
	loaded  bool
	x11     bool // synthetic input and window reads work
}

func (h *host) logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(h.start).Seconds(), fmt.Sprintf(format, args...))
}

func (h *host) pass(name, detail string) {
	if !h.checks[name] {
		h.checks[name] = true
		h.logf("PASS %-16s %s", name, detail)
	}
}

func (h *host) fail(name, detail string) {
	h.failed = append(h.failed, name)
	h.logf("FAIL %-16s %s", name, detail)
}

func (h *host) check(name string, ok bool, detail string) {
	if ok {
		h.pass(name, detail)
	} else {
		h.fail(name, detail)
	}
}

func (h *host) post(fn func()) {
	h.mu.Lock()
	h.queue = append(h.queue, fn)
	h.mu.Unlock()
	h.b.Wake()
}

func (h *host) after(d time.Duration, fn func()) {
	go func() { time.Sleep(d); h.post(fn) }()
}

func (h *host) Drain() {
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

func (h *host) OnLifecycle(ev platform.LifecycleEvent) {
	if ev == platform.LifecycleStarted {
		h.onStarted()
	}
}

func (h *host) ShouldClose(platform.SurfaceID) bool { return true }

func (h *host) OnSurfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent) {
	if ev.Kind == platform.SurfaceClosed {
		h.pass("closed", fmt.Sprintf("surface %d", id))
		h.surface = nil
		if !*smoke {
			h.b.Quit()
		}
	}
}

func (h *host) OnMessage(_ platform.SurfaceID, _ string, msg string) {
	h.logf("page: %s", msg)
	if msg == "loaded" && !h.loaded {
		h.loaded = true
		if *smoke {
			h.after(500*time.Millisecond, h.stepLayout)
		}
		return
	}
	h.msgs = append(h.msgs, msg)
}

func (h *host) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// ---- sinks ----

type sink struct {
	h      *host
	name   string
	cv     platform.Canvas
	paint  func(img *image.RGBA)
	frames int
	errs   []error
	events []input.Event
	sizes  [][3]float32
	focus  []bool
}

func (s *sink) CanvasFrame() {
	if s.cv == nil {
		s.h.fail(s.name+"-early", "CanvasFrame before AttachCanvas returned")
		return
	}
	w, ht := s.cv.Size()
	sc := s.cv.Scale()
	img := image.NewRGBA(image.Rect(0, 0, int(math.Round(float64(w*sc))), int(math.Round(float64(ht*sc)))))
	s.paint(img)
	if err := s.cv.Present(img); err != nil {
		s.errs = append(s.errs, err)
	}
	s.frames++
	if s.frames == 1 {
		s.h.check(s.name+"-first", img.Rect.Dx() == int(math.Round(float64(w*sc))),
			fmt.Sprintf("%dx%d px for %gx%g @%g", img.Rect.Dx(), img.Rect.Dy(), w, ht, sc))
	}
}

func (s *sink) CanvasInput(ev input.Event)     { s.events = append(s.events, ev) }
func (s *sink) CanvasResized(w, h, sc float32) { s.sizes = append(s.sizes, [3]float32{w, h, sc}) }
func (s *sink) CanvasFocus(f bool)             { s.focus = append(s.focus, f) }

func (s *sink) pointerKinds() []input.PointerKind {
	var kinds []input.PointerKind
	for _, ev := range s.events {
		if p, ok := ev.(input.Pointer); ok {
			kinds = append(kinds, p.Kind)
		}
	}
	return kinds
}

// paintA: left half transparent, right half opaque red.
func paintA(img *image.RGBA) {
	b := img.Bounds()
	for y := range b.Dy() {
		for x := b.Dx() / 2; x < b.Dx(); x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
}

func paintB(img *image.RGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+1], img.Pix[i+3] = 200, 255
	}
}

// ---- script ----

func (h *host) onStarted() {
	h.hk, _ = h.b.(hooks)
	if _, ok := h.b.(platform.CanvasAttacher); !ok || h.hk == nil {
		h.fail("attacher", "backend lacks CanvasAttacher or smoke hooks")
		h.b.Quit()
		return
	}
	s, err := h.b.NewSurface(surfaceID, platform.SurfaceOptions{
		Title: "Tango attach smoke", Width: 480, Height: 320, Resizable: true, Visible: true,
		URL: h.b.AssetOrigin() + "/index.html",
	})
	if err != nil {
		h.fail("surface", err.Error())
		h.b.Quit()
		return
	}
	h.surface = s
	h.a = &sink{h: h, name: "a", paint: paintA}
	cv, err := h.b.(platform.CanvasAttacher).AttachCanvas(surfaceID, platform.CanvasLayer{Bounds: boundsA, Above: true, PassThrough: true}, h.a)
	if err != nil {
		h.fail("attach", err.Error())
		h.b.Quit()
		return
	}
	h.a.cv = cv
	w, ht := cv.Size()
	h.check("attach", w == 200 && ht == 100, fmt.Sprintf("size %gx%g scale %g", w, ht, cv.Scale()))
	if *smoke {
		h.after(scaled(30*time.Second), func() {
			h.fail("timeout", "script did not finish")
			h.b.Quit()
		})
	}
}

func (h *host) state(cv platform.Canvas) linux.SmokeAttachState {
	st, err := h.hk.SmokeAttachState(cv)
	if err != nil {
		h.fail("state", err.Error())
	}
	return st
}

func (h *host) px(x, y float64) color.RGBA {
	c, err := h.hk.SmokeAttachPixel(h.a.cv, x, y)
	if err != nil {
		h.logf("pixel (%g,%g): %v", x, y, err)
	}
	return c
}

// skip records name as skipped (no synthetic input on this GDK backend).
func (h *host) skip(name string) {
	if !h.checks[name] {
		h.checks[name] = true
		h.logf("SKIP %-16s %v", name, linux.ErrSmokeUnsupported)
	}
}

func (h *host) presses() (n int, first *input.Pointer) {
	for _, ev := range h.a.events {
		if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press {
			if first == nil {
				first = &p
			}
			n++
		}
	}
	return n, first
}

func isRed(c color.RGBA) bool   { return c.R > 200 && c.G < 60 && c.B < 60 }
func isBlue(c color.RGBA) bool  { return c.B > 200 && c.R < 60 && c.G < 60 }
func isGreen(c color.RGBA) bool { return c.G > 150 && c.R < 60 && c.B < 60 }

func (h *host) stepLayout() {
	h.surface.Focus()
	st := h.state(h.a.cv)
	h.check("bounds", st.InOverlay && st.AboveWeb && st.Frame == [4]float64{40, 30, 200, 100},
		fmt.Sprintf("%+v", st))
	h.check("input-shape", st.PassThrough && st.Shape, fmt.Sprintf("pass-through %v shape %v", st.PassThrough, st.Shape))
	h.check("web-background", !st.WebTransparent, "web view keeps its background with only a canvas above")
	h.x11 = st.X11
	if !h.x11 {
		for _, n := range []string{"move-routing", "click-through", "click-canvas", "focus", "shot-above", "focus-web"} {
			h.skip(n)
		}
		h.stepUnderlay()
		return
	}

	h.a.events = nil
	_ = h.hk.SmokeAttachMove(h.a.cv, 170, 80)
	h.after(150*time.Millisecond, func() {
		_ = h.hk.SmokeAttachMove(h.a.cv, 180, 82)
		h.after(150*time.Millisecond, func() {
			_ = h.hk.SmokeAttachMove(h.a.cv, 60, 80)
			h.after(300*time.Millisecond, h.stepClickThrough)
		})
	})
}

func (h *host) stepClickThrough() {
	kinds := h.a.pointerKinds()
	moved := false
	for _, k := range kinds {
		moved = moved || k == input.Move
	}
	h.check("move-routing", moved && len(kinds) > 0 && kinds[len(kinds)-1] == input.Leave,
		fmt.Sprintf("canvas pointer kinds %v", kinds))

	h.a.events = nil
	if err := h.hk.SmokeAttachClick(h.a.cv, 60, 80); err != nil {
		h.fail("click-through", err.Error())
	}
	h.after(500*time.Millisecond, func() {
		presses, _ := h.presses()
		h.check("click-through", presses == 0 && slicesContains(h.msgs, "down:60,80"),
			fmt.Sprintf("page messages %v, canvas presses %d", h.msgs, presses))
		h.a.events = nil
		h.msgs = nil
		if err := h.hk.SmokeAttachClick(h.a.cv, 180, 80); err != nil {
			h.fail("click-canvas", err.Error())
		}
		h.after(400*time.Millisecond, h.checkCanvasClick)
	})
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (h *host) checkCanvasClick() {
	_, press := h.presses()
	h.check("click-canvas", press != nil && press.Pos.X == 140 && press.Pos.Y == 50 && len(h.msgs) == 0,
		fmt.Sprintf("press %+v, page messages %v", press, h.msgs))
	st := h.state(h.a.cv)
	h.check("focus", st.Focused && len(h.a.focus) > 0 && h.a.focus[len(h.a.focus)-1],
		fmt.Sprintf("focused %v, focus events %v", st.Focused, h.a.focus))

	red, hole, out := h.px(180, 80), h.px(60, 80), h.px(300, 200)
	h.check("shot-above", isRed(red) && isBlue(hole) && isBlue(out),
		fmt.Sprintf("opaque %v, transparent %v, outside %v", red, hole, out))

	if err := h.hk.SmokeAttachClick(h.a.cv, 300, 200); err != nil {
		h.fail("focus-web", err.Error())
	}
	h.after(500*time.Millisecond, func() {
		st := h.state(h.a.cv)
		f := h.a.focus
		h.check("focus-web", !st.Focused && len(f) > 0 && !f[len(f)-1],
			fmt.Sprintf("focused %v, focus events %v", st.Focused, f))
		h.stepUnderlay()
	})
}

func (h *host) stepUnderlay() {
	h.bb = &sink{h: h, name: "b", paint: paintB}
	cv, err := h.b.(platform.CanvasAttacher).AttachCanvas(surfaceID, platform.CanvasLayer{}, h.bb)
	if err != nil {
		h.fail("attach-below", err.Error())
		h.stepClose()
		return
	}
	h.bb.cv = cv
	st := h.state(cv)
	w, ht := cv.Size()
	h.check("attach-below", st.InOverlay && !st.AboveWeb && st.WebTransparent && !st.PassThrough && w == 480 && ht == 320,
		fmt.Sprintf("%+v size %gx%g", st, w, ht))
	h.surface.Eval(`document.documentElement.style.background='transparent';document.body.style.background='transparent'`)
	h.after(600*time.Millisecond, func() {
		if !h.x11 {
			h.skip("shot-below")
			h.stepResize()
			return
		}
		g, red := h.px(300, 200), h.px(180, 80)
		h.check("shot-below", isGreen(g) && isRed(red), fmt.Sprintf("page area %v, canvas A %v", g, red))
		h.stepResize()
	})
}

func (h *host) stepResize() {
	nb := len(h.bb.sizes)
	fb := h.bb.frames
	h.surface.SetSize(640, 400)
	h.after(500*time.Millisecond, func() {
		w, ht := h.bb.cv.Size()
		got := len(h.bb.sizes) > nb && h.bb.sizes[len(h.bb.sizes)-1][0] == 640
		h.check("resize-fill", got && w == 640 && ht == 400 && h.bb.frames > fb,
			fmt.Sprintf("B size %gx%g, resizes %v, frames +%d", w, ht, h.bb.sizes, h.bb.frames-fb))
		st := h.state(h.a.cv)
		h.check("resize-pinned", st.Frame == [4]float64{40, 30, 200, 100}, fmt.Sprintf("A frame %v", st.Frame))
		h.stepSetLayer()
	})
}

func (h *host) stepSetLayer() {
	ac, ok := h.a.cv.(platform.AttachedCanvas)
	if !ok {
		h.fail("set-layer", "not a platform.AttachedCanvas")
		h.stepClose()
		return
	}
	na := len(h.a.sizes)
	err := ac.SetLayer(platform.CanvasLayer{Bounds: platform.Rect{X: 300, Y: 200, Width: 120, Height: 80},
		Above: true, PassThrough: true, Anchor: platform.AnchorBottomRight})
	h.after(400*time.Millisecond, func() { // GTK allocates in its next layout cycle
		st := h.state(h.a.cv)
		w, ht := h.a.cv.Size()
		h.check("set-layer", err == nil && st.Frame == [4]float64{300, 200, 120, 80} && st.AboveWeb && w == 120 && ht == 80 && len(h.a.sizes) > na,
			fmt.Sprintf("err %v frame %v size %gx%g resizes %v", err, st.Frame, w, ht, h.a.sizes[na:]))
		h.a.events = nil
		if !h.x11 {
			h.skip("hit-after-move")
			h.stepAnchor()
			return
		}
		_ = h.hk.SmokeAttachClick(h.a.cv, 400, 240)
		h.after(400*time.Millisecond, func() {
			n, p := h.presses()
			h.check("hit-after-move", n > 0 && p.Pos.X == 100 && p.Pos.Y == 40, fmt.Sprintf("press %+v at the new place", p))
			h.stepAnchor()
		})
	})
}

func (h *host) stepAnchor() {
	h.surface.SetSize(700, 450)
	h.after(500*time.Millisecond, func() {
		st := h.state(h.a.cv)
		h.check("anchor", st.Frame == [4]float64{360, 250, 120, 80}, fmt.Sprintf("A frame %v after 640x400 -> 700x450", st.Frame))
		h.stepDetach()
	})
}

func (h *host) stepDetach() {
	cv := h.bb.cv.(platform.AttachedCanvas)
	cv.Detach()
	cv.Detach() // idempotent
	st := h.state(cv)
	err := cv.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	cv.Invalidate() // harmless
	h.check("detach", !st.InOverlay && !st.WebTransparent && err != nil,
		fmt.Sprintf("%+v, Present: %v", st, err))
	h.stepIdle()
}

func (h *host) stepIdle() {
	h.after(300*time.Millisecond, func() {
		na, nb := h.a.frames, h.bb.frames
		h.after(2*time.Second, func() {
			da, db := h.a.frames-na, h.bb.frames-nb
			h.check("idle", da == 0 && db == 0, fmt.Sprintf("frames in 2s: A %d, detached B %d", da, db))
			h.stepClose()
		})
	})
}

func (h *host) stepClose() {
	cv := h.a.cv
	if h.surface != nil {
		h.surface.Close()
	}
	h.post(func() {
		cv.Invalidate()
		err := cv.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)))
		h.check("after-close", err != nil, fmt.Sprintf("Present: %v", err))
		var early []string
		for _, s := range []*sink{h.a, h.bb} {
			if s != nil && len(s.errs) > 0 {
				early = append(early, fmt.Sprintf("%s: %v", s.name, s.errs))
			}
		}
		h.check("present-errors", len(early) == 0, fmt.Sprintf("%v", early))
		h.after(200*time.Millisecond, h.b.Quit)
	})
}

var required = []string{
	"attach", "a-first", "bounds", "input-shape", "web-background", "move-routing",
	"click-through", "click-canvas", "focus", "shot-above", "focus-web", "attach-below", "b-first",
	"shot-below", "resize-fill", "resize-pinned", "set-layer", "hit-after-move", "anchor", "detach",
	"idle", "closed", "after-close", "present-errors",
}

func main() {
	flag.Parse()
	if *smoke {
		go func() {
			time.Sleep(scaled(50 * time.Second))
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(2)
		}()
	}
	h := &host{b: linux.New(), start: time.Now(), checks: map[string]bool{}}
	if err := h.b.Init(h, platform.AppOptions{Name: "Tango Attach Smoke", ID: "dev.tango.smokeattach"}); err != nil {
		fmt.Println("Init:", err)
		os.Exit(1)
	}
	if err := h.b.Run(); err != nil {
		fmt.Println("Run:", err)
		os.Exit(1)
	}
	if !*smoke {
		return
	}
	var missing []string
	for _, c := range required {
		if !h.checks[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 || len(h.failed) > 0 {
		fmt.Printf("FAIL: missing checks: %s; failed: %s\n", strings.Join(missing, ", "), strings.Join(h.failed, ", "))
		os.Exit(1)
	}
	fmt.Printf("PASS: all %d checks\n", len(required))
}

func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
