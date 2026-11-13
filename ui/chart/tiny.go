package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

type tinyKind uint8

const (
	tinyLine tinyKind = iota
	tinyArea
	tinyColumn
	tinyProgress
)

// TinyW builds a sparkline for statistic cards: no axes or legend, a
// compact tooltip on hover.
type TinyW struct {
	common[TinyW]
	kind   tinyKind
	vals   []float64
	names  []string
	smooth bool
	color  *theme.Color
	target *float64
}

func tiny(c *ui.Context, k tinyKind, vals []float64) *TinyW {
	w := &TinyW{kind: k, vals: vals}
	w.init(c, w, 46)
	if k == tinyProgress {
		w.height = 8
	}
	return w
}

// TinyLine starts a sparkline.
func TinyLine(c *ui.Context, vals []float64) *TinyW { return tiny(c, tinyLine, vals) }

// TinyArea starts a filled sparkline.
func TinyArea(c *ui.Context, vals []float64) *TinyW { return tiny(c, tinyArea, vals) }

// TinyColumn starts a mini column chart.
func TinyColumn(c *ui.Context, vals []float64) *TinyW { return tiny(c, tinyColumn, vals) }

// TinyProgress starts a thin progress bar (ratio 0..1).
func TinyProgress(c *ui.Context, ratio float64) *TinyW {
	return tiny(c, tinyProgress, []float64{ratio})
}

// Smooth draws the line as a monotone curve.
func (w *TinyW) Smooth() *TinyW { w.smooth = true; return w }

// Color sets the mark color (default: first palette color).
func (w *TinyW) Color(col theme.Color) *TinyW { w.color = &col; return w }

// Names labels the points in the tooltip (e.g. dates).
func (w *TinyW) Names(names ...string) *TinyW { w.names = names; return w }

// Target draws a marker line at a target value (progress: ratio).
func (w *TinyW) Target(v float64) *TinyW { w.target = &v; return w }

