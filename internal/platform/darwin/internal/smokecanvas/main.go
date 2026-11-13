//go:build darwin && !ios

// Command smokecanvas drives a canvas surface of the macOS backend (the
// self-drawn UI window, docs/specs/canvas-surface.md) without the root
// tango package. It draws a gradient, a 1-device-pixel stripe pattern and
// a square under the mouse.
//
//	go run ./internal/platform/darwin/internal/smokecanvas          # interactive
//	go run ./internal/platform/darwin/internal/smokecanvas -smoke   # self-check, exits 0 on PASS
//
// With -smoke it checks: the first frame is presented at Size×Scale pixels;
// a resize reports CanvasResized and presents a new frame; Invalidate from
// another goroutine produces a frame; 1000 Invalidate calls in one second
// coalesce to at most one frame per display refresh; InvalidateAt fires on
// time and an earlier deadline replaces a later one; synthetic mouse,
// scroll, key and NSTextInputClient input arrive as input events; a still
// window produces no frame for 2 s; closing reaches the host; a window
// screenshot shows the drawn pixels at full Retina resolution.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the overall deadlines for slow
// machines (CI).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/darwin"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() }

const surfaceID platform.SurfaceID = 1

var smoke = flag.Bool("smoke", false, "run the automatic checks and exit")

type host struct {
	b     platform.Backend
	mu    sync.Mutex
	queue []func()

	start   time.Time
	checks  map[string]bool
	failed  []string
	skipped int

	surface platform.Surface
	cv      platform.Canvas
	sink    *sink
	drops   []platform.SurfaceEvent // SurfaceFilesDropped
}

func (h *host) logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(h.start).Seconds(), fmt.Sprintf(format, args...))
}

func (h *host) pass(name, detail string) {
	if !h.checks[name] {
		h.checks[name] = true
		h.logf("PASS %-18s %s", name, detail)
	}
}

func (h *host) fail(name, detail string) {
	if skippable(name) {
		// Counted as done so the summary does not report it missing.
		h.checks[name] = true
		h.skipped++
		h.logf("SKIP %-18s %s (TANGO_SMOKE_SKIP)", name, detail)
		return
	}
	h.failed = append(h.failed, name)
	h.logf("FAIL %-18s %s", name, detail)
}

// displayLinkChecks need the display link to fire, which macOS pauses on a
// locked screen, a headless CI runner or a fully occluded window.
var displayLinkChecks = map[string]bool{"invalidate": true, "coalesce": true, "invalidate-at": true}

// skippable reports whether a failure of name is downgraded to SKIP:
// TANGO_SMOKE_SKIP is a comma-separated list; "displaylink" covers the
// checks that depend on vsync callbacks.
func skippable(name string) bool {
	for _, s := range strings.Split(os.Getenv("TANGO_SMOKE_SKIP"), ",") {
		if strings.TrimSpace(s) == "displaylink" && displayLinkChecks[name] {
			return true
		}
	}
	return false
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
	switch ev {
	case platform.LifecycleStarted:
		h.onStarted()
	case platform.LifecycleTerminating:
		h.logf("terminating")
	}
}

func (h *host) ShouldClose(platform.SurfaceID) bool { return true }

func (h *host) OnSurfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent) {
	switch ev.Kind {
	case platform.SurfaceClosed:
		h.pass("closed", fmt.Sprintf("surface %d", id))
		h.surface = nil
		if !*smoke {
			h.b.Quit()
		}
	case platform.SurfaceResized:
		h.logf("surface resized %dx%d", ev.Width, ev.Height)
	case platform.SurfaceFilesDropped:
		h.logf("files dropped %v at %d,%d", ev.Paths, ev.X, ev.Y)
		h.drops = append(h.drops, ev)
	case platform.SurfaceFocused, platform.SurfaceBlurred:
		h.logf("surface focus event %d", ev.Kind)
	}
}

func (h *host) OnMessage(platform.SurfaceID, string, string) {}

func (h *host) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// ---- sink ----

type frameInfo struct {
	at         time.Time
	w, h       int // pixels presented
	logW, logH float32
	scale      float32
	err        error
}

type sink struct {
	h       *host
	frames  []frameInfo
	events  []input.Event
	resizes [][3]float32
	focus   []bool
	mouse   geom.Point
	quiet   bool     // ignore input-driven invalidation (idle check)
	drag    []string // CanvasDragOver / CanvasDragLeave calls
}

