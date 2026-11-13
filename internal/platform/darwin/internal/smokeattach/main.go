//go:build darwin && !ios

// Command smokeattach drives a canvas attached to a WebView surface of the
// macOS backend (docs/specs/canvas-surface.md, step D) without the root
// tango package.
//
//	go run ./internal/platform/darwin/internal/smokeattach          # interactive
//	go run ./internal/platform/darwin/internal/smokeattach -smoke   # self-check, exits 0 on PASS
//
// The page is blue. Canvas A sits above it at (40,30) 200×100 with
// PassThrough: its left half is transparent, its right half opaque red.
// Canvas B fills the window below the WebView and is green.
//
// With -smoke it checks: A's first frame is presented at its bounds ×
// scale; A sits at its bounds above the WebView; hit testing returns the
// WebView over A's transparent half and A over its opaque half; a
// synthetic click on the transparent half reaches the page (a JS message)
// and not A, a click on the opaque half reaches A and makes it first
// responder; mouse moves over the opaque half go to A only; a screenshot
// shows red over A's opaque half and the page elsewhere; attaching B turns
// the WebView's background off and a screenshot shows B through the
// transparent page; resizing the window resizes B and keeps A at its
// bounds; SetLayer moves and resizes A; detaching B restores the WebView
// background and makes B's Present fail; a still window produces no frame
// for 2 s; closing reaches the host and A's Present fails.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the overall deadlines.
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

const page = `<!doctype html><html><head><style>
html,body{margin:0;height:100%;background:rgb(0,0,255);color:#fff;font:14px sans-serif}
p{position:absolute;left:300px;top:20px;margin:0}
#hov{position:absolute;left:20px;top:20px;width:300px;height:150px}
</style></head><body><p>page</p><div id="hov"></div><script>
document.addEventListener('mousedown',e=>__tangoNative.post('down:'+e.clientX+','+e.clientY));
hov.addEventListener('mouseenter',()=>__tangoNative.post('hov-in'));
hov.addEventListener('mouseleave',()=>__tangoNative.post('hov-out'));
addEventListener('load',()=>__tangoNative.post('loaded'));
</script></body></html>`

var boundsA = platform.Rect{X: 40, Y: 30, Width: 200, Height: 100}

// hooks are the backend's test hooks (canvas_attach_smoke.go).
type hooks interface {
	SmokeAttachState(cv platform.Canvas) (darwin.SmokeAttachState, error)
	SmokeAttachHit(cv platform.Canvas, x, y float64) (string, error)
	SmokeAttachClick(cv platform.Canvas, x, y float64) error
	SmokeAttachMove(cv platform.Canvas, x, y float64) (bool, error)
	SmokeAttachPointer(cv platform.Canvas, x, y float64) (bool, error)
	SmokeCanvasDrop(cv platform.Canvas, paths []string, x, y float64) (uint, bool, error)
	SmokeCanvasEditMenu(cv platform.Canvas) (map[string]bool, error)
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
	hov     []string // hov-in / hov-out from the page
	drops   []platform.SurfaceEvent
	loaded  bool
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
	if ev.Kind == platform.SurfaceFilesDropped {
		h.logf("files dropped %v at %d,%d", ev.Paths, ev.X, ev.Y)
		h.drops = append(h.drops, ev)
	}
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
			h.after(300*time.Millisecond, h.stepLayout)
		}
		return
	}
	if strings.HasPrefix(msg, "hov-") {
		h.hov = append(h.hov, msg)
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
	paint  func(img *image.RGBA, scale float32)
	frames int
	errs   []error
	events []input.Event
	sizes  [][3]float32
	focus  []bool
	drag   []string
}

func (s *sink) CanvasDragOver(x, y float32) { s.drag = append(s.drag, fmt.Sprintf("over %g,%g", x, y)) }
func (s *sink) CanvasDragLeave()            { s.drag = append(s.drag, "leave") }

func (s *sink) CanvasFrame() {
	if s.cv == nil {
		s.h.fail(s.name+"-early", "CanvasFrame before AttachCanvas returned")
		return
	}
	w, ht := s.cv.Size()
	sc := s.cv.Scale()
	img := image.NewRGBA(image.Rect(0, 0, int(math.Round(float64(w*sc))), int(math.Round(float64(ht*sc)))))
	s.paint(img, sc)
	if err := s.cv.Present(img); err != nil {
		s.errs = append(s.errs, err)
	}
	s.frames++
	if s.frames == 1 {
		s.h.check(s.name+"-first", img.Rect.Dx() == int(math.Round(float64(w*sc))),
			fmt.Sprintf("%dx%d px for %gx%g @%g", img.Rect.Dx(), img.Rect.Dy(), w, ht, sc))
	}
}

