package geom

import "math"

// Affine is a 2-D affine transform in the usual
//
//	| A C E |
//	| B D F |
//	| 0 0 1 |
//
// layout: a point (x, y) maps to (A·x + C·y + E, B·x + D·y + F). The zero
// value is degenerate; start from Identity or one of the constructors.
// Angles are radians, positive turns clockwise (y grows downwards).
type Affine struct{ A, B, C, D, E, F float32 }

// Identity is the transform that changes nothing.
func Identity() Affine { return Affine{A: 1, D: 1} }

// Translate moves by (dx, dy).
func Translate(dx, dy float32) Affine { return Affine{A: 1, D: 1, E: dx, F: dy} }

// Scale scales about the origin.
func Scale(sx, sy float32) Affine { return Affine{A: sx, D: sy} }

// Rotate rotates by angle radians (clockwise on screen) about the origin.
func Rotate(angle float64) Affine {
	s, c := math.Sincos(angle)
	// Snap the quarter turns so 90° rotations stay pixel exact.
	if math.Abs(s) < 1e-12 {
		s = 0
	}
	if math.Abs(c) < 1e-12 {
		c = 0
	}
	return Affine{A: float32(c), B: float32(s), C: float32(-s), D: float32(c)}
}

// Skew shears by the given angles (radians): x' = x + tan(ax)·y and
// y' = y + tan(ay)·x, like CSS skew().
func Skew(ax, ay float64) Affine {
	return Affine{A: 1, B: float32(math.Tan(ay)), C: float32(math.Tan(ax)), D: 1}
}

// ScaleAround scales by (sx, sy) keeping c fixed.
func ScaleAround(sx, sy float32, c Point) Affine {
	return Translate(-c.X, -c.Y).Then(Scale(sx, sy)).Then(Translate(c.X, c.Y))
}

// RotateAround rotates by angle radians about c.
func RotateAround(angle float64, c Point) Affine {
	return Translate(-c.X, -c.Y).Then(Rotate(angle)).Then(Translate(c.X, c.Y))
}

// Mul returns the matrix product m·n: the result applies n first, then m.
func (m Affine) Mul(n Affine) Affine {
	return Affine{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

// Then returns the transform that applies m first, then n.
func (m Affine) Then(n Affine) Affine { return n.Mul(m) }

// Apply maps p.
func (m Affine) Apply(p Point) Point {
	return Point{m.A*p.X + m.C*p.Y + m.E, m.B*p.X + m.D*p.Y + m.F}
}

// IsIdentity reports whether m changes nothing.
func (m Affine) IsIdentity() bool { return m == Affine{A: 1, D: 1} }

// Det is the determinant (area scale factor, negative when mirrored).
func (m Affine) Det() float32 { return m.A*m.D - m.B*m.C }

// Invert returns the inverse transform; ok is false when m is singular.
func (m Affine) Invert() (Affine, bool) {
	det := float64(m.A)*float64(m.D) - float64(m.B)*float64(m.C)
	if det == 0 || math.IsNaN(det) {
		return Affine{}, false
	}
	id := 1 / det
	a, b, c, d := float64(m.D)*id, -float64(m.B)*id, -float64(m.C)*id, float64(m.A)*id
	e := -(a*float64(m.E) + c*float64(m.F))
	f := -(b*float64(m.E) + d*float64(m.F))
	return Affine{float32(a), float32(b), float32(c), float32(d), float32(e), float32(f)}, true
}

// AxisAligned reports whether m only scales (by positive factors) and
// translates, so rectangles stay rectangles with the same orientation.
func (m Affine) AxisAligned() bool { return m.B == 0 && m.C == 0 && m.A > 0 && m.D > 0 }

// MaxScale is the largest factor by which m stretches a length.
func (m Affine) MaxScale() float32 {
	a, b, c, d := float64(m.A), float64(m.B), float64(m.C), float64(m.D)
	// Largest singular value of the 2×2 part.
	s1 := a*a + b*b + c*c + d*d
	s2 := math.Sqrt(math.Max(0, (a*a+b*b-c*c-d*d)*(a*a+b*b-c*c-d*d)+4*(a*c+b*d)*(a*c+b*d)))
	return float32(math.Sqrt((s1 + s2) / 2))
}

// Bounds returns the axis-aligned bounding box of r after m.
func (m Affine) Bounds(r Rect) Rect {
	p := [4]Point{m.Apply(Point{r.X, r.Y}), m.Apply(Point{r.Right(), r.Y}), m.Apply(Point{r.Right(), r.Bottom()}), m.Apply(Point{r.X, r.Bottom()})}
	x0, y0, x1, y1 := p[0].X, p[0].Y, p[0].X, p[0].Y
	for _, q := range p[1:] {
		x0, y0 = min(x0, q.X), min(y0, q.Y)
		x1, y1 = max(x1, q.X), max(y1, q.Y)
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}
