package graph

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// xform maps graph units to screen pixels: s = Origin + p·K.
type xform struct {
	Origin geom.Point
	K      float32
}

func (m xform) pt(p geom.Point) geom.Point {
	return geom.Pt(m.Origin.X+p.X*m.K, m.Origin.Y+p.Y*m.K)
}

func (m xform) rect(r geom.Rect) geom.Rect {
	return geom.Rect{X: m.Origin.X + r.X*m.K, Y: m.Origin.Y + r.Y*m.K, W: r.W * m.K, H: r.H * m.K}
}

func (m xform) inv(s geom.Point) geom.Point {
	return geom.Pt((s.X-m.Origin.X)/m.K, (s.Y-m.Origin.Y)/m.K)
}

func (m xform) invRect(r geom.Rect) geom.Rect {
	p := m.inv(geom.Pt(r.X, r.Y))
	return geom.Rect{X: p.X, Y: p.Y, W: r.W / m.K, H: r.H / m.K}
}

// seg is one piece of an edge: a cubic to P (or a straight line when
// Line is set; C1 / C2 are then unused).
type seg struct {
	C1, C2, P geom.Point
	Line      bool
}

// curve is a routed edge in graph units.
type curve struct {
	Start geom.Point
	Segs  []seg
}

func (cv *curve) end() geom.Point {
	if len(cv.Segs) == 0 {
		return cv.Start
	}
	return cv.Segs[len(cv.Segs)-1].P
}

// endDir returns the unit direction the curve arrives at its end with.
func (cv *curve) endDir() geom.Point {
	if len(cv.Segs) == 0 {
		return geom.Pt(1, 0)
	}
	s := cv.Segs[len(cv.Segs)-1]
	from := cv.Start
	if len(cv.Segs) > 1 {
		from = cv.Segs[len(cv.Segs)-2].P
	}
	if !s.Line {
		if d := sub(s.P, s.C2); length(d) > 0.01 {
			return unit(d)
		}
		if d := sub(s.P, s.C1); length(d) > 0.01 {
			return unit(d)
		}
	}
	return unit(sub(s.P, from))
}

// startDir returns the unit direction the curve leaves its start with.
func (cv *curve) startDir() geom.Point {
	if len(cv.Segs) == 0 {
		return geom.Pt(1, 0)
	}
	s := cv.Segs[0]
	if !s.Line {
		if d := sub(s.C1, cv.Start); length(d) > 0.01 {
			return unit(d)
		}
		if d := sub(s.C2, cv.Start); length(d) > 0.01 {
			return unit(d)
		}
	}
	return unit(sub(s.P, cv.Start))
}

