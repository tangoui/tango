package paint

import (
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

var (
	rampRed  = color.NRGBA{255, 0, 0, 255}
	rampBlue = color.NRGBA{0, 0, 255, 255}
	red64    = wide64(rampRed)
	blue64   = wide64(rampBlue)
)

func wide64(c color.NRGBA) color.NRGBA64 {
	return color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
}

// lerp64 interpolates non-premultiplied 8-bit colors at 16 bits, as
// GradientRamp does.
func lerp64(a, b color.NRGBA, f float32) color.NRGBA64 {
	l := func(x, y uint8) uint16 { return uint16((float32(x)+(float32(y)-float32(x))*f)*257 + 0.5) }
	return color.NRGBA64{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

// TestGradientRampTwoStops: entries are CSS's non-premultiplied sRGB
// interpolation at t = i/255 (16 bits per channel), clamped beyond the end
// stops.
func TestGradientRampTwoStops(t *testing.T) {
	clear := color.NRGBA{0, 0, 255, 0}
	ramp := GradientRamp([]Stop{{0.25, rampRed}, {0.75, clear}})
	for i, want := range map[int]color.NRGBA64{
		0:   red64,
		63:  red64,                              // t = 0.247 < first offset
		128: lerp64(rampRed, clear, rampF(128)), // non-premultiplied
		192: wide64(clear),                      // t = 0.753 > last offset
		255: wide64(clear),                      // clamped
		102: lerp64(rampRed, clear, rampF(102)),
	} {
		if got := ramp[i]; got != want {
			t.Errorf("ramp[%d] = %v, want %v", i, got, want)
		}
	}
	// Sub-level precision: #f0→#e6 over the ramp changes every entry.
	low := GradientRamp([]Stop{{0, color.NRGBA{0xf0, 0xf2, 0xf5, 255}}, {1, color.NRGBA{0xe6, 0xe8, 0xeb, 255}}})
	for i := 1; i < RampWidth; i++ {
		if low[i].R >= low[i-1].R {
			t.Fatalf("low-contrast ramp not strictly decreasing at %d: %d, %d", i, low[i-1].R, low[i].R)
		}
	}
}

// rampF is entry i's fraction between the stops at 0.25 and 0.75, in the
// float32 arithmetic GradientRamp uses.
func rampF(i int) float32 {
	t := float32(i) / 255
	return (t - 0.25) / (0.75 - 0.25)
}

// TestGradientRampDegenerate: a single stop fills the ramp; two stops at
// the same offset are a hard edge (the earlier one wins at exactly that
// offset); unsorted stops are stably sorted by Offset first; offsets
// outside 0..1 extend the gradient; no stops is transparent.
func TestGradientRampDegenerate(t *testing.T) {
	for _, c := range GradientRamp([]Stop{{0.3, rampBlue}}) {
		if c != blue64 {
			t.Fatalf("single stop: %v", c)
		}
	}
	hard := GradientRamp([]Stop{{0, rampRed}, {0.5, rampRed}, {0.5, rampBlue}, {1, rampBlue}})
	for i, c := range hard {
		want := red64
		if float32(i)/255 > 0.5 {
			want = blue64
		}
		if c != want {
			t.Fatalf("hard edge: ramp[%d] = %v, want %v", i, c, want)
		}
	}
	// Two stops alone at one offset: before it the first, after it the second.
	two := GradientRamp([]Stop{{0.5, rampRed}, {0.5, rampBlue}})
	if two[127] != red64 || two[128] != blue64 {
		t.Fatalf("same-offset pair: %v %v", two[127], two[128])
	}
	sorted := GradientRamp([]Stop{{0, rampRed}, {0.4, rampBlue}, {1, color.NRGBA{0, 255, 0, 255}}})
	unsorted := GradientRamp([]Stop{{1, color.NRGBA{0, 255, 0, 255}}, {0, rampRed}, {0.4, rampBlue}})
	if sorted != unsorted {
		t.Fatal("unsorted stops differ from sorted ones")
	}
	// Stable: equal offsets keep their order (red then blue → blue after).
	st := GradientRamp([]Stop{{1, rampBlue}, {0.5, rampRed}, {0.5, rampBlue}, {0, rampRed}})
	if st != hard {
		t.Fatal("stable sort lost the order of equal offsets")
	}
	out := GradientRamp([]Stop{{-1, rampRed}, {2, rampBlue}})
	if want := lerp64(rampRed, rampBlue, (0-(-1))/float32(2-(-1))); out[0] != want {
		t.Fatalf("offset -1..2: ramp[0] = %v, want %v", out[0], want)
	}
	if empty := GradientRamp(nil); empty[0] != (color.NRGBA64{}) || empty[255] != (color.NRGBA64{}) {
		t.Fatal("no stops: want transparent")
	}
	// The input slice is not reordered.
	in := []Stop{{1, rampBlue}, {0, rampRed}}
	GradientRamp(in)
	if in[0].Offset != 1 {
		t.Fatal("GradientRamp sorted its argument in place")
	}
}

// TestRampPremul: rows are premultiplied 16-bit, rounded to nearest; an
// 8-bit level stays exactly on its 16-bit value.
func TestRampPremul(t *testing.T) {
	var ramp [RampWidth]color.NRGBA64
	ramp[0] = color.NRGBA64{0xffff, 0x8000, 0x0101, 0x8000}
	ramp[1] = wide64(color.NRGBA{10, 20, 30, 255})
	row := RampPremul(&ramp)
	if got := row[0:4]; got[0] != 0x8000 || got[1] != 0x4000 || got[2] != 0x0081 || got[3] != 0x8000 {
		t.Fatalf("premul %#x", got)
	}
	if got := row[4:8]; got[0] != 10*257 || got[1] != 20*257 || got[2] != 30*257 || got[3] != 0xffff {
		t.Fatalf("opaque %v", got)
	}
}

// TestRampSample: t = i/255 lands on entry i; between entries the
// premultiplied values are interpolated linearly; t is clamped to 0..1 and
// NaN samples entry 0.
func TestRampSample(t *testing.T) {
	var row [RampWidth * 4]uint16
	for i := range RampWidth {
		row[4*i], row[4*i+3] = uint16(i)*257, 0xffff
	}
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 2e-6 }
	for _, i := range []int{0, 1, 77, 128, 254, 255} {
		c := RampSample(&row, float32(i)/255)
		if !near(c[0], float32(i)/255) || !near(c[3], 1) {
			t.Fatalf("t=%d/255: %v", i, c)
		}
	}
	if c := RampSample(&row, 10.5/255); !near(c[0], 10.5/255) {
		t.Fatalf("half way: %v", c)
	}
	for _, tt := range []float32{-3, float32(math.NaN()), float32(math.Inf(-1))} {
		if c := RampSample(&row, tt); c[0] != 0 {
			t.Fatalf("t=%v: %v, want entry 0", tt, c)
		}
	}
	for _, tt := range []float32{1.5, float32(math.Inf(1))} {
		if c := RampSample(&row, tt); !near(c[0], 1) {
			t.Fatalf("t=%v: %v, want entry 255", tt, c)
		}
	}
}

// TestDither4x4: the thresholds of a 4×4 tile are (0..15 + 0.5)/16, each
// once, in the Bayer order, repeating every 4 pixels (also for negative
// coordinates).
func TestDither4x4(t *testing.T) {
	bayer := [4][4]int{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	for y := -8; y < 8; y++ {
		for x := -8; x < 8; x++ {
			want := (float32(bayer[y&3][x&3]) + 0.5) / 16
			if got := Dither4x4(x, y); got != want {
				t.Fatalf("(%d,%d): %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestDitherPremul: the offset stays below half a level, so 8-bit levels
// round back to themselves, color channels stay within 0..alpha, and a
// sub-level value is spread so the 4×4 tile's average rounds to it.
func TestDitherPremul(t *testing.T) {
	for _, v := range []uint8{0, 1, 128, 254, 255} {
		for y := range 4 {
			for x := range 4 {
				c := DitherPremul([4]float32{float32(v) / 255, 0, 0, 1}, Dither4x4(x, y))
				if unorm8(c[0]) != v {
					t.Fatalf("level %d at (%d,%d) → %d", v, x, y, unorm8(c[0]))
				}
			}
		}
	}
	if c := DitherPremul([4]float32{0.5, 0.5, 0.5, 0.5}, 1); c[0] > 0.5 {
		t.Fatalf("channel above alpha: %v", c)
	}
	sum := 0
	for y := range 4 {
		for x := range 4 {
			sum += int(unorm8(DitherPremul([4]float32{240.25 / 255, 0, 0, 1}, Dither4x4(x, y))[0]))
		}
	}
	if sum != 240*16+4 {
		t.Fatalf("tile sum %d, want 240.25 × 16", sum)
	}
}

// TestGradientT: the gradient parameter of each kind.
func TestGradientT(t *testing.T) {
	s, e := geom.Pt(10, 20), geom.Pt(30, 20)
	for _, c := range []struct {
		kind GradientKind
		p    geom.Point
		want float32
	}{
		{GradientLinear, geom.Pt(10, 50), 0},
		{GradientLinear, geom.Pt(20, -5), 0.5},
		{GradientLinear, geom.Pt(40, 0), 1.5},
		{GradientLinear, geom.Pt(0, 0), -0.5},
		{GradientRadial, geom.Pt(10, 20), 0},
		{GradientRadial, geom.Pt(10, 30), 0.5},
		{GradientRadial, geom.Pt(-6, 32), 1},
		{GradientConic, geom.Pt(20, 20), 0},     // along End
		{GradientConic, geom.Pt(10, 25), 0.25},  // below: clockwise on screen
		{GradientConic, geom.Pt(0, 20), 0.5},    // opposite
		{GradientConic, geom.Pt(10, 10), 0.75},  // above
		{GradientConic, geom.Pt(10, 20), 0},     // the center itself
		{GradientConic, geom.Pt(20, 30), 0.125}, // diagonal
		{GradientKind(9), geom.Pt(20, 0), 0.5},  // unknown kinds are linear
	} {
		if got := GradientT(c.kind, c.p, s, e); math.Abs(float64(got-c.want)) > 3e-6 {
			t.Errorf("kind %d at %v: %v, want %v", c.kind, c.p, got, c.want)
		}
	}
	// Start == End: linear and radial are 0 at Start, conic starts along +x.
	if got := GradientT(GradientLinear, geom.Pt(5, 5), s, s); got != 0 {
		t.Errorf("degenerate linear %v", got)
	}
	if got := GradientT(GradientRadial, geom.Pt(10, 23), s, s); got != 3 {
		t.Errorf("degenerate radial %v (radius 1)", got)
	}
	if got := GradientT(GradientConic, geom.Pt(10, 25), s, s); math.Abs(float64(got-0.25)) > 3e-6 {
		t.Errorf("degenerate conic %v", got)
	}
}

// TestGradientConicAccuracy: the conic angle's arctangent polynomial
// (Abramowitz–Stegun 4.4.49) stays within 3e-6 turns of the exact angle
// all the way round, and t is in [0, 1].
func TestGradientConicAccuracy(t *testing.T) {
	s, e := geom.Pt(0, 0), geom.Pt(3, -2)
	a0 := math.Atan2(-2, 3)
	worst := 0.0
	for i := range 3600 {
		a := float64(i) * 2 * math.Pi / 3600
		p := geom.Pt(float32(50*math.Cos(a+a0)), float32(50*math.Sin(a+a0)))
		got := GradientT(GradientConic, p, s, e)
		if !(got >= 0 && got <= 1) {
			t.Fatalf("t %v out of range at %v", got, p)
		}
		want := a / (2 * math.Pi)
		d := math.Abs(float64(got) - want)
		d = min(d, 1-d) // 0 and 1 are the same direction
		worst = max(worst, d)
	}
	if worst > 3e-6 {
		t.Fatalf("worst conic error %v turns", worst)
	}
}

// TestCheckerSelect: the cell index parity from the anchor.
func TestCheckerSelect(t *testing.T) {
	a := geom.Pt(10, 20)
	for _, c := range []struct {
		p    geom.Point
		want float32
	}{
		{geom.Pt(10.5, 20.5), 0},
		{geom.Pt(14.5, 20.5), 1},
		{geom.Pt(14.5, 24.5), 0},
		{geom.Pt(9.5, 20.5), 1}, // left of the anchor
		{geom.Pt(5.5, 15.5), 0},
		{geom.Pt(10.5, 1e6+0.5), 1}, // cell row 249995
	} {
		if got := CheckerSelect(c.p, a, 4); got != c.want {
			t.Errorf("at %v: %v, want %v", c.p, got, c.want)
		}
	}
}

// TestCPUGradientUsesRamp: paint.CPU draws a rect gradient as RampSample of
// GradientT at each device pixel center, dithered (exact source-over,
// rounded once).
func TestCPUGradientUsesRamp(t *testing.T) {
	stops := []Stop{{0, rampRed}, {0.3, color.NRGBA{0, 200, 0, 128}}, {1, rampBlue}}
	for _, kind := range []GradientKind{GradientLinear, GradientRadial, GradientConic} {
		for _, scale := range []float32{1, 2} {
			l := &List{Background: color.NRGBA{255, 255, 255, 255}}
			g := Gradient{Kind: kind, Rect: geom.R(4, 4, 60, 40), Start: geom.Pt(20, 20), End: geom.Pt(60, 36), Stops: stops}
			l.Gradient(g)
			cpu := NewCPU()
			cpu.Render(l, geom.Size{W: 70, H: 50}, scale)
			img := cpu.Image()
			ramp := GradientRamp(stops)
			row := RampPremul(&ramp)
			for y := int(5 * scale); y < int(43*scale); y++ {
				for x := int(5 * scale); x < int(63*scale); x++ {
					p := geom.Pt(float32(x)+0.5, float32(y)+0.5)
					c := DitherPremul(RampSample(&row, GradientT(kind, p, geom.Pt(20*scale, 20*scale), geom.Pt(60*scale, 36*scale))), Dither4x4(x, y))
					want := [4]float32{c[0] + (1 - c[3]), c[1] + (1 - c[3]), c[2] + (1 - c[3]), 1}
					got := img.RGBAAt(x, y)
					for k, v := range []uint8{got.R, got.G, got.B, got.A} {
						if d := math.Abs(float64(v) - float64(want[k])*255); d > 0.51 {
							t.Fatalf("kind %d scale %v at (%d,%d): %v, want %v", kind, scale, x, y, got, want)
						}
					}
				}
			}
		}
	}
}

// TestGradientRampNonFinite: a NaN stop offset is dropped and ±Inf is
// clamped to 1 / 0, so non-finite offsets never produce garbage colors
// (CPU and GPU share the ramp).
func TestGradientRampNonFinite(t *testing.T) {
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	red := color.NRGBA{255, 0, 0, 255}
	blue := color.NRGBA{0, 0, 255, 255}
	green := color.NRGBA{0, 255, 0, 255}
	cases := []struct {
		name      string
		got, want []Stop
	}{
		{"nan dropped", []Stop{{0, red}, {nan, green}, {1, blue}}, []Stop{{0, red}, {1, blue}}},
		{"nan first", []Stop{{nan, green}, {0, red}, {1, blue}}, []Stop{{0, red}, {1, blue}}},
		{"+inf is 1", []Stop{{0, red}, {inf, blue}}, []Stop{{0, red}, {1, blue}}},
		{"-inf is 0", []Stop{{-inf, red}, {1, blue}}, []Stop{{0, red}, {1, blue}}},
		{"all nan", []Stop{{nan, red}, {nan, blue}}, nil},
	}
	for _, c := range cases {
		if g, w := GradientRamp(c.got), GradientRamp(c.want); g != w {
			t.Errorf("%s: ramp differs (entry 128: %v, want %v)", c.name, g[128], w[128])
		}
	}
	in := []Stop{{0, red}, {nan, green}, {inf, blue}}
	GradientRamp(in)
	if in[2].Offset != inf || in[1].Offset == in[1].Offset {
		t.Fatal("GradientRamp modified its argument")
	}
}
