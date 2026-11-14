package editor

import (
	"image/color"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/theme"
)

// brPieces are the fragments of the long lines of the bracket index
// tests: brackets (balanced and not), word brackets with and without word
// boundaries, a case variant that stops the scan ("Do"), CJK, tabs and runs
// without brackets longer than a checkpoint segment.
var brPieces = []string{
	"(", ")", "[", "]", "{", "}", "a(b)[c]{d}", "f(x, [1, 2], {k: (v)}); ", "begin ", " end ", "beginx", "xend",
	"_begin ", "foo ", "  ", "\t", "中文（全角）", "x", "y", strings.Repeat("z", 5000), "((((", "))))",
}

func genBrLine(r *rand.Rand, n int) string {
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(brPieces[r.IntN(len(brPieces))])
		if r.IntN(4000) == 0 {
			sb.WriteString(" Do ")
		}
	}
	return sb.String()
}

// brEdit returns a random edit of snap: mostly inside a long line (small,
// sometimes a paste or a cut), sometimes a line break or an edit across
// lines, sometimes two edits of one line at once.
func brEdit(r *rand.Rand, snap buffer.Snapshot) []buffer.Edit {
	text := snap.String()
	pick := func() int {
		// A long line, mostly.
		for range 8 {
			l := r.IntN(snap.LineCount())
			if lineEnd(snap, l)-snap.LineStart(l) > maxTokenizationLineLength || r.IntN(8) == 0 {
				return fixBoundary(text, snap.LineStart(l)+r.IntN(lineEnd(snap, l)-snap.LineStart(l)+1))
			}
		}
		return fixBoundary(text, r.IntN(len(text)+1))
	}
	one := func(at int) buffer.Edit {
		end := at
		switch r.IntN(4) {
		case 0:
			end = at + r.IntN(8)
		case 1:
			if r.IntN(10) == 0 {
				end = at + r.IntN(30000)
			}
		}
		end = fixBoundary(text, min(end, len(text)))
		if end < at {
			end = at
		}
		// Not across a line break unless asked.
		if nl := strings.IndexByte(text[at:end], '\n'); nl >= 0 && r.IntN(10) != 0 {
			end = at + nl
		}
		ins := ""
		switch k := r.IntN(10); {
		case k < 6:
			ins = brPieces[r.IntN(len(brPieces))]
		case k < 7:
			ins = genBrLine(r, 64<<r.IntN(9))
		case k < 8 && r.IntN(4) == 0:
			ins = "\n"
		}
		return buffer.Edit{Start: at, End: end, Text: ins}
	}
	e := one(pick())
	if r.IntN(6) == 0 {
		// A second edit on the same line, after the first.
		l := snap.LineOf(e.End)
		if e2 := one(e.End + r.IntN(lineEnd(snap, l)-e.End+1)); e2.Start >= e.End && snap.LineOf(e2.End) == l {
			return []buffer.Edit{e, e2}
		}
	}
	return []buffer.Edit{e}
}

func brText(r *rand.Rand) string {
	return "f(a, {\n" + genBrLine(r, 150<<10) + "\n} end)\nbegin [\n" + genBrLine(r, 60<<10) + "\nx ]\n"
}

