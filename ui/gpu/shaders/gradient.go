package shaders

import "github.com/tangoui/tango/ui/shader/sl"

// GradientT is the gradient parameter of the point p for a gradient of
// kind (paint.GradientKind as a float: 0 linear, 1 radial, 2 conic; the
// host sends unknown kinds as 0) from start to end. It is
// paint.GradientT's formula, operation by operation (a test keeps them
// equal): linear projects p - start onto end - start; radial is
// |p - start| / |end - start|; conic is the clockwise angle from end -
// start to p - start in turns, in [0, 1), with the Abramowitz–Stegun
// 4.4.49 arctangent (atan2Poly). Degenerate end == start: linear is 0,
// radial uses radius 1, conic starts along +x.
func GradientT(kind float32, p, start, end sl.Vec2) float32 {
	dx := end.X - start.X
	dy := end.Y - start.Y
	vx := p.X - start.X
	vy := p.Y - start.Y
	if kind > 1.5 {
		if dx == 0 && dy == 0 {
			dx = 1
		}
		t := atan2Poly(dx*vy-dy*vx, dx*vx+dy*vy) * 0.15915494
		if t < 0 {
			t = t + 1
		}
		return sl.Min(t, 0.99999994)
	}
	if kind > 0.5 {
		r := sl.Sqrt(dx*dx + dy*dy)
		if r == 0 {
			r = 1
		}
		return sl.Sqrt(vx*vx+vy*vy) / r
	}
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		l2 = 1
	}
	return (vx*dx + vy*dy) / l2
}

// atan2Poly is atan2(y, x) in (-π, π]: the Abramowitz–Stegun 4.4.49
// polynomial for atan on [0, 1] (error below 1e-5) and the octant
// symmetries; atan2Poly(0, 0) is 0. paint's atan2Poly is the same.
func atan2Poly(y, x float32) float32 {
	ax := sl.Abs(x)
	ay := sl.Abs(y)
	hi := sl.Max(ax, ay)
	if hi == 0 {
		return 0
	}
	a := sl.Min(ax, ay) / hi
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

// RampU maps the gradient parameter t to the horizontal texture
// coordinate of the ramp texture (paint.RampWidth texels per row): t is
// clamped to 0..1 (NaN to 0) and t*255 lands on the texel centers, so
// t = i/255 samples entry i exactly and the linear filter interpolates
// between neighbouring entries (paint.RampSample).
func RampU(t float32) float32 {
	if !(t > 0) {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return (t*255 + 0.5) / 256
}

// CheckerSelect is 0 when the pixel centered at p lies in a checkerboard
// cell an even number of steps from the cell at anchor (cells of size
// cell), 1 otherwise. It is paint.CheckerSelect's formula.
func CheckerSelect(p, anchor sl.Vec2, cell float32) float32 {
	cx := sl.Floor((p.X - anchor.X) / cell)
	cy := sl.Floor((p.Y - anchor.Y) / cell)
	n := (cx + cy) * 0.5
	return (n - sl.Floor(n)) * 2
}

// Dither4x4 is the 4×4 Bayer ordered-dither threshold of the device pixel
// at integer coordinates (x, y), in (0, 1): (B+0.5)/16, B the Bayer index
// built from the two low bits of x and y (2×2 Bayer 0 2 / 3 1, nested).
// It is paint.Dither4x4's table.
func Dither4x4(x, y float32) float32 {
	x0 := x - 2*sl.Floor(x*0.5)
	y0 := y - 2*sl.Floor(y*0.5)
	hx := sl.Floor(x * 0.5)
	hy := sl.Floor(y * 0.5)
	x1 := hx - 2*sl.Floor(hx*0.5)
	y1 := hy - 2*sl.Floor(hy*0.5)
	b := 4*(2*(x0+y0-2*x0*y0)+y0) + 2*(x1+y1-2*x1*y1) + y1
	return (b + 0.5) * 0.0625
}

// DitherPremul offsets the color channels of the premultiplied color c by
// (d-0.5)/255, clamped to 0..alpha (alpha unchanged), so the 8-bit target
// write spreads sub-level gradient values over neighbouring pixels. It is
// paint.DitherPremul.
func DitherPremul(c sl.Vec4, d float32) sl.Vec4 {
	n := (d - 0.5) / 255
	return sl.V4(sl.Clamp(c.X+n, 0, c.W), sl.Clamp(c.Y+n, 0, c.W), sl.Clamp(c.Z+n, 0, c.W), c.W)
}
