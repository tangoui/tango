package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// DualW builds a dual-axes chart: columns on the left axis, lines on the
// right axis, over a shared category x.
type DualW struct {
	common[DualW]
	data    []Row
	xf      string
	cols    []string
	lines   []string
	alias   map[string]string
	lineFmt Formatter
	smooth  bool
}

// DualAxes starts a column + line chart.
//
//	chart.DualAxes(c, rows).X("month").Column("sales").Line("rate").Show()
func DualAxes(c *ui.Context, data []Row) *DualW {
	w := &DualW{data: data, alias: map[string]string{}}
	w.init(c, w, 300)
	return w
}

// X sets the category field.
func (w *DualW) X(field string) *DualW { w.xf = field; return w }

// Column adds fields drawn as grouped columns on the left axis.
func (w *DualW) Column(fields ...string) *DualW { w.cols = append(w.cols, fields...); return w }

// Line adds fields drawn as lines on the right axis.
func (w *DualW) Line(fields ...string) *DualW { w.lines = append(w.lines, fields...); return w }

// Alias sets the legend / tooltip name of a field.
func (w *DualW) Alias(field, name string) *DualW { w.alias[field] = name; return w }

// LineFormat formats the right axis and line values.
func (w *DualW) LineFormat(f Formatter) *DualW { w.lineFmt = f; return w }

// Smooth draws the lines as monotone curves.
func (w *DualW) Smooth() *DualW { w.smooth = true; return w }

func (w *DualW) name(f string) string {
	if a, ok := w.alias[f]; ok {
		return a
	}
	return f
}

