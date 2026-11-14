package editor

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/editor/when"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func carets(offs ...int) []Selection {
	out := make([]Selection, len(offs))
	for i, o := range offs {
		out[i] = Selection{Anchor: o, Head: o}
	}
	return out
}

func wantSels(t *testing.T, st *State, ctx string, want ...Selection) {
	t.Helper()
	if !slices.Equal(st.Sels, want) {
		t.Errorf("%s: sels %v, want %v", ctx, st.Sels, want)
	}
}

// TestMultiCursorTyping: typing, deleting, Enter, Tab and an IME commit act
// on every cursor as one edit set, one undo step restoring every cursor.
func TestMultiCursorTyping(t *testing.T) {
	st := NewState("ab\n  ab\nab", "a.txt", theme.Base("vs"))
	text := func() string { return st.Snapshot().String() }
	st.SetSelections(carets(1, 6, 9))

	st.Insert("X")
	st.Insert("Y")
	if text() != "aXYb\n  aXYb\naXYb" {
		t.Fatalf("typed: %q", text())
	}
	wantSels(t, st, "after typing", carets(3, 10, 15)...)
	if !st.Undo() || text() != "ab\n  ab\nab" {
		t.Fatalf("undo: %q", text())
	}
	wantSels(t, st, "after undo", carets(1, 6, 9)...)
	if st.Undo() {
		t.Error("typing at three cursors took more than one undo step")
	}
	st.Redo()
	wantSels(t, st, "after redo", carets(3, 10, 15)...)

	// Backspace at every caret, then a selection mixed with carets.
	st.Delete(Backward, Char)
	if text() != "aXb\n  aXb\naXb" {
		t.Fatalf("deleteLeft: %q", text())
	}
	st.SetSelections([]Selection{sel(0, 2), {Anchor: 7, Head: 7}})
	st.Insert("_")
	if text() != "_b\n  a_Xb\naXb" {
		t.Fatalf("replace selection + caret: %q", text())
	}
	wantSels(t, st, "after replace", carets(1, 7)...)

	// Adjacent carets deleting the same character merge.
	st = NewState("abc", "a.txt", nil)
	st.SetSelections(carets(1, 2))
	st.Delete(Backward, Char)
	st.Delete(Backward, Char)
	if text := st.Snapshot().String(); text != "c" {
		t.Fatalf("overlapping deletes: %q", text)
	}
	wantSels(t, st, "merged after deletes", carets(0)...)

	// Enter keeps each line's indentation; Tab goes to each caret's stop.
	st = NewState("a\n  b", "a.txt", nil)
	st.SetSelections(carets(1, 5))
	st.Execute("type", TypeArgs{Text: "\n"})
	if got := st.Snapshot().String(); got != "a\n\n  b\n  " {
		t.Fatalf("Enter: %q", got)
	}
	st = NewState("a\nabc", "a.txt", nil)
	st.Model.SetTabSize(4)
	st.SetSelections(carets(1, 5))
	st.Execute("tab", nil)
	if got := st.Snapshot().String(); got != "a   \nabc " {
		t.Fatalf("Tab: %q", got)
	}

	// Through the View: typed text and an IME commit reach every cursor.
	goText(t)
	st = NewState("one\ntwo\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 400, 200, pinMac)
	ed.focus(t)
	st.SetSelections(carets(3, 7))
	ed.Frame()
	ed.Type("!")
	ed.Compose("ni", 2)
	ed.Type("你")
	if got := st.Snapshot().String(); got != "one!你\ntwo!你\n" {
		t.Fatalf("view typing + IME: %q", got)
	}
	st.Undo() // the IME commit is a step of its own
	if got := st.Snapshot().String(); got != "one!\ntwo!\n" {
		t.Fatalf("undo IME commit: %q", got)
	}
	// Esc removes the secondary cursors.
	ed.Key(input.KeyEscape, 0)
	if len(st.Sels) != 1 || st.Primary() != sel(4, 4) {
		t.Fatalf("Esc: %v", st.Sels)
	}
	if st.Execute("removeSecondaryCursors", nil) {
		t.Error("removeSecondaryCursors ran with a single cursor (precondition)")
	}
}

// TestSelectionsMergeOverlap: Monaco's CursorCollection.normalize.
func TestSelectionsMergeOverlap(t *testing.T) {
	st := NewState(strings.Repeat("abcdefghij", 3), "a.txt", nil)
	for _, c := range []struct {
		name string
		in   []Selection
		want []Selection
	}{
		{"overlapping", []Selection{sel(0, 3), sel(2, 5)}, []Selection{sel(0, 5)}},
		{"touching non-empty stay", []Selection{sel(0, 2), sel(2, 4)}, []Selection{sel(0, 2), sel(2, 4)}},
		{"caret touching a selection", []Selection{sel(2, 2), sel(0, 2)}, []Selection{sel(0, 2)}},
		// SetSelections adding cursors makes the last one the last added
		// (Monaco's setSelections): merging into an earlier one, it gives
		// the direction.
		{"caret inside", []Selection{sel(6, 1), sel(3, 3)}, []Selection{sel(1, 6)}},
		{"last added's direction", []Selection{sel(5, 2), sel(3, 8)}, []Selection{sel(2, 8)}},
		{"later winner keeps place", []Selection{sel(20, 20), sel(3, 8), sel(9, 4)}, []Selection{sel(20, 20), sel(9, 3)}},
		{"winner's direction when the last added is apart", []Selection{sel(5, 2), sel(3, 8), sel(20, 20)}, []Selection{sel(8, 2), sel(20, 20)}},
		{"same caret", carets(4, 4, 4), carets(4)},
		{"order kept", carets(10, 0, 5), carets(10, 0, 5)},
		{"chain", []Selection{sel(0, 3), sel(2, 6), sel(5, 9), sel(12, 12)}, []Selection{sel(0, 9), sel(12, 12)}},
		{"clamped", []Selection{sel(-4, 100)}, []Selection{sel(0, 30)}},
	} {
		st.SetSelections(c.in)
		wantSels(t, st, c.name, c.want...)
	}

	// The last added cursor keeps its direction when it merges into an
	// earlier one (Monaco's lastAddedCursorIndex): primary caret at 10, a
	// cursor added at 20 and dragged left to 5.
	st.SetSelections(carets(10))
	st.AddCursor(20)
	st.setSels([]Selection{sel(10, 10), sel(20, 5)}, nil)
	wantSels(t, st, "last added keeps its direction", sel(20, 5))
	st.SetSelections([]Selection{sel(10, 10), sel(25, 26)})
	st.setSels([]Selection{sel(10, 10), sel(25, 26), sel(12, 4)}, nil)
	wantSels(t, st, "last added (index 2) keeps its direction", sel(12, 4), sel(25, 26))
	st.setSels([]Selection{sel(12, 4), sel(25, 26), sel(26, 20)}, nil)
	wantSels(t, st, "the merged cursor stays the last added one", sel(12, 4), sel(26, 20))
	// Without a last added cursor among the parts the winner decides.
	if out, _, _ := mergeSelsLast([]Selection{sel(5, 2), sel(3, 8)}, -1); !slices.Equal(out, []Selection{sel(8, 2)}) {
		t.Errorf("winner's direction: %v", out)
	}

	// Moves that make cursors meet merge them, keeping the primary.
	st.SetSelections(carets(2, 1, 9))
	st.Move(Backward, Doc, false)
	wantSels(t, st, "cursorTop", carets(0)...)
	st.SetSelections(carets(12, 3))
	st.Move(Forward, LineEdge, false)
	wantSels(t, st, "cursorEnd on one line", carets(30)...)
	// Vertical moves keep each cursor's goal column.
	st = NewState("abcdef\nab\nabcdef", "a.txt", nil)
	st.SetSelections(carets(5, 1))
	st.Move(Forward, Line, false)
	wantSels(t, st, "down once", carets(9, 8)...)
	st.Move(Forward, Line, false)
	wantSels(t, st, "down twice", carets(15, 11)...)
}

// TestAddNextOccurrence: editor.action.addSelectionToNextFindMatch (Cmd+D),
// Monaco's rules: from a caret the word under it first, then its next
// whole-word, case-sensitive occurrences, wrapping around; from a
// selection its text, case-insensitive, not as a whole word (Monaco's
// default find options).
func TestAddNextOccurrence(t *testing.T) {
	doc := "foo Foo foo foobar xfoo foo"
	//      0   4   8   12     19   24
	st := NewState(doc, "a.txt", nil)
	st.SetSelections(carets(1))
	add := func(ctx string) {
		t.Helper()
		if !st.Execute("editor.action.addSelectionToNextFindMatch", nil) {
			t.Fatalf("%s: did nothing", ctx)
		}
	}
	add("1")
	wantSels(t, st, "word under caret", sel(0, 3))
	add("2")
	add("3")
	wantSels(t, st, "whole words, case-sensitive", sel(0, 3), sel(8, 11), sel(24, 27))
	add("wrap")
	wantSels(t, st, "wrapping onto a selected match", sel(0, 3), sel(8, 11), sel(24, 27))

	// From a selection: substring matches, ignoring case.
	st.SetSelections([]Selection{sel(0, 3)})
	for range 5 {
		add("from selection")
	}
	wantSels(t, st, "selection seed", sel(0, 3), sel(4, 7), sel(8, 11), sel(12, 15), sel(20, 23), sel(24, 27))
	st = NewState("Foo foo FOO Foo", "a.txt", nil)
	st.SetSelections([]Selection{sel(0, 3)})
	add("Foo")
	wantSels(t, st, "case-insensitive from a selection", sel(0, 3), sel(4, 7))
	// Selections whose texts differ only in case continue the search.
	st.SetSelections([]Selection{sel(0, 3), sel(8, 11)})
	add("same text ignoring case")
	wantSels(t, st, "differing case is the same text", sel(0, 3), sel(8, 11), sel(12, 15))
	st = NewState(doc, "a.txt", nil)

	// Selections holding different text: select the word under each caret.
	st.SetSelections([]Selection{sel(0, 3), {Anchor: 13, Head: 13}})
	add("mixed")
	wantSels(t, st, "expanded to words", sel(0, 3), sel(12, 18))

	// Several carets: each is expanded to its word (an empty selection
	// after the first is never "the same text").
	st = NewState("foo bar foo", "a.txt", nil)
	st.SetSelections(carets(1, 5))
	add("carets")
	wantSels(t, st, "carets expanded to words", sel(0, 3), sel(4, 7))
	st.SetSelections([]Selection{sel(0, 3), {Anchor: 9, Head: 9}})
	add("selection and caret")
	wantSels(t, st, "selection and caret", sel(0, 3), sel(8, 11))

	// No word under the caret: nothing.
	st = NewState("a  b", "a.txt", nil)
	st.SetSelections(carets(2))
	if st.Execute("editor.action.addSelectionToNextFindMatch", nil) {
		t.Error("no word under the caret, yet it ran")
	}
	// Moving the cursor ends the session: the next Cmd+D starts over.
	st = NewState("ab ab cd cd", "a.txt", nil)
	st.SetSelections(carets(0))
	st.AddSelectionToNextFindMatch()
	st.AddSelectionToNextFindMatch()
	st.MoveCaret(7, false)
	st.AddSelectionToNextFindMatch()
	wantSels(t, st, "new session", sel(6, 8))
}

// TestSelectHighlights: editor.action.selectHighlights (Cmd+Shift+L).
func TestSelectHighlights(t *testing.T) {
	st := NewState("foo bar foo Foo foo_x foo", "a.txt", nil)
	st.SetSelections(carets(9))
	if !st.Execute("editor.action.selectHighlights", nil) {
		t.Fatal("did nothing")
	}
	// Whole words (seed from a caret), case-sensitive; the occurrence at
	// the caret is primary.
	if len(st.Sels) != 3 || st.Sels[0] != sel(8, 11) {
		t.Fatalf("sels %v", st.Sels)
	}
	got := slices.Clone(st.Sels)
	slices.SortFunc(got, func(a, b Selection) int { return a.Anchor - b.Anchor })
	if !slices.Equal(got, []Selection{sel(0, 3), sel(8, 11), sel(22, 25)}) {
		t.Errorf("matches %v", got)
	}
	// From a selection: every occurrence of its text, ignoring case.
	st.SetSelections([]Selection{sel(0, 3)})
	st.SelectHighlights()
	if len(st.Sels) != 5 || st.Sels[0] != sel(0, 3) {
		t.Errorf("from selection: %v", st.Sels)
	}
	// Typing replaces them all, one undo step.
	st.Insert("z")
	if got := st.Snapshot().String(); got != "z bar z z z_x z" {
		t.Fatalf("typed over highlights: %q", got)
	}
	st.Undo()
	if len(st.Sels) != 5 {
		t.Errorf("undo restored %d selections", len(st.Sels))
	}
}

// TestColumnSelectMixedWidths: a column box spans the same visual columns
// on lines with tabs and wide (CJK) characters, by keyboard and by
// Shift+Alt+drag.
func TestColumnSelectMixedWidths(t *testing.T) {
	lines := []string{"a\tbcd", "中文xy", "abcdefgh", "ab", ""}
	st := NewState(strings.Join(lines, "\n"), "a.txt", nil)
	st.Model.SetTabSize(4)
	snap := st.Snapshot()
	ls := func(i int) int { return snap.LineStart(i) }

	st.ColumnSelect(0, 1, 3, 4)
	// Line 0: column 1 after "a", column 4 after the tab. Line 1: column 1
	// is inside 中 (nearest boundary: before it, ties go left), column 4
	// after 文. Line 3 is shorter than the box: up to its end.
	wantSels(t, st, "box 1..4",
		sel(ls(0)+1, ls(0)+2), sel(ls(1), ls(1)+6), sel(ls(2)+1, ls(2)+4), sel(ls(3)+1, ls(3)+2))

	// Keyboard: cursorColumnSelectRight widens the box by one column,
	// cursorColumnSelectDown adds the empty line (it reaches no column of
	// the box, so it gets no selection).
	st.Execute("cursorColumnSelectRight", nil)
	wantSels(t, st, "right",
		sel(ls(0)+1, ls(0)+3), sel(ls(1), ls(1)+7), sel(ls(2)+1, ls(2)+5), sel(ls(3)+1, ls(3)+2))
	st.Execute("cursorColumnSelectDown", nil)
	if len(st.Sels) != 4 {
		t.Errorf("down onto an empty line: %v", st.Sels)
	}
	for range 4 {
		st.Execute("cursorColumnSelectUp", nil)
	}
	wantSels(t, st, "up to line 0", sel(ls(0)+1, ls(0)+3))
	// Typing over a column box.
	st.ColumnSelect(0, 4, 2, 4)
	st.Insert("|")
	if got := st.Snapshot().String(); got != "a\t|bcd\n中文|xy\nabcd|efgh\nab\n" {
		t.Errorf("typed into column carets: %q", got)
	}

	// From a caret, the box starts at the caret: Cmd+Shift+Alt+Down.
	st = NewState(strings.Join(lines, "\n"), "a.txt", nil)
	st.Model.SetTabSize(4)
	snap = st.Snapshot()
	st.MoveCaret(ls(2)+4, false)
	st.Execute("cursorColumnSelectUp", nil)
	st.Execute("cursorColumnSelectUp", nil)
	wantSels(t, st, "from caret", carets(ls(2)+4, ls(1)+6, ls(0)+2)...)
}

// TestColumnSelectMouseFromCaret: Shift+Alt+drag starts the box at the
// primary caret.
func TestColumnSelectMouseFromCaret(t *testing.T) {
	goText(t)
	lines := []string{"a\tbcd", "中文xy", "abcdefgh"}
	st := NewState(strings.Join(lines, "\n"), "a.txt", theme.Base("vs"))
	st.Model.SetTabSize(4)
	ed := newEditDriver(st, 500, 300, nil)
	ed.focus(t)
	snap := st.Snapshot()
	ed.Click(ed.at(t, st, 0, 1))
	ed.Advance(time.Second)
	g := (*ed.vw).vs
	v := st.Viewport
	pt := geom.Pt(g.text.X-v.ScrollX+4*g.colW+1, g.text.Y+2*v.LineHeight+v.LineHeight/2)
	mods := input.ModShift | input.ModAlt
	ed.Inject(input.Pointer{Kind: input.Move, Pos: pt})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: pt, Button: input.ButtonLeft, Mods: mods})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: pt, Button: input.ButtonLeft, Mods: mods})
	ed.Settle()
	ls := snap.LineStart
	wantSels(t, st, "Shift+Alt+click", sel(1, 2), sel(ls(1), ls(1)+6), sel(ls(2)+1, ls(2)+4))
}

