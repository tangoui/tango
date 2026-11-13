package editor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func rangesOf(t *testing.T, text, file string) []FoldingRange {
	t.Helper()
	st := NewState(text, file, nil)
	defer st.Close()
	return st.FoldingRanges()
}

func TestIndentFoldingRanges(t *testing.T) {
	py := "def f():\n    a = 1\n\n    if a:\n        b()\n\nx = 2\n"
	// Off-side: the blank line after a block is not part of it.
	if got, want := rangesOf(t, py, "a.py"), []FoldingRange{{0, 4, ""}, {3, 4, ""}}; !slices.Equal(got, want) {
		t.Errorf("python: %v, want %v", got, want)
	}
	// Not off-side (plain text): it is.
	if got, want := rangesOf(t, py, "a.txt"), []FoldingRange{{0, 5, ""}, {3, 5, ""}}; !slices.Equal(got, want) {
		t.Errorf("plain text: %v, want %v", got, want)
	}
	yaml := "a:\n  b: 1\n  c:\n    - x\n\nd: 2\n"
	if got, want := rangesOf(t, yaml, "a.yaml"), []FoldingRange{{0, 3, ""}, {2, 3, ""}}; !slices.Equal(got, want) {
		t.Errorf("yaml: %v, want %v", got, want)
	}
	// Braces: the closing line stays visible (Monaco's indentation
	// ranges end before it).
	goSrc := "func f() {\n\tif x {\n\t\ty()\n\t}\n}\n"
	if got, want := rangesOf(t, goSrc, "a.go"), []FoldingRange{{0, 3, ""}, {1, 2, ""}}; !slices.Equal(got, want) {
		t.Errorf("go: %v, want %v", got, want)
	}
	// Tabs count to the next tab stop.
	if got, want := rangesOf(t, "a\n\tb\n        c\nd", "a.txt"), []FoldingRange{{0, 2, ""}, {1, 2, ""}}; !slices.Equal(got, want) {
		t.Errorf("tabs: %v, want %v", got, want)
	}
	// Blank lines at the end of the document belong to the last block
	// (not off-side).
	if got, want := rangesOf(t, "a\n\tb\n\n", "a.txt"), []FoldingRange{{0, 3, ""}}; !slices.Equal(got, want) {
		t.Errorf("trailing blank lines: %v, want %v", got, want)
	}
	// The limit keeps the shallowest ranges.
	var b strings.Builder
	for range 3 {
		b.WriteString("a\n\tb\n\t\tc\n")
	}
	st := NewState(b.String(), "a.txt", nil)
	rs := indentRanges(st.Snapshot(), 4, false, nil, 3)
	if want := []FoldingRange{{0, 2, ""}, {3, 5, ""}, {6, 9, ""}}; !slices.Equal(rs, want) {
		t.Errorf("limited: %v, want %v", rs, want)
	}
}

func TestMarkerFolding(t *testing.T) {
	ts := "// #region setup\nconst a = 1;\nconst b = 2;\n// #endregion\nfunction f() {\n  g();\n}\n"
	if got, want := rangesOf(t, ts, "a.ts"), []FoldingRange{{0, 3, ""}, {4, 5, ""}}; !slices.Equal(got, want) {
		t.Errorf("markers: %v, want %v", got, want)
	}
	// Markers win over the indentation; nested regions; an unmatched end
	// marker is ignored.
	ts2 := "// #region outer\n  x;\n// #region inner\ny;\n// #endregion\n// #endregion\n// #endregion\nz;\n"
	if got, want := rangesOf(t, ts2, "a.ts"), []FoldingRange{{0, 5, ""}, {2, 4, ""}}; !slices.Equal(got, want) {
		t.Errorf("nested markers: %v, want %v", got, want)
	}
	st := NewState(ts, "a.ts", nil)
	st.MoveCaret(st.Snapshot().LineStart(5), false)
	if !st.Execute("editor.foldAllMarkerRegions", nil) {
		t.Fatal("foldAllMarkerRegions did not run")
	}
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 3, ""}}) {
		t.Errorf("folded %v", got)
	}
}

// foldGoDoc is a Go source with a block comment and two functions:
//
//	0 /*            (0,2)
//	3 func f() {    (3,6)
//	4 	if x {      (4,5)
//	8 func g() {    (8,9)
const foldGoDoc = "/*\n * doc\n */\nfunc f() {\n\tif x {\n\t\ty()\n\t}\n}\nfunc g() {\n\tz()\n}\n"

