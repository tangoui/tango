package chart

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Text alignment of drawText relative to (x, y).
type hAlign uint8
type vAlign uint8

const (
	alignLeft hAlign = iota
	alignCenter
	alignRight
)

const (
	alignTop vAlign = iota
	alignMiddle
	alignBottom
)

func tstyle(size float32, w text.Weight) text.Style {
	if w == 0 {
		w = text.Regular
	}
	return text.Style{Size: size, Weight: w, LineHeight: float32(math.Round(float64(size) * 1.5))}
}

func textWidth(s string, st text.Style) float32 { return text.Width(s, st) }

// drawText draws one line of text aligned at (x, y) and returns its size.
// Numbers and values use it: their paragraph direction comes from the
// text (left to right for digits).
func drawText(l *paint.List, s string, x, y float32, ha hAlign, va vAlign, st text.Style, col theme.Color) geom.Size {
	return drawTextDir(l, s, x, y, ha, va, st, col, text.DirAuto)
}

// drawTextDir is drawText with an explicit paragraph direction (label text
// in RTL scopes). ha is physical: positions are computed by the caller.
func drawTextDir(l *paint.List, s string, x, y float32, ha hAlign, va vAlign, st text.Style, col theme.Color, dir text.Direction) geom.Size {
	if s == "" {
		return geom.Size{}
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true, Direction: dir})
	w, h := lay.Width, lay.Height
	switch ha {
	case alignCenter:
		x -= w / 2
	case alignRight:
		x -= w
	}
	switch va {
	case alignMiddle:
		y -= h / 2
	case alignBottom:
		y -= h
	}
	l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
	return geom.Size{W: w, H: h}
}

// StepMode selects a step line shape.
type StepMode uint8

const (
	StepNone   StepMode = iota
	StepEnd             // horizontal then vertical ("hv")
	StepStart           // vertical then horizontal ("vh")
	StepMiddle          // change halfway between points ("hvh")
)

// tracePath appends pts as a polyline, step line or monotone curve. With
// move the path starts with MoveTo, otherwise the first point is joined
// with LineTo (closing areas).
func tracePath(p *paint.Path, pts []geom.Point, smooth bool, step StepMode, move bool) {
	if len(pts) == 0 {
		return
	}
	if move {
		p.MoveTo(pts[0].X, pts[0].Y)
	} else {
		p.LineTo(pts[0].X, pts[0].Y)
	}
	switch {
	case step != StepNone:
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			switch step {
			case StepEnd:
				p.LineTo(b.X, a.Y)
			case StepStart:
				p.LineTo(a.X, b.Y)
			case StepMiddle:
				m := (a.X + b.X) / 2
				p.LineTo(m, a.Y)
				p.LineTo(m, b.Y)
			}
			p.LineTo(b.X, b.Y)
		}
	case smooth && len(pts) > 2:
		monotone(p, pts)
	default:
		for _, q := range pts[1:] {
			p.LineTo(q.X, q.Y)
		}
	}
}

// monotone appends a monotone cubic Hermite curve through pts (x must be
// non-decreasing). Tangents follow Fritsch & Butland (1984), which keeps
// the curve inside the data range between points: no overshoot above a
// peak or below zero.
func monotone(p *paint.Path, pts []geom.Point) {
	n := len(pts)
	h := make([]float64, n-1)
	d := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		h[i] = float64(pts[i+1].X - pts[i].X)
		if h[i] != 0 {
			d[i] = float64(pts[i+1].Y-pts[i].Y) / h[i]
		}
	}
	m := make([]float64, n)
	m[0], m[n-1] = d[0], d[n-2]
	for i := 1; i < n-1; i++ {
		if d[i-1]*d[i] <= 0 || h[i-1] == 0 || h[i] == 0 {
			m[i] = 0
			continue
		}
		m[i] = 3 * (h[i-1] + h[i]) / ((2*h[i]+h[i-1])/d[i-1] + (h[i]+2*h[i-1])/d[i])
	}
	// One-sided end tangents, limited so the end segments stay monotone.
	m[0] = endSlope(d[0], m[1])
	m[n-1] = endSlope(d[n-2], m[n-2])
	for i := 0; i < n-1; i++ {
		a, b := pts[i], pts[i+1]
		dx := float32(h[i] / 3)
		p.CubeTo(a.X+dx, a.Y+dx*float32(m[i]), b.X-dx, b.Y-dx*float32(m[i+1]), b.X, b.Y)
	}
}