// TestAltClickAddsCursor: Alt+click adds a caret, Alt+click on a caret
// removes it, Alt+drag selects with the added cursor; a plain click
// removes the secondary cursors.
func TestAltClickAddsCursor(t *testing.T) {
	goText(t)
	st := NewState("hello world\nsecond line\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 200, nil)
	ed.focus(t)
	altClick := func(p geom.Point) {
		ed.Inject(input.Pointer{Kind: input.Move, Pos: p})
		ed.Inject(input.Pointer{Kind: input.Press, Pos: p, Button: input.ButtonLeft, Mods: input.ModAlt})
		ed.Inject(input.Pointer{Kind: input.Release, Pos: p, Button: input.ButtonLeft, Mods: input.ModAlt})
		ed.Settle()
		ed.Advance(time.Second)
	}
	ed.Click(ed.at(t, st, 0, 2))
	ed.Advance(time.Second)
	altClick(ed.at(t, st, 1, 3))
	wantSels(t, st, "alt+click", carets(2, 15)...)
	altClick(ed.at(t, st, 0, 8))
	wantSels(t, st, "second alt+click", carets(2, 15, 8)...)
	altClick(ed.at(t, st, 1, 3))
	wantSels(t, st, "alt+click on a caret removes it", carets(2, 8)...)
	ed.Type("#")
	if got := st.Snapshot().String(); got != "he#llo wo#rld\nsecond line\n" {
		t.Fatalf("typed at the cursors: %q", got)
	}

	// Alt+drag: a new selection, the others stay.
	from, to := ed.at(t, st, 1, 0), ed.at(t, st, 1, 6)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: from})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: from, Button: input.ButtonLeft, Mods: input.ModAlt})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Move, Pos: to, Mods: input.ModAlt})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Release, Pos: to, Button: input.ButtonLeft, Mods: input.ModAlt})
	ed.Settle()
	ed.Advance(time.Second)
	l1 := st.Snapshot().LineStart(1)
	wantSels(t, st, "alt+drag", sel(3, 3), sel(10, 10), sel(l1, l1+6))

	ed.Click(ed.at(t, st, 0, 0))
	wantSels(t, st, "plain click", carets(0)...)
}

