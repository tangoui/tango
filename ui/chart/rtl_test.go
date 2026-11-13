package chart

import (
	"image"
	"math"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// renderIn renders page in a scope of direction dir and returns the
// driver after settling.
func renderIn(dir ui.Direction, page func(c *ui.Context)) *ui.Driver {
	d := ui.NewDriver(func(c *ui.Context) {
		ui.ConfigProvider(c).Direction(dir).Body(func() { page(c) })
	}, ui.DriverOptions{Width: 600, Height: 400})
	d.Settle()
	return d
}

// sameRows reports whether a and b have identical pixels in rows y0..y1.
func sameRows(a, b *image.RGBA, y0, y1 int) (int, bool) {
	diff := 0
	r := a.Bounds()
	for y := max(y0, r.Min.Y); y < min(y1, r.Max.Y); y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				diff++
			}
		}
	}
	return diff, diff == 0
}

// TestRTLDefaultPlotLTR: in an RTL scope the plot is drawn exactly as in
// LTR (time / category axes still run left to right) while the legend
// mirrors to the other side.
func TestRTLDefaultPlotLTR(t *testing.T) {
	var v View
	page := func(c *ui.Context) {
		v = Line(c, twoCities()).X("m").Y("t").Series("city").Legend(LegendTop).Show()
	}
	dl := renderIn(ui.LTR, page)
	imgL := cloneRGBA(dl.Image())
	lgL, okL := dl.FindText("东京")
	vr := v.Rect()
	dr := renderIn(ui.RTL, page)
	imgR := dr.Image()
	lgR, okR := dr.FindText("东京")
	if !okL || !okR {
		t.Fatal("legend item not found")
	}
	// Legend mirrored: items flow from the inline start.
	mid := vr.X + vr.W/2
	if lgL.Center().X >= mid || lgR.Center().X <= mid {
		t.Fatalf("legend not mirrored: ltr %v rtl %v (mid %v)", lgL, lgR, mid)
	}
	// Plot pixels (below the legend row) identical.
	y0 := int(math.Ceil(float64(max(lgL.Bottom(), lgR.Bottom())))) + 6
	if n, ok := sameRows(imgL, imgR, y0, int(vr.Bottom())); !ok {
		t.Fatalf("plot differs between LTR and RTL: %d px", n)
	}
	// Hit testing follows the LTR geometry: same tooltip at the same point.
	p := geom.Pt(vr.X+vr.W*0.3, vr.Y+vr.H*0.6)
	dl.Move(p)
	dl.Frame()
	tipL, _ := v.Tooltip()
	dr.Move(p)
	dr.Frame()
	tipR, ok := v.Tooltip()
	if !ok || tipR.Title != tipL.Title {
		t.Fatalf("rtl tooltip %+v, ltr %+v", tipR, tipL)
	}
}

