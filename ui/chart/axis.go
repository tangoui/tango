package chart

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

type axisKind uint8

const (
	axBand axisKind = iota
	axPoint
	axLinear
	axLog
	axTime
)

type tick struct {
	at    float64 // normalised position
	label string
	major bool
}

// axis is one scale with its ticks, normalised to 0..1.
type axis struct {
	kind  axisKind
	n     int
	cats  []string
	band  Band
	point Point
	lin   Linear
	log   Log
	unit  TimeUnit
	tip   TimeUnit // tooltip precision (from the data spacing)
	loc   *time.Location
	ticks []tick
	fmt   Formatter
	hide  bool // no labels / ticks
	step  float64
}

func bandAxis(cats []string, inner, outer float64) *axis {
	a := &axis{kind: axBand, n: len(cats), cats: cats, band: Band{N: len(cats), R0: 0, R1: 1, PaddingInner: inner, PaddingOuter: outer}}
	for i, c := range cats {
		a.ticks = append(a.ticks, tick{at: a.band.Center(i), label: c})
	}
	return a
}

func pointAxis(cats []string, pad float64) *axis {
	a := &axis{kind: axPoint, n: len(cats), cats: cats, point: Point{N: len(cats), R0: 0, R1: 1, Padding: pad}}
	for i, c := range cats {
		a.ticks = append(a.ticks, tick{at: a.point.Map(i), label: c})
	}
	return a
}

// valueAxis builds a nice linear axis over [lo, hi].
func valueAxis(lo, hi float64, count int, f Formatter) *axis {
	nlo, nhi, step := NiceDomain(lo, hi, count)
	a := &axis{kind: axLinear, lin: Linear{nlo, nhi, 0, 1}, step: step, fmt: f}
	if a.fmt == nil {
		d := stepDecimals(step)
		a.fmt = func(v float64) string { return FormatNumber(v, d) }
	}
	for _, v := range ticksIn(nlo, nhi, step) {
		a.ticks = append(a.ticks, tick{at: a.lin.Map(v), label: a.fmt(v)})
	}
	return a
}

// fixedAxis is a linear axis with an exact domain (percent stacks, gauges).
func fixedAxis(lo, hi float64, count int, f Formatter) *axis {
	step := NiceStep(hi-lo, count)
	a := &axis{kind: axLinear, lin: Linear{lo, hi, 0, 1}, step: step, fmt: f}
	if a.fmt == nil {
		d := stepDecimals(step)
		a.fmt = func(v float64) string { return FormatNumber(v, d) }
	}
	for _, v := range ticksIn(lo, hi, step) {
		a.ticks = append(a.ticks, tick{at: a.lin.Map(v), label: a.fmt(v)})
	}
	return a
}

func logAxis(lo, hi float64, f Formatter) *axis {
	s := Log{D0: math.Max(lo, 1e-12), D1: math.Max(hi, 1e-12), R0: 0, R1: 1}
	s.Nice()
	a := &axis{kind: axLog, log: s, fmt: f}
	if a.fmt == nil {
		a.fmt = func(v float64) string { return FormatCompact(v, "en") }
	}
	for _, v := range s.Ticks() {
		a.ticks = append(a.ticks, tick{at: s.Map(v), label: a.fmt(v)})
	}
	return a
}

// timeAxis spans [lo, hi] (unix seconds) exactly, with calendar ticks.
func timeAxis(lo, hi float64, loc *time.Location, count int, lb Labels) *axis {
	if hi == lo {
		lo, hi = lo-43200, hi+43200
	}
	a := &axis{kind: axTime, lin: Linear{lo, hi, 0, 1}, loc: loc}
	t0, t1 := numTime(lo, loc), numTime(hi, loc)
	a.unit, _ = TimeInterval(t0, t1, count)
	a.tip = a.unit
	for _, tk := range TimeTicks(t0, t1, count, lb) {
		a.ticks = append(a.ticks, tick{at: a.lin.Map(timeNum(tk.T)), label: tk.Label, major: tk.Major})
	}
	return a
}

// norm maps a value (index for band/point axes) to 0..1.
func (a *axis) norm(v float64) float64 {
	switch a.kind {
	case axBand:
		return a.band.Center(int(v))
	case axPoint:
		return a.point.Map(int(v))
	case axLog:
		return a.log.Map(v)
	}
	return a.lin.Map(v)
}

// label formats a value for tooltips.
func (a *axis) label(v float64) string {
	switch a.kind {
	case axBand, axPoint:
		i := int(v)
		if i >= 0 && i < len(a.cats) {
			return a.cats[i]
		}
		return ""
	case axTime:
		return timeFormat(numTime(v, a.loc), a.tip)
	}
	if a.fmt != nil {
		return a.fmt(v)
	}
	return FormatNumber(v, decimalsOf(v))
}

// zero returns the normalised baseline (0 clamped into the domain).
func (a *axis) zero() float64 {
	if a.kind == axLog {
		return 0
	}
	lo, hi := math.Min(a.lin.D0, a.lin.D1), math.Max(a.lin.D0, a.lin.D1)
	return a.lin.Map(math.Max(lo, math.Min(hi, 0)))
}