// TestInsertCursorAboveBelow: editor.action.insertCursorAbove/Below keep
// the visual column; insertCursorAtEndOfEachLineSelected.
func TestInsertCursorAboveBelow(t *testing.T) {
	st := NewState("abcdef\n\tx\nabcdef", "a.txt", nil)
	st.Model.SetTabSize(4)
	st.MoveCaret(5, false)
	st.Execute("editor.action.insertCursorBelow", nil)
	st.Execute("editor.action.insertCursorBelow", nil)
	// Line 1: column 5 is after "\tx"; line 2: column 5.
	wantSels(t, st, "below", carets(5, 9, 15)...)
	st.MoveCaret(16, false)
	st.Execute("editor.action.insertCursorAbove", nil)
	wantSels(t, st, "above", carets(16, 9)...)
	st.Options.ReadOnly = true
	if st.Execute("editor.action.insertCursorAbove", nil) {
		t.Error("insertCursorAbove in a read-only editor (Monaco refuses)")
	}
	st.Options.ReadOnly = false

	// On the first or last line the copy stays and merges away.
	st.MoveCaret(3, false)
	st.Execute("editor.action.insertCursorAbove", nil)
	wantSels(t, st, "above on the first line", carets(3)...)
	st.MoveCaret(13, false)
	st.Execute("editor.action.insertCursorBelow", nil)
	wantSels(t, st, "below on the last line", carets(13)...)

	// The goal column carries over a short line.
	gs := NewState("abcdefghij\n\nabcdefghij", "a.txt", nil)
	gs.MoveCaret(22, false)
	gs.Execute("editor.action.insertCursorAbove", nil)
	wantSels(t, gs, "above onto an empty line", carets(22, 11)...)
	gs.Execute("editor.action.insertCursorAbove", nil)
	wantSels(t, gs, "above again: back at column 10", carets(22, 11, 10)...)
	gs.MoveCaret(10, false)
	gs.Execute("editor.action.insertCursorBelow", nil)
	gs.Execute("editor.action.insertCursorBelow", nil)
	wantSels(t, gs, "below twice", carets(10, 11, 22)...)

	st.SetSelections([]Selection{sel(2, 13)})
	st.Execute("editor.action.insertCursorAtEndOfEachLineSelected", nil)
	wantSels(t, st, "line ends", carets(6, 9, 13)...)
	st.SetSelections([]Selection{sel(0, 7)}) // ends at column 0 of line 1
	st.Execute("editor.action.insertCursorAtEndOfEachLineSelected", nil)
	wantSels(t, st, "selection ending at column 0", carets(6)...)
}

