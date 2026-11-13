package paint

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// SegOp is a path segment verb.
type SegOp uint8

const (
	SegMove SegOp = iota
	SegLine
	SegQuad
	SegCubic
	SegClose
)

// Seg is one path segment; only the first N points are meaningful
// (Move/Line 1, Quad 2, Cubic 3, Close 0).
type Seg struct {
	Op SegOp
	P  [3]geom.Point
}

// Path is a resolution-independent outline in logical pixels.
type Path struct {
	Segs  []Seg
	start geom.Point
	cur   geom.Point
}

// NewPath returns an empty path.
func NewPath() *Path { return &Path{} }

// MoveTo starts a new subpath.
func (p *Path) MoveTo(x, y float32) *Path {
	p.Segs = append(p.Segs, Seg{Op: SegMove, P: [3]geom.Point{{X: x, Y: y}}})
	p.start, p.cur = geom.Pt(x, y), geom.Pt(x, y)
	return p
}

// LineTo adds a straight segment.
func (p *Path) LineTo(x, y float32) *Path {
	p.Segs = append(p.Segs, Seg{Op: SegLine, P: [3]geom.Point{{X: x, Y: y}}})
	p.cur = geom.Pt(x, y)
	return p
}

// QuadTo adds a quadratic Bézier.
func (p *Path) QuadTo(cx, cy, x, y float32) *Path {
	p.Segs = append(p.Segs, Seg{Op: SegQuad, P: [3]geom.Point{{X: cx, Y: cy}, {X: x, Y: y}}})
	p.cur = geom.Pt(x, y)
	return p
}

// CubeTo adds a cubic Bézier.
func (p *Path) CubeTo(c1x, c1y, c2x, c2y, x, y float32) *Path {
	p.Segs = append(p.Segs, Seg{Op: SegCubic, P: [3]geom.Point{{X: c1x, Y: c1y}, {X: c2x, Y: c2y}, {X: x, Y: y}}})
	p.cur = geom.Pt(x, y)
	return p
}

// Close closes the current subpath.
func (p *Path) Close() *Path {
	p.Segs = append(p.Segs, Seg{Op: SegClose})
	p.cur = p.start
	return p
}

// Current returns the pen position.
func (p *Path) Current() geom.Point { return p.cur }

// Arc appends a circular arc centred at (cx,cy) from angle a0 sweeping by
// sweep radians (positive = clockwise in y-down space). If the path is empty
// or closed it starts with MoveTo, otherwise with LineTo.
func (p *Path) Arc(cx, cy, r float32, a0, sweep float64) *Path {
	sx := cx + r*float32(math.Cos(a0))
	sy := cy + r*float32(math.Sin(a0))
	if len(p.Segs) == 0 || p.Segs[len(p.Segs)-1].Op == SegClose {
		p.MoveTo(sx, sy)
	} else {
		p.LineTo(sx, sy)
	}
	n := int(math.Ceil(math.Abs(sweep) / (math.Pi / 2)))
	if n == 0 {
		return p
	}
	step := sweep / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4)
	a := a0
	for i := 0; i < n; i++ {
		b := a + step
		ca, sa := math.Cos(a), math.Sin(a)
		cb, sb := math.Cos(b), math.Sin(b)
		rr := float64(r)
		p.CubeTo(
			cx+float32(rr*(ca-k*sa)), cy+float32(rr*(sa+k*ca)),
			cx+float32(rr*(cb+k*sb)), cy+float32(rr*(sb-k*cb)),
			cx+float32(rr*cb), cy+float32(rr*sb))
		a = b
	}
	return p
}

// Circle appends a closed clockwise circle.
func (p *Path) Circle(cx, cy, r float32) *Path {
	p.MoveTo(cx+r, cy)
	p.Segs = p.Segs[:len(p.Segs)-1]
	p.Arc(cx, cy, r, 0, 2*math.Pi)
	return p.Close()
}

// RRect appends a closed rounded rectangle, clockwise.
func (p *Path) RRect(r geom.Rect, rad geom.Radii) *Path {
	rad = rad.Clamp(r.W, r.H)
	x0, y0, x1, y1 := r.X, r.Y, r.Right(), r.Bottom()
	const k = 0.5522847498
	p.MoveTo(x0+rad.TL, y0)
	p.LineTo(x1-rad.TR, y0)
	if rad.TR > 0 {
		p.CubeTo(x1-rad.TR+rad.TR*k, y0, x1, y0+rad.TR-rad.TR*k, x1, y0+rad.TR)
	}
	p.LineTo(x1, y1-rad.BR)
	if rad.BR > 0 {
		p.CubeTo(x1, y1-rad.BR+rad.BR*k, x1-rad.BR+rad.BR*k, y1, x1-rad.BR, y1)
	}
	p.LineTo(x0+rad.BL, y1)
	if rad.BL > 0 {
		p.CubeTo(x0+rad.BL-rad.BL*k, y1, x0, y1-rad.BL+rad.BL*k, x0, y1-rad.BL)
	}
	p.LineTo(x0, y0+rad.TL)
	if rad.TL > 0 {
		p.CubeTo(x0, y0+rad.TL-rad.TL*k, x0+rad.TL-rad.TL*k, y0, x0+rad.TL, y0)
	}
	return p.Close()
}

// Rect appends a closed rectangle.
func (p *Path) Rect(r geom.Rect) *Path { return p.RRect(r, geom.Radii{}) }

// Transformed returns a copy scaled by s then translated by (dx,dy).
func (p *Path) Transformed(s, dx, dy float32) *Path {
	q := &Path{Segs: make([]Seg, len(p.Segs))}
	for i, sg := range p.Segs {
		for j := range sg.P {
			sg.P[j] = geom.Pt(sg.P[j].X*s+dx, sg.P[j].Y*s+dy)
		}
		q.Segs[i] = sg
	}
	return q
}

