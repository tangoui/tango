package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// RadarW builds a radar (spider) chart.
type RadarW struct {
	common[RadarW]
	xyOpts[RadarW]
	area   bool
	circle bool
}

// Radar starts a radar chart: X is the dimension field, Y the value.
//
//	chart.Radar(c, rows).X("item").Y("score").Series("user").Show()
func Radar(c *ui.Context, data []Row) *RadarW {
	w := &RadarW{area: true}
	w.init(c, w, 320)
	w.xyOpts.self, w.data = w, data
	return w
}

// NoArea draws outlines only.
func (w *RadarW) NoArea() *RadarW { w.area = false; return w }

// Circle draws a circular grid instead of polygons.
func (w *RadarW) Circle() *RadarW { w.circle = true; return w }

// Show builds the chart.
func (w *RadarW) Show() View {
	f := w.begin()
	st := f.st
	f.kind(f.c.Locale().Chart.Radar, 0)
	g := buildCatGrid(w.data, w.xf, w.yf, w.sf)
	n := len(g.cats)
	if n < 3 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	hidden := f.hiddenMask(g.names)
	ns := len(g.names)
	flat := make([]float64, 0, ns*n)
	for s := range g.vals {
		for _, v := range g.vals[s] {
			if math.IsNaN(v) || hidden[s] {
				v = 0
			}
			flat = append(flat, v)
		}
	}
	cur := f.tweenVals(flat, true)
	_, hi, _ := valueDomain(g.vals, nil, hidden, true)
	ax := valueAxis(0, hi, 4, w.format)
	if w.yMax != nil {
		ax = fixedAxis(0, *w.yMax, 4, w.format)
	}
	vf := w.valueFmt()
	ts := tstyle(st.Font, 0)
	maxLW := float32(0)
	for _, cname := range g.cats {
		maxLW = max(maxLW, textWidth(cname, ts))
	}
	items := make([]legendItem, ns)
	for s, nm := range g.names {
		items[s] = legendItem{name: nm, color: st.Color(s), marker: st.LegendMarker}
	}
	polarFor := func(r geom.Rect) Polar {
		R := min(r.W/2-maxLW-12, r.H/2-14)
		p := Polar{Center: r.Center(), Outer: max(R, 20), Start: -math.Pi / 2, Sweep: 2 * math.Pi}
		if f.mirror {
			p = p.Mirrored()
		}
		return p
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		pl := polarFor(r)
		t, d := pl.Locate(p)
		if d > pl.Outer+12 {
			return nil
		}
		i := int(math.Round(t*float64(n))) % n
		h := &hover{index: i, series: -1, anchor: p}
		h.tip = Tip{Index: i, Title: g.cats[i]}
		for s, nm := range g.names {
			if hidden[s] || math.IsNaN(g.vals[s][i]) {
				continue
			}
			if nm == "" {
				nm = w.yf
			}
			h.tip.Items = append(h.tip.Items, TipItem{Name: nm, Value: vf(g.vals[s][i]), Color: st.Color(s)})
		}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		pl := polarFor(r)
		cx, cy := pl.Center.X, pl.Center.Y
		// Grid rings at the value ticks and spokes.
		for _, tk := range ax.ticks {
			if tk.at <= 0 {
				continue
			}
			rr := float32(tk.at) * pl.Outer
			p := paint.NewPath()
			if w.circle {
				p.Circle(cx, cy, rr)
			} else {
				for i := 0; i <= n; i++ {
					q := pl.At(float64(i%n)/float64(n), tk.at)
					if i == 0 {
						p.MoveTo(q.X, q.Y)
					} else {
						p.LineTo(q.X, q.Y)
					}
				}
				p.Close()
			}
			l.StrokePath(p, paint.StrokeStyle{Width: 1, Dash: st.GridDash}, st.Grid)
		}
		for i := 0; i < n; i++ {
			q := pl.At(float64(i)/float64(n), 1)
			l.Line(geom.Pt(cx, cy), q, 1, st.Grid)
			a := pl.Angle(float64(i) / float64(n))
			lp := pl.At(float64(i)/float64(n), 1)
			lp.X += 8 * float32(math.Cos(a))
			lp.Y += 8 * float32(math.Sin(a))
			ha := alignCenter
			if math.Cos(a) > 0.1 {
				ha = alignLeft
			} else if math.Cos(a) < -0.1 {
				ha = alignRight
			}
			va := alignMiddle
			if math.Sin(a) < -0.9 {
				va = alignBottom
			} else if math.Sin(a) > 0.9 {
				va = alignTop
			}
			drawTextDir(l, g.cats[i], lp.X, lp.Y, ha, va, ts, st.Label, f.para)
		}
		for _, tk := range ax.ticks {
			if tk.at == 0 {
				continue
			}
			// Value labels beside the 12 o'clock spoke, on the side the
			// sweep moves away from.
			if f.mirror {
				drawText(l, tk.label, cx-3, cy-float32(tk.at)*pl.Outer, alignRight, alignMiddle, tstyle(st.Font-1, 0), st.Label)
			} else {
				drawText(l, tk.label, cx+3, cy-float32(tk.at)*pl.Outer, alignLeft, alignMiddle, tstyle(st.Font-1, 0), st.Label)
			}
		}
		if h != nil {
			q := pl.At(float64(h.index)/float64(n), 1)
			dashLine(l, geom.Pt(cx, cy), q, 1, st.Crosshair, st.CrosshairDash)
		}
		for s := 0; s < ns; s++ {
			if hidden[s] {
				continue
			}
			col := st.Color(s)
			p := paint.NewPath()
			pts := make([]geom.Point, n)
			for i := 0; i < n; i++ {
				pts[i] = pl.At(float64(i)/float64(n), ax.lin.Map(cur[s*n+i]))
				if i == 0 {
					p.MoveTo(pts[i].X, pts[i].Y)
				} else {
					p.LineTo(pts[i].X, pts[i].Y)
				}
			}
			p.Close()
			if w.area {
				l.Path(p, theme.Fade(col, 0.25))
			}
			l.StrokePath(p, paint.StrokeStyle{Width: st.LineWidth}, col)
			for i, q := range pts {
				rr := float32(3)
				if h != nil && h.index == i {
					rr = 4.5
				}
				l.Path(paint.NewPath().Circle(q.X, q.Y, rr+1), st.Surface)
				l.Path(paint.NewPath().Circle(q.X, q.Y, rr), col)
			}
		}
	}
	if f.on() {
		f.sem.desc = f.gridSummary(g, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: f.legendPos(w.legend, LegendAuto, ns), hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