func (a *axis) maxLabelW(st text.Style) float32 {
	var w float32
	if a == nil || a.hide {
		return 0
	}
	for _, t := range a.ticks {
		w = max(w, textWidth(t.label, st))
	}
	return w
}

// cart is a laid-out cartesian frame: plot rect plus axes.
type cart struct {
	Cartesian
	x, y, y2 *axis
	st       *Style
	ts       text.Style
	lineH    float32
	gridX    bool           // vertical grid lines on the base axis (scatter)
	para     text.Direction // paragraph direction of category / time labels
}

// layoutCart reserves room for tick labels around the plot. With f.mirror
// the reserved margins swap sides (value axis on the right, a second value
// axis on the left), so the plot rect is the mirror image of the LTR one.
func (f *frame) layoutCart(r geom.Rect, x, y, y2 *axis, transposed bool, st *Style) *cart {
	c := layoutCart(r, x, y, y2, transposed, st)
	c.para = f.para
	if f.mirror {
		c.Mirror = true
		c.Plot.X = 2*r.X + r.W - c.Plot.Right()
	}
	return c
}

// layoutCart lays out a left-to-right cartesian frame in r.
func layoutCart(r geom.Rect, x, y, y2 *axis, transposed bool, st *Style) *cart {
	c := &cart{x: x, y: y, y2: y2, st: st, ts: tstyle(st.Font, 0)}
	c.Transposed = transposed
	c.lineH = text.LineHeightOf(c.ts)
	gap := st.LabelGap + st.TickLen
	var left, right, top, bottom float32
	top = c.lineH / 2
	if transposed {
		left = x.maxLabelW(c.ts) + gap
		bottom = c.lineH + st.LabelGap + st.TickLen
		// Last value label centred on the right edge.
		if n := len(y.ticks); n > 0 && !y.hide {
			right = textWidth(y.ticks[n-1].label, c.ts)/2 + 2
		}
	} else {
		left = y.maxLabelW(c.ts) + st.LabelGap + 4
		if y.hide {
			left = 0
		}
		if !x.hide {
			bottom = c.lineH + gap
		}
		right = 4
		if y2 != nil && !y2.hide {
			right = y2.maxLabelW(c.ts) + st.LabelGap + 4
		} else if n := len(x.ticks); n > 0 && !x.hide && x.kind != axBand {
			last := x.ticks[n-1]
			if over := textWidth(last.label, c.ts)/2 - float32(1-last.at)*r.W; over > right {
				right = over + 2
			}
		}
		if n := len(x.ticks); n > 0 && !x.hide && x.kind != axBand {
			first := x.ticks[0]
			if over := textWidth(first.label, c.ts)/2 - float32(first.at)*r.W; over > left {
				left = over + 2
			}
		}
	}
	c.Plot = geom.Rect{X: r.X + left, Y: r.Y + top, W: max(1, r.W-left-right), H: max(1, r.H-top-bottom)}
	return c
}

// visibleTicks picks every k-th label so neighbours do not overlap.
func visibleTicks(ts []tick, length float32, sty text.Style, horizontal bool, lineH float32) []bool {
	keep := make([]bool, len(ts))
	if len(ts) == 0 {
		return keep
	}
	size := func(t tick) float32 {
		if horizontal {
			return textWidth(t.label, sty)
		}
		return lineH
	}
	minGap := float32(12)
	if !horizontal {
		minGap = 2
	}
	for k := 1; k <= len(ts); k++ {
		ok := true
		prevEnd := float32(math.Inf(-1))
		for i := 0; i < len(ts); i += k {
			p := float32(ts[i].at) * length
			s := size(ts[i])
			if p-s/2 < prevEnd+minGap {
				ok = false
				break
			}
			prevEnd = p + s/2
		}
		if ok {
			for i := range keep {
				keep[i] = i%k == 0
			}
			return keep
		}
	}
	keep[0] = true
	return keep
}

// labelDir is the paragraph direction of an axis' labels: numbers keep
// their own (left-to-right) direction, category and time labels follow the
// scope.
func (c *cart) labelDir(a *axis) text.Direction {
	switch a.kind {
	case axLinear, axLog:
		return text.DirAuto
	}
	return c.para
}

// drawGrid draws value grid lines (behind the marks).
func (c *cart) drawGrid(l *paint.List) {
	st := c.st
	p := c.Plot
	for _, t := range c.y.ticks {
		if c.Transposed {
			x := crisp(c.HX(t.at))
			dashLine(l, geom.Pt(x, p.Y), geom.Pt(x, p.Bottom()), 1, st.Grid, st.GridDash)
		} else {
			y := crisp(p.Bottom() - float32(t.at)*p.H)
			dashLine(l, geom.Pt(p.X, y), geom.Pt(p.Right(), y), 1, st.Grid, st.GridDash)
		}
	}
	if c.gridX {
		for _, t := range c.x.ticks {
			x := crisp(c.HX(t.at))
			dashLine(l, geom.Pt(x, p.Y), geom.Pt(x, p.Bottom()), 1, st.Grid, st.GridDash)
		}
	}
}