func (s *sink) CanvasInput(ev input.Event) { s.events = append(s.events, ev) }
func (s *sink) CanvasResized(w, h, sc float32) {
	s.sizes = append(s.sizes, [3]float32{w, h, sc})
}
func (s *sink) CanvasFocus(f bool) { s.focus = append(s.focus, f) }

// paintA: left half transparent except a faint "shadow" (alpha 16) in its
// top 20 logical rows, right half opaque red.
func paintA(img *image.RGBA, scale float32) {
	b := img.Bounds()
	for y := range int(20 * scale) {
		for x := range b.Dx() / 2 {
			img.SetRGBA(x, y, color.RGBA{16, 0, 0, 16})
		}
	}
	for y := range b.Dy() {
		for x := b.Dx() / 2; x < b.Dx(); x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
}

func paintB(img *image.RGBA, scale float32) {
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
	s, err := h.b.(platform.CreatorEx).NewSurfaceEx(surfaceID, platform.SurfaceOptions{
		Title: "Tango attach smoke", Width: 480, Height: 320, Resizable: true, Visible: true,
		URL: h.b.AssetOrigin() + "/index.html",
	}, platform.SurfaceExtras{Centered: true, AcceptFiles: true})
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
		h.after(scaled(20*time.Second), func() {
			h.fail("timeout", "script did not finish")
			h.b.Quit()
		})
	}
}

func (h *host) state(cv platform.Canvas) darwin.SmokeAttachState {
	st, err := h.hk.SmokeAttachState(cv)
	if err != nil {
		h.fail("state", err.Error())
	}
	return st
}

func (h *host) stepLayout() {
	st := h.state(h.a.cv)
	h.check("bounds", st.InContainer && st.AboveWeb && st.Frame == [4]float64{40, 30, 200, 100},
		fmt.Sprintf("%+v", st))
	h.check("web-background", st.DrawsBackground, "WebView draws its background with only a canvas above")

	hit := func(x, y float64) string { r, _ := h.hk.SmokeAttachHit(h.a.cv, x, y); return r }
	ht, ho, hout := hit(60, 80), hit(180, 80), hit(300, 200)
	h.check("hit-test", ht == "web" && ho == "canvas" && hout == "web",
		fmt.Sprintf("transparent=%s opaque=%s outside=%s", ht, ho, hout))

	// Moves: over the opaque half the canvas alone gets them.
	sw1, _ := h.hk.SmokeAttachMove(h.a.cv, 180, 80)
	sw2, _ := h.hk.SmokeAttachMove(h.a.cv, 60, 80)
	var kinds []input.PointerKind
	for _, ev := range h.a.events {
		if p, ok := ev.(input.Pointer); ok {
			kinds = append(kinds, p.Kind)
		}
	}
	h.check("move-routing", sw1 && !sw2 && len(kinds) >= 2 && kinds[len(kinds)-1] == input.Leave,
		fmt.Sprintf("opaque swallowed=%v transparent swallowed=%v canvas pointer kinds=%v", sw1, sw2, kinds))

	// HitAlpha: the faint shadow (alpha 16) at (60, 40) takes clicks with
	// the default threshold and lets them through with HitAlpha 32.
	hs1 := hit(60, 40)
	ac := h.a.cv.(platform.AttachedCanvas)
	err := ac.SetLayer(platform.CanvasLayer{Bounds: boundsA, Above: true, PassThrough: true, HitAlpha: 32})
	hs2, ho2 := hit(60, 40), hit(180, 80)
	h.check("hit-alpha", err == nil && hs1 == "canvas" && hs2 == "web" && ho2 == "canvas",
		fmt.Sprintf("shadow default=%s HitAlpha32=%s, opaque=%s, err %v", hs1, hs2, ho2, err))
	h.stepHover()
}

// stepHover: the page's hover goes when the pointer moves onto the
// opaque overlay and comes back over the transparent part.
func (h *host) stepHover() {
	move := func(x, y float64) bool { sw, _ := h.hk.SmokeAttachPointer(h.a.cv, x, y); return sw }
	h.hov = nil
	move(60, 80)
	h.after(300*time.Millisecond, func() {
		in1 := slicesContains(h.hov, "hov-in")
		sw := move(180, 80)
		h.after(300*time.Millisecond, func() {
			out := slicesContains(h.hov, "hov-out")
			back := move(60, 80)
			h.after(300*time.Millisecond, func() {
				h.check("web-hover", in1 && sw && out && !back && len(h.hov) == 3 && h.hov[2] == "hov-in",
					fmt.Sprintf("page hover events %v (opaque move swallowed=%v)", h.hov, sw))
				h.stepClicks()
			})
		})
	})
}

