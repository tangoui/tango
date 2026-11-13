package chart

import "math"

// LTTB downsamples the series (xs[i], ys[i]) to at most threshold points
// with the Largest-Triangle-Three-Buckets algorithm (Steinarsson, 2013,
// "Downsampling Time Series for Visual Representation"). It returns the
// indices of the kept points in ascending order; the first and last points
// are always kept. NaN y values are skipped. With threshold < 3 or fewer
// points than threshold, every valid index is returned.
func LTTB(xs, ys []float64, threshold int) []int {
	n := min(len(xs), len(ys))
	idx := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if !math.IsNaN(ys[i]) {
			idx = append(idx, i)
		}
	}
	m := len(idx)
	if threshold < 3 || m <= threshold {
		return idx
	}
	out := make([]int, 0, threshold)
	out = append(out, idx[0])
	// The m-2 interior points are split into threshold-2 buckets.
	every := float64(m-2) / float64(threshold-2)
	a := 0 // position (in idx) of the last selected point
	for b := 0; b < threshold-2; b++ {
		// Current bucket [s, e).
		s := int(math.Floor(float64(b)*every)) + 1
		e := int(math.Floor(float64(b+1)*every)) + 1
		e = min(e, m-1)
		// Average of the next bucket (the last point for the final bucket).
		ns, ne := e, int(math.Floor(float64(b+2)*every))+1
		ne = min(ne, m)
		if ns >= ne {
			ns, ne = m-1, m
		}
		var avx, avy float64
		for j := ns; j < ne; j++ {
			avx += xs[idx[j]]
			avy += ys[idx[j]]
		}
		cnt := float64(ne - ns)
		avx /= cnt
		avy /= cnt
		ax, ay := xs[idx[a]], ys[idx[a]]
		best, bestArea := s, -1.0
		for j := s; j < e; j++ {
			px, py := xs[idx[j]], ys[idx[j]]
			// Twice the triangle area (a, p, avg); the factor is irrelevant.
			area := math.Abs((ax-avx)*(py-ay) - (ax-px)*(avy-ay))
			if area > bestArea {
				best, bestArea = j, area
			}
		}
		out = append(out, idx[best])
		a = best
	}
	out = append(out, idx[m-1])
	return out
}
