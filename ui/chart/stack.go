package chart

import "math"

// Stack stacks series values per x position. vals[s][i] is the value of
// series s at x i (NaN = missing, treated as 0); hidden series contribute
// nothing. Positive values stack upward from 0 and negative values downward,
// so mixed signs never overlap. With percent each column is normalised so
// the absolute values sum to 1 (positives to the share above 0, negatives
// below). It returns the lower and upper bound of every segment.
func Stack(vals [][]float64, hidden []bool, percent bool) (lo, hi [][]float64) {
	ns := len(vals)
	nx := 0
	for _, v := range vals {
		nx = max(nx, len(v))
	}
	lo = make([][]float64, ns)
	hi = make([][]float64, ns)
	for s := range vals {
		lo[s] = make([]float64, nx)
		hi[s] = make([]float64, nx)
	}
	for i := 0; i < nx; i++ {
		total := 0.0
		if percent {
			for s, v := range vals {
				if i < len(v) && !isHidden(hidden, s) && !math.IsNaN(v[i]) {
					total += math.Abs(v[i])
				}
			}
		}
		pos, neg := 0.0, 0.0
		for s, v := range vals {
			x := 0.0
			if i < len(v) && !isHidden(hidden, s) && !math.IsNaN(v[i]) {
				x = v[i]
			}
			if percent {
				if total > 0 {
					x /= total
				} else {
					x = 0
				}
			}
			if x >= 0 {
				lo[s][i], hi[s][i] = pos, pos+x
				pos += x
			} else {
				lo[s][i], hi[s][i] = neg+x, neg
				neg += x
			}
		}
	}
	return lo, hi
}

func isHidden(h []bool, i int) bool { return i < len(h) && h[i] }