func (h *host) stepClicks() {
	h.a.events = nil
	if err := h.hk.SmokeAttachClick(h.a.cv, 60, 80); err != nil {
		h.fail("click-through", err.Error())
	}
	h.after(400*time.Millisecond, func() {
		presses := 0
		for _, ev := range h.a.events {
			if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press {
				presses++
			}
		}
		h.check("click-through", presses == 0 && slicesContains(h.msgs, "down:60,80"),
			fmt.Sprintf("page messages %v, canvas presses %d", h.msgs, presses))
		h.a.events = nil
		if err := h.hk.SmokeAttachClick(h.a.cv, 180, 80); err != nil {
			h.fail("click-canvas", err.Error())
		}
		h.after(300*time.Millisecond, h.checkCanvasClick)
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
	var press *input.Pointer
	for _, ev := range h.a.events {
		if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press {
			press = &p
			break
		}
	}
	h.check("click-canvas", press != nil && press.Pos.X == 140 && press.Pos.Y == 50 && len(h.msgs) == 1,
		fmt.Sprintf("press %+v, page messages %v", press, h.msgs))
	st := h.state(h.a.cv)
	h.check("first-responder", st.FirstResponder, fmt.Sprintf("focus events %v", h.a.focus))
	h.checkDropEdit()
	h.shot("shot-above", func(px func(x, y int) color.RGBA) (bool, string) {
		red, hole, out := px(180, 80), px(60, 80), px(300, 200)
		ok := isRed(red) && isBlue(hole) && isBlue(out)
		return ok, fmt.Sprintf("opaque %v, transparent %v, outside %v", red, hole, out)
	}, h.stepUnderlay)
}

// checkDropEdit: files dropped on A's opaque half are reported in content
// coordinates; A's Edit menu items follow text input.
func (h *host) checkDropEdit() {
	h.a.drag = nil
	op, dropped, err := h.hk.SmokeCanvasDrop(h.a.cv, []string{"/tmp/x.txt"}, 150, 50)
	var got platform.SurfaceEvent
	if len(h.drops) > 0 {
		got = h.drops[len(h.drops)-1]
	}
	// A at (40, 30): view (150, 50) is content (190, 80).
	h.check("drop-files", err == nil && op == 1 && dropped && len(h.drops) == 1 && got.X == 190 && got.Y == 80 &&
		len(got.Paths) == 1 && got.Paths[0] == "/tmp/x.txt" && len(h.a.drag) >= 2 && h.a.drag[0] == "over 150,50",
		fmt.Sprintf("op %d dropped %v err %v event %+v feedback %v", op, dropped, err, got, h.a.drag))

	h.a.cv.SetTextInput(false, geom.Rect{})
	off, e1 := h.hk.SmokeCanvasEditMenu(h.a.cv)
	h.a.cv.SetTextInput(true, geom.R(150, 20, 2, 16))
	on, e2 := h.hk.SmokeCanvasEditMenu(h.a.cv)
	h.a.cv.SetTextInput(false, geom.Rect{})
	h.check("edit-menu", e1 == nil && e2 == nil && !off["copy:"] && !off["paste:"] && !off["undo:"] && !off["selectAll:"] &&
		on["copy:"] && on["cut:"] && on["undo:"] && on["redo:"] && on["selectAll:"] && on["paste:"] == on["pasteboard-text"],
		fmt.Sprintf("text off %v; text on %v", off, on))
}

func isRed(c color.RGBA) bool   { return c.R > 200 && c.G < 60 && c.B < 60 }
func isBlue(c color.RGBA) bool  { return c.B > 200 && c.R < 60 && c.G < 60 }
func isGreen(c color.RGBA) bool { return c.G > 150 && c.R < 60 && c.B < 60 }

// shot captures the window and runs check on content-area pixels
// (logical coordinates); a failed capture (no screen recording permission)
// is a skip.
func (h *host) shot(name string, check func(px func(x, y int) color.RGBA) (bool, string), next func()) {
	wn := objc.ID(h.surface.Native()).Send(objc.RegisterName("windowNumber"))
	scale := float64(h.a.cv.Scale())
	_, chh := h.surface.Size()
	go func() {
		file := fmt.Sprintf("/tmp/tango-smokeattach-%s.png", name)
		out, err := exec.Command("screencapture", "-x", "-o", fmt.Sprintf("-l%d", wn), file).CombinedOutput()
		var img image.Image
		if err == nil {
			if f, e := os.Open(file); e == nil {
				img, err = png.Decode(f)
				f.Close()
			} else {
				err = e
			}
		}
		h.post(func() {
			if err != nil {
				h.logf("SKIP %s: %v %s", name, err, bytes.TrimSpace(out))
				h.checks[name] = true
				next()
				return
			}
			b := img.Bounds()
			top := b.Max.Y - int(float64(chh)*scale)
			px := func(x, y int) color.RGBA {
				r, g, bl, a := img.At(b.Min.X+int(float64(x)*scale), top+int(float64(y)*scale)).RGBA()
				return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}
			}
			ok, msg := check(px)
			h.check(name, ok, msg+" ("+file+")")
			next()
		})
	}()
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
	h.check("attach-below", st.InContainer && !st.AboveWeb && !st.DrawsBackground && w == 480 && ht == 320,
		fmt.Sprintf("%+v size %gx%g", st, w, ht))
	h.surface.Eval(`document.documentElement.style.background='transparent';document.body.style.background='transparent'`)
	h.after(400*time.Millisecond, func() {
		h.shot("shot-below", func(px func(x, y int) color.RGBA) (bool, string) {
			g, red := px(300, 200), px(180, 80)
			return isGreen(g) && isRed(red), fmt.Sprintf("page area %v, canvas A %v", g, red)
		}, h.stepResize)
	})
}

func (h *host) stepResize() {
	nb := len(h.bb.sizes)
	fb := h.bb.frames
	h.surface.SetSize(640, 400)
	h.after(400*time.Millisecond, func() {
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
	err := ac.SetLayer(platform.CanvasLayer{Bounds: platform.Rect{X: 300, Y: 200, Width: 120, Height: 80}, Above: true, PassThrough: true})
	st := h.state(h.a.cv)
	h.after(300*time.Millisecond, func() {
		w, ht := h.a.cv.Size()
		h.check("set-layer", err == nil && st.Frame == [4]float64{300, 200, 120, 80} && st.AboveWeb && w == 120 && ht == 80 && len(h.a.sizes) > na,
			fmt.Sprintf("err %v frame %v size %gx%g resizes %v", err, st.Frame, w, ht, h.a.sizes[na:]))
		hit, _ := h.hk.SmokeAttachHit(h.a.cv, 400, 240)
		h.check("hit-after-move", hit == "canvas", "opaque half at the new place: "+hit)
		h.stepAnchor()
	})
}

// stepAnchor: a bottom-right anchored canvas keeps its distance to that
// corner, a centred one its offset from the centre, as the window resizes.
func (h *host) stepAnchor() {
	ac := h.a.cv.(platform.AttachedCanvas)
	// 640x400 now: 20 px from the right and bottom edges.
	err := ac.SetLayer(platform.CanvasLayer{Bounds: platform.Rect{X: 500, Y: 300, Width: 120, Height: 80}, Above: true, PassThrough: true,
		Anchor: platform.AnchorBottomRight})
	h.surface.SetSize(700, 450)
	h.after(300*time.Millisecond, func() {
		br := h.state(h.a.cv).Frame
		err2 := ac.SetLayer(platform.CanvasLayer{Bounds: platform.Rect{X: 260, Y: 185, Width: 120, Height: 80}, Above: true, PassThrough: true,
			Anchor: platform.AnchorCenter})
		h.surface.SetSize(600, 400)
		h.after(300*time.Millisecond, func() {
			c := h.state(h.a.cv).Frame
			h.check("anchor", err == nil && err2 == nil && br == [4]float64{560, 350, 120, 80} && c == [4]float64{210, 160, 120, 80},
				fmt.Sprintf("bottom-right %v (want 560,350), centre %v (want 210,160), errs %v %v", br, c, err, err2))
			h.stepDetach()
		})
	})
}

func (h *host) stepDetach() {
	cv := h.bb.cv.(platform.AttachedCanvas)
	cv.Detach()
	cv.Detach() // idempotent
	st := h.state(cv)
	err := cv.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	cv.Invalidate() // harmless
	h.check("detach", !st.InContainer && st.DrawsBackground && err != nil,
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
	"attach", "a-first", "bounds", "web-background", "hit-test", "move-routing", "click-through",
	"click-canvas", "first-responder", "shot-above", "attach-below", "b-first", "shot-below",
	"resize-fill", "resize-pinned", "set-layer", "hit-after-move", "detach", "idle", "closed",
	"after-close", "present-errors", "hit-alpha", "web-hover", "drop-files", "edit-menu", "anchor",
}

func main() {
	flag.Parse()
	if *smoke {
		go func() {
			time.Sleep(scaled(40 * time.Second))
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(2)
		}()
	}
	h := &host{b: darwin.New(), start: time.Now(), checks: map[string]bool{}}
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
