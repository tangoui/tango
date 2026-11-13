package editor

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/regex"
)

// Folding ranges (Monaco's indentRangeProvider and syntaxRangeProvider).
//
// A folding range is a span of model lines whose lines after the first can
// be hidden. The built-in strategy derives them from the indentation and
// the language's folding markers (Conf.Folding); a FoldingRangeProvider
// (milestone 3: the language server) replaces it under FoldingStrategy
// "auto".

// FoldingRange is a foldable span of model lines: Start is shown, Start+1
// .. End are hidden when it is folded. Lines are 0-based and inclusive.
type FoldingRange struct {
	Start, End int
	// Kind is FoldingKindComment, FoldingKindImports, FoldingKindRegion or
	// "" (Monaco's FoldingRangeKind). The built-in strategy leaves it ""
	// (as Monaco's indentation provider), so editor.foldAllBlockComments
	// and editor.foldAllMarkerRegions then find their ranges by the text
	// of the start line.
	Kind string
}

// Folding range kinds (Monaco's FoldingRangeKind values).
const (
	FoldingKindComment = "comment"
	FoldingKindImports = "imports"
	FoldingKindRegion  = "region"
)

// FoldingRangeProvider computes the folding ranges of a document
// (Monaco's FoldingRangeProvider). The editor calls it on the UI goroutine
// when the ranges of a new text version are first needed (a fold command,
// the gutter's folding controls), so it must answer quickly: a provider
// backed by a slow service answers from its last result. Ranges may come
// in any order; ranges outside the document, of a single line, starting on
// the same line as an earlier one or crossing an enclosing one are
// dropped. An error, or nil ranges, falls back to the built-in strategy
// (as Monaco does when every provider returns null).
type FoldingRangeProvider interface {
	FoldingRanges(ctx context.Context, snap buffer.Snapshot) ([]FoldingRange, error)
}

// FoldingStrategy is how folding ranges are computed (Monaco's
// foldingStrategy option).
type FoldingStrategy string

const (
	// FoldingAuto uses the registered FoldingRangeProvider, else the
	// indentation.
	FoldingAuto FoldingStrategy = "auto"
	// FoldingIndentation always uses the indentation (and the language's
	// folding markers).
	FoldingIndentation FoldingStrategy = "indentation"
)

// ShowFoldingControls is when the gutter shows the folding controls
// (Monaco's showFoldingControls option).
type ShowFoldingControls string

const (
	// FoldingControlsAlways always shows them.
	FoldingControlsAlways ShowFoldingControls = "always"
	// FoldingControlsNever never shows them, and the gutter leaves no room
	// for them.
	FoldingControlsNever ShowFoldingControls = "never"
	// FoldingControlsMouseOver shows the controls of folded ranges always
	// and the others while the pointer is over the gutter.
	FoldingControlsMouseOver ShowFoldingControls = "mouseover"
)

// foldingMaximumRegions is Monaco's foldingMaximumRegions default: beyond
// it the deepest ranges are dropped.
const foldingMaximumRegions = 5000

// indentLevel is Monaco's computeIndentLevel: the visual indentation of
// s (tabs to the next multiple of tab), -1 for a blank line.
func indentLevel(s string, tab int) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			n++
		case '\t':
			n = n - n%tab + tab
		default:
			return n
		}
	}
	return -1
}

// markerMatcher matches the language's folding markers (RegExp.exec /
// test: a match anywhere in the line).
type markerMatcher struct {
	// The patterns RE2 can run, searched for directly (an anchored
	// pattern fails at once on other lines): "(start)|(?:end)" when the
	// flags agree, else each.
	stdBoth, stdStart, stdEnd *regexp.Regexp
	// Otherwise the regex package's patterns behind a lazy prefix.
	both       regex.Regexp
	start, end regex.Regexp
}

// match reports whether line s matches the start or the end marker.
func (m *markerMatcher) match(s string) (start, end bool) {
	if m == nil {
		return false, false
	}
	switch {
	case m.stdBoth != nil:
		g := m.stdBoth.FindStringSubmatchIndex(s)
		if g == nil {
			return false, false
		}
		return g[2] >= 0, g[2] < 0
	case m.stdStart != nil:
		if m.stdStart.MatchString(s) {
			return true, false
		}
		return false, m.stdEnd.MatchString(s)
	case m.both != nil:
		g := m.both.MatchStart(s)
		if g == nil {
			return false, false
		}
		return g[2] >= 0, g[2] < 0
	}
	if test(m.start, s) {
		return true, false
	}
	return false, test(m.end, s)
}

