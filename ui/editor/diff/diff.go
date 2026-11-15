package diff

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Size thresholds below which the O(N*M) dynamic-programming alignment is
// used instead of Myers (Monaco's values).
const (
	dpLineLimit = 1700 // lines on both sides together
	dpCharLimit = 500  // characters on both sides together
)

// windowMargin is how many lines of the common prefix and suffix are kept
// around the differing middle. The heuristics may slide a change into them
// (shiftToBetterPosition looks at most 100 elements away).
const windowMargin = 128

// SplitLines splits text into lines at "\r\n", "\r" and "\n", like
// buffer.NewSnapshot. Text ending in a line break ends with an empty line;
// empty text is one empty line.
func SplitLines(text string) []string {
	if !strings.ContainsRune(text, '\r') {
		return strings.Split(text, "\n")
	}
	var lines []string
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			lines = append(lines, text[start:i])
			start = i + 1
		case '\r':
			lines = append(lines, text[start:i])
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	return append(lines, text[start:])
}

// Snapshots diffs two snapshots. Snapshots store LF-normalised text, so a
// different line-ending style (Snapshot.EOL) is not a change; compare EOL()
// separately if that matters.
func Snapshots(ctx context.Context, original, modified buffer.Snapshot, opts Options) Result {
	return Lines(ctx, strings.Split(original.String(), "\n"), strings.Split(modified.String(), "\n"), opts)
}

// Lines diffs two texts given as lines without line breaks (see
// SplitLines). An empty slice is treated as one empty line. The context
// can cancel the computation, which then ends like a timeout.
func Lines(ctx context.Context, original, modified []string, opts Options) Result {
	if len(original) == 0 {
		original = []string{""}
	}
	if len(modified) == 0 {
		modified = []string{""}
	}
	res := Result{OriginalLines: len(original), ModifiedLines: len(modified)}
	if slices.Equal(original, modified) {
		res.Identical = true
		return res
	}
	if isEmptyText(original) || isEmptyText(modified) {
		res.Changes = []Change{{
			Original: LineRange{0, len(original)},
			Modified: LineRange{0, len(modified)},
			Inner:    []RangeMapping{{wholeText(original), wholeText(modified)}},
		}}
		return res
	}
	c := &computer{
		orig:       original,
		mod:        modified,
		bud:        newBudget(ctx, opts.Timeout),
		considerWS: !opts.IgnoreTrimWhitespace,
		opts:       opts,
	}
	res.Changes = c.run()
	if opts.ComputeMoves {
		res.Moves = c.computeMoves(res.Changes)
	}
	res.TimedOut = c.timedOut
	return res
}

func isEmptyText(lines []string) bool { return len(lines) == 1 && lines[0] == "" }

func wholeText(lines []string) Range {
	last := len(lines) - 1
	return Range{End: buffer.Pos{Line: last, Col: len(lines[last])}}
}

// computer holds the state of one diff.
type computer struct {
	orig, mod  []string
	bud        *budget
	considerWS bool
	opts       Options
	timedOut   bool

	// Trimmed-line hashes of the window [w0, w0+len(hash1)) of the original
	// and [w0, w0+len(hash2)) of the modified text; lines outside are hashed
	// on demand.
	hashes       hasher
	w0           int
	hash1, hash2 []int32
}

