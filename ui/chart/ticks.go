package chart

import (
	"fmt"
	"math"
	"time"

	"github.com/tangoui/tango/ui"
)

// NiceStep returns a round tick step (1, 2 or 5 × 10^k) that divides span
// into about count intervals.
func NiceStep(span float64, count int) float64 {
	if count < 1 {
		count = 1
	}
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return 1
	}
	raw := span / float64(count)
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	f := raw / p
	switch {
	case f >= math.Sqrt(50):
		f = 10
	case f >= math.Sqrt(10):
		f = 5
	case f >= math.Sqrt(2):
		f = 2
	default:
		f = 1
	}
	return f * p
}

// NiceDomain extends [lo, hi] outward to multiples of a nice step with
// about count ticks. A zero span is widened around the value (0 becomes
// [0, 1]); NaN or infinite input yields [0, 1].
func NiceDomain(lo, hi float64, count int) (nlo, nhi, step float64) {
	if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		return 0, 1, NiceStep(1, count)
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		if lo == 0 {
			hi = 1
		} else {
			d := math.Abs(lo) * 0.5
			lo, hi = lo-d, hi+d
		}
	}
	step = NiceStep(hi-lo, count)
	for i := 0; i < 3; i++ {
		nlo = math.Floor(lo/step+1e-9) * step
		nhi = math.Ceil(hi/step-1e-9) * step
		s2 := NiceStep(nhi-nlo, count)
		if s2 == step {
			break
		}
		step = s2
	}
	return roundTo(nlo, step), roundTo(nhi, step), step
}

// NiceTicks returns the nice domain and its ticks for data in [lo, hi].
func NiceTicks(lo, hi float64, count int) []float64 {
	a, b, step := NiceDomain(lo, hi, count)
	return ticksIn(a, b, step)
}

func ticksIn(lo, hi, step float64) []float64 {
	if step <= 0 {
		return []float64{lo}
	}
	i0 := math.Ceil(lo/step - 1e-9)
	i1 := math.Floor(hi/step + 1e-9)
	if i1-i0 > 1000 {
		i1 = i0 + 1000
	}
	var out []float64
	for i := i0; i <= i1; i++ {
		out = append(out, roundTo(i*step, step))
	}
	return out
}

func roundTo(v, step float64) float64 {
	d := stepDecimals(step)
	p := math.Pow(10, float64(d))
	r := math.Round(v*p) / p
	if r == 0 {
		return 0 // no -0
	}
	return r
}

// Labels formats the calendar parts of time tick labels.
type Labels struct {
	Year  func(y int) string // "2026年" / "2026"
	Month func(m int) string // "2月" / "Feb"
}

// LabelsOf takes the time label texts from a ui locale.
func LabelsOf(l *ui.Locale) Labels {
	if l == nil {
		l = ui.ZhCN
	}
	return Labels{Year: l.Year, Month: l.Month}
}

func (lb Labels) year(y int) string {
	if lb.Year == nil {
		return fmt.Sprint(y)
	}
	return lb.Year(y)
}

func (lb Labels) month(m time.Month) string {
	if lb.Month == nil {
		return fmt.Sprintf("%02d", int(m))
	}
	return lb.Month(int(m))
}

// TimeUnit is a tick granularity.
type TimeUnit uint8

const (
	Second TimeUnit = iota
	Minute
	Hour
	Day
	Week
	Month
	Year
)

// TimeTick is one time tick with its label.
type TimeTick struct {
	T     time.Time
	Label string
	Major bool // a coarser boundary (new year, month or day)
}

type tInterval struct {
	unit TimeUnit
	n    int
	d    time.Duration // approximate length
}

var tIntervals = []tInterval{
	{Second, 1, time.Second}, {Second, 5, 5 * time.Second}, {Second, 15, 15 * time.Second}, {Second, 30, 30 * time.Second},
	{Minute, 1, time.Minute}, {Minute, 5, 5 * time.Minute}, {Minute, 15, 15 * time.Minute}, {Minute, 30, 30 * time.Minute},
	{Hour, 1, time.Hour}, {Hour, 3, 3 * time.Hour}, {Hour, 6, 6 * time.Hour}, {Hour, 12, 12 * time.Hour},
	{Day, 1, 24 * time.Hour}, {Day, 2, 48 * time.Hour}, {Week, 1, 7 * 24 * time.Hour},
	{Month, 1, 30 * 24 * time.Hour}, {Month, 3, 91 * 24 * time.Hour}, {Month, 6, 182 * 24 * time.Hour},
	{Year, 1, 365 * 24 * time.Hour},
}

