package diff

import (
	"sort"
	"unicode/utf8"
)

// computeMoves finds moved blocks among changes (Monaco's computeMovedLines)
// and diffs each block against its new copy.
func (c *computer) computeMoves(changes []Change) []Move {
	found := c.movedLines(changes)
	if len(found) == 0 {
		return nil
	}
	moves := make([]Move, len(found))
	for i, m := range found {
		mappings := c.refine(seqDiff{m.Original.span(), m.Modified.span()})
		moves[i] = Move{Original: m.Original, Modified: m.Modified, Changes: groupChanges(mappings, c.orig, c.mod)}
	}
	return moves
}

// lineMove pairs a block of original lines with a block of modified lines.
type lineMove struct{ Original, Modified LineRange }

func (m lineMove) join(o lineMove) lineMove {
	return lineMove{m.Original.join(o.Original), m.Modified.join(o.Modified)}
}

func (c *computer) movedLines(changes []Change) []lineMove {
	moves, excluded := c.movesFromDeletionsToInsertions(changes)
	if !c.bud.ok() {
		c.timedOut = true
		return nil
	}
	var rest []Change
	for i, ch := range changes {
		if !excluded[i] {
			rest = append(rest, ch)
		}
	}
	moves = append(moves, c.unchangedMoves(rest)...)
	moves = joinCloseMoves(moves)
	// Ignore moves too small to be worth showing.
	kept := moves[:0]
	for _, m := range moves {
		chars, longLines := 0, 0
		for i := m.Original.Start; i < m.Original.End; i++ {
			t := trimSpace(c.orig[i])
			chars += utf8.RuneCountInString(t) + 1
			if utf8.RuneCountInString(t) >= 2 {
				longLines++
			}
		}
		if chars-1 >= 15 && longLines >= 2 {
			kept = append(kept, m)
		}
	}
	return removeMovesInSameChange(changes, kept)
}

// movesFromDeletionsToInsertions pairs pure deletions with pure insertions
// of at least three lines whose character histograms are over 90% similar.
// excluded marks the indexes of the changes used up by a pair.
func (c *computer) movesFromDeletionsToInsertions(changes []Change) ([]lineMove, map[int]bool) {
	type fragment struct {
		index int
		r     LineRange
		hist  map[rune]int
		total int
	}
	newFragment := func(i int, r LineRange, lines []string) *fragment {
		f := &fragment{index: i, r: r, hist: map[rune]int{}}
		for _, line := range lines[r.Start:r.End] {
			for _, ch := range line {
				f.hist[ch]++
				f.total++
			}
			f.hist['\n']++
			f.total++
		}
		return f
	}
	similarity := func(a, b *fragment) float64 {
		diff := 0
		for k, v := range a.hist {
			diff += abs(v - b.hist[k])
		}
		for k, v := range b.hist {
			if _, ok := a.hist[k]; !ok {
				diff += v
			}
		}
		return 1 - float64(diff)/float64(a.total+b.total)
	}
	var deletions, insertions []*fragment
	for i, ch := range changes {
		if ch.Modified.Empty() && ch.Original.Len() >= 3 {
			deletions = append(deletions, newFragment(i, ch.Original, c.orig))
		}
		if ch.Original.Empty() && ch.Modified.Len() >= 3 {
			insertions = append(insertions, newFragment(i, ch.Modified, c.mod))
		}
	}
	var moves []lineMove
	excluded := map[int]bool{}
	for _, del := range deletions {
		best, bestSim := -1, -1.0
		for j, ins := range insertions {
			if ins == nil {
				continue
			}
			if s := similarity(del, ins); s > bestSim {
				best, bestSim = j, s
			}
		}
		if bestSim > 0.90 && best >= 0 {
			ins := insertions[best]
			insertions[best] = nil
			moves = append(moves, lineMove{del.r, ins.r})
			excluded[del.index] = true
			excluded[ins.index] = true
		}
		if !c.bud.ok() {
			break
		}
	}
	return moves, excluded
}

