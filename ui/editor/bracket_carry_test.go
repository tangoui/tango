package editor

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// brCarryDoc is Go text with nested brackets of every kind on most lines
// and some brackets in strings and comments (not brackets).
func brCarryDoc(lines int) string {
	var sb strings.Builder
	sb.WriteString("package p\n\nfunc f() {\n")
	for i := range lines {
		switch i % 9 {
		case 0:
			fmt.Fprintf(&sb, "\tif x%d(a[%d], b) {\n", i, i)
		case 4:
			sb.WriteString("\t}\n")
		case 6:
			fmt.Fprintf(&sb, "\ts := \"(([%d\" // )]{\n", i)
		default:
			fmt.Fprintf(&sb, "\t\tv%d := g(h(a[%d]), m{k: %d})\n", i, i, i)
		}
	}
	sb.WriteString("}\n")
	return sb.String()
}

// brCarryView returns a fully highlighted State and its view, the bracket
// tree known everywhere, and spans() giving the bracket colors of lines
// lo..hi through the view's cache (bracketSpans), as a frame asks them.
func brCarryView(t *testing.T, text string, lo, hi int) (*State, *ViewW, func() map[int][]brSpan) {
	t.Helper()
	th := theme.Base("vs-dark")
	st := highlighted(t, text, "a.go", th)
	t.Cleanup(st.Close)
	vs := newViewState()
	w := &ViewW{st: st, opts: DefaultOptions(), vs: &vs}
	spans := func() map[int][]brSpan {
		snap := st.Snapshot()
		if hl := st.Highlighter(); hl != nil {
			hl.Highlight(snap, snap.LineCount(), far.Sub(far.Add(-60e9)))
		}
		st.lines() // sync the view lines to the text
		st.Model.beginBracketFrame()
		return w.bracketSpans(snap, st.lines(), lo, min(hi, snap.LineCount()-1), min(hi, snap.LineCount()-1), th, far)
	}
	return st, w, spans
}

// TestBracketColorsCarriedOverTyping: typing that adds or removes no
// bracket and leaves the line's tokenizer end state alone recomputes the
// colors of the edited line only; the other lines in view keep theirs.
func TestBracketColorsCarriedOverTyping(t *testing.T) {
	st, w, spans := brCarryView(t, brCarryDoc(400), 100, 160)
	want := spans()
	if len(want) < 40 {
		t.Fatalf("only %d lines with colored brackets in view", len(want))
	}
	for k := range 5 {
		line := 120 + k
		snap := st.Snapshot()
		ls := snap.LineStart(line)
		at := ls + strings.IndexByte(snap.Line(line), 'v') + 1 // inside an identifier
		if at <= ls {
			at = ls + 1
		}
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "z"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		before := w.vs.brcLines
		got := spans()
		if n := w.vs.brcLines - before; n > 1 {
			t.Fatalf("typing a letter on line %d recomputed %d lines' bracket colors, want at most 1", line, n)
		}
		fresh := freshBracketSpans(t, st, 100, 160)
		if !maps.EqualFunc(got, fresh, slices.Equal) {
			t.Fatalf("after typing on line %d: carried colors differ from a fresh computation", line)
		}
	}
}

// freshBracketSpans computes lines lo..hi's bracket colors with a new view
// (no cache) on the same text.
func freshBracketSpans(t *testing.T, st *State, lo, hi int) map[int][]brSpan {
	t.Helper()
	vs := newViewState()
	w := &ViewW{st: st, opts: DefaultOptions(), vs: &vs}
	snap := st.Snapshot()
	st.Model.beginBracketFrame()
	hi = min(hi, snap.LineCount()-1)
	return w.bracketSpans(snap, st.lines(), lo, hi, hi, st.Theme, far)
}