func endSlope(d, mNext float64) float64 {
	s := (3*d - mNext) / 2
	if s*d <= 0 {
		return 0
	}
	if math.Abs(s) > 3*math.Abs(d) {
		return 3 * d
	}
	return s
}

// drawMarker draws a legend / tooltip marker centred at (cx, cy).
func drawMarker(l *paint.List, m Marker, cx, cy float32, col, bg theme.Color) {
	switch m {
	case MarkerRoundRect:
		r := geom.Rect{X: cx - 12.5, Y: cy - 7, W: 25, H: 14}
		l.Fill(r, geom.Rad(3), col)
	case MarkerLine:
		l.Line(geom.Pt(cx-12.5, cy), geom.Pt(cx+12.5, cy), 2, col)
		l.Path(paint.NewPath().Circle(cx, cy, 4.5), col)
		l.Path(paint.NewPath().Circle(cx, cy, 2.5), bg)
	case MarkerSquare:
		l.Fill(geom.Rect{X: cx - 4, Y: cy - 4, W: 8, H: 8}, geom.Rad(1), col)
	default:
		l.Path(paint.NewPath().Circle(cx, cy, 4), col)
	}
}

func markerSize(m Marker) geom.Size {
	switch m {
	case MarkerRoundRect, MarkerLine:
		return geom.Size{W: 25, H: 14}
	}
	return geom.Size{W: 8, H: 8}
}

// dashLine strokes a straight line, dashed when dash is set.
func dashLine(l *paint.List, a, b geom.Point, w float32, col theme.Color, dash []float32) {
	if len(dash) == 0 {
		l.Line(a, b, w, col)
		return
	}
	l.StrokePath(paint.NewPath().MoveTo(a.X, a.Y).LineTo(b.X, b.Y), paint.StrokeStyle{Width: w, Dash: dash}, col)
}

// crisp snaps a coordinate to the pixel centre for 1px lines.
func crisp(v float32) float32 { return float32(math.Floor(float64(v))) + 0.5 }

// textOn picks white or dark text for a label drawn on fill.
func textOn(fill theme.Color) theme.Color {
	if theme.Luma(fill) > 0.72 {
		return theme.RGBA(0, 0, 0, 0.85)
	}
	return theme.Hex("#ffffff")
}

// lerpColor samples a color ramp at t in [0,1].
func lerpColor(ramp []theme.Color, t float64) theme.Color {
	if len(ramp) == 0 {
		return theme.Color{}
	}
	if len(ramp) == 1 || t <= 0 {
		return ramp[0]
	}
	if t >= 1 {
		return ramp[len(ramp)-1]
	}
	f := t * float64(len(ramp)-1)
	i := int(f)
	return theme.Lerp(ramp[i], ramp[i+1], float32(f-float64(i)))
}

// ringSpan is ring for a sector given in either angle order (mirrored
// polar charts sweep counter-clockwise).
func ringSpan(cx, cy, r0, r1 float32, a0, a1 float64) *paint.Path {
	if a1 < a0 {
		a0, a1 = a1, a0
	}
	return ring(cx, cy, r0, r1, a0, a1)
}

// ring returns a closed annular sector path between angles a0 and a1.
func ring(cx, cy, r0, r1 float32, a0, a1 float64) *paint.Path {
	p := paint.NewPath()
	if a1-a0 >= 2*math.Pi-1e-6 {
		p.Circle(cx, cy, r1)
		if r0 > 0 {
			p.Append(paint.NewPath().Circle(cx, cy, r0).Reversed())
		}
		return p
	}
	p.Arc(cx, cy, r1, a0, a1-a0)
	if r0 > 0 {
		p.Arc(cx, cy, r0, a1, a0-a1)
	} else {
		p.LineTo(cx, cy)
	}
	return p.Close()
}