// TestCursorUndo: cursorUndo / cursorRedo walk a selection history of
// their own, which an edit clears.
func TestCursorUndo(t *testing.T) {
	st := NewState("one two three\nfour", "a.txt", nil)
	st.MoveCaret(4, false)
	st.Move(Forward, Word, true)
	st.SetSelections(carets(1, 15))
	if !st.Execute("cursorUndo", nil) {
		t.Fatal("cursorUndo did nothing")
	}
	wantSels(t, st, "undo 1", sel(4, 7))
	st.CursorUndo()
	wantSels(t, st, "undo 2", carets(4)...)
	st.Execute("cursorRedo", nil)
	wantSels(t, st, "redo", sel(4, 7))
	st.CursorRedo()
	wantSels(t, st, "redo 2", carets(1, 15)...)
	if st.CursorRedo() {
		t.Error("redo past the end")
	}
	// A new selection change clears the redo stack.
	st.CursorUndo()
	st.MoveCaret(0, false)
	if st.CursorRedo() {
		t.Error("redo after a new selection change")
	}
	// Edits clear the history; the text undo is untouched.
	st.Insert("x")
	if st.CursorUndo() {
		t.Error("cursorUndo after an edit")
	}
	if !st.Undo() || st.Snapshot().String() != "one two three\nfour" {
		t.Error("text undo")
	}
	// The history is bounded.
	for i := range 2 * cursorHistoryMax {
		st.MoveCaret(i%10, false)
	}
	if len(st.curUndo) != cursorHistoryMax {
		t.Errorf("cursor history %d entries, want %d", len(st.curUndo), cursorHistoryMax)
	}
}

