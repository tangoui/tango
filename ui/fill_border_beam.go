package ui

// BorderBeam (Ant Design 6.4): a decorative light beam that runs along the
// border of its content. The beam is a short stroke following the rounded
// rectangle, fading from a transparent tail to the head; several beams
// can be spaced evenly. It stands still under NoAnimations (drawn at a
// fixed spot, no frames requested) and is hidden under ReduceMotion, like
// Ant's prefers-reduced-motion rule.

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// BeamStop is a color stop of a BorderBeam gradient (Percent 0..100 from
// tail to head).
type BeamStop struct {
	Color   theme.Color
	Percent float32
}

// BorderBeamW decorates its body with moving border beams.
type BorderBeamW struct {
	c      *Context
	stops  []BeamStop
	dur    time.Duration
	width  float32
	size   float32
	count  int
	outset float32
	radius float32
	radSet bool
	active bool
	opts   []layout.Option
}

// BorderBeam starts a beam decoration; add the content with Body.
//
//	ui.BorderBeam(c).Duration(3 * time.Second).Body(func() { ui.Card(c, "AI").Body(...) })
func BorderBeam(c *Context) *BorderBeamW {
	return &BorderBeamW{c: c, dur: 6 * time.Second, width: 1, size: 100, count: 1, active: true}
}

// Color sets a single beam color (fading to transparent at the tail).
func (w *BorderBeamW) Color(col theme.Color) *BorderBeamW {
	w.stops = []BeamStop{{col, 0}, {col, 100}}
	return w
}

// Gradient sets the beam colors from tail (0) to head (100).
func (w *BorderBeamW) Gradient(stops ...BeamStop) *BorderBeamW { w.stops = stops; return w }

// Duration sets the time of one loop (default 6s).
func (w *BorderBeamW) Duration(d time.Duration) *BorderBeamW {
	if d > 0 {
		w.dur = d
	}
	return w
}

// LineWidth sets the beam thickness (default 1px).
func (w *BorderBeamW) LineWidth(px float32) *BorderBeamW { w.width = max(0.5, px); return w }

// Size sets the beam length (default 100px).
func (w *BorderBeamW) Size(px float32) *BorderBeamW { w.size = max(4, px); return w }

// Count spaces n beams evenly (default 1).
func (w *BorderBeamW) Count(n int) *BorderBeamW { w.count = max(1, n); return w }

// Outset moves the beam outside (positive) or inside (negative) the edge.
func (w *BorderBeamW) Outset(px float32) *BorderBeamW { w.outset = px; return w }

// Radius sets the corner radius the beam follows (default th.RadiusLG,
// the Card radius).
func (w *BorderBeamW) Radius(px float32) *BorderBeamW { w.radius, w.radSet = px, true; return w }

// Active shows or hides the beam (e.g. only while hovered).
func (w *BorderBeamW) Active(on bool) *BorderBeamW { w.active = on; return w }

// Layout applies layout options to the wrapper.
func (w *BorderBeamW) Layout(o ...layout.Option) *BorderBeamW {
	w.opts = append(w.opts, o...)
	return w
}

// Body builds the decorated content and returns the wrapper box.
func (w *BorderBeamW) Body(fn func()) *Box {
	c, th := w.c, w.c.Theme
	b := c.box("fill-beam", layout.Column, w.opts)
	b.Body(fn)
	if !w.active || c.reduceMotion {
		return b
	}
	phase := 0.125 // still frame: the beam rests near the top-left corner
	if !c.noAnim {
		period := w.dur.Nanoseconds()
		phase = float64(c.now.UnixNano()%period) / float64(period)
		c.animating = true
	}
	stops := w.stops
	if len(stops) == 0 {
		stops = []BeamStop{{th.Primary.Hover, 0}, {th.Primary.Base, 100}}
	}
	rad := th.RadiusLG
	if w.radSet {
		rad = w.radius
	}
	width, size, count, outset := w.width, w.size, w.count, w.outset
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		pr := r.Expand(outset).Inset(geom.All(width / 2))
		rr := max(0, min(rad+outset-width/2, min(pr.W, pr.H)/2))
		total := fillPerimeter(pr, rr)
		if total <= 0 {
			return
		}
		seg := min(float64(size), total/float64(count))
		for k := 0; k < count; k++ {
			head := math.Mod(phase*total+float64(k)*total/float64(count), total)
			fillDrawBeam(l, pr, rr, head, seg, width, stops)
		}
	})
	return b
}

