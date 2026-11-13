//go:build windows && (amd64 || arm64)

// Command smokecanvas drives a canvas surface of the Windows backend (the
// self-drawn UI window, docs/specs/canvas-surface.md) without the root
// tango package. It draws a gradient, a 1-device-pixel stripe pattern and
// a square under the mouse.
//
//	go run ./internal/platform/windows/internal/smokecanvas          # interactive
//	go run ./internal/platform/windows/internal/smokecanvas -smoke   # self-check, exits 0 on PASS
//
// With -smoke it checks: no sink callback before NewCanvasSurface returns;
// the first frame is presented at Size×Scale pixels; a capture of the
// canvas shows the drawn pixels unscaled (red on top, blue at the bottom,
// sharp 1-device-pixel stripes: the RGBA→BGRA swap and the DPI handling);
// a half-size frame stays pinned to the top-left; a resize reports
// CanvasResized and presents a new frame; Invalidate from another
// goroutine produces a frame; 1000 Invalidate calls in one second coalesce
// to at most one frame per display refresh; InvalidateAt fires on time and
// an earlier deadline replaces a later one; synthetic window messages
// (mouse, wheel, keys, WM_CHAR with a surrogate pair, IMR_QUERYCHARPOSITION)
// arrive as the expected input events; the OLE drop target (AcceptFiles)
// reports hover, leave and the dropped paths; a still window produces no frame
// for 2 s; closing reaches the host and Present fails afterwards.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the overall deadlines for slow
// machines (CI).
//
// The IME itself (composition with a real input method) and the system
// cursor shapes cannot be synthesized; check them interactively: run
// without -smoke, press Enter to turn text input on at the mouse position,
// type with Microsoft Pinyin and watch the log.
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
	"syscall"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/windows"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() }

const surfaceID platform.SurfaceID = 1

var smoke = flag.Bool("smoke", false, "run the automatic checks and exit")

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	findWindowEx         = user32.NewProc("FindWindowExW")
	sendMessage          = user32.NewProc("SendMessageW")
	clientToScreen       = user32.NewProc("ClientToScreen")
	getKeyboardState     = user32.NewProc("GetKeyboardState")
	setKeyboardState     = user32.NewProc("SetKeyboardState")
	systemParametersInfo = user32.NewProc("SystemParametersInfoW")
	getDC                = user32.NewProc("GetDC")
	releaseDC            = user32.NewProc("ReleaseDC")
	createCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection     = gdi32.NewProc("CreateDIBSection")
	selectObject         = gdi32.NewProc("SelectObject")
	bitBlt               = gdi32.NewProc("BitBlt")
	deleteObject         = gdi32.NewProc("DeleteObject")
	deleteDC             = gdi32.NewProc("DeleteDC")
)

const (
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmChar          = 0x0102
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmRButtonDown   = 0x0204
	wmRButtonUp     = 0x0205
	wmMouseWheel    = 0x020A
	wmImeRequest    = 0x0288
	imrQueryCharPos = 0x0006
	mkLButton       = 0x0001
	mkRButton       = 0x0002
	mkShift         = 0x0004
	vkControl       = 0x11
	vkLeft          = 0x25
	vkShift         = 0x10
	vkLShift        = 0xA0
	vkEnd           = 0x23
	vkF5            = 0x74
	vkNumpad1       = 0x61
	vkNumpad8       = 0x68
	vkUp            = 0x26
	vkOEM2          = 0xBF // '/' on the US layout
	srcCopy         = 0x00CC0020
)

type point struct{ X, Y int32 }

type host struct {
	b     platform.Backend
	mu    sync.Mutex
	queue []func()

	start  time.Time
	checks map[string]bool
	failed []string

	creating bool // inside NewCanvasSurface
	surface  platform.Surface
	cv       platform.Canvas
	child    uintptr // the canvas HWND
	sink     *sink
	dropped  []platform.SurfaceEvent // SurfaceFilesDropped
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
	case platform.SurfaceFilesDropped:
		h.logf("files dropped %v at %d,%d", ev.Paths, ev.X, ev.Y)
		h.dropped = append(h.dropped, ev)
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
	drag    []string // CanvasDragOver / CanvasDragLeave, as "over" / "leave"
}

// CanvasDragOver and CanvasDragLeave implement platform.CanvasDropTarget.
func (s *sink) CanvasDragOver(x, y float32) {
	if !s.early("CanvasDragOver") {
		s.drag = append(s.drag, "over")
	}
}