// TimeInterval picks the granularity giving at most about count ticks over
// [t0, t1].
func TimeInterval(t0, t1 time.Time, count int) (TimeUnit, int) {
	if count < 1 {
		count = 1
	}
	span := t1.Sub(t0)
	if span < 0 {
		span = -span
	}
	for _, iv := range tIntervals {
		if span/iv.d <= time.Duration(count) {
			return iv.unit, iv.n
		}
	}
	years := span.Hours() / (24 * 365)
	return Year, max(1, int(NiceStep(years, count)))
}

// floorTime truncates t to the start of an n-unit interval.
func floorTime(t time.Time, u TimeUnit, n int) time.Time {
	y, mo, d := t.Date()
	loc := t.Location()
	switch u {
	case Second:
		s := t.Second() / n * n
		return time.Date(y, mo, d, t.Hour(), t.Minute(), s, 0, loc)
	case Minute:
		return time.Date(y, mo, d, t.Hour(), t.Minute()/n*n, 0, 0, loc)
	case Hour:
		return time.Date(y, mo, d, t.Hour()/n*n, 0, 0, 0, loc)
	case Day:
		return time.Date(y, mo, d, 0, 0, 0, 0, loc)
	case Week:
		wd := (int(t.Weekday()) + 6) % 7 // Monday = 0
		return time.Date(y, mo, d-wd, 0, 0, 0, 0, loc)
	case Month:
		m := (int(mo) - 1) / n * n
		return time.Date(y, time.Month(m+1), 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(y/n*n, 1, 1, 0, 0, 0, 0, loc)
	}
}

func addTime(t time.Time, u TimeUnit, n int) time.Time {
	switch u {
	case Second:
		return t.Add(time.Duration(n) * time.Second)
	case Minute:
		return t.Add(time.Duration(n) * time.Minute)
	case Hour:
		return t.Add(time.Duration(n) * time.Hour)
	case Day:
		return t.AddDate(0, 0, n)
	case Week:
		return t.AddDate(0, 0, 7*n)
	case Month:
		return t.AddDate(0, n, 0)
	default:
		return t.AddDate(n, 0, 0)
	}
}

// TimeTicks returns calendar-aligned ticks inside [t0, t1] at an automatic
// granularity. Labels are hierarchical: a tick that starts a new year shows
// the year, one that starts a new month (on day ticks) the month, and so on.
func TimeTicks(t0, t1 time.Time, count int, lb Labels) []TimeTick {
	if t1.Before(t0) {
		t0, t1 = t1, t0
	}
	if t0.Equal(t1) {
		return []TimeTick{{T: t0, Label: timeLabel(t0, Day, lb, true), Major: true}}
	}
	u, n := TimeInterval(t0, t1, count)
	t := floorTime(t0, u, n)
	if t.Before(t0) {
		t = addTime(t, u, n)
	}
	var out []TimeTick
	var prev time.Time
	for i := 0; !t.After(t1) && i < 500; i++ {
		major := i == 0 || boundary(prev, t, u)
		out = append(out, TimeTick{T: t, Label: timeLabel(t, u, lb, major), Major: major})
		prev = t
		t = addTime(t, u, n)
	}
	return out
}

func boundary(prev, t time.Time, u TimeUnit) bool {
	switch u {
	case Second, Minute, Hour:
		return prev.YearDay() != t.YearDay() || prev.Year() != t.Year()
	case Day, Week:
		return prev.Month() != t.Month() || prev.Year() != t.Year()
	default:
		return prev.Year() != t.Year()
	}
}

func timeLabel(t time.Time, u TimeUnit, lb Labels, major bool) string {
	switch u {
	case Second:
		if major && t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
			return t.Format("01-02")
		}
		return t.Format("15:04:05")
	case Minute, Hour:
		if major && t.Hour() == 0 && t.Minute() == 0 {
			return t.Format("01-02")
		}
		return t.Format("15:04")
	case Day, Week:
		if t.YearDay() == 1 {
			return lb.year(t.Year())
		}
		return t.Format("01-02")
	case Month:
		if t.Month() == 1 {
			return lb.year(t.Year())
		}
		return lb.month(t.Month())
	default:
		return lb.year(t.Year())
	}
}

// timeFormat formats a time for tooltips at the axis granularity.
func timeFormat(t time.Time, u TimeUnit) string {
	switch u {
	case Second:
		return t.Format("2006-01-02 15:04:05")
	case Minute, Hour:
		return t.Format("2006-01-02 15:04")
	case Month:
		return t.Format("2006-01")
	case Year:
		return t.Format("2006")
	default:
		return t.Format("2006-01-02")
	}
}