// Show builds the chart.
func (w *DualW) Show() View {
	f := w.begin()
	st := f.st
	f.kind(f.c.Locale().Chart.DualAxes, 0)
	cats := uniq(w.data, w.xf)
	fields := append(append([]string(nil), w.cols...), w.lines...)
	if len(cats) == 0 || len(fields) == 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	nx, nf := len(cats), len(fields)
	ci := map[string]int{}
	for i, c := range cats {
		ci[c] = i
	}
	vals := make([][]float64, nf)
	for k := range vals {
		vals[k] = make([]float64, nx)
		for i := range vals[k] {
			vals[k][i] = math.NaN()
		}
	}
	for _, r := range w.data {
		i := ci[Str(r[w.xf])]
		for k, fl := range fields {
			if v, ok := Num(r[fl]); ok {
				vals[k][i] = v
			}
		}
	}
	names := make([]string, nf)
	for k, fl := range fields {
		names[k] = w.name(fl)
	}
	hidden := f.hiddenMask(names)
	flat := make([]float64, 0, nf*nx)
	for k := range vals {
		for _, v := range vals[k] {
			if hidden[k] {
				v = 0
			}
			flat = append(flat, v)
		}
	}
	cur := f.tweenVals(flat, true)
	nc := len(w.cols)
	_, chi, _ := valueDomain(vals[:nc], nil, hidden[:nc], true)
	llo, lhi, _ := valueDomain(vals[nc:], nil, hidden[nc:], true)
	vf := w.valueFmt()
	lf := w.lineFmt
	if lf == nil {
		lf = vf
	}
	items := make([]legendItem, nf)
	for k := range fields {
		m := st.LegendMarker
		if k >= nc {
			m = MarkerLine
			if st.LegendMarker == MarkerCircle {
				m = MarkerCircle
			}
		}
		items[k] = legendItem{name: names[k], color: st.Color(k), marker: m}
	}
	var cache struct {
		r  geom.Rect
		cc *cart
	}
	layoutFor := func(r geom.Rect) *cart {
		if cache.cc != nil && cache.r == r {
			return cache.cc
		}
		_, ph := estimatePlot(r)
		n := tickCount(ph, 48)
		xa := bandAxis(cats, math.Max(st.BarPadding, 0.3), st.BarOuter)
		ya := valueAxis(0, math.Max(chi, 0), n, w.format)
		// Align the right axis to the same number of ticks.
		y2 := alignedAxis(math.Min(llo, 0), lhi, len(ya.ticks)-1, w.lineFmt)
		cc := f.layoutCart(r, xa, ya, y2, false, st)
		cache.r, cache.cc = r, cc
		return cc
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		cc := layoutFor(r)
		if !cc.Plot.Contains(p) {
			return nil
		}
		i := xIndexAt(nil, cc.x, cc.baseNorm(p))
		if i < 0 {
			return nil
		}
		h := &hover{index: i, series: -1, anchor: p}
		h.tip = Tip{Index: i, Title: cats[i]}
		for k := range fields {
			if hidden[k] || math.IsNaN(vals[k][i]) {
				continue
			}
			fm := vf
			if k >= nc {
				fm = lf
			}
			h.tip.Items = append(h.tip.Items, TipItem{Name: names[k], Value: fm(vals[k][i]), Color: st.Color(k)})
		}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		cc := layoutFor(r)
		b := cc.x.band
		if h != nil {
			x0 := b.Map(h.index) - b.Step()*b.PaddingInner/2
			l.Fill(cc.Rect(x0, x0+b.Step(), 0, 1), geom.Radii{}, st.BandHover)
		}
		cc.drawGrid(l)
		vis := 0
		for k := 0; k < nc; k++ {
			if !hidden[k] {
				vis++
			}
		}
		k2 := 0
		for k := 0; k < nc; k++ {
			if hidden[k] {
				continue
			}
			for i := 0; i < nx; i++ {
				v := cur[k*nx+i]
				if math.IsNaN(vals[k][i]) {
					continue
				}
				sw := b.Bandwidth() / float64(vis)
				x0 := b.Map(i) + float64(k2)*sw
				l.Fill(cc.Rect(x0, x0+sw, cc.y.norm(0), cc.y.norm(v)), geom.Radii{}, st.Color(k))
			}
			k2++
		}
		for k := nc; k < nf; k++ {
			if hidden[k] {
				continue
			}
			var pts []geom.Point
			for i := 0; i < nx; i++ {
				if math.IsNaN(vals[k][i]) {
					continue
				}
				pts = append(pts, cc.At(b.Center(i), cc.y2.norm(cur[k*nx+i])))
			}
			p := paint.NewPath()
			tracePath(p, pts, w.smooth, StepNone, true)
			col := st.Color(k)
			l.StrokePath(p, paint.StrokeStyle{Width: st.LineWidth, Cap: paint.CapRound}, col)
			for i, q := range pts {
				rr := float32(3)
				if h != nil && i == h.index {
					rr = 4.5
				}
				l.Path(paint.NewPath().Circle(q.X, q.Y, rr+1.5), col)
				l.Path(paint.NewPath().Circle(q.X, q.Y, rr), st.Surface)
			}
		}
		cc.drawAxes(l)
	}
	if f.on() {
		lo, hi, np := valueRange(vals)
		f.sem.desc = f.summary(nf, np, cats[0], cats[nx-1], lo, hi, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: f.legendPos(w.legend, LegendAuto, nf), hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}

// alignedAxis builds an axis from lo with exactly n intervals of a round
// step covering hi, so its ticks line up with another axis' grid.
func alignedAxis(lo, hi float64, n int, f Formatter) *axis {
	if n < 1 {
		n = 1
	}
	raw := (hi - lo) / float64(n)
	if raw <= 0 {
		raw = 1
	}
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	step := 10 * p
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*p >= raw*(1-1e-9) {
			step = m * p
			break
		}
	}
	lo = math.Floor(lo/step) * step
	if f == nil {
		d := stepDecimals(step)
		f = func(v float64) string { return FormatNumber(v, d) }
	}
	a := &axis{kind: axLinear, lin: Linear{lo, lo + step*float64(n), 0, 1}, step: step, fmt: f}
	for i := 0; i <= n; i++ {
		v := roundTo(lo+step*float64(i), step)
		a.ticks = append(a.ticks, tick{at: a.lin.Map(v), label: f(v)})
	}
	return a
}