// TestBracketLongIndexMatchesFull: after random edits of long lines (and
// line breaks, edits across lines, several edits at once), a tree whose
// long lines follow the edits in their checkpoint indexes knows the same
// start stacks and lowest depths as a tree scanned from scratch without
// indexes, and its indexes equal fresh ones; with word brackets and a case
// variant that stops the scan.
func TestBracketLongIndexMatchesFull(t *testing.T) {
	for _, c := range []struct {
		name string
		conf *bracketConf
	}{
		{"plain", bracketConfOf(nil)},
		{"words", bracketConfOf(&langs.Conf{Brackets: [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}, {"begin", "end"}, {"Do", "od"}}})},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := rand.New(rand.NewPCG(5, uint64(len(c.name))))
			snap := buffer.New(brText(r)).Snapshot()
			tr := newBracketTree(c.conf, snap)
			tr.ensure(plainSource{}, snap.LineCount(), time.Time{})
			incremental := 0
			for step := range 300 {
				edits := brEdit(r, snap)
				ns, err := snap.Apply(edits)
				if err != nil {
					t.Fatal(err)
				}
				tr.edit(snap, ns, edits)
				pending := 0
				for _, ix := range tr.long {
					if ix.dlo >= 0 {
						pending++
					}
				}
				tr.ensure(plainSource{}, ns.LineCount(), time.Time{})
				snap = ns
				bracketLongIndexOff = true
				full := newBracketTree(c.conf, ns)
				full.ensure(plainSource{}, ns.LineCount(), time.Time{})
				bracketLongIndexOff = false
				if len(tr.start) != ns.LineCount() {
					t.Fatalf("step %d: %d lines, want %d", step, len(tr.start), ns.LineCount())
				}
				sameTree(t, tr, full, ns.LineCount())
				for l, ix := range tr.long {
					text := ns.Line(l)
					if len(text) <= maxTokenizationLineLength {
						continue
					}
					if ix.dlo >= 0 || ix.n != len(text) {
						t.Fatalf("step %d: line %d's index not up to date", step, l)
					}
					fresh := c.conf.indexLine(text, tr.start[l])
					if !sameIndex(c.conf, ix, fresh, text, tr.start[l]) {
						t.Fatalf("step %d: line %d: the index differs from a fresh one (%d vs %d checkpoints)", step, l, len(ix.cks), len(fresh.cks))
					}
				}
				incremental += pending
			}
			if incremental < 100 {
				t.Errorf("only %d incremental rescans", incremental)
			}
		})
	}
}

// sameIndex reports whether ix holds the states a scan of text from
// start has at ix's checkpoints (scanning from one to the next pauses
// exactly at it, with the same regexp start, stack and lowest depth), and
// the same end, as fresh.
func sameIndex(conf *bracketConf, ix, fresh *brIndex, text string, start *bnode) bool {
	if !stackEqual(ix.end, fresh.end) || ix.stop != fresh.stop || ix.minD() != fresh.minD() || ix.cks[0].p != 0 || !stackEqual(ix.cks[0].s, start) {
		return false
	}
	st := brState{s: start, minD: stackDepth(start)}
	p, fr := 0, true
	for k := range ix.cks {
		if c := ix.cks[k]; p != c.p || fr != c.fresh || !stackEqual(st.s, c.s) {
			return false
		}
		st.minD = stackDepth(st.s)
		var stopped bool
		p, fr, stopped = conf.scanRange(text, p, fr, len(text), ix.segEnd(k), func(d *bracketDef, b0, b1 int) { conf.apply(&st, d, b0, b1, nil) })
		if st.minD != ix.cks[k].min || (stopped && k != len(ix.cks)-1) {
			return false
		}
	}
	return stackEqual(st.s, ix.end)
}

// TestBracketLongQueriesMatchFull: matching, the enclosing pair and the
// colors of the rows in view, around random offsets of long lines after
// random edits, are those of the whole-line scans (bracketLongIndexOff).
func TestBracketLongQueriesMatchFull(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 3))
	th := theme.Base("vs")
	pal := paletteOf(th)
	st := newTestState(brText(r), "a.ts", th)
	queries := 0
	for step := range 120 {
		snap := st.Snapshot()
		edits := brEdit(r, snap)
		sels := make([]Selection, len(edits))
		for k, e := range edits {
			sels[k] = sel(e.Start, e.End)
		}
		// Several selections: the same text at each (one change of several
		// edits).
		st.SetSelections(sels)
		st.Insert(edits[0].Text)
		snap = st.Snapshot()
		text := snap.String()
		bracketLongIndexOff = true
		ref := newTestState(text, "a.ts", th)
		bracketLongIndexOff = false
		for range 12 {
			l := r.IntN(snap.LineCount())
			n := lineEnd(snap, l) - snap.LineStart(l)
			if n <= maxTokenizationLineLength {
				continue
			}
			off := snap.LineStart(l) + r.IntN(n+1)
			if r.IntN(2) == 0 {
				// Next to a bracket.
				if k := strings.IndexAny(text[off:lineEnd(snap, l)], "()[]{}"); k >= 0 {
					off += k + r.IntN(2)
				}
			}
			w0 := max(0, off-snap.LineStart(l)-r.IntN(3000))
			win := [2]int{w0, min(n, w0+r.IntN(6000))}
			got := longQueries(st, l, off, win, pal)
			bracketLongIndexOff = true
			want := longQueries(ref, l, off, win, pal)
			bracketLongIndexOff = false
			if got.unknown || want.unknown {
				continue
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("step %d: line %d offset %d window %v:\n got  %+v\n want %+v", step, l, off, win, got, want)
			}
			queries++
		}
	}
	if queries < 200 {
		t.Errorf("only %d queries", queries)
	}
}

