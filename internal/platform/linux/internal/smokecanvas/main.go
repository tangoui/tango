//go:build linux && !android

// Command smokecanvas drives a canvas surface of the Linux backend (the
// self-drawn UI window, docs/specs/canvas-surface.md) without the root
// tango package. It draws a gradient, a 1-device-pixel stripe pattern and
// a square under the mouse.
//
//	go run ./internal/platform/linux/internal/smokecanvas          # interactive
//	go run ./internal/platform/linux/internal/smokecanvas -smoke   # self-check, exits 0 on PASS
//
// With -smoke it checks: the first frame is presented at Size×Scale pixels;
// an offscreen render of the widget (gtk_widget_draw) shows the frame with
// the right channel order and sharp device-pixel stripes; a smaller frame
// is pinned to the top-left; a resize reports CanvasResized and presents a
// new frame; Invalidate from another goroutine produces a frame; 1000
// Invalidate calls in one second coalesce to at most about one frame per
// display refresh; InvalidateAt fires on time and an earlier deadline
// replaces a later one; synthetic GdkEvents (whose fields are checked
// against GDK's accessors) arrive as pointer, scroll, key, text and IME
// events; a still window produces no frame for 2 s; closing reaches the
// host and a closed canvas refuses Present.
//
// The IM checks expect GtkIMContextSimple, so -smoke sets
// GTK_IM_MODULE=gtk-im-context-simple unless TANGO_SMOKE_KEEP_IM=1.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the overall deadlines for slow
// machines (CI).
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
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/linux"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() }

const surfaceID platform.SurfaceID = 1

var smoke = flag.Bool("smoke", false, "run the automatic checks and exit")

// smokeHooks are the test hooks of canvas_smoke_linux.go.
type smokeHooks interface {
	SmokeCanvasInput(platform.Canvas) error
	SmokeCanvasRender(platform.Canvas) (*image.RGBA, error)
}

type host struct {
	b     platform.Backend
	mu    sync.Mutex
	queue []func()

	start  time.Time
	checks map[string]bool
	failed []string

	surface platform.Surface
	cv      platform.Canvas
	sink    *sink
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
	h.failed = append(h.failed, name)
	h.logf("FAIL %-18s %s", name, detail)
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
	quiet   bool // ignore input-driven invalidation (smoke mode)
}

func (s *sink) CanvasFrame() {
	cv := s.h.cv
	if cv == nil {
		s.h.fail("early-frame", "CanvasFrame before NewCanvasSurface returned")
		return
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
	if s.h.cv == nil {
		s.h.fail("early-input", "CanvasInput before NewCanvasSurface returned")
		return
	}
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
			s.h.logf("text input on at the mouse (Escape turns it off)")
		}
		if t, ok := ev.(input.KeyEvent); ok && t.Down && t.Key == input.KeyEscape {
			s.h.cv.SetTextInput(false, geom.Rect{})
		}
		if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press {
			s.h.cv.SetCursor(input.Cursor((int(p.Pos.X) / 40) % 9)) // cursor shape by column
		}
	}
}

func (s *sink) CanvasResized(w, h, scale float32) {
	if s.h.cv == nil {
		s.h.fail("early-resize", "CanvasResized before NewCanvasSurface returned")
		return
	}
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
	}, platform.SurfaceExtras{Centered: true}, h.sink)
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
		h.fail("size-scale", fmt.Sprintf("%+v (a tiling window manager may impose its own size)", f))
	}
	if *smoke {
		h.after(500*time.Millisecond, h.stepRender)
	}
}

func (h *host) hooks() smokeHooks {
	hk, _ := h.b.(smokeHooks)
	return hk
}

func rgbAt(img *image.RGBA, x, y int) [4]int {
	c := img.RGBAAt(x, y)
	return [4]int{int(c.R), int(c.G), int(c.B), int(c.A)}
}

