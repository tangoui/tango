//go:build darwin && !ios && arm64

package quartz

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
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

func TestNativeRenderCompletesChangedTextBeforeSubmittingTransaction(t *testing.T) {
	r := newNativeTestRenderer(t)
	if e := r.Render(&paint.List{Ops: []paint.Op{textOp("ready", 10, 10)}}, geom.Size{W: 120, H: 60}, 2); e != nil {
		t.Fatal(e)
	}
	if len(r.nodes) != 1 || r.nodes[0].paints == 0 {
		t.Fatal("text drawing left for an extra implicit presentation transaction")
	}
}

func TestNativeTextKeepsBackingWhenInkWidthChanges(t *testing.T) {
	r := newNativeTestRenderer(t)
	size := geom.Size{W: 300, H: 100}
	nativeFrame(t, r, &paint.List{Ops: []paint.Op{textOp("888888", 10, 10)}}, size, 2)
	n := r.nodes[0]
	before := getNativeTestRect(n.layer, "bounds").Size
	ink := n.frame.Size()
	nativeFrame(t, r, &paint.List{Ops: []paint.Op{textOp("11111", 10, 10)}}, size, 2)
	if n != r.nodes[0] || ink == n.frame.Size() {
		t.Fatal("test did not retain a node with different ink bounds")
	}
	after := getNativeTestRect(n.layer, "bounds").Size
	if before != after {
		t.Fatalf("minor text change resized backing %v -> %v", before, after)
	}
	nativeFrame(t, r, &paint.List{Ops: []paint.Op{textOp("i", 10, 10)}}, size, 2)
	after = getNativeTestRect(n.layer, "bounds").Size
	if after.W*after.H > 4*float64(n.frame.Dx()*n.frame.Dy())/4 {
		t.Fatal("short replacement retained more than four times required backing area")
	}
}
func getNativeTestRect(layer objc.ID, name string) nativeRect {
	var get func(objc.ID, objc.SEL) nativeRect
	lib, e := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if e != nil {
		panic(e)
	}
	defer purego.Dlclose(lib)
	addr, e := purego.Dlsym(lib, "objc_msgSend")
	if e != nil {
		panic(e)
	}
	purego.RegisterFunc(&get, addr)
	return get(layer, sel(name))
}
func TestNativeRetainedTextBackingHonorsChangingClips(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		r := newNativeTestRenderer(t)
		text := textOp("Latin 中文 888888", 10, 10)
		size := geom.Size{W: 200, H: 80}
		nativeFrame(t, r, &paint.List{Ops: []paint.Op{text}}, size, scale)
		for _, clip := range []geom.Rect{{X: 12, Y: 14, W: 35, H: 8}, {X: 18, Y: 10, W: 70, H: 9}} {
			l := &paint.List{Ops: []paint.Op{paint.PushClip{Rect: clip}, text, paint.Pop{}}}
			nativeFrame(t, r, l, size, scale)
			snap := nativeSnapshot(t, r)
			allowed := clipBounds(clip, scale)
			ink := 0
			for y := 0; y < snap.Rect.Dy(); y++ {
				for x := 0; x < snap.Rect.Dx(); x++ {
					if snap.RGBAAt(x, y).A != 0 {
						ink++
						if !image.Pt(x, y).In(allowed) {
							t.Fatalf("retained text escaped new clip @%v: %d,%d", scale, x, y)
						}
					}
				}
			}
			if ink == 0 {
				t.Fatalf("retained text disappeared @%v clip=%v", scale, clip)
			}
		}
	}
}