// unchangedMoves finds runs of at least three lines that appear, unchanged
// after trimming, in a deleted part and an inserted part of the changes,
// and extends them over neighbouring similar lines.
func (c *computer) unchangedMoves(changes []Change) []lineMove {
	key := func(s Side, i int) [3]int32 {
		return [3]int32{c.lineHash(s, i), c.lineHash(s, i+1), c.lineHash(s, i+2)}
	}
	original3 := map[[3]int32][]LineRange{}
	for _, ch := range changes {
		for i := ch.Original.Start; i < ch.Original.End-2; i++ {
			k := key(Original, i)
			original3[k] = append(original3[k], LineRange{i, i + 3})
		}
	}
	type mapping struct{ orig, mod LineRange }
	var possible []*mapping
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Modified.Start < changes[j].Modified.Start })
	for _, ch := range changes {
		var lastMappings []*mapping
		for i := ch.Modified.Start; i < ch.Modified.End-2; i++ {
			cur := LineRange{i, i + 3}
			var next []*mapping
			for _, r := range original3[key(Modified, i)] {
				extended := false
				for _, lm := range lastMappings {
					// Does this match continue a match of the previous line?
					if lm.orig.End+1 == r.End && lm.mod.End+1 == cur.End {
						lm.orig.End, lm.mod.End = r.End, cur.End
						next = append(next, lm)
						extended = true
						break
					}
				}
				if !extended {
					m := &mapping{orig: r, mod: cur}
					possible = append(possible, m)
					next = append(next, m)
				}
			}
			lastMappings = next
		}
		if !c.bud.ok() {
			c.timedOut = true
			return nil
		}
	}
	sort.SliceStable(possible, func(i, j int) bool { return possible[i].mod.Len() > possible[j].mod.Len() })

	var moves []lineMove
	var modSet, origSet lineSet
	for _, m := range possible {
		delta := m.mod.Start - m.orig.Start
		modFree := modSet.subtractFrom(m.mod)
		origFree := origSet.subtractFrom(m.orig).withDelta(delta)
		for _, r := range modFree.intersection(origFree).ranges {
			if r.Len() < 3 {
				continue
			}
			o := LineRange{r.Start - delta, r.End - delta}
			moves = append(moves, lineMove{o, r})
			modSet.add(r)
			origSet.add(o)
		}
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].Original.Start < moves[j].Original.Start })

	// lastChange returns the index of the last change whose side range
	// starts at or before (strict: before) line, or -1.
	lastChange := func(s Side, line int, strict bool) int {
		return sort.Search(len(changes), func(i int) bool {
			st := changes[i].lines(s).Start
			if strict {
				return st >= line
			}
			return st > line
		}) - 1
	}
	for i, mv := range moves {
		above := 0
		if a, b := lastChange(Original, mv.Original.Start, false), lastChange(Modified, mv.Modified.Start, false); a >= 0 && b >= 0 {
			above = max(mv.Original.Start-changes[a].Original.Start, mv.Modified.Start-changes[b].Modified.Start)
		}
		below := 0
		if a, b := lastChange(Original, mv.Original.End, true), lastChange(Modified, mv.Modified.End, true); a >= 0 && b >= 0 {
			below = max(changes[a].Original.End-mv.Original.End, changes[b].Modified.End-mv.Modified.End)
		}
		top := 0
		for ; top < above; top++ {
			o, m := mv.Original.Start-top-1, mv.Modified.Start-top-1
			if o < 0 || m < 0 || o >= len(c.orig) || m >= len(c.mod) {
				break
			}
			if modSet.contains(m) || origSet.contains(o) || !c.linesSimilar(c.orig[o], c.mod[m]) {
				break
			}
		}
		if top > 0 {
			origSet.add(LineRange{mv.Original.Start - top, mv.Original.Start})
			modSet.add(LineRange{mv.Modified.Start - top, mv.Modified.Start})
		}
		bottom := 0
		for ; bottom < below; bottom++ {
			o, m := mv.Original.End+bottom, mv.Modified.End+bottom
			if o >= len(c.orig) || m >= len(c.mod) {
				break
			}
			if modSet.contains(m) || origSet.contains(o) || !c.linesSimilar(c.orig[o], c.mod[m]) {
				break
			}
		}
		if bottom > 0 {
			origSet.add(LineRange{mv.Original.End, mv.Original.End + bottom})
			modSet.add(LineRange{mv.Modified.End, mv.Modified.End + bottom})
		}
		if top > 0 || bottom > 0 {
			moves[i] = lineMove{
				LineRange{mv.Original.Start - top, mv.Original.End + bottom},
				LineRange{mv.Modified.Start - top, mv.Modified.End + bottom},
			}
		}
	}
	return moves
}