func (s *sink) CanvasDragLeave() {
	if !s.early("CanvasDragLeave") {
		s.drag = append(s.drag, "leave")
	}
}

func (s *sink) early(what string) bool {
	if s.h.creating || s.h.cv == nil {
		s.h.fail("no-early-callback", what+" before NewCanvasSurface returned")
		return true
	}
	return false
}

func (s *sink) CanvasFrame() {
	if s.early("CanvasFrame") {
		return
	}
	cv := s.h.cv
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
	if s.early("CanvasInput") {
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
			s.h.logf("text input on at %v", s.mouse)
		}
		if t, ok := ev.(input.KeyEvent); ok && t.Down && t.Key == input.KeyEscape {
			s.h.cv.SetTextInput(false, geom.Rect{})
			s.h.logf("text input off")
		}
		if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press && p.Button == input.ButtonRight {
			c := input.Cursor((int(p.Pos.X) / 40) % 9)
			s.h.cv.SetCursor(c)
			s.h.logf("cursor %d", c)
		}
	}
}

func (s *sink) CanvasResized(w, h, scale float32) {
	if s.early("CanvasResized") {
		return
	}
	s.resizes = append(s.resizes, [3]float32{w, h, scale})
	s.h.logf("CanvasResized %gx%g @%g", w, h, scale)
}

func (s *sink) CanvasFocus(f bool) {
	if s.early("CanvasFocus") {
		return
	}
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
	h.creating = true
	s, cv, err := cc.NewCanvasSurface(surfaceID, platform.SurfaceOptions{
		Title: "Tango canvas smoke", Width: 480, Height: 320, MinWidth: 200, MinHeight: 150,
		Resizable: true, Visible: true,
	}, platform.SurfaceExtras{
		Centered:    true,
		Appearance:  platform.Appearance{},
		AcceptFiles: true,
	}, h.sink)
	h.creating = false
	if err != nil {
		h.fail("canvas-creator", err.Error())
		h.b.Quit()
		return
	}
	h.surface, h.cv = s, cv
	h.child, _, _ = findWindowEx.Call(s.Native(), 0, uintptr(unsafe.Pointer(utf16("TangoCanvas"))), 0)
	w, ht := cv.Size()
	if h.child == 0 {
		h.fail("canvas-creator", "no TangoCanvas child window")
	} else {
		h.pass("canvas-creator", fmt.Sprintf("native=%#x child=%#x size=%gx%g scale=%g", s.Native(), h.child, w, ht, cv.Scale()))
	}
	if *smoke {
		h.after(scaled(15*time.Second), func() {
			h.fail("timeout", "script did not finish")
			h.b.Quit()
		})
	}
}

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func (h *host) onFirstFrame() {
	if !h.checks["no-early-callback"] && !contains(h.failed, "no-early-callback") {
		h.pass("no-early-callback", "first sink call after NewCanvasSurface returned")
	}
	f := h.sink.frames[0]
	if f.err != nil {
		h.fail("first-present", f.err.Error())
	} else {
		h.pass("first-present", fmt.Sprintf("%dx%d px", f.w, f.h))
	}
	if near(f.logW, 480) && near(f.logH, 320) && f.w == int(math.Round(480*float64(f.scale))) && f.h == int(math.Round(320*float64(f.scale))) {
		h.pass("size-scale", fmt.Sprintf("%gx%g @%g = %dx%d px", f.logW, f.logH, f.scale, f.w, f.h))
	} else {
		h.fail("size-scale", fmt.Sprintf("%+v", f))
	}
	if *smoke {
		h.after(500*time.Millisecond, h.stepScreenshot)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.51 }

// capture reads the canvas window's client pixels (BGRA) with BitBlt from
// its DC: a GDI window's DWM redirection surface holds what it painted.
func capture(hwnd uintptr, w, h int) ([]byte, error) {
	src, _, _ := getDC.Call(hwnd)
	if src == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer releaseDC.Call(hwnd, src)
	mem, _, _ := createCompatibleDC.Call(src)
	if mem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer deleteDC.Call(mem)
	type bitmapInfo struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
		Colors                 [1]uint32
	}
	bi := bitmapInfo{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits uintptr
	bmp, _, _ := createDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == 0 {
		return nil, fmt.Errorf("CreateDIBSection failed")
	}
	defer deleteObject.Call(bmp)
	old, _, _ := selectObject.Call(mem, bmp)
	r, _, _ := bitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), src, 0, 0, srcCopy)
	selectObject.Call(mem, old)
	if r == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}
	out := make([]byte, w*h*4)
	copy(out, unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&bits))), w*h*4)) // GDI memory, not Go's
	return out, nil
}

