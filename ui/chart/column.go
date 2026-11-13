package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ColumnW builds a column (vertical) or bar (horizontal) chart.
type ColumnW struct {
	common[ColumnW]
	xyOpts[ColumnW]
	horizontal bool
	stack      bool
	percent    bool
	radius     float32
	maxWidth   float32
	padding    float64
}

// Column starts a vertical bar chart. With Series the bars are grouped
// side by side; Stack and Percent stack them instead.
//
//	chart.Column(c, rows).X("month").Y("sales").Series("shop").Stack().Show()
func Column(c *ui.Context, data []Row) *ColumnW {
	w := &ColumnW{padding: -1}
	w.init(c, w, 300)
	w.xyOpts.self, w.data = w, data
	return w
}

// Bar starts a horizontal bar chart: X is the category field (listed top
// to bottom), Y the value.
func Bar(c *ui.Context, data []Row) *ColumnW {
	w := Column(c, data)
	w.horizontal = true
	return w
}

// Stack stacks the series.
func (w *ColumnW) Stack() *ColumnW { w.stack = true; return w }

// Percent stacks the series normalised to 100%.
func (w *ColumnW) Percent() *ColumnW { w.stack, w.percent = true, true; return w }

// Radius rounds the bar ends.
func (w *ColumnW) Radius(r float32) *ColumnW { w.radius = r; return w }

// MaxWidth caps the bar thickness.
func (w *ColumnW) MaxWidth(px float32) *ColumnW { w.maxWidth = px; return w }

// Padding sets the empty fraction between bands (0..1).
func (w *ColumnW) Padding(p float64) *ColumnW { w.padding = p; return w }