// The sink implements platform.CanvasDropTarget.
func (s *sink) CanvasDragOver(x, y float32) { s.drag = append(s.drag, fmt.Sprintf("over %g,%g", x, y)) }
func (s *sink) CanvasDragLeave()            { s.drag = append(s.drag, "leave") }

func (s *sink) CanvasFrame() {
	cv := s.h.cv
	if cv == nil {
		return // early callback before NewCanvasSurface returned
	}
	w, h := cv.Size()
	sc := cv.Scale()
	pw, ph := int(math.Round(float64(w*sc))), int(math.Round(float64(h*sc)))
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	draw(img, sc, s.mouse)
	err := cv.Present(img)
	s.frames = append(s.frames, frameInfo{time.Now(), pw, ph, w, h, sc, err})
	if len(s.frames) == 1 {
		s.h.onFirstFrame()
	}
}

func (s *sink) CanvasInput(ev input.Event) {
	s.events = append(s.events, ev)
	if p, ok := ev.(input.Pointer); ok {
		s.mouse = p.Pos
		if !s.quiet {
			s.h.cv.Invalidate()
		}
	}
	if !*smoke {
		s.h.logf("input %T %+v", ev, ev)
		if t, ok := ev.(input.KeyEvent); ok && t.Down && t.Key == input.KeyEnter {
			s.h.cv.SetTextInput(true, geom.R(s.mouse.X, s.mouse.Y, 2, 20))
		}
	}
}

func (s *sink) CanvasResized(w, h, scale float32) {
	s.resizes = append(s.resizes, [3]float32{w, h, scale})
	s.h.logf("CanvasResized %gx%g @%g", w, h, scale)
	s.h.cv.Invalidate()
}

func (s *sink) CanvasFocus(f bool) {
	s.focus = append(s.focus, f)
	s.h.logf("CanvasFocus %v", f)
}

