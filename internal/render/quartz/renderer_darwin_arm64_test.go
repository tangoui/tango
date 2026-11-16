//go:build darwin && !ios && arm64

package quartz

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

func newNativeTestRenderer(t *testing.T) *renderer {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if err := loadNative(); err != nil {
		t.Skipf("CoreGraphics unavailable: %v", err)
	}
	layer := objc.ID(objc.GetClass("CALayer")).Send(sel("new"))
	if layer == 0 {
		t.Fatal("no CALayer")
	}
	t.Cleanup(func() { layer.Send(sel("release")) })
	r, _, e := NewPresenter(uintptr(layer))
	if e != nil {
		t.Fatal(e)
	}
	native := r.(*renderer)
	t.Cleanup(func() { _ = native.Close() })
	return native
}
func nativeFrame(t *testing.T, r *renderer, l *paint.List, size geom.Size, scale float32) {
	t.Helper()
	if e := r.Render(l, size, scale); e != nil {
		t.Fatal(e)
	}
	if e := r.present(image.Pt(int(math.Ceil(float64(size.W*scale))), int(math.Ceil(float64(size.H*scale)))), scale); e != nil {
		t.Fatal(e)
	}
}
func nativeSnapshot(t *testing.T, r *renderer) *image.RGBA {
	t.Helper()
	b := r.scene.bounds
	img := image.NewRGBA(b)
	if b.Empty() {
		return img
	}
	ctx := cgBitmapCreate(unsafe.Pointer(&img.Pix[0]), b.Dx(), b.Dy(), 8, img.Stride, nativeSRGB, 1|(4<<12))
	if ctx == 0 {
		t.Fatal("no bitmap context")
	}
	cgTranslate(ctx, 0, float64(b.Dy()))
	cgScale(ctx, float64(r.scene.scale), -float64(r.scene.scale))
	r.layer.Send(sel("renderInContext:"), ctx)
	cgContextRelease(ctx)
	runtime.KeepAlive(img)
	return img
}
func nearImage(t *testing.T, got, want *image.RGBA, tolerance int) {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("bounds %v vs %v", got.Rect, want.Rect)
	}
	for i, v := range got.Pix {
		d := int(v) - int(want.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > tolerance {
			t.Fatalf("pixel byte %d got %d want %d tolerance %d", i, v, want.Pix[i], tolerance)
		}
	}
}
func textOp(s string, x, y float32) paint.Text {
	l := text.Shape(s, text.Style{Size: 16}, text.Options{})
	return paint.Text{Origin: geom.Pt(x, y), Glyphs: append([]paint.Glyph(nil), l.Glyphs...), Color: color.NRGBA{A: 255}}
}
func TestNativeRetainsTextAcrossMovementAndReordering(t *testing.T) {
	r := newNativeTestRenderer(t)
	one, two := textOp("one", 10, 10), textOp("two", 10, 40)
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}, Ops: []paint.Op{one, two}}
	nativeFrame(t, r, l, geom.Size{W: 160, H: 100}, 2)
	if len(r.nodes) != 2 {
		t.Fatal("missing native text layers")
	}
	a, b := r.nodes[0], r.nodes[1]
	ap, bp := a.paints, b.paints
	if ap == 0 || bp == 0 {
		t.Fatal("text layers were not actually painted")
	}
	one.Origin.Y += 5
	two.Origin.Y += 5
	l.Ops = []paint.Op{two, one}
	nativeFrame(t, r, l, geom.Size{W: 160, H: 100}, 2)
	if r.nodes[0] != b || r.nodes[1] != a {
		t.Fatal("matching text contents were discarded on order change")
	}
	if a.paints != ap || b.paints != bp {
		t.Fatal("moving identical text rerasterized it")
	}
}
func TestNativeTextVisibleAtFractionalScales(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		r := newNativeTestRenderer(t)
		l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}, Ops: []paint.Op{textOp("Hello 中文", 7.25, 10.5)}}
		nativeFrame(t, r, l, geom.Size{W: 180, H: 70}, scale)
		img := nativeSnapshot(t, r)
		ink := 0
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				c := img.RGBAAt(x, y)
				if c.R < 128 && c.G < 128 && c.B < 128 && c.A > 200 {
					if !image.Pt(x, y).In(r.nodes[0].frame) {
						t.Fatalf("ink %d,%d outside planned text frame %v", x, y, r.nodes[0].frame)
					}
					ink++
				}
			}
		}
		if ink < 100 {
			t.Fatalf("scale %v missing text ink: %d", scale, ink)
		}
	}
}
func TestNativePatchAndFallbackPixels(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		for _, complex := range []bool{false, true} {
			r := newNativeTestRenderer(t)
			ops := []paint.Op{paint.FillRect{Rect: geom.Rect{X: 4.25, Y: 3.5, W: 55, H: 35}, Radii: geom.Radii{TL: 3, TR: 6, BR: 9, BL: 2}, Color: color.NRGBA{R: 180, G: 40, B: 100, A: 180}}, paint.ShadowRect{Rect: geom.Rect{X: 12, Y: 10, W: 25, H: 25}, Shadow: paint.Shadow{X: 2, Y: 3, Blur: 5, Color: color.NRGBA{A: 100}}}}
			if complex {
				ops = append([]paint.Op{paint.PushOpacity{Alpha: .5}}, ops...)
			}
			l := &paint.List{Background: color.NRGBA{R: 30, G: 70, B: 120, A: 128}, Ops: ops}
			nativeFrame(t, r, l, geom.Size{W: 80, H: 60}, scale)
			got := nativeSnapshot(t, r)
			cpu := paint.NewCPU()
			_ = cpu.Render(l, geom.Size{W: 80, H: 60}, scale)
			nearImage(t, got, cpu.Image(), 4)
		}
	}
}
func TestNativeSharedGlyphMutationIsRedrawn(t *testing.T) {
	r := newNativeTestRenderer(t)
	op := textOp("A", 10, 10)
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}, Ops: []paint.Op{op}}
	nativeFrame(t, r, l, geom.Size{W: 80, H: 60}, 2)
	before := nativeSnapshot(t, r)
	p := r.nodes[0].paints
	b := textOp("B", 10, 10)
	op.Glyphs[0].ID = b.Glyphs[0].ID
	nativeFrame(t, r, l, geom.Size{W: 80, H: 60}, 2)
	after := nativeSnapshot(t, r)
	same := true
	for i, v := range before.Pix {
		if after.Pix[i] != v {
			same = false
			break
		}
	}
	if same || r.nodes[0].paints <= p {
		t.Fatal("mutated shared glyph data remained stale")
	}
}
func TestNativeTrimPreservesPixelsAndCloseRetiresCallbacks(t *testing.T) {
	r := newNativeTestRenderer(t)
	base := len(nativeDrawers)
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}, Ops: []paint.Op{textOp("retained", 10, 10), paint.FillRect{Rect: geom.Rect{X: 5, Y: 40, W: 60, H: 20}, Radii: geom.Radii{TL: 4}, Color: color.NRGBA{R: 200, A: 255}}}}
	nativeFrame(t, r, l, geom.Size{W: 120, H: 80}, 2)
	before := nativeSnapshot(t, r)
	r.Trim()
	nearImage(t, nativeSnapshot(t, r), before, 0)
	nativeFrame(t, r, l, geom.Size{W: 120, H: 80}, 2)
	nearImage(t, nativeSnapshot(t, r), before, 0)
	empty := &paint.List{Background: l.Background}
	nativeFrame(t, r, empty, geom.Size{W: 120, H: 80}, 2)
	if len(nativeDrawers) != base {
		t.Fatal("offscreen text callback retained")
	}
	_ = r.Close()
	_ = r.Close()
	if len(nativeDrawers) != base {
		t.Fatal("close retained native callbacks")
	}
}
func TestNativeImageInvalidation(t *testing.T) {
	r := newNativeTestRenderer(t)
	asset := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			asset.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	l := &paint.List{Ops: []paint.Op{paint.Image{Img: asset, Rect: geom.Rect{W: 16, H: 16}}}}
	nativeFrame(t, r, l, geom.Size{W: 20, H: 20}, 1)
	if nativeSnapshot(t, r).RGBAAt(4, 4).R < 200 {
		t.Fatal("missing image")
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			asset.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	r.InvalidateImage(asset)
	nativeFrame(t, r, l, geom.Size{W: 20, H: 20}, 1)
	if nativeSnapshot(t, r).RGBAAt(4, 4).B < 200 {
		t.Fatal("stale native image snapshot")
	}
}

func TestNativeFailedFrameReleasesNewLayersAndImageOwners(t *testing.T) {
	r := newNativeTestRenderer(t)
	base := len(nativeDrawers)
	saved := cgImageCreate
	calls := 0
	cgImageCreate = func(a, b, c, d, e int, f uintptr, g uint32, h uintptr, i unsafe.Pointer, j bool, k int32) uintptr {
		calls++
		if calls == 2 {
			return 0
		}
		return saved(a, b, c, d, e, f, g, h, i, j, k)
	}
	t.Cleanup(func() { cgImageCreate = saved })
	l := &paint.List{Ops: []paint.Op{
		paint.FillRect{Rect: geom.Rect{W: 20, H: 20}, Radii: geom.Radii{TL: 3}, Color: color.NRGBA{R: 255, A: 255}},
		textOp("owned", 25, 5),
		paint.FillRect{Rect: geom.Rect{X: 60, W: 20, H: 20}, Radii: geom.Radii{TR: 3}, Color: color.NRGBA{B: 255, A: 255}},
	}}
	if err := r.Render(l, geom.Size{W: 120, H: 50}, 1); err == nil {
		t.Fatal("native allocation failure was not reported")
	}
	if len(nativeDrawers) != base || len(r.images) != 0 {
		t.Fatalf("failed frame retained callbacks/images: %d/%d", len(nativeDrawers)-base, len(r.images))
	}
	cgImageCreate = saved
	nativeFrame(t, r, l, geom.Size{W: 120, H: 50}, 1)
	if len(r.nodes) != 3 {
		t.Fatal("renderer did not recover from failed frame")
	}
}

func TestNativePrependingContentDoesNotStealALayerNeededLater(t *testing.T) {
	r := newNativeTestRenderer(t)
	one, two := textOp("one", 10, 10), textOp("two", 10, 40)
	l := &paint.List{Ops: []paint.Op{one, two}}
	nativeFrame(t, r, l, geom.Size{W: 140, H: 90}, 1)
	retained := r.nodes[0]
	paints := retained.paints
	three := textOp("three", 10, 10)
	one.Origin.Y = 40
	l.Ops = []paint.Op{three, one}
	nativeFrame(t, r, l, geom.Size{W: 140, H: 90}, 1)
	if r.nodes[1] != retained || retained.paints != paints {
		t.Fatal("new leading content destroyed an unchanged retained layer")
	}
}

func TestNativeCloseClearsBackgroundForCPUTakeover(t *testing.T) {
	r := newNativeTestRenderer(t)
	nativeFrame(t, r, &paint.List{Background: color.NRGBA{R: 255, A: 255}}, geom.Size{W: 30, H: 30}, 1)
	layer := r.layer
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if layer.Send(sel("backgroundColor")) != 0 {
		t.Fatal("native background survived close and would blend under a CPU replacement")
	}
}
