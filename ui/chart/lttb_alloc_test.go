package chart

import (
	"math"
	"reflect"
	"testing"
)

func TestLTTBDenseInputDoesNotAllocateFullIndex(t *testing.T) {
	xs, ys := make([]float64, 10000), make([]float64, 10000)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = math.Sin(float64(i) / 7)
	}
	var got []int
	allocs := testing.AllocsPerRun(20, func() { got = LTTB(xs, ys, 1000) })
	if len(got) != 1000 || got[0] != 0 || got[len(got)-1] != 9999 {
		t.Fatalf("bad sampling endpoints: %v", got)
	}
	if allocs > 1 {
		t.Fatalf("dense input allocates %.0f times; want result allocation only", allocs)
	}
}

func TestLTTBMatchesReferenceDenseAndSparse(t *testing.T) {
	for n := 0; n < 130; n++ {
		xs, ys := make([]float64, n), make([]float64, n)
		for i := range xs {
			xs[i] = float64(i)
			ys[i] = math.Sin(float64(i) * 1.73)
		}
		for sparse := 0; sparse < 3; sparse++ {
			if sparse == 1 {
				for i := 3; i < n; i += 7 {
					ys[i] = math.NaN()
				}
			}
			if sparse == 2 {
				for i := range ys {
					ys[i] = math.NaN()
				}
			}
			for _, threshold := range []int{-1, 0, 2, 3, 4, 9, n / 2, n, n + 1} {
				got, want := LTTB(xs, ys, threshold), referenceLTTB(xs, ys, threshold)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("n=%d sparse=%d threshold=%d got=%v want=%v", n, sparse, threshold, got, want)
				}
			}
		}
	}
}
