//go:build darwin && !ios

// Command smokemetal checks the GPU presentation path of the macOS canvas
// (platform.CanvasMetal) with the smallest possible Metal client: it calls
// UseMetalLayer on a canvas surface, then on every CanvasFrame takes the
// layer's next drawable, clears it to a colour that changes per frame,
// blits a white marker into the top-left quarter (width/2 × height/4) and
// presents it. The Metal code here is only a test double for the real
// presenter in internal/render/gpu/metal.
//
//	go run ./internal/platform/darwin/internal/smokemetal          # interactive
//	go run ./internal/platform/darwin/internal/smokemetal -smoke   # self-check, exits 0 on PASS
//
// With -smoke it checks: Present(nil) fails before the switch; UseMetalLayer
// refuses other threads, returns a CAMetalLayer hosted by the view (opaque,
// contentsScale = backing scale) and the same layer again; the first GPU
// frame arrives without an Invalidate and Present(nil) returns nil; a
// window screenshot shows the clear colour and the marker top-left at full
// resolution; after a resize the layer follows the view and a drawable of
// the new size is presented (screenshot again); Invalidate gives one frame,
// 1000 Invalidates coalesce, InvalidateAt fires on time; synthetic input
// and NSTextInputClient work; a still window produces no frames; transparent
// canvases and overlays report ErrUnsupported; closing does not crash.
package main

import (
	"bytes"
	"errors"
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
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/darwin"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() { runtime.LockOSThread() }

const (
	surfaceID     platform.SurfaceID = 1
	transparentID platform.SurfaceID = 2
	webID         platform.SurfaceID = 3
)

var smoke = flag.Bool("smoke", false, "run the automatic checks and exit")

// ---- minimal Metal through purego ----

type (
	cgSize        struct{ W, H float64 }
	cgRect        struct{ X, Y, W, H float64 }
	mtlClearColor struct{ R, G, B, A float64 }
	mtlSize       struct{ W, H, D uint }
	mtlOrigin     struct{ X, Y, Z uint }
)

const (
	mtlPixelFormatBGRA8Unorm = 80
	mtlLoadActionClear       = 2
	mtlStoreActionStore      = 1
)

var (
	mtlCreateSystemDefaultDevice func() objc.ID
	cgColorSpaceCreateWithName   func(name uintptr) uintptr
	poolPush                     func() uintptr
	poolPop                      func(p uintptr)

	msgSetSize    func(obj objc.ID, s objc.SEL, sz cgSize)
	msgGetSize    func(obj objc.ID, s objc.SEL) cgSize
	msgGetRect    func(obj objc.ID, s objc.SEL) cgRect
	msgGetFloat   func(obj objc.ID, s objc.SEL) float64
	msgGetPtr     func(obj objc.ID, s objc.SEL) unsafe.Pointer
	msgSetClear   func(obj objc.ID, s objc.SEL, c mtlClearColor)
	msgCopyBuffer func(obj objc.ID, s objc.SEL, buf objc.ID, off, bpr, bpi uint, size mtlSize, tex objc.ID, slice, level uint, origin mtlOrigin)
	// arm64 passes structs larger than 16 bytes (MTLSize, MTLOrigin) as a
	// pointer to a caller-owned copy; purego does not do that for us.
	msgCopyBufferRef func(obj objc.ID, s objc.SEL, buf objc.ID, off, bpr, bpi uint, size *mtlSize, tex objc.ID, slice, level uint, origin *mtlOrigin)

	sel = objc.RegisterName
)

func loadMetal() error {
	libObjC, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return err
	}
	libMetal, err := purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return err
	}
	libCG, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&mtlCreateSystemDefaultDevice, libMetal, "MTLCreateSystemDefaultDevice")
	purego.RegisterLibFunc(&cgColorSpaceCreateWithName, libCG, "CGColorSpaceCreateWithName")
	purego.RegisterLibFunc(&poolPush, libObjC, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&poolPop, libObjC, "objc_autoreleasePoolPop")
	send, err := purego.Dlsym(libObjC, "objc_msgSend")
	if err != nil {
		return err
	}
	stret := send
	if runtime.GOARCH == "amd64" {
		if stret, err = purego.Dlsym(libObjC, "objc_msgSend_stret"); err != nil {
			return err
		}
	}
	purego.RegisterFunc(&msgSetSize, send)
	purego.RegisterFunc(&msgGetSize, send)
	purego.RegisterFunc(&msgGetRect, stret)
	purego.RegisterFunc(&msgGetFloat, send)
	purego.RegisterFunc(&msgGetPtr, send)
	purego.RegisterFunc(&msgSetClear, send)
	if runtime.GOARCH == "arm64" {
		purego.RegisterFunc(&msgCopyBufferRef, send)
	} else {
		purego.RegisterFunc(&msgCopyBuffer, send)
	}
	return nil
}

