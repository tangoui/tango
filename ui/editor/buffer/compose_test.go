package buffer

import (
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"
)

// composeQuadratic is the previous compose (one scan of a per span of b),
// the reference for TestComposeMatchesReference.
func composeQuadratic(a, b batch) batch {
	if len(a) == 0 {
		return slices.Clone(b)
	}
	c := slices.Clone(a)
	// Back to front: merging b[k] only changes c at or after b[k].start, so
	// the current-coordinate offsets of the earlier b spans stay valid.
	for k := len(b) - 1; k >= 0; k-- {
		y := b[k]
		// i: first span of c whose current end reaches y.start; delta is
		// the length change of the spans before it.
		i, delta := 0, 0
		for ; i < len(c); i++ {
			if c[i].start+delta+c[i].n >= y.start {
				break
			}
			delta += c[i].n - (c[i].end - c[i].start)
		}
		// [i, j): the spans of c that y touches.
		j, d, lastEnd := i, delta, 0
		for ; j < len(c); j++ {
			cs := c[j].start + d
			if cs > y.end {
				break
			}
			lastEnd = cs + c[j].n
			d += c[j].n - (c[j].end - c[j].start)
		}
		if j == i {
			c = slices.Insert(c, i, span{start: y.start - delta, end: y.end - delta, n: y.n})
			continue
		}
		cs := c[i].start + delta
		curStart, curEnd := min(y.start, cs), max(y.end, lastEnd)
		m := span{
			start: c[i].start - (cs - curStart),
			end:   c[j-1].end + (curEnd - lastEnd),
			n:     (curEnd - curStart) - (y.end - y.start) + y.n,
		}
		c = slices.Replace(c, i, j, m)
	}
	return slices.DeleteFunc(c, func(sp span) bool { return sp.start == sp.end && sp.n == 0 })
}

// randomBatch returns a valid batch over a text of length n.
func randomBatch(r *rand.Rand, n int) batch {
	var bt batch
	pos := 0
	for pos <= n && r.IntN(4) != 0 {
		start := pos + r.IntN(4)
		if start > n {
			break
		}
		end := start + r.IntN(3)
		if end > n {
			end = n
		}
		sp := span{start: start, end: end, n: r.IntN(3)}
		if sp.start == sp.end && sp.n == 0 {
			sp.n = 1
		}
		bt = append(bt, sp)
		pos = end
		if r.IntN(3) == 0 {
			pos++ // not always touching
		}
	}
	return bt
}

func outLen(n int, bt batch) int {
	for _, sp := range bt {
		n += sp.n - (sp.end - sp.start)
	}
	return n
}

// TestComposeMatchesReference: the linear compose gives the same batches
// as the previous algorithm on random input.
func TestComposeMatchesReference(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for it := range 200000 {
		n := r.IntN(12)
		a := randomBatch(r, n)
		b := randomBatch(r, outLen(n, a))
		got, want := compose(a, b), composeQuadratic(a, b)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !slices.Equal(got, want) {
			t.Fatalf("iteration %d: compose(%v, %v) = %v, want %v", it, a, b, got, want)
		}
	}
}

// TestComposeLinear: composing many large batches stays fast. 30 composes
// of 10 000 spans take about 5 ms (30 ms under -race) on an Apple M4; the
// previous quadratic algorithm took about 1 s (27 s under -race), far
// above the gate.
func TestComposeLinear(t *testing.T) {
	const n = 10000
	mk := func(k int) batch { // one insert at each of n positions, k already typed
		bt := make(batch, n)
		for j := range bt {
			p := j*(8+k) + 8 + k
			bt[j] = span{start: p, end: p, n: 1}
		}
		return bt
	}
	start := time.Now()
	var c batch
	for k := range 30 {
		c = compose(c, mk(k))
	}
	limit := 100 * time.Millisecond
	if raceEnabled {
		limit *= 10
	}
	// Shared CI runners (TANGO_SMOKE_TIMEOUT_SCALE, which CI sets) vary by
	// 2x and more; a quadratic compose misses the gate by far more.
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		limit = time.Duration(float64(limit) * 2 * f)
	}
	if d := time.Since(start); d > limit {
		t.Errorf("30 composes of %d spans took %v, want ≤ %v", n, d, limit)
	}
	if len(c) != n || c[0].n != 30 {
		t.Fatalf("composed %d spans, first %+v", len(c), c[0])
	}
}
