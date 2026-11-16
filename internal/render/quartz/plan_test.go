package quartz

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

type planGlyphSource struct{}

func (*planGlyphSource) GlyphPath(_ uint32, size float32) *paint.Path {
	return paint.NewPath().RRect(geom.Rect{Y: -size, W: 4, H: size}, geom.Radii{})
}

type planGlyphs struct{}

func (planGlyphs) Available(src paint.GlyphSource) bool { _, ok := src.(*planGlyphSource); return ok }
func (planGlyphs) Key(src paint.GlyphSource) uintptr {
	if src != nil {
		return 1
	}
	return 0
}
func (planGlyphs) Bounds(run nativeglyph.Run) (image.Rectangle, bool) {
	var b image.Rectangle
	for _, p := range run.Positions {
		b = b.Union(image.Rect(int(math.Floor(float64(p.X))), int(p.Y), int(math.Ceil(float64(p.X+4))), int(p.Y+6)))
	}
	return b, true
}
func (planGlyphs) Draw(uintptr, nativeglyph.Run) bool { return true }

func TestPlanTextContentIndependentOfIntegerMovement(t *testing.T) {
	s := newScene(planGlyphs{})
	src := &planGlyphSource{}
	glyphs := []paint.Glyph{{Src: src, ID: 3, Size: 10, X: 1.25, Y: 4}}
	l := &paint.List{Ops: []paint.Op{paint.Text{Origin: geom.Pt(10, 20), Glyphs: glyphs, Color: color.NRGBA{A: 255}}}}
	if e := s.plan(l, geom.Size{W: 100, H: 100}, 2); e != nil {
		t.Fatal(e)
	}
	if s.full || len(s.items) != 1 || s.items[0].kind != itemText {
		t.Fatal("native text not planned")
	}
	old := s.items[0]
	if old.frame != image.Rect(22, 42, 27, 48) {
		t.Fatalf("frame %v", old.frame)
	}
	l.Ops[0] = paint.Text{Origin: geom.Pt(15, 27), Glyphs: glyphs, Color: color.NRGBA{A: 255}}
	if e := s.plan(l, geom.Size{W: 100, H: 100}, 2); e != nil {
		t.Fatal(e)
	}
	if s.items[0].key != old.key || s.items[0].frame.Min.Sub(old.frame.Min) != image.Pt(10, 14) {
		t.Fatal("integer movement changed text contents")
	}
	glyphs[0].ID = 9
	if old.runs[0].IDs[0] != 3 {
		t.Fatal("retained glyph IDs alias caller storage")
	}
	if e := s.plan(l, geom.Size{W: 100, H: 100}, 2); e != nil {
		t.Fatal(e)
	}
	if s.items[0].key == old.key || s.items[0].runs[0].IDs[0] != 9 {
		t.Fatal("mutated glyph storage did not invalidate contents")
	}
}

func TestPlanClipAndPointerOps(t *testing.T) {
	s := newScene(nil)
	fill := paint.FillRect{Rect: geom.Rect{X: 5, Y: 5, W: 20, H: 20}, Color: color.NRGBA{R: 255, A: 255}}
	l := &paint.List{Ops: []paint.Op{&paint.PushClip{Rect: geom.Rect{W: 10, H: 20}}, &fill, &paint.Pop{}, (*paint.FillRect)(nil)}}
	if e := s.plan(l, geom.Size{W: 100, H: 100}, 1); e != nil {
		t.Fatal(e)
	}
	if s.full || len(s.items) != 1 || s.items[0].frame != image.Rect(5, 5, 10, 20) {
		t.Fatalf("clipped items %+v", s.items)
	}
	if s.items[0].kind != itemSolid {
		t.Fatal("integer solid used a bitmap")
	}
}

