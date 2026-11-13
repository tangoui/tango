package chart

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
)

func TestNiceDomainBasic(t *testing.T) {
	lo, hi, step := NiceDomain(3, 97, 5)
	if lo != 0 || hi != 100 || step != 20 {
		t.Fatalf("got %v %v %v", lo, hi, step)
	}
	ticks := NiceTicks(3, 97, 5)
	if len(ticks) != 6 || ticks[0] != 0 || ticks[5] != 100 {
		t.Fatalf("ticks %v", ticks)
	}
}

func TestNiceDomainAllNegative(t *testing.T) {
	lo, hi, step := NiceDomain(-87, -12, 5)
	if lo > -87 || hi < -12 || hi > 0 {
		t.Fatalf("domain [%v,%v] does not cover data or crosses zero", lo, hi)
	}
	for _, v := range ticksIn(lo, hi, step) {
		if v > 0 {
			t.Fatalf("positive tick %v", v)
		}
	}
	// With the zero baseline used by bars the domain ends at 0.
	a := valueAxis(-87, 0, 5, nil)
	if a.lin.D1 != 0 || a.lin.D0 > -87 {
		t.Fatalf("bar domain %v", a.lin)
	}
	if a.ticks[len(a.ticks)-1].label != "0" || !strings.HasPrefix(a.ticks[0].label, "-") {
		t.Fatalf("labels %v", a.ticks)
	}
}

func TestNiceDomainSinglePointAndZeroSpan(t *testing.T) {
	for _, v := range []float64{5, -5, 0, 1e-7, 123456} {
		lo, hi, step := NiceDomain(v, v, 5)
		if !(lo < hi) || step <= 0 {
			t.Fatalf("%v: degenerate domain [%v,%v] step %v", v, lo, hi, step)
		}
		if v < lo || v > hi {
			t.Fatalf("%v outside [%v,%v]", v, lo, hi)
		}
	}
	lo, hi, _ := NiceDomain(0, 0, 5)
	if lo != 0 || hi != 1 {
		t.Fatalf("zero span: [%v,%v]", lo, hi)
	}
	lo, hi, _ = NiceDomain(math.NaN(), 3, 5)
	if lo != 0 || hi != 1 {
		t.Fatalf("NaN: [%v,%v]", lo, hi)
	}
}

func TestTicksNoFloatNoise(t *testing.T) {
	for _, v := range NiceTicks(0, 0.7, 7) {
		s := FormatNumber(v, -1)
		if len(s) > 4 {
			t.Fatalf("noisy tick %q", s)
		}
	}
}

func TestLinearLogBandPoint(t *testing.T) {
	l := Linear{0, 10, 100, 200}
	if l.Map(5) != 150 || l.Invert(150) != 5 {
		t.Fatal("linear")
	}
	lg := Log{D0: 3, D1: 870, R0: 0, R1: 1}
	lg.Nice()
	if lg.D0 != 1 || lg.D1 != 1000 {
		t.Fatalf("log nice %v %v", lg.D0, lg.D1)
	}
	if math.Abs(lg.Map(10)-1.0/3) > 1e-9 || lg.Map(-5) != 0 {
		t.Fatalf("log map %v", lg.Map(10))
	}
	if ts := lg.Ticks(); ts[0] != 1 || ts[len(ts)-1] != 1000 {
		t.Fatalf("log ticks %v", ts)
	}
	b := Band{N: 4, R0: 0, R1: 100, PaddingInner: 0.2}
	if math.Abs(b.Step()-100/3.8) > 1e-9 || b.Index(b.Center(2)) != 2 || b.Index(-1) != -1 {
		t.Fatalf("band step %v", b.Step())
	}
	p := Point{N: 5, R0: 0, R1: 100}
	if p.Map(0) != 0 || p.Map(4) != 100 || p.Index(60) != 2 {
		t.Fatal("point")
	}
	if (Point{N: 1, R0: 0, R1: 100}).Map(0) != 50 {
		t.Fatal("single point not centred")
	}
}