func TestNativeBatchedTextRetainsColorsClipsAndBorrowedGlyphSafety(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		r := newNativeTestRenderer(t)
		one, two := textOp("hello", 10, 10), textOp("world", 52, 10)
		one.Color = color.NRGBA{R: 255, A: 255}
		two.Color = color.NRGBA{B: 255, A: 255}
		clip := geom.Rect{X: 52, Y: 10, W: 20, H: 22}
		l := &paint.List{Ops: []paint.Op{one, paint.PushClip{Rect: clip}, two, paint.Pop{}}}
		nativeFrame(t, r, l, geom.Size{W: 180, H: 70}, scale)
		if len(r.nodes) != 1 || len(r.nodes[0].texts) != 2 {
			t.Fatal("same-row text was not batched")
		}
		n := r.nodes[0]
		paints := n.paints
		img := nativeSnapshot(t, r)
		red, blue := 0, 0
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				p := img.RGBAAt(x, y)
				if p.R > p.B && p.A > 100 {
					red++
				}
				if p.B > p.R && p.A > 100 {
					blue++
					if !image.Pt(x, y).In(clipBounds(clip, scale)) {
						t.Fatal("batched blue text escaped its own clip")
					}
				}
			}
		}
		if red == 0 || blue == 0 {
			t.Fatal("batched text lost a color or glyph run")
		}
		oldID := n.texts[1].runs[0].IDs[0]
		two.Glyphs[0].ID = one.Glyphs[0].ID
		if n.texts[1].runs[0].IDs[0] != oldID {
			t.Fatal("batch retained borrowed glyph storage")
		}
		nativeFrame(t, r, l, geom.Size{W: 180, H: 70}, scale)
		if n.paints <= paints {
			t.Fatal("mutated glyph buffer did not redraw batch")
		}
	}
}

func TestNativeBatchedOverlappingAlphaMatchesSeparateLayers(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		for _, reverse := range []bool{false, true} {
			a, b := textOp("MMMM", 10, 10), textOp("WWWW", 19, 10)
			a.Color = color.NRGBA{R: 240, G: 30, A: 123}
			b.Color = color.NRGBA{B: 230, G: 60, A: 181}
			if reverse {
				a, b = b, a
			}
			sentinel := paint.FillRect{Rect: geom.Rect{X: 150, Y: 65, W: 1, H: 1}, Color: color.NRGBA{G: 255, A: 255}}
			clip := geom.Rect{X: 15, Y: 12, W: 35, H: 15}
			batched := &paint.List{Background: color.NRGBA{R: 25, G: 40, B: 50, A: 80}, Ops: []paint.Op{a, paint.PushClip{Rect: clip}, b, paint.Pop{}, sentinel}}
			separate := &paint.List{Background: batched.Background, Ops: []paint.Op{a, sentinel, paint.PushClip{Rect: clip}, b, paint.Pop{}}}
			r, ref := newNativeTestRenderer(t), newNativeTestRenderer(t)
			size := geom.Size{W: 170, H: 75}
			nativeFrame(t, r, batched, size, scale)
			nativeFrame(t, ref, separate, size, scale)
			if len(r.nodes) != 2 || len(ref.nodes) != 3 {
				t.Fatal("reference did not separate overlapping text")
			}
			got, want := nativeSnapshot(t, r), nativeSnapshot(t, ref)
			for i, p := range got.Pix {
				d := int(p) - int(want.Pix[i])
				if d < 0 {
					d = -d
				}
				if d > 2 {
					t.Fatalf("compositing differs @%v reverse=%v byte %d: %d vs %d", scale, reverse, i, p, want.Pix[i])
				}
			}
		}
	}
}

// bulkTextList is a grid of labels on backgrounds; seed changes every label.
func bulkTextList(seed int) *paint.List {
	var ops []paint.Op
	for i := 0; i < 48; i++ {
		x, y := float32(i%8)*90+4, float32(i/8)*30+4
		ops = append(ops, paint.FillRect{Rect: geom.Rect{X: x, Y: y, W: 80, H: 24}, Color: color.NRGBA{R: 240, G: 232, B: 184, A: 255}})
		t := textOp(fmt.Sprintf("%d.%02d", (i*7919+seed*31)%1000, (i+seed)%100), x+4, y+4)
		t.Color = color.NRGBA{R: 160, G: 16, B: 40, A: 255} // asymmetric: catches channel order
		ops = append(ops, t)
	}
	return &paint.List{Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Ops: ops}
}