// TestBracketColorsCarryMatchesFresh: after every kind of edit — typing a
// letter, adding or removing an opening or closing bracket (matching or
// not), opening or closing a string or a comment, adding or joining lines,
// several edits at once, undo and redo — the carried colors equal a fresh
// computation, in view of the edit and away from it.
func TestBracketColorsCarryMatchesFresh(t *testing.T) {
	st, w, spans := brCarryView(t, brCarryDoc(300), 100, 150)
	spans()
	r := rand.New(rand.NewPCG(3, 5))
	inserts := []string{"z", "(", ")", "[", "]", "{", "}", "\"", "//", "/*", "*/", "`", "\n", "\t", " "}
	carried := 0
	for step := range 400 {
		snap := st.Snapshot()
		n := snap.LineCount()
		line := 90 + r.IntN(min(n-90, 80))
		if step%7 == 6 {
			line = 3 + r.IntN(n-4) // anywhere
		}
		ls := snap.LineStart(line)
		text := snap.Line(line)
		at := ls + r.IntN(len(text)+1)
		var edits []buffer.Edit
		switch step % 6 {
		case 0, 1, 2:
			edits = []buffer.Edit{{Start: at, End: at, Text: inserts[r.IntN(len(inserts))]}}
		case 3: // delete one byte (a bracket, a quote, a letter, a line break)
			if at < snap.Len() {
				edits = []buffer.Edit{{Start: at, End: at + 1}}
			}
		case 4: // two edits on two lines
			a := snap.LineStart(max(3, line-5)) + 1
			if a < at {
				edits = []buffer.Edit{{Start: a, End: a, Text: inserts[r.IntN(len(inserts))]}, {Start: at, End: at, Text: "q"}}
			}
		case 5:
			if r.IntN(2) == 0 {
				st.Undo()
			} else {
				st.Redo()
			}
		}
		if len(edits) > 0 {
			if err := st.Edit(edits, Selection{}); err != nil {
				continue // an edit splitting a character: skip it
			}
		}
		before := w.vs.brcLines
		got := spans()
		if w.vs.brcLines-before <= 1 && st.Snapshot().Version() != snap.Version() {
			carried++
		}
		fresh := freshBracketSpans(t, st, 100, 150)
		if !maps.EqualFunc(got, fresh, slices.Equal) {
			for i := range fresh {
				if !slices.Equal(got[i], fresh[i]) {
					t.Fatalf("step %d (kind %d, edit line %d): line %d carried %v, fresh %v", step, step%6, line, i, got[i], fresh[i])
				}
			}
			t.Fatalf("step %d: carried colors have extra lines", step)
		}
	}
	if carried == 0 {
		t.Fatal("no step carried the colors: the test does not exercise the carry")
	}
}

// TestBracketColorsCarryOuterPairChange: an edit that leaves a line's end
// stack and lowest depth alone but changes how it closes the brackets of
// earlier lines (a closing brace popping an unclosed parenthesis, then a
// parenthesis closing it first) recolors those earlier lines, on a short
// line and on a line too long to tokenize.
func TestBracketColorsCarryOuterPairChange(t *testing.T) {
	for _, long := range []bool{false, true} {
		pad := "b"
		if long {
			pad = strings.Repeat("b ", maxTokenizationLineLength/2+1)
		}
		text := "package p\n\nfunc f() {\n\tg(a,\n\t\t[c]\n\t" + pad + "}\n\th(c)\n}\nvar x = [1]\n"
		st, _, spans := brCarryView(t, text, 0, 20)
		spans()
		snap := st.Snapshot()
		at := snap.LineStart(5) + len(snap.Line(5)) - 1 // before the closing brace
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: ")"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		for k := range 2 {
			got := spans()
			if fresh := freshBracketSpans(t, st, 0, 20); !maps.EqualFunc(got, fresh, slices.Equal) {
				for i := range fresh {
					if !slices.Equal(got[i], fresh[i]) {
						t.Fatalf("long %v, step %d: line %d carried %v, fresh %v", long, k, i, got[i], fresh[i])
					}
				}
				t.Fatalf("long %v, step %d: carried colors have extra lines", long, k)
			}
			st.Undo()
		}
	}
}