// stepRender checks the frame as GTK paints it: red at the top, blue at
// the bottom (RGBA → cairo ARGB32 channel order) and stripes alternating
// every device pixel (no scaling between frame and widget).
func (h *host) stepRender() {
	hk := h.hooks()
	if hk == nil {
		h.fail("render", "no smoke hooks")
		h.stepResize()
		return
	}
	img, err := hk.SmokeCanvasRender(h.cv)
	if err != nil {
		h.fail("render", err.Error())
		h.stepResize()
		return
	}
	sc := h.cv.Scale()
	b := img.Bounds()
	pw, ph := b.Dx(), b.Dy()
	top, bot := rgbAt(img, pw-20, ph/10), rgbAt(img, pw-20, ph*9/10)
	y := int(40 * sc)
	alt, n := 0, 0
	for x := int(30 * sc); x < int(90*sc); x++ {
		n++
		if (img.RGBAAt(x, y).R > 128) != (img.RGBAAt(x+1, y).R > 128) {
			alt++
		}
	}
	detail := fmt.Sprintf("%dx%d px; top rgba%v bottom rgba%v; %d/%d sharp 1px stripes", pw, ph, top, bot, alt, n)
	if top[0] > 150 && top[2] < 100 && bot[2] > 150 && bot[0] < 100 && top[3] == 255 && alt >= n*9/10 {
		h.pass("render", detail)
	} else {
		h.fail("render", detail)
	}
	h.stepGravity()
}

// stepGravity presents a half-size frame (as can happen for a moment
// during a resize) and checks it is painted unscaled at the top-left.
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
	img, err := h.hooks().SmokeCanvasRender(h.cv)
	if err != nil {
		h.fail("gravity", err.Error())
	} else {
		tl := rgbAt(img, pw/2-10, 10)
		bl := rgbAt(img, pw/2-10, ph/2-10)
		out := rgbAt(img, pw*3/4, ph*3/4)
		detail := fmt.Sprintf("half frame top rgba%v bottom rgba%v, outside rgba%v", tl, bl, out)
		if tl[0] > 150 && tl[2] < 100 && bl[2] > 150 && bl[0] < 100 && out[3] == 0 {
			h.pass("gravity", detail)
		} else {
			h.fail("gravity", detail)
		}
	}
	h.cv.Invalidate() // back to a full frame
	h.after(100*time.Millisecond, h.stepResize)
}

func (h *host) stepResize() {
	n := len(h.sink.frames)
	r := len(h.sink.resizes)
	h.surface.SetSize(640, 400)
	h.after(500*time.Millisecond, func() {
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
			// ~1.1 s at 60 Hz is ~66 frames; 144 Hz panels allow up to ~160.
			if d >= 10 && d <= 170 {
				h.pass("coalesce", detail)
			} else {
				h.fail("coalesce", detail)
			}
			h.stepInvalidateAt()
		})
	}()
}