// stdRegexp compiles a JavaScript regex for an unanchored search with
// RE2, nil when it cannot be translated.
func stdRegexp(source, flags string) *regexp.Regexp {
	g, ok := regex.Translate(source, flags)
	if !ok {
		return nil
	}
	re, err := regexp.Compile(g)
	if err != nil {
		return nil
	}
	return re
}

// markersOf compiles conf's folding markers, nil for none.
func markersOf(conf *langs.Conf) *markerMatcher {
	if conf == nil || conf.Folding.Start == nil || conf.Folding.End == nil {
		return nil
	}
	if v, ok := markerCache.Load(conf); ok {
		return v.(*markerMatcher)
	}
	s, e := conf.Folding.Start, conf.Folding.End
	m := &markerMatcher{}
	if s.Flags == e.Flags {
		m.stdBoth = stdRegexp(`(`+s.Source+`)|(?:`+e.Source+`)`, s.Flags)
	}
	if m.stdBoth == nil {
		m.stdStart, m.stdEnd = stdRegexp(s.Source, s.Flags), stdRegexp(e.Source, e.Flags)
		if m.stdStart == nil || m.stdEnd == nil {
			m.stdStart, m.stdEnd = nil, nil
			if s.Flags == e.Flags {
				m.both, _ = regex.Compile(`[\s\S]*?(?:(`+s.Source+`)|(?:`+e.Source+`))`, s.Flags)
			}
			if m.both == nil {
				m.start, m.end = compileTest(s), compileTest(e)
			}
		}
	}
	if m.stdBoth == nil && m.stdStart == nil && m.both == nil && m.start == nil && m.end == nil {
		m = nil
	}
	v, _ := markerCache.LoadOrStore(conf, m)
	return v.(*markerMatcher)
}

