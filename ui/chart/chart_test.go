package chart

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

func twoCities() []Row {
	var rows []Row
	for i, m := range []string{"1月", "2月", "3月", "4月"} {
		rows = append(rows, Row{"m": m, "city": "东京", "t": 10 + i}, Row{"m": m, "city": "伦敦", "t": 5 + i})
	}
	return rows
}

func TestLegendToggle(t *testing.T) {
	var v View
	d := ui.NewDriver(func(c *ui.Context) {
		v = Line(c, twoCities()).X("m").Y("t").Series("city").Show()
	}, ui.DriverOptions{Width: 600, Height: 400})
	d.Settle()
	if v.Hidden("伦敦") {
		t.Fatal("hidden before click")
	}
	click := func(s string) {
		r, ok := d.FindText(s)
		if !ok {
			t.Fatalf("legend item %q not found", s)
		}
		d.Click(r.Center())
	}
	click("伦敦")
	if !v.Hidden("伦敦") {
		t.Fatal("legend click did not hide the series")
	}
	// The tooltip now only lists the visible series.
	r := v.Rect()
	d.Move(geom.Pt(r.X+r.W*0.5, r.Y+r.H*0.6))
	d.Frame()
	tip, ok := v.Tooltip()
	if !ok || len(tip.Items) != 1 || tip.Items[0].Name != "东京" {
		t.Fatalf("tooltip after hide: %+v %v", tip, ok)
	}
	// Hiding the last visible series is refused.
	click("东京")
	if v.Hidden("东京") {
		t.Fatal("all series hidden")
	}
	// Clicking again shows it.
	click("伦敦")
	if v.Hidden("伦敦") {
		t.Fatal("second click did not show the series")
	}
}

func TestTooltipHitColumn(t *testing.T) {
	rows := []Row{{"k": "A", "v": 1}, {"k": "B", "v": 2}, {"k": "C", "v": 3}}
	var v View
	d := ui.NewDriver(func(c *ui.Context) {
		v = Column(c, rows).X("k").Y("v").Format(Unit(" 件", 0)).Show()
	}, ui.DriverOptions{Width: 600, Height: 400})
	d.Settle()
	if _, ok := v.Tooltip(); ok {
		t.Fatal("tooltip without hover")
	}
	r := v.Rect()
	d.Move(geom.Pt(r.X+r.W*0.55, r.Y+r.H*0.5))
	d.Frame()
	tip, ok := v.Tooltip()
	if !ok || tip.Title != "B" || tip.Index != 1 || len(tip.Items) != 1 || tip.Items[0].Value != "2 件" {
		t.Fatalf("tooltip %+v %v", tip, ok)
	}
	d.Move(geom.Pt(r.Right()-6, r.Y+r.H*0.5))
	d.Frame()
	if tip, _ := v.Tooltip(); tip.Title != "C" {
		t.Fatalf("right edge hit %q", tip.Title)
	}
	// Leaving the chart hides it.
	d.Move(geom.Pt(r.X+r.W*0.5, r.Bottom()+50))
	d.Frame()
	if _, ok := v.Tooltip(); ok {
		t.Fatal("tooltip outside the chart")
	}
}

func TestTooltipHitPieAndTimeLine(t *testing.T) {
	rows := []Row{{"n": "甲", "v": 3}, {"n": "乙", "v": 1}}
	var pie View
	d := ui.NewDriver(func(c *ui.Context) {
		pie = Pie(c, rows).Angle("v").Color("n").NoLabel().Legend(LegendNone).Show()
	}, ui.DriverOptions{Width: 400, Height: 400})
	d.Settle()
	r := pie.Rect()
	// 甲 covers 12 → 9 o'clock clockwise; a point right of the centre is in it.
	d.Move(geom.Pt(r.X+r.W/2+60, r.Y+r.H/2))
	d.Frame()
	if tip, ok := pie.Tooltip(); !ok || tip.Items[0].Name != "甲" || tip.Items[0].Value != "3（75.0%）" {
		t.Fatalf("pie tip %+v", tip)
	}

	day := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	var ts []Row
	for i := 0; i < 61; i++ {
		ts = append(ts, Row{"d": day.AddDate(0, 0, i), "v": i})
	}
	var line View
	d2 := ui.NewDriver(func(c *ui.Context) {
		line = Line(c, ts).X("d").Y("v").Show()
	}, ui.DriverOptions{Width: 600, Height: 300})
	d2.Settle()
	lr := line.Rect()
	d2.Move(geom.Pt(lr.Right()-5, lr.Y+lr.H/2))
	d2.Frame()
	if tip, ok := line.Tooltip(); !ok || tip.Title != "2026-01-30" || tip.Items[0].Value != "60" {
		t.Fatalf("time tip %+v", tip)
	}
}

func TestEmptyAndUpdateTween(t *testing.T) {
	data := []Row{{"k": "A", "v": 10}}
	var v View
	page := func(c *ui.Context) { v = Column(c, data).X("k").Y("v").Show() }
	d := ui.NewDriver(func(c *ui.Context) { Line(c, nil).Show() }, ui.DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if _, ok := d.FindText(ui.ZhCN.NoData); !ok {
		t.Fatal("empty state text missing")
	}
	d = ui.NewDriver(page, ui.DriverOptions{Width: 400, Height: 300, Animations: true})
	d.Settle()
	st := ui.State[chartState](d.Runner().Context(), v.ID(), nil)
	if st.tw.cur[0] != 10 {
		t.Fatalf("settled value %v", st.tw.cur)
	}
	data = []Row{{"k": "A", "v": 20}}
	d.Frame()
	d.Advance(updateDur / 2)
	d.Frame()
	mid := st.tw.cur[0]
	if !(mid > 10 && mid < 20) {
		t.Fatalf("value did not ease: %v", mid)
	}
	d.Settle()
	if st.tw.cur[0] != 20 {
		t.Fatalf("final %v", st.tw.cur[0])
	}
}

func bigRows(n int) []Row {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{"t": start.Add(time.Duration(i) * time.Minute), "v": math.Sin(float64(i)/50) * 100}
	}
	return rows
}

func TestLargeLineDownsampled(t *testing.T) {
	rows := bigRows(10000)
	d := ui.NewDriver(func(c *ui.Context) { Line(c, rows).X("t").Y("v").Show() }, ui.DriverOptions{Width: 800, Height: 400})
	d.Settle()
	start := time.Now()
	for i := 0; i < 5; i++ {
		d.Move(geom.Pt(float32(100+i*100), 200))
	}
	if el := time.Since(start) / 5; el > 200*time.Millisecond && !raceEnabled {
		t.Fatalf("10k-point frame too slow: %v", el)
	}
}

// BenchmarkLine10k measures a full frame (build, layout, paint, CPU
// raster) of a 10 000-point line with a moving tooltip.
func BenchmarkLine10k(b *testing.B) {
	rows := bigRows(10000)
	d := ui.NewDriver(func(c *ui.Context) { Line(c, rows).X("t").Y("v").Show() }, ui.DriverOptions{Width: 960, Height: 400})
	d.Settle()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Move(geom.Pt(float32(100+i%700), 200))
	}
}

// BenchmarkLTTB10k measures downsampling 10 000 points to 1 600.
func BenchmarkLTTB10k(b *testing.B) {
	xs := make([]float64, 10000)
	ys := make([]float64, 10000)
	for i := range xs {
		xs[i], ys[i] = float64(i), math.Sin(float64(i)/40)
	}
	for i := 0; i < b.N; i++ {
		LTTB(xs, ys, 1600)
	}
}