func TestPlanPatchPixelParityAndReuse(t *testing.T) {
	red := color.NRGBA{R: 200, G: 20, B: 40, A: 180}
	ops := []paint.Op{
		paint.FillRect{Rect: geom.Rect{X: 10, Y: 10, W: 24, H: 20}, Radii: geom.Radii{TL: 3, TR: 4, BR: 5, BL: 6}, Color: red},
		paint.StrokeRect{Rect: geom.Rect{X: 10, Y: 10, W: 24, H: 20}, Radii: geom.Radii{TL: 4, TR: 4, BR: 4, BL: 4}, Width: 1.5, Color: red},
		paint.ShadowRect{Rect: geom.Rect{X: 10, Y: 10, W: 24, H: 20}, Radii: geom.Radii{TL: 4, TR: 4, BR: 4, BL: 4}, Shadow: paint.Shadow{X: 1, Y: 2, Blur: 6, Spread: 1, Color: red}},
		paint.FillPath{Path: paint.NewPath().MoveTo(10, 10).LineTo(30, 12).LineTo(20, 30).Close(), Color: red},
		paint.Checker{Rect: geom.Rect{X: 10, Y: 10, W: 24, H: 20}, Cell: 3},
	}
	for _, scale := range []float32{1, 1.5, 2} {
		for _, op := range ops {
			s := newScene(nil)
			list := &paint.List{Ops: []paint.Op{op}}
			if e := s.plan(list, geom.Size{W: 80, H: 70}, scale); e != nil {
				t.Fatal(e)
			}
			if s.full || len(s.items) != 1 || s.items[0].kind != itemPatch {
				t.Fatalf("%T did not use a bounded patch", op)
			}
			got := s.pixels(s.items[0])
			ref := paint.NewCPU()
			_ = ref.Render(list, geom.Size{W: 80, H: 70}, scale)
			if got == nil {
				t.Fatal("missing patch")
			}
			for y := 0; y < ref.Image().Rect.Dy(); y++ {
				for x := 0; x < ref.Image().Rect.Dx(); x++ {
					var actual color.RGBA
					if image.Pt(x, y).In(got.Rect) {
						actual = got.RGBAAt(x, y)
					}
					if actual != ref.Image().RGBAAt(x, y) {
						t.Fatalf("%T @%v pixel %d,%d patch %v ref %v bounds %v", op, scale, x, y, actual, ref.Image().RGBAAt(x, y), got.Rect)
					}
				}
			}
		}
	}
	// Equal rounded rows may share one immutable bitmap despite different positions.
	s := newScene(nil)
	first := ops[0].(paint.FillRect)
	list := &paint.List{Ops: []paint.Op{first}}
	_ = s.plan(list, geom.Size{W: 100, H: 100}, 2)
	key := s.items[0].key
	first.Rect.Y += 30
	list.Ops[0] = first
	_ = s.plan(list, geom.Size{W: 100, H: 100}, 2)
	if key != s.items[0].key {
		t.Fatal("integer movement changed rounded patch identity")
	}
}

func TestPlanComplexOpsFallbackCompletely(t *testing.T) {
	s := newScene(nil)
	for _, extra := range []paint.Op{paint.PushOpacity{Alpha: 0.4}, paint.PushTransform{M: geom.Affine{A: 1, D: 1, E: 3}}, paint.PushClip{Rect: geom.Rect{W: 50, H: 50}, Radii: geom.Radii{TL: 4}}, paint.BackdropBlur{Rect: geom.Rect{X: 2, Y: 2, W: 20, H: 20}, Radius: 2}} {
		list := &paint.List{Background: color.NRGBA{R: 30, G: 60, B: 90, A: 128}, Ops: []paint.Op{paint.FillRect{Rect: geom.Rect{W: 50, H: 50}, Color: color.NRGBA{R: 255, A: 128}}, extra, paint.FillRect{Rect: geom.Rect{X: 5, Y: 5, W: 20, H: 20}, Color: color.NRGBA{G: 255, A: 100}}}}
		if e := s.plan(list, geom.Size{W: 50, H: 50}, 1); e != nil {
			t.Fatal(e)
		}
		if !s.full || len(s.items) != 1 {
			t.Fatalf("%T missing full fallback", extra)
		}
		got := s.pixels(s.items[0])
		ref := paint.NewCPU()
		_ = ref.Render(list, geom.Size{W: 50, H: 50}, 1)
		for i, p := range got.Pix {
			if p != ref.Image().Pix[i] {
				t.Fatalf("%T fallback dropped drawing", extra)
			}
		}
	}
}

// Embedding demonstrates a future/unrecognized drawing command without opening
// paint's sealed Op implementation to arbitrary new drawing behavior.
type unknownPaintOp struct{ paint.Op }

func TestPlanRejectsUnknownOps(t *testing.T) {
	s := newScene(nil)
	if err := s.plan(&paint.List{Ops: []paint.Op{unknownPaintOp{}}}, geom.Size{W: 10, H: 10}, 1); err == nil {
		t.Fatal("unknown drawing command was silently omitted")
	}
}

type valueImage struct{ color.RGBA }

