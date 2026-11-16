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
	// Dense inputs use their original indices directly. Only build a mapping
	// when NaN values actually make the input sparse.
	var idx []int
	for i := 0; i < n; i++ {
		if math.IsNaN(ys[i]) {
			if idx == nil {
				idx = make([]int, i, n)
				for j := range idx {
					idx[j] = j
				}
			}
		} else if idx != nil {
			idx = append(idx, i)
		}
	}
	m := n
	if idx != nil {
		m = len(idx)
	}
	if threshold < 3 || m <= threshold {
		if idx == nil {
			idx = make([]int, m)
			for i := range idx {
				idx[i] = i
			}
		}
		return idx
	}
	out := make([]int, 0, threshold)
	out = append(out, lttbIndex(idx, 0))
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
		if idx == nil {
			for j := ns; j < ne; j++ {
				avx += xs[j]
				avy += ys[j]
			}
		} else {
			for j := ns; j < ne; j++ {
				avx += xs[idx[j]]
				avy += ys[idx[j]]
			}
		}
		cnt := float64(ne - ns)
		avx /= cnt
		avy /= cnt
		ax, ay := xs[lttbIndex(idx, a)], ys[lttbIndex(idx, a)]
		best, bestArea := s, -1.0
		if idx == nil {
			for j := s; j < e; j++ {
				px, py := xs[j], ys[j]
				area := math.Abs((ax-avx)*(py-ay) - (ax-px)*(avy-ay))
				if area > bestArea {
					best, bestArea = j, area
				}
			}
		} else {
			for j := s; j < e; j++ {
				px, py := xs[idx[j]], ys[idx[j]]
				area := math.Abs((ax-avx)*(py-ay) - (ax-px)*(avy-ay))
				if area > bestArea {
					best, bestArea = j, area
				}
			}
		}
		out = append(out, lttbIndex(idx, best))
		a = best
	}
	out = append(out, lttbIndex(idx, m-1))
	return out
}

// lttbIndex is inlined; nil is the identity mapping for dense series.
func lttbIndex(idx []int, i int) int {
	if idx == nil {
		return i
	}
	return idx[i]
}