// fillPerimeter is the length of the rounded rectangle outline.
func fillPerimeter(r geom.Rect, rad float32) float64 {
	return 2*float64(r.W+r.H) - 8*float64(rad) + 2*math.Pi*float64(rad)
}

// fillPerimPoint returns the point at arc length s (clockwise from the end
// of the top-left corner) on the outline of r with corner radius rad.
func fillPerimPoint(r geom.Rect, rad float32, s float64) geom.Point {
	L := fillPerimeter(r, rad)
	s = math.Mod(s, L)
	if s < 0 {
		s += L
	}
	R := float64(rad)
	arc := math.Pi / 2 * R
	edges := [4]float64{float64(r.W) - 2*R, float64(r.H) - 2*R, float64(r.W) - 2*R, float64(r.H) - 2*R}
	x0, y0 := float64(r.X), float64(r.Y)
	x1, y1 := float64(r.Right()), float64(r.Bottom())
	// Corner centres in clockwise order after each edge: TR, BR, BL, TL.
	centres := [4][2]float64{{x1 - R, y0 + R}, {x1 - R, y1 - R}, {x0 + R, y1 - R}, {x0 + R, y0 + R}}
	starts := [4]float64{-math.Pi / 2, 0, math.Pi / 2, math.Pi}
	for side := 0; side < 4; side++ {
		if s <= edges[side] {
			t := s
			switch side {
			case 0:
				return geom.Pt(float32(x0+R+t), float32(y0))
			case 1:
				return geom.Pt(float32(x1), float32(y0+R+t))
			case 2:
				return geom.Pt(float32(x1-R-t), float32(y1))
			default:
				return geom.Pt(float32(x0), float32(y1-R-t))
			}
		}
		s -= edges[side]
		if s <= arc {
			a := starts[side] + s/max(R, 1e-6)
			cx, cy := centres[side][0], centres[side][1]
			return geom.Pt(float32(cx+R*math.Cos(a)), float32(cy+R*math.Sin(a)))
		}
		s -= arc
	}
	return geom.Pt(r.X+rad, r.Y)
}

// fillBeamColor samples the gradient at f (0 tail .. 1 head) with the
// transparent tail reserved by Ant.
func fillBeamColor(stops []BeamStop, f float32) theme.Color {
	col := stops[0].Color
	p := f * 100
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if p <= b.Percent {
			t := float32(0)
			if b.Percent > a.Percent {
				t = (p - a.Percent) / (b.Percent - a.Percent)
			}
			col = theme.Lerp(a.Color, b.Color, max(0, min(1, t)))
			break
		}
		col = b.Color
	}
	fade := f * f * (3 - 2*f) // smoothstep: transparent tail, solid head
	col.A = uint8(float32(col.A) * fade)
	return col
}

// fillDrawBeam strokes one beam ending at arc length head.
func fillDrawBeam(l *paint.List, r geom.Rect, rad float32, head, length float64, width float32, stops []BeamStop) {
	const n = 28
	step := length / n
	st := paint.StrokeStyle{Width: width, Cap: paint.CapButt}
	for j := 0; j < n; j++ {
		s0 := head - length + float64(j)*step
		col := fillBeamColor(stops, (float32(j)+0.5)/n)
		if col.A == 0 {
			continue
		}
		p := paint.NewPath()
		for k := 0; k <= 2; k++ {
			pt := fillPerimPoint(r, rad, s0+step*float64(k)/2)
			if k == 0 {
				p.MoveTo(pt.X, pt.Y)
			} else {
				p.LineTo(pt.X, pt.Y)
			}
		}
		l.StrokePath(p, st, col)
	}
}
