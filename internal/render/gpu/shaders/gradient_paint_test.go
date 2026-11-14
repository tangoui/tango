package shaders

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestPaintGradientTMatchesShader: paint.CPU and the GPU shader share one
// gradient parameter definition, so paint.GradientT must equal GradientT
// (kind as the float the instance carries) at every sample point.
func TestPaintGradientTMatchesShader(t *testing.T) {
	ends := [][2]geom.Point{
		{geom.Pt(10, 20), geom.Pt(70, 50)},
		{geom.Pt(40.3, 30.7), geom.Pt(41, 12.25)},
		{geom.Pt(-20, 5), geom.Pt(120, 5)},
		{geom.Pt(30, 30), geom.Pt(30, 30)},     // Start == End
		{geom.Pt(0.5, 60), geom.Pt(80, -10.5)}, // steep diagonal
		{geom.Pt(25, 25), geom.Pt(25, 26)},     // tiny radius
	}
	for _, kind := range []paint.GradientKind{paint.GradientLinear, paint.GradientRadial, paint.GradientConic} {
		for _, e := range ends {
			for y := -4; y < 80; y++ {
				for x := -4; x < 96; x++ {
					for _, o := range [][2]float32{{0.5, 0.5}, {0.13, 0.77}} {
						p := geom.Pt(float32(x)+o[0], float32(y)+o[1])
						got := paint.GradientT(kind, p, e[0], e[1])
						want := GradientT(float32(kind), sl.V2(p.X, p.Y), sl.V2(e[0].X, e[0].Y), sl.V2(e[1].X, e[1].Y))
						if d := got - want; d > 1e-6 || d < -1e-6 || got != got {
							t.Fatalf("kind %d %v at %v: paint %v, shader %v", kind, e, p, got, want)
						}
					}
				}
			}
		}
	}
}

// TestPaintCheckerMatchesShader: the checkerboard cell parity is shared.
func TestPaintCheckerMatchesShader(t *testing.T) {
	for _, c := range []struct {
		anchor geom.Point
		cell   float32
	}{{geom.Pt(0, 0), 4}, {geom.Pt(10.5, 3.25), 6}, {geom.Pt(-7, 40), 1.5}, {geom.Pt(3, 3), 13}} {
		for y := -4; y < 80; y++ {
			for x := -4; x < 96; x++ {
				p := geom.Pt(float32(x)+0.5, float32(y)+0.5)
				got := paint.CheckerSelect(p, c.anchor, c.cell)
				want := CheckerSelect(sl.V2(p.X, p.Y), sl.V2(c.anchor.X, c.anchor.Y), c.cell)
				if got != want {
					t.Fatalf("anchor %v cell %v at %v: paint %v, shader %v", c.anchor, c.cell, p, got, want)
				}
			}
		}
	}
}

// TestRampSampleMatchesTexture: the shader's ramp lookup (RampU, then a
// linear-filtered sample at the row center) equals paint.RampSample of the
// same row, so paint.CPU reproduces the GPU's filtering.
func TestRampSampleMatchesTexture(t *testing.T) {
	stops := [][]paint.Stop{
		{{Offset: 0, Color: color.NRGBA{255, 0, 0, 255}}, {Offset: 1, Color: color.NRGBA{0, 0, 255, 255}}},
		{{Offset: 0, Color: color.NRGBA{0x16, 0x77, 0xff, 40}}, {Offset: 0.5, Color: color.NRGBA{10, 200, 30, 255}}, {Offset: 0.5, Color: color.NRGBA{250, 250, 0, 128}}, {Offset: 1, Color: color.NRGBA{}}},
		{{Offset: 0.2, Color: color.NRGBA{0xf0, 0xf2, 0xf5, 255}}, {Offset: 0.9, Color: color.NRGBA{0xe6, 0xe8, 0xeb, 255}}},
	}
	const rows = 4
	img := image.NewRGBA64(image.Rect(0, 0, paint.RampWidth, rows))
	var want [][paint.RampWidth * 4]uint16
	for i, s := range stops {
		ramp := paint.GradientRamp(s)
		row := paint.RampPremul(&ramp)
		for k, v := range row {
			img.Pix[i*img.Stride+2*k], img.Pix[i*img.Stride+2*k+1] = uint8(v>>8), uint8(v) // RGBA64 is big-endian
		}
		want = append(want, row)
	}
	tex := sl.NewTexture(img, sl.Linear)
	for i := range stops {
		v := (float32(i) + 0.5) / rows
		for k := -20; k <= 1020; k++ {
			tt := float32(k) / 1000
			got := tex.Sample(sl.V2(RampU(tt), v))
			ref := paint.RampSample(&want[i], tt)
			for c, g := range []float32{got.X, got.Y, got.Z, got.W} {
				if d := g - ref[c]; d > 1e-6 || d < -1e-6 {
					t.Fatalf("stops %d t=%v: texture %v, paint %v", i, tt, got, ref)
				}
			}
		}
	}
}

// TestPaintDitherMatchesShader: the ordered-dither table and its
// application are shared.
func TestPaintDitherMatchesShader(t *testing.T) {
	for y := -9; y < 40; y++ {
		for x := -9; x < 40; x++ {
			got := paint.Dither4x4(x, y)
			want := Dither4x4(float32(x), float32(y))
			if got != want {
				t.Fatalf("(%d,%d): paint %v, shader %v", x, y, got, want)
			}
			for _, c := range [][4]float32{{0.3, 0.6, 0.9, 1}, {0, 0.001, 0.4999, 0.5}, {0.2, 0.2, 0.2, 0.2}, {0, 0, 0, 0}} {
				p := paint.DitherPremul(c, got)
				s := DitherPremul(sl.V4(c[0], c[1], c[2], c[3]), want)
				if p != [4]float32{s.X, s.Y, s.Z, s.W} {
					t.Fatalf("(%d,%d) %v: paint %v, shader %v", x, y, c, p, s)
				}
			}
		}
	}
}