func caretAt(st *State, line, col int) { st.MoveCaret(st.Snapshot().LineStart(line)+col, false) }

func TestFoldCommands(t *testing.T) {
	st := NewState(foldGoDoc, "a.go", nil)
	snap := st.Snapshot()
	if got, want := st.FoldingRanges(), []FoldingRange{{0, 2, ""}, {3, 6, ""}, {4, 5, ""}, {8, 9, ""}}; !slices.Equal(got, want) {
		t.Fatalf("ranges %v, want %v", got, want)
	}
	run := func(id string, want ...FoldingRange) {
		t.Helper()
		if !st.Execute(id, nil) {
			t.Fatalf("%s did not run", id)
		}
		if got := st.Folded(); !slices.Equal(got, want) {
			t.Fatalf("%s: folded %v, want %v", id, got, want)
		}
	}
	// Fold from inside the inner block: the caret moves to the end of the
	// start line.
	caretAt(st, 5, 1)
	run("editor.fold", FoldingRange{4, 5, ""})
	if h := st.Primary().Head; h != lineEnd(snap, 4) {
		t.Fatalf("caret %d, want the end of line 4 (%d)", h, lineEnd(snap, 4))
	}
	if n := st.viewCount(); n != snap.LineCount()-1 {
		t.Fatalf("%d view lines, want %d", n, snap.LineCount()-1)
	}
	// Folded again: the first unfolded parent.
	run("editor.fold", FoldingRange{3, 6, ""}, FoldingRange{4, 5, ""})
	if n := st.viewCount(); n != snap.LineCount()-3 {
		t.Fatalf("%d view lines, want %d", n, snap.LineCount()-3)
	}
	run("editor.unfold", FoldingRange{4, 5, ""})
	run("editor.toggleFold", FoldingRange{3, 6, ""}, FoldingRange{4, 5, ""})
	run("editor.toggleFold", FoldingRange{4, 5, ""})
	run("editor.unfoldAll")
	run("editor.foldAll", FoldingRange{0, 2, ""}, FoldingRange{3, 6, ""}, FoldingRange{4, 5, ""}, FoldingRange{8, 9, ""})
	run("editor.unfoldAll")
	run("editor.foldRecursively", FoldingRange{3, 6, ""}, FoldingRange{4, 5, ""})
	run("editor.unfoldAll")
	// Level 1 except the region holding the caret; level 2.
	run("editor.foldLevel1", FoldingRange{0, 2, ""}, FoldingRange{8, 9, ""})
	run("editor.unfoldAll")
	caretAt(st, 0, 0)
	run("editor.foldLevel2", FoldingRange{4, 5, ""})
	run("editor.unfoldAll")
	run("editor.foldAllBlockComments", FoldingRange{0, 2, ""})

	// Moving down from a folded line skips its body.
	run("editor.unfoldAll")
	caretAt(st, 3, 0)
	run("editor.fold", FoldingRange{3, 6, ""})
	st.Move(Forward, Line, false)
	if l := st.Snapshot().LineOf(st.Primary().Head); l != 7 {
		t.Fatalf("down from the folded line went to line %d, want 7", l)
	}
	st.Move(Backward, Line, false)
	if l := st.Snapshot().LineOf(st.Primary().Head); l != 3 {
		t.Fatalf("up to the folded line went to line %d, want 3", l)
	}
	if got := st.Folded(); len(got) != 1 {
		t.Fatalf("vertical moves unfolded: %v", got)
	}
	// Arguments: selectionLines and levels.
	run("editor.unfoldAll")
	if !st.Execute("editor.fold", map[string]any{"selectionLines": []any{float64(8)}}) {
		t.Fatal("fold with args did not run")
	}
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{8, 9, ""}}) {
		t.Fatalf("selectionLines: %v", got)
	}

	// Folding off: the commands are refused and the folds go away.
	st.Options.Folding = false
	if st.Execute("editor.foldAll", nil) {
		t.Fatal("foldAll ran with folding off")
	}
	st.foldPrepare(false)
	if got := st.Folded(); len(got) != 0 || st.viewCount() != snap.LineCount() {
		t.Fatalf("folding off: folded %v, %d view lines", got, st.viewCount())
	}
}