// draw paints a red→blue vertical gradient, 1-device-pixel black/white
// stripes in logical (20,20)-(100,60), and a green 24pt square at mouse.
func draw(img *image.RGBA, scale float32, mouse geom.Point) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		t := float64(y) / float64(max(b.Dy()-1, 1))
		c := color.RGBA{uint8(255 * (1 - t)), 40, uint8(255 * t), 255}
		row := img.Pix[y*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = c.R, c.G, c.B, c.A
		}
	}
	px := func(v float32) int { return int(v * scale) }
	for y := px(20); y < px(60) && y < b.Dy(); y++ {
		for x := px(20); x < px(100) && x < b.Dx(); x++ {
			v := uint8(0)
			if x%2 == 0 {
				v = 255
			}
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	for y := px(mouse.Y - 12); y < px(mouse.Y+12); y++ {
		for x := px(mouse.X - 12); x < px(mouse.X+12); x++ {
			if image.Pt(x, y).In(b) {
				img.SetRGBA(x, y, color.RGBA{0, 200, 0, 255})
			}
		}
	}
}

// ---- script ----

func (h *host) onStarted() {
	cc, ok := h.b.(platform.CanvasCreator)
	if !ok {
		h.fail("canvas-creator", "backend does not implement platform.CanvasCreator")
		h.b.Quit()
		return
	}
	h.sink = &sink{h: h, quiet: *smoke}
	s, cv, err := cc.NewCanvasSurface(surfaceID, platform.SurfaceOptions{
		Title: "Tango canvas smoke", Width: 480, Height: 320, MinWidth: 200, MinHeight: 150,
		Resizable: true, Visible: true,
	}, platform.SurfaceExtras{
		Centered:    true,
		Appearance:  platform.Appearance{},
		AcceptFiles: true,
	}, h.sink)
	if err != nil {
		h.fail("canvas-creator", err.Error())
		h.b.Quit()
		return
	}
	h.surface, h.cv = s, cv
	w, ht := cv.Size()
	h.pass("canvas-creator", fmt.Sprintf("native=%#x size=%gx%g scale=%g", s.Native(), w, ht, cv.Scale()))
	if *smoke {
		h.after(scaled(15*time.Second), func() {
			h.fail("timeout", "script did not finish")
			h.b.Quit()
		})
	}
}

func (h *host) onFirstFrame() {
	f := h.sink.frames[0]
	if f.err != nil {
		h.fail("first-present", f.err.Error())
	} else {
		h.pass("first-present", fmt.Sprintf("%dx%d px", f.w, f.h))
	}
	if f.logW == 480 && f.logH == 320 && f.w == int(480*f.scale) && f.h == int(320*f.scale) {
		h.pass("size-scale", fmt.Sprintf("%gx%g @%g = %dx%d px", f.logW, f.logH, f.scale, f.w, f.h))
	} else {
		h.fail("size-scale", fmt.Sprintf("%+v", f))
	}
	if *smoke {
		h.after(500*time.Millisecond, h.stepScreenshot)
	}
}

func (h *host) stepScreenshot() {
	wn := objc.ID(h.surface.Native()).Send(objc.RegisterName("windowNumber"))
	scale := h.cv.Scale()
	cw, ch := h.cv.Size()
	go func() {
		file := "/tmp/tango-smokecanvas.png"
		out, err := exec.Command("screencapture", "-x", "-o", fmt.Sprintf("-l%d", wn), file).CombinedOutput()
		var msg string
		var ok, skip bool
		if err != nil {
			skip, msg = true, fmt.Sprintf("screencapture failed: %v %s", err, bytes.TrimSpace(out))
		} else {
			ok, skip, msg = checkShot(file, int(cw*scale), int(ch*scale), scale)
		}
		h.post(func() {
			switch {
			case ok:
				h.pass("screenshot", msg)
			case skip:
				h.logf("SKIP screenshot: %s", msg)
			default:
				h.fail("screenshot", msg)
			}
			if ok {
				h.stepGravity()
			} else {
				h.stepResize()
			}
		})
	}()
}

// stepGravity presents a half-size frame (as can happen for a moment during
// live resize) and checks it is pinned to the top-left, unscaled.
func (h *host) stepGravity() {
	w, ht := h.cv.Size()
	sc := h.cv.Scale()
	pw, ph := int(w*sc), int(ht*sc)
	half := image.NewRGBA(image.Rect(0, 0, pw/2, ph/2))
	draw(half, sc, geom.Pt(-100, -100))
	if err := h.cv.Present(half); err != nil {
		h.fail("gravity", err.Error())
		h.stepResize()
		return
	}
	wn := objc.ID(h.surface.Native()).Send(objc.RegisterName("windowNumber"))
	go func() {
		time.Sleep(150 * time.Millisecond)
		file := "/tmp/tango-smokecanvas-gravity.png"
		_, err := exec.Command("screencapture", "-x", "-o", fmt.Sprintf("-l%d", wn), file).CombinedOutput()
		msg, ok := "", false
		if err == nil {
			ok, msg = checkGravity(file, pw, ph)
		} else {
			msg = err.Error()
		}
		h.post(func() {
			if ok {
				h.pass("gravity", msg)
			} else {
				h.fail("gravity", msg)
			}
			h.cv.Invalidate() // back to a full frame
			h.after(100*time.Millisecond, h.stepResize)
		})
	}()
}

func checkGravity(file string, pw, ph int) (bool, string) {
	f, err := os.Open(file)
	if err != nil {
		return false, err.Error()
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return false, err.Error()
	}
	b := img.Bounds()
	top := b.Max.Y - ph
	rgb := func(x, y int) [3]int {
		R, G, B, _ := img.At(b.Min.X+x, top+y).RGBA()
		return [3]int{int(R >> 8), int(G >> 8), int(B >> 8)}
	}
	tl := rgb(pw/2-10, 10)      // top of the half frame: red
	bl := rgb(pw/2-10, ph/2-10) // bottom of the half frame: blue
	out := rgb(pw*3/4, ph*3/4)  // outside the frame: not drawn
	ok := tl[0] > 150 && tl[2] < 100 && bl[2] > 150 && bl[0] < 100 && out != tl && out != bl
	return ok, fmt.Sprintf("half frame top rgb%v bottom rgb%v, outside rgb%v", tl, bl, out)
}

// checkShot verifies the captured window: content at the bottom (below the
// title bar), red at the top of the content, blue at the bottom, and the
// stripes alternating every device pixel.
func checkShot(file string, pw, ph int, scale float32) (ok, skip bool, msg string) {
	f, err := os.Open(file)
	if err != nil {
		return false, true, err.Error()
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return false, true, err.Error()
	}
	b := img.Bounds()
	if b.Dx() != pw {
		return false, false, fmt.Sprintf("capture %dx%d, want width %d (content %dx%d px)", b.Dx(), b.Dy(), pw, pw, ph)
	}
	top := b.Max.Y - ph // content top in the capture
	if top < 0 {
		return false, false, fmt.Sprintf("capture %dx%d shorter than content %d", b.Dx(), b.Dy(), ph)
	}
	at := func(x, y int) (r, g, bl int) {
		R, G, B, _ := img.At(b.Min.X+x, top+y).RGBA()
		return int(R >> 8), int(G >> 8), int(B >> 8)
	}
	r1, _, b1 := at(pw-20, ph/10)
	r2, _, b2 := at(pw-20, ph*9/10)
	if !(r1 > 150 && b1 < 100 && b2 > 150 && r2 < 100) {
		if r1 == r2 && b1 == b2 {
			return false, true, fmt.Sprintf("capture is uniform (%d,%d) — no screen recording permission?", r1, b1)
		}
		return false, false, fmt.Sprintf("gradient wrong: top rgb(%d,_,%d) bottom rgb(%d,_,%d)", r1, b1, r2, b2)
	}
	y := int(40 * scale)
	alt := 0
	n := 0
	for x := int(30 * scale); x < int(90*scale); x++ {
		a, _, _ := at(x, y)
		c, _, _ := at(x+1, y)
		n++
		if (a > 128) != (c > 128) {
			alt++
		}
	}
	if alt < n*9/10 {
		return false, false, fmt.Sprintf("stripes blurred: %d/%d device-pixel transitions", alt, n)
	}
	return true, false, fmt.Sprintf("%dx%d capture; gradient top rgb(%d,_,%d) bottom rgb(%d,_,%d); %d/%d sharp 1px stripes", b.Dx(), b.Dy(), r1, b1, r2, b2, alt, n)
}

func (h *host) stepResize() {
	n := len(h.sink.frames)
	r := len(h.sink.resizes)
	h.surface.SetSize(640, 400)
	h.after(400*time.Millisecond, func() {
		sc := h.cv.Scale()
		gotResize := false
		for _, rs := range h.sink.resizes[r:] {
			if rs[0] == 640 && rs[1] == 400 && rs[2] == sc {
				gotResize = true
			}
		}
		var fr *frameInfo
		for i := n; i < len(h.sink.frames); i++ {
			if f := h.sink.frames[i]; f.w == int(640*sc) && f.h == int(400*sc) && f.err == nil {
				fr = &f
			}
		}
		if gotResize && fr != nil {
			h.pass("resize", fmt.Sprintf("CanvasResized 640x400@%g, presented %dx%d", sc, fr.w, fr.h))
		} else {
			h.fail("resize", fmt.Sprintf("resizes=%v frames since=%d", h.sink.resizes[r:], len(h.sink.frames)-n))
		}
		h.stepInvalidate()
	})
}

func (h *host) stepInvalidate() {
	n := len(h.sink.frames)
	go h.cv.Invalidate()
	h.after(200*time.Millisecond, func() {
		if d := len(h.sink.frames) - n; d >= 1 {
			h.pass("invalidate", fmt.Sprintf("goroutine Invalidate → %d frame(s)", d))
		} else {
			h.fail("invalidate", "no frame within 200ms")
		}
		h.stepCoalesce()
	})
}

func (h *host) stepCoalesce() {
	n := len(h.sink.frames)
	var calls atomic.Int64
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				h.cv.Invalidate()
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	go func() {
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
		h.post(func() {
			fr := h.sink.frames[n:]
			d := len(fr)
			span := time.Duration(0)
			if d > 1 {
				span = fr[d-1].at.Sub(fr[0].at)
			}
			detail := fmt.Sprintf("%d Invalidate calls → %d frames over %v", calls.Load(), d, span.Round(time.Millisecond))
			if d >= 10 && d <= 130 {
				h.pass("coalesce", detail)
			} else {
				h.fail("coalesce", detail)
			}
			h.stepInvalidateAt()
		})
	}()
}

func (h *host) stepInvalidateAt() {
	h.sink.quiet = true                    // from here on, real mouse movement must not cause frames
	h.after(200*time.Millisecond, func() { // let the display link go idle
		n := len(h.sink.frames)
		ne := len(h.sink.events)
		start := time.Now()
		late, early := start.Add(500*time.Millisecond), start.Add(250*time.Millisecond)
		go func() {
			h.cv.InvalidateAt(late)
			h.cv.InvalidateAt(early) // the earlier deadline wins
		}()
		h.after(800*time.Millisecond, func() {
			fr := h.sink.frames[n:]
			if len(fr) == 0 {
				h.fail("invalidate-at", "no frame")
			} else {
				d := fr[0].at.Sub(early)
				detail := fmt.Sprintf("frame %+v after the 250ms deadline, %d frame(s) in 800ms (%d input events meanwhile)", d.Round(time.Millisecond), len(fr), len(h.sink.events)-ne)
				if d >= -2*time.Millisecond && d <= 50*time.Millisecond && len(fr) == 1 {
					h.pass("invalidate-at", detail)
				} else {
					h.fail("invalidate-at", detail)
				}
			}
			h.stepInput()
		})
	})
}

func (h *host) stepInput() {
	hook, ok := h.b.(interface {
		SmokeCanvasInput(platform.Canvas) error
	})
	if !ok {
		h.fail("input", "no SmokeCanvasInput hook")
		h.stepIdle()
		return
	}
	h.sink.quiet = true
	h.sink.events = nil
	if err := hook.SmokeCanvasInput(h.cv); err != nil {
		h.fail("text-input-client", err.Error())
	} else {
		h.pass("text-input-client", "marked/selected range, firstRect → screen, actualRange")
	}
	evs := h.sink.events
	has := func(name string, match func(input.Event) bool) {
		for _, e := range evs {
			if match(e) {
				h.pass(name, fmt.Sprintf("%T %+v", e, e))
				return
			}
		}
		h.fail(name, "not delivered")
	}
	has("mouse-move", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Move && p.Pos == geom.Pt(30, 20)
	})
	has("mouse-press", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonLeft && p.Pos == geom.Pt(100, 50) && p.Clicks == 2 && p.Mods == input.ModShift
	})
	has("mouse-release", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Release && p.Button == input.ButtonLeft
	})
	has("mouse-right", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonRight && p.Pos == geom.Pt(10, 10)
	})
	has("scroll-precise", func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DY == 30 && s.DX == 0
	})
	has("scroll-line", func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DY > 0 && s.DY != 30
	})
	has("key-left", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && k.Down && k.Mods == 0
	})
	has("key-left-up", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && !k.Down
	})
	has("key-cmd-c", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyC && k.Down && k.Mods == input.ModMeta
	})
	has("key-f5", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyF5 && k.Down && k.Mods == 0
	})
	has("key-shift-left-down", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyShiftLeft && k.Down && k.Mods == input.ModShift
	})
	has("key-shift-left-up", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyShiftLeft && !k.Down && k.Mods == 0
	})
	has("key-numpad1", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyNumpad1 && k.Down
	})
	has("key-slash", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeySlash && k.Down
	})
	has("key-f6-text-on", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyF6 && k.Down
	})
	has("composition", func(e input.Event) bool {
		c, ok := e.(input.Composition)
		return ok && c.Text == "ni hao" && c.Cursor == 6
	})
	has("text-input", func(e input.Event) bool {
		t, ok := e.(input.TextInput)
		return ok && t.Text == "你好"
	})
	has("command-enter", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyEnter && k.Down
	})
	h.stepDropEdit()
}

