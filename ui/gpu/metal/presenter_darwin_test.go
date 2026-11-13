//go:build darwin

package metal

import (
	"errors"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
)

// newTestLayer returns a standalone CAMetalLayer (no window, no view) with
// the given drawableSize, released at the end of the test. The calling
// test runs on a locked OS thread inside an autorelease pool.
func newTestLayer(t *testing.T, size image.Point) id {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if err := loadPresenter(); err != nil {
		t.Skipf("no Metal: %v", err)
	}
	drain := autoreleasePool()
	t.Cleanup(drain)
	if class("CAMetalLayer") == 0 {
		t.Skip("no CAMetalLayer class")
	}
	layer := retain(class("CAMetalLayer").Send(sel("layer")))
	if layer == 0 {
		t.Fatal("[CAMetalLayer layer] returned nil")
	}
	t.Cleanup(func() { release(layer) })
	msgSetSize(layer, sel("setDrawableSize:"), cgSize{float64(size.X), float64(size.Y)})
	return layer
}

// startPresenter runs newPresenter on layer, skipping without a Metal
// device, and closes it at the end of the test.
func startPresenter(t *testing.T, layer id) *presenter {
	t.Helper()
	p, err := newPresenter(uintptr(layer))
	if errors.Is(err, ErrNoDevice) || errors.Is(err, ErrUnsupported) {
		t.Skipf("no Metal device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func drawableSize(layer id) image.Point {
	s := msgSize(layer, sel("drawableSize"))
	return image.Pt(int(s.W), int(s.H))
}

// devicePixels is the drawable size the uihost glue passes to present.
func devicePixels(s geom.Size, scale float32) image.Point {
	return image.Pt(int(math.Round(float64(s.W*scale))), int(math.Round(float64(s.H*scale))))
}

// solidAndText is the solid parity scene with the text scene drawn on top.
func solidAndText(t *testing.T) *paint.List {
	l := gputest.Scenes[0].Make(t)
	l.Ops = append(l.Ops, gputest.Scenes[3].Make(t).Ops...)
	return l
}

func TestPresenterOffscreenLayer(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)

	if got := layer.Send(sel("device")); got != p.dev.dev {
		t.Errorf("layer.device = %#x, want the presenter's device %#x", got, p.dev.dev)
	}
	if got := msgU64(layer, sel("pixelFormat")); got != pixelFormatBGRA8Unorm {
		t.Errorf("layer.pixelFormat = %d, want BGRA8Unorm (%d)", got, pixelFormatBGRA8Unorm)
	}
	if !msgBool(layer, sel("framebufferOnly")) {
		t.Error("layer.framebufferOnly = NO, want YES")
	}
	if layer.Send(sel("colorspace")) == 0 {
		t.Error("layer.colorspace not set")
	}
	if msgBool(layer, sel("presentsWithTransaction")) {
		t.Error("presentsWithTransaction = YES, want the default NO")
	}

	const scale = 2
	l := solidAndText(t)
	size := devicePixels(gputest.SceneSize, scale)
	if err := p.r.Render(l, gputest.SceneSize, scale); err != nil {
		t.Fatal(err)
	}
	if err := p.present(size, scale); err != nil {
		t.Fatalf("present: %v", err)
	}
	if got := drawableSize(layer); got != size {
		t.Errorf("drawableSize = %v, want %v", got, size)
	}
	t.Logf("drawables presented: %d", p.presented)
	if p.presented != 1 {
		t.Errorf("presented %d drawables, want 1 (offscreen nextDrawable returned nil?)", p.presented)
	}

	ref := paint.NewCPU()
	if err := ref.Render(l, gputest.SceneSize, scale); err != nil {
		t.Fatal(err)
	}
	gputest.Check(t, "solid+text", l, scale, ref.Image(), readPixels(t, p.dev), 4)
}

// TestPresenterCopy checks the RGBA → BGRA copy pass itself: the drawable
// is framebufferOnly and cannot be read back, so the same pass is encoded
// into a readable BGRA8 texture larger than the frame; every pixel must
// equal the target's with R and B swapped, and pixels beyond the frame
// repeat its edge.
func TestPresenterCopy(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[4].Make(t) // card: shadows (patches) and AA edges
	if err := p.r.Render(l, gputest.SceneSize, 1.5); err != nil {
		t.Fatal(err)
	}
	want := readPixels(t, p.dev)
	fw, fh := want.Rect.Dx(), want.Rect.Dy()
	dw, dh := fw+3, fh+2
	got := p.dev.copyToBGRAForTest(t, image.Pt(dw, dh))
	for y := range dh {
		for x := range dw {
			w := want.RGBAAt(min(x, fw-1), min(y, fh-1))
			o := y*dw*4 + x*4
			g := color.RGBA{got[o+2], got[o+1], got[o], got[o+3]}
			if g != w {
				t.Fatalf("pixel %d,%d = %v, want %v", x, y, g, w)
			}
		}
	}
}

func TestPresenterResize(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[1].Make(t)
	for i, step := range []struct {
		size  geom.Size
		scale float32
	}{{geom.Size{W: 96, H: 56}, 2}, {geom.Size{W: 120.5, H: 70}, 2}, {geom.Size{W: 120.5, H: 70}, 1}} {
		if err := p.r.Render(l, step.size, step.scale); err != nil {
			t.Fatal(err)
		}
		px := devicePixels(step.size, step.scale)
		if err := p.present(px, step.scale); err != nil {
			t.Fatalf("step %d: present: %v", i, err)
		}
		if got := drawableSize(layer); got != px {
			t.Fatalf("step %d: drawableSize = %v, want %v", i, got, px)
		}
		ref := paint.NewCPU()
		ref.Render(l, step.size, step.scale)
		gputest.Check(t, "rounded", l, step.scale, ref.Image(), readPixels(t, p.dev), 2)
	}
	// The layer must not be touched when nothing changed.
	before := p.drawableSets
	if err := p.present(devicePixels(geom.Size{W: 120.5, H: 70}, 1), 1); err != nil {
		t.Fatal(err)
	}
	if p.drawableSets != before {
		t.Errorf("drawableSize set again for an unchanged size")
	}
}

// TestPresenterNoDrawable: with no drawable to draw into, present returns
// nil and presents nothing.
func TestPresenterNoDrawable(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)

	// Before any frame there is nothing to present.
	if err := p.present(image.Pt(50, 50), 1); err != nil {
		t.Fatalf("present before a frame: %v", err)
	}
	if p.presented != 0 {
		t.Fatalf("presented %d drawables before any frame", p.presented)
	}

	l := gputest.Scenes[0].Make(t)
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	// A zero-sized drawable (minimized window): no error, layer untouched.
	sz := drawableSize(layer)
	for _, z := range []image.Point{{}, {X: 0, Y: 30}, {X: 30, Y: 0}, {X: -1, Y: 5}} {
		if err := p.present(z, 1); err != nil {
			t.Fatalf("present(%v): %v", z, err)
		}
	}
	if got := drawableSize(layer); got != sz {
		t.Errorf("drawableSize changed to %v by a zero-size present", got)
	}
	if p.presented != 0 {
		t.Fatalf("presented %d drawables for zero sizes", p.presented)
	}

	// A layer without a drawable to give: an offscreen layer never
	// throttles nextDrawable (it hands out new drawables while others are
	// held), but one without a device returns nil, as an invisible
	// window's layer does. present reports no error and presents nothing.
	size := devicePixels(gputest.SceneSize, 1)
	msgSetID(layer, sel("setDevice:"), 0)
	if d := layer.Send(sel("nextDrawable")); d != 0 {
		t.Fatal("a CAMetalLayer without a device gave a drawable")
	}
	if err := p.present(size, 1); err != nil {
		t.Fatalf("present without a drawable: %v", err)
	}
	if p.presented != 0 {
		t.Fatalf("presented %d drawables, want 0", p.presented)
	}
}

