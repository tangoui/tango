package paint

import (
	"image/color"
	"math"
	"slices"

	"github.com/tangoui/tango/ui/geom"
)

// RampWidth is the number of entries of a gradient color ramp: one per
// t = i/255, i = 0..255.
const RampWidth = 256

// GradientRamp evaluates stops at t = i/255 for every ramp entry i. Colors
// are interpolated linearly in non-premultiplied sRGB (alpha included), as
// CSS does, and kept at 16 bits per channel (an 8-bit stop channel v is
// v*257): a ramp of 8-bit entries would be a staircase for gradients
// spanning few levels (#f0f2f5→#e6e8eb changes by one level every 25
// entries), leaving the ordered dither (Dither4x4) nothing between levels
// to spread. Before the first stop the ramp holds the first stop's color
// and after the last the last's. Stops are stably sorted by Offset first
// (a copy; stops is not modified), so unsorted stops work and stops
// sharing an Offset keep their order: two stops at one offset are a hard
// edge, the earlier one's color holding at exactly that offset. A stop
// with a NaN Offset is ignored and ±Inf offsets count as 1 / 0. No
// (usable) stops is transparent.
//
// Both renderers draw gradients through this ramp: paint.CPU samples it
// with RampSample, the GPU renderer uploads it (premultiplied, RampPremul)
// as a row of its ramp texture.
func GradientRamp(stops []Stop) [RampWidth]color.NRGBA64 {
	var ramp [RampWidth]color.NRGBA64
	if len(stops) == 0 {
		return ramp
	}
	s := finiteStops(stops)
	if len(s) == 0 {
		return ramp
	}
	if !slices.IsSortedFunc(s, cmpStop) {
		if &s[0] == &stops[0] {
			s = slices.Clone(stops)
		}
		slices.SortStableFunc(s, cmpStop)
	}
	for i := range ramp {
		ramp[i] = stopColor64(s, float32(i)/255)
	}
	return ramp
}

// finiteStops returns stops without NaN offsets and with ±Inf offsets
// clamped to 1 / 0 (a copy when anything changes; stops itself otherwise),
// so non-finite offsets neither produce garbage colors nor break sorting.
func finiteStops(stops []Stop) []Stop {
	ok := func(s Stop) bool { return !math.IsNaN(float64(s.Offset)) && !math.IsInf(float64(s.Offset), 0) }
	if !slices.ContainsFunc(stops, func(s Stop) bool { return !ok(s) }) {
		return stops
	}
	out := make([]Stop, 0, len(stops))
	for _, s := range stops {
		switch {
		case math.IsNaN(float64(s.Offset)):
			continue
		case math.IsInf(float64(s.Offset), 1):
			s.Offset = 1
		case math.IsInf(float64(s.Offset), -1):
			s.Offset = 0
		}
		out = append(out, s)
	}
	return out
}

func cmpStop(a, b Stop) int {
	switch {
	case a.Offset < b.Offset:
		return -1
	case a.Offset > b.Offset:
		return 1
	}
	return 0
}

// stopColor64 evaluates sorted stops at t, interpolating non-premultiplied
// channels at 16 bits.
func stopColor64(stops []Stop, t float32) color.NRGBA64 {
	wide := func(c color.NRGBA) color.NRGBA64 {
		return color.NRGBA64{uint16(c.R) * 0x101, uint16(c.G) * 0x101, uint16(c.B) * 0x101, uint16(c.A) * 0x101}
	}
	if t <= stops[0].Offset {
		return wide(stops[0].Color)
	}
	for i := 1; i < len(stops); i++ {
		b := stops[i]
		if t <= b.Offset {
			a := stops[i-1]
			if b.Offset <= a.Offset {
				return wide(b.Color)
			}
			f := max(0, min(1, (t-a.Offset)/(b.Offset-a.Offset)))
			l := func(x, y uint8) uint16 {
				return uint16((float32(x)+(float32(y)-float32(x))*f)*257 + 0.5)
			}
			return color.NRGBA64{l(a.Color.R, b.Color.R), l(a.Color.G, b.Color.G), l(a.Color.B, b.Color.B), l(a.Color.A, b.Color.A)}
		}
	}
	return wide(stops[len(stops)-1].Color)
}

// RampPremul converts ramp to the row both renderers sample: RampWidth
// premultiplied RGBA texels of 16-bit unsigned normalized channels (the
// GPU's RGBA16Unorm ramp texture), each rounded to nearest.
func RampPremul(ramp *[RampWidth]color.NRGBA64) [RampWidth * 4]uint16 {
	var row [RampWidth * 4]uint16
	for i, c := range ramp {
		a := uint64(c.A)
		row[4*i+0] = uint16((uint64(c.R)*a + 0x7fff) / 0xffff)
		row[4*i+1] = uint16((uint64(c.G)*a + 0x7fff) / 0xffff)
		row[4*i+2] = uint16((uint64(c.B)*a + 0x7fff) / 0xffff)
		row[4*i+3] = c.A
	}
	return row
}