// Show builds the chart.
func (w *ColumnW) Show() View {
	f := w.begin()
	st := f.st
	if w.horizontal {
		f.kind(f.c.Locale().Chart.Bar, 0)
	} else {
		f.kind(f.c.Locale().Chart.Column, 0)
	}
	g := f.gridFor(w.data, w.xf, w.yf, w.sf, true)
	if g.empty() {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	hidden := f.hiddenMask(g.names)
	ns, nx := len(g.names), len(g.xs)
	grouped := ns > 1 && !w.stack

	var lo, hi [][]float64
	if w.stack {
		lo, hi = Stack(g.vals, hidden, w.percent)
	} else {
		lo = make([][]float64, ns)
		hi = make([][]float64, ns)
		for s := range g.vals {
			lo[s] = make([]float64, nx)
			hi[s] = make([]float64, nx)
			for i, v := range g.vals[s] {
				if math.IsNaN(v) || hidden[s] {
					v = 0
				}
				if v < 0 {
					lo[s][i] = v
				} else {
					hi[s][i] = v
				}
			}
		}
	}
	flat := make([]float64, 0, 2*ns*nx)
	for s := range hi {
		flat = append(flat, hi[s]...)
		flat = append(flat, lo[s]...)
	}
	cur := f.tweenVals(flat, true)
	ahi := make([][]float64, ns)
	alo := make([][]float64, ns)
	for s := 0; s < ns; s++ {
		ahi[s] = cur[2*s*nx : (2*s+1)*nx]
		alo[s] = cur[(2*s+1)*nx : (2*s+2)*nx]
	}

	vf := w.valueFmt()
	dlo, dhi, _ := valueDomain(hi, lo, hidden, true)
	pad := st.BarPadding
	if w.padding >= 0 {
		pad = w.padding
	}
	if grouped && w.padding < 0 {
		pad = math.Max(pad, 0.2)
	}
	items := make([]legendItem, ns)
	for s, n := range g.names {
		items[s] = legendItem{name: n, color: st.Color(s), marker: st.LegendMarker}
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
		xa := bandAxis(g.cats, pad, st.BarOuter)
		var ya *axis
		if w.percent {
			ya = fixedAxis(0, 1, 5, Percent(0))
		} else {
			n := tickCount(ph, 48)
			if w.horizontal {
				n = tickCount(pw, 90)
			}
			ya = buildValueAxis(dlo, dhi, w.yMin, w.yMax, n, w.format)
		}
		cc := f.layoutCart(r, xa, ya, nil, w.horizontal, st)
		cache.r, cache.cc = r, cc
		return cc
	}
	// barSpan returns the normalised base-axis extent of a bar.
	barSpan := func(cc *cart, s, i int) (float64, float64) {
		b := cc.x.band
		x0, bw := b.Map(i), b.Bandwidth()
		length := cc.Plot.W
		if w.horizontal {
			length = cc.Plot.H
		}
		if grouped {
			vis := 0
			k := 0
			for t := 0; t < ns; t++ {
				if !hidden[t] {
					if t == s {
						k = vis
					}
					vis++
				}
			}
			vis = max(vis, 1)
			gap := 0.0
			if vis > 1 {
				gap = 2 / float64(length) // 2px between grouped bars
			}
			sw := (bw - gap*float64(vis-1)) / float64(vis)
			x0 += float64(k) * (sw + gap)
			bw = sw
		}
		if w.maxWidth > 0 && bw*float64(length) > float64(w.maxWidth) {
			m := float64(w.maxWidth) / float64(length)
			x0 += (bw - m) / 2
			bw = m
		}
		return x0, x0 + bw
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		cc := layoutFor(r)
		if !cc.Plot.Contains(p) {
			return nil
		}
		i := xIndexAt(g, cc.x, cc.baseNorm(p))
		if i < 0 {
			return nil
		}
		h := &hover{index: i, series: -1, anchor: p}
		h.tip.Index, h.tip.Title = i, g.cats[i]
		for s, n := range g.names {
			v := g.vals[s][i]
			if hidden[s] || math.IsNaN(v) {
				continue
			}
			if n == "" {
				n = w.yf
			}
			val := vf(v)
			if w.percent {
				val += "（" + Percent(1)(hi[s][i]-lo[s][i]) + "）"
			}
			h.tip.Items = append(h.tip.Items, TipItem{Name: n, Value: val, Color: st.Color(s)})
		}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		cc := layoutFor(r)
		if h != nil {
			b := cc.x.band
			x0 := b.Map(h.index) - b.Step()*b.PaddingInner/2
			hr := cc.Rect(x0, x0+b.Step(), 0, 1)
			l.Fill(hr, geom.Radii{}, st.BandHover)
		}
		cc.drawGrid(l)
		ts := tstyle(st.Font, 0)
		for s := 0; s < ns; s++ {
			if hidden[s] {
				continue
			}
			col := st.Color(s)
			for i := 0; i < nx; i++ {
				if math.IsNaN(g.vals[s][i]) {
					continue
				}
				x0, x1 := barSpan(cc, s, i)
				y0, y1 := alo[s][i], ahi[s][i]
				if !w.stack && g.vals[s][i] >= 0 {
					y0 = 0
				} else if !w.stack {
					y1 = 0
				}
				br := cc.Rect(x0, x1, cc.y.norm(y0), cc.y.norm(y1))
				if br.W <= 0 || br.H <= 0 {
					continue
				}
				rad := geom.Radii{}
				if rr := w.radius + st.BarRadius; rr > 0 {
					rr = min(rr, br.W/2, br.H/2)
					top := !w.stack || isTopSegment(hi, lo, hidden, s, i)
					switch {
					case !top:
					case w.horizontal && f.mirror:
						rad = geom.Radii{TL: rr, BL: rr}
					case w.horizontal:
						rad = geom.Radii{TR: rr, BR: rr}
					default:
						rad = geom.Radii{TL: rr, TR: rr}
					}
				}
				l.Fill(br, rad, col)
				if w.stack && st.SliceStroke > 0 && ns > 1 {
					// Thin separators between stacked segments (G2).
					if w.horizontal && f.mirror {
						l.Fill(geom.Rect{X: br.X - 0.5, Y: br.Y, W: 1, H: br.H}, geom.Radii{}, st.Surface)
					} else if w.horizontal {
						l.Fill(geom.Rect{X: br.Right() - 0.5, Y: br.Y, W: 1, H: br.H}, geom.Radii{}, st.Surface)
					} else {
						l.Fill(geom.Rect{X: br.X, Y: br.Y - 0.5, W: br.W, H: 1}, geom.Radii{}, st.Surface)
					}
				}
				if w.labels {
					lbl := vf(g.vals[s][i])
					if w.percent {
						lbl = Percent(0)(hi[s][i] - lo[s][i])
					}
					w.drawLabel(l, lbl, br, col, ts, st, f.mirror)
				}
			}
		}
		cc.drawAxes(l)
	}
	if f.on() {
		f.sem.desc = f.gridSummary(g, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: f.legendPos(w.legend, LegendAuto, ns), hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}

func (w *ColumnW) drawLabel(l *paint.List, s string, br geom.Rect, col theme.Color, ts text.Style, st *Style, mirror bool) {
	tw := textWidth(s, ts)
	if w.stack {
		if (w.horizontal && tw+8 > br.W) || (!w.horizontal && (ts.Size+4 > br.H || tw+4 > br.W)) {
			return
		}
		drawText(l, s, br.X+br.W/2, br.Y+br.H/2, alignCenter, alignMiddle, ts, textOn(col))
		return
	}
	if w.horizontal && mirror {
		drawText(l, s, br.X-6, br.Y+br.H/2, alignRight, alignMiddle, ts, st.Label)
		return
	}
	if w.horizontal {
		drawText(l, s, br.Right()+6, br.Y+br.H/2, alignLeft, alignMiddle, ts, st.Label)
		return
	}
	drawText(l, s, br.X+br.W/2, br.Y-4, alignCenter, alignBottom, ts, st.Label)
}

// isTopSegment reports whether segment s is the outermost visible one of
// its stack at x i.
func isTopSegment(hi, lo [][]float64, hidden []bool, s, i int) bool {
	pos := hi[s][i] > 0 || lo[s][i] >= 0
	for t := s + 1; t < len(hi); t++ {
		if hidden[t] || hi[t][i] == lo[t][i] {
			continue
		}
		if (hi[t][i] > 0) == pos {
			return false
		}
	}
	return true
}