// TestPresenterHugeSize: a drawable size beyond the Metal texture limit is
// clamped (CAMetalLayer ignores a larger drawableSize) so present does not
// set it again on every frame.
func TestPresenterHugeSize(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := p.present(image.Pt(20000, 300), 1); err != nil {
			t.Fatal(err)
		}
	}
	if got := drawableSize(layer); got != image.Pt(maxDrawable, 300) {
		t.Errorf("drawableSize = %v, want %v", got, image.Pt(maxDrawable, 300))
	}
	if p.drawableSets != 1 {
		t.Errorf("drawableSize set %d times, want 1", p.drawableSets)
	}
}

// TestPresenterDeferredError (Task 8 review item 1): a command buffer error
// collected outside Execute is returned by the next Execute (so the
// Renderer re-uploads its textures) and by present.
func TestPresenterDeferredError(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("metal: command buffer status 5: boom")
	p.dev.mu.Lock()
	p.dev.gpuFailed(boom, false)
	p.dev.mu.Unlock()
	if err := p.present(devicePixels(gputest.SceneSize, 1), 1); !errors.Is(err, boom) {
		t.Fatalf("present: %v, want %v", err, boom)
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); !errors.Is(err, boom) {
		t.Fatalf("Render: %v, want %v", err, boom)
	}
	// Reported once to Execute; the frame after renders normally.
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(devicePixels(gputest.SceneSize, 1), 1); err != nil {
		t.Fatal(err)
	}
	p.dev.mu.Lock()
	lost := p.dev.lost
	p.dev.mu.Unlock()
	if lost != nil {
		t.Fatalf("one failure lost the device: %v", lost)
	}
}