// Bulk text must draw exactly what one layer per label draws, through the
// transition into bulk mode, across changing content, and back out of it.
func TestNativeBulkTextMatchesSeparateLayers(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		r, ref := newNativeTestRenderer(t), newNativeTestRenderer(t)
		ref.scene.bulkOff = true
		size := geom.Size{W: 740, H: 190}
		frames := 4 + bulkCalmFrames + 2
		for f := 0; f < frames; f++ {
			seed := f
			if f >= 4 {
				seed = 3 // content stops changing: bulk mode must end
			}
			l := bulkTextList(seed)
			nativeFrame(t, r, l, size, scale)
			nativeFrame(t, ref, l, size, scale)
			if f == 2 && !r.scene.bulk {
				t.Fatal("changing labels did not enter bulk mode")
			}
			if f == 2 || f == frames-1 {
				nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, ref), 2)
			}
		}
		if r.scene.bulk {
			t.Fatal("static labels stayed in bulk mode")
		}
		texts := 0
		for _, n := range r.nodes {
			if n.kind == itemText {
				texts++
			}
		}
		if texts < 40 {
			t.Fatalf("after bulk mode, %d text layers; want per-label layers back", texts)
		}
	}
}

func TestNativeBulkSurfacesAlternateAndRelease(t *testing.T) {
	r := newNativeTestRenderer(t)
	size := geom.Size{W: 740, H: 190}
	var shown []uintptr
	for f := 0; f < 4; f++ {
		nativeFrame(t, r, bulkTextList(f), size, 1)
		if f > 0 {
			if !r.scene.bulkMerged || r.surfaces.shown == 0 {
				t.Fatalf("frame %d: bulk text not drawn into a surface", f)
			}
			shown = append(shown, r.surfaces.shown)
		}
	}
	for i := 1; i < len(shown); i++ {
		if shown[i] == shown[i-1] {
			t.Fatal("consecutive bulk frames reused the displayed surface")
		}
	}
	if len(r.surfaces.ring) > surfaceMax {
		t.Fatalf("%d surfaces, want at most %d", len(r.surfaces.ring), surfaceMax)
	}
	for f := 0; f < bulkCalmFrames+1; f++ {
		nativeFrame(t, r, bulkTextList(9), size, 1)
	}
	if len(r.surfaces.ring) != 0 {
		t.Fatalf("bulk mode ended with %d surfaces retained", len(r.surfaces.ring))
	}
}

// Labels cut by a clip (a scrolled table, a narrow column) must stay cut in
// the bulk raster exactly as in their own layers.
func TestNativeBulkTextKeepsPartialClips(t *testing.T) {
	clipped := func(seed int) *paint.List {
		l := bulkTextList(seed)
		var ops []paint.Op
		for i, op := range l.Ops {
			if txt, ok := op.(paint.Text); ok && i%4 == 1 {
				r := geom.Rect{X: txt.Origin.X, Y: txt.Origin.Y - 2, W: 14, H: 9}
				ops = append(ops, paint.PushClip{Rect: r}, txt, paint.Pop{})
				continue
			}
			ops = append(ops, op)
		}
		l.Ops = ops
		return l
	}
	for _, scale := range []float32{1, 2} {
		r, ref := newNativeTestRenderer(t), newNativeTestRenderer(t)
		ref.scene.bulkOff = true
		size := geom.Size{W: 740, H: 190}
		for f := 0; f < 4; f++ {
			nativeFrame(t, r, clipped(f), size, scale)
			nativeFrame(t, ref, clipped(f), size, scale)
		}
		if !r.scene.bulkMerged {
			t.Fatal("clipped grid did not merge")
		}
		nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, ref), 2)
	}
}

// Recoloring cells in place keeps each background's layer where it is: the
// sublayer order does not change and the result matches a fresh renderer.
func TestNativeRecoloredSolidsKeepTheirLayers(t *testing.T) {
	cells := func(seed int) *paint.List {
		colors := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}}
		l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
		for i := 0; i < 30; i++ {
			l.Ops = append(l.Ops, paint.FillRect{Rect: geom.Rect{X: float32(i%6) * 20, Y: float32(i/6) * 10, W: 20, H: 10}, Color: colors[(i*7+seed)%3]})
		}
		return l
	}
	r := newNativeTestRenderer(t)
	size := geom.Size{W: 120, H: 50}
	nativeFrame(t, r, cells(0), size, 1)
	before := append([]*node(nil), r.nodes...)
	nativeFrame(t, r, cells(1), size, 1)
	for i := range before {
		if r.nodes[i] != before[i] {
			t.Fatalf("cell %d changed layer when only recolored", i)
		}
	}
	fresh := newNativeTestRenderer(t)
	nativeFrame(t, fresh, cells(1), size, 1)
	nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, fresh), 0)
}