// drawAxes draws axis lines, ticks and labels (above the grid).
func (c *cart) drawAxes(l *paint.List) {
	st := c.st
	p := c.Plot
	ts := c.ts
	if c.Transposed {
		// Category axis on the left (right when mirrored), top to bottom.
		if !c.x.hide {
			x, out, ha := crisp(p.X), -st.TickLen, alignRight
			lx := p.X - st.TickLen - st.LabelGap
			if c.Mirror {
				x, out, ha = crisp(p.Right()), st.TickLen, alignLeft
				lx = p.Right() + st.TickLen + st.LabelGap
			}
			if st.XAxisLine {
				l.Line(geom.Pt(x, p.Y), geom.Pt(x, p.Bottom()), 1, st.AxisLine)
			}
			keep := visibleTicks(c.x.ticks, p.H, ts, false, c.lineH)
			dir := c.labelDir(c.x)
			for i, t := range c.x.ticks {
				if !keep[i] {
					continue
				}
				y := p.Y + float32(t.at)*p.H
				if st.TickLen > 0 {
					l.Line(geom.Pt(x+out, crisp(y)), geom.Pt(x, crisp(y)), 1, st.Tick)
				}
				drawTextDir(l, t.label, lx, y, ha, alignMiddle, ts, st.Label, dir)
			}
		}
		if !c.y.hide {
			keep := visibleTicks(c.y.ticks, p.W, ts, true, c.lineH)
			for i, t := range c.y.ticks {
				if keep[i] {
					drawText(l, t.label, c.HX(t.at), p.Bottom()+st.LabelGap+2, alignCenter, alignTop, ts, st.Label)
				}
			}
		}
		return
	}
	// Base axis along the bottom.
	if !c.x.hide {
		y := crisp(p.Bottom())
		if st.XAxisLine {
			l.Line(geom.Pt(p.X, y), geom.Pt(p.Right(), y), 1, st.AxisLine)
		}
		keep := visibleTicks(c.x.ticks, p.W, ts, true, c.lineH)
		if st.TickLen > 0 {
			if c.x.kind == axBand && st.XAxisLine {
				// ECharts: ticks on band boundaries.
				for i := 0; i <= c.x.n; i++ {
					bx := crisp(c.HX(c.x.band.Map(i) - c.x.band.Step()*c.x.band.PaddingInner/2))
					if i == c.x.n {
						bx = crisp(c.HX(c.x.band.Map(i-1) + c.x.band.Bandwidth() + c.x.band.Step()*c.x.band.PaddingInner/2))
					}
					l.Line(geom.Pt(bx, y), geom.Pt(bx, y+st.TickLen), 1, st.Tick)
				}
			} else {
				for i, t := range c.x.ticks {
					if keep[i] {
						x := crisp(c.HX(t.at))
						l.Line(geom.Pt(x, y), geom.Pt(x, y+st.TickLen), 1, st.Tick)
					}
				}
			}
		}
		dir := c.labelDir(c.x)
		for i, t := range c.x.ticks {
			if !keep[i] {
				continue
			}
			s := ts
			if t.major && c.x.kind == axTime && st.Name == "echarts" {
				s = tstyle(st.Font, text.Semibold)
			}
			drawTextDir(l, t.label, c.HX(t.at), p.Bottom()+st.TickLen+st.LabelGap, alignCenter, alignTop, s, st.Label, dir)
		}
	}
	// Value axis on the left and the second one on the right; swapped when
	// mirrored.
	yx, yl, yha := p.X, p.X-st.LabelGap-4, alignRight
	y2l, y2ha := p.Right()+st.LabelGap+4, alignLeft
	if c.Mirror {
		yx, yl, yha = p.Right(), p.Right()+st.LabelGap+4, alignLeft
		y2l, y2ha = p.X-st.LabelGap-4, alignRight
	}
	if !c.y.hide {
		keep := visibleTicks(c.y.ticks, p.H, ts, false, c.lineH)
		if st.YAxisLine {
			l.Line(geom.Pt(crisp(yx), p.Y), geom.Pt(crisp(yx), p.Bottom()), 1, st.AxisLine)
		}
		dir := c.labelDir(c.y)
		for i, t := range c.y.ticks {
			if keep[i] {
				drawTextDir(l, t.label, yl, p.Bottom()-float32(t.at)*p.H, yha, alignMiddle, ts, st.Label, dir)
			}
		}
	}
	if c.y2 != nil && !c.y2.hide {
		keep := visibleTicks(c.y2.ticks, p.H, ts, false, c.lineH)
		for i, t := range c.y2.ticks {
			if keep[i] {
				drawText(l, t.label, y2l, p.Bottom()-float32(t.at)*p.H, y2ha, alignMiddle, ts, st.Label)
			}
		}
	}
}

// baseIndex returns the category / x index nearest to the pointer.
func (c *cart) baseNorm(pt geom.Point) float64 {
	nx, _ := c.Norm(pt)
	return nx
}

// tickCount returns a tick count suited to a pixel length.
func tickCount(length, per float32) int {
	return max(2, min(10, int(length/per)))
}