// linesSimilar reports whether two lines are equal after trimming, or share
// more than 60% of the non-blank characters of the longer one (which must
// have more than 10).
func (c *computer) linesSimilar(a, b string) bool {
	a, b = trimSpace(a), trimSpace(b)
	if a == b {
		return true
	}
	if len(a) > 300 && len(b) > 300 {
		return false
	}
	ra, rb := []int32(a), []int32(b)
	diffs, _ := myers(ra, rb, c.bud)
	common := 0
	for _, eq := range invert(diffs, len(ra)) {
		for i := eq.a.start; i < eq.a.end; i++ {
			if ra[i] != ' ' && ra[i] != '\t' {
				common++
			}
		}
	}
	longer := ra
	if len(rb) > len(ra) {
		longer = rb
	}
	nonBlank := 0
	for _, r := range longer {
		if r != ' ' && r != '\t' {
			nonBlank++
		}
	}
	return nonBlank > 10 && float64(common)/float64(nonBlank) > 0.6
}

// joinCloseMoves sorts moves by original position and joins moves that
// follow each other within two lines on both sides together.
func joinCloseMoves(moves []lineMove) []lineMove {
	if len(moves) == 0 {
		return moves
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].Original.Start < moves[j].Original.Start })
	res := []lineMove{moves[0]}
	for _, cur := range moves[1:] {
		last := &res[len(res)-1]
		od := cur.Original.Start - last.Original.End
		md := cur.Modified.Start - last.Modified.End
		if od >= 0 && md >= 0 && od+md <= 2 {
			*last = last.join(cur)
			continue
		}
		res = append(res, cur)
	}
	return res
}

// removeMovesInSameChange drops moves whose two ends fall into the same
// change: those are better shown as an ordinary edit.
func removeMovesInSameChange(changes []Change, moves []lineMove) []lineMove {
	last := func(s Side, line int) int {
		return sort.Search(len(changes), func(i int) bool { return changes[i].lines(s).Start >= line }) - 1
	}
	var out []lineMove
	for _, m := range moves {
		o := last(Original, m.Original.End)
		if o < 0 {
			o = -2 // Monaco's placeholder change, distinct from "none"
		}
		if o != last(Modified, m.Modified.End) {
			out = append(out, m)
		}
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// lineSet is a set of lines kept as sorted, disjoint, non-touching ranges.
type lineSet struct{ ranges []LineRange }

func (s *lineSet) add(r LineRange) {
	// Ranges that intersect or touch r are merged into it.
	i := sort.Search(len(s.ranges), func(i int) bool { return s.ranges[i].End >= r.Start })
	j := i
	for j < len(s.ranges) && s.ranges[j].Start <= r.End {
		r = r.join(s.ranges[j])
		j++
	}
	s.ranges = append(s.ranges[:i], append([]LineRange{r}, s.ranges[j:]...)...)
}

func (s *lineSet) contains(line int) bool {
	i := sort.Search(len(s.ranges), func(i int) bool { return s.ranges[i].End > line })
	return i < len(s.ranges) && s.ranges[i].Start <= line
}

// subtractFrom returns r without the lines of s.
func (s *lineSet) subtractFrom(r LineRange) lineSet {
	var out lineSet
	start := r.Start
	for _, x := range s.ranges {
		if x.End <= start {
			continue
		}
		if x.Start >= r.End {
			break
		}
		if x.Start > start {
			out.ranges = append(out.ranges, LineRange{start, x.Start})
		}
		start = max(start, x.End)
	}
	if start < r.End {
		out.ranges = append(out.ranges, LineRange{start, r.End})
	}
	return out
}

func (s lineSet) withDelta(d int) lineSet {
	out := lineSet{ranges: make([]LineRange, len(s.ranges))}
	for i, r := range s.ranges {
		out.ranges[i] = LineRange{r.Start + d, r.End + d}
	}
	return out
}

func (s lineSet) intersection(o lineSet) lineSet {
	var out lineSet
	for i, j := 0, 0; i < len(s.ranges) && j < len(o.ranges); {
		a, b := s.ranges[i], o.ranges[j]
		if st, en := max(a.Start, b.Start), min(a.End, b.End); st < en {
			out.ranges = append(out.ranges, LineRange{st, en})
		}
		if a.End < b.End {
			i++
		} else {
			j++
		}
	}
	return out
}