// TestOtherViewMapsAllSelections: an edit in one view maps every
// selection of the others.
func TestOtherViewMapsAllSelections(t *testing.T) {
	m := NewModel("a.txt", "aaa\nbbb\nccc", nil)
	v1, v2 := NewView(m, nil), NewView(m, nil)
	defer v1.Close()
	defer v2.Close()
	v2.SetSelections([]Selection{sel(9, 11), sel(1, 1), sel(5, 6)})
	v1.SetSelections(carets(0, 4, 8))
	v1.Insert(">>")
	wantSels(t, v2, "mapped", sel(15, 17), sel(3, 3), sel(9, 10))
	v1.Undo()
	wantSels(t, v2, "mapped back", sel(9, 11), sel(1, 1), sel(5, 6))
}

// TestCopyPasteMultiCursor: copy joins the selections with line breaks;
// pasting as many lines as cursors spreads them (multiCursorPaste).
func TestCopyPasteMultiCursor(t *testing.T) {
	goText(t)
	st := NewState("ab\ncd\nef", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 400, 200, pinMac)
	ed.focus(t)
	st.SetSelections([]Selection{sel(3, 4), sel(0, 1)})
	ed.Frame()
	ed.Key(input.KeyC, input.ModMeta)
	if got := ed.Clipboard(); got != "a\nc" {
		t.Fatalf("copy: %q", got)
	}
	st.SetSelections(carets(2, 5))
	ed.Frame()
	ed.SetClipboard("1\n2\n")
	ed.Key(input.KeyV, input.ModMeta)
	if got := st.Snapshot().String(); got != "ab1\ncd2\nef" {
		t.Fatalf("spread paste: %q", got)
	}
	ed.SetClipboard("x\ny\nz")
	ed.Key(input.KeyV, input.ModMeta)
	if got := st.Snapshot().String(); got != "ab1x\ny\nz\ncd2x\ny\nz\nef" {
		t.Fatalf("paste of other line count: %q", got)
	}
}