func TestTimeTicksAcrossYear(t *testing.T) {
	t0 := time.Date(2025, 10, 15, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	ticks := TimeTicks(t0, t1, 6, LabelsOf(ui.ZhCN))
	if len(ticks) < 3 {
		t.Fatalf("too few ticks %v", ticks)
	}
	found := false
	for i, tk := range ticks {
		if tk.T.Before(t0) || tk.T.After(t1) {
			t.Fatalf("tick %v outside range", tk.T)
		}
		if tk.T.Day() != 1 {
			t.Fatalf("month tick not on the 1st: %v", tk.T)
		}
		if tk.T.Year() == 2026 && tk.T.Month() == 1 {
			found = true
			if tk.Label != "2026年" || !tk.Major {
				t.Fatalf("year boundary label %q major=%v", tk.Label, tk.Major)
			}
		}
		if i > 0 && !tk.T.After(ticks[i-1].T) {
			t.Fatal("ticks not increasing")
		}
	}
	if !found {
		t.Fatalf("no January 2026 tick: %v", ticks)
	}
	en := TimeTicks(t0, t1, 6, LabelsOf(ui.EnUS))
	if en[0].Label != "Nov" {
		t.Fatalf("english month label %q", en[0].Label)
	}
}

func TestTimeIntervalGranularity(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		span time.Duration
		unit TimeUnit
	}{
		{30 * time.Second, Second}, {40 * time.Minute, Minute}, {10 * time.Hour, Hour},
		{9 * 24 * time.Hour, Day}, {200 * 24 * time.Hour, Month}, {12 * 365 * 24 * time.Hour, Year},
	}
	for _, c := range cases {
		if u, _ := TimeInterval(base, base.Add(c.span), 6); u != c.unit {
			t.Errorf("span %v: unit %v, want %v", c.span, u, c.unit)
		}
	}
	// Hour ticks crossing midnight label the new day.
	ticks := TimeTicks(base.Add(18*time.Hour), base.Add(30*time.Hour), 6, LabelsOf(nil))
	day := false
	for _, tk := range ticks {
		if tk.Label == "01-02" {
			day = true
		}
	}
	if !day {
		t.Fatalf("midnight not labelled: %v", ticks)
	}
}