// indentRanges returns the folding ranges of snap by indentation and
// markers (Monaco's computeRanges, in the order of their start lines):
//
//   - a line starts a range when the next non-blank lines are indented
//     deeper; the range ends at the last of them, or, for off-side
//     languages (Python, YAML), at the last line before the next line at
//     the same or a lower indentation, blank lines at the end included only
//     when not off-side; ranges must span two lines at least;
//   - a line matching the start marker opens a range that ends at the
//     matching end marker line (included), whatever the indentation; an
//     unmatched start marker counts as ordinary text, an unmatched end
//     marker is ignored.
//
// Beyond limit ranges, the most deeply indented ones are dropped (Monaco's
// foldingMaximumRegions).
func indentRanges(snap buffer.Snapshot, tab int, offSide bool, markers *markerMatcher, limit int) []FoldingRange {
	tab = max(tab, 1)
	type region struct {
		indent    int // -1 sentinel, -2 an end marker
		endAbove  int
		line      int
		startLine int
	}
	n := snap.LineCount()
	// The lines' indentation and markers, read top down in blocks (a
	// Snapshot.Line per line costs two tree walks).
	info := make([]int32, n) // indent, -1 blank; markerStart / markerEnd bits
	const (
		markerStart = 1 << 30
		markerEnd   = 1 << 29
		indentMask  = markerEnd - 1
	)
	const block = 2048
	for l0 := 0; l0 < n; l0 += block {
		l1 := min(n, l0+block)
		end := snap.Len()
		if l1 < n {
			end = snap.LineStart(l1) - 1
		}
		text := snap.Slice(snap.LineStart(l0), end)
		for l := l0; l < l1; l++ {
			line := text
			if i := strings.IndexByte(text, '\n'); i >= 0 {
				line, text = text[:i], text[i+1:]
			}
			ind := indentLevel(line, tab)
			if ind < 0 {
				info[l] = -1
				continue
			}
			v := int32(min(ind, indentMask))
			if markers != nil {
				if st, en := markers.match(line); st {
					v |= markerStart
				} else if en {
					v |= markerEnd
				}
			}
			info[l] = v
		}
	}
	// Ranges are found bottom up; out is reversed at the end.
	var out []FoldingRange
	var indents []int // indent of each range in out
	stack := []region{{indent: -1, endAbove: n, line: n}}
	for line := n - 1; line >= 0; line-- {
		v := info[line]
		ind := int(v & indentMask)
		if v < 0 {
			ind = -1
		}
		prev := &stack[len(stack)-1]
		if ind == -1 {
			if offSide {
				// Blank lines belong to the block before them.
				prev.endAbove = line
			}
			continue
		}
		if v&(markerStart|markerEnd) != 0 {
			isStart, isEnd := v&markerStart != 0, v&markerEnd != 0
			if isStart {
				// Discard the regions up to the end marker.
				i := len(stack) - 1
				for i > 0 && stack[i].indent != -2 {
					i--
				}
				if i > 0 {
					stack = stack[:i+1]
					prev = &stack[i]
					out = append(out, FoldingRange{Start: line, End: prev.line})
					indents = append(indents, ind)
					prev.line, prev.indent, prev.endAbove = line, ind, line
					continue
				}
			} else if isEnd {
				stack = append(stack, region{indent: -2, endAbove: line, line: line})
				continue
			}
		}
		if prev.indent > ind {
			for prev.indent > ind {
				stack = stack[:len(stack)-1]
				prev = &stack[len(stack)-1]
			}
			if end := prev.endAbove - 1; end-line >= 1 {
				out = append(out, FoldingRange{Start: line, End: end})
				indents = append(indents, ind)
			}
		}
		if prev.indent == ind {
			prev.endAbove = line
		} else {
			stack = append(stack, region{indent: ind, endAbove: line, line: line})
		}
	}
	slices.Reverse(out)
	slices.Reverse(indents)
	if limit > 0 && len(out) > limit {
		// Keep the shallowest ranges (Monaco's RangesCollector).
		counts := map[int]int{}
		maxInd := 0
		for _, ind := range indents {
			counts[ind]++
			maxInd = max(maxInd, ind)
		}
		entries, cut := 0, maxInd+1
		for ind := 0; ind <= maxInd; ind++ {
			if c := counts[ind]; c > 0 {
				if c+entries > limit {
					cut = ind
					break
				}
				entries += c
			}
		}
		kept := out[:0]
		for k, r := range out {
			if ind := indents[k]; ind < cut || (ind == cut && entries < limit) {
				if ind == cut {
					entries++
				}
				kept = append(kept, r)
			}
		}
		out = kept
	}
	return out
}

// sanitizeRanges returns provider ranges valid for a document of n lines,
// sorted by start line, properly nested, one per start line, and at most
// limit of them (the most deeply nested dropped first), as Monaco's
// sanitizeRanges and FoldingRegions.sanitizeAndMerge leave them.
func sanitizeRanges(rs []FoldingRange, n, limit int) []FoldingRange {
	in := make([]FoldingRange, 0, len(rs))
	for _, r := range rs {
		if r.Start >= 0 && r.End > r.Start && r.End < n {
			in = append(in, r)
		}
	}
	slices.SortStableFunc(in, func(a, b FoldingRange) int { return cmp.Compare(a.Start, b.Start) })
	out := in[:0]
	var levels []int
	var stack []FoldingRange
	for _, r := range in {
		if k := len(out); k > 0 && out[k-1].Start == r.Start {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].End < r.Start {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && stack[len(stack)-1].End < r.End {
			continue // crosses its parent
		}
		levels = append(levels, len(stack))
		stack = append(stack, r)
		out = append(out, r)
	}
	if limit > 0 && len(out) > limit {
		counts := map[int]int{}
		maxL := 0
		for _, l := range levels {
			counts[l]++
			maxL = max(maxL, l)
		}
		entries, cut := 0, maxL+1
		for l := 0; l <= maxL; l++ {
			if c := counts[l]; c > 0 {
				if c+entries > limit {
					cut = l
					break
				}
				entries += c
			}
		}
		kept := out[:0]
		for k, r := range out {
			if l := levels[k]; l < cut || (l == cut && entries < limit) {
				if l == cut {
					entries++
				}
				kept = append(kept, r)
			}
		}
		out = kept
	}
	return out
}