type testProvider struct {
	rs    []FoldingRange
	err   error
	calls int
}

func (p *testProvider) FoldingRanges(_ context.Context, _ buffer.Snapshot) ([]FoldingRange, error) {
	p.calls++
	return p.rs, p.err
}

func TestFoldingProvider(t *testing.T) {
	st := NewState(foldGoDoc, "a.go", nil)
	p := &testProvider{rs: []FoldingRange{{8, 9, FoldingKindRegion}, {0, 2, FoldingKindComment}, {0, 1, ""}, {3, 9, ""}, {20, 30, ""}}}
	st.setFoldingProvider(p)
	// Sorted, one per start line, crossing and out-of-range ones dropped.
	if got, want := st.FoldingRanges(), []FoldingRange{{0, 2, FoldingKindComment}, {3, 9, ""}, {8, 9, FoldingKindRegion}}; !slices.Equal(got, want) {
		t.Fatalf("provider ranges %v, want %v", got, want)
	}
	st.FoldingRanges()
	if p.calls != 1 {
		t.Fatalf("%d provider calls for one text", p.calls)
	}
	// Kinds: the block comment command folds the comment ranges.
	st.Execute("editor.foldAllBlockComments", nil)
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}}) {
		t.Fatalf("comments folded %v", got)
	}
	st.Execute("editor.foldAllMarkerRegions", nil)
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}, {8, 9, ""}}) {
		t.Fatalf("regions folded %v", got)
	}
	// "indentation" ignores the provider; an error falls back to it.
	st.Execute("editor.unfoldAll", nil)
	st.Options.FoldingStrategy = FoldingIndentation
	if got := st.FoldingRanges(); len(got) != 4 {
		t.Fatalf("indentation strategy: %v", got)
	}
	st.Options.FoldingStrategy = FoldingAuto
	st.setFoldingProvider(&testProvider{err: errors.New("no server")})
	if got := st.FoldingRanges(); len(got) != 4 {
		t.Fatalf("failing provider: %v", got)
	}
}

func TestCaretEntersFoldUnfolds(t *testing.T) {
	st := NewState(foldGoDoc, "a.go", nil)
	fold := func() {
		t.Helper()
		st.Execute("editor.unfoldAll", nil)
		caretAt(st, 3, 0)
		st.Execute("editor.fold", nil)
		if len(st.Folded()) != 1 {
			t.Fatal("not folded")
		}
	}
	// Right from the end of the folded line into its body.
	fold()
	caretAt(st, 3, len(st.Snapshot().Line(3)))
	st.Move(Forward, Char, false)
	if got := st.Folded(); len(got) != 0 {
		t.Fatalf("caret moved into the fold: still folded %v", got)
	}
	// A caret put there.
	fold()
	caretAt(st, 5, 0)
	if got := st.Folded(); len(got) != 0 {
		t.Fatalf("caret set in the fold: still folded %v", got)
	}
	// Enter at the end of the folded line: the new line is in the region.
	fold()
	caretAt(st, 3, len(st.Snapshot().Line(3)))
	st.Insert("\n")
	if got := st.Folded(); len(got) != 0 {
		t.Fatalf("Enter at the folded line: still folded %v", got)
	}
	// Nested folds: the regions whose body holds the caret unfold, the
	// others stay (a region starting on the caret's line too).
	st.Execute("editor.foldAll", nil)
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}, {3, 7, ""}, {5, 6, ""}, {9, 10, ""}}) {
		t.Fatalf("foldAll: folded %v", got)
	}
	caretAt(st, 5, 0)
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}, {5, 6, ""}, {9, 10, ""}}) {
		t.Fatalf("nested: folded %v", got)
	}
	st.Execute("editor.foldAll", nil)
	caretAt(st, 6, 0)
	if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}, {9, 10, ""}}) {
		t.Fatalf("nested: folded %v", got)
	}
	// Another view's edit moving the caret into a fold.
	m := NewModel("a.go", foldGoDoc, DefaultRegistry())
	a, b := NewView(m, nil), NewView(m, nil)
	caretAt(b, 3, 0)
	b.Execute("editor.fold", nil)
	caretAt(b, 3, len(b.Snapshot().Line(3)))
	caretAt(a, 3, len(a.Snapshot().Line(3)))
	a.Insert("\n") // b's caret moves after the insertion, into the region
	if got := b.Folded(); len(got) != 0 {
		t.Fatalf("other view's insert moved the caret into the fold: still folded %v", got)
	}
}

