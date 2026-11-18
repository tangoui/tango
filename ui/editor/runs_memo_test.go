package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// runsMemoView is a highlighted Go document with brackets on every line,
// shown in a 640×400 view with the caret at line 10.
func runsMemoView(t *testing.T) (*State, func() *ViewW) {
	t.Helper()
	var doc strings.Builder
	doc.WriteString("package p\n\nfunc f() {\n")
	for i := range 200 {
		doc.WriteString("\tv := g(h(a[")
		doc.WriteString(strings.Repeat("x", i%7))
		doc.WriteString("]), \"s\") // c\n")
	}
	doc.WriteString("}\n")
	return runsMemoDoc(t, doc.String())
}

// runsMemoDoc shows the Go text in a highlighted 640×400 view.
func runsMemoDoc(t *testing.T, text string) (*State, func() *ViewW) {
	t.Helper()
	goText(t)
	var doc strings.Builder
	doc.WriteString(text)
	st := NewState(doc.String(), "memo.go", theme.Base("vs-dark"))
	t.Cleanup(st.Close)
	st.Highlighter().HighlightFor(st.hlKey, st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	d, vw := viewDriver(st, 640, 400, nil)
	frame := func() *ViewW {
		snap := st.Snapshot()
		st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		d.Frame()
		return *vw
	}
	for range 3 {
		frame()
	}
	return st, frame
}

// TestStyleRunsKeptForUnchangedRows: typing a letter recomputes the style
// runs of the edited row only; a frame without a change recomputes none.
func TestStyleRunsKeptForUnchangedRows(t *testing.T) {
	st, frame := runsMemoView(t)
	w := frame()
	built := w.vs.runsBuilt
	w = frame()
	if got := w.vs.runsBuilt - built; got != 0 {
		t.Fatalf("a frame without a change recomputed %d rows' style runs, want 0", got)
	}
	for k := range 3 {
		snap := st.Snapshot()
		at := snap.LineStart(8+k) + 2
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "z"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		built = w.vs.runsBuilt
		w = frame()
		if got := w.vs.runsBuilt - built; got > 1 {
			t.Fatalf("typing a letter on line %d recomputed %d rows' style runs, want at most 1", 8+k, got)
		}
	}
}

// TestStyleRunsMemoMatchesFresh: after edits of every kind, a theme
// switch, a font size change and control characters toggled, the rows
// drawn with the kept style runs fingerprint (damage.go: text, colors,
// bold/italic/underline) exactly as rows computed afresh, and a change of
// theme or font size recomputes every row.
func TestStyleRunsMemoMatchesFresh(t *testing.T) {
	st, frame := runsMemoView(t)
	r := rand.New(rand.NewPCG(5, 9))
	inserts := []string{"z", "(", ")", "[", "]", "{", "}", "\"", "/*", "*/", "//", "\n", "\t", "\x01"}
	kept, dark := 0, true
	for step := range 120 {
		w := frame()
		switch step % 10 {
		case 7:
			dark = !dark
			if dark {
				st.Theme = theme.Base("vs-dark")
			} else {
				st.Theme = theme.Base("vs")
			}
		case 8:
			if st.Options.FontSize == 12 {
				st.Options.FontSize = 16
			} else {
				st.Options.FontSize = 12
			}
		case 9:
			st.Options.RenderControlCharacters = !st.Options.RenderControlCharacters
		default:
			snap := st.Snapshot()
			line := 3 + r.IntN(20)
			at := snap.LineStart(line) + r.IntN(len(snap.Line(line))+1)
			if step%5 == 4 && at < snap.Len() {
				_ = st.Edit([]buffer.Edit{{Start: at, End: at + 1}}, Selection{})
			} else {
				_ = st.Edit([]buffer.Edit{{Start: at, End: at, Text: inserts[r.IntN(len(inserts))]}}, Selection{})
			}
		}
		built := w.vs.runsBuilt
		w = frame()
		got := slices.Clone(w.vs.dmg.rows)
		n := w.vs.runsBuilt - built
		if step%10 == 7 || step%10 == 8 {
			if int(n) < len(got) {
				t.Fatalf("step %d: theme or font size changed but only %d of %d rows recomputed", step, n, len(got))
			}
		} else if int(n) < len(got)/2 {
			kept++
		}
		w.vs.runMemo = nil // the next frame computes every row afresh
		w = frame()
		if !slices.Equal(got, w.vs.dmg.rows) {
			t.Fatalf("step %d (kind %d): rows with kept style runs differ from fresh ones", step, step%10)
		}
	}
	if kept == 0 {
		t.Fatal("no frame kept style runs: the test does not exercise the memo")
	}
}

// TestStyleRunsMemoFollowsTokens: opening a block comment recolors the
// unchanged lines below it: their text is the same, their tokens are not.
func TestStyleRunsMemoFollowsTokens(t *testing.T) {
	st, frame := runsMemoDoc(t, "package p\n\nvar x = 1\n"+strings.Repeat("var y = \"s\" + 2\n", 100))
	frame()
	at := st.Snapshot().LineStart(5)
	if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "/*"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	w := frame()
	got := slices.Clone(w.vs.dmg.rows)
	w.vs.runMemo = nil
	w = frame()
	if !slices.Equal(got, w.vs.dmg.rows) {
		t.Fatal("rows below a new block comment kept their old colors")
	}
}