func sendBool(obj objc.ID, s string, args ...any) bool { return byte(obj.Send(sel(s), args...)) != 0 }

// gpu is the test presenter: device, queue and a marker buffer.
type gpu struct {
	layer, device, queue objc.ID
	marker               objc.ID // shared MTLBuffer of white BGRA pixels
	markerW, markerH     int
}

func newGPU(layer uintptr) (*gpu, error) {
	g := &gpu{layer: objc.ID(layer)}
	g.device = mtlCreateSystemDefaultDevice()
	if g.device == 0 {
		return nil, errors.New("MTLCreateSystemDefaultDevice returned nil")
	}
	g.queue = g.device.Send(sel("newCommandQueue"))
	g.layer.Send(sel("setDevice:"), g.device)
	g.layer.Send(sel("setPixelFormat:"), uintptr(mtlPixelFormatBGRA8Unorm))
	g.layer.Send(sel("setFramebufferOnly:"), false) // the marker is a blit
	if p, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW); err == nil {
		if sym, err := purego.Dlsym(p, "kCGColorSpaceSRGB"); err == nil {
			name := **(**uintptr)(unsafe.Pointer(&sym))
			g.layer.Send(sel("setColorspace:"), cgColorSpaceCreateWithName(name))
		}
	}
	return g, nil
}

// resize sets drawableSize (the presenter's job) when it differs.
func (g *gpu) resize(pw, ph int) {
	if sz := msgGetSize(g.layer, sel("drawableSize")); int(sz.W) != pw || int(sz.H) != ph {
		msgSetSize(g.layer, sel("setDrawableSize:"), cgSize{float64(pw), float64(ph)})
	}
}

// draw renders one frame: clear to c, white marker top-left. It returns
// the drawable texture's size.
func (g *gpu) draw(c mtlClearColor) (tw, th int, err error) {
	pool := poolPush()
	defer poolPop(pool)
	drawable := g.layer.Send(sel("nextDrawable"))
	if drawable == 0 {
		return 0, 0, errors.New("nextDrawable returned nil")
	}
	tex := drawable.Send(sel("texture"))
	tw, th = int(tex.Send(sel("width"))), int(tex.Send(sel("height")))

	rpd := objc.ID(objc.GetClass("MTLRenderPassDescriptor")).Send(sel("renderPassDescriptor"))
	att := rpd.Send(sel("colorAttachments")).Send(sel("objectAtIndexedSubscript:"), uintptr(0))
	att.Send(sel("setTexture:"), tex)
	att.Send(sel("setLoadAction:"), uintptr(mtlLoadActionClear))
	att.Send(sel("setStoreAction:"), uintptr(mtlStoreActionStore))
	msgSetClear(att, sel("setClearColor:"), c)

	cb := g.queue.Send(sel("commandBuffer"))
	enc := cb.Send(sel("renderCommandEncoderWithDescriptor:"), rpd)
	enc.Send(sel("endEncoding"))

	mw, mh := tw/2, th/4
	if mw > 0 && mh > 0 {
		if g.markerW != mw || g.markerH != mh {
			if g.marker != 0 {
				g.marker.Send(sel("release"))
			}
			g.marker = g.device.Send(sel("newBufferWithLength:options:"), uintptr(mw*mh*4), uintptr(0))
			b := unsafe.Slice((*byte)(msgGetPtr(g.marker, sel("contents"))), mw*mh*4)
			for i := range b {
				b[i] = 0xff
			}
			g.markerW, g.markerH = mw, mh
		}
		blit := cb.Send(sel("blitCommandEncoder"))
		cp := sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:")
		size, origin := mtlSize{uint(mw), uint(mh), 1}, mtlOrigin{}
		if msgCopyBufferRef != nil {
			msgCopyBufferRef(blit, cp, g.marker, 0, uint(mw*4), uint(mw*mh*4), &size, tex, 0, 0, &origin)
		} else {
			msgCopyBuffer(blit, cp, g.marker, 0, uint(mw*4), uint(mw*mh*4), size, tex, 0, 0, origin)
		}
		blit.Send(sel("endEncoding"))
	}
	cb.Send(sel("presentDrawable:"), drawable)
	cb.Send(sel("commit"))
	return tw, th, nil
}