// run aligns the lines, refines every changed block to characters and
// returns the changes.
func (c *computer) run() []Change {
	n1, n2 := len(c.orig), len(c.mod)
	// The common prefix and suffix (equal after trimming) cannot contain a
	// change except a whitespace-only one, so only a window around the
	// middle is aligned. This keeps a small edit in a huge file cheap.
	prefix := 0
	for prefix < min(n1, n2) && trimEqual(c.orig[prefix], c.mod[prefix]) {
		prefix++
	}
	suffix := 0
	for suffix < min(n1, n2)-prefix && trimEqual(c.orig[n1-1-suffix], c.mod[n2-1-suffix]) {
		suffix++
	}
	c.w0 = max(0, prefix-windowMargin)
	cut := max(0, suffix-windowMargin)
	c.hashes = make(hasher, n1+n2-2*cut-2*c.w0)
	c.hash1 = c.hashLines(c.orig[c.w0 : n1-cut])
	c.hash2 = c.hashLines(c.mod[c.w0 : n2-cut])
	seq1 := &lineSeq{c.hash1, c.orig[c.w0 : n1-cut]}
	seq2 := &lineSeq{c.hash2, c.mod[c.w0 : n2-cut]}

	var diffs []seqDiff
	var timedOut bool
	if len(c.hash1)+len(c.hash2) < dpLineLimit {
		diffs, timedOut = dpDiff(c.hash1, c.hash2, c.bud, func(i, j int) float64 {
			a, b := seq1.lines[i], seq2.lines[j]
			switch {
			case a != b:
				return 0.99 // equal only after trimming
			case b == "":
				return 0.1
			}
			return 1 + math.Log(1+float64(len(b)))
		})
	} else {
		diffs, timedOut = myers(c.hash1, c.hash2, c.bud)
	}
	if timedOut {
		// Fall back to one block over the differing middle; the common
		// prefix and suffix are known to match.
		c.timedOut = true
		diffs = nil
		if mid := (seqDiff{span{prefix, n1 - suffix}, span{prefix, n2 - suffix}}); !mid.a.empty() || !mid.b.empty() {
			diffs = []seqDiff{mid.delta(-c.w0)}
		}
	}
	diffs = optimizeDiffs(seq1, seq2, diffs)
	diffs = removeShortMatchingLines(seq1, diffs)

	var mappings []RangeMapping
	last1, last2 := 0, 0
	// Lines aligned as equal may still differ in white space.
	scanWhitespace := func(count int) {
		if !c.considerWS {
			return
		}
		for i := range count {
			a, b := last1+i, last2+i
			if c.orig[a] != c.mod[b] {
				mappings = append(mappings, c.refine(seqDiff{span{a, a + 1}, span{b, b + 1}})...)
			}
		}
	}
	for _, d := range diffs {
		d = d.delta(c.w0)
		scanWhitespace(d.a.start - last1)
		last1, last2 = d.a.end, d.b.end
		mappings = append(mappings, c.refine(d)...)
	}
	scanWhitespace(n1 - last1)
	return groupChanges(mappings, c.orig, c.mod)
}

func (c *computer) hashLines(lines []string) []int32 {
	h := make([]int32, len(lines))
	for i, l := range lines {
		h[i] = c.hashes.hash(trimSpace(l))
	}
	return h
}

// lineHash returns the trimmed-text hash of line i of a side.
func (c *computer) lineHash(s Side, i int) int32 {
	lines, hashes := c.orig, c.hash1
	if s == Modified {
		lines, hashes = c.mod, c.hash2
	}
	if j := i - c.w0; j >= 0 && j < len(hashes) {
		return hashes[j]
	}
	return c.hashes.hash(trimSpace(lines[i]))
}

// refine turns a line diff into character-level range mappings.
func (c *computer) refine(d seqDiff) []RangeMapping {
	rm := c.lineDiffRange(d)
	if !c.bud.ok() {
		// Out of time: keep the whole block rather than building character
		// sequences that would only produce the same answer.
		c.timedOut = true
		return []RangeMapping{rm}
	}
	s1 := newCharSeq(c.orig, rm.Original, c.considerWS)
	s2 := newCharSeq(c.mod, rm.Modified, c.considerWS)
	var diffs []seqDiff
	var timedOut bool
	if len(s1.elems)+len(s2.elems) < dpCharLimit {
		diffs, timedOut = dpDiff(s1.elems, s2.elems, c.bud, nil)
	} else {
		diffs, timedOut = myers(s1.elems, s2.elems, c.bud)
	}
	c.timedOut = c.timedOut || timedOut
	diffs = optimizeDiffs(s1, s2, diffs)
	diffs = extendToWords(s1, s2, diffs, (*charSeq).word, false)
	if c.opts.ExtendToSubwords {
		diffs = extendToWords(s1, s2, diffs, (*charSeq).subWord, true)
	}
	diffs = removeShortMatches(diffs)
	diffs = removeShortMatchingText(s1, s2, diffs)
	out := make([]RangeMapping, len(diffs))
	for i, d := range diffs {
		out[i] = RangeMapping{s1.translate(d.a), s2.translate(d.b)}
	}
	return out
}

