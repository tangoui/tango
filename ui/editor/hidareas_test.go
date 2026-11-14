package editor

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// funcsDoc is a Go document of n functions of 6 lines each (after 2 lines
// of header): foldable bodies.
func funcsDoc(n int) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := range n {
		fmt.Fprintf(&b, "func f%d(x int) int {\n\tif x > %d {\n\t\treturn x\n\t}\n\treturn 0\n}\n", i, i)
	}
	return b.String()
}

// TestHiddenAreasAPI: hidden areas hide their lines in the view (not in
// another view of the Model), follow edits, report their owner and never
// hide every line.
func TestHiddenAreasAPI(t *testing.T) {
	st := NewState(manyLines(30), "a.txt", nil)
	other := NewView(st.Model, nil)
	st.SetHiddenAreas("diff", []LineRange{{5, 10}, {20, 20}, {25, 27}})
	if got := st.HiddenAreas("diff"); !slices.Equal(got, []LineRange{{5, 10}, {25, 27}}) {
		t.Fatalf("HiddenAreas = %v", got)
	}
	n := st.Snapshot().LineCount()
	if c := st.viewCount(); c != n-7 {
		t.Fatalf("view lines %d, want %d", c, n-7)
	}
	if c := other.viewCount(); c != n {
		t.Fatalf("other view hides lines: %d of %d", c, n)
	}
	if o, ok := st.HiddenAreaAt(7); !ok || o != "diff" {
		t.Fatalf("HiddenAreaAt(7) = %q, %v", o, ok)
	}
	if _, ok := st.HiddenAreaAt(10); ok {
		t.Fatal("line 10 reported hidden")
	}
	// Two lines inserted by the other view above the areas move them.
	other.MoveCaret(0, false)
	other.Insert("a\nb\n")
	if got := st.HiddenAreas("diff"); !slices.Equal(got, []LineRange{{7, 12}, {27, 29}}) {
		t.Fatalf("after inserting 2 lines: %v", got)
	}
	if !st.vl.hidden(7) || st.vl.hidden(6) || !st.vl.hidden(11) || st.vl.hidden(12) {
		t.Fatalf("hidden lines %v", st.vl.hid)
	}
	// Every line hidden: the first stays.
	st.SetHiddenAreas("all", []LineRange{{0, 1000}})
	if c := st.viewCount(); c != 1 || st.vl.hidden(0) {
		t.Fatalf("everything hidden: %d view lines, hid %v", c, st.vl.hid)
	}
	st.SetHiddenAreas("all", nil)
	st.SetHiddenAreas("diff", nil)
	if c := st.viewCount(); c != st.Snapshot().LineCount() || st.tracks() || len(st.Model.trackers) != 0 {
		t.Fatalf("after removing: %d view lines, tracked %v", c, st.Model.trackers)
	}
	st.SetHiddenAreas("x", []LineRange{{1, 2}})
	st.Close()
	if len(st.Model.trackers) != 0 {
		t.Fatal("closed view still tracked")
	}
}

// TestHiddenAreasWithFolds: random folds, unfolds, hidden areas and edits
// (by this view and another, across the areas' edges): the view hides
// exactly the union of the folded regions and the areas, the fold
// commands never show an area, and the view lines stay consistent.
func TestHiddenAreasWithFolds(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 11))
	st := NewState(funcsDoc(40), "main.go", nil)
	other := NewView(st.Model, nil)
	defer other.Close()
	check := func(step string) {
		t.Helper()
		vl := st.lines()
		n := vl.n
		want := make([]bool, n)
		for _, sp := range st.fold.hidden(n) {
			for l := sp.a; l <= sp.b; l++ {
				want[l] = true
			}
		}
		for _, o := range []string{"a", "b"} {
			for _, lr := range st.HiddenAreas(o) {
				for l := lr.Start; l < lr.End && l < n; l++ {
					want[l] = true
				}
			}
		}
		if !slices.Contains(want, false) {
			want[0] = false
		}
		hidden := 0
		for l := range n {
			if vl.hidden(l) != want[l] {
				t.Fatalf("%s: line %d hidden %v, want %v (hid %v, folds %v)", step, l, vl.hidden(l), want[l], vl.hid, st.fold.hidden(n))
			}
			if want[l] {
				hidden++
			}
		}
		if vl.Count() != n-hidden {
			t.Fatalf("%s: Count %d, want %d", step, vl.Count(), n-hidden)
		}
		for v := range vl.Count() {
			l, row := vl.find(v)
			if vl.hidden(l) || row != 0 || vl.viewOf(l) != v {
				t.Fatalf("%s: find(%d) = %d, %d (viewOf %d)", step, v, l, row, vl.viewOf(l))
			}
		}
	}
	randAreas := func() []LineRange {
		n := st.Snapshot().LineCount()
		var out []LineRange
		for range r.IntN(4) {
			a := r.IntN(n)
			out = append(out, LineRange{a, a + 1 + r.IntN(8)})
		}
		return out
	}
	for step := range 400 {
		snap := st.Snapshot()
		n := snap.LineCount()
		what := ""
		switch k := r.IntN(12); {
		case k < 2:
			o := []string{"a", "b"}[r.IntN(2)]
			st.SetHiddenAreas(o, randAreas())
			what = "areas " + o
		case k < 4:
			l := 2 + 6*r.IntN(40)
			if l < n {
				st.MoveCaret(snap.LineStart(l), false)
				st.Execute("editor.fold", nil)
			}
			what = "fold"
		case k == 4:
			st.Execute("editor.unfoldAll", nil)
			what = "unfold all"
		case k == 5:
			st.Execute("editor.foldAll", nil)
			what = "fold all"
		case k < 9:
			v := []*State{st, other}[r.IntN(2)]
			l := r.IntN(n)
			v.MoveCaret(snap.LineStart(l), false)
			v.Insert([]string{"x\n", "\n\n", "y"}[r.IntN(3)])
			what = "insert"
		default:
			v := []*State{st, other}[r.IntN(2)]
			l := r.IntN(n)
			a := snap.LineStart(l)
			b := min(snap.Len(), a+1+r.IntN(60))
			v.SetSelections([]Selection{{Anchor: a, Head: b}})
			v.Delete(Backward, Char)
			what = "delete"
		}
		check(fmt.Sprintf("step %d (%s)", step, what))
		if what == "unfold all" && len(st.fold.folds) != 0 {
			t.Fatal("unfold all left folds")
		}
	}
	// Unfold all leaves the areas hidden.
	st = NewState(funcsDoc(4), "main.go", nil)
	st.Execute("editor.foldAll", nil)
	st.SetHiddenAreas("a", []LineRange{{4, 9}})
	st.Execute("editor.unfoldAll", nil)
	if !st.lines().hidden(5) {
		t.Fatal("unfold all showed a hidden area")
	}
}