// When the window server still reads every surface, a bulk frame falls back
// to a CGImage. That frame must keep the surfaces (releasing them would make
// the next frames allocate new window-sized ones) and reuse one fallback
// raster across such frames; leaving bulk mode releases both.
func TestNativeBulkFallbackKeepsSurfacesAndRaster(t *testing.T) {
	r := newNativeTestRenderer(t)
	size := geom.Size{W: 740, H: 190}
	for f := 0; f < 5; f++ {
		nativeFrame(t, r, bulkTextList(f), size, 1)
	}
	ring := append([]surface(nil), r.surfaces.ring...)
	if len(ring) == 0 {
		t.Fatal("no surfaces before fallback")
	}
	old := surfaceBusy
	surfaceBusy = func(uintptr) bool { return true }
	defer func() { surfaceBusy = old }()
	for len(r.surfaces.ring) < surfaceMax {
		nativeFrame(t, r, bulkTextList(len(r.surfaces.ring)+10), size, 1)
	}
	ring = append(ring[:0], r.surfaces.ring...)
	var buf *image.RGBA
	for f := 0; f < 3; f++ {
		nativeFrame(t, r, bulkTextList(20+f), size, 1)
		if !r.scene.bulkMerged {
			t.Fatal("fallback frame left bulk mode")
		}
		if len(r.surfaces.ring) != len(ring) || r.surfaces.ring[0] != ring[0] {
			t.Fatalf("fallback frame %d released or replaced the surfaces", f)
		}
		if r.scene.bulkBuf == nil || buf != nil && r.scene.bulkBuf != buf {
			t.Fatalf("fallback frame %d did not reuse the raster", f)
		}
		buf = r.scene.bulkBuf
	}
	ref := newNativeTestRenderer(t)
	ref.scene.bulkOff = true
	nativeFrame(t, ref, bulkTextList(22), size, 1)
	nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, ref), 2)
	surfaceBusy = old
	for f := 0; f < bulkCalmFrames+1; f++ {
		nativeFrame(t, r, bulkTextList(9), size, 1)
	}
	if len(r.surfaces.ring) != 0 || r.scene.bulkBuf != nil {
		t.Fatal("leaving bulk mode kept surfaces or the fallback raster")
	}
}

// Resizing the window during bulk mode replaces every surface with ones of
// the new size: none of the old size stays in the ring.
func TestNativeBulkSurfacesFollowWindowSize(t *testing.T) {
	r := newNativeTestRenderer(t)
	small, large := geom.Size{W: 740, H: 190}, geom.Size{W: 760, H: 220}
	for f := 0; f < 5; f++ {
		nativeFrame(t, r, bulkTextList(f), small, 1)
	}
	if len(r.surfaces.ring) == 0 {
		t.Fatal("no surfaces before resize")
	}
	for f := 5; f < 9; f++ {
		nativeFrame(t, r, bulkTextList(f), large, 1)
	}
	want := image.Pt(760, 220)
	if len(r.surfaces.ring) == 0 {
		t.Fatal("no surfaces after resize")
	}
	for _, s := range r.surfaces.ring {
		if s.size != want {
			t.Fatalf("surface of size %v kept after resize to %v", s.size, want)
		}
	}
	ref := newNativeTestRenderer(t)
	ref.scene.bulkOff = true
	nativeFrame(t, ref, bulkTextList(8), large, 1)
	nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, ref), 2)
}