func TestFoldSurvivesRemoteEdit(t *testing.T) {
	m := NewModel("a.go", foldGoDoc, DefaultRegistry())
	a, b := NewView(m, nil), NewView(m, nil)
	defer a.Close()
	defer b.Close()
	b.Viewport = Viewport{Height: 100, Width: 300, LineHeight: 10}
	caretAt(b, 0, 0)
	caretAt(b, 3, 0)
	b.Execute("editor.fold", nil)
	if got := b.Folded(); !slices.Equal(got, []FoldingRange{{3, 6, ""}}) {
		t.Fatalf("folded %v", got)
	}
	// Ten lines inserted above.
	a.Edit([]buffer.Edit{{Start: 0, End: 0, Text: strings.Repeat("// x\n", 10)}}, Selection{})
	if got := b.Folded(); !slices.Equal(got, []FoldingRange{{13, 16, ""}}) {
		t.Fatalf("after inserting above: %v", got)
	}
	if n, want := b.viewCount(), b.Snapshot().LineCount()-3; n != want {
		t.Fatalf("%d view lines, want %d", n, want)
	}
	if v := b.rowOf(b.Snapshot().LineStart(17), false); v != 14 {
		t.Fatalf("line 17 on view line %d, want 14", v)
	}
	// A line deleted inside the region.
	s := a.Snapshot()
	a.Edit([]buffer.Edit{{Start: s.LineStart(15), End: s.LineStart(16)}}, Selection{})
	if got := b.Folded(); !slices.Equal(got, []FoldingRange{{13, 15, ""}}) {
		t.Fatalf("after deleting inside: %v", got)
	}
	if n, want := b.viewCount(), b.Snapshot().LineCount()-2; n != want {
		t.Fatalf("%d view lines, want %d", n, want)
	}
	// Text typed on the start line keeps the fold.
	s = a.Snapshot()
	a.Edit([]buffer.Edit{{Start: s.LineStart(13), End: s.LineStart(13), Text: "// c\n"}}, Selection{})
	if got := b.Folded(); !slices.Equal(got, []FoldingRange{{14, 16, ""}}) {
		t.Fatalf("after inserting a line before the start line: %v", got)
	}
	// The start line deleted: the fold goes away.
	s = a.Snapshot()
	a.Edit([]buffer.Edit{{Start: s.LineStart(14), End: s.LineStart(15)}}, Selection{})
	if got := b.Folded(); len(got) != 0 {
		t.Fatalf("after deleting the start line: %v", got)
	}
	if n := b.viewCount(); n != b.Snapshot().LineCount() {
		t.Fatalf("%d view lines, want %d", n, b.Snapshot().LineCount())
	}
	// Undo brings the text back but not the fold (Monaco: the decoration
	// is gone).
	b.Execute("editor.unfoldAll", nil)
	caretAt(b, 13, 0)
	b.Execute("editor.fold", nil)
	s = a.Snapshot()
	a.Edit([]buffer.Edit{{Start: lineEnd(s, 12), End: lineEnd(s, 13)}}, Selection{}) // the start line, with the break before it
	if got := b.Folded(); len(got) != 0 {
		t.Fatalf("after deleting the start line from the line before: %v", got)
	}
}

func TestFoldViewState(t *testing.T) {
	m := NewModel("a.go", foldGoDoc, DefaultRegistry())
	st := NewView(m, nil)
	defer st.Close()
	caretAt(st, 3, 0)
	st.Execute("editor.fold", nil)
	caretAt(st, 8, 0)
	st.Execute("editor.fold", nil)
	v := st.SaveViewState()
	if !slices.Equal(v.Folded, []int{3, 8}) {
		t.Fatalf("saved folded %v", v.Folded)
	}
	other := NewView(m, nil)
	defer other.Close()
	v.Folded = append(v.Folded, 1, 99, -1) // not range starts, out of range
	other.RestoreViewState(v)
	if got := other.Folded(); !slices.Equal(got, []FoldingRange{{3, 6, ""}, {8, 9, ""}}) {
		t.Fatalf("restored %v", got)
	}
	if got := other.SaveViewState().Folded; !slices.Equal(got, []int{3, 8}) {
		t.Fatalf("saved again %v", got)
	}
	// The top line is a model line: restoring with a fold above keeps it.
	other.Viewport = Viewport{Height: 30, Width: 300, LineHeight: 10}
	other.RestoreViewState(ViewState{TopLine: 8, Folded: []int{3}})
	if l := other.topModelLine(); l != 8 {
		t.Fatalf("top line %d, want 8", l)
	}
	if other.Viewport.TopLine != 5 {
		t.Fatalf("top view line %d, want 5", other.Viewport.TopLine)
	}
}