type longQueryRes struct {
	unknown         bool
	match, encl     bracketPair
	matchOK, enclOK bool
	spans           []brSpan
}

// longQueries asks st about offset off of long line l and the colors of the
// bytes win of l.
func longQueries(st *State, l, off int, win [2]int, pal bracketPalette) longQueryRes {
	snap := st.Snapshot()
	if hl := st.Highlighter(); hl != nil {
		hl.Highlight(snap, snap.LineCount(), time.Minute)
	}
	t := st.bracketTreeFor(snap.LineCount()-1, far)
	if t == nil {
		// The Highlighter has not finished (its pause guard latched a
		// long line under load): nothing to compare.
		return longQueryRes{unknown: true}
	}
	var q longQueryRes
	q.match, q.matchOK = st.matchBracket(off, far)
	q.encl, q.enclOK = st.enclosingBrackets(off, far)
	if !q.matchOK {
		q.match = bracketPair{}
	}
	if !q.enclOK {
		q.encl = bracketPair{}
	}
	sp, _ := st.bracketColorsIn(t, []int{l}, map[int][][2]int{l: {win}}, pal, far, nil)
	for _, s := range sp[l] {
		if s.b1 > win[0] && s.b0 < win[1] {
			q.spans = append(q.spans, s)
		}
	}
	return q
}

// TestOverrideBoundsMatchesAll: overrideBounds of a row looks only at the
// spans that reach into the row, with the result of walking them all.
func TestOverrideBoundsMatchesAll(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 2))
	set := func(st theme.Style, c color.NRGBA) theme.Style {
		st.Foreground = c
		return st
	}
	for range 2000 {
		var over []brSpan
		for p := r.IntN(5); p < 2000; p += 1 + r.IntN(20) {
			n := 1 + r.IntN(3)
			over = append(over, brSpan{p, p + n, color.NRGBA{R: uint8(p)}})
			p += n
		}
		b0 := r.IntN(2000)
		n := r.IntN(300)
		var bounds []styleBound
		for a := 0; a < n || a == 0; a += 1 + r.IntN(40) {
			bounds = append(bounds, styleBound{a, theme.Style{Foreground: color.NRGBA{G: uint8(a)}}})
		}
		got := overrideBounds(slices.Clone(bounds), over, b0, n, set)
		want := overrideBoundsAll(slices.Clone(bounds), over, b0, n, set)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("b0 %d n %d:\n got  %v\n want %v", b0, n, got, want)
		}
		if lim := len(bounds) + 2*len(spansIn(over, b0, n)); cap(got) > lim {
			t.Fatalf("capacity %d for %d bounds and the row's spans (%d)", cap(got), len(bounds), lim)
		}
	}
}

func spansIn(over []brSpan, b0, n int) []brSpan {
	var out []brSpan
	for _, sp := range over {
		if sp.b1 > b0 && sp.b0 < b0+n {
			out = append(out, sp)
		}
	}
	return out
}

// overrideBoundsAll is overrideBounds walking every span (before Task 5b).
func overrideBoundsAll(bounds []styleBound, over []brSpan, b0, n int, set func(theme.Style, color.NRGBA) theme.Style) []styleBound {
	out := make([]styleBound, 0, len(bounds)+2*len(over))
	push := func(b styleBound) {
		if n := len(out); n > 0 && out[n-1].at == b.at {
			out[n-1] = b
			return
		}
		out = append(out, b)
	}
	k := 0
	cur := bounds[0].st
	for _, sp := range over {
		a, z := max(sp.b0, b0)-b0, min(sp.b1, b0+n)-b0
		if a >= z {
			continue
		}
		for ; k < len(bounds) && bounds[k].at <= a; k++ {
			if bounds[k].at < a {
				push(bounds[k])
			}
			cur = bounds[k].st
		}
		push(styleBound{a, set(cur, sp.c)})
		for ; k < len(bounds) && bounds[k].at <= z; k++ {
			cur = bounds[k].st
		}
		if z < n {
			push(styleBound{z, cur})
		}
	}
	for _, b := range bounds[k:] {
		push(b)
	}
	return out
}