func manyCursorDoc(n int) (string, []Selection) {
	var b strings.Builder
	sels := make([]Selection, n)
	for i := range n {
		line := fmt.Sprintf("line %d", i)
		b.WriteString(line)
		off := b.Len()
		sels[i] = Selection{Anchor: off, Head: off}
		b.WriteByte('\n')
	}
	return b.String(), sels
}

// gateLimit is a timing gate, relaxed under the race detector and on
// shared CI runners (TANGO_SMOKE_TIMEOUT_SCALE, which CI sets). The gates
// catch superlinear regressions, which miss them by far more.
func gateLimit(d time.Duration) time.Duration {
	if raceEnabled {
		d *= 20
	}
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		d = time.Duration(float64(d) * f)
	}
	return d
}

// TestManyCursorsUndoMultiChar: typing ten characters at 10 000 cursors is
// one undo step, undone and redone in ≤ 50 ms each (the step's batches
// compose in linear time).
func TestManyCursorsUndoMultiChar(t *testing.T) {
	const n = 10000
	doc, sels := manyCursorDoc(n)
	st := NewState(doc, "a.txt", nil)
	st.SetSelections(sels)
	for range 10 {
		st.Insert("x")
	}
	typed := st.Snapshot().String()
	start := time.Now()
	if !st.Undo() {
		t.Fatal("undo")
	}
	du := time.Since(start)
	if st.Snapshot().String() != doc || !slices.Equal(st.Sels, sels) {
		t.Fatal("undo did not restore the text and the cursors in one step")
	}
	start = time.Now()
	if !st.Redo() {
		t.Fatal("redo")
	}
	dr := time.Since(start)
	if st.Snapshot().String() != typed {
		t.Fatal("redo did not restore the typed text")
	}
	t.Logf("undo %v, redo %v", du, dr)
	if lim := gateLimit(50 * time.Millisecond); du > lim || dr > lim {
		t.Errorf("undo %v / redo %v of 10 characters at %d cursors, want ≤ %v", du, dr, n, lim)
	}
}

// TestManyCursorsTypingPrimaryInMiddle: the 50 ms gate holds when the
// selections are not in document order (the primary in the middle), the
// sorting path.
func TestManyCursorsTypingPrimaryInMiddle(t *testing.T) {
	const n = 10000
	doc, sels := manyCursorDoc(n)
	rot := append(slices.Clone(sels[n/2:]), sels[:n/2]...)
	st := NewState(doc, "a.txt", nil)
	st.SetSelections(rot)
	if len(st.Sels) != n || st.Sels[0] != sels[n/2] {
		t.Fatalf("selections %d, primary %v", len(st.Sels), st.Primary())
	}
	start := time.Now()
	st.Insert("x")
	d := time.Since(start)
	t.Logf("typed at %d unordered cursors in %v", n, d)
	if lim := gateLimit(50 * time.Millisecond); d > lim {
		t.Errorf("typing took %v, want ≤ %v", d, lim)
	}
	if st.Primary().Head != sels[n/2].Head+n/2+1 || st.Snapshot().Len() != len(doc)+n {
		t.Fatalf("primary %v, length %d", st.Primary(), st.Snapshot().Len())
	}
	st.Undo()
	if !slices.Equal(st.Sels, rot) {
		t.Error("undo did not restore the cursor order")
	}
}

