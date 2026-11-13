package chart

import (
	"math"
	"time"
)

// Linear maps the domain [D0, D1] onto the range [R0, R1].
type Linear struct {
	D0, D1 float64
	R0, R1 float64
}

// Map returns the range position of v.
func (s Linear) Map(v float64) float64 {
	if s.D1 == s.D0 {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + (v-s.D0)/(s.D1-s.D0)*(s.R1-s.R0)
}

// Invert returns the domain value at range position p.
func (s Linear) Invert(p float64) float64 {
	if s.R1 == s.R0 {
		return s.D0
	}
	return s.D0 + (p-s.R0)/(s.R1-s.R0)*(s.D1-s.D0)
}

// Ticks returns about count nice tick values inside the domain.
func (s Linear) Ticks(count int) []float64 {
	lo, hi := math.Min(s.D0, s.D1), math.Max(s.D0, s.D1)
	step := NiceStep(hi-lo, count)
	return ticksIn(lo, hi, step)
}

// Nice extends the domain outward to nice round values and returns the
// tick step.
func (s *Linear) Nice(count int) float64 {
	lo, hi, step := NiceDomain(s.D0, s.D1, count)
	s.D0, s.D1 = lo, hi
	return step
}

// Log maps a positive domain logarithmically onto the range.
type Log struct {
	D0, D1 float64 // > 0
	R0, R1 float64
	Base   float64 // 0 means 10
}

func (s Log) base() float64 {
	if s.Base <= 1 {
		return 10
	}
	return s.Base
}

func (s Log) lg(v float64) float64 {
	if v <= 0 {
		v = math.SmallestNonzeroFloat64
	}
	return math.Log(v) / math.Log(s.base())
}

// Map returns the range position of v (values <= 0 clamp to the low end).
func (s Log) Map(v float64) float64 {
	a, b := s.lg(s.D0), s.lg(s.D1)
	if a == b {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + (s.lg(math.Max(v, math.Min(s.D0, s.D1)))-a)/(b-a)*(s.R1-s.R0)
}

// Nice extends the domain to whole powers of the base.
func (s *Log) Nice() {
	b := s.base()
	lo, hi := math.Min(s.D0, s.D1), math.Max(s.D0, s.D1)
	if lo <= 0 {
		lo = math.Min(1, hi)
	}
	if hi <= 0 {
		hi = 1
	}
	s.D0 = math.Pow(b, math.Floor(s.lg(lo)))
	s.D1 = math.Pow(b, math.Ceil(s.lg(hi)))
	if s.D0 == s.D1 {
		s.D1 *= b
	}
}

// Ticks returns the powers of the base inside the domain (and 2×, 5× steps
// when the domain spans fewer than three decades).
func (s Log) Ticks() []float64 {
	b := s.base()
	lo, hi := math.Min(s.D0, s.D1), math.Max(s.D0, s.D1)
	if lo <= 0 || hi <= 0 {
		return nil
	}
	e0, e1 := math.Floor(s.lg(lo)), math.Ceil(s.lg(hi))
	var out []float64
	mult := []float64{1}
	if e1-e0 < 3 && b == 10 {
		mult = []float64{1, 2, 5}
	}
	for e := e0; e <= e1; e++ {
		for _, m := range mult {
			v := m * math.Pow(b, e)
			if v >= lo*(1-1e-9) && v <= hi*(1+1e-9) {
				out = append(out, v)
			}
		}
	}
	return out
}

// TimeScale maps a time domain onto the range.
type TimeScale struct {
	D0, D1 time.Time
	R0, R1 float64
}

// Map returns the range position of t.
func (s TimeScale) Map(t time.Time) float64 {
	return Linear{timeNum(s.D0), timeNum(s.D1), s.R0, s.R1}.Map(timeNum(t))
}

// Ticks returns about count calendar-aligned ticks with labels.
func (s TimeScale) Ticks(count int, loc Labels) []TimeTick {
	return TimeTicks(s.D0, s.D1, count, loc)
}

// Band divides the range into equal bands, one per category (bars).
type Band struct {
	N            int
	R0, R1       float64
	PaddingInner float64 // fraction of the step left empty between bands
	PaddingOuter float64 // fraction of the step before the first / after the last band
}

// Step returns the distance between band starts.
func (s Band) Step() float64 {
	n := float64(s.N)
	if n <= 0 {
		return 0
	}
	return (s.R1 - s.R0) / math.Max(1, n-s.PaddingInner+2*s.PaddingOuter)
}

// Bandwidth returns the width of one band.
func (s Band) Bandwidth() float64 { return s.Step() * (1 - s.PaddingInner) }

// Map returns the start of band i.
func (s Band) Map(i int) float64 { return s.R0 + s.Step()*(s.PaddingOuter+float64(i)) }

// Center returns the middle of band i.
func (s Band) Center(i int) float64 { return s.Map(i) + s.Bandwidth()/2 }

// Index returns the band whose step contains p (-1 outside).
func (s Band) Index(p float64) int {
	st := s.Step()
	if st == 0 || p < math.Min(s.R0, s.R1) || p > math.Max(s.R0, s.R1) {
		return -1
	}
	f := (p-s.R0)/st - s.PaddingOuter + s.PaddingInner/2
	i := int(math.Floor(f))
	if i < 0 || i >= s.N {
		return -1
	}
	return i
}

// Point places categories at evenly spaced points (lines over categories).
// Padding is the fraction of a step left before the first and after the
// last point: 0 puts them on the edges, 0.5 on band centres.
type Point struct {
	N       int
	R0, R1  float64
	Padding float64
}

func (s Point) step() float64 {
	d := float64(s.N-1) + 2*s.Padding
	if d <= 0 {
		return 0
	}
	return (s.R1 - s.R0) / d
}

// Map returns the position of point i.
func (s Point) Map(i int) float64 {
	if s.N == 1 && s.Padding == 0 {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + s.step()*(s.Padding+float64(i))
}

// Index returns the point nearest to p.
func (s Point) Index(p float64) int {
	if s.N <= 0 {
		return -1
	}
	st := s.step()
	if st == 0 {
		return 0
	}
	i := int(math.Round((p-s.R0)/st - s.Padding))
	return max(0, min(s.N-1, i))
}
