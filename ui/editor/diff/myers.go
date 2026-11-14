package diff

// myers aligns a and b with Myers' greedy O((N+M)D) algorithm. It returns
// the differing stretches in order; on timeout it returns the trivial diff
// and true.
func myers(a, b []int32, bud *budget) ([]seqDiff, bool) {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return trivial(n, m), false
	}
	slide := func(x, y int) int {
		if y < 0 {
			return x
		}
		for x < n && y < m && a[x] == b[y] {
			x++
			y++
		}
		return x
	}
	// v.get(k) is the furthest x reached on diagonal k (x-y = k) with the
	// current number of non-diagonal steps; paths.get(k) the snakes that led
	// there.
	var v diag[int]
	var paths diag[*snake]
	v.set(0, slide(0, 0))
	if x := v.get(0); x != 0 {
		paths.set(0, &snake{x: 0, y: 0, n: x})
	}
	k := 0
outer:
	for d := 1; ; d++ {
		if !bud.ok() {
			return trivial(n, m), true
		}
		// Diagonals further out than the sequences cannot reach the end.
		lo := -min(d, m+d%2)
		hi := min(d, n+d%2)
		for k = lo; k <= hi; k += 2 {
			fromTop := -1 // insert an element of b: x stays
			if k != hi {
				fromTop = v.get(k + 1)
			}
			fromLeft := -1 // delete an element of a: x+1
			if k != lo {
				fromLeft = v.get(k-1) + 1
			}
			x := min(max(fromTop, fromLeft), n)
			y := x - k
			if x > n || y > m {
				continue
			}
			nx := slide(x, y)
			v.set(k, nx)
			last := paths.get(k - 1)
			if x == fromTop {
				last = paths.get(k + 1)
			}
			if nx != x {
				paths.set(k, &snake{prev: last, x: x, y: y, n: nx - x})
			} else {
				paths.set(k, last)
			}
			if nx == n && nx-k == m {
				break outer
			}
		}
	}
	var out []seqDiff
	lastA, lastB := n, m
	for p := paths.get(k); ; p = p.prev {
		endA, endB := 0, 0
		if p != nil {
			endA, endB = p.x+p.n, p.y+p.n
		}
		if endA != lastA || endB != lastB {
			out = append(out, seqDiff{span{endA, lastA}, span{endB, lastB}})
		}
		if p == nil {
			break
		}
		lastA, lastB = p.x, p.y
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, false
}

// snake is a run of n equal elements starting at (x, y), linked to the
// snake before it on the same path.
type snake struct {
	prev    *snake
	x, y, n int
}

// diag is a slice indexed by a possibly negative diagonal number.
type diag[T any] struct{ pos, neg []T }

func (d *diag[T]) get(k int) T {
	var zero T
	if k < 0 {
		k = -k - 1
		if k < len(d.neg) {
			return d.neg[k]
		}
		return zero
	}
	if k < len(d.pos) {
		return d.pos[k]
	}
	return zero
}

func (d *diag[T]) set(k int, v T) {
	s := &d.pos
	if k < 0 {
		s, k = &d.neg, -k-1
	}
	if k >= len(*s) {
		*s = append(*s, make([]T, k+1-len(*s))...)
	}
	(*s)[k] = v
}
