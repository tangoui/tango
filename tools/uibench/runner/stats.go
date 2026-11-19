package main

import (
	"math"
	"slices"
)

func sorted(xs []float64) []float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s
}

// quantile is the q-quantile of sorted s, interpolated between the two
// nearest ranks (R's type 7, numpy's default).
func quantile(s []float64, q float64) float64 {
	if len(s) == 0 {
		return 0
	}
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := min(lo+1, len(s)-1)
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

func median(xs []float64) float64 { return quantile(sorted(xs), 0.5) }

// quartiles returns the first and third quartiles: the interquartile range
// the judge compares.
func quartiles(xs []float64) (q1, q3 float64) {
	s := sorted(xs)
	return quantile(s, 0.25), quantile(s, 0.75)
}

// percentile is the nearest-rank p-th percentile, a sample that occurred:
// the frame interval p99 is an interval some frame really took.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := sorted(xs)
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	return s[max(rank, 1)-1]
}

// each maps rounds to one metric.
func each(rs []Round, f func(Round) float64) []float64 {
	out := make([]float64, len(rs))
	for i, r := range rs {
		out[i] = f(r)
	}
	return out
}
