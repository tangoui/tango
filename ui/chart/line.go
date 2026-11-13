package chart

import (
	"fmt"
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// LineW builds a line or area chart.
type LineW struct {
	common[LineW]
	xyOpts[LineW]
	smooth bool
	step   StepMode
	area   bool
	stack  bool
	points bool
	logY   bool
}

// Line starts a line chart over rows.
//
//	chart.Line(c, rows).X("month").Y("value").Series("city").Smooth().Show()
func Line(c *ui.Context, data []Row) *LineW {
	w := &LineW{}
	w.init(c, w, 300)
	w.xyOpts.self, w.data = w, data
	return w
}

// Area starts an area chart (a line chart with the area filled).
func Area(c *ui.Context, data []Row) *LineW { return Line(c, data).Area() }

// Smooth draws monotone curves instead of straight segments.
func (w *LineW) Smooth() *LineW { w.smooth = true; return w }

// Step draws a step line (default StepEnd: horizontal, then vertical).
func (w *LineW) Step(mode ...StepMode) *LineW {
	w.step = StepEnd
	if len(mode) > 0 {
		w.step = mode[0]
	}
	return w
}

// Area fills the area below each line.
func (w *LineW) Area() *LineW { w.area = true; return w }

// Stack stacks the series (stacked area when combined with Area).
func (w *LineW) Stack() *LineW { w.stack = true; return w }

// Points draws a symbol at every data point.
func (w *LineW) Points() *LineW { w.points = true; return w }

// Log uses a logarithmic value axis.
func (w *LineW) Log() *LineW { w.logY = true; return w }

// Show builds the chart.
func (w *LineW) Show() View {
	f := w.begin()
	st := f.st
	if w.area {
		f.kind(f.c.Locale().Chart.Area, 0)
	} else {
		f.kind(f.c.Locale().Chart.Line, 0)
	}
	g := f.gridFor(w.data, w.xf, w.yf, w.sf, false)
	if g.empty() {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	hidden := f.hiddenMask(g.names)
	ns, nx := len(g.names), len(g.xs)

	// Values (stacked bounds when stacking), eased on data changes.
	var lo, hi [][]float64
	if w.stack {
		lo, hi = Stack(g.vals, hidden, false)
		for s := range hi { // keep gaps where a series has no value
			for i, v := range g.vals[s] {
				if math.IsNaN(v) {
					hi[s][i], lo[s][i] = math.NaN(), math.NaN()
				}
			}
		}
	} else {
		hi = g.vals
	}
	flat := make([]float64, 0, ns*nx*2)
	for s := range hi {
		v := hi[s]
		if hidden[s] && !w.stack {
			v = make([]float64, nx)
			for i := range v {
				v[i] = math.NaN()
			}
		}
		flat = append(flat, v...)
	}
	if w.stack {
		for s := range lo {
			flat = append(flat, lo[s]...)
		}
	}
	cur := f.tweenVals(flat, false)
	ahi := make([][]float64, ns)
	var alo [][]float64
	for s := 0; s < ns; s++ {
		ahi[s] = cur[s*nx : (s+1)*nx]
	}
	if w.stack {
		alo = make([][]float64, ns)
		for s := 0; s < ns; s++ {
			alo[s] = cur[(ns+s)*nx : (ns+s+1)*nx]
		}
	}

	vf := w.valueFmt()
	zero := !w.noZero && !w.logY
	dlo, dhi, _ := valueDomain(hi, lo, hidden, zero || w.area)
	items := make([]legendItem, ns)
	marker := st.LegendMarker
	if marker == MarkerRoundRect {
		marker = MarkerLine
	}
	for s, n := range g.names {
		items[s] = legendItem{name: n, color: st.Color(s), marker: marker}
	}

	type geomCache struct {
		r      geom.Rect
		cart   *cart
		xa, ya *axis
	}
	var gc geomCache
	layoutFor := func(r geom.Rect) *cart {
		if gc.cart != nil && gc.r == r {
			return gc.cart
		}
		pw, ph := estimatePlot(r)
		xa := baseAxis(g, st, pw, f.c, nil)
		var ya *axis
		if w.logY {
			ya = logAxis(dlo, dhi, w.format)
		} else {
			ya = buildValueAxis(dlo, dhi, w.yMin, w.yMax, tickCount(ph, 48), w.format)
		}
		gc = geomCache{r: r, cart: f.layoutCart(r, xa, ya, nil, false, st), xa: xa, ya: ya}
		return gc.cart
	}
	xAt := func(cc *cart, i int) float64 {
		if g.kind == kindCat {
			return cc.x.norm(float64(i))
		}
		return cc.x.norm(g.xs[i])
	}

	hit := func(r geom.Rect, p geom.Point) *hover {
		cc := layoutFor(r)
		if !cc.Plot.Expand(4).Contains(p) {
			return nil
		}
		i := xIndexAt(g, cc.x, cc.baseNorm(p))
		if i < 0 {
			return nil
		}
		h := &hover{index: i, series: -1, anchor: p}
		h.tip.Index = i
		h.tip.Title = cc.x.label(g.xs[i])
		for s, n := range g.names {
			if hidden[s] || math.IsNaN(g.vals[s][i]) {
				continue
			}
			name := n
			if name == "" {
				name = w.yf
			}
			h.tip.Items = append(h.tip.Items, TipItem{Name: name, Value: vf(g.vals[s][i]), Color: st.Color(s)})
		}
		return h
	}

	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		cc := layoutFor(r)
		pl := cc.Plot
		cc.drawGrid(l)
		thr := int(pl.W * 2)
		// Entry: reveal from the start of the x axis.
		clip := pl.Expand(6)
		clip.Y, clip.H = r.Y, r.H
		if f.mirror {
			clip.X += clip.W * (1 - f.enter)
		}
		clip.W = clip.W * f.enter
		l.Clip(clip, geom.Radii{})
		zeroY := cc.y.zero()
		for s := 0; s < ns; s++ {
			if hidden[s] {
				continue
			}
			col := st.Color(s)
			idx := f.downsample(s, g.xs, ahi[s], thr)
			pts := make([]geom.Point, 0, len(idx))
			for _, i := range idx {
				v := ahi[s][i]
				if math.IsNaN(v) {
					continue
				}
				pts = append(pts, cc.At(xAt(cc, i), cc.y.norm(v)))
			}
			if len(pts) == 0 {
				continue
			}
			if w.area {
				ap := paint.NewPath()
				tracePath(ap, pts, w.smooth, w.step, true)
				if w.stack && alo != nil {
					base := make([]geom.Point, 0, len(idx))
					for k := len(idx) - 1; k >= 0; k-- {
						i := idx[k]
						if math.IsNaN(alo[s][i]) {
							continue
						}
						base = append(base, cc.At(xAt(cc, i), cc.y.norm(alo[s][i])))
					}
					tracePath(ap, base, w.smooth, reverseStep(w.step), false)
				} else {
					by := cc.At(0, zeroY).Y
					ap.LineTo(pts[len(pts)-1].X, by).LineTo(pts[0].X, by)
				}
				ap.Close()
				if st.AreaGrad && !w.stack {
					l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Path: ap,
						Start: geom.Pt(0, pl.Y), End: geom.Pt(0, pl.Bottom()),
						Stops: []paint.Stop{{Offset: 0, Color: theme.Fade(col, 0.5)}, {Offset: 1, Color: theme.Fade(col, 0.02)}}})
				} else {
					a := st.AreaOpacity
					if w.stack && st.AreaGrad {
						a = 0.85
					}
					l.Path(ap, theme.Fade(col, a))
				}
			}
			lp := paint.NewPath()
			tracePath(lp, pts, w.smooth, w.step, true)
			lw := st.LineWidth
			if w.area && w.stack && st.AreaGrad {
				lw = 0 // G2 stacked areas have no outline
			}
			if lw > 0 {
				l.StrokePath(lp, paint.StrokeStyle{Width: lw, Cap: paint.CapRound}, col)
			}
			if (w.points || st.ShowPoints) && len(pts) <= int(pl.W/6) {
				for _, p := range pts {
					l.Path(paint.NewPath().Circle(p.X, p.Y, st.PointRadius+1), col)
					l.Path(paint.NewPath().Circle(p.X, p.Y, st.PointRadius-0.5), st.Surface)
				}
			}
			if w.labels && len(pts) <= int(pl.W/28) {
				ts := tstyle(st.Font, 0)
				k := 0
				for _, i := range idx {
					if math.IsNaN(ahi[s][i]) {
						continue
					}
					p := pts[k]
					k++
					drawText(l, vf(g.vals[s][i]), p.X, p.Y-6, alignCenter, alignBottom, ts, st.Label)
				}
			}
		}
		l.Pop()
		if h != nil {
			x := cc.At(xAt(cc, h.index), 0).X
			dashLine(l, geom.Pt(crisp(x), pl.Y), geom.Pt(crisp(x), pl.Bottom()), 1, st.Crosshair, st.CrosshairDash)
			for s := 0; s < ns; s++ {
				v := ahi[s][h.index]
				if hidden[s] || math.IsNaN(v) {
					continue
				}
				p := cc.At(xAt(cc, h.index), cc.y.norm(v))
				l.Path(paint.NewPath().Circle(p.X, p.Y, 5), st.Surface)
				l.Path(paint.NewPath().Circle(p.X, p.Y, 3.5), st.Color(s))
			}
		}
		cc.drawAxes(l)
	}

	if f.on() {
		f.sem.desc = f.gridSummary(g, vf)
	}
	return f.finish(plotSpec{
		items:     items,
		legendPos: f.legendPos(w.legend, LegendAuto, ns),
		hit:       hit,
		paint:     paintFn,
		tooltip:   !w.noTooltip,
		loading:   w.loading,
	})
}

func reverseStep(s StepMode) StepMode {
	switch s {
	case StepEnd:
		return StepStart
	case StepStart:
		return StepEnd
	}
	return s
}

// downsample returns the indices to draw for series s: all of them, or an
// LTTB selection of at most thr points, cached while the values are stable.
func (f *frame) downsample(s int, xs, ys []float64, thr int) []int {
	n := len(ys)
	if thr < 3 || n <= thr {
		idx := make([]int, 0, n)
		for i := range ys {
			idx = append(idx, i)
		}
		return idx
	}
	ds := &f.cs.ds
	key := f.cs.tw.sig ^ uint64(thr)*0x9E3779B97F4A7C15
	if !f.c.AnimDone(f.id, fmt.Sprint("tw", f.cs.tw.gen)) || len(f.cs.tw.cur) == 0 {
		key = 0
	}
	if key != 0 && ds.key == key && s < len(ds.idx) && ds.idx[s] != nil {
		return ds.idx[s]
	}
	idx := LTTB(xs, ys, thr)
	if key != 0 {
		if ds.key != key {
			ds.key, ds.idx = key, nil
		}
		for len(ds.idx) <= s {
			ds.idx = append(ds.idx, nil)
		}
		ds.idx[s] = idx
	}
	return idx
}
