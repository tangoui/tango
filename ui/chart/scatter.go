package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ScatterW builds a scatter or bubble chart.
type ScatterW struct {
	common[ScatterW]
	xyOpts[ScatterW]
	sizeF      string
	rMin, rMax float32
	opacity    float64
	xFmt       Formatter
	logX       bool
}

// Scatter starts a scatter chart; X and Y are numeric fields.
//
//	chart.Scatter(c, rows).X("height").Y("weight").Series("gender").Show()
func Scatter(c *ui.Context, data []Row) *ScatterW {
	w := &ScatterW{rMin: 4, rMax: 24, opacity: 0.65}
	w.init(c, w, 300)
	w.xyOpts.self, w.data = w, data
	w.noZero = true
	return w
}

// Bubble starts a bubble chart: Size maps a field to the bubble area.
func Bubble(c *ui.Context, data []Row) *ScatterW { return Scatter(c, data) }

// Size maps a numeric field to the point area (bubbles).
func (w *ScatterW) Size(field string) *ScatterW { w.sizeF = field; return w }

// Radius sets the bubble radius range (default 4..24, points 4).
func (w *ScatterW) Radius(min, max float32) *ScatterW { w.rMin, w.rMax = min, max; return w }

// XFormat formats x tick labels and tooltip values.
func (w *ScatterW) XFormat(f Formatter) *ScatterW { w.xFmt = f; return w }

// Zero includes zero in both axes.
func (w *ScatterW) Zero() *ScatterW { w.noZero = false; return w }

type scPoint struct {
	x, y, size float64
	s          int
	row        int
}

// Show builds the chart.
func (w *ScatterW) Show() View {
	f := w.begin()
	st := f.st
	f.kind(f.c.Locale().Chart.Scatter, 0)
	names := uniq(w.data, w.sf)
	if w.sf == "" {
		names = []string{""}
	}
	sIdx := map[string]int{}
	for i, n := range names {
		sIdx[n] = i
	}
	var pts []scPoint
	xlo, xhi, ylo, yhi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	slo, shi := math.Inf(1), math.Inf(-1)
	hidden := f.hiddenMask(names)
	for i, r := range w.data {
		x, ok1 := Num(r[w.xf])
		y, ok2 := Num(r[w.yf])
		if !ok1 || !ok2 {
			continue
		}
		p := scPoint{x: x, y: y, row: i}
		if w.sf != "" {
			p.s = sIdx[Str(r[w.sf])]
		}
		if w.sizeF != "" {
			p.size, _ = Num(r[w.sizeF])
			slo, shi = math.Min(slo, p.size), math.Max(shi, p.size)
		}
		pts = append(pts, p)
		if hidden[p.s] {
			continue
		}
		xlo, xhi = math.Min(xlo, x), math.Max(xhi, x)
		ylo, yhi = math.Min(ylo, y), math.Max(yhi, y)
	}
	if len(pts) == 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	if math.IsInf(xlo, 1) {
		xlo, xhi, ylo, yhi = 0, 1, 0, 1
	}
	if !w.noZero {
		xlo, ylo = math.Min(xlo, 0), math.Min(ylo, 0)
	}
	if w.sizeF != "" {
		// Leave room for the largest bubbles at the edges.
		dx, dy := (xhi-xlo)*0.08, (yhi-ylo)*0.08
		xlo, xhi, ylo, yhi = xlo-dx, xhi+dx, ylo-dy, yhi+dy
		if w.noZero && xlo < 0 && xhi-dx >= 0 && xlo+dx >= 0 {
			xlo = 0
		}
	}
	vf := w.valueFmt()
	xf := w.xFmt
	if xf == nil {
		xf = vf
	}
	radius := func(p scPoint) float32 {
		if w.sizeF == "" {
			return w.rMin
		}
		if shi <= slo {
			return (w.rMin + w.rMax) / 2
		}
		// Area proportional to the value.
		t := math.Sqrt((p.size - slo) / (shi - slo))
		return w.rMin + float32(t)*(w.rMax-w.rMin)
	}
	grow := f.tweenVals([]float64{1}, true)[0]
	items := make([]legendItem, len(names))
	for i, n := range names {
		items[i] = legendItem{name: n, color: st.Color(i), marker: MarkerCircle}
	}
	var cache struct {
		r  geom.Rect
		cc *cart
	}
	layoutFor := func(r geom.Rect) *cart {
		if cache.cc != nil && cache.r == r {
			return cache.cc
		}
		pw, ph := estimatePlot(r)
		xa := valueAxis(xlo, xhi, tickCount(pw, 90), w.xFmt)
		ya := buildValueAxis(ylo, yhi, w.yMin, w.yMax, tickCount(ph, 48), w.format)
		cc := f.layoutCart(r, xa, ya, nil, false, st)
		cc.gridX = true
		cache.r, cache.cc = r, cc
		return cc
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		cc := layoutFor(r)
		best, bd := -1, float32(math.Inf(1))
		for i, q := range pts {
			if hidden[q.s] {
				continue
			}
			pp := cc.At(cc.x.norm(q.x), cc.y.norm(q.y))
			d := float32(math.Hypot(float64(pp.X-p.X), float64(pp.Y-p.Y))) - radius(q)
			if d < bd {
				best, bd = i, d
			}
		}
		if best < 0 || bd > 8 {
			return nil
		}
		q := pts[best]
		h := &hover{index: best, series: q.s, anchor: p}
		name := names[q.s]
		h.tip.Index = q.row
		h.tip.Title = name
		h.tip.Items = []TipItem{{Name: w.xf, Value: xf(q.x), Color: st.Color(q.s)}, {Name: w.yf, Value: vf(q.y), Color: st.Color(q.s)}}
		if w.sizeF != "" {
			h.tip.Items = append(h.tip.Items, TipItem{Name: w.sizeF, Value: vf(q.size), Color: st.Color(q.s)})
		}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		cc := layoutFor(r)
		cc.drawGrid(l)
		cc.drawAxes(l)
		for i, q := range pts {
			if hidden[q.s] {
				continue
			}
			col := st.Color(q.s)
			pp := cc.At(cc.x.norm(q.x), cc.y.norm(q.y))
			rr := radius(q) * float32(grow)
			if h != nil && h.index == i {
				rr += 2
			}
			l.Path(paint.NewPath().Circle(pp.X, pp.Y, rr), theme.Fade(col, w.opacity))
			l.StrokePath(paint.NewPath().Circle(pp.X, pp.Y, rr), paint.StrokeStyle{Width: 1}, col)
		}
		if h != nil {
			q := pts[h.index]
			pp := cc.At(cc.x.norm(q.x), cc.y.norm(q.y))
			pl := cc.Plot
			dashLine(l, geom.Pt(crisp(pp.X), pl.Y), geom.Pt(crisp(pp.X), pl.Bottom()), 1, st.Crosshair, []float32{4, 4})
			dashLine(l, geom.Pt(pl.X, crisp(pp.Y)), geom.Pt(pl.Right(), crisp(pp.Y)), 1, st.Crosshair, []float32{4, 4})
		}
	}
	if f.on() {
		x0, x1, y0, y1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range pts {
			x0, x1 = math.Min(x0, p.x), math.Max(x1, p.x)
			y0, y1 = math.Min(y0, p.y), math.Max(y1, p.y)
		}
		f.sem.desc = f.summary(len(names), len(pts), xf(x0), xf(x1), y0, y1, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: f.legendPos(w.legend, LegendAuto, len(names)), hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