// Show builds the chart.
func (w *TinyW) Show() View {
	f := w.begin()
	st := f.st
	if w.kind == tinyProgress {
		f.kind(f.c.Locale().A11y.Progress, a11y.RoleProgressBar)
	} else {
		f.kind(f.c.Locale().Chart.Sparkline, a11y.RoleImage)
	}
	if len(w.vals) == 0 {
		return f.finish(plotSpec{empty: true, loading: w.loading})
	}
	col := st.Color(0)
	if w.color != nil {
		col = *w.color
	}
	cur := f.tweenVals(w.vals, w.kind != tinyLine && w.kind != tinyArea)
	n := len(w.vals)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range w.vals {
		if !math.IsNaN(v) {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if w.kind == tinyColumn || w.kind == tinyArea {
		lo = math.Min(lo, 0)
	}
	if hi <= lo {
		hi = lo + 1
	}
	vf := w.valueFmt()
	ym := func(r geom.Rect, v float64) float32 {
		return r.Bottom() - 1 - float32((v-lo)/(hi-lo))*(r.H-2)
	}
	band := Band{N: n, R0: 0, R1: 1, PaddingInner: 0.25}
	// flip mirrors a pixel x inside r when the chart is mirrored.
	flip := func(r geom.Rect, x float32) float32 {
		if f.mirror {
			return r.X + r.Right() - x
		}
		return x
	}
	xm := func(r geom.Rect, i int) float32 {
		if w.kind == tinyColumn {
			return flip(r, r.X+float32(band.Center(i))*r.W)
		}
		if n == 1 {
			return r.X + r.W/2
		}
		return flip(r, r.X+1+float32(i)/float32(n-1)*(r.W-2))
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		p.X = flip(r, p.X)
		var i int
		if w.kind == tinyProgress {
			i = 0
		} else if w.kind == tinyColumn {
			i = band.Index(float64((p.X - r.X) / r.W))
			if i < 0 {
				return nil
			}
		} else {
			i = Point{N: n, R0: float64(r.X + 1), R1: float64(r.Right() - 1)}.Index(float64(p.X))
		}
		name := ""
		if i < len(w.names) {
			name = w.names[i]
		}
		val := vf(w.vals[i])
		if w.kind == tinyProgress && w.format == nil {
			val = Percent(0)(w.vals[0])
		}
		h := &hover{index: i, anchor: p}
		h.tip = Tip{Index: i, Items: []TipItem{{Name: name, Value: val, Color: col}}}
		return h
	}
	paintFn := func(l *paint.List, r geom.Rect, h *hover) {
		switch w.kind {
		case tinyProgress:
			l.Fill(r, geom.Radii{}, st.Track)
			v := math.Max(0, math.Min(1, cur[0]))
			bar := geom.Rect{X: r.X, Y: r.Y, W: r.W * float32(v), H: r.H}
			if f.mirror {
				bar.X = r.Right() - bar.W
			}
			l.Fill(bar, geom.Radii{}, col)
			if w.target != nil {
				x := flip(r, r.X+r.W*float32(math.Max(0, math.Min(1, *w.target))))
				l.Fill(geom.Rect{X: x - 1, Y: r.Y - 3, W: 2, H: r.H + 6}, geom.Radii{}, st.Positive)
			}
		case tinyColumn:
			for i, v := range cur {
				x0 := r.X + float32(band.Map(i))*r.W
				bw := float32(band.Bandwidth()) * r.W
				if f.mirror {
					x0 = r.X + r.Right() - x0 - bw
				}
				y0, y1 := ym(r, 0), ym(r, v)
				c := col
				if h != nil && h.index != i {
					c = theme.Fade(col, 0.65)
				}
				l.Fill(geom.Rect{X: x0, Y: min(y0, y1), W: bw, H: abs32(y1 - y0)}, geom.Radii{}, c)
			}
		default:
			pts := make([]geom.Point, 0, n)
			for i, v := range cur {
				if !math.IsNaN(v) {
					pts = append(pts, geom.Pt(xm(r, i), ym(r, v)))
				}
			}
			if len(pts) == 0 {
				return
			}
			reveal := geom.Rect{X: r.X - 2, Y: r.Y - 4, W: (r.W + 4) * f.enter, H: r.H + 8}
			if f.mirror {
				reveal.X = r.Right() + 2 - reveal.W
			}
			l.Clip(reveal, geom.Radii{})
			if w.kind == tinyArea {
				ap := paint.NewPath()
				tracePath(ap, pts, w.smooth, StepNone, true)
				by := ym(r, math.Max(lo, 0))
				ap.LineTo(pts[len(pts)-1].X, by).LineTo(pts[0].X, by).Close()
				if st.AreaGrad {
					l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Path: ap, Start: geom.Pt(0, r.Y), End: geom.Pt(0, r.Bottom()),
						Stops: []paint.Stop{{Offset: 0, Color: theme.Fade(col, 0.55)}, {Offset: 1, Color: theme.Fade(col, 0.05)}}})
				} else {
					l.Path(ap, theme.Fade(col, 0.45))
				}
			}
			lp := paint.NewPath()
			tracePath(lp, pts, w.smooth, StepNone, true)
			lw := float32(1.5)
			if w.kind == tinyLine {
				lw = 2
			}
			l.StrokePath(lp, paint.StrokeStyle{Width: lw, Cap: paint.CapRound}, col)
			l.Pop()
			if w.target != nil {
				y := crisp(ym(r, *w.target))
				dashLine(l, geom.Pt(r.X, y), geom.Pt(r.Right(), y), 1, st.Crosshair, []float32{3, 3})
			}
			if h != nil && h.index < n && !math.IsNaN(cur[h.index]) {
				p := geom.Pt(xm(r, h.index), ym(r, cur[h.index]))
				l.Line(geom.Pt(crisp(p.X), r.Y), geom.Pt(crisp(p.X), r.Bottom()), 1, st.Crosshair)
				l.Path(paint.NewPath().Circle(p.X, p.Y, 4), st.Surface)
				l.Path(paint.NewPath().Circle(p.X, p.Y, 2.5), col)
			}
		}
	}
	if f.on() {
		if w.kind == tinyProgress {
			r := math.Max(0, math.Min(1, w.vals[0]))
			f.sem.ranged, f.sem.now, f.sem.lo, f.sem.hi = true, r*100, 0, 100
			f.sem.value = Percent(0)(r)
			if w.format != nil {
				f.sem.value = w.format(w.vals[0])
			}
		} else {
			x0, x1 := "", ""
			if len(w.names) > 0 {
				x0, x1 = w.names[0], w.names[len(w.names)-1]
			}
			vlo, vhi, np := valueRange([][]float64{w.vals})
			f.sem.desc = f.summary(1, np, x0, x1, vlo, vhi, vf)
		}
	}
	return f.finish(plotSpec{legendPos: LegendNone, hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
