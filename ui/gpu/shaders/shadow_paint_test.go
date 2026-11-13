package shaders

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/shader/sl"
)

type shadowCase struct {
	r     geom.Rect
	rad   geom.Radii
	sigma float32
}

var shadowCases = []shadowCase{
	{geom.R(10, 10, 40, 30), geom.Rad(8), 1},
	{geom.R(10, 10, 40, 30), geom.Rad(8), 3},
	{geom.R(10, 10, 40, 30), geom.Rad(8), 8},
	{geom.R(10.3, 6.7, 36.5, 24.25), geom.Radii{TL: 15, TR: 2, BR: 0, BL: 20}, 2},
	{geom.R(10, 10, 40, 30), geom.Rad(15), 0.25},
	{geom.R(10, 10, 40, 30), geom.Rad(15), 1.5},
	{geom.R(12, 12, 60, 40), geom.Rad(20), 6},
	{geom.R(20, 20, 10, 8), geom.Rad(4), 10},
	{geom.R(10, 10, 40, 8), geom.Rad(4), 20},                    // blur much wider than the rect
	{geom.R(10, 10, 40, 30), geom.Rad(30), 0.7},                 // radii > half size: clamped
	{geom.R(10.3, 10.7, 3.5, 2.25), geom.Rad(1), 1},             // sub-pixel-ish rect
	{geom.R(4, 4, 70, 50), geom.Radii{TL: 0, TR: 25, BR: 3}, 4}, // mixed corners
}

func shaderShadow(px, py float32, c shadowCase) float32 {
	return ShadowCoverage(sl.V2(px, py), sl.V2(c.r.X, c.r.Y), sl.V2(c.r.W, c.r.H),
		sl.V4(c.rad.TL, c.rad.TR, c.rad.BR, c.rad.BL), c.sigma)
}

// TestPaintShadowCoverageMatchesShader: paint.CPU and the GPU shader share
// one shadow definition, so paint.ShadowCoverage must equal ShadowCoverage
// at every sample point.
func TestPaintShadowCoverageMatchesShader(t *testing.T) {
	cases := append([]shadowCase{
		{geom.R(3, 3, 0, 20), geom.Rad(4), 2},  // empty
		{geom.R(3, 3, 20, -5), geom.Rad(4), 2}, // negative height
		{geom.R(3, 3, 20, 20), geom.Rad(4), 0}, // no blur: an anti-aliased rrect
	}, shadowCases...)
	for _, c := range cases {
		n := 0
		for y := -4; y < 96; y++ {
			for x := -4; x < 96; x++ {
				for _, o := range [][2]float32{{0.5, 0.5}, {0.13, 0.77}, {0.91, 0.02}} {
					px, py := float32(x)+o[0], float32(y)+o[1]
					got := paint.ShadowCoverage(px, py, c.r, c.rad, c.sigma)
					want := shaderShadow(px, py, c)
					if d := got - want; d > 1e-6 || d < -1e-6 {
						t.Fatalf("rect %v radii %v sigma %v at (%v,%v): paint %v, shader %v", c.r, c.rad, c.sigma, px, py, got, want)
					}
					if !(got >= 0 && got <= 1) {
						t.Fatalf("coverage %v out of range at (%v,%v)", got, px, py)
					}
					if got > 0 {
						n++
					}
				}
			}
		}
		if (n == 0) != c.r.Empty() {
			t.Fatalf("rect %v: %d covered samples", c.r, n)
		}
	}
}

// gaussianShadow is the true shadow: the rounded rectangle convolved with a
// 2D Gaussian of the given sigma, averaged over the unit pixel at (px, py)
// (4×4 sub-samples), all in float64. Per sub-sample the convolution is
// integrated numerically along y (midpoint rule, 400 strips over ±7σ) and
// exactly along x: the Gaussian mass of the rectangle's row [xl(y), xr(y)],
// which the rounded corners indent, by math.Erf.
func gaussianShadow(px, py float64, c shadowCase) float64 {
	x0, y0 := float64(c.r.X), float64(c.r.Y)
	x1, y1 := x0+float64(c.r.W), y0+float64(c.r.H)
	m := math.Min(x1-x0, y1-y0) / 2
	cl := func(v float32) float64 { return math.Max(0, math.Min(float64(v), m)) }
	tl, tr, br, bl := cl(c.rad.TL), cl(c.rad.TR), cl(c.rad.BR), cl(c.rad.BL)
	arc := func(r, d float64) float64 { return r - math.Sqrt(math.Max(r*r-d*d, 0)) } // indent at distance d from the arc center row
	row := func(y float64) (float64, float64) {
		l, r := x0, x1
		if y < y0+tl {
			l = x0 + arc(tl, y0+tl-y)
		} else if y > y1-bl {
			l = x0 + arc(bl, y-(y1-bl))
		}
		if y < y0+tr {
			r = x1 - arc(tr, y0+tr-y)
		} else if y > y1-br {
			r = x1 - arc(br, y-(y1-br))
		}
		return l, r
	}
	sigma := float64(c.sigma)
	k := 1 / (sigma * math.Sqrt2)
	const ss, strips = 4, 400
	total := 0.0
	for sy := 0; sy < ss; sy++ {
		for sx := 0; sx < ss; sx++ {
			qx := px - 0.5 + (float64(sx)+0.5)/ss
			qy := py - 0.5 + (float64(sy)+0.5)/ss
			lo, hi := math.Max(y0, qy-7*sigma), math.Min(y1, qy+7*sigma)
			if !(hi > lo) {
				continue
			}
			dy := (hi - lo) / strips
			for i := 0; i < strips; i++ {
				y := lo + (float64(i)+0.5)*dy
				l, r := row(y)
				g := math.Exp(-(qy-y)*(qy-y)/(2*sigma*sigma)) / (sigma * math.Sqrt(2*math.Pi))
				total += g * dy * 0.5 * (math.Erf((qx-l)*k) - math.Erf((qx-r)*k))
			}
		}
	}
	return total / (ss * ss)
}