// The CGImage fallback raster is released once fallbacks stop, even while
// bulk mode continues, so one busy moment does not pin a window-sized
// buffer for the rest of the session.
func TestNativeBulkFallbackRasterAgesOut(t *testing.T) {
	r := newNativeTestRenderer(t)
	size := geom.Size{W: 740, H: 190}
	for f := 0; f < 5; f++ {
		nativeFrame(t, r, bulkTextList(f), size, 1)
	}
	old := surfaceBusy
	surfaceBusy = func(uintptr) bool { return true }
	for len(r.surfaces.ring) < surfaceMax || r.scene.bulkBuf == nil {
		nativeFrame(t, r, bulkTextList(len(r.surfaces.ring)+10), size, 1)
	}
	surfaceBusy = old
	for f := 0; f < bulkBufKeepFrames; f++ {
		nativeFrame(t, r, bulkTextList(30+f%7), size, 1)
	}
	if r.scene.bulkBuf == nil {
		t.Fatal("fallback raster released too early")
	}
	for f := 0; f < 3; f++ {
		nativeFrame(t, r, bulkTextList(40+f), size, 1)
	}
	if !r.scene.bulkMerged {
		t.Fatal("test left bulk mode")
	}
	if r.scene.bulkBuf != nil {
		t.Fatal("fallback raster kept after fallbacks stopped")
	}
}

// A frame that leaves bulk text for a complete software frame (an opacity
// group, here) or an empty window must release the bulk surfaces: they are
// window-sized and nothing displays them any more.
func TestNativeBulkSurfacesReleasedByFullAndEmptyFrames(t *testing.T) {
	for _, tc := range []struct {
		name string
		list func(int) *paint.List
		size geom.Size
	}{
		{"full frame", func(f int) *paint.List {
			l := bulkTextList(f)
			l.Ops = append([]paint.Op{paint.PushOpacity{Alpha: 0.5}}, append(l.Ops, paint.Pop{})...)
			return l
		}, geom.Size{W: 740, H: 190}},
		{"empty window", bulkTextList, geom.Size{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newNativeTestRenderer(t)
			for f := 0; f < 5; f++ {
				nativeFrame(t, r, bulkTextList(f), geom.Size{W: 740, H: 190}, 1)
			}
			if len(r.surfaces.ring) == 0 {
				t.Fatal("no surfaces in bulk mode")
			}
			if e := r.Render(tc.list(9), tc.size, 1); e != nil {
				t.Fatal(e)
			}
			if r.scene.bulkMerged || len(r.surfaces.ring) != 0 || r.scene.bulkBuf != nil {
				t.Fatalf("merged=%v surfaces=%d buf=%v after a frame with no bulk text", r.scene.bulkMerged, len(r.surfaces.ring), r.scene.bulkBuf != nil)
			}
		})
	}
}

// Pixel parity with per-label layers for the cases the bulk lift must not
// change: translucent labels over backgrounds, a later translucent fill
// covering labels (the frame re-plans with per-label text), and the frame
// that enters bulk mode.
func TestNativeBulkTextParityAlphaOverlapAndEntry(t *testing.T) {
	translucent := func(f int) *paint.List {
		l := bulkTextList(f)
		for i, op := range l.Ops {
			if txt, ok := op.(paint.Text); ok {
				txt.Color.A = 120
				l.Ops[i] = txt
			}
		}
		return l
	}
	covered := func(f int) *paint.List {
		l := bulkTextList(f)
		l.Ops = append(l.Ops, paint.FillRect{Rect: geom.Rect{X: 0, Y: 0, W: 300, H: 70}, Color: color.NRGBA{R: 20, G: 60, B: 200, A: 110}})
		return l
	}
	for _, scale := range []float32{1, 2} {
		r, ref := newNativeTestRenderer(t), newNativeTestRenderer(t)
		ref.scene.bulkOff = true
		size := geom.Size{W: 740, H: 190}
		check := func(name string, l *paint.List) {
			t.Helper()
			nativeFrame(t, r, l, size, scale)
			nativeFrame(t, ref, l, size, scale)
			nearImage(t, nativeSnapshot(t, r), nativeSnapshot(t, ref), 2)
		}
		check("first", translucent(0))
		check("entry", translucent(1)) // enters bulk mode
		if !r.scene.bulkMerged {
			t.Fatal("translucent grid did not merge on entry")
		}
		check("translucent", translucent(2))
		check("covered", covered(3)) // lift unsafe: re-planned per label
		if r.scene.bulkMerged {
			t.Fatal("covered labels were lifted above the translucent fill")
		}
		check("uncovered", translucent(4))
		if !r.scene.bulkMerged {
			t.Fatal("bulk did not resume after the fill went away")
		}
	}
}
