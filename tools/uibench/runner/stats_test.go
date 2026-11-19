package main

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestMedianIQR(t *testing.T) {
	for _, tc := range []struct {
		name        string
		xs          []float64
		med, q1, q3 float64
	}{
		{"odd", []float64{5, 1, 3, 2, 4}, 3, 2, 4},
		{"even", []float64{6, 1, 5, 2, 4, 3}, 3.5, 2.25, 4.75},
	} {
		if got := median(tc.xs); !near(got, tc.med) {
			t.Errorf("%s: median %v, want %v", tc.name, got, tc.med)
		}
		q1, q3 := quartiles(tc.xs)
		if !near(q1, tc.q1) || !near(q3, tc.q3) {
			t.Errorf("%s: quartiles %v,%v, want %v,%v", tc.name, q1, q3, tc.q1, tc.q3)
		}
	}
	// The input is not reordered.
	xs := []float64{3, 1, 2}
	median(xs)
	if xs[0] != 3 || xs[1] != 1 {
		t.Errorf("median sorted its input: %v", xs)
	}
}

func TestP99(t *testing.T) {
	xs := make([]float64, 1000)
	for i := range xs {
		xs[i] = float64(1000 - i) // 1000 … 1, unsorted
	}
	if got := percentile(xs, 99); got != 990 {
		t.Errorf("p99 of 1..1000 = %v, want 990", got)
	}
	if got := percentile(xs, 50); got != 500 {
		t.Errorf("p50 of 1..1000 = %v, want 500", got)
	}
	if got := percentile(nil, 99); got != 0 {
		t.Errorf("p99 of nothing = %v, want 0", got)
	}
}
