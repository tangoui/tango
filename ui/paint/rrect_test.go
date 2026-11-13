package paint

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func covByte(c float32) int { return int(math.Floor(float64(c)*255 + 0.5)) }

func near(a, b, tol int) bool { return a-b <= tol && b-a <= tol }

// A rounded FillRect uses the shared RRectCoverage definition.
func TestRoundedFillUsesRRectCoverage(t *testing.T) {
	r, rad := geom.R(3.3, 2.6, 30.5, 20.2), geom.Radii{TL: 8, TR: 2.5, BR: 12, BL: 0}
	for _, s := range []float32{1, 1.5, 2} {
		l := &List{}
		l.Fill(r, rad, color.NRGBA{0, 0, 0, 255})
		img := render(t, l, 40, 30, s).Image()
		dr := geom.Rect{X: r.X * s, Y: r.Y * s, W: r.W * s, H: r.H * s}
		drad := geom.Radii{TL: rad.TL * s, TR: rad.TR * s, BR: rad.BR * s, BL: rad.BL * s}
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				want := covByte(RRectCoverage(float32(x)+0.5, float32(y)+0.5, dr, drad))
				if got := int(img.RGBAAt(x, y).A); !near(got, want, 1) {
					t.Fatalf("scale %v (%d,%d): alpha %d, want %d", s, x, y, got, want)
				}
			}
		}
	}
}

// Inside a rounded clip every op is multiplied by the clip coverage on its
// own (as the GPU does), not composited as one masked group: at a partly
// covered edge pixel the lower op shows through the upper one.
func TestRoundedClipMultipliesEachOp(t *testing.T) {
	clip, crad := geom.R(4, 4, 30.5, 20.5), geom.Rad(6)
	l := &List{Background: color.NRGBA{255, 255, 255, 255}}
	l.Clip(clip, crad)
	l.Fill(geom.R(0, 0, 40, 30), geom.Radii{}, color.NRGBA{0, 0, 255, 255})
	l.Fill(geom.R(0, 0, 40, 30), geom.Radii{}, color.NRGBA{255, 0, 0, 255})
	l.Pop()
	img := render(t, l, 40, 30, 1).Image()
	// Pixel 34 straddles the right edge at 34.5: coverage 0.5.
	m := RRectCoverage(34.5, 14.5, clip, crad)
	if m != 0.5 {
		t.Fatalf("setup: coverage %v", m)
	}
	// white → blue·m → red·m: R = 255(1-m)+255m... G = 255(1-m)², B = 255(1-m)m·... computed per op:
	r0, g0, b0 := 255.0, 255.0, 255.0
	mm := float64(m)
	r0, g0, b0 = r0*(1-mm), g0*(1-mm), b0*(1-mm)+255*mm // blue
	r0, g0, b0 = r0*(1-mm)+255*mm, g0*(1-mm), b0*(1-mm) // red
	got := img.RGBAAt(34, 14)
	if !near(int(got.R), int(r0+0.5), 2) || !near(int(got.G), int(g0+0.5), 2) || !near(int(got.B), int(b0+0.5), 2) {
		t.Fatalf("edge pixel %v, want ≈ (%.0f,%.0f,%.0f)", got, r0, g0, b0)
	}
	// Interior unaffected.
	if c := img.RGBAAt(20, 14); c != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("interior %v", c)
	}
}

// Nested rounded clips multiply their coverages.
func TestNestedRoundedClipsMultiply(t *testing.T) {
	a, arad := geom.R(2, 2, 30, 30), geom.Rad(10)
	b, brad := geom.R(2.5, 2.5, 30, 30), geom.Rad(12)
	l := &List{}
	l.Clip(a, arad)
	l.Clip(b, brad)
	l.Fill(geom.R(0, 0, 40, 40), geom.Radii{}, color.NRGBA{0, 0, 0, 255})
	l.Pop()
	l.Pop()
	img := render(t, l, 40, 40, 1).Image()
	checked := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			ca := RRectCoverage(float32(x)+0.5, float32(y)+0.5, a, arad)
			cb := RRectCoverage(float32(x)+0.5, float32(y)+0.5, b, brad)
			want := covByte(ca * cb)
			// Clip rects are snapped to whole pixels (CPU.devRect, the GPU
			// scissor) before the coverage applies.
			if !(image.Pt(x, y).In(image.Rect(2, 2, 32, 32).Intersect(image.Rect(3, 3, 33, 33)))) {
				want = 0
			}
			got := int(img.RGBAAt(x, y).A)
			if !near(got, want, 2) {
				t.Fatalf("(%d,%d): alpha %d, want %d (%.3f × %.3f)", x, y, got, want, ca, cb)
			}
			if ca > 0 && ca < 1 && cb > 0 && cb < 1 {
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no pixel partly covered by both clips")
	}
}

// Ops other than FillRect also respect a rounded clip's coverage (text-free
// smoke check over paths, strokes, gradients, shadows and images).
func TestRoundedClipCoversAllOps(t *testing.T) {
	clip, crad := geom.R(5, 5, 30, 30), geom.Rad(10)
	ops := []func(l *List){
		func(l *List) { l.Path(NewPath().Rect(geom.R(0, 0, 40, 40)), color.NRGBA{0, 0, 0, 255}) },
		func(l *List) { l.Stroke(geom.R(0, 0, 40, 40), geom.Radii{}, 20, color.NRGBA{0, 0, 0, 255}) },
		func(l *List) {
			l.Add(LinearGradient{Rect: geom.R(0, 0, 40, 40), P0: geom.Pt(0, 0), P1: geom.Pt(40, 0),
				From: color.NRGBA{0, 0, 0, 255}, To: color.NRGBA{0, 0, 0, 255}})
		},
		func(l *List) {
			l.Shadows(geom.R(-10, -10, 60, 60), geom.Radii{}, []Shadow{{Blur: 2, Color: color.NRGBA{0, 0, 0, 255}}})
		},
		func(l *List) { l.DrawImage(solidImage(color.NRGBA{0, 0, 0, 255}), geom.R(0, 0, 40, 40), geom.Radii{}) },
		func(l *List) {
			l.Opacity(1)
			l.Fill(geom.R(0, 0, 40, 40), geom.Radii{}, color.NRGBA{0, 0, 0, 255})
			l.Pop()
		},
	}
	for i, op := range ops {
		l := &List{}
		l.Clip(clip, crad)
		op(l)
		l.Pop()
		img := render(t, l, 40, 40, 1).Image()
		for y := 0; y < 40; y++ {
			for x := 0; x < 40; x++ {
				want := covByte(RRectCoverage(float32(x)+0.5, float32(y)+0.5, clip, crad))
				if got := int(img.RGBAAt(x, y).A); !near(got, want, 2) {
					t.Fatalf("op %d (%d,%d): alpha %d, want %d", i, x, y, got, want)
				}
			}
		}
	}
}

func solidImage(c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}