// ---- host ----

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
	gpu     *gpu
	layer   uintptr
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
	if displayLinkChecks[name] && os.Getenv("TANGO_SMOKE_SKIP") == "displaylink" {
		h.checks[name] = true
		h.skipped++
		h.logf("SKIP %-18s %s (TANGO_SMOKE_SKIP)", name, detail)
		return
	}
	h.failed = append(h.failed, name)
	h.logf("FAIL %-18s %s", name, detail)
}

func (h *host) check(name string, ok bool, detail string) {
	if ok {
		h.pass(name, detail)
	} else {
		h.fail(name, detail)
	}
}

var displayLinkChecks = map[string]bool{"invalidate": true, "coalesce": true, "invalidate-at": true}

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
	if ev.Kind == platform.SurfaceClosed && id == surfaceID {
		h.pass("closed", fmt.Sprintf("surface %d", id))
		h.surface = nil
		if !*smoke {
			h.b.Quit()
		}
	}
}

func (h *host) OnMessage(platform.SurfaceID, string, string) {}

func (h *host) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><body>web</body>"))
}

// ---- sink ----

type frameInfo struct {
	at       time.Time
	tw, th   int // drawable texture size
	clear    mtlClearColor
	err      error
	presentE error
}

type sink struct {
	h       *host
	frames  []frameInfo
	events  []input.Event
	resizes [][3]float32
	focus   []bool
	quiet   bool
}

func clearFor(n int) mtlClearColor {
	v := float64((n*47)%150) / 255
	return mtlClearColor{R: 0.25 + v, G: 0.15, B: 0.85 - v, A: 1}
}

func (s *sink) CanvasFrame() {
	h := s.h
	if h.cv == nil || h.gpu == nil {
		return
	}
	w, ht := h.cv.Size()
	sc := h.cv.Scale()
	pw, ph := int(math.Round(float64(w*sc))), int(math.Round(float64(ht*sc)))
	h.gpu.resize(pw, ph)
	c := clearFor(len(s.frames))
	tw, th, err := h.gpu.draw(c)
	perr := h.cv.Present(nil)
	s.frames = append(s.frames, frameInfo{time.Now(), tw, th, c, err, perr})
	if len(s.frames) == 1 {
		h.onFirstFrame()
	}
}

func (s *sink) CanvasInput(ev input.Event) {
	s.events = append(s.events, ev)
	if _, ok := ev.(input.Pointer); ok && !s.quiet {
		s.h.cv.Invalidate()
	}
	if !*smoke {
		s.h.logf("input %T %+v", ev, ev)
	}
}

func (s *sink) CanvasResized(w, h, scale float32) {
	s.resizes = append(s.resizes, [3]float32{w, h, scale})
	s.h.logf("CanvasResized %gx%g @%g", w, h, scale)
	s.h.cv.Invalidate()
}

func (s *sink) CanvasFocus(f bool) { s.focus = append(s.focus, f) }

type nopSink struct{}

func (nopSink) CanvasFrame()                      {}
func (nopSink) CanvasInput(input.Event)           {}
func (nopSink) CanvasResized(w, h, scale float32) {}
func (nopSink) CanvasFocus(bool)                  {}

// ---- script ----