// TestPresenterDeviceLost (Task 8 review item 3): maxFailures consecutive
// GPU failures, or one device-removed error, make Execute and present
// return ErrDeviceLost from then on.
func TestPresenterDeviceLost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		removed bool
		n       int
	}{{"consecutive", false, maxFailures}, {"removed", true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			layer := newTestLayer(t, image.Pt(10, 10))
			p := startPresenter(t, layer)
			l := gputest.Scenes[0].Make(t)
			size := devicePixels(gputest.SceneSize, 1)
			for i := range tc.n {
				p.r.Render(l, gputest.SceneSize, 1) // reports the previous injected error from frame 1 on
				p.dev.mu.Lock()
				p.dev.wait()
				p.dev.gpuFailed(errors.New("injected"), tc.removed)
				p.dev.mu.Unlock()
				err := p.present(size, 1)
				if lost := i == tc.n-1; errors.Is(err, ErrDeviceLost) != lost {
					t.Fatalf("frame %d: present = %v, lost %v", i, err, lost)
				}
			}
			if err := p.r.Render(l, gputest.SceneSize, 1); !errors.Is(err, ErrDeviceLost) {
				t.Fatalf("Render after loss: %v", err)
			}
			if err := p.present(size, 1); !errors.Is(err, ErrDeviceLost) {
				t.Fatalf("present after loss: %v", err)
			}
		})
	}
}

// TestPresenterSuccessResetsFailures: a frame that completes on the GPU
// resets the failure count, so isolated failures never add up to a loss.
func TestPresenterSuccessResetsFailures(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	size := devicePixels(gputest.SceneSize, 1)
	for i := range 3 * maxFailures {
		p.r.Render(l, gputest.SceneSize, 1) // reports the injected error every other frame
		if i%2 == 0 {
			p.dev.mu.Lock()
			p.dev.wait() // completes successfully: resets the count
			p.dev.gpuFailed(errors.New("injected"), false)
			p.dev.mu.Unlock()
		}
		if err := p.present(size, 1); errors.Is(err, ErrDeviceLost) {
			t.Fatalf("frame %d: device lost after isolated failures", i)
		}
	}
}

func TestCommandBufferErrorLost(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadPresenter(); err != nil {
		t.Skip(err)
	}
	defer autoreleasePool()()
	for code, want := range map[uint64]bool{1: false, 2: false, 3: false, 4: true, 8: false, 11: true} {
		e := msgNSError(class("NSError"), sel("errorWithDomain:code:userInfo:"), nsString("MTLCommandBufferErrorDomain"), code, 0)
		if got := cbErrorLost(e); got != want {
			t.Errorf("code %d: lost %v, want %v", code, got, want)
		}
	}
	if cbErrorLost(0) {
		t.Error("nil error: lost")
	}
}

func TestPresenterClose(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	var c interface{ Close() error } = p.r.(interface{ Close() error })
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.present(image.Pt(10, 10), 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("present after Close: %v", err)
	}
}

func TestNewPresenterBadLayer(t *testing.T) {
	if _, _, err := NewPresenter(0); err == nil {
		t.Fatal("NewPresenter(0): want an error")
	}
}

// copyToBGRAForTest encodes the present pass into a new readable
// BGRA8Unorm texture of the given size and returns its pixels (BGRA,
// tightly packed rows).
func (d *Device) copyToBGRAForTest(t *testing.T, size image.Point) []byte {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	defer autoreleasePool()()
	desc := msgTexDesc(class("MTLTextureDescriptor"), sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"), pixelFormatBGRA8Unorm, uint64(size.X), uint64(size.Y), false)
	msgSetU64(desc, sel("setUsage:"), usageRenderTarget)
	msgSetU64(desc, sel("setStorageMode:"), textureStoragePrivate)
	tex := d.dev.Send(sel("newTextureWithDescriptor:"), desc)
	if tex == 0 {
		t.Fatal("cannot create the BGRA texture")
	}
	defer release(tex)
	n := size.X * size.Y * 4
	buf := msgNewBuffer(d.dev, sel("newBufferWithLength:options:"), uint64(n), storageModeShared)
	defer release(buf)
	cb := d.queue.Send(sel("commandBuffer"))
	d.encodeCopy(cb, tex)
	enc := cb.Send(sel("blitCommandEncoder"))
	copyTextureToBuffer(enc, tex, mtlOrigin{}, mtlSize{uint64(size.X), uint64(size.Y), 1}, buf, 0, uint64(size.X*4), uint64(n))
	enc.Send(sel("endEncoding"))
	cb.Send(sel("commit"))
	cb.Send(sel("waitUntilCompleted"))
	if st := msgU64(cb, sel("status")); st != cbStatusCompleted {
		t.Fatal(nsError("copy pass", cb.Send(sel("error"))))
	}
	return append([]byte(nil), unsafe.Slice((*byte)(msgPtr(buf, sel("contents"))), n)...)
}