// TestHiddenAreasCaret: Left and Right step over a hidden area as over a
// folded region; Up and Down skip it; a caret moved into it (a click
// elsewhere, an edit) stays there: the editor does not show the area.
func TestHiddenAreasCaret(t *testing.T) {
	st := NewState("l0\nl1\nl2\nl3\nl4\nl5\n", "a.txt", nil)
	st.SetHiddenAreas("diff", []LineRange{{2, 4}})
	snap := st.Snapshot()
	st.MoveCaret(lineEnd(snap, 1), false)
	st.Move(Forward, Char, false)
	if h := st.Primary().Head; h != snap.LineStart(4) {
		t.Fatalf("Right from line 1's end: %d, want line 4's start %d", h, snap.LineStart(4))
	}
	st.Move(Backward, Char, false)
	if h := st.Primary().Head; h != lineEnd(snap, 1) {
		t.Fatalf("Left back: %d, want %d", h, lineEnd(snap, 1))
	}
	st.MoveCaret(snap.LineStart(1), false)
	st.Move(Forward, Line, false)
	if l := snap.LineOf(st.Primary().Head); l != 4 {
		t.Fatalf("Down from line 1: line %d, want 4", l)
	}
	st.MoveCaret(snap.LineStart(3)+1, false)
	if !st.lines().hidden(3) || st.HiddenAreas("diff") == nil {
		t.Fatal("moving the caret into the area showed it")
	}
	// Hidden lines at the top: Left from the first shown line stays.
	st.SetHiddenAreas("diff", []LineRange{{0, 2}})
	st.MoveCaret(snap.LineStart(2), false)
	st.Move(Backward, Char, false)
	if h := st.Primary().Head; h != snap.LineStart(2) {
		t.Fatalf("Left from the first shown line: %d, want %d", h, snap.LineStart(2))
	}
}

// TestHiddenAreasDamageVerified runs hidden areas through the strict
// verifier: setting and removing them redraws the whole view (like
// folding), typing and other views' edits around them, scrolling over
// them, folding next to them.
func TestHiddenAreasDamageVerified(t *testing.T) {
	goText(t)
	th := theme.Base("vs-dark")
	st := NewState(funcsDoc(40), "main.go", th)
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, 1.25)
	h.focusAt(3)
	whole := func(what string) {
		t.Helper()
		if h.frame() {
			t.Fatalf("%s: drawn partially", what)
		}
		dam, _ := h.lineHints()
		r := h.vw.vs.rect
		if !slices.ContainsFunc(dam, func(d geom.Rect) bool { return d.Intersect(r) == r }) {
			t.Fatalf("%s: damage %v does not cover the view %v", what, dam, r)
		}
	}
	st.SetHiddenAreas("diff", []LineRange{{6, 12}, {20, 30}})
	whole("set areas")
	for i := range 4 {
		h.typeText(string(rune('a' + i)))
		h.step("type")
	}
	h.key(input.KeyEnter, 0)
	h.step("enter above the areas")
	h.key(input.KeyDown, 0)
	h.step("down")
	for range 3 {
		h.key(input.KeyDown, 0)
		h.step("down over an area")
	}
	other := NewView(st.Model, th)
	defer other.Close()
	other.MoveCaret(other.Snapshot().LineStart(14), false)
	other.Insert("x\ny\n")
	h.step("other view inserts between the areas")
	p := geom.Pt(240, 180)
	for _, dy := range []float32{8, 16, 4, 2.4, 30, -8, -4} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step(fmt.Sprintf("scroll %v", dy))
	}
	caretAt(st, 2, 3)
	h.step("caret")
	st.Execute("editor.fold", nil)
	whole("fold before an area")
	st.Execute("editor.unfoldAll", nil)
	whole("unfold all")
	st.SetHiddenAreas("diff", nil)
	whole("remove areas")
	h.typeText("z")
	h.step("type")
	h.verified()
}