func cloneRGBA(m *image.RGBA) *image.RGBA {
	c := image.NewRGBA(m.Bounds())
	copy(c.Pix, m.Pix)
	return c
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

// TestCartesianMirror: x' = W − x inside the plot, and Norm inverts At.
func TestCartesianMirror(t *testing.T) {
	plot := geom.Rect{X: 40, Y: 10, W: 500, H: 300}
	ltr := Cartesian{Plot: plot}
	rtl := Cartesian{Plot: plot, Mirror: true}
	for _, nx := range []float64{0, 0.25, 0.5, 0.9, 1} {
		a, b := ltr.At(nx, 0.3), rtl.At(nx, 0.3)
		if !near(b.X-plot.X, plot.W-(a.X-plot.X)) || a.Y != b.Y {
			t.Fatalf("nx %v: ltr %v mirrored %v", nx, a, b)
		}
		gx, gy := rtl.Norm(b)
		if math.Abs(gx-nx) > 1e-5 || math.Abs(gy-0.3) > 1e-5 {
			t.Fatalf("Norm(%v) = %v, %v", b, gx, gy)
		}
	}
	// Transposed: the value axis runs right to left.
	tr := Cartesian{Plot: plot, Transposed: true, Mirror: true}
	if p := tr.At(0.5, 0); !near(p.X, plot.Right()) {
		t.Fatalf("transposed zero at %v", p)
	}
}

// TestMirrorCartLayout: the mirrored plot rect is the mirror image of the
// LTR one, and the value axis labels sit right of the plot.
func TestMirrorCartLayout(t *testing.T) {
	st := StyleOf(theme.AntDesign())
	r := geom.Rect{X: 10, Y: 0, W: 600, H: 300}
	mk := func(mirror bool) (*cart, *paint.List) {
		f := &frame{mirror: mirror}
		x := pointAxis([]string{"A", "B", "C", "D"}, 0)
		y := valueAxis(0, 1000, 5, nil)
		cc := f.layoutCart(r, x, y, nil, false, st)
		l := &paint.List{}
		cc.drawAxes(l)
		return cc, l
	}
	cl, ll := mk(false)
	cm, lm := mk(true)
	if !near(cm.Plot.X-r.X, r.Right()-cl.Plot.Right()) || cm.Plot.W != cl.Plot.W {
		t.Fatalf("plot ltr %v mirrored %v", cl.Plot, cm.Plot)
	}
	// Category positions mirrored: first category at the right edge.
	if p0, q0 := cl.At(cl.x.norm(0), 0), cm.At(cm.x.norm(0), 0); !near(q0.X-r.X, r.W-(p0.X-r.X)) {
		t.Fatalf("first category ltr %v mirrored %v", p0, q0)
	}
	// Value labels (left of the LTR plot, right of the mirrored one).
	side := func(l *paint.List, cc *cart) (left, right int) {
		for _, op := range l.Ops {
			if tx, ok := op.(paint.Text); ok && tx.Origin.Y < cc.Plot.Bottom()-2 {
				if tx.Origin.X < cc.Plot.X {
					left++
				} else if tx.Origin.X >= cc.Plot.Right() {
					right++
				}
			}
		}
		return
	}
	if l, rr := side(ll, cl); l == 0 || rr != 0 {
		t.Fatalf("ltr value labels left %d right %d", l, rr)
	}
	if l, rr := side(lm, cm); l != 0 || rr == 0 {
		t.Fatalf("mirrored value labels left %d right %d", l, rr)
	}
}

// TestMirrorHit: hovering the mirrored point of a mirrored chart hits the
// same category as the LTR chart.
func TestMirrorHit(t *testing.T) {
	rows := []Row{{"k": "A", "v": 1}, {"k": "B", "v": 2}, {"k": "C", "v": 3}, {"k": "D", "v": 4}}
	for _, kind := range []string{"column", "line", "tiny"} {
		var v View
		mirror := false
		d := ui.NewDriver(func(c *ui.Context) {
			switch kind {
			case "column":
				w := Column(c, rows).X("k").Y("v")
				if mirror {
					w.Mirror()
				}
				v = w.Show()
			case "line":
				w := Line(c, rows).X("k").Y("v")
				if mirror {
					w.Mirror()
				}
				v = w.Show()
			default:
				w := TinyColumn(c, []float64{1, 2, 3, 4})
				if mirror {
					w.Mirror()
				}
				v = w.Show()
			}
		}, ui.DriverOptions{Width: 600, Height: 400})
		d.Settle()
		r := v.Rect()
		p := geom.Pt(r.X+r.W*0.2, r.Y+r.H*0.5)
		d.Move(p)
		d.Frame()
		tl, ok := v.Tooltip()
		if !ok {
			t.Fatalf("%s: no ltr tooltip", kind)
		}
		mirror = true
		d.Settle()
		d.Move(geom.Pt(2*r.X+r.W-p.X, p.Y))
		d.Frame()
		tm, ok := v.Tooltip()
		if !ok || tm.Index != tl.Index {
			t.Fatalf("%s: mirrored hit %+v, ltr %+v", kind, tm, tl)
		}
		d.Move(p)
		d.Frame()
		if tm, _ := v.Tooltip(); tm.Index == tl.Index {
			t.Fatalf("%s: mirrored chart not mirrored (index %d)", kind, tm.Index)
		}
	}
}

// TestPolarMirror: mirrored polar coordinates sweep counter-clockwise from
// the mirrored start angle, and Locate inverts At.
func TestPolarMirror(t *testing.T) {
	p := NewPolar(geom.Rect{W: 200, H: 200}, 0)
	m := p.Mirrored()
	// A quarter of the way round: 3 o'clock clockwise, 9 o'clock mirrored.
	a, b := p.At(0.25, 1), m.At(0.25, 1)
	if !near(a.X, 200) || !near(b.X, 0) || !near(a.Y, b.Y) {
		t.Fatalf("quarter: ltr %v mirrored %v", a, b)
	}
	for _, tt := range []float64{0.1, 0.4, 0.7} {
		q := m.At(tt, 0.5)
		got, _ := m.Locate(q)
		if math.Abs(got-tt) > 1e-5 {
			t.Fatalf("Locate(%v) = %v want %v", q, got, tt)
		}
		// x' = W − x around the centre.
		if o := p.At(tt, 0.5); !near(q.X, 200-o.X) || !near(q.Y, o.Y) {
			t.Fatalf("t %v: %v vs %v", tt, o, q)
		}
	}
	// Gauge: start mirrored (θ → π − θ).
	g := Polar{Start: math.Pi * 5 / 6, Sweep: math.Pi * 4 / 3}.Mirrored()
	if math.Abs(g.Start-math.Pi/6) > 1e-9 || g.Sweep >= 0 {
		t.Fatalf("gauge mirrored %+v", g)
	}
}

// TestPieMirrorSweep: the first slice of a mirrored pie lies left of 12
// o'clock (counter-clockwise), and hovering it there finds it.
func TestPieMirrorSweep(t *testing.T) {
	rows := []Row{{"t": "a", "v": 1}, {"t": "b", "v": 3}}
	for _, mirror := range []bool{false, true} {
		var v View
		d := ui.NewDriver(func(c *ui.Context) {
			w := Pie(c, rows).Angle("v").Color("t").NoLabel().Legend(LegendNone)
			if mirror {
				w.Mirror()
			}
			v = w.Show()
		}, ui.DriverOptions{Width: 400, Height: 400})
		d.Settle()
		r := v.Rect()
		c := r.Center()
		R := min(r.W, r.H)/2 - 4
		// Slice "a" spans the first quarter: probe at 45° from 12 o'clock.
		dx := R * 0.5 * float32(math.Sin(math.Pi/4))
		dy := -R * 0.5 * float32(math.Cos(math.Pi/4))
		if mirror {
			dx = -dx
		}
		d.Move(geom.Pt(c.X+dx, c.Y+dy))
		d.Frame()
		tip, ok := v.Tooltip()
		if !ok || tip.Items[0].Name != "a" {
			t.Fatalf("mirror=%v: tooltip %+v", mirror, tip)
		}
		d.Move(geom.Pt(c.X-dx, c.Y+dy))
		d.Frame()
		if tip, ok := v.Tooltip(); !ok || tip.Items[0].Name != "b" {
			t.Fatalf("mirror=%v: other side %+v", mirror, tip)
		}
	}
}
