package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// HeatmapW builds a heatmap over two category fields.
type HeatmapW struct {
	common[HeatmapW]
	data          []Row
	xf, yf, colF  string
	ramp          []Color
	noScaleLegend bool
}

// Color is a theme color (ramps, overrides).
type Color = theme.Color

// Heatmap starts a heatmap: X and Y are category fields, Value the numeric
// field mapped onto the sequential palette.
//
//	chart.Heatmap(c, rows).X("hour").Y("weekday").Value("count").Show()
func Heatmap(c *ui.Context, data []Row) *HeatmapW {
	w := &HeatmapW{data: data}
	w.init(c, w, 300)
	return w
}

// X sets the column category field.
func (w *HeatmapW) X(field string) *HeatmapW { w.xf = field; return w }

// Y sets the row category field (first category on top).
func (w *HeatmapW) Y(field string) *HeatmapW { w.yf = field; return w }

// Value sets the numeric field mapped to color.
func (w *HeatmapW) Value(field string) *HeatmapW { w.colF = field; return w }

// Ramp overrides the sequential palette (low → high).
func (w *HeatmapW) Ramp(cs ...Color) *HeatmapW { w.ramp = cs; return w }

// Show builds the chart.
func (w *HeatmapW) Show() View {
	f := w.begin()
	st := f.st
	f.kind(f.c.Locale().Chart.Heatmap, 0)
	xs := uniq(w.data, w.xf)
	ys := uniq(w.data, w.yf)
	if len(xs) == 0 || len(ys) == 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	xi, yi := map[string]int{}, map[string]int{}
	for i, s := range xs {
		xi[s] = i
	}
	for i, s := range ys {
		yi[s] = i
	}
	nx, ny := len(xs), len(ys)
	vals := make([]float64, nx*ny)
	for i := range vals {
		vals[i] = math.NaN()
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range w.data {
		v, ok := Num(r[w.colF])
		if !ok {
			continue
		}
		vals[yi[Str(r[w.yf])]*nx+xi[Str(r[w.xf])]] = v
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if math.IsInf(lo, 1) {
		lo, hi = 0, 1
	}
	if hi == lo {
		hi = lo + 1
	}
	ramp := w.ramp
	if len(ramp) == 0 {
		ramp = st.Sequential
	}
	vf := w.valueFmt()
	alpha := f.tweenVals([]float64{1}, true)[0]
	rev := make([]string, ny)
	for i, s := range ys {
		rev[ny-1-i] = s
	}
	ts := tstyle(st.Font, 0)
	legendH := float32(0)
	if !w.noScaleLegend {
		legendH = 30
	}
	var cache struct {
		r  geom.Rect
		cc *cart
	}
	layoutFor := func(r geom.Rect) *cart {
		if cache.cc != nil && cache.r == r {
			return cache.cc
		}
		rr := r
		rr.H -= legendH
		xa := bandAxis(xs, 0, 0)
		ya := bandAxis(rev, 0, 0)
		cst := *st
		cst.TickLen = 0
		cst.XAxisLine = false
		cc := f.layoutCart(rr, xa, ya, nil, false, &cst)
		cache.r, cache.cc = r, cc
		return cc
	}
	cell := func(cc *cart, i, j int) geom.Rect {
		bx, by := cc.x.band, cc.y.band
		x0 := bx.Map(i)
		y0 := by.Map(ny - 1 - j)
		return cc.Rect(x0, x0+bx.Bandwidth(), y0, y0+by.Bandwidth())
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		cc := layoutFor(r)
		if !cc.Plot.Contains(p) {
			return nil
		}
		nxp, nyp := cc.Norm(p)
		i := cc.x.band.Index(nxp)
		jr := cc.y.band.Index(nyp)
		if i < 0 || jr < 0 {
			return nil
		}
		j := ny - 1 - jr
		v := vals[j*nx+i]
		if math.IsNaN(v) {
			return nil
		}
		h := &hover{index: j*nx + i, anchor: p}
		h.tip = Tip{Index: h.index, Title: ys[j] + " · " + xs[i], Items: []TipItem{{Name: w.colF, Value: vf(v), Color: lerpColor(ramp, (v-lo)/(hi-lo))}}}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		cc := layoutFor(r)
		l.Opacity(float32(alpha))
		for j := 0; j < ny; j++ {
			for i := 0; i < nx; i++ {
				v := vals[j*nx+i]
				if math.IsNaN(v) {
					continue
				}
				cr := cell(cc, i, j)
				col := lerpColor(ramp, (v-lo)/(hi-lo))
				in := cr
				in.X += 0.5
				in.Y += 0.5
				in.W -= 1
				in.H -= 1
				l.Fill(in, geom.Radii{}, col)
				if w.labels && cr.W > textWidth(vf(v), ts)+4 && cr.H > 14 {
					drawText(l, vf(v), cr.X+cr.W/2, cr.Y+cr.H/2, alignCenter, alignMiddle, ts, textOn(col))
				}
			}
		}
		l.Pop()
		if h != nil {
			cr := cell(cc, h.index%nx, h.index/nx)
			l.Stroke(cr.Expand(1), geom.Radii{}, 2, st.Title)
		}
		cc.drawAxes(l)
		if legendH > 0 {
			bw := min(240, cc.Plot.W*0.5)
			bx := cc.Plot.X + (cc.Plot.W-bw)/2
			by := r.Bottom() - 12
			stops := make([]paint.Stop, len(ramp))
			for k, c := range ramp {
				stops[k] = paint.Stop{Offset: float32(k) / float32(max(1, len(ramp)-1)), Color: c}
			}
			br := geom.Rect{X: bx, Y: by - 4, W: bw, H: 8}
			// The scale runs low to high along the x direction.
			g0, g1 := geom.Pt(br.X, 0), geom.Pt(br.Right(), 0)
			if f.mirror {
				g0, g1 = g1, g0
			}
			l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: br, Radii: geom.Rad(2), Start: g0, End: g1, Stops: stops})
			if f.mirror {
				drawText(l, vf(hi), bx-8, by, alignRight, alignMiddle, ts, st.Label)
				drawText(l, vf(lo), br.Right()+8, by, alignLeft, alignMiddle, ts, st.Label)
			} else {
				drawText(l, vf(lo), bx-8, by, alignRight, alignMiddle, ts, st.Label)
				drawText(l, vf(hi), br.Right()+8, by, alignLeft, alignMiddle, ts, st.Label)
			}
			if h != nil {
				v := vals[h.index]
				x := bx + bw*float32((v-lo)/(hi-lo))
				if f.mirror {
					x = bx + bw*float32(1-(v-lo)/(hi-lo))
				}
				l.Path(paint.NewPath().MoveTo(x, by-5).LineTo(x-5, by-11).LineTo(x+5, by-11).Close(), st.Title)
			}
		}
	}
	if f.on() {
		vlo, vhi, np := valueRange([][]float64{vals})
		f.sem.desc = f.summary(ny, np, xs[0], xs[nx-1], vlo, vhi, vf)
	}
	return f.finish(plotSpec{legendPos: LegendNone, hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