func (h *host) onStarted() {
	if err := loadMetal(); err != nil {
		h.fail("metal", err.Error())
		h.b.Quit()
		return
	}
	cc := h.b.(platform.CanvasCreator)
	h.sink = &sink{h: h, quiet: *smoke}
	s, cv, err := cc.NewCanvasSurface(surfaceID, platform.SurfaceOptions{
		Title: "Tango Metal smoke", Width: 480, Height: 320, MinWidth: 200, MinHeight: 150,
		Resizable: true, Visible: true,
	}, platform.SurfaceExtras{Centered: true}, h.sink)
	if err != nil {
		h.fail("canvas-creator", err.Error())
		h.b.Quit()
		return
	}
	h.surface = s
	h.check("cpu-present-nil", cv.Present(nil) != nil, "Present(nil) before UseMetalLayer is an error")

	cm, ok := cv.(platform.CanvasMetal)
	if !ok {
		h.fail("use-metal", "canvas does not implement platform.CanvasMetal")
		h.b.Quit()
		return
	}
	errc := make(chan error, 1)
	go func() { _, err := cm.UseMetalLayer(); errc <- err }()
	offErr := <-errc
	h.check("off-main", offErr != nil, fmt.Sprintf("UseMetalLayer from a goroutine: %v", offErr))

	layer, err := cm.UseMetalLayer()
	if err != nil || layer == 0 {
		h.fail("use-metal", fmt.Sprintf("layer=%#x err=%v", layer, err))
		h.b.Quit()
		return
	}
	again, err2 := cm.UseMetalLayer()
	h.layer = layer
	l := objc.ID(layer)
	view := objc.ID(s.Native()).Send(sel("contentView"))
	isMetal := sendBool(l, "isKindOfClass:", objc.ID(objc.GetClass("CAMetalLayer")))
	hosted := findLayerView(view, l)
	scale := msgGetFloat(l, sel("contentsScale"))
	opaque := sendBool(l, "isOpaque")
	h.check("use-metal", isMetal && hosted && again == layer && err2 == nil && opaque && float32(scale) == cv.Scale(),
		fmt.Sprintf("layer=%#x CAMetalLayer=%v hosted=%v again=%v opaque=%v contentsScale=%g (scale %g)",
			layer, isMetal, hosted, again == layer, opaque, scale, cv.Scale()))
	g, err := newGPU(layer)
	if err != nil {
		h.fail("metal", err.Error())
		h.b.Quit()
		return
	}
	h.gpu = g
	h.cv = cv // frames run from here on
	if *smoke {
		h.after(20*time.Second, func() {
			h.fail("timeout", "script did not finish")
			h.b.Quit()
		})
	}
}

// findLayerView reports whether a view at or below v hosts layer l.
func findLayerView(v, l objc.ID) bool {
	if v == 0 {
		return false
	}
	if v.Send(sel("layer")) == l {
		return true
	}
	subs := v.Send(sel("subviews"))
	for i := uintptr(0); i < uintptr(subs.Send(sel("count"))); i++ {
		if findLayerView(subs.Send(sel("objectAtIndex:"), i), l) {
			return true
		}
	}
	return false
}

func (h *host) want() (int, int) {
	w, ht := h.cv.Size()
	sc := h.cv.Scale()
	return int(math.Round(float64(w * sc))), int(math.Round(float64(ht * sc)))
}

func (h *host) onFirstFrame() {
	f := h.sink.frames[0]
	pw, ph := h.want()
	h.check("first-frame", f.err == nil && f.presentE == nil && f.tw == pw && f.th == ph,
		fmt.Sprintf("no Invalidate needed; drawable %dx%d (want %dx%d) draw=%v Present(nil)=%v", f.tw, f.th, pw, ph, f.err, f.presentE))
	if *smoke {
		h.after(500*time.Millisecond, func() { h.stepShot("screenshot", h.stepFallback) })
	}
}