// stepDropEdit checks file drops (a simulated NSDraggingInfo) and the
// Edit menu validation with text input off and on.
func (h *host) stepDropEdit() {
	hk, ok := h.b.(interface {
		SmokeCanvasDrop(cv platform.Canvas, paths []string, x, y float64) (uint, bool, error)
		SmokeCanvasEditMenu(cv platform.Canvas) (map[string]bool, error)
	})
	if !ok {
		h.fail("drop-files", "no drop / edit menu hooks")
		h.stepIdle()
		return
	}
	h.sink.drag = nil
	op, dropped, err := hk.SmokeCanvasDrop(h.cv, []string{"/tmp/tango a.txt", "/tmp/b.png"}, 120, 70)
	var got platform.SurfaceEvent
	if len(h.drops) > 0 {
		got = h.drops[len(h.drops)-1]
	}
	h.check("drop-files", err == nil && op == 1 && dropped && len(h.drops) == 1 &&
		strings.Join(got.Paths, "|") == "/tmp/tango a.txt|/tmp/b.png" && got.X == 120 && got.Y == 70,
		fmt.Sprintf("op %d dropped %v err %v event %+v", op, dropped, err, got))
	h.check("drop-feedback", len(h.sink.drag) >= 2 && h.sink.drag[0] == "over 120,70" && h.sink.drag[len(h.sink.drag)-1] == "leave",
		fmt.Sprintf("%v", h.sink.drag))
	op, dropped, err = hk.SmokeCanvasDrop(h.cv, nil, 120, 70)
	h.check("drop-refused", err == nil && op == 0 && !dropped && len(h.drops) == 1,
		fmt.Sprintf("text-only drag: op %d dropped %v err %v drops %d", op, dropped, err, len(h.drops)))

	h.cv.SetTextInput(false, geom.Rect{})
	off, err1 := hk.SmokeCanvasEditMenu(h.cv)
	h.cv.SetTextInput(true, geom.R(10, 10, 2, 16))
	on, err2 := hk.SmokeCanvasEditMenu(h.cv)
	h.cv.SetTextInput(false, geom.Rect{})
	okOff := !off["copy:"] && !off["cut:"] && !off["paste:"] && !off["selectAll:"] && !off["undo:"] && !off["redo:"] && off["performClose:"]
	okOn := on["copy:"] && on["cut:"] && on["selectAll:"] && on["undo:"] && on["redo:"] && on["paste:"] == on["pasteboard-text"]
	h.check("edit-menu", err1 == nil && err2 == nil && okOff && okOn, fmt.Sprintf("text off %v; text on %v", off, on))
	h.stepIdle()
}