// TestShadowMatchesGaussianReference: the closed-form approximation stays
// within 0.02 of the true Gaussian blur at every pixel, for blur radii from
// a quarter pixel to 20 and corner radii up to (and beyond) half the size.
func TestShadowMatchesGaussianReference(t *testing.T) {
	if testing.Short() {
		t.Skip("numerical integration is slow")
	}
	worst := 0.0
	for _, c := range shadowCases {
		reach := float32(3*c.sigma + 3)
		maxErr, at := 0.0, [2]float32{}
		for y := float32(math.Floor(float64(c.r.Y - reach))); y < c.r.Bottom()+reach; y++ {
			for x := float32(math.Floor(float64(c.r.X - reach))); x < c.r.Right()+reach; x++ {
				px, py := x+0.5, y+0.5
				got := float64(shaderShadow(px, py, c))
				want := gaussianShadow(float64(px), float64(py), c)
				if e := math.Abs(got - want); e > maxErr {
					maxErr, at = e, [2]float32{px, py}
				}
			}
		}
		t.Logf("rect %v radii %v sigma %v: max error %.4f at %v", c.r, c.rad, c.sigma, maxErr, at)
		if maxErr > 0.02 {
			t.Errorf("rect %v radii %v sigma %v: max error %.4f at %v, want <= 0.02", c.r, c.rad, c.sigma, maxErr, at)
		}
		worst = math.Max(worst, maxErr)
	}
	t.Logf("worst error %.4f", worst)
}

// TestShadowCoverageExtremes: huge blur, huge or negative radii, empty,
// inverted, infinite and NaN inputs give finite coverage in 0..1 (no NaN
// black blocks), and degenerate rectangles nothing.
func TestShadowCoverageExtremes(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, c := range []shadowCase{
		{geom.R(10, 10, 40, 30), geom.Rad(8), 80},
		{geom.R(10, 10, 40, 30), geom.Rad(1000), 3},
		{geom.R(10, 10, 40, 30), geom.Rad(-5), 3},
		{geom.R(10, 10, 0.01, 30), geom.Rad(8), 0.5},
		{geom.R(10, 10, 40, 30), geom.Rad(8), 1e-7},
		{geom.R(10, 10, 40, 30), geom.Rad(8), -3},
		{geom.R(10, 10, 40, 30), geom.Rad(8), nan},
		{geom.R(10, 10, 40, 30), geom.Radii{TL: nan, TR: inf}, 3},
		{geom.R(10, 10, inf, 30), geom.Rad(8), 3},
		{geom.R(10, 10, 0, 0), geom.Rad(8), 3},
		{geom.R(10, 10, -40, 30), geom.Rad(8), 3},
	} {
		for y := float32(-20); y < 80; y += 0.7 {
			for x := float32(-20); x < 80; x += 0.7 {
				got := shaderShadow(x, y, c)
				if !(got >= 0 && got <= 1) {
					t.Fatalf("rect %v radii %v sigma %v at (%v,%v): coverage %v", c.r, c.rad, c.sigma, x, y, got)
				}
				if p := paint.ShadowCoverage(x, y, c.r, c.rad, c.sigma); p != got && !(math.Abs(float64(p-got)) <= 1e-6) {
					t.Fatalf("rect %v radii %v sigma %v at (%v,%v): paint %v, shader %v", c.r, c.rad, c.sigma, x, y, p, got)
				}
				if (c.r.W <= 0 || c.r.H <= 0) && got != 0 {
					t.Fatalf("empty rect %v covers %v", c.r, got)
				}
			}
		}
	}
	// Far inside a big rect the coverage is 1; far outside it is 0.
	big := shadowCase{geom.R(0, 0, 200, 200), geom.Rad(10), 5}
	if got := shaderShadow(100, 100, big); got != 1 {
		t.Errorf("center coverage %v, want 1", got)
	}
	if got := shaderShadow(-40, 100, big); got != 0 {
		t.Errorf("coverage 8σ outside %v, want 0", got)
	}
	// Edge midpoint: half.
	if got := shaderShadow(0, 100, big); math.Abs(float64(got)-0.5) > 1e-3 {
		t.Errorf("edge coverage %v, want 0.5", got)
	}
}