// lineDiffRange returns the text range covered by a line diff, choosing the
// line breaks it includes so that both ranges stay inside their texts.
func (c *computer) lineDiffRange(d seqDiff) RangeMapping {
	n1, n2 := len(c.orig), len(c.mod)
	if d.a.end < n1 && d.b.end < n2 {
		// Whole lines including their trailing line break.
		return RangeMapping{
			Range{buffer.Pos{Line: d.a.start}, buffer.Pos{Line: d.a.end}},
			Range{buffer.Pos{Line: d.b.start}, buffer.Pos{Line: d.b.end}},
		}
	}
	if !d.a.empty() && !d.b.empty() {
		// Touches the end: whole lines without the last line break.
		return RangeMapping{
			Range{buffer.Pos{Line: d.a.start}, lineEnd(c.orig, d.a.end-1)},
			Range{buffer.Pos{Line: d.b.start}, lineEnd(c.mod, d.b.end-1)},
		}
	}
	if d.a.start > 0 && d.b.start > 0 {
		// Insertion or deletion at the end: include the line break before.
		return RangeMapping{
			Range{lineEnd(c.orig, d.a.start-1), lineEnd(c.orig, d.a.end-1)},
			Range{lineEnd(c.mod, d.b.start-1), lineEnd(c.mod, d.b.end-1)},
		}
	}
	// Not reachable for a valid diff of non-empty texts; fall back to the
	// whole texts.
	return RangeMapping{wholeText(c.orig), wholeText(c.mod)}
}

func lineEnd(lines []string, i int) buffer.Pos {
	return buffer.Pos{Line: i, Col: len(lines[i])}
}

// lineChange returns the lines touched by a character mapping. A mapping
// that ends at column 0 does not touch its last line, and one that starts
// at the end of a line does not touch that line, as long as the range
// stays non-empty.
func lineChange(m RangeMapping, orig, mod []string) Change {
	o, md := m.Original, m.Modified
	startDelta, endDelta := 0, 0
	if md.End.Col == 0 && o.End.Col == 0 && o.Start.Line <= o.End.Line && md.Start.Line <= md.End.Line {
		endDelta = -1
	}
	if md.Start.Col >= len(mod[md.Start.Line]) && o.Start.Col >= len(orig[o.Start.Line]) &&
		o.Start.Line <= o.End.Line+endDelta && md.Start.Line <= md.End.Line+endDelta {
		startDelta = 1
	}
	return Change{
		Original: LineRange{o.Start.Line + startDelta, o.End.Line + 1 + endDelta},
		Modified: LineRange{md.Start.Line + startDelta, md.End.Line + 1 + endDelta},
		Inner:    []RangeMapping{m},
	}
}

// groupChanges merges the line ranges of sorted character mappings into
// changes; mappings whose line ranges touch end up in one change.
func groupChanges(mappings []RangeMapping, orig, mod []string) []Change {
	var out []Change
	var prev Change
	for i, m := range mappings {
		ch := lineChange(m, orig, mod)
		if i > 0 && (prev.Original.intersectsOrTouches(ch.Original) || prev.Modified.intersectsOrTouches(ch.Modified)) {
			last := &out[len(out)-1]
			last.Original = last.Original.join(ch.Original)
			last.Modified = last.Modified.join(ch.Modified)
			last.Inner = append(last.Inner, m)
		} else {
			out = append(out, ch)
		}
		prev = ch
	}
	return out
}
