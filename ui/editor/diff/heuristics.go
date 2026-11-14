package diff

import (
	"math"
	"strings"
)

// The heuristics below clean up raw alignments; they follow Monaco's
// heuristicSequenceOptimizations.

// optimizeDiffs joins diffs that become adjacent when slid, then slides
// insertions and deletions to the best-scoring boundary.
func optimizeDiffs(s1, s2 sequence, diffs []seqDiff) []seqDiff {
	diffs = joinByShifting(s1, s2, diffs)
	// A second pass sometimes finds more joins.
	diffs = joinByShifting(s1, s2, diffs)
	return shiftToBoundaries(s1, s2, diffs)
}

// joinByShifting slides every insertion or deletion as far left as it can
// go (its text repeats just before it), merging it into the previous diff
// when they meet; then does the same to the right.
func joinByShifting(s1, s2 sequence, diffs []seqDiff) []seqDiff {
	if len(diffs) == 0 {
		return diffs
	}
	e1, e2 := s1.elements(), s2.elements()
	res := []seqDiff{diffs[0]}
	for _, cur := range diffs[1:] {
		prev := res[len(res)-1]
		if cur.a.empty() || cur.b.empty() {
			length := cur.a.start - prev.a.end
			d := 1
			for ; d <= length; d++ {
				if e1[cur.a.start-d] != e1[cur.a.end-d] || e2[cur.b.start-d] != e2[cur.b.end-d] {
					break
				}
			}
			d--
			if d == length {
				res[len(res)-1] = seqDiff{span{prev.a.start, cur.a.end - length}, span{prev.b.start, cur.b.end - length}}
				continue
			}
			cur = cur.delta(-d)
		}
		res = append(res, cur)
	}
	out := make([]seqDiff, 0, len(res))
	for i := 0; i < len(res)-1; i++ {
		next, cur := res[i+1], res[i]
		if cur.a.empty() || cur.b.empty() {
			length := next.a.start - cur.a.end
			d := 0
			for ; d < length; d++ {
				if !s1.stronglyEqual(cur.a.start+d, cur.a.end+d) || !s2.stronglyEqual(cur.b.start+d, cur.b.end+d) {
					break
				}
			}
			if d == length {
				res[i+1] = seqDiff{span{cur.a.start + length, next.a.end}, span{cur.b.start + length, next.b.end}}
				continue
			}
			cur = cur.delta(d)
		}
		out = append(out, cur)
	}
	return append(out, res[len(res)-1])
}

// shiftToBoundaries slides each insertion or deletion, without touching its
// neighbours, to the position with the best boundary score, e.g.
//
//	import { I[Arr, I]Bar } from "foo";  ->  import { [IArr, ]IBar } from "foo";
func shiftToBoundaries(s1, s2 sequence, diffs []seqDiff) []seqDiff {
	n1, n2 := len(s1.elements()), len(s2.elements())
	for i, d := range diffs {
		valid1, valid2 := span{0, n1}, span{0, n2}
		if i > 0 {
			valid1.start, valid2.start = diffs[i-1].a.end+1, diffs[i-1].b.end+1
		}
		if i+1 < len(diffs) {
			valid1.end, valid2.end = diffs[i+1].a.start-1, diffs[i+1].b.start-1
		}
		if d.a.empty() {
			diffs[i] = shiftToBetterPosition(d, s1, s2, valid1, valid2)
		} else if d.b.empty() {
			diffs[i] = shiftToBetterPosition(d.swap(), s2, s1, valid2, valid1).swap()
		}
	}
	return diffs
}

// shiftToBetterPosition slides an insertion d (d.a empty) within the valid
// ranges to the offset with the highest boundary score.
func shiftToBetterPosition(d seqDiff, s1, s2 sequence, valid1, valid2 span) seqDiff {
	const maxShift = 100 // keeps the scan cheap
	before := 1
	for d.a.start-before >= valid1.start && d.b.start-before >= valid2.start &&
		s2.stronglyEqual(d.b.start-before, d.b.end-before) && before < maxShift {
		before++
	}
	before--
	after := 0
	for d.a.start+after < valid1.end && d.b.end+after < valid2.end &&
		s2.stronglyEqual(d.b.start+after, d.b.end+after) && after < maxShift {
		after++
	}
	if before == 0 && after == 0 {
		return d
	}
	best, bestScore := 0, -1
	for delta := -before; delta <= after; delta++ {
		score := s1.boundaryScore(d.a.start+delta) + s2.boundaryScore(d.b.start+delta) + s2.boundaryScore(d.b.end+delta)
		if score > bestScore {
			best, bestScore = delta, score
		}
	}
	return d.delta(best)
}

// removeShortMatches joins diffs separated by at most two equal elements on
// either side.
func removeShortMatches(diffs []seqDiff) []seqDiff {
	var res []seqDiff
	for _, d := range diffs {
		if n := len(res); n > 0 {
			last := res[n-1]
			if d.a.start-last.a.end <= 2 || d.b.start-last.b.end <= 2 {
				res[n-1] = last.join(d)
				continue
			}
		}
		res = append(res, d)
	}
	return res
}

