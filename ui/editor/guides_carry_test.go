package editor

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// guideCarryState is a Go document of nested blocks with the view at lines
// 66..105 and rows with their text (as layout gives them), so the active
// guide of a caret elsewhere is computed from the whole block (activeGuide's
// cached path). Lines 63..73 are an if block whose end is in the view, so a
// caret at line 64 has a block that a one-line shift visibly moves.
const viewTop = 66

func guideCarryState(t *testing.T) (*State, *ViewW, func() []lineView) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("package p\n\nfunc f() {\n")
	for i := range 3000 {
		switch i % 50 {
		case 10:
			sb.WriteString("\tif x {\n")
		case 20:
			sb.WriteString("\t}\n")
		case 30:
			sb.WriteString("\n") // a blank line inside the block
		default:
			if i%50 > 10 && i%50 < 20 {
				fmt.Fprintf(&sb, "\t\ty%d := g(%d)\n", i, i)
			} else {
				fmt.Fprintf(&sb, "\tx%d := g(%d)\n", i, i)
			}
		}
	}
	sb.WriteString("}\n")
	st := NewState(sb.String(), "a.go", theme.Base("vs"))
	t.Cleanup(st.Close)
	vs := newViewState()
	w := &ViewW{st: st, opts: DefaultOptions(), vs: &vs}
	rows := func() []lineView {
		snap := st.Snapshot()
		out := make([]lineView, 40)
		for k := range out {
			i := viewTop + k
			out[k] = lineView{i: i, vi: i, rows: 1, text: snap.Line(i)}
		}
		return out
	}
	return st, w, rows
}

// TestActiveGuideCarriedOverEditsElsewhere: an edit that changes no line's
// indentation (typing inside a line, here far from the view and the caret)
// keeps the active block of a caret outside the view: it is carried to the
// new text, not searched again over the whole block.
func TestActiveGuideCarriedOverEditsElsewhere(t *testing.T) {
	st, w, rows := guideCarryState(t)
	caret := 2000
	want := w.guidesFor(st.Snapshot(), rows(), caret)
	scans := w.vs.guideScans
	for _, line := range []int{1500, 120, 2000, 2900} {
		snap := st.Snapshot()
		at := snap.LineStart(line) + len(snap.Line(line)) // end of the line
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "z"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		got := w.guidesFor(st.Snapshot(), rows(), caret)
		if !slices.Equal(got, want) {
			t.Fatalf("after typing on line %d: guides %v, want %v", line, got, want)
		}
	}
	if n := w.vs.guideScans - scans; n != 0 {
		t.Fatalf("typing inside lines searched the active block %d times, want 0 (carried)", n)
	}
}

// TestActiveGuideCarryMatchesFreshSearch: after every kind of edit
// (typing inside a line, indenting or outdenting a line, emptying or
// filling a blank line, inserting or joining lines, several edits at once,
// undo and redo) the active guide equals a fresh search of the new text.
func TestActiveGuideCarryMatchesFreshSearch(t *testing.T) {
	st, w, rows := guideCarryState(t)
	freshView := func() *ViewW {
		vs := newViewState()
		return &ViewW{st: st, opts: DefaultOptions(), vs: &vs}
	}
	carets := []int{64, 2000, 30} // inside the if block above the view, below, above
	views := []*ViewW{w, freshView(), freshView()}
	for k, caret := range carets {
		views[k].guidesFor(st.Snapshot(), rows(), caret)
	}
	carried := 0
	r := rand.New(rand.NewPCG(7, 11))
	for step := range 300 {
		ver := st.Snapshot().Version()
		snap := st.Snapshot()
		n := snap.LineCount()
		line := 40 + r.IntN(min(n-40, 120)) // around the view and the if block
		if step%5 == 4 {
			line = 40 + r.IntN(min(n-40, 2500)) // sometimes far away
		}
		ls := snap.LineStart(line)
		text := snap.Line(line)
		var edits []buffer.Edit
		switch step % 8 {
		case 0: // type inside the line, after its indentation
			at := ls + len(text)
			edits = []buffer.Edit{{Start: at, End: at, Text: "q"}}
		case 1: // indent
			edits = []buffer.Edit{{Start: ls, End: ls, Text: "\t"}}
		case 2: // outdent
			if strings.HasPrefix(text, "\t") {
				edits = []buffer.Edit{{Start: ls, End: ls + 1}}
			}
		case 3: // empty the line (it becomes blank)
			edits = []buffer.Edit{{Start: ls, End: ls + len(text)}}
		case 4: // insert a line break
			at := ls + len(text)/2
			edits = []buffer.Edit{{Start: at, End: at, Text: "\n\t\t"}}
		case 5: // join with the next line
			if line+1 < n {
				end := ls + len(text)
				edits = []buffer.Edit{{Start: end, End: end + 1}}
			}
		case 6: // several edits at once, on lines before and after the view
			a, b := snap.LineStart(45), snap.LineStart(line)+len(text)
			if a < b {
				edits = []buffer.Edit{{Start: a, End: a, Text: "w"}, {Start: b, End: b, Text: "w"}}
			}
		case 7:
			if r.IntN(2) == 0 {
				st.Undo()
			} else {
				st.Redo()
			}
		}
		if len(edits) > 0 {
			if err := st.Edit(edits, Selection{}); err != nil {
				t.Fatal(err)
			}
		}
		// Each persistent view keeps its own off-screen caret across the
		// steps, so its cached block is carried whenever activeCarries
		// allows; a new view searches the same text afresh.
		for k, caret := range carets {
			scans := views[k].vs.guideScans
			got := views[k].guidesFor(st.Snapshot(), rows(), caret)
			if st.Snapshot().Version() != ver && views[k].vs.guideScans == scans {
				carried++ // a new text, no search: the block was carried
			}
			fresh := freshView().guidesFor(st.Snapshot(), rows(), caret)
			if !slices.Equal(got, fresh) {
				t.Fatalf("step %d (kind %d, line %d, caret %d): carried guides %v, fresh %v", step, step%8, line, caret, got, fresh)
			}
		}
	}
	if carried == 0 {
		t.Fatal("no step carried a block: the test does not exercise activeCarries")
	}
}
