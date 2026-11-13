package chart

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Cartesian maps normalised coordinates (0..1, y up) into a plot rect.
// Transposed swaps the axes: x runs top to bottom and y left to right
// (horizontal bars). Mirror reverses the horizontal pixel axis: it runs
// right to left (x for columns, y for horizontal bars).
type Cartesian struct {
	Plot       geom.Rect
	Transposed bool
	Mirror     bool
}

// HX returns the pixel x of normalised position t on the horizontal pixel
// axis (x, or y when transposed).
func (c Cartesian) HX(t float64) float32 {
	p := c.Plot
	if c.Mirror {
		return p.X + float32(1-t)*p.W
	}
	return p.X + float32(t)*p.W
}

// hNorm inverts HX.
func (c Cartesian) hNorm(x float32) float64 {
	p := c.Plot
	if c.Mirror {
		return float64((p.Right() - x) / p.W)
	}
	return float64((x - p.X) / p.W)
}

// At returns the pixel position of normalised (nx, ny).
func (c Cartesian) At(nx, ny float64) geom.Point {
	p := c.Plot
	if c.Transposed {
		return geom.Pt(c.HX(ny), p.Y+float32(nx)*p.H)
	}
	return geom.Pt(c.HX(nx), p.Bottom()-float32(ny)*p.H)
}

// Norm inverts At.
func (c Cartesian) Norm(pt geom.Point) (nx, ny float64) {
	p := c.Plot
	if p.W <= 0 || p.H <= 0 {
		return 0, 0
	}
	if c.Transposed {
		return float64((pt.Y - p.Y) / p.H), c.hNorm(pt.X)
	}
	return c.hNorm(pt.X), float64((p.Bottom() - pt.Y) / p.H)
}

// Rect returns the pixel rect spanning normalised x0..x1 and y0..y1.
func (c Cartesian) Rect(x0, x1, y0, y1 float64) geom.Rect {
	a, b := c.At(x0, y0), c.At(x1, y1)
	x, y := min(a.X, b.X), min(a.Y, b.Y)
	return geom.Rect{X: x, Y: y, W: abs32(a.X - b.X), H: abs32(a.Y - b.Y)}
}

// Polar maps (t, r) — t the fraction of the sweep, r the fraction between
// Inner and Outer — to pixels. Start is the angle of t = 0 in radians
// (y-down, so -π/2 is 12 o'clock) and Sweep the total angle (2π clockwise
// for a full circle).
type Polar struct {
	Center       geom.Point
	Inner, Outer float32
	Start, Sweep float64
}

// NewPolar centres a full clockwise circle starting at 12 o'clock in r.
func NewPolar(r geom.Rect, inner float64) Polar {
	R := min(r.W, r.H) / 2
	return Polar{Center: r.Center(), Inner: R * float32(inner), Outer: R, Start: -math.Pi / 2, Sweep: 2 * math.Pi}
}

// Mirrored returns p reflected about the vertical line through its centre:
// the start angle is mirrored (θ → π − θ) and the sweep reversed, so a
// clockwise chart becomes counter-clockwise.
func (p Polar) Mirrored() Polar {
	p.Start = math.Pi - p.Start
	p.Sweep = -p.Sweep
	return p
}

// Angle returns the angle of t.
func (p Polar) Angle(t float64) float64 { return p.Start + t*p.Sweep }

// Radius returns the pixel radius of r.
func (p Polar) Radius(r float64) float32 { return p.Inner + float32(r)*(p.Outer-p.Inner) }

// At returns the pixel position of (t, r).
func (p Polar) At(t, r float64) geom.Point {
	a := p.Angle(t)
	rr := p.Radius(r)
	return geom.Pt(p.Center.X+rr*float32(math.Cos(a)), p.Center.Y+rr*float32(math.Sin(a)))
}

// Locate inverts At: t in [0,1) for full circles, and the pixel distance
// from the centre.
func (p Polar) Locate(pt geom.Point) (t float64, dist float32) {
	dx, dy := float64(pt.X-p.Center.X), float64(pt.Y-p.Center.Y)
	dist = float32(math.Hypot(dx, dy))
	a := math.Atan2(dy, dx) - p.Start
	if p.Sweep < 0 {
		a = -a
	}
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a / math.Abs(p.Sweep), dist
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
