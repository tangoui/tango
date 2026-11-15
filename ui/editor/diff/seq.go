package diff

import (
	"context"
	"time"
)

// span is a half-open range of sequence offsets.
type span struct{ start, end int }

func (s span) len() int         { return s.end - s.start }
func (s span) empty() bool      { return s.end <= s.start }
func (s span) delta(n int) span { return span{s.start + n, s.end + n} }
func (s span) join(o span) span { return span{min(s.start, o.start), max(s.end, o.end)} }
func (s span) intersects(o span) bool {
	return max(s.start, o.start) < min(s.end, o.end)
}

// intersect returns the (possibly empty) overlap of s and o; ok is false
// when they are apart.
func (s span) intersect(o span) (span, bool) {
	start, end := max(s.start, o.start), min(s.end, o.end)
	if start <= end {
		return span{start, end}, true
	}
	return span{}, false
}

// pair is a point in the edit graph: an offset into each sequence.
type pair struct{ a, b int }

// seqDiff says that elements a of the first sequence correspond to elements
// b of the second (a replacement, or an insertion/deletion when one is
// empty).
type seqDiff struct{ a, b span }

func (d seqDiff) delta(n int) seqDiff {
	if n == 0 {
		return d
	}
	return seqDiff{d.a.delta(n), d.b.delta(n)}
}

func (d seqDiff) deltaStart(n int) seqDiff {
	return seqDiff{span{d.a.start + n, d.a.end}, span{d.b.start + n, d.b.end}}
}

func (d seqDiff) deltaEnd(n int) seqDiff {
	return seqDiff{span{d.a.start, d.a.end + n}, span{d.b.start, d.b.end + n}}
}

func (d seqDiff) swap() seqDiff          { return seqDiff{d.b, d.a} }
func (d seqDiff) join(o seqDiff) seqDiff { return seqDiff{d.a.join(o.a), d.b.join(o.b)} }
func (d seqDiff) starts() pair           { return pair{d.a.start, d.b.start} }
func (d seqDiff) ends() pair             { return pair{d.a.end, d.b.end} }

func (d seqDiff) intersect(o seqDiff) (seqDiff, bool) {
	a, ok1 := d.a.intersect(o.a)
	b, ok2 := d.b.intersect(o.b)
	return seqDiff{a, b}, ok1 && ok2
}

func diffBetween(start, end pair) seqDiff {
	return seqDiff{span{start.a, end.a}, span{start.b, end.b}}
}

// invert returns the equal stretches around diffs: one before each diff and
// one after the last, some possibly empty. lenA is the length of the first
// sequence.
func invert(diffs []seqDiff, lenA int) []seqDiff {
	out := make([]seqDiff, 0, len(diffs)+1)
	prev := pair{}
	shift := 0
	for _, d := range diffs {
		out = append(out, diffBetween(prev, d.starts()))
		prev = d.ends()
		shift = d.b.end - d.a.end
	}
	out = append(out, diffBetween(prev, pair{lenA, lenA + shift}))
	return out
}

// budget is the time budget of one diff. Once exhausted it stays exhausted,
// so every later stage falls back to its cheap path.
type budget struct {
	ctx      context.Context
	deadline time.Time
	expired  bool
}

func newBudget(ctx context.Context, timeout time.Duration) *budget {
	b := &budget{ctx: ctx}
	if timeout > 0 {
		b.deadline = time.Now().Add(timeout)
	}
	return b
}

// ok reports whether there is time left.
func (b *budget) ok() bool {
	if b.expired {
		return false
	}
	if b.ctx.Err() != nil || (!b.deadline.IsZero() && !time.Now().Before(b.deadline)) {
		b.expired = true
	}
	return !b.expired
}

// trivial is the diff that replaces everything.
func trivial(n, m int) []seqDiff { return []seqDiff{{span{0, n}, span{0, m}}} }