// extendToWords widens diffs to whole words (as found by find) when most of
// a word changed anyway; with force, whenever any of it changed.
func extendToWords(s1, s2 *charSeq, diffs []seqDiff, find func(*charSeq, int) (span, bool), force bool) []seqDiff {
	equal := invert(diffs, len(s1.elems))
	var extra []seqDiff
	last := pair{}
	scan := func(p pair, eq seqDiff) {
		if p.a < last.a || p.b < last.b {
			return
		}
		w1, ok1 := find(s1, p.a)
		w2, ok2 := find(s2, p.b)
		if !ok1 || !ok2 {
			return
		}
		w := seqDiff{w1, w2}
		part, _ := w.intersect(eq)
		eq1, eq2 := part.a.len(), part.b.len()
		// The word cannot touch earlier equal stretches (they were handled
		// already) but may run into the following ones.
		for len(equal) > 0 {
			next := equal[0]
			if !next.a.intersects(w.a) && !next.b.intersects(w.b) {
				break
			}
			v1, ok1 := find(s1, next.a.start)
			v2, ok2 := find(s2, next.b.start)
			if !ok1 || !ok2 {
				break
			}
			v := seqDiff{v1, v2}
			part, _ := v.intersect(next)
			eq1 += part.a.len()
			eq2 += part.b.len()
			w = w.join(v)
			if w.a.end < next.a.end {
				break
			}
			equal = equal[1:] // the word covers this stretch entirely
		}
		total := w.a.len() + w.b.len()
		if (force && eq1+eq2 < total) || float64(eq1+eq2) < float64(total)*2/3 {
			extra = append(extra, w)
		}
		last = w.ends()
	}
	for len(equal) > 0 {
		next := equal[0]
		equal = equal[1:]
		if next.a.empty() {
			continue
		}
		scan(next.starts(), next)
		// next is not empty, so the element before its end is equal on both
		// sides.
		scan(pair{next.a.end - 1, next.b.end - 1}, next)
	}
	return mergeDiffs(diffs, extra)
}

// mergeDiffs merges two sorted diff lists, joining overlapping entries.
func mergeDiffs(x, y []seqDiff) []seqDiff {
	var res []seqDiff
	for len(x) > 0 || len(y) > 0 {
		var next seqDiff
		if len(x) > 0 && (len(y) == 0 || x[0].a.start < y[0].a.start) {
			next, x = x[0], x[1:]
		} else {
			next, y = y[0], y[1:]
		}
		if n := len(res); n > 0 && res[n-1].a.end >= next.a.start {
			res[n-1] = res[n-1].join(next)
		} else {
			res = append(res, next)
		}
	}
	return res
}

// joinRepeatedly merges neighbouring diffs while shouldJoin says so, for at
// most a few rounds.
func joinRepeatedly(diffs []seqDiff, shouldJoin func(before, after seqDiff) bool) []seqDiff {
	if len(diffs) == 0 {
		return diffs
	}
	for round := 0; round <= 10; round++ {
		joined := false
		res := []seqDiff{diffs[0]}
		for _, cur := range diffs[1:] {
			last := res[len(res)-1]
			if shouldJoin(last, cur) {
				joined = true
				res[len(res)-1] = last.join(cur)
			} else {
				res = append(res, cur)
			}
		}
		diffs = res
		if !joined {
			break
		}
	}
	return diffs
}

// removeShortMatchingLines joins line diffs separated only by lines with
// at most four non-white-space characters, when one of them is large.
func removeShortMatchingLines(s1 *lineSeq, diffs []seqDiff) []seqDiff {
	return joinRepeatedly(diffs, func(before, after seqDiff) bool {
		n := 0
		for _, line := range s1.lines[before.a.end:after.a.start] {
			n += nonSpaceLen(line, 4-n)
			if n > 4 {
				return false
			}
		}
		return before.a.len()+before.b.len() > 5 || after.a.len()+after.b.len() > 5
	})
}

// removeShortMatchingText joins character diffs separated by a short piece
// of unchanged text when the diffs around it are long, and widens long
// diffs over short line prefixes and suffixes.
func removeShortMatchingText(s1, s2 *charSeq, diffs []seqDiff) []seqDiff {
	if len(diffs) == 0 {
		return diffs
	}
	diffs = joinRepeatedly(diffs, func(before, after seqDiff) bool {
		gap := span{before.a.end, after.a.start}
		if s1.countLines(gap) > 5 || gap.len() > 500 {
			return false
		}
		text := strings.TrimSpace(s1.text(gap))
		if len([]rune(text)) > 20 || strings.ContainsAny(text, "\r\n") {
			return false
		}
		const limit = 2*40 + 50
		weight := func(lines, n int) float64 { return math.Pow(float64(min(lines*40+n, limit)), 1.5) }
		side := func(d seqDiff) float64 {
			return math.Pow(weight(s1.countLines(d.a), d.a.len())+weight(s2.countLines(d.b), d.b.len()), 1.5)
		}
		return side(before)+side(after) > math.Pow(math.Pow(limit, 1.5), 1.5)*1.3
	})
	out := make([]seqDiff, 0, len(diffs))
	for i, cur := range diffs {
		d := cur
		mark := func(r span) bool {
			return !r.empty() && cur.a.len()+cur.b.len() > 100 && s1.trimmedLen(r, 3) <= 3
		}
		full := s1.fullLines(cur.a)
		if prefix := (span{full.start, cur.a.start}); mark(prefix) {
			d = d.deltaStart(-prefix.len())
		}
		if suffix := (span{cur.a.end, full.end}); mark(suffix) {
			d = d.deltaEnd(suffix.len())
		}
		lo := pair{}
		if i > 0 {
			lo = diffs[i-1].ends()
		}
		hi := pair{math.MaxInt, math.MaxInt}
		if i+1 < len(diffs) {
			hi = diffs[i+1].starts()
		}
		d, _ = d.intersect(diffBetween(lo, hi))
		if n := len(out); n > 0 && d.starts() == out[n-1].ends() {
			out[n-1] = out[n-1].join(d)
		} else {
			out = append(out, d)
		}
	}
	return out
}