// TestManyCursorsTyping: one character at 10 000 cursors in ≤ 50 ms, one
// undo step back.
func TestManyCursorsTyping(t *testing.T) {
	const n = 10000
	for _, file := range []string{"a.txt", "a.go"} {
		doc, sels := manyCursorDoc(n)
		st := NewState(doc, file, nil)
		st.SetSelections(sels)
		if len(st.Sels) != n {
			t.Fatalf("%d selections", len(st.Sels))
		}
		start := time.Now()
		st.Insert("x")
		d := time.Since(start)
		limit := gateLimit(50 * time.Millisecond)
		t.Logf("%s: typed at %d cursors in %v", file, n, d)
		if d > limit {
			t.Errorf("%s: typing at %d cursors took %v, want ≤ %v", file, n, d, limit)
		}
		if got := st.Snapshot().Len(); got != len(doc)+n {
			t.Fatalf("length %d, want %d", got, len(doc)+n)
		}
		if !strings.HasPrefix(st.Snapshot().String(), "line 0x\nline 1x\n") || len(st.Sels) != n || st.Sels[n-1].Head != sels[n-1].Head+n {
			t.Fatalf("%s: wrong result", file)
		}
		start = time.Now()
		if !st.Undo() {
			t.Fatal("undo")
		}
		t.Logf("%s: undo in %v", file, time.Since(start))
		if st.Snapshot().String() != doc || !slices.Equal(st.Sels, sels) {
			t.Fatalf("%s: one undo did not restore the text and the cursors", file)
		}
		if st.Undo() {
			t.Errorf("%s: more than one undo step", file)
		}
	}
}

// TestMultiCursorDefaultKeys: Monaco 0.56's default keys of the new
// commands, per platform.
func TestMultiCursorDefaultKeys(t *testing.T) {
	ctx := when.MapContext{"editorFocus": true, "editorTextFocus": true, "textInputFocus": true}
	alt, shift, ctrl, meta := input.ModAlt, input.ModShift, input.ModCtrl, input.ModMeta
	for _, c := range []struct {
		p    keymap.Platform
		key  input.Key
		mods input.Modifiers
		want string
	}{
		{keymap.Mac, input.KeyD, meta, "editor.action.addSelectionToNextFindMatch"},
		{keymap.Mac, input.KeyL, meta | shift, "editor.action.selectHighlights"},
		{keymap.Mac, input.KeyUp, meta | alt, "editor.action.insertCursorAbove"},
		{keymap.Mac, input.KeyDown, meta | alt, "editor.action.insertCursorBelow"},
		{keymap.Mac, input.KeyI, shift | alt, "editor.action.insertCursorAtEndOfEachLineSelected"},
		{keymap.Mac, input.KeyU, meta, "cursorUndo"},
		{keymap.Mac, input.KeyEscape, 0, "removeSecondaryCursors"},
		{keymap.Mac, input.KeyEscape, shift, "removeSecondaryCursors"},
		{keymap.Mac, input.KeyLeft, meta | shift | alt, "cursorColumnSelectLeft"},
		{keymap.Mac, input.KeyPageDown, meta | shift | alt, "cursorColumnSelectPageDown"},
		{keymap.Mac, input.KeyLeft, shift | alt, "cursorWordLeftSelect"},
		{keymap.Windows, input.KeyD, ctrl, "editor.action.addSelectionToNextFindMatch"},
		{keymap.Windows, input.KeyDown, ctrl | alt, "editor.action.insertCursorBelow"},
		{keymap.Windows, input.KeyUp, ctrl | shift | alt, "cursorColumnSelectUp"},
		{keymap.Linux, input.KeyDown, shift | alt, "editor.action.insertCursorBelow"},
		{keymap.Linux, input.KeyUp, ctrl | shift, "editor.action.insertCursorAbove"},
		// No column selection on Linux: the key is Monaco's copyLinesUp there.
		{keymap.Linux, input.KeyUp, ctrl | shift | alt, "editor.action.copyLinesUpAction"},
	} {
		km := keymap.New(c.p, keymap.DefaultRules(c.p))
		if errs := km.Errors(); len(errs) != 0 {
			t.Fatalf("%v defaults: %v", c.p, errs)
		}
		cmd, _, _ := km.Resolve(input.KeyEvent{Key: c.key, Code: c.key, Down: true, Mods: c.mods}, ctx)
		if cmd != c.want {
			t.Errorf("%v %v+%v: %q, want %q", c.p, c.mods, c.key, cmd, c.want)
		}
	}
}