func TestFoldingDisabledForLargeFile(t *testing.T) {
	small := strings.Repeat("a\n\tb\n", LargeFileLines/2-1) + "c" // LargeFileLines-1 lines
	large := strings.Repeat("a\n\tb\n", LargeFileLines/2) + "c"   // one more line than LargeFileLines
	for _, c := range []struct {
		text  string
		large bool
	}{{small, false}, {large, true}} {
		st := NewState(c.text, "a.txt", nil)
		if st.Model.LargeFile() != c.large {
			t.Fatalf("%d lines: LargeFile %v", st.Snapshot().LineCount(), st.Model.LargeFile())
		}
		ran := st.Execute("editor.foldAll", nil)
		if v, _ := st.contextKeys().Value(CtxFoldingEnabled); v != !c.large || ran == c.large {
			t.Fatalf("%d lines: foldingEnabled %v, foldAll ran %v", st.Snapshot().LineCount(), v, ran)
		}
		if got := len(st.Folded()); (got == 0) != c.large {
			t.Fatalf("%d lines: %d folded", st.Snapshot().LineCount(), got)
		}
		st.Close()
	}
	// The size threshold.
	st := NewState(strings.Repeat("x", LargeFileSize+1), "a.txt", nil)
	if !st.Model.LargeFile() || len(st.FoldingRanges()) != 0 {
		t.Fatal("a file over 20 MB has folding")
	}
}

// TestFoldViewMapping checks the view lines with hidden lines, with and
// without wrapping, against plain sums, and the conversions through them.
func TestFoldViewMapping(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		var b strings.Builder
		for i := range 60 {
			fmt.Fprintf(&b, "func f%d() {\n\tx := %q\n\ty()\n}\n", i, strings.Repeat("word ", i%7))
		}
		st := NewState(b.String(), "a.go", nil)
		if wrap {
			st.setWrap(wrapConfig{col: 12, tab: 4, indent: WrapIndentSame}, 7)
			st.measure(0, st.Snapshot().LineCount()-1)
		}
		st.Execute("editor.foldAll", nil)
		vl := st.lines()
		checkViewLines(t, vl)
		snap := st.Snapshot()
		for l := range snap.LineCount() {
			v, x := vl.ModelToView(l, 0)
			gl, _ := vl.ViewToModel(v, x)
			if vl.hidden(l) {
				if want, _ := vl.visibleLine(l); gl != want {
					t.Fatalf("wrap %v: hidden line %d maps to %d, want %d", wrap, l, gl, want)
				}
				continue
			}
			if gl != l {
				t.Fatalf("wrap %v: line %d -> view %d -> line %d", wrap, l, v, gl)
			}
			if r := st.rowOf(snap.LineStart(l), false); r != v || st.peekRowOf(snap.LineStart(l), false) != v {
				t.Fatalf("wrap %v: line %d: rowOf %d, ModelToView %d", wrap, l, r, v)
			}
		}
		// Edits keep the mapping consistent.
		caretAt(st, 2, 1)
		st.Insert("a\nb\n")
		checkViewLines(t, st.lines())
		st.Execute("editor.unfoldAll", nil)
		checkViewLines(t, st.lines())
		if st.viewCount() < snap.LineCount() {
			t.Fatalf("wrap %v: %d view lines after unfolding all", wrap, st.viewCount())
		}
	}
}