func (h *host) check(name string, ok bool, detail string) {
	if ok {
		h.pass(name, detail)
	} else {
		h.fail(name, detail)
	}
}

func (h *host) stepIdle() {
	h.surface.Focus() // try to become key for the focus check
	h.after(300*time.Millisecond, func() {
		n := len(h.sink.frames)
		h.after(2*time.Second, func() {
			if d := len(h.sink.frames) - n; d == 0 {
				h.pass("idle", "no frame in 2s while still")
			} else {
				h.fail("idle", fmt.Sprintf("%d frame(s) in 2s while still", d))
			}
			if len(h.sink.focus) > 0 && h.sink.focus[0] {
				h.pass("focus", fmt.Sprintf("CanvasFocus %v", h.sink.focus))
			} else {
				h.logf("SKIP focus: %v (window may not have become key: app not frontmost)", h.sink.focus)
			}
			h.stepClose()
		})
	})
}

func (h *host) stepClose() {
	cv := h.cv
	h.surface.Close()
	h.post(func() {
		cv.Invalidate() // must be harmless after close
		cv.InvalidateAt(time.Now())
		if err := cv.Present(image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
			h.pass("after-close", "Invalidate harmless, Present: "+err.Error())
		} else {
			h.fail("after-close", "Present succeeded on a closed canvas")
		}
		h.after(200*time.Millisecond, h.b.Quit)
	})
}