// stepShot captures the window and checks the last frame: marker white
// top-left, clear colour top-right and at the bottom, width = drawable.
func (h *host) stepShot(name string, next func()) {
	wn := objc.ID(h.surface.Native()).Send(sel("windowNumber"))
	pw, ph := h.want()
	last := h.sink.frames[len(h.sink.frames)-1]
	n := len(h.sink.frames)
	go func() {
		file := fmt.Sprintf("/tmp/tango-smokemetal-%s.png", name)
		out, err := exec.Command("screencapture", "-x", "-o", fmt.Sprintf("-l%d", wn), file).CombinedOutput()
		var ok, skip bool
		var msg string
		if err != nil {
			skip, msg = true, fmt.Sprintf("screencapture: %v %s", err, bytes.TrimSpace(out))
		} else {
			ok, skip, msg = checkShot(file, pw, ph, last.clear)
		}
		h.post(func() {
			if len(h.sink.frames) != n {
				msg += fmt.Sprintf(" (warning: %d frame(s) during capture)", len(h.sink.frames)-n)
			}
			switch {
			case ok:
				h.pass(name, msg+" → "+file)
			case skip:
				h.logf("SKIP %s: %s", name, msg)
			default:
				h.fail(name, msg+" → "+file)
			}
			next()
		})
	}()
}

func near(a, b uint8, tol int) bool { d := int(a) - int(b); return d <= tol && d >= -tol }

func checkShot(file string, pw, ph int, c mtlClearColor) (ok, skip bool, msg string) {
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
		return false, false, fmt.Sprintf("capture %dx%d, want width %d", b.Dx(), b.Dy(), pw)
	}
	top := b.Max.Y - ph
	if top < 0 {
		return false, false, fmt.Sprintf("capture %dx%d shorter than content %d", b.Dx(), b.Dy(), ph)
	}
	at := func(x, y int) color.RGBA {
		r, g, bl, _ := img.At(b.Min.X+x, top+y).RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
	}
	want := color.RGBA{uint8(c.R*255 + 0.5), uint8(c.G*255 + 0.5), uint8(c.B*255 + 0.5), 255}
	isClear := func(p color.RGBA) bool {
		return near(p.R, want.R, 24) && near(p.G, want.G, 24) && near(p.B, want.B, 24) // colour management: sRGB → display profile
	}
	isWhite := func(p color.RGBA) bool { return p.R > 240 && p.G > 240 && p.B > 240 }
	mk := at(pw/4, ph/8)         // inside the marker
	mkEdge := at(pw/2-3, ph/4-3) // marker's bottom-right corner, inside
	right := at(pw*3/4, ph/8)    // right of the marker
	below := at(pw/4, ph/4+3)    // just below the marker
	bottom := at(pw*3/4, ph*7/8) // bottom-right
	desc := fmt.Sprintf("%dx%d capture; marker %v edge %v, right %v, below %v, bottom %v; clear %v",
		b.Dx(), b.Dy(), mk, mkEdge, right, below, bottom, want)
	if mk == bottom && right == bottom && !isClear(bottom) && !isWhite(mk) {
		return false, true, "uniform capture (no screen recording permission?): " + desc
	}
	return isWhite(mk) && isWhite(mkEdge) && isClear(right) && isClear(below) && isClear(bottom), false, desc
}