type shot struct {
	pix  []byte
	w, h int
}

func (s shot) rgb(x, y int) [3]int {
	i := (y*s.w + x) * 4
	return [3]int{int(s.pix[i+2]), int(s.pix[i+1]), int(s.pix[i])}
}

func (h *host) grab() (shot, error) {
	w, ht := h.cv.Size()
	sc := h.cv.Scale()
	pw, ph := int(math.Round(float64(w*sc))), int(math.Round(float64(ht*sc)))
	pix, err := capture(h.child, pw, ph)
	return shot{pix, pw, ph}, err
}

func (h *host) stepScreenshot() {
	s, err := h.grab()
	if err != nil {
		h.logf("SKIP screenshot: %v", err)
		h.stepResize()
		return
	}
	sc := h.cv.Scale()
	top, bottom := s.rgb(s.w-20, s.h/10), s.rgb(s.w-20, s.h*9/10)
	switch {
	case top == bottom:
		h.logf("SKIP screenshot: capture is uniform rgb%v (window covered or not composed?)", top)
		h.stepResize()
		return
	case !(top[0] > 150 && top[2] < 100 && bottom[2] > 150 && bottom[0] < 100):
		h.fail("screenshot", fmt.Sprintf("gradient wrong (channel swap?): top rgb%v bottom rgb%v", top, bottom))
	default:
		y := int(40 * sc)
		alt, n := 0, 0
		for x := int(30 * sc); x < int(90*sc); x++ {
			n++
			if (s.rgb(x, y)[0] > 128) != (s.rgb(x+1, y)[0] > 128) {
				alt++
			}
		}
		if alt < n*9/10 {
			h.fail("screenshot", fmt.Sprintf("stripes blurred: %d/%d device-pixel transitions", alt, n))
		} else {
			h.pass("screenshot", fmt.Sprintf("%dx%d; top rgb%v bottom rgb%v; %d/%d sharp 1px stripes", s.w, s.h, top, bottom, alt, n))
		}
	}
	h.stepPartial()
}

// stepPartial presents a half-size frame (as can happen for a moment
// during live resize) and checks it is pinned to the top-left, unscaled,
// with the rest filled by the frame's top-left color.
func (h *host) stepPartial() {
	w, ht := h.cv.Size()
	sc := h.cv.Scale()
	pw, ph := int(w*sc), int(ht*sc)
	half := image.NewRGBA(image.Rect(0, 0, pw/2, ph/2))
	draw(half, sc, geom.Pt(-100, -100))
	if err := h.cv.Present(half); err != nil {
		h.fail("partial-frame", err.Error())
		h.stepResize()
		return
	}
	h.after(150*time.Millisecond, func() {
		s, err := h.grab()
		if err != nil {
			h.fail("partial-frame", err.Error())
		} else {
			tl := s.rgb(pw/2-10, 10)
			bl := s.rgb(pw/2-10, ph/2-10)
			out := s.rgb(pw*3/4, ph*3/4)
			want := [3]int{int(half.Pix[0]), int(half.Pix[1]), int(half.Pix[2])}
			detail := fmt.Sprintf("half frame top rgb%v bottom rgb%v, outside rgb%v (fill rgb%v)", tl, bl, out, want)
			if tl[0] > 150 && tl[2] < 100 && bl[2] > 150 && bl[0] < 100 && out == want {
				h.pass("partial-frame", detail)
			} else {
				h.fail("partial-frame", detail)
			}
		}
		h.cv.Invalidate() // back to a full frame
		h.after(100*time.Millisecond, h.stepResize)
	})
}

