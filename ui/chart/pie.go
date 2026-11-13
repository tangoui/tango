package chart

import (
	"math"
	"sort"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// PieW builds a pie or donut chart.
type PieW struct {
	common[PieW]
	data             []Row
	angleF, colorF   string
	inner            float64
	noLabel          bool
	center           bool
	cTitle, cValue   string
	cSet             bool
	labelPct, inside bool
}

// Pie starts a pie chart: Angle is the value field, Color the slice name
// field. Rows with the same name are summed.
//
//	chart.Pie(c, rows).Angle("value").Color("type").Inner(0.6).Show()
func Pie(c *ui.Context, data []Row) *PieW {
	w := &PieW{data: data}
	w.init(c, w, 300)
	return w
}

// Donut starts a donut (ring) chart with a centre statistic.
func Donut(c *ui.Context, data []Row) *PieW { return Pie(c, data).Inner(0.64) }

// Angle sets the value field.
func (w *PieW) Angle(field string) *PieW { w.angleF = field; return w }

// Color sets the slice name field.
func (w *PieW) Color(field string) *PieW { w.colorF = field; return w }

// Inner sets the hole radius as a fraction of the outer radius (donut).
func (w *PieW) Inner(r float64) *PieW { w.inner = r; return w }

// NoLabel hides the outer labels and their leader lines.
func (w *PieW) NoLabel() *PieW { w.noLabel = true; return w }

// InnerLabel draws percentages inside the slices instead of outer labels.
func (w *PieW) InnerLabel() *PieW { w.inside = true; return w }

// Center sets the donut centre statistic (default: total of the visible
// slices; empty strings hide it).
func (w *PieW) Center(title, value string) *PieW {
	w.cTitle, w.cValue, w.cSet = title, value, true
	return w
}

type slice struct {
	name   string
	value  float64
	a0, a1 float64
}

// Show builds the chart.
func (w *PieW) Show() View {
	f := w.begin()
	st, th := f.st, f.th
	if w.inner > 0 {
		f.kind(f.c.Locale().Chart.Donut, 0)
	} else {
		f.kind(f.c.Locale().Chart.Pie, 0)
	}
	names := uniq(w.data, w.colorF)
	if w.colorF == "" {
		names = nil
	}
	vals := make([]float64, len(names))
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}
	for _, r := range w.data {
		if v, ok := Num(r[w.angleF]); ok && v > 0 {
			vals[idx[Str(r[w.colorF])]] += v
		}
	}
	total := 0.0
	for _, v := range vals {
		total += v
	}
	if len(names) == 0 || total <= 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	hidden := f.hiddenMask(names)
	target := make([]float64, len(vals))
	visTotal := 0.0
	for i, v := range vals {
		if !hidden[i] {
			target[i] = v
			visTotal += v
		}
	}
	cur := f.tweenVals(target, false)
	sum := 0.0
	for _, v := range cur {
		sum += v
	}
	sweep := 2 * math.Pi * float64(f.enter)
	slices := make([]slice, len(names))
	// Slices run clockwise from 12 o'clock, counter-clockwise when mirrored
	// (from the mirrored start, 3π/2 ≡ -π/2, as Polar.Mirrored gives).
	a, dir := -math.Pi/2, 1.0
	if f.mirror {
		a, dir = 3*math.Pi/2, -1
	}
	for i, v := range cur {
		da := 0.0
		if sum > 0 {
			da = v / sum * sweep
		}
		if f.mirror {
			slices[i] = slice{name: names[i], value: vals[i], a0: a - da, a1: a}
		} else {
			slices[i] = slice{name: names[i], value: vals[i], a0: a, a1: a + da}
		}
		a += dir * da
	}
	vf := w.valueFmt()
	pct := func(i int) string {
		if visTotal <= 0 || hidden[i] {
			return "0%"
		}
		return FormatNumber(vals[i]/visTotal*100, 1) + "%"
	}
	items := make([]legendItem, len(names))
	for i, n := range names {
		items[i] = legendItem{name: n, color: st.Color(i), marker: MarkerCircle}
	}
	if st.LegendMarker == MarkerRoundRect {
		for i := range items {
			items[i].marker = MarkerRoundRect
		}
	}
	defPos := LegendRight
	if st.LegendCenter {
		defPos = LegendTop
	}
	pos := f.legendPos(w.legend, defPos, 2)
	labels := !w.noLabel && !w.inside
	ts := tstyle(st.Font, 0)
	lh := text.LineHeightOf(ts)
	labelText := func(i int) string { return names[i] + "  " + pct(i) }
	maxLW := float32(0)
	if labels {
		for i := range names {
			maxLW = max(maxLW, textWidth(labelText(i), ts))
		}
	}
	polarFor := func(r geom.Rect) Polar {
		R := min(r.W, r.H)/2 - 4
		if labels {
			R = min(r.H/2-lh, r.W/2-maxLW-28)
			R = max(R, min(r.W, r.H)/4)
		}
		p := Polar{Center: r.Center(), Inner: R * float32(w.inner), Outer: R, Start: -math.Pi / 2, Sweep: 2 * math.Pi}
		if f.mirror {
			p = p.Mirrored()
		}
		return p
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		pl := polarFor(r)
		t, d := pl.Locate(p)
		if d > pl.Outer+6 || d < pl.Inner {
			return nil
		}
		ang := pl.Angle(t)
		for i, s := range slices {
			if hidden[i] || s.a1 <= s.a0 {
				continue
			}
			if ang >= s.a0 && ang < s.a1 {
				h := &hover{index: i, series: i, anchor: p}
				h.tip = Tip{Index: i, Items: []TipItem{{Name: s.name, Value: vf(s.value) + "（" + pct(i) + "）", Color: st.Color(i)}}}
				return h
			}
		}
		return nil
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		pl := polarFor(r)
		cx, cy := pl.Center.X, pl.Center.Y
		for i, s := range slices {
			if s.a1-s.a0 <= 1e-4 {
				continue
			}
			r1 := pl.Outer
			if h != nil && h.index == i {
				r1 += 6
			}
			path := ring(cx, cy, pl.Inner, r1, s.a0, s.a1)
			l.Path(path, st.Color(i))
			if st.SliceStroke > 0 {
				l.StrokePath(path, paint.StrokeStyle{Width: st.SliceStroke}, st.Surface)
			}
			if w.inside && s.a1-s.a0 > 0.25 {
				m := (s.a0 + s.a1) / 2
				rr := (pl.Inner + pl.Outer) / 2
				if pl.Inner == 0 {
					rr = pl.Outer * 0.62
				}
				drawText(l, pct(i), cx+rr*float32(math.Cos(m)), cy+rr*float32(math.Sin(m)), alignCenter, alignMiddle, ts, textOn(st.Color(i)))
			}
		}
		if labels && f.enter >= 1 {
			w.drawLabels(l, pl, slices, hidden, labelText, ts, lh, st, f.para)
		}
		if w.inner > 0 {
			title, value := w.cTitle, w.cValue
			if !w.cSet {
				title = f.c.Locale().Chart.Total
				value = vf(visTotal)
			}
			if h != nil && !w.cSet {
				title, value = names[h.index], vf(vals[h.index])
			}
			vs := tstyle(min(24, pl.Inner*0.42), text.Medium)
			tst := tstyle(th.FontSize, 0)
			hT, hV := text.LineHeightOf(tst), text.LineHeightOf(vs)
			if title == "" {
				hT = 0
			}
			if value == "" {
				hV = 0
			}
			y := cy - (hT+hV)/2
			if title != "" {
				drawTextDir(l, title, cx, y, alignCenter, alignTop, tst, st.Title, f.para)
			}
			if value != "" {
				drawText(l, value, cx, y+hT, alignCenter, alignTop, vs, th.TextHeading)
			}
		}
	}
	if f.on() {
		lo, hi, _ := valueRange([][]float64{vals})
		f.sem.desc = f.summary(1, len(names), "", "", lo, hi, vf)
	}
	return f.finish(plotSpec{items: items, legendPos: pos, hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}

type pieLabel struct {
	i      int
	mid    float64
	y      float32
	anchor geom.Point
	elbow  geom.Point
}

// drawLabels places outer labels on both sides with leader lines, pushing
// them apart vertically so they never overlap.
func (w *PieW) drawLabels(l *paint.List, pl Polar, slices []slice, hidden []bool, txt func(int) string, ts text.Style, lh float32, st *Style, dir text.Direction) {
	var left, right []pieLabel
	cx, cy := pl.Center.X, pl.Center.Y
	for i, s := range slices {
		if hidden[i] || s.a1-s.a0 < 0.02 {
			continue
		}
		m := (s.a0 + s.a1) / 2
		cos, sin := float32(math.Cos(m)), float32(math.Sin(m))
		lb := pieLabel{i: i, mid: m,
			anchor: geom.Pt(cx+pl.Outer*cos, cy+pl.Outer*sin),
			elbow:  geom.Pt(cx+(pl.Outer+12)*cos, cy+(pl.Outer+12)*sin)}
		lb.y = lb.elbow.Y
		if cos >= 0 {
			right = append(right, lb)
		} else {
			left = append(left, lb)
		}
	}
	place := func(ls []pieLabel, sideRight bool) {
		sort.Slice(ls, func(a, b int) bool { return ls[a].y < ls[b].y })
		top := cy - pl.Outer - lh
		bottom := cy + pl.Outer + lh
		for k := range ls {
			if k > 0 && ls[k].y < ls[k-1].y+lh {
				ls[k].y = ls[k-1].y + lh
			}
			ls[k].y = max(ls[k].y, top)
		}
		for k := len(ls) - 1; k >= 0; k-- {
			if k < len(ls)-1 && ls[k].y > ls[k+1].y-lh {
				ls[k].y = ls[k+1].y - lh
			}
			ls[k].y = min(ls[k].y, bottom)
		}
		for _, lb := range ls {
			col := st.Color(lb.i)
			x2 := cx + pl.Outer + 24
			if !sideRight {
				x2 = cx - pl.Outer - 24
			}
			p := paint.NewPath().MoveTo(lb.anchor.X, lb.anchor.Y).LineTo(lb.elbow.X, lb.y).LineTo(x2, lb.y)
			l.StrokePath(p, paint.StrokeStyle{Width: 1}, col)
			if sideRight {
				drawTextDir(l, txt(lb.i), x2+4, lb.y, alignLeft, alignMiddle, ts, st.Title, dir)
			} else {
				drawTextDir(l, txt(lb.i), x2-4, lb.y, alignRight, alignMiddle, ts, st.Title, dir)
			}
		}
	}
	place(right, true)
	place(left, false)
}