func (v valueImage) ColorModel() color.Model { return color.RGBAModel }
func (v valueImage) Bounds() image.Rectangle { return image.Rect(0, 0, 8, 8) }
func (v valueImage) At(int, int) color.Color { return v.RGBA }
func TestPlanDistinctValueImagesNeverShareContents(t *testing.T) {
	s := newScene(nil)
	l := &paint.List{Ops: []paint.Op{
		paint.Image{Img: valueImage{color.RGBA{R: 255, A: 255}}, Rect: geom.Rect{W: 8, H: 8}},
		paint.Image{Img: valueImage{color.RGBA{B: 255, A: 255}}, Rect: geom.Rect{X: 10, W: 8, H: 8}},
	}}
	if err := s.plan(l, geom.Size{W: 30, H: 20}, 1); err != nil {
		t.Fatal(err)
	}
	if len(s.items) != 2 || s.items[0].key == s.items[1].key {
		t.Fatal("distinct value-based image sources shared one native snapshot")
	}
}

func TestPlanBoundsNativeBitmapResidency(t *testing.T) {
	s := newScene(nil)
	l := &paint.List{}
	for i := 0; i < 65; i++ {
		l.Ops = append(l.Ops, paint.FillRect{Rect: geom.Rect{W: 512, H: 512}, Radii: geom.Radii{TL: 4}, Color: color.NRGBA{R: uint8(i), A: 255}})
	}
	if err := s.plan(l, geom.Size{W: 512, H: 512}, 1); err != nil {
		t.Fatal(err)
	}
	if !s.full || len(s.items) != 1 {
		t.Fatal("a frame requiring over 64MB of native bitmaps was not bounded")
	}
	if got := s.pixels(s.items[0]).RGBAAt(100, 100); got.R != 64 || got.A != 255 {
		t.Fatal("bounded fallback omitted later drawing")
	}
}

func TestPlanEmptyFillsNeverDraw(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		for _, r := range []geom.Rect{{X: 20, Y: 10, W: -10, H: 10}, {X: 10, Y: 20, W: 10, H: -10}, {X: 10, Y: 10, H: 10}, {X: 10, Y: 10, W: 10}} {
			s := newScene(nil)
			l := &paint.List{Ops: []paint.Op{paint.PushClip{Rect: geom.Rect{X: 5, Y: 5, W: 30, H: 30}}, paint.FillRect{Rect: r, Color: color.NRGBA{R: 255, A: 255}}, paint.Pop{}}}
			if e := s.plan(l, geom.Size{W: 50, H: 50}, scale); e != nil {
				t.Fatal(e)
			}
			if len(s.items) != 0 {
				t.Fatalf("empty rect %+v @%v produced content", r, scale)
			}
		}
	}
}
func TestPlanGradientSnapshotTracksDeviceDitherPhase(t *testing.T) {
	s := newScene(nil)
	size := geom.Size{W: 80, H: 80}
	for _, scale := range []float32{1, 1.5, 2} {
		a := paint.Gradient{Rect: geom.Rect{X: 8, Y: 8, W: 16, H: 20}, Start: geom.Pt(8, 8), End: geom.Pt(8, 28), Stops: []paint.Stop{{Offset: 0, Color: color.NRGBA{R: 31, A: 255}}, {Offset: 1, Color: color.NRGBA{R: 95, A: 255}}}}
		if e := s.plan(&paint.List{Ops: []paint.Op{a}}, size, scale); e != nil {
			t.Fatal(e)
		}
		key := s.items[0].key
		step := 1 / scale
		a.Rect.X += step
		a.Start.X += step
		a.End.X += step
		if e := s.plan(&paint.List{Ops: []paint.Op{a}}, size, scale); e != nil {
			t.Fatal(e)
		}
		if key == s.items[0].key {
			t.Fatalf("gradient @%v reused bitmap from a different dither phase", scale)
		}
	}
}
func TestPlanCheckerSnapshotTracksResolvedDefaults(t *testing.T) {
	oldLight, oldDark := paint.CheckerLight, paint.CheckerDark
	t.Cleanup(func() { paint.CheckerLight, paint.CheckerDark = oldLight, oldDark })
	s := newScene(nil)
	l := &paint.List{Ops: []paint.Op{paint.Checker{Rect: geom.Rect{W: 20, H: 20}, Cell: 4}}}
	size := geom.Size{W: 30, H: 30}
	if e := s.plan(l, size, 1); e != nil {
		t.Fatal(e)
	}
	key := s.items[0].key
	paint.CheckerLight = color.NRGBA{R: 255, A: 255}
	if e := s.plan(l, size, 1); e != nil {
		t.Fatal(e)
	}
	if key == s.items[0].key {
		t.Fatal("changed checker defaults reused stale snapshot")
	}
}
