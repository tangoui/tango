package diff

// dpDiff aligns a and b with a dynamic-programming LCS that prefers long
// diagonals, which gives nicer alignments than Myers on small inputs. score,
// if not nil, weighs the match of a[i] and b[j]. The cost is O(N*M) time and
// memory, so callers only use it below a size threshold.
func dpDiff(a, b []int32, bud *budget, score func(i, j int) float64) ([]seqDiff, bool) {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return trivial(n, m), false
	}
	const (
		fromA    = 1 // skip an element of a
		fromB    = 2 // skip an element of b
		diagonal = 3
	)
	lcs := make([]float64, n*m)
	dirs := make([]uint8, n*m)
	runs := make([]int32, n*m) // length of the diagonal run ending here
	at := func(i, j int) int { return i*m + j }
	for i := 0; i < n; i++ {
		if !bud.ok() {
			return trivial(n, m), true
		}
		for j := 0; j < m; j++ {
			horizontal, vertical := 0.0, 0.0
			if i > 0 {
				horizontal = lcs[at(i-1, j)]
			}
			if j > 0 {
				vertical = lcs[at(i, j-1)]
			}
			extended := -1.0
			if a[i] == b[j] {
				extended = 0
				if i > 0 && j > 0 {
					extended = lcs[at(i-1, j-1)]
					if dirs[at(i-1, j-1)] == diagonal {
						// Prefer consecutive diagonals.
						extended += float64(runs[at(i-1, j-1)])
					}
				}
				if score != nil {
					extended += score(i, j)
				} else {
					extended++
				}
			}
			best := max(horizontal, vertical, extended)
			switch best {
			case extended:
				prev := int32(0)
				if i > 0 && j > 0 {
					prev = runs[at(i-1, j-1)]
				}
				runs[at(i, j)] = prev + 1
				dirs[at(i, j)] = diagonal
			case horizontal:
				dirs[at(i, j)] = fromA
			default:
				dirs[at(i, j)] = fromB
			}
			lcs[at(i, j)] = best
		}
	}
	var out []seqDiff
	lastA, lastB := n, m
	report := func(i, j int) {
		if i+1 != lastA || j+1 != lastB {
			out = append(out, seqDiff{span{i + 1, lastA}, span{j + 1, lastB}})
		}
		lastA, lastB = i, j
	}
	for i, j := n-1, m-1; i >= 0 && j >= 0; {
		switch dirs[at(i, j)] {
		case diagonal:
			report(i, j)
			i--
			j--
		case fromA:
			i--
		default:
			j--
		}
	}
	report(-1, -1)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, false
}