// TestMultiCursorPaint: every selection and every caret is drawn.
func TestMultiCursorPaint(t *testing.T) {
	goText(t)
	st := NewState("hello there\nhello wide\nthird line\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 200, nil)
	ed.focus(t)
	snap := st.Snapshot()
	l1, l2 := snap.LineStart(1), snap.LineStart(2)
	st.SetSelections([]Selection{sel(0, 5), sel(l1, l1+5), {Anchor: l2 + 3, Head: l2 + 3}})
	ed.Advance(time.Second) // a fresh blink cycle starts visible
	img := ed.Frame()
	selBg := defaultColor(st.Theme, "editor.selectionBackground")
	cur := defaultColor(st.Theme, "editorCursor.foreground")
	rowRect := func(line, col0, col1 int) geom.Rect {
		a, b := ed.at(t, st, line, col0), ed.at(t, st, line, col1)
		lh := st.Viewport.LineHeight
		return geom.Rect{X: a.X - 1, Y: a.Y - lh/2 + 1, W: b.X - a.X + 2, H: lh - 2}
	}
	for _, l := range []int{0, 1} {
		if n := countColor(img, rowRect(l, 1, 4), selBg); n == 0 {
			t.Errorf("selection on line %d not drawn", l)
		}
	}
	for _, c := range []struct{ line, col int }{{0, 5}, {1, 5}, {2, 3}} {
		r := rowRect(c.line, c.col, c.col)
		r.X -= 1
		r.W = 4
		if n := countColor(img, r, cur); n < int(r.H/2) {
			t.Errorf("caret at line %d col %d not drawn", c.line, c.col)
		}
	}
}

// TestAltDragOverPrimaryKeepsDirection: Alt+press at 20, drag left over
// the primary caret at 10 to 5: the merged selection's caret follows the
// pointer (Monaco's lastAddedCursorIndex).
func TestAltDragOverPrimaryKeepsDirection(t *testing.T) {
	goText(t)
	st := NewState("abcdefghijklmnopqrstuvwxyz\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 200, nil)
	ed.focus(t)
	ed.Click(ed.at(t, st, 0, 10))
	ed.Advance(time.Second)
	from, to := ed.at(t, st, 0, 20), ed.at(t, st, 0, 5)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: from})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: from, Button: input.ButtonLeft, Mods: input.ModAlt})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Move, Pos: to, Mods: input.ModAlt})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Release, Pos: to, Button: input.ButtonLeft, Mods: input.ModAlt})
	ed.Settle()
	wantSels(t, st, "alt+drag over the primary", sel(20, 5))
}

// TestLastAddedStaysValid: every path that changes the selections keeps
// the last added cursor a valid index (edits that merge cursors, undo,
// another view's edit, normalize, RestoreViewState).
func TestLastAddedStaysValid(t *testing.T) {
	m := NewModel("a.txt", "abcdef\nabcdef", nil)
	st, other := NewView(m, nil), NewView(m, nil)
	defer st.Close()
	defer other.Close()
	check := func(ctx string) {
		t.Helper()
		if st.lastAdded < 0 || st.lastAdded >= len(st.Sels) {
			t.Fatalf("%s: lastAdded %d with %d selections", ctx, st.lastAdded, len(st.Sels))
		}
	}
	st.SetSelections(carets(1, 3, 4, 5))
	check("set")
	if st.lastAdded != 3 {
		t.Fatalf("lastAdded %d, want 3", st.lastAdded)
	}
	st.Delete(Backward, Word) // all four merge into one caret
	check("delete merging the cursors")
	st.Undo()
	check("undo")
	st.Redo()
	check("redo")
	// Undo back to fewer cursors than the last added index.
	st.SetSelections(carets(0))
	st.Insert("x")
	st.SetSelections(carets(1, 3, 4, 5))
	st.Undo()
	check("undo to one cursor")
	st.SetSelections(carets(1, 2, 3, 9))
	other.SetSelections([]Selection{sel(0, 7)})
	other.Insert("") // deletes the first line under st's cursors
	check("other view's edit")
	st.Sels = []Selection{sel(0, 0), sel(0, 0)} // set directly
	st.lastAdded = 1
	st.normalize()
	check("normalize")
	st.RestoreViewState(ViewState{Selections: carets(0)})
	check("restore")
}