func (h *host) stepResize() {
	n := len(h.sink.frames)
	r := len(h.sink.resizes)
	h.surface.SetSize(640, 400)
	h.after(400*time.Millisecond, func() {
		sc := h.cv.Scale()
		gotResize := false
		for _, rs := range h.sink.resizes[r:] {
			if near(rs[0], 640) && near(rs[1], 400) && rs[2] == sc {
				gotResize = true
			}
		}
		var fr *frameInfo
		for i := n; i < len(h.sink.frames); i++ {
			if f := h.sink.frames[i]; f.w == int(math.Round(640*float64(sc))) && f.h == int(math.Round(400*float64(sc))) && f.err == nil {
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
			minGap := time.Hour
			for i := 1; i < d; i++ {
				minGap = min(minGap, fr[i].at.Sub(fr[i-1].at))
			}
			if d > 1 {
				span = fr[d-1].at.Sub(fr[0].at)
			}
			detail := fmt.Sprintf("%d Invalidate calls → %d frames over %v, min gap %v", calls.Load(), d, span.Round(time.Millisecond), minGap.Round(100*time.Microsecond))
			// At most one frame per refresh: ≤ 250 Hz displays assumed.
			if d >= 10 && int64(d) <= calls.Load()/4 && minGap >= 3*time.Millisecond {
				h.pass("coalesce", detail)
			} else {
				h.fail("coalesce", detail)
			}
			h.stepInvalidateAt()
		})
	}()
}

func (h *host) stepInvalidateAt() {
	h.sink.quiet = true // from here on, real mouse movement must not cause frames
	h.after(200*time.Millisecond, func() {
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

func (h *host) send(m uint32, wp, lp uintptr) uintptr {
	r, _, _ := sendMessage.Call(h.child, uintptr(m), wp, lp)
	return r
}

// lp packs a client position (logical) into a mouse message lParam.
func (h *host) lp(x, y float32) uintptr {
	sc := h.cv.Scale()
	px, py := int16(math.Round(float64(x*sc))), int16(math.Round(float64(y*sc)))
	return uintptr(uint16(px)) | uintptr(uint16(py))<<16
}

func (h *host) screenLP(x, y float32) uintptr {
	sc := h.cv.Scale()
	p := point{int32(math.Round(float64(x * sc))), int32(math.Round(float64(y * sc)))}
	clientToScreen.Call(h.child, uintptr(unsafe.Pointer(&p)))
	return uintptr(uint16(int16(p.X))) | uintptr(uint16(int16(p.Y)))<<16
}

func (h *host) stepInput() {
	if h.child == 0 {
		h.fail("input", "no canvas child window")
		h.stepIdle()
		return
	}
	h.sink.quiet = true
	h.sink.events = nil

	// Mouse: move, Shift+double click, right click.
	h.send(wmMouseMove, 0, h.lp(40, 20))
	h.send(wmLButtonDown, mkLButton|mkShift, h.lp(100, 60))
	h.send(wmLButtonUp, mkShift, h.lp(100, 60))
	h.send(wmLButtonDown, mkLButton|mkShift, h.lp(100, 60))
	h.send(wmLButtonUp, mkShift, h.lp(100, 60))
	h.send(wmRButtonDown, mkRButton, h.lp(20, 20))
	h.send(wmRButtonUp, 0, h.lp(20, 20))

	// Wheel: one notch towards the user, then a quarter notch.
	var lines uint32 = 3
	systemParametersInfo.Call(0x0068, 0, uintptr(unsafe.Pointer(&lines)), 0) // SPI_GETWHEELSCROLLLINES
	wheel := func(delta int16) { h.send(wmMouseWheel, uintptr(uint16(delta))<<16, h.screenLP(60, 60)) }
	wheel(-120)
	wheel(-30)

	// Keys: Left down/up, Ctrl+C (GetKeyState reads the thread's key state).
	h.send(wmKeyDown, vkLeft, 1)
	h.send(wmKeyUp, vkLeft, 0xC0000001)
	var ks, saved [256]byte
	getKeyboardState.Call(uintptr(unsafe.Pointer(&ks[0])))
	saved = ks
	ks[vkControl] = 0x80
	setKeyboardState.Call(uintptr(unsafe.Pointer(&ks[0])))
	h.send(wmKeyDown, 'C', 1)
	h.send(wmKeyUp, 'C', 0xC0000001)
	setKeyboardState.Call(uintptr(unsafe.Pointer(&saved[0])))

	// New keys (lParam: repeat 1, scan code in bits 16-23, extended bit 24):
	// F5; Left Shift down (key state already down, as during the real
	// WM_KEYDOWN) and up; keypad 1 with NumLock on and off; '/'.
	keyLP := func(scan uintptr, ext bool) uintptr {
		v := 1 | scan<<16
		if ext {
			v |= 1 << 24
		}
		return v
	}
	h.send(wmKeyDown, vkF5, keyLP(0x3F, false))
	h.send(wmKeyUp, vkF5, keyLP(0x3F, false)|0xC0000000)
	ks = saved
	ks[vkShift], ks[vkLShift] = 0x80, 0x80
	setKeyboardState.Call(uintptr(unsafe.Pointer(&ks[0])))
	h.send(wmKeyDown, vkShift, keyLP(0x2A, false))
	setKeyboardState.Call(uintptr(unsafe.Pointer(&saved[0])))
	h.send(wmKeyUp, vkShift, keyLP(0x2A, false)|0xC0000000)
	h.send(wmKeyDown, vkNumpad1, keyLP(0x4F, false))
	h.send(wmKeyUp, vkNumpad1, keyLP(0x4F, false)|0xC0000000)
	h.send(wmKeyDown, vkEnd, keyLP(0x4F, false)) // NumLock off
	h.send(wmKeyUp, vkEnd, keyLP(0x4F, false)|0xC0000000)
	h.send(wmKeyDown, vkEnd, keyLP(0x4F, true)) // the navigation block End
	h.send(wmKeyUp, vkEnd, keyLP(0x4F, true)|0xC0000000)
	// Keypad 8 pressed with NumLock off, released after NumLock went on:
	// the release still reports Up with Code Numpad8.
	h.send(wmKeyDown, vkUp, keyLP(0x48, false))
	h.send(wmKeyUp, vkNumpad8, keyLP(0x48, false)|0xC0000000)
	h.send(wmKeyDown, vkOEM2, keyLP(0x35, false))
	h.send(wmKeyUp, vkOEM2, keyLP(0x35, false)|0xC0000000)

	// Text: off → WM_CHAR ignored; on → text, surrogate pair joined,
	// Backspace dropped; IMR_QUERYCHARPOSITION at the caret.
	h.send(wmChar, 'x', 1)
	caret := geom.R(40, 80, 2, 20)
	h.cv.SetTextInput(true, caret)
	for _, u := range []uint16{'h', 'i', 0xD83D, 0xDE00, 0x08} {
		h.send(wmChar, uintptr(u), 1)
	}
	type imeCharPosition struct {
		Size, CharPos uint32
		Pt            point
		LineHeight    uint32
		Document      [4]int32
	}
	q := imeCharPosition{Size: uint32(unsafe.Sizeof(imeCharPosition{}))}
	ok := h.send(wmImeRequest, imrQueryCharPos, uintptr(unsafe.Pointer(&q)))
	sc := h.cv.Scale()
	want := point{int32(math.Round(float64(caret.X * sc))), int32(math.Round(float64(caret.Y * sc)))}
	clientToScreen.Call(h.child, uintptr(unsafe.Pointer(&want)))
	if ok == 1 && q.Pt == want && q.LineHeight == uint32(math.Round(float64(caret.H*sc))) {
		h.pass("ime-char-position", fmt.Sprintf("caret at screen %v, line height %d", q.Pt, q.LineHeight))
	} else {
		h.fail("ime-char-position", fmt.Sprintf("ret=%d %+v, want pt %v", ok, q, want))
	}
	h.cv.SetTextInput(false, geom.Rect{})

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
	at := func(p geom.Point, x, y float32) bool { return near(p.X, x) && near(p.Y, y) }
	has("mouse-move", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Move && at(p.Pos, 40, 20)
	})
	has("mouse-press", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonLeft && at(p.Pos, 100, 60) && p.Clicks == 2 && p.Mods&input.ModShift != 0
	})
	has("mouse-release", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Release && p.Button == input.ButtonLeft && p.Clicks == 2
	})
	has("mouse-right", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonRight && at(p.Pos, 20, 20)
	})
	notch := float32(lines) * 40
	if lines == 0xFFFFFFFF {
		_, notch = h.cv.Size()
	}
	has("scroll-notch", func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DX == 0 && near(s.DY, notch) && at(s.Pos, 60, 60)
	})
	has("scroll-fraction", func(e input.Event) bool {
		s, ok := e.(input.Scroll)
		return ok && s.DX == 0 && near(s.DY, notch/4)
	})
	has("key-left", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && k.Down && k.Mods == 0
	})
	has("key-left-up", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && !k.Down
	})
	has("key-ctrl-c", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyC && k.Down && k.Mods == input.ModCtrl
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
	var n1, nEnd, nEndUp, nNavEnd, n8Up int
	for _, e := range evs {
		k, ok := e.(input.KeyEvent)
		switch {
		case !ok:
		case k.Key == input.KeyNumpad1 && k.Code == 0 && k.Down:
			n1++
		case k.Key == input.KeyEnd && k.Code == input.KeyNumpad1 && k.Down:
			nEnd++
		case k.Key == input.KeyEnd && k.Code == input.KeyNumpad1 && !k.Down:
			nEndUp++
		case k.Key == input.KeyEnd && k.Code == 0 && k.Down:
			nNavEnd++
		case k.Key == input.KeyUp && k.Code == input.KeyNumpad8 && !k.Down:
			n8Up++
		}
	}
	if n1 == 1 {
		h.pass("key-numpad1", "VK_NUMPAD1 (NumLock on) is Numpad1 with no Code")
	} else {
		h.fail("key-numpad1", fmt.Sprintf("%d Numpad1 presses, want 1", n1))
	}
	if nEnd == 1 && nEndUp == 1 && nNavEnd == 1 && n8Up == 1 {
		h.pass("key-numlock-off", "NumLock-off VK_END (scan 0x4F, not extended) is End with Code Numpad1 down and up; "+
			"the extended End has no Code; keypad 8 released after NumLock changed keeps Up/Numpad8")
	} else {
		h.fail("key-numlock-off", fmt.Sprintf("End/Numpad1 down %d up %d, End down %d, Up/Numpad8 up %d; want 1 each",
			nEnd, nEndUp, nNavEnd, n8Up))
	}
	has("key-slash", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeySlash && k.Down
	})
	has("text-input", func(e input.Event) bool {
		t, ok := e.(input.TextInput)
		return ok && t.Text == "h"
	})
	has("text-surrogate", func(e input.Event) bool {
		t, ok := e.(input.TextInput)
		return ok && t.Text == "😀"
	})
	var texts []string
	for _, e := range evs {
		if t, ok := e.(input.TextInput); ok {
			texts = append(texts, t.Text)
		}
	}
	if strings.Join(texts, "") == "hi😀" {
		h.pass("text-filter", fmt.Sprintf("%q (WM_CHAR ignored while off, Backspace dropped)", texts))
	} else {
		h.fail("text-filter", fmt.Sprintf("%q", texts))
	}
	h.stepDrop()
}