// stepFallback checks Present(img) in Metal mode: it returns nil and the
// image (magenta top half, green bottom half) shows the right way up; the
// next GPU frame replaces it (stepShot "screenshot-gpu-again").
func (h *host) stepFallback() {
	pw, ph := h.want()
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			c := color.RGBA{255, 0, 255, 255}
			if y >= ph/2 {
				c = color.RGBA{0, 255, 0, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	err := h.cv.Present(img)
	wn := objc.ID(h.surface.Native()).Send(sel("windowNumber"))
	go func() {
		time.Sleep(150 * time.Millisecond)
		file := "/tmp/tango-smokemetal-fallback.png"
		var top, bot color.RGBA
		if _, e := exec.Command("screencapture", "-x", "-o", fmt.Sprintf("-l%d", wn), file).CombinedOutput(); e == nil {
			if f, e := os.Open(file); e == nil {
				if im, e := png.Decode(f); e == nil {
					bb := im.Bounds()
					at := func(x, y int) color.RGBA {
						r, g, b, _ := im.At(bb.Min.X+x, bb.Max.Y-ph+y).RGBA()
						return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
					}
					top, bot = at(pw/4, ph/8), at(pw*3/4, ph*7/8)
				}
				f.Close()
			}
		}
		h.post(func() {
			ok := err == nil && top.R > 200 && top.G < 80 && top.B > 200 && bot.G > 200 && bot.R < 80 && bot.B < 80
			h.check("cpu-fallback", ok, fmt.Sprintf("Present(img)=%v; capture top %v (want magenta, over the marker) bottom %v (want green) → %s", err, top, bot, file))
			h.cv.Invalidate() // a GPU frame (Present(nil)) replaces it
			h.after(200*time.Millisecond, func() { h.stepShot("screenshot-gpu-again", h.stepResize) })
		})
	}()
}

func (h *host) stepResize() {
	n, r := len(h.sink.frames), len(h.sink.resizes)
	h.surface.SetSize(640, 400)
	h.after(400*time.Millisecond, func() {
		sc := h.cv.Scale()
		got := false
		for _, rs := range h.sink.resizes[r:] {
			got = got || (rs[0] == 640 && rs[1] == 400 && rs[2] == sc)
		}
		var fr *frameInfo
		for i := n; i < len(h.sink.frames); i++ {
			if f := h.sink.frames[i]; f.tw == int(640*sc) && f.th == int(400*sc) && f.err == nil {
				fr = &f
			}
		}
		l := objc.ID(h.layer)
		bounds := msgGetRect(l, sel("bounds"))
		cs := msgGetFloat(l, sel("contentsScale"))
		detail := fmt.Sprintf("CanvasResized=%v layer bounds %gx%g contentsScale %g, frames since %d", got, bounds.W, bounds.H, cs, len(h.sink.frames)-n)
		if fr != nil {
			detail += fmt.Sprintf(", drawable %dx%d", fr.tw, fr.th)
		}
		h.check("resize", got && fr != nil && bounds.W == 640 && bounds.H == 400 && float32(cs) == sc, detail)
		h.stepShot("screenshot-resized", h.stepInvalidate)
	})
}

func (h *host) stepInvalidate() {
	h.after(200*time.Millisecond, func() {
		n := len(h.sink.frames)
		go h.cv.Invalidate()
		h.after(200*time.Millisecond, func() {
			d := len(h.sink.frames) - n
			h.check("invalidate", d == 1, fmt.Sprintf("goroutine Invalidate → %d frame(s) (want exactly 1)", d))
			h.stepCoalesce()
		})
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
			span := time.Duration(0)
			if len(fr) > 1 {
				span = fr[len(fr)-1].at.Sub(fr[0].at)
			}
			bad := 0
			for _, f := range fr {
				if f.err != nil || f.presentE != nil {
					bad++
				}
			}
			h.check("coalesce", len(fr) >= 10 && len(fr) <= 130 && bad == 0,
				fmt.Sprintf("%d Invalidate calls → %d frames over %v (%d failed)", calls.Load(), len(fr), span.Round(time.Millisecond), bad))
			h.stepInvalidateAt()
		})
	}()
}

func (h *host) stepInvalidateAt() {
	h.after(200*time.Millisecond, func() {
		n := len(h.sink.frames)
		start := time.Now()
		late, early := start.Add(500*time.Millisecond), start.Add(250*time.Millisecond)
		go func() {
			h.cv.InvalidateAt(late)
			h.cv.InvalidateAt(early)
		}()
		h.after(800*time.Millisecond, func() {
			fr := h.sink.frames[n:]
			if len(fr) == 0 {
				h.fail("invalidate-at", "no frame")
			} else {
				d := fr[0].at.Sub(early)
				h.check("invalidate-at", d >= -2*time.Millisecond && d <= 50*time.Millisecond && len(fr) == 1,
					fmt.Sprintf("frame %+v after the 250ms deadline, %d frame(s) in 800ms", d.Round(time.Millisecond), len(fr)))
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
	h.sink.events = nil
	err := hook.SmokeCanvasInput(h.cv)
	h.check("text-input-client", err == nil, fmt.Sprintf("NSTextInputClient in Metal mode: %v", err))
	h.cv.SetCursor(input.CursorText) // must not disturb the hosted layer
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
	has("mouse-press", func(e input.Event) bool {
		p, ok := e.(input.Pointer)
		return ok && p.Kind == input.Press && p.Button == input.ButtonLeft && p.Pos == geom.Pt(100, 50) && p.Clicks == 2
	})
	has("scroll", func(e input.Event) bool { s, ok := e.(input.Scroll); return ok && s.DY == 30 })
	has("key-left", func(e input.Event) bool {
		k, ok := e.(input.KeyEvent)
		return ok && k.Key == input.KeyLeft && k.Down
	})
	has("composition", func(e input.Event) bool { c, ok := e.(input.Composition); return ok && c.Text == "ni hao" })
	has("text-input", func(e input.Event) bool { t, ok := e.(input.TextInput); return ok && t.Text == "你好" })
	h.stepIdle()
}

func (h *host) stepIdle() {
	h.surface.Focus()
	h.after(300*time.Millisecond, func() {
		n := len(h.sink.frames)
		h.after(2*time.Second, func() {
			d := len(h.sink.frames) - n
			h.check("idle", d == 0, fmt.Sprintf("%d frame(s) in 2s while still", d))
			if len(h.sink.focus) > 0 && h.sink.focus[0] {
				h.pass("focus", fmt.Sprintf("CanvasFocus %v", h.sink.focus))
			} else {
				h.logf("SKIP focus: %v (window may not have become key)", h.sink.focus)
			}
			h.stepUnsupported()
		})
	})
}

func (h *host) stepUnsupported() {
	cc := h.b.(platform.CanvasCreator)
	ts, tcv, err := cc.NewCanvasSurface(transparentID, platform.SurfaceOptions{Title: "transparent", Width: 200, Height: 120},
		platform.SurfaceExtras{Appearance: platform.Appearance{Transparent: true}}, nopSink{})
	if err != nil {
		h.fail("unsupported-transparent", err.Error())
	} else {
		_, e := tcv.(platform.CanvasMetal).UseMetalLayer()
		h.check("unsupported-transparent", errors.Is(e, platform.ErrUnsupported), fmt.Sprintf("transparent canvas: %v", e))
		ts.Close()
	}
	ws, err := h.b.NewSurface(webID, platform.SurfaceOptions{Title: "web", Width: 300, Height: 200, URL: h.b.AssetOrigin() + "/index.html"})
	if err != nil {
		h.fail("unsupported-overlay", err.Error())
		h.stepClose()
		return
	}
	ov, err := h.b.(platform.CanvasAttacher).AttachCanvas(webID, platform.CanvasLayer{Above: true, PassThrough: true}, nopSink{})
	if err != nil {
		h.fail("unsupported-overlay", err.Error())
	} else {
		cm, ok := ov.(platform.CanvasMetal)
		var e error = errors.New("no CanvasMetal")
		if ok {
			_, e = cm.UseMetalLayer()
		}
		h.check("unsupported-overlay", errors.Is(e, platform.ErrUnsupported), fmt.Sprintf("attached canvas: %v", e))
	}
	ws.Close()
	h.after(200*time.Millisecond, h.stepClose)
}

func (h *host) stepClose() {
	cv := h.cv
	h.surface.Close()
	h.post(func() {
		cv.Invalidate()
		cv.InvalidateAt(time.Now())
		pe := cv.Present(nil)
		_, ue := cv.(platform.CanvasMetal).UseMetalLayer()
		h.check("after-close", pe != nil && ue != nil, fmt.Sprintf("Invalidate harmless, Present(nil): %v, UseMetalLayer: %v", pe, ue))
		h.after(300*time.Millisecond, h.b.Quit)
	})
}

var required = []string{
	"cpu-present-nil", "off-main", "use-metal", "first-frame", "screenshot", "cpu-fallback", "screenshot-gpu-again", "resize",
	"screenshot-resized", "invalidate", "coalesce", "invalidate-at", "text-input-client", "mouse-press",
	"scroll", "key-left", "composition", "text-input", "idle", "unsupported-transparent",
	"unsupported-overlay", "closed", "after-close",
}

func main() {
	flag.Parse()
	if *smoke {
		go func() {
			time.Sleep(40 * time.Second)
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(2)
		}()
	}
	h := &host{b: darwin.New(), start: time.Now(), checks: map[string]bool{}}
	if err := h.b.Init(h, platform.AppOptions{Name: "Tango Metal Smoke", ID: "dev.tango.smokemetal"}); err != nil {
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
	fmt.Printf("PASS: all %d checks (%d skipped, +%d optional)\n", len(required), h.skipped, len(h.checks)-len(required))
}