// damageFoldScript is TestViewDamageVerified's script with folding:
// folding and unfolding redraw the whole view; typing, scrolling, edits
// through another view and the gutter's controls appearing under the
// pointer are partial.
func damageFoldScript(t *testing.T) {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := range 40 {
		fmt.Fprintf(&b, "func f%d(x int) int {\n\tif x > %d {\n\t\treturn x\n\t}\n\treturn 0\n}\n", i, i)
	}
	th := theme.Base("vs-dark")
	st := NewState(b.String(), "main.go", th)
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
	caretAt(st, 3, 3)
	h.step("caret")
	st.Execute("editor.fold", nil) // f0's if
	whole("fold")
	for i := range 4 {
		h.typeText(string(rune('a' + i)))
		h.step("type on the folded line")
	}
	h.key(input.KeyDown, 0)
	h.step("down over the fold")
	st.Execute("editor.foldAll", nil)
	whole("fold all")
	p := geom.Pt(240, 180)
	for _, dy := range []float32{8, 16, 4, 2.4, -8, -4} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step(fmt.Sprintf("scroll %v", dy))
	}
	// Another view types on a shown line.
	other := NewView(st.Model, th)
	defer other.Close()
	ol, _ := st.lines().find(st.Viewport.TopLine + 3)
	other.MoveCaret(other.Snapshot().LineStart(ol)+1, false)
	other.Insert("x")
	h.step("other view types")
	st.Execute("editor.unfoldAll", nil)
	whole("unfold all")
	// The pointer over the gutter shows every control, then leaves.
	g := h.vw.vs.gutter
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(g.X+5, g.Y+40)})
	h.step("hover gutter")
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(g.Right()+100, g.Y+40)})
	h.step("leave gutter")
	// A click on a control folds its region, another unfolds it.
	v := st.Viewport
	row, line := -1, -1
	for r := 1; r < 10; r++ {
		if l, _ := st.lines().find(v.TopLine + r); st.fold.rangeAt(l) {
			row, line = r, l
			break
		}
	}
	if row < 0 {
		t.Fatal("no folding range in the view")
	}
	cp := geom.Pt(h.vw.vs.foldX+foldControlW/2, g.Y+(float32(row)+0.5)*v.LineHeight-v.OffsetY)
	click := func() {
		h.now = h.now.Add(time.Second)
		h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: cp})
		h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: cp})
		h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: cp})
	}
	click()
	whole("click a control")
	if st.fold.foldAt(line) < 0 {
		t.Fatalf("the click did not fold line %d", line)
	}
	click()
	whole("click again")
	if st.fold.foldAt(line) >= 0 {
		t.Fatalf("the click did not unfold line %d", line)
	}
	for i := range 4 {
		h.typeText(string(rune('a' + i)))
		h.step("type")
	}
	h.verified()
}

// TestFoldKeys runs the fold commands from Monaco's default keys on macOS
// and Linux.
func TestFoldKeys(t *testing.T) {
	goText(t)
	linuxKeymap := keymap.New(keymap.Linux, keymap.DefaultRules(keymap.Linux))
	for _, c := range []struct {
		name         string
		km           *keymap.Keymap
		mod, foldMod input.Modifiers
	}{
		{"mac", macKeymap, input.ModMeta, input.ModMeta | input.ModAlt},
		{"linux", linuxKeymap, input.ModCtrl, input.ModCtrl | input.ModShift},
	} {
		st := NewState(foldGoDoc, "a.go", theme.Base("vs"))
		ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(c.km) })
		ed.focus(t)
		caretAt(st, 5, 0)
		ed.Key(input.KeyBracketLeft, c.foldMod)
		if got := st.Folded(); !slices.Equal(got, []FoldingRange{{4, 5, ""}}) {
			t.Fatalf("%s: fold key: %v", c.name, got)
		}
		ed.Key(input.KeyBracketRight, c.foldMod)
		if got := st.Folded(); len(got) != 0 {
			t.Fatalf("%s: unfold key: %v", c.name, got)
		}
		ed.Key(input.KeyK, c.mod)
		ed.Key(input.Key0, c.mod)
		if got := st.Folded(); len(got) != 4 {
			t.Fatalf("%s: fold all chord: %v", c.name, got)
		}
		ed.Key(input.KeyK, c.mod)
		ed.Key(input.KeyJ, c.mod)
		if got := st.Folded(); len(got) != 0 {
			t.Fatalf("%s: unfold all chord: %v", c.name, got)
		}
		ed.Key(input.KeyK, c.mod)
		ed.Key(input.Key1, c.mod)
		if got := st.Folded(); !slices.Equal(got, []FoldingRange{{0, 2, ""}, {8, 9, ""}}) {
			t.Fatalf("%s: fold level 1 chord: %v", c.name, got)
		}
	}
}
