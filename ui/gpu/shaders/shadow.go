package shaders

import "github.com/tangoui/tango/ui/shader/sl"

// ShadowCoverage returns the coverage of the pixel centered at p by the
// rectangle at origin with the given size and corner radii (top-left,
// top-right, bottom-right, bottom-left) blurred by a Gaussian of standard
// deviation sigma, in 0..1. It is paint.ShadowCoverage's formula (a test
// keeps them equal):
//
//   - The pixel's own box filter is folded into the blur as a Gaussian of
//     variance 1/12, so s = sqrt(sigma² + 1/12) (sigma 0 is an
//     anti-aliased rounded rectangle; a negative or NaN sigma counts as 0).
//   - The square-cornered rectangle's blurred coverage is separable and
//     exact: Φx·Φy, each axis the Gaussian mass between the two edges,
//     0.5·(erf((p-lo)/(s√2)) - erf((p-hi)/(s√2))).
//   - Each rounded corner (radius clamped to half the shorter side, as in
//     RRectCoverage) subtracts the Gaussian mass of the region it cuts
//     off the square corner (shadowCorner).
//
// An empty rectangle covers nothing. Erf is sl.Erf (Abramowitz–Stegun
// 7.1.26), the same polynomial on every backend.
func ShadowCoverage(p, origin, size sl.Vec2, radii sl.Vec4, sigma float32) float32 {
	if !(size.X > 0) || !(size.Y > 0) {
		return 0
	}
	sg := sl.Max(0, sigma)
	s := sl.Sqrt(sg*sg + 0.083333336)
	k := 0.70710677 / s
	reach := 3 * s
	hi := origin.Add(size)
	cx := 0.5 * (sl.Erf((p.X-origin.X)*k) - sl.Erf((p.X-hi.X)*k))
	cy := 0.5 * (sl.Erf((p.Y-origin.Y)*k) - sl.Erf((p.Y-hi.Y)*k))
	m := sl.Min(size.X, size.Y) * 0.5
	l := p.X - origin.X
	t := p.Y - origin.Y
	r := hi.X - p.X
	b := hi.Y - p.Y
	cut := shadowCorner(l, t, sl.Clamp(radii.X, 0, m), k, reach) +
		shadowCorner(r, t, sl.Clamp(radii.Y, 0, m), k, reach) +
		shadowCorner(r, b, sl.Clamp(radii.Z, 0, m), k, reach) +
		shadowCorner(l, b, sl.Clamp(radii.W, 0, m), k, reach)
	return sl.Clamp(cx*cy-cut, 0, 1)
}

// shadowCorner returns the Gaussian mass (k = 1/(s√2), reach = 3s) of the
// region a corner of radius r cuts off a square corner, seen from a pixel
// u, v from the corner along its two edges (positive inside). The region
// is {0 <= y' < r, 0 <= x' < xi(y')} with xi(y') = r - sqrt(r² - (r-y')²).
// It is symmetric in x' and y', so the pixel's offsets may be swapped: the
// larger one is taken along y', which keeps the strips away from y' = 0,
// where xi is steep, whenever the pixel is. The y' range within reach of
// the pixel is cut into 4 equal strips; each strip's Gaussian mass in y' is
// exact (erf) and multiplies the x' mass of [0, xi] at the strip's middle.
// Beyond reach of the corner square (or r <= 0, NaN) it is 0.
func shadowCorner(u, v, r, k, reach float32) float32 {
	if !(r > 0) || u > r+reach || v > r+reach || u < -reach || v < -reach {
		return 0
	}
	a := sl.Min(u, v) // along x'
	c := sl.Max(u, v) // along y'
	y0 := sl.Max(0, c-reach)
	h := (sl.Min(r, c+reach) - y0) * 0.25
	if !(h > 0) {
		return 0
	}
	ea := sl.Erf(a * k)
	e0 := sl.Erf((c - y0) * k)
	acc := float32(0)
	for i := 0; i < 4; i++ {
		y1 := y0 + h*float32(i+1)
		e1 := sl.Erf((c - y1) * k)
		d := r - (y0 + h*(float32(i)+0.5))
		xi := r - sl.Sqrt(sl.Max(r*r-d*d, 0))
		acc = acc + (e0-e1)*(ea-sl.Erf((a-xi)*k))
		e0 = e1
	}
	return acc * 0.25
}