func TestFormat(t *testing.T) {
	cases := map[string]string{
		FormatNumber(1234567.891, 2):   "1,234,567.89",
		FormatNumber(-1234, 0):         "-1,234",
		FormatNumber(-0.0001, 2):       "0.00",
		FormatNumber(999, 0):           "999",
		FormatCompact(123456, "zh-CN"): "12.3万",
		FormatCompact(3.2e8, "zh-CN"):  "3.2亿",
		FormatCompact(1500, "en-US"):   "1.5K",
		FormatCompact(3.2e8, "zh-TW"):  "3.2億",
		FormatCompact(123456, "ja"):    "12.3万",
		FormatCompact(2e6, "en-US"):    "2M",
		FormatCompact(950, "en-US"):    "950",
		Percent(1)(0.256):              "25.6%",
		Unit("°C", 1)(21.5):            "21.5°C",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}

func TestStack(t *testing.T) {
	nan := math.NaN()
	vals := [][]float64{{1, 2, -1}, {3, nan, -2}, {4, 1, 5}}
	lo, hi := Stack(vals, nil, false)
	want := [][2]float64{{0, 1}, {1, 4}, {4, 8}}
	for s, w := range want {
		if lo[s][0] != w[0] || hi[s][0] != w[1] {
			t.Fatalf("s%d x0: [%v,%v] want %v", s, lo[s][0], hi[s][0], w)
		}
	}
	// NaN contributes nothing.
	if hi[2][1] != 3 || lo[2][1] != 2 {
		t.Fatalf("nan column: %v %v", lo[2][1], hi[2][1])
	}
	// Negatives stack downward, positives upward from 0.
	if lo[0][2] != -1 || hi[0][2] != 0 || lo[1][2] != -3 || hi[1][2] != -1 || lo[2][2] != 0 || hi[2][2] != 5 {
		t.Fatalf("mixed signs: %v %v", lo, hi)
	}
	// Hidden series vanish; percent sums to 1.
	lo, hi = Stack(vals, []bool{false, true, false}, true)
	if hi[1][0]-lo[1][0] != 0 {
		t.Fatal("hidden series has height")
	}
	if math.Abs(hi[2][0]-1) > 1e-12 || math.Abs(hi[0][0]-0.2) > 1e-12 {
		t.Fatalf("percent: %v", hi)
	}
	// An all-zero column stays at zero.
	lo, hi = Stack([][]float64{{0}, {0}}, nil, true)
	if hi[1][0] != 0 || lo[1][0] != 0 {
		t.Fatal("zero column")
	}
}

func TestLTTB(t *testing.T) {
	n := 10000
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = math.Sin(float64(i) / 300)
	}
	ys[4321] = 50 // a spike must survive
	ys[777] = math.NaN()
	idx := LTTB(xs, ys, 500)
	if len(idx) != 500 || idx[0] != 0 || idx[len(idx)-1] != n-1 {
		t.Fatalf("len %d first %d last %d", len(idx), idx[0], idx[len(idx)-1])
	}
	spike := false
	for k, i := range idx {
		if k > 0 && i <= idx[k-1] {
			t.Fatal("indices not increasing")
		}
		if i == 777 {
			t.Fatal("NaN point selected")
		}
		if i == 4321 {
			spike = true
		}
	}
	if !spike {
		t.Fatal("LTTB dropped the spike")
	}
	if got := LTTB(xs[:10], ys[:10], 50); len(got) != 10 {
		t.Fatal("small input not returned whole")
	}
	if got := LTTB(nil, nil, 50); len(got) != 0 {
		t.Fatal("empty input")
	}
}

func TestBuildGrid(t *testing.T) {
	rows := []Row{
		{"m": "Jan", "s": "a", "v": 1}, {"m": "Feb", "s": "a", "v": int64(2)},
		{"m": "Jan", "s": "b", "v": float32(3)}, {"m": "Mar", "s": "b", "v": nil},
	}
	g := buildGrid(rows, "m", "v", "s")
	if g.kind != kindCat || len(g.cats) != 3 || len(g.names) != 2 {
		t.Fatalf("grid %+v", g)
	}
	if g.vals[0][1] != 2 || g.vals[1][0] != 3 || !math.IsNaN(g.vals[1][2]) {
		t.Fatalf("vals %v", g.vals)
	}
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tg := buildGrid([]Row{{"t": day.AddDate(0, 0, 2), "v": 3}, {"t": day, "v": 1}}, "t", "v", "")
	if tg.kind != kindTime || tg.vals[0][0] != 1 || tg.xs[0] >= tg.xs[1] {
		t.Fatalf("time grid not sorted: %+v", tg)
	}
}

func TestPolarAndCartesian(t *testing.T) {
	c := Cartesian{Plot: geomRect(10, 20, 100, 50)}
	p := c.At(0.5, 1)
	if p.X != 60 || p.Y != 20 {
		t.Fatalf("at %v", p)
	}
	if nx, ny := c.Norm(p); math.Abs(nx-0.5) > 1e-6 || math.Abs(ny-1) > 1e-6 {
		t.Fatal("norm")
	}
	pl := NewPolar(geomRect(0, 0, 100, 100), 0)
	top := pl.At(0, 1)
	if math.Abs(float64(top.X-50)) > 1e-3 || math.Abs(float64(top.Y)) > 1e-3 {
		t.Fatalf("12 o'clock %v", top)
	}
	if tt, d := pl.Locate(pl.At(0.25, 1)); math.Abs(tt-0.25) > 1e-6 || math.Abs(float64(d-50)) > 1e-3 {
		t.Fatalf("locate %v %v", tt, d)
	}
}