// RampSample returns the premultiplied color (0..1) of the ramp row at
// gradient parameter t, exactly as the GPU's linear-filtered sample of the
// row at u = RampU(t) computes it (shaders.RampU and sl.Texture2D's
// bilinear filter): t is clamped to 0..1 (NaN is 0), then entry
// floor(t*255) and the next are interpolated by the fraction.
func RampSample(row *[RampWidth * 4]uint16, t float32) [4]float32 {
	if !(t > 0) {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	u := (t*255 + 0.5) / RampWidth
	x := u*RampWidth - 0.5
	x = max(-0.5, min(x, RampWidth-0.5))
	x0 := float32(math.Floor(float64(x)))
	f := x - x0
	i := int(x0)
	i0, i1 := min(max(i, 0), RampWidth-1), min(max(i+1, 0), RampWidth-1)
	var c [4]float32
	for k := range c {
		a := float32(row[4*i0+k]) / 0xffff
		b := float32(row[4*i1+k]) / 0xffff
		c[k] = a*(1-f) + b*f
	}
	return c
}

// Dither4x4 is the 4×4 Bayer ordered-dither threshold of device pixel
// (x, y), in (0, 1): (B+0.5)/16 for the Bayer index B in 0..15. Both
// renderers dither gradients with it (DitherPremul); shaders.Dither4x4 is
// the same table (a test keeps them equal).
func Dither4x4(x, y int) float32 {
	m2 := func(i, j int) int { return 2*(i^j) + j } // 2×2 Bayer: 0 2 / 3 1
	b := 4*m2(x&1, y&1) + m2((x>>1)&1, (y>>1)&1)
	return (float32(b) + 0.5) / 16
}

// DitherPremul offsets the color channels of the premultiplied color c
// (0..1) by (d-0.5)/255 for the dither threshold d, clamped to 0..alpha,
// so that rounding the result to 8 bits spreads a gradient's sub-level
// values over neighbouring pixels instead of leaving visible bands. Alpha
// is not dithered; a color exactly on an 8-bit level stays on it (the
// offset is below half a level). shaders.DitherPremul is the same.
func DitherPremul(c [4]float32, d float32) [4]float32 {
	n := (d - 0.5) / 255
	for k := range 3 {
		c[k] = min(max(c[k]+n, 0), c[3])
	}
	return c
}

// GradientT is the gradient parameter (offset 0 at Start, 1 at the end)
// of the point p. It is shaders.GradientT's formula, operation by
// operation (a test keeps them equal):
//
//   - GradientLinear (and unknown kinds): the projection of p - Start onto
//     End - Start, divided by its squared length (0 everywhere when Start
//     == End).
//   - GradientRadial: |p - Start| / |End - Start| (radius 1 when Start ==
//     End).
//   - GradientConic: the angle from the direction End - Start (+x when
//     Start == End) to p - Start, clockwise on screen (y down), in turns,
//     in [0, 1). The arctangent is the Abramowitz–Stegun 4.4.49
//     polynomial (error below 1e-5 rad), the same on every backend.
//
// The result is not clamped; RampSample clamps it.
func GradientT(kind GradientKind, p, start, end geom.Point) float32 {
	dx, dy := end.X-start.X, end.Y-start.Y
	vx, vy := p.X-start.X, p.Y-start.Y
	switch kind {
	case GradientRadial:
		r := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if r == 0 {
			r = 1
		}
		return float32(math.Sqrt(float64(vx*vx+vy*vy))) / r
	case GradientConic:
		if dx == 0 && dy == 0 {
			dx = 1
		}
		t := atan2Poly(dx*vy-dy*vx, dx*vx+dy*vy) * 0.15915494 // 1/2π
		if t < 0 {
			t = t + 1
		}
		return min(t, 0x1.fffffep-1)
	}
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		l2 = 1
	}
	return (vx*dx + vy*dy) / l2
}

// atan2Poly is atan2(y, x) in (-π, π] by the Abramowitz–Stegun 4.4.49
// polynomial for atan on [0, 1] and the octant symmetries; atan2Poly(0, 0)
// is 0. shaders.atan2Poly is the same.
func atan2Poly(y, x float32) float32 {
	ax, ay := abs32(x), abs32(y)
	hi := max(ax, ay)
	if hi == 0 {
		return 0
	}
	a := min(ax, ay) / hi
	s := a * a
	r := a * (0.999866 + s*(-0.3302995+s*(0.180141+s*(-0.085133+s*0.0208351))))
	if ay > ax {
		r = 1.5707964 - r
	}
	if x < 0 {
		r = 3.1415927 - r
	}
	if y < 0 {
		r = -r
	}
	return r
}

// CheckerSelect is the checkerboard color of the pixel centered at p for
// cells of size cell whose grid starts at anchor: 0 for the anchor cell's
// color (Checker.A) and every cell an even number of steps away, 1 for the
// others. It is shaders.CheckerSelect's formula.
func CheckerSelect(p, anchor geom.Point, cell float32) float32 {
	cx := float32(math.Floor(float64((p.X - anchor.X) / cell)))
	cy := float32(math.Floor(float64((p.Y - anchor.Y) / cell)))
	n := (cx + cy) * 0.5
	return (n - float32(math.Floor(float64(n)))) * 2
}

// CheckerColors returns the two colors o draws: A and B, or CheckerLight
// and CheckerDark when both are zero.
func CheckerColors(o Checker) (a, b color.NRGBA) {
	if o.A == (color.NRGBA{}) && o.B == (color.NRGBA{}) {
		return CheckerLight, CheckerDark
	}
	return o.A, o.B
}

// gradientDrawable reports whether a gradient with these stops and points
// draws anything: it needs a stop and finite Start/End (non-finite points
// give no defined parameter; both renderers drop such gradients).
func gradientDrawable(stops []Stop, start, end geom.Point) bool {
	return len(stops) > 0 && finite32(start.X) && finite32(start.Y) && finite32(end.X) && finite32(end.Y)
}

// GradientDrawable reports whether g draws anything (see gradientDrawable);
// the GPU renderer drops the gradients paint.CPU drops.
func GradientDrawable(g Gradient) bool { return gradientDrawable(g.Stops, g.Start, g.End) }

func finite32(f float32) bool { return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0) }
