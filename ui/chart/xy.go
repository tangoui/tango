package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

// xyOpts holds the field mapping shared by cartesian charts.
type xyOpts[T any] struct {
	self       *T
	data       []Row
	xf, yf, sf string
	yMin, yMax *float64
	noZero     bool
}

// X sets the x (category / time / number) field.
func (o *xyOpts[T]) X(field string) *T { o.xf = field; return o.self }

// Y sets the value field.
func (o *xyOpts[T]) Y(field string) *T { o.yf = field; return o.self }

// Series sets the field that splits rows into series (legend, colors).
func (o *xyOpts[T]) Series(field string) *T { o.sf = field; return o.self }

// YMin fixes the lower end of the value axis.
func (o *xyOpts[T]) YMin(v float64) *T { o.yMin = &v; return o.self }

// YMax fixes the upper end of the value axis.
func (o *xyOpts[T]) YMax(v float64) *T { o.yMax = &v; return o.self }

// NoZero lets the value axis start near the data instead of at zero.
func (o *xyOpts[T]) NoZero() *T { o.noZero = true; return o.self }

// valueDomain returns the extent of the visible values.
func valueDomain(lo, hi [][]float64, hidden []bool, zero bool) (float64, float64, bool) {
	a, b := math.Inf(1), math.Inf(-1)
	scan := func(vs [][]float64) {
		for s, v := range vs {
			if isHidden(hidden, s) {
				continue
			}
			for _, x := range v {
				if !math.IsNaN(x) {
					a, b = math.Min(a, x), math.Max(b, x)
				}
			}
		}
	}
	scan(hi)
	scan(lo)
	if math.IsInf(a, 1) {
		return 0, 1, false
	}
	if zero {
		a, b = math.Min(a, 0), math.Max(b, 0)
	}
	return a, b, true
}

// applyMinMax overrides the data extent with fixed bounds.
func applyMinMax(lo, hi float64, mn, mx *float64) (float64, float64) {
	if mn != nil {
		lo = *mn
	}
	if mx != nil {
		hi = *mx
	}
	return lo, hi
}

// buildValueAxis makes the value axis for a domain, honouring fixed bounds.
func buildValueAxis(lo, hi float64, mn, mx *float64, count int, f Formatter) *axis {
	lo, hi = applyMinMax(lo, hi, mn, mx)
	if mn != nil && mx != nil && hi > lo {
		return fixedAxis(lo, hi, count, f)
	}
	a := valueAxis(lo, hi, count, f)
	if mn != nil {
		a.lin.D0 = *mn
	}
	if mx != nil {
		a.lin.D1 = *mx
	}
	if mn != nil || mx != nil {
		a = fixedAxis(a.lin.D0, a.lin.D1, count, f)
	}
	return a
}

// baseAxis builds the x axis of a grid for lines and points.
func baseAxis(g *grid, st *Style, length float32, c *ui.Context, f Formatter) *axis {
	switch g.kind {
	case kindTime:
		lo, hi := g.xs[0], g.xs[len(g.xs)-1]
		a := timeAxis(lo, hi, g.loc, tickCount(length, 90), LabelsOf(c.Locale()))
		a.tip = dataTimeUnit(g.xs)
		return a
	case kindNum:
		lo, hi := g.xs[0], g.xs[len(g.xs)-1]
		a := valueAxis(lo, hi, tickCount(length, 80), f)
		return a
	}
	pad := 0.0
	if st.PointPad {
		pad = 0.5
	}
	return pointAxis(g.cats, pad)
}

// xIndexAt returns the grid index nearest to the pointer on a base axis.
func xIndexAt(g *grid, a *axis, nx float64) int {
	switch a.kind {
	case axPoint:
		return a.point.Index(nx)
	case axBand:
		i := a.band.Index(nx)
		if i < 0 {
			if nx < 0.5 {
				return 0
			}
			return a.n - 1
		}
		return i
	}
	return nearestX(g.xs, a.lin.Invert(nx))
}

func estimatePlot(r geom.Rect) (w, h float32) { return max(1, r.W-60), max(1, r.H-30) }

// dataTimeUnit returns the finest calendar unit the sample times need.
func dataTimeUnit(xs []float64) TimeUnit {
	gap := math.Inf(1)
	for i := 1; i < len(xs); i++ {
		gap = math.Min(gap, xs[i]-xs[i-1])
	}
	switch {
	case gap < 60:
		return Second
	case gap < 3600:
		return Minute
	case gap < 86400:
		return Hour
	case gap >= 365*86400:
		return Year
	case gap >= 28*86400:
		return Month
	}
	return Day
}
