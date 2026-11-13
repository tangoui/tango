package chart

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// LiquidW builds a liquid fill (water wave) gauge.
type LiquidW struct {
	common[LiquidW]
	ratio float64
	title string
	still bool
}

// Liquid starts a water wave gauge at ratio 0..1.
func Liquid(c *ui.Context, ratio float64) *LiquidW {
	w := &LiquidW{ratio: ratio}
	w.init(c, w, 220)
	return w
}

// Title sets the caption under the percentage.
func (w *LiquidW) Title(s string) *LiquidW { w.title = s; return w }

// Still stops the wave motion (static screenshots, reduced motion).
func (w *LiquidW) Still() *LiquidW { w.still = true; return w }

// Show builds the chart.
func (w *LiquidW) Show() View {
	f := w.begin()
	st, th := f.st, f.th
	f.kind(f.c.Locale().Chart.Liquid, a11y.RoleMeter)
	f.sem.name = w.title
	cur := f.tweenVals([]float64{math.Max(0, math.Min(1, w.ratio))}, true)[0]
	phase := 0.0
	if !w.still && !w.noAnim && !f.c.ReduceMotion() {
		now := f.c.Now()
		phase = float64(now.UnixMilli()%4000) / 4000 * 2 * math.Pi
		f.c.WakeAt(now.Add(33 * time.Millisecond))
	}
	vf := w.format
	if vf == nil {
		vf = Percent(2)
	}
	col := st.Color(0)
	hit := func(r geom.Rect, p geom.Point) *hover {
		R := min(r.W, r.H) / 2
		c := r.Center()
		if math.Hypot(float64(p.X-c.X), float64(p.Y-c.Y)) > float64(R) {
			return nil
		}
		return &hover{anchor: p, tip: Tip{Items: []TipItem{{Name: w.title, Value: vf(w.ratio), Color: col}}}}
	}
	paintFn := func(l *paint.List, r geom.Rect, _ *hover) {
		R := min(r.W, r.H)/2 - 2
		c := r.Center()
		l.StrokePath(paint.NewPath().Circle(c.X, c.Y, R), paint.StrokeStyle{Width: 2}, col)
		inner := R - 6
		clip := geom.Rect{X: c.X - inner, Y: c.Y - inner, W: 2 * inner, H: 2 * inner}
		l.Clip(clip, geom.Rad(inner))
		level := c.Y + inner - float32(cur)*2*inner
		amp := inner * 0.06
		for k, a := range []float64{0.45, 1} {
			// The wave is traced from the start side (mirrored: the right).
			x0, x1, sx := clip.X, clip.Right(), float32(1)
			if f.mirror {
				x0, x1, sx = clip.Right(), clip.X, -1
			}
			p := paint.NewPath().MoveTo(x0, clip.Bottom())
			off := phase + float64(k)*math.Pi*0.7
			for x := float32(0); x <= clip.W; x += 3 {
				y := level + amp*float32(math.Sin(float64(x)/float64(clip.W)*2*math.Pi+off))
				p.LineTo(x0+sx*x, y)
			}
			p.LineTo(x1, clip.Bottom()).Close()
			l.Path(p, theme.Fade(col, a*0.85))
		}
		l.Pop()
		vs := tstyle(min(32, R*0.32), text.Medium)
		lh := text.LineHeightOf(vs)
		y := c.Y - lh/2
		if w.title != "" {
			y = c.Y - lh*0.75
		}
		txtCol := th.TextHeading
		if float32(cur) > 0.55 {
			txtCol = theme.Hex("#ffffff")
		}
		drawText(l, vf(cur), c.X, y, alignCenter, alignTop, vs, txtCol)
		if w.title != "" {
			drawTextDir(l, w.title, c.X, y+lh, alignCenter, alignTop, tstyle(th.FontSize, 0), txtCol, f.para)
		}
	}
	if f.on() {
		r := math.Max(0, math.Min(1, w.ratio))
		f.sem.ranged, f.sem.now, f.sem.lo, f.sem.hi, f.sem.value = true, r, 0, 1, vf(w.ratio)
	}
	return f.finish(plotSpec{legendPos: LegendNone, hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
