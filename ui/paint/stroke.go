package paint

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Cap is a stroke end style.
//
// Append-only: never reorder or insert; the zero value stays CapButt.
type Cap uint8

const (
	CapButt Cap = iota
	CapRound
	CapSquare
)

// StrokeStyle configures StrokePath.
type StrokeStyle struct {
	Width float32
	Cap   Cap
	// Round joins are always used; they look right for UI icons.
	Dash []float32 // on/off lengths; nil = solid
}

// StrokeToFill converts a stroked path into a fillable path whose polygons
// all share one orientation, so the non-zero rule unions them.
func StrokeToFill(p *Path, st StrokeStyle) *Path {
	hw := st.Width / 2
	out := &Path{}
	tol := max(0.05, st.Width/8)
	for _, pl := range p.Flatten(tol) {
		pts := dedupe(pl.Pts)
		if pl.Closed && len(pts) > 1 {
			// Snap a last point that already (nearly) returns to the start:
			// a micro segment has a random direction and would stick out.
			if dist(pts[len(pts)-1], pts[0]) < 1e-3 {
				pts = pts[:len(pts)-1]
			}
			if len(pts) > 1 {
				pts = append(pts, pts[0])
			}
		}
		var pieces [][]geom.Point
		if len(st.Dash) > 0 {
			pieces = dashPolyline(pts, st.Dash)
		} else {
			pieces = [][]geom.Point{pts}
		}
		for _, pc := range pieces {
			strokePolyline(out, pc, hw, st.Cap, pl.Closed && len(st.Dash) == 0)
		}
	}
	return out
}

func dedupe(pts []geom.Point) []geom.Point {
	out := pts[:0:0]
	for i, p := range pts {
		if i > 0 && dist(p, out[len(out)-1]) < 1e-4 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func strokePolyline(out *Path, pts []geom.Point, hw float32, cp Cap, closed bool) {
	if len(pts) == 1 {
		if cp == CapRound {
			addCircle(out, pts[0], hw)
		} else if cp == CapSquare {
			addPoly(out, []geom.Point{{X: pts[0].X - hw, Y: pts[0].Y - hw}, {X: pts[0].X + hw, Y: pts[0].Y - hw}, {X: pts[0].X + hw, Y: pts[0].Y + hw}, {X: pts[0].X - hw, Y: pts[0].Y + hw}})
		}
		return
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b.X-a.X, b.Y-a.Y
		l := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if l == 0 {
			continue
		}
		ux, uy := dx/l, dy/l
		nx, ny := -uy*hw, ux*hw
		ea, eb := a, b
		if cp == CapSquare && !closed {
			if i == 0 {
				ea = geom.Pt(a.X-ux*hw, a.Y-uy*hw)
			}
			if i+2 == len(pts) {
				eb = geom.Pt(b.X+ux*hw, b.Y+uy*hw)
			}
		}
		addPoly(out, []geom.Point{{X: ea.X + nx, Y: ea.Y + ny}, {X: eb.X + nx, Y: eb.Y + ny}, {X: eb.X - nx, Y: eb.Y - ny}, {X: ea.X - nx, Y: ea.Y - ny}})
		// Round join at interior vertices that actually turn.
		if i+2 < len(pts) || closed {
			c := pts[(i+2)%len(pts)]
			if i+2 >= len(pts) {
				c = pts[1]
			}
			ex, ey := c.X-b.X, c.Y-b.Y
			cross := dx*ey - dy*ex
			dot := dx*ex + dy*ey
			if math.Abs(float64(cross)) > 1e-3*float64(l) || dot < 0 {
				addJoin(out, b, hw, ux, uy, ex, ey)
			}
		}
	}
	if cp == CapRound && !closed {
		addCircle(out, pts[0], hw)
		addCircle(out, pts[len(pts)-1], hw)
	}
}

// addJoin fills the wedge between two segment normals with a round fan.
func addJoin(out *Path, c geom.Point, hw, ux, uy, ex, ey float32) {
	el := float32(math.Sqrt(float64(ex*ex + ey*ey)))
	if el == 0 {
		return
	}
	ex, ey = ex/el, ey/el
	a0 := math.Atan2(float64(uy), float64(ux))
	a1 := math.Atan2(float64(ey), float64(ex))
	d := a1 - a0
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	// The outer side of the turn is opposite the turn direction.
	var start float64
	if d > 0 {
		start = a0 - math.Pi/2
	} else {
		start = a0 + math.Pi/2
	}
	n := int(math.Ceil(math.Abs(d)/(math.Pi/8))) + 1
	pts := []geom.Point{c}
	for i := 0; i <= n; i++ {
		a := start + d*float64(i)/float64(n)
		pts = append(pts, geom.Pt(c.X+hw*float32(math.Cos(a)), c.Y+hw*float32(math.Sin(a))))
	}
	addPoly(out, pts)
}

func addCircle(out *Path, c geom.Point, r float32) {
	n := max(8, int(r*4))
	pts := make([]geom.Point, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.Pt(c.X+r*float32(math.Cos(a)), c.Y+r*float32(math.Sin(a)))
	}
	addPoly(out, pts)
}

// addPoly appends a closed polygon, normalised to clockwise (y-down) order.
func addPoly(out *Path, pts []geom.Point) {
	var area float32
	for i := range pts {
		j := (i + 1) % len(pts)
		area += pts[i].X*pts[j].Y - pts[j].X*pts[i].Y
	}
	if area < 0 {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	out.MoveTo(pts[0].X, pts[0].Y)
	for _, p := range pts[1:] {
		out.LineTo(p.X, p.Y)
	}
	out.Close()
}

func dashPolyline(pts []geom.Point, dash []float32) [][]geom.Point {
	var out [][]geom.Point
	di := 0
	left := dash[0]
	on := true
	var cur []geom.Point
	if on {
		cur = []geom.Point{pts[0]}
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		seg := dist(a, b)
		pos := float32(0)
		for seg-pos > left {
			pos += left
			t := pos / seg
			p := geom.Pt(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
			if on {
				cur = append(cur, p)
				out = append(out, cur)
				cur = nil
			} else {
				cur = []geom.Point{p}
			}
			on = !on
			di = (di + 1) % len(dash)
			left = dash[di]
		}
		left -= seg - pos
		if on {
			cur = append(cur, b)
		}
	}
	if on && len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}