var required = []string{
	"canvas-creator", "first-present", "size-scale", "resize", "invalidate", "coalesce",
	"invalidate-at", "text-input-client", "mouse-move", "mouse-press", "mouse-release", "mouse-right",
	"scroll-precise", "scroll-line", "key-left", "key-left-up", "key-cmd-c",
	"key-f5", "key-shift-left-down", "key-shift-left-up", "key-numpad1", "key-slash", "key-f6-text-on", "composition",
	"text-input", "command-enter", "drop-files", "drop-feedback", "drop-refused", "edit-menu",
	"idle", "closed", "after-close",
}

func main() {
	flag.Parse()
	if *smoke {
		go func() {
			time.Sleep(scaled(30 * time.Second))
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(2)
		}()
	}
	h := &host{b: darwin.New(), start: time.Now(), checks: map[string]bool{}}
	if err := h.b.Init(h, platform.AppOptions{Name: "Tango Canvas Smoke", ID: "dev.tango.smokecanvas"}); err != nil {
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
	if h.skipped > 0 {
		fmt.Printf("PASS: all %d checks (%d skipped, +%d optional)\n", len(required), h.skipped, len(h.checks)-len(required))
		return
	}
	fmt.Printf("PASS: all %d checks (+%d optional)\n", len(required), len(h.checks)-len(required))
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
