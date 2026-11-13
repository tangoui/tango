package chart

import (
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// GaugeW builds a gauge (dial) chart.
type GaugeW struct {
	common[GaugeW]
	value    float64
	lo, hi   float64
	title    string
	stops    []float64
	stopCols []theme.Color
	valFmt   Formatter
}

// Gauge starts a gauge showing value on the range 0..1 (see Range).
//
//	chart.Gauge(c, 0.72).Title("完成率").Show()
func Gauge(c *ui.Context, value float64) *GaugeW {
	w := &GaugeW{value: value, lo: 0, hi: 1}
	w.init(c, w, 260)
	return w
}

// Range sets the scale bounds (default 0..1, shown as percentages).
func (w *GaugeW) Range(lo, hi float64) *GaugeW { w.lo, w.hi = lo, hi; return w }

// Title sets the caption under the value.
func (w *GaugeW) Title(s string) *GaugeW { w.title = s; return w }

// Bands colors the track by thresholds (upper bounds in scale units, sorted)
// instead of a single progress arc.
func (w *GaugeW) Bands(stops []float64, colors ...theme.Color) *GaugeW {
	w.stops, w.stopCols = stops, colors
	return w
}

// Show builds the chart.
func (w *GaugeW) Show() View {
	f := w.begin()
	st, th := f.st, f.th
	f.kind(f.c.Locale().Chart.Gauge, a11y.RoleMeter)
	f.sem.name = w.title
	span := w.hi - w.lo
	if span <= 0 {
		span = 1
	}
	frac := math.Max(0, math.Min(1, (w.value-w.lo)/span))
	cur := f.tweenVals([]float64{frac}, true)[0]
	vf := w.format
	if vf == nil {
		if w.lo == 0 && w.hi == 1 {
			vf = Percent(0)
		} else {
			vf = Thousands(0)
		}
	}
	const sweep = 240 * math.Pi / 180
	start := math.Pi/2 + (2*math.Pi-sweep)/2 // bottom gap centred
	polarFor := func(r geom.Rect) Polar {
		titleH := float32(0)
		if w.title != "" {
			titleH = text.LineHeightOf(tstyle(th.FontSize, 0))
		}
		// Arc on top, value (≈0.42R tall) and title below the hub.
		R := min(r.W/2-4, (r.H-10-titleH)/1.72)
		R = max(R, 20)
		c := geom.Pt(r.X+r.W/2, r.Y+R+6)
		p := Polar{Center: c, Outer: R, Start: start, Sweep: sweep}
		if f.mirror {
			p = p.Mirrored() // from the lower right, counter-clockwise
		}
		return p
	}
	hit := func(r geom.Rect, p geom.Point) *hover {
		pl := polarFor(r)
		if _, d := pl.Locate(p); d > pl.Outer {
			return nil
		}
		name := w.title
		if name == "" {
			name = "value"
		}
		return &hover{anchor: p, tip: Tip{Items: []TipItem{{Name: name, Value: vf(w.value), Color: st.Color(0)}}}}
	}
	paintFn := func(l *paint.List, r geom.Rect, _ *hover) {
		pl := polarFor(r)
		cx, cy, R := pl.Center.X, pl.Center.Y, pl.Outer
		thick := max(8, R*0.1)
		r1, r0 := R, R-thick
		a0, sweep := pl.Start, pl.Sweep
		if len(w.stops) > 0 {
			prev := 0.0
			for i, s := range w.stops {
				t := math.Max(0, math.Min(1, (s-w.lo)/span))
				col := st.Color(i)
				if i < len(w.stopCols) {
					col = w.stopCols[i]
				}
				l.Path(ringSpan(cx, cy, r0, r1, a0+prev*sweep, a0+t*sweep), col)
				prev = t
			}
		} else {
			l.Path(ringSpan(cx, cy, r0, r1, a0, a0+sweep), st.Track)
			if cur > 0 {
				l.Path(ringSpan(cx, cy, r0, r1, a0, a0+cur*sweep), st.Color(0))
			}
		}
		// Ticks: 10 major, 5 minor each.
		ts := tstyle(st.Font, 0)
		for i := 0; i <= 50; i++ {
			t := float64(i) / 50
			a := a0 + t*sweep
			cos, sin := float32(math.Cos(a)), float32(math.Sin(a))
			ri := r0 - 4
			ln := float32(4)
			if i%5 == 0 {
				ln = 8
			}
			l.Line(geom.Pt(cx+ri*cos, cy+ri*sin), geom.Pt(cx+(ri-ln)*cos, cy+(ri-ln)*sin), 1, st.Tick)
			if i%10 == 0 {
				lr := ri - 8 - 14
				drawText(l, vf(w.lo+t*span), cx+lr*cos, cy+lr*sin, alignCenter, alignMiddle, ts, st.Label)
			}
		}
		// Needle.
		a := a0 + cur*sweep
		cos, sin := float32(math.Cos(a)), float32(math.Sin(a))
		nl := r0 - 14
		px, py := -sin, cos
		needle := paint.NewPath().MoveTo(cx+px*3, cy+py*3).LineTo(cx+nl*cos, cy+nl*sin).LineTo(cx-px*3, cy-py*3).Close()
		ncol := st.Title
		l.Path(needle, ncol)
		l.Path(paint.NewPath().Circle(cx, cy, 7), ncol)
		l.Path(paint.NewPath().Circle(cx, cy, 3.5), st.Surface)
		// Value and title.
		vs := tstyle(min(30, R*0.28), text.Medium)
		y := cy + R*0.3
		drawText(l, vf(w.lo+cur*span), cx, y, alignCenter, alignTop, vs, th.TextHeading)
		if w.title != "" {
			drawTextDir(l, w.title, cx, y+text.LineHeightOf(vs), alignCenter, alignTop, tstyle(th.FontSize, 0), st.Title, f.para)
		}
	}
	if f.on() {
		f.sem.ranged, f.sem.now, f.sem.lo, f.sem.hi, f.sem.value = true, w.value, w.lo, w.hi, vf(w.value)
	}
	return f.finish(plotSpec{legendPos: LegendNone, hit: hit, paint: paintFn, tooltip: !w.noTooltip, loading: w.loading})
}