func (h *host) stepInvalidateAt() {
	h.after(200*time.Millisecond, func() { // let the tick callback go idle
		n := len(h.sink.frames)
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
				detail := fmt.Sprintf("frame %+v after the 250ms deadline, %d frame(s) in 800ms", d.Round(time.Millisecond), len(fr))
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
	hk := h.hooks()
	if hk == nil {
		h.fail("event-offsets", "no smoke hooks")
		h.stepIdle()
		return
	}
	h.sink.events = nil
	if err := hk.SmokeCanvasInput(h.cv); err != nil {
		h.fail("event-offsets", err.Error())
	} else {
		h.pass("event-offsets", "synthetic GdkEvent fields agree with gdk_event_get_*")
	}
	evs := h.sink.events
	find := func(match func(input.Event) bool) (input.Event, int) {
		n := 0
		var first input.Event
		for _, e := range evs {
			if match(e) {
				if n == 0 {
					first = e
				}
				n++
			}
		}
		return first, n
	}
	has := func(name string, optional bool, match func(input.Event) bool) {
		if e, n := find(match); n > 0 {
			h.pass(name, fmt.Sprintf("%T %+v", e, e))
		} else if optional {
			h.logf("SKIP %-18s not delivered (optional)", name)
		} else {
			h.fail(name, "not delivered")
		}
	}
	has("mouse-enter", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Move && p.Pos == geom.Pt(5, 5)
	})
	has("mouse-move", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Move && p.Pos == geom.Pt(30, 20)
	})
	has("mouse-press", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonLeft && p.Pos == geom.Pt(100, 50) && p.Clicks == 2 && p.Mods == input.ModShift
	})
	if _, n := find(func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonLeft
	}); n == 2 {
		h.pass("double-click", "2 presses (GDK_2BUTTON_PRESS not reported twice)")
	} else {
		h.fail("double-click", fmt.Sprintf("%d left presses, want 2", n))
	}
	has("mouse-release", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Release && p.Button == input.ButtonLeft
	})
	has("mouse-right", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonRight && p.Pos == geom.Pt(10, 10) && p.Clicks == 1
	})
	has("mouse-leave", false, func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Leave
	})
	has("scroll-smooth", false, func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DY == 30 && s.DX == 0
	})
	has("scroll-line", false, func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DY == 40 && s.DX == 0
	})
	has("key-left", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && k.Down && k.Mods == 0
	})
	has("key-left-up", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && !k.Down
	})
	has("key-ctrl-c", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyC && k.Down && k.Mods == input.ModCtrl
	})
	has("key-modifier", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyShiftLeft && k.Down && k.Mods == input.ModShift
	})
	has("key-shift-left-up", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyShiftLeft && !k.Down && k.Mods == 0
	})
	has("key-f5", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyF5 && k.Down && k.Mods == 0
	})
	if _, n := find(func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyNumpad1 && k.Code == 0 && k.Down
	}); n == 1 {
		h.pass("key-numpad1", "KP_1 (NumLock on) is Numpad1 with no Code")
	} else {
		h.fail("key-numpad1", fmt.Sprintf("%d Numpad1 presses, want 1", n))
	}
	count := func(key, code input.Key, down bool) int {
		_, n := find(func(e input.Event) bool {
			k, ok := e.(input.KeyEvent)
			return ok && k.Key == key && k.Code == code && k.Down == down
		})
		return n
	}
	endDown, endUp := count(input.KeyEnd, input.KeyNumpad1, true), count(input.KeyEnd, input.KeyNumpad1, false)
	navEnd, up8 := count(input.KeyEnd, 0, true), count(input.KeyUp, input.KeyNumpad8, false)
	if endDown == 1 && endUp == 1 && navEnd == 1 && up8 == 1 {
		h.pass("key-numlock-off", "KP_End (NumLock off) is End with Code Numpad1 down and up; "+
			"the navigation block End has no Code; keypad 8 released after NumLock changed keeps Up/Numpad8")
	} else {
		h.fail("key-numlock-off", fmt.Sprintf("End/Numpad1 down %d up %d, End down %d, Up/Numpad8 up %d; want 1 each",
			endDown, endUp, navEnd, up8))
	}
	has("key-slash", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeySlash && k.Down
	})
	has("text-input", false, func(e input.Event) bool {
		t, ok := e.(input.TextInput)
		return ok && t.Text == "a"
	})
	if _, n := find(func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyA && k.Down
	}); n == 1 {
		h.pass("im-routing", "'a' is a KeyEvent with text input off, TextInput with it on")
	} else {
		h.fail("im-routing", fmt.Sprintf("%d KeyA presses, want 1", n))
	}
	has("text-enter", false, func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyEnter && k.Down
	})
	has("composition", true, func(e input.Event) bool {
		c, ok := e.(input.Composition)
		return ok && strings.HasPrefix(c.Text, "u") && c.Cursor > 0
	})
	has("text-hex", true, func(e input.Event) bool {
		t, ok := e.(input.TextInput)
		return ok && t.Text == "你"
	})
	h.stepIdle()
}

func (h *host) stepIdle() {
	h.surface.Focus()
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
				h.logf("SKIP focus: %v (the window manager may not have activated the window)", h.sink.focus)
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
		cv.SetCursor(input.CursorText)
		cv.SetTextInput(true, geom.Rect{})
		if err := cv.Present(image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
			h.pass("after-close", "Invalidate harmless, Present: "+err.Error())
		} else {
			h.fail("after-close", "Present succeeded on a closed canvas")
		}
		h.after(200*time.Millisecond, h.b.Quit)
	})
}

var required = []string{
	"canvas-creator", "first-present", "size-scale", "render", "gravity", "resize", "invalidate",
	"coalesce", "invalidate-at", "event-offsets", "mouse-enter", "mouse-move", "mouse-press",
	"double-click", "mouse-release", "mouse-right", "mouse-leave", "scroll-smooth", "scroll-line",
	"key-left", "key-left-up", "key-ctrl-c", "key-modifier", "key-shift-left-up", "key-f5", "key-numpad1",
	"key-numlock-off", "key-slash", "text-input", "im-routing", "text-enter",
	"idle", "closed", "after-close",
}

func main() {
	flag.Parse()
	if *smoke {
		if os.Getenv("TANGO_SMOKE_KEEP_IM") != "1" {
			os.Setenv("GTK_IM_MODULE", "gtk-im-context-simple")
		}
		go func() {
			time.Sleep(scaled(30 * time.Second))
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(2)
		}()
	}
	h := &host{b: linux.New(), start: time.Now(), checks: map[string]bool{}}
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