func (h *host) check(name string, ok bool, detail string) {
	if ok {
		h.pass(name, detail)
	} else {
		h.fail(name, detail)
	}
}

// stepDrop drives the canvas's OLE drop target with a CF_HDROP data
// object (the backend's SmokeCanvasDrop hook): a drag that leaves again
// reports hover and leave but no drop; a drop reports the paths.
func (h *host) stepDrop() {
	hook, ok := h.b.(interface {
		SmokeCanvasDrop(cv platform.Canvas, paths []string, cancel bool) (uint32, error)
	})
	if !ok {
		h.fail("file-drop", "no SmokeCanvasDrop hook")
		h.stepIdle()
		return
	}
	paths := []string{`C:\Windows\win.ini`, `C:\Temp\tango smoke ü.txt`}
	h.sink.drag = nil
	eff, err := hook.SmokeCanvasDrop(h.cv, paths, true)
	h.check("drop-cancel", err == nil && eff == 1 && len(h.dropped) == 0 && strings.Join(h.sink.drag, ",") == "over,over,leave",
		fmt.Sprintf("effect %d err %v hover %v drops %d", eff, err, h.sink.drag, len(h.dropped)))
	h.sink.drag = nil
	eff, err = hook.SmokeCanvasDrop(h.cv, paths, false)
	got := ""
	if len(h.dropped) == 1 {
		got = strings.Join(h.dropped[0].Paths, "|")
	}
	h.check("file-drop", err == nil && eff == 1 && got == strings.Join(paths, "|") && strings.Join(h.sink.drag, ",") == "over,over,leave",
		fmt.Sprintf("effect %d err %v paths %q hover %v", eff, err, got, h.sink.drag))
	h.stepIdle()
}

func (h *host) stepIdle() {
	h.surface.Focus() // try to become the foreground window for the focus check
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
				h.logf("SKIP focus: %v (window may not have become the foreground window)", h.sink.focus)
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
	"canvas-creator", "no-early-callback", "first-present", "size-scale", "resize", "invalidate",
	"coalesce", "invalidate-at", "mouse-move", "mouse-press", "mouse-release", "mouse-right",
	"scroll-notch", "scroll-fraction", "key-left", "key-left-up", "key-ctrl-c", "key-f5",
	"key-shift-left-down", "key-shift-left-up", "key-numpad1", "key-numlock-off", "key-slash", "text-input",
	"text-surrogate", "text-filter", "ime-char-position", "drop-cancel", "file-drop", "idle", "closed", "after-close",
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
	h := &host{b: windows.New(), start: time.Now(), checks: map[string]bool{}}
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
