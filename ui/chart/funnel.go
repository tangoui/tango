package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// FunnelW builds a funnel chart.
type FunnelW struct {
	common[FunnelW]
	data   []Row
	xf, yf string
	noRate bool
}

// Funnel starts a funnel: X is the stage field, Y the value; stages keep
// the row order (largest first).
//
//	chart.Funnel(c, rows).X("stage").Y("count").Show()
func Funnel(c *ui.Context, data []Row) *FunnelW {
	w := &FunnelW{data: data}
	w.init(c, w, 300)
	return w
}

// X sets the stage field.
func (w *FunnelW) X(field string) *FunnelW { w.xf = field; return w }

// Y sets the value field.
func (w *FunnelW) Y(field string) *FunnelW { w.yf = field; return w }

// NoConversion hides the stage-to-stage conversion rates.
func (w *FunnelW) NoConversion() *FunnelW { w.noRate = true; return w }

// Show builds the chart.
func (w *FunnelW) Show() View {
	f := w.begin()
	st := f.st
	f.kind(f.c.Locale().Chart.Funnel, 0)
	g := buildCatGrid(w.data, w.xf, w.yf, "")
	n := len(g.cats)
	if n == 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	hidden := f.hiddenMask(g.cats)
	raw := g.vals[0]
	target := make([]float64, n)
	maxV := 0.0
	for i, v := range raw {
		if math.IsNaN(v) || hidden[i] {
			v = 0
		}
		target[i] = v
		maxV = math.Max(maxV, v)
	}
	cur := f.tweenVals(target, true)
	vf := w.valueFmt()
	rateName := f.c.Locale().Chart.Conversion
	items := make([]legendItem, n)
	for i, c := range g.cats {
		items[i] = legendItem{name: c, color: st.Color(i), marker: st.LegendMarker}
	}
	// Visible stages in order.
	var vis []int
	for i := range g.cats {
		if !hidden[i] {
			vis = append(vis, i)
		}
	}
	ts := tstyle(st.Font, 0)
	rateW := float32(0)
	if !w.noRate {
		rateW = textWidth(rateName+" 100.0%", ts) + 24
	}
	layoutFor := func(r geom.Rect) (geom.Rect, float32) {
		body := geom.Rect{X: r.X + rateW, Y: r.Y + 4, W: max(10, r.W-2*rateW), H: r.H - 8}
		if len(vis) == 0 {
			return body, 0
		}
		return body, body.H / float32(len(vis))
	}
	width := func(body geom.Rect, v float64) float32 {
		if maxV <= 0 {
			return 0
		}
		return body.W * float32(v/maxV)
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		body, rowH := layoutFor(r)
		if rowH <= 0 || p.Y < body.Y || p.Y >= body.Bottom() {
			return nil
		}
		k := int((p.Y - body.Y) / rowH)
		if k < 0 || k >= len(vis) {
			return nil
		}
		i := vis[k]
		h := &hover{index: i, series: i, anchor: p}
		h.tip = Tip{Index: i, Title: g.cats[i], Items: []TipItem{{Name: w.yf, Value: vf(raw[i]), Color: st.Color(i)}}}
		if k > 0 && raw[vis[0]] > 0 {
			h.tip.Items = append(h.tip.Items, TipItem{Name: rateName, Value: Percent(1)(raw[i] / raw[vis[k-1]]), Color: st.Color(i)})
		}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		body, rowH := layoutFor(r)
		cx := body.X + body.W/2
		for k, i := range vis {
			y0 := body.Y + float32(k)*rowH
			y1 := y0 + rowH - 2
			wt := width(body, cur[i])
			wb := wt * 0.6
			if k+1 < len(vis) {
				wb = width(body, cur[vis[k+1]])
			}
			col := st.Color(i)
			p := paint.NewPath().MoveTo(cx-wt/2, y0).LineTo(cx+wt/2, y0).LineTo(cx+wb/2, y1).LineTo(cx-wb/2, y1).Close()
			l.Path(p, col)
			if h != nil && h.index == i {
				l.StrokePath(p, paint.StrokeStyle{Width: 1.5}, st.Surface)
				l.Path(p, theme.Fade(st.Surface, 0.15))
			}
			label := g.cats[i] + " " + vf(raw[i])
			// Outside labels go on the right and conversion rates on the
			// left; the sides swap when mirrored.
			switch {
			case textWidth(label, ts)+8 < (wt+wb)/2:
				drawTextDir(l, label, cx, (y0+y1)/2, alignCenter, alignMiddle, ts, textOn(col), f.para)
			case f.mirror:
				drawTextDir(l, label, cx-wt/2-8, (y0+y1)/2, alignRight, alignMiddle, ts, st.Label, f.para)
			default:
				drawTextDir(l, label, cx+wt/2+8, (y0+y1)/2, alignLeft, alignMiddle, ts, st.Label, f.para)
			}
			if !w.noRate && k > 0 && raw[vis[k-1]] > 0 {
				rate := rateName + " " + Percent(1)(raw[i]/raw[vis[k-1]])
				if f.mirror {
					drawTextDir(l, rate, body.Right()+12, y0, alignLeft, alignMiddle, ts, st.Label, f.para)
					l.Line(geom.Pt(body.Right()+8, crisp(y0)), geom.Pt(cx+wt/2+4, crisp(y0)), 1, st.Grid)
				} else {
					drawTextDir(l, rate, body.X-12, y0, alignRight, alignMiddle, ts, st.Label, f.para)
					l.Line(geom.Pt(body.X-8, crisp(y0)), geom.Pt(cx-wt/2-4, crisp(y0)), 1, st.Grid)
				}
			}
		}
	}
	if f.on() {
		f.sem.desc = f.gridSummary(g, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: f.legendPos(w.legend, LegendAuto, n), hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