// Reversed returns the path with every subpath's direction reversed (used to
// punch holes under the non-zero rule).
func (p *Path) Reversed() *Path {
	var out Path
	for _, sp := range p.Flatten(0.1) {
		n := len(sp.Pts)
		if n == 0 {
			continue
		}
		out.MoveTo(sp.Pts[n-1].X, sp.Pts[n-1].Y)
		for i := n - 2; i >= 0; i-- {
			out.LineTo(sp.Pts[i].X, sp.Pts[i].Y)
		}
		out.Close()
	}
	return &out
}

// Append adds all segments of q.
func (p *Path) Append(q *Path) *Path {
	p.Segs = append(p.Segs, q.Segs...)
	p.cur = q.cur
	return p
}

// Bounds returns the control-point bounding box.
func (p *Path) Bounds() geom.Rect {
	first := true
	var x0, y0, x1, y1 float32
	for _, s := range p.Segs {
		n := 0
		switch s.Op {
		case SegMove, SegLine:
			n = 1
		case SegQuad:
			n = 2
		case SegCubic:
			n = 3
		}
		for i := 0; i < n; i++ {
			pt := s.P[i]
			if first {
				x0, y0, x1, y1 = pt.X, pt.Y, pt.X, pt.Y
				first = false
				continue
			}
			x0, y0 = min(x0, pt.X), min(y0, pt.Y)
			x1, y1 = max(x1, pt.X), max(y1, pt.Y)
		}
	}
	return geom.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Polyline is a flattened subpath.
type Polyline struct {
	Pts    []geom.Point
	Closed bool
}

// Flatten converts curves to line segments with the given tolerance.
func (p *Path) Flatten(tol float32) []Polyline {
	var out []Polyline
	var cur *Polyline
	var pen geom.Point
	ensure := func() {
		if cur == nil {
			out = append(out, Polyline{Pts: []geom.Point{pen}})
			cur = &out[len(out)-1]
		}
	}
	for _, s := range p.Segs {
		switch s.Op {
		case SegMove:
			out = append(out, Polyline{Pts: []geom.Point{s.P[0]}})
			cur = &out[len(out)-1]
			pen = s.P[0]
		case SegLine:
			ensure()
			cur.Pts = append(cur.Pts, s.P[0])
			pen = s.P[0]
		case SegQuad:
			ensure()
			n := curveSteps(pen, s.P[0], s.P[0], s.P[1], tol)
			for i := 1; i <= n; i++ {
				t := float32(i) / float32(n)
				u := 1 - t
				cur.Pts = append(cur.Pts, geom.Pt(
					u*u*pen.X+2*u*t*s.P[0].X+t*t*s.P[1].X,
					u*u*pen.Y+2*u*t*s.P[0].Y+t*t*s.P[1].Y))
			}
			pen = s.P[1]
		case SegCubic:
			ensure()
			n := curveSteps(pen, s.P[0], s.P[1], s.P[2], tol)
			for i := 1; i <= n; i++ {
				t := float32(i) / float32(n)
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				cur.Pts = append(cur.Pts, geom.Pt(
					a*pen.X+b*s.P[0].X+c*s.P[1].X+d*s.P[2].X,
					a*pen.Y+b*s.P[0].Y+c*s.P[1].Y+d*s.P[2].Y))
			}
			pen = s.P[2]
		case SegClose:
			if cur != nil {
				cur.Closed = true
				pen = cur.Pts[0]
			}
			cur = nil
		}
	}
	return out
}

func curveSteps(p0, p1, p2, p3 geom.Point, tol float32) int {
	d := dist(p0, p1) + dist(p1, p2) + dist(p2, p3)
	if tol <= 0 {
		tol = 0.25
	}
	n := int(math.Ceil(math.Sqrt(float64(d / tol))))
	return max(1, min(n, 64))
}

func dist(a, b geom.Point) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// Rotated returns a copy rotated by angle radians around (cx, cy).
func (p *Path) Rotated(angle float64, cx, cy float32) *Path {
	s, c := float32(math.Sin(angle)), float32(math.Cos(angle))
	q := &Path{Segs: make([]Seg, len(p.Segs))}
	for i, sg := range p.Segs {
		for j := range sg.P {
			x, y := sg.P[j].X-cx, sg.P[j].Y-cy
			sg.P[j] = geom.Pt(cx+x*c-y*s, cy+x*s+y*c)
		}
		q.Segs[i] = sg
	}
	return q
}

// Transform returns a copy with every point mapped through m. Affine maps
// keep Bézier segments exact, so no flattening is needed.
func (p *Path) Transform(m geom.Affine) *Path {
	q := &Path{Segs: make([]Seg, len(p.Segs)), start: m.Apply(p.start), cur: m.Apply(p.cur)}
	for i, sg := range p.Segs {
		n := 0
		switch sg.Op {
		case SegMove, SegLine:
			n = 1
		case SegQuad:
			n = 2
		case SegCubic:
			n = 3
		}
		for j := 0; j < n; j++ {
			sg.P[j] = m.Apply(sg.P[j])
		}
		q.Segs[i] = sg
	}
	return q
}

// Skewed returns a copy of p slanted horizontally around y = 0 (the glyph
// baseline): x' = x - k*y, so points above the baseline (y < 0) move right.
func (p *Path) Skewed(k float32) *Path {
	q := &Path{Segs: make([]Seg, len(p.Segs)), start: p.start, cur: p.cur}
	for i, s := range p.Segs {
		for j := range s.P {
			s.P[j].X -= k * s.P[j].Y
		}
		q.Segs[i] = s
	}
	return q
}
