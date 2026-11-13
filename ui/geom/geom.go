// Package geom holds the small geometry vocabulary shared by every layer of
// the Tango self-drawn UI (layout, paint, text, input). All values are in
// logical pixels unless a doc comment says otherwise; renderers multiply by
// the window scale factor at the very end.
package geom

import "math"

// Point is a position in logical pixels.
type Point struct{ X, Y float32 }

// Pt is shorthand for Point{x, y}.
func Pt(x, y float32) Point { return Point{x, y} }

// Add returns p+q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p-q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Size is a width/height pair.
type Size struct{ W, H float32 }

// Rect is an axis-aligned rectangle given by origin and size.
type Rect struct{ X, Y, W, H float32 }

// R is shorthand for Rect{x, y, w, h}.
func R(x, y, w, h float32) Rect { return Rect{x, y, w, h} }

// Right returns X+W.
func (r Rect) Right() float32 { return r.X + r.W }

// Bottom returns Y+H.
func (r Rect) Bottom() float32 { return r.Y + r.H }

// Size returns the rectangle size.
func (r Rect) Size() Size { return Size{r.W, r.H} }

// Min returns the top-left corner.
func (r Rect) Min() Point { return Point{r.X, r.Y} }

// Center returns the center point.
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Empty reports whether the rectangle has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains reports whether p lies inside r (half-open on the far edges).
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.W && p.Y < r.Y+r.H
}

// Offset translates r.
func (r Rect) Offset(dx, dy float32) Rect { return Rect{r.X + dx, r.Y + dy, r.W, r.H} }

// Inset shrinks r by the insets. The result never has negative size.
func (r Rect) Inset(in Insets) Rect {
	o := Rect{r.X + in.Left, r.Y + in.Top, r.W - in.Left - in.Right, r.H - in.Top - in.Bottom}
	if o.W < 0 {
		o.W = 0
	}
	if o.H < 0 {
		o.H = 0
	}
	return o
}

// Expand grows r by d on every side (negative d shrinks).
func (r Rect) Expand(d float32) Rect { return Rect{r.X - d, r.Y - d, r.W + 2*d, r.H + 2*d} }

// Intersect returns the overlap of r and s (zero Rect when disjoint).
func (r Rect) Intersect(s Rect) Rect {
	x0 := max(r.X, s.X)
	y0 := max(r.Y, s.Y)
	x1 := min(r.Right(), s.Right())
	y1 := min(r.Bottom(), s.Bottom())
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Union returns the smallest rectangle containing r and s. Empty operands
// are ignored.
func (r Rect) Union(s Rect) Rect {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	x0 := min(r.X, s.X)
	y0 := min(r.Y, s.Y)
	x1 := max(r.Right(), s.Right())
	y1 := max(r.Bottom(), s.Bottom())
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Insets are per-edge distances, CSS order.
type Insets struct{ Top, Right, Bottom, Left float32 }

// All returns equal insets on every edge.
func All(v float32) Insets { return Insets{v, v, v, v} }

// VH returns CSS two-value insets: vertical, horizontal.
func VH(v, h float32) Insets { return Insets{v, h, v, h} }

// Horizontal returns Left+Right.
func (i Insets) Horizontal() float32 { return i.Left + i.Right }

// Vertical returns Top+Bottom.
func (i Insets) Vertical() float32 { return i.Top + i.Bottom }

// Add sums two insets.
func (i Insets) Add(o Insets) Insets {
	return Insets{i.Top + o.Top, i.Right + o.Right, i.Bottom + o.Bottom, i.Left + o.Left}
}

// Radii are per-corner radii: top-left, top-right, bottom-right, bottom-left.
type Radii struct{ TL, TR, BR, BL float32 }

// Rad returns equal radii.
func Rad(r float32) Radii { return Radii{r, r, r, r} }

// IsZero reports whether all radii are zero.
func (r Radii) IsZero() bool { return r.TL == 0 && r.TR == 0 && r.BR == 0 && r.BL == 0 }

// Clamp limits each radius so it fits in a w×h box.
func (r Radii) Clamp(w, h float32) Radii {
	m := min(w, h) / 2
	c := func(v float32) float32 { return max(0, min(v, m)) }
	return Radii{c(r.TL), c(r.TR), c(r.BR), c(r.BL)}
}

// Grow adds d to every non-negative radius (used for outlines/shadows).
func (r Radii) Grow(d float32) Radii {
	g := func(v float32) float32 {
		if v+d < 0 {
			return 0
		}
		return v + d
	}
	return Radii{g(r.TL), g(r.TR), g(r.BR), g(r.BL)}
}

// Inf is a convenient "unbounded" length.
var Inf = float32(math.Inf(1))

// IsInf reports whether v is +Inf.
func IsInf(v float32) bool { return math.IsInf(float64(v), 1) }