// bounds returns a box containing the curve (control polygon hull).
func (cv *curve) bounds() geom.Rect {
	x0, y0, x1, y1 := cv.Start.X, cv.Start.Y, cv.Start.X, cv.Start.Y
	add := func(p geom.Point) {
		x0, y0, x1, y1 = min(x0, p.X), min(y0, p.Y), max(x1, p.X), max(y1, p.Y)
	}
	for _, s := range cv.Segs {
		if !s.Line {
			add(s.C1)
			add(s.C2)
		}
		add(s.P)
	}
	return geom.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// flatten appends a polyline approximation of the curve to dst.
func (cv *curve) flatten(dst []geom.Point) []geom.Point {
	dst = append(dst, cv.Start)
	p0 := cv.Start
	for _, s := range cv.Segs {
		if s.Line {
			dst = append(dst, s.P)
		} else {
			const steps = 16
			for i := 1; i <= steps; i++ {
				dst = append(dst, cubicAt(p0, s.C1, s.C2, s.P, float32(i)/steps))
			}
		}
		p0 = s.P
	}
	return dst
}

// path returns the curve in screen space, trimmed by trimStart / trimEnd
// screen pixels along the end directions (room for arrowheads).
func (cv *curve) path(m xform, trimStart, trimEnd float32) *paint.Path {
	p := paint.NewPath()
	start := m.pt(cv.Start)
	if trimStart > 0 {
		start = add(start, scale(cv.startDir(), trimStart))
	}
	p.MoveTo(start.X, start.Y)
	for i, s := range cv.Segs {
		end := m.pt(s.P)
		if i == len(cv.Segs)-1 && trimEnd > 0 {
			end = sub(end, scale(cv.endDir(), trimEnd))
		}
		if s.Line {
			p.LineTo(end.X, end.Y)
			continue
		}
		c1, c2 := m.pt(s.C1), m.pt(s.C2)
		p.CubeTo(c1.X, c1.Y, c2.X, c2.Y, end.X, end.Y)
	}
	return p
}

func cubicAt(p0, c1, c2, p1 geom.Point, t float32) geom.Point {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return geom.Pt(a*p0.X+b*c1.X+c*c2.X+d*p1.X, a*p0.Y+b*c1.Y+c*c2.Y+d*p1.Y)
}

// polyMid returns the point halfway along a polyline by length.
func polyMid(pts []geom.Point) geom.Point {
	if len(pts) == 0 {
		return geom.Point{}
	}
	var total float32
	for i := 1; i < len(pts); i++ {
		total += length(sub(pts[i], pts[i-1]))
	}
	half := total / 2
	for i := 1; i < len(pts); i++ {
		l := length(sub(pts[i], pts[i-1]))
		if half <= l && l > 0 {
			return lerp(pts[i-1], pts[i], half/l)
		}
		half -= l
	}
	return pts[len(pts)-1]
}

// polyDist returns the distance from p to a polyline.
func polyDist(pts []geom.Point, p geom.Point) float32 {
	best := float32(math.Inf(1))
	for i := 1; i < len(pts); i++ {
		best = min(best, segDist(pts[i-1], pts[i], p))
	}
	if len(pts) == 1 {
		best = length(sub(p, pts[0]))
	}
	return best
}

func segDist(a, b, p geom.Point) float32 {
	ab := sub(b, a)
	l2 := ab.X*ab.X + ab.Y*ab.Y
	if l2 == 0 {
		return length(sub(p, a))
	}
	t := ((p.X-a.X)*ab.X + (p.Y-a.Y)*ab.Y) / l2
	t = max(0, min(1, t))
	return length(sub(p, add(a, scale(ab, t))))
}

func sub(a, b geom.Point) geom.Point           { return geom.Pt(a.X-b.X, a.Y-b.Y) }
func add(a, b geom.Point) geom.Point           { return geom.Pt(a.X+b.X, a.Y+b.Y) }
func scale(a geom.Point, k float32) geom.Point { return geom.Pt(a.X*k, a.Y*k) }
func length(a geom.Point) float32              { return float32(math.Hypot(float64(a.X), float64(a.Y))) }
func lerp(a, b geom.Point, t float32) geom.Point {
	return geom.Pt(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
}
func unit(a geom.Point) geom.Point {
	l := length(a)
	if l == 0 {
		return geom.Pt(1, 0)
	}
	return geom.Pt(a.X/l, a.Y/l)
}

// anchor is where an edge meets a node: the point and the outward normal
// (zero normal: leave in the straight-line direction).
type anchor struct {
	P, N geom.Point
}

// sideAnchor returns the point of side s of r at offset (<= 0: middle).
func sideAnchor(r geom.Rect, s Side, off float32) anchor {
	switch s {
	case SideTop:
		return anchor{geom.Pt(r.X+offOr(off, r.W), r.Y), geom.Pt(0, -1)}
	case SideBottom:
		return anchor{geom.Pt(r.X+offOr(off, r.W), r.Bottom()), geom.Pt(0, 1)}
	case SideLeft:
		return anchor{geom.Pt(r.X, r.Y+offOr(off, r.H)), geom.Pt(-1, 0)}
	default:
		return anchor{geom.Pt(r.Right(), r.Y+offOr(off, r.H)), geom.Pt(1, 0)}
	}
}

func offOr(off, extent float32) float32 {
	if off <= 0 {
		return extent / 2
	}
	return min(off, extent)
}

// facing resolves an automatic side of r towards point to.
func facing(r geom.Rect, s Side, to geom.Point) Side {
	c := r.Center()
	dx, dy := to.X-c.X, to.Y-c.Y
	switch s {
	case SideHorizontal:
		if dx < 0 {
			return SideLeft
		}
		return SideRight
	case SideVertical:
		if dy < 0 {
			return SideTop
		}
		return SideBottom
	case SideAuto:
		// Compare the direction with the box diagonal.
		if math.Abs(float64(dx))*float64(max(r.H, 1)) > math.Abs(float64(dy))*float64(max(r.W, 1)) {
			return facing(r, SideHorizontal, to)
		}
		return facing(r, SideVertical, to)
	}
	return s
}

// boundary returns where the ray from the centre of n towards to leaves
// the node outline.
func boundary(n *Node, to geom.Point) anchor {
	r := n.Rect()
	c := r.Center()
	d := sub(to, c)
	hw, hh := float64(max(r.W/2, 0.5)), float64(max(r.H/2, 0.5))
	dx, dy := float64(d.X), float64(d.Y)
	if dx == 0 && dy == 0 {
		return anchor{c, geom.Point{}}
	}
	var t float64
	switch n.Shape {
	case ShapeEllipse:
		t = 1 / math.Sqrt(dx*dx/(hw*hw)+dy*dy/(hh*hh))
	case ShapeDiamond:
		t = 1 / (math.Abs(dx)/hw + math.Abs(dy)/hh)
	default:
		t = math.Inf(1)
		if dx != 0 {
			t = hw / math.Abs(dx)
		}
		if dy != 0 {
			t = math.Min(t, hh/math.Abs(dy))
		}
	}
	p := geom.Pt(c.X+float32(dx*t), c.Y+float32(dy*t))
	return anchor{p, unit(d)}
}

// endAnchor resolves one end of an edge on node n with optional port,
// facing point to.
func endAnchor(n *Node, portID string, kind EdgeKind, pref Side, to geom.Point) anchor {
	r := n.Rect()
	if p, ok := n.port(portID); ok && p.Side != SideAuto {
		return sideAnchor(r, facing(r, p.Side, to), p.Offset)
	}
	if kind == EdgeStraight {
		return boundary(n, to)
	}
	// Side midpoints lie on the outline of every built-in shape.
	return sideAnchor(r, facing(r, pref, to), 0)
}

// refPoint is the point an end of the edge aims at: the anchor of a fixed
// port, else the node centre.
func refPoint(n *Node, portID string) geom.Point {
	p, ok := n.port(portID)
	if !ok {
		return n.Center()
	}
	r := n.Rect()
	switch p.Side {
	case SideAuto:
		return n.Center()
	case SideHorizontal: // left or right: aim at the port's height
		return geom.Pt(r.Center().X, r.Y+offOr(p.Offset, r.H))
	case SideVertical:
		return geom.Pt(r.X+offOr(p.Offset, r.W), r.Center().Y)
	}
	return sideAnchor(r, p.Side, p.Offset).P
}

// route computes the geometry of edge e between nodes a and b. pref is the
// side family used by ends without a fixed port (SideAuto, SideHorizontal
// or SideVertical: the flow direction of an auto layout).
func route(e *Edge, a, b *Node, kind EdgeKind, pref Side) curve {
	if e.Kind != EdgeDefault {
		kind = e.Kind
	}
	if kind == EdgeDefault {
		kind = EdgeBezier
	}
	if a == b {
		return selfLoop(a, e)
	}
	bends := e.Bends
	toA, toB := refPoint(b, e.ToPort), refPoint(a, e.FromPort)
	if len(bends) > 0 {
		toA, toB = bends[0], bends[len(bends)-1]
	}
	s := endAnchor(a, e.FromPort, kind, pref, toA)
	t := endAnchor(b, e.ToPort, kind, pref, toB)
	switch kind {
	case EdgeStraight:
		cv := curve{Start: s.P}
		for _, p := range bends {
			cv.Segs = append(cv.Segs, seg{P: p, Line: true})
		}
		cv.Segs = append(cv.Segs, seg{P: t.P, Line: true})
		return cv
	case EdgeOrthogonal:
		if len(bends) > 0 {
			return orthogonalVia(s, t, bends)
		}
		return orthogonal(s, t)
	}
	return bezier(s, t, bends)
}

// bezier joins s and t through bends with a smooth (Catmull-Rom) curve
// that leaves and enters along the anchor normals.
func bezier(s, t anchor, bends []geom.Point) curve {
	pts := make([]geom.Point, 0, len(bends)+2)
	pts = append(pts, s.P)
	pts = append(pts, bends...)
	pts = append(pts, t.P)
	m := len(pts) - 1
	tan := make([]geom.Point, len(pts))
	for i := 1; i < m; i++ {
		tan[i] = scale(sub(pts[i+1], pts[i-1]), 0.5)
	}
	first := length(sub(pts[1], pts[0]))
	last := length(sub(pts[m], pts[m-1]))
	off := func(l float32) float32 { return max(24, min(l*0.5, 160)) * 3 }
	if s.N == (geom.Point{}) {
		tan[0] = sub(pts[1], pts[0])
	} else {
		tan[0] = scale(s.N, off(first))
	}
	if t.N == (geom.Point{}) {
		tan[m] = sub(pts[m], pts[m-1])
	} else {
		tan[m] = scale(t.N, -off(last))
	}
	cv := curve{Start: pts[0], Segs: make([]seg, 0, m)}
	for i := 0; i < m; i++ {
		cv.Segs = append(cv.Segs, seg{
			C1: add(pts[i], scale(tan[i], 1.0/3)),
			C2: sub(pts[i+1], scale(tan[i+1], 1.0/3)),
			P:  pts[i+1],
		})
	}
	return cv
}

// orthStub is how far an orthogonal edge runs straight out of a side.
const orthStub = 16

// orthogonal joins s and t with horizontal and vertical segments and
// rounded corners.
func orthogonal(s, t anchor) curve {
	n0, n1 := s.N, t.N
	if n0 == (geom.Point{}) {
		n0 = geom.Pt(1, 0)
	}
	if n1 == (geom.Point{}) {
		n1 = geom.Pt(-1, 0)
	}
	a := add(s.P, scale(n0, orthStub))
	b := add(t.P, scale(n1, orthStub))
	pts := []geom.Point{s.P, a}
	h0, h1 := n0.Y == 0, n1.Y == 0
	switch {
	case h0 && h1:
		if (b.X-a.X)*n0.X >= 0 && (a.X-b.X)*n1.X >= 0 {
			mx := (a.X + b.X) / 2
			pts = append(pts, geom.Pt(mx, a.Y), geom.Pt(mx, b.Y))
		} else {
			my := (a.Y + b.Y) / 2
			pts = append(pts, geom.Pt(a.X, my), geom.Pt(b.X, my))
		}
	case !h0 && !h1:
		if (b.Y-a.Y)*n0.Y >= 0 && (a.Y-b.Y)*n1.Y >= 0 {
			my := (a.Y + b.Y) / 2
			pts = append(pts, geom.Pt(a.X, my), geom.Pt(b.X, my))
		} else {
			mx := (a.X + b.X) / 2
			pts = append(pts, geom.Pt(mx, a.Y), geom.Pt(mx, b.Y))
		}
	case h0:
		if (b.X-a.X)*n0.X >= 0 {
			pts = append(pts, geom.Pt(b.X, a.Y))
		} else {
			pts = append(pts, geom.Pt(a.X, b.Y))
		}
	default:
		if (b.Y-a.Y)*n0.Y >= 0 {
			pts = append(pts, geom.Pt(a.X, b.Y))
		} else {
			pts = append(pts, geom.Pt(b.X, a.Y))
		}
	}
	pts = append(pts, b, t.P)
	return rounded(simplify(pts), 8)
}

// orthogonalVia routes an orthogonal edge through layout bends: between
// consecutive points it runs along the flow, jogs across it halfway, and
// continues (so it uses the channels the layout kept free between nodes).
func orthogonalVia(s, t anchor, bends []geom.Point) curve {
	horiz := s.N.Y == 0 && s.N.X != 0
	if s.N == (geom.Point{}) {
		horiz = true
	}
	pts := []geom.Point{s.P, add(s.P, scale(s.N, orthStub))}
	targets := append(append([]geom.Point(nil), bends...), add(t.P, scale(t.N, orthStub)))
	for _, b := range targets {
		prev := pts[len(pts)-1]
		if horiz {
			mx := (prev.X + b.X) / 2
			pts = append(pts, geom.Pt(mx, prev.Y), geom.Pt(mx, b.Y), b)
		} else {
			my := (prev.Y + b.Y) / 2
			pts = append(pts, geom.Pt(prev.X, my), geom.Pt(b.X, my), b)
		}
	}
	pts = append(pts, t.P)
	return rounded(simplify(pts), 8)
}

// simplify drops repeated and collinear points.
func simplify(pts []geom.Point) []geom.Point {
	out := pts[:0:0]
	for _, p := range pts {
		if n := len(out); n > 0 && near(out[n-1], p) {
			continue
		}
		if n := len(out); n >= 2 && collinear(out[n-2], out[n-1], p) {
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

func near(a, b geom.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.01 && math.Abs(float64(a.Y-b.Y)) < 0.01
}

func collinear(a, b, c geom.Point) bool {
	cr := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	if math.Abs(float64(cr)) > 0.01 {
		return false
	}
	// Same direction only: a doubling back is a real corner.
	return (b.X-a.X)*(c.X-b.X)+(b.Y-a.Y)*(c.Y-b.Y) >= 0
}

// rounded turns a polyline into lines joined by quarter-round corners of
// radius r (smaller where segments are short).
func rounded(pts []geom.Point, r float32) curve {
	cv := curve{Start: pts[0]}
	if len(pts) < 3 {
		for _, p := range pts[1:] {
			cv.Segs = append(cv.Segs, seg{P: p, Line: true})
		}
		return cv
	}
	for i := 1; i < len(pts)-1; i++ {
		p0, p, p1 := pts[i-1], pts[i], pts[i+1]
		l0, l1 := length(sub(p, p0)), length(sub(p1, p))
		rr := min(r, l0/2, l1/2)
		a := sub(p, scale(unit(sub(p, p0)), rr))
		b := add(p, scale(unit(sub(p1, p)), rr))
		cv.Segs = append(cv.Segs, seg{P: a, Line: true})
		// Quadratic corner (control p) as a cubic.
		cv.Segs = append(cv.Segs, seg{C1: lerp(a, p, 2.0/3), C2: lerp(b, p, 2.0/3), P: b})
	}
	cv.Segs = append(cv.Segs, seg{P: pts[len(pts)-1], Line: true})
	return cv
}

// selfLoop routes an edge from a node to itself: out of the right side
// (at the ports' heights, or near the top-right corner) and back.
func selfLoop(n *Node, e *Edge) curve {
	r := n.Rect()
	y0, y1 := r.Y+r.H*0.3, r.Y+r.H*0.7
	if p, ok := n.port(e.FromPort); ok {
		y0 = r.Y + offOr(p.Offset, r.H)
	}
	if p, ok := n.port(e.ToPort); ok {
		y1 = r.Y + offOr(p.Offset, r.H)
	}
	if y0 == y1 {
		y1 += 8
	}
	x := r.Right()
	out := x + 28
	return rounded([]geom.Point{{X: x, Y: y0}, {X: out, Y: y0}, {X: out, Y: y1}, {X: x, Y: y1}}, 8)
}
