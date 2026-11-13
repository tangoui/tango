package editor

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// dndPress presses at from and moves to the points in turn, one frame
// each, without releasing.
func (ed *editDriver) dndPress(from geom.Point, to ...geom.Point) {
	ed.Inject(input.Pointer{Kind: input.Move, Pos: from})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: from})
	ed.Frame()
	for _, p := range to {
		ed.Inject(input.Pointer{Kind: input.Move, Pos: p})
		ed.Frame()
	}
}

// dndRelease releases the pointer at p, with mods held.
func (ed *editDriver) dndRelease(p geom.Point, mods ...input.Modifiers) {
	var m input.Modifiers
	for _, x := range mods {
		m |= x
	}
	ed.Inject(input.Pointer{Kind: input.Release, Pos: p, Mods: m})
	ed.Settle()
	ed.Advance(time.Second) // no double click with the next press
}

func TestDragMoveText(t *testing.T) {
	goText(t)
	doc := "hello world\nsecond line\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	l1 := st.Snapshot().LineStart(1)

	// Drag "hello" after "second": the text moves and stays selected; the
	// insertion point is shown while dragging and the pointer is an arrow.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	target := ed.at(t, st, 1, 6)
	ed.dndPress(ed.at(t, st, 0, 2), ed.at(t, st, 0, 8), target)
	if d := (*ed.vw).vs.dnd; d == nil || d.target != l1+6 {
		t.Fatalf("insertion point while dragging: %+v", d)
	}
	if st.Primary() != sel(0, 5) || text() != doc {
		t.Fatalf("dragging changed the selection or text: %+v %q", st.Primary(), text())
	}
	if c := ed.Cursor(); c != input.CursorDefault {
		t.Errorf("pointer while dragging: %v, want the arrow", c)
	}
	ed.dndRelease(target)
	if got, want := text(), " world\nsecondhello line\n"; got != want {
		t.Fatalf("move: %q, want %q", got, want)
	}
	if want := sel(l1+6-5, l1+6); st.Primary() != want || len(st.Sels) != 1 {
		t.Errorf("selection after move: %+v, want %+v", st.Sels, want)
	}
	if (*ed.vw).vs.dnd != nil {
		t.Error("drag state kept after the drop")
	}
	if ed.changes != 1 {
		t.Errorf("Changed reported %d times, want 1", ed.changes)
	}
	// One undo step, which restores the selection too.
	st.Undo()
	if text() != doc {
		t.Fatalf("undo move: %q", text())
	}
	wantSels(t, st, "undo move", sel(0, 5))

	// Move and release in one frame (a busy machine) after a drag frame:
	// still a drag, dropped where the pointer ended.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2), ed.at(t, st, 0, 8))
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: target})
	ed.Settle()
	ed.Advance(time.Second)
	if got, want := text(), " world\nsecondhello line\n"; got != want {
		t.Fatalf("move released in the frame of the move: %q, want %q", got, want)
	}
	st.Undo()

	// Release then a move in one frame right after the press: a click,
	// whatever the pointer did after the release (the text stays).
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Move, Pos: ed.at(t, st, 0, 2)})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: ed.at(t, st, 0, 2)})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Release, Pos: ed.at(t, st, 0, 2)})
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target})
	ed.Settle()
	ed.Advance(time.Second)
	if text() != doc || st.Primary() != sel(2, 2) {
		t.Fatalf("release then move in one frame: %q %+v", text(), st.Primary())
	}

	// Changing the primary selection while dragging (Shift+Right)
	// cancels the drag.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2), target)
	ed.Inject(input.KeyEvent{Key: input.KeyRight, Down: true, Mods: input.ModShift})
	ed.Inject(input.KeyEvent{Key: input.KeyRight, Mods: input.ModShift})
	ed.Frame()
	ed.dndRelease(target)
	if text() != doc || st.Primary() != sel(0, 6) {
		t.Fatalf("drop after Shift+Right: %q %+v", text(), st.Primary())
	}

	// Moving upwards, before the selection.
	st.SetSelections([]Selection{sel(l1, l1+6)}) // "second"
	ed.Frame()
	target = ed.at(t, st, 0, 0)
	ed.dndPress(ed.at(t, st, 1, 3), target)
	ed.dndRelease(target)
	if got, want := text(), "secondhello world\n line\n"; got != want {
		t.Fatalf("move up: %q, want %q", got, want)
	}
	if st.Primary() != sel(0, 6) {
		t.Errorf("selection after move up: %+v", st.Primary())
	}
	st.Undo()

	// Dropping inside the dragged selection (or on its edges) does
	// nothing.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	for _, col := range []int{4, 5, 0} {
		ed.dndPress(ed.at(t, st, 0, 1), ed.at(t, st, 0, col))
		if d := (*ed.vw).vs.dnd; d == nil || d.target != -1 {
			t.Errorf("insertion point over the selection (col %d): %+v", col, d)
		}
		ed.dndRelease(ed.at(t, st, 0, col))
		if text() != doc || st.Primary() != sel(0, 5) {
			t.Errorf("drop inside at col %d: %q %+v", col, text(), st.Primary())
		}
	}

	// A press on the selection without a drag is a click: a caret there.
	ed.Click(ed.at(t, st, 0, 3))
	ed.Advance(time.Second)
	if st.Primary() != sel(3, 3) || text() != doc {
		t.Errorf("click in the selection: %+v", st.Primary())
	}

	// A press past the end of the line (not on its text) selects as usual.
	st.SetSelections([]Selection{sel(0, l1+3)})
	ed.Frame()
	g := (*ed.vw).vs
	ed.Drag(geom.Pt(g.text.X+g.text.W-5, ed.at(t, st, 0, 0).Y), ed.at(t, st, 1, 8))
	ed.Advance(time.Second)
	if text() != doc || st.Primary() != sel(11, l1+8) {
		t.Errorf("press past the line end: %q %+v", text(), st.Primary())
	}

	// Esc cancels the drag.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2), target)
	ed.Inject(input.KeyEvent{Key: input.KeyEscape, Down: true})
	ed.Frame()
	ed.dndRelease(target)
	if text() != doc {
		t.Errorf("Esc did not cancel: %q", text())
	}

	// Releasing outside the editor drops nothing.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	out := geom.Pt(g.rect.X+g.rect.W+20, g.rect.Y+10)
	ed.dndPress(ed.at(t, st, 0, 2), ed.at(t, st, 1, 4), out)
	ed.dndRelease(out)
	if text() != doc {
		t.Errorf("drop outside the editor: %q", text())
	}

	// DragAndDrop off: the press selects as usual.
	st.Options.DragAndDrop = false
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.Drag(ed.at(t, st, 0, 2), ed.at(t, st, 1, 6))
	ed.Advance(time.Second)
	if text() != doc || st.Primary() != sel(2, l1+6) {
		t.Errorf("dragAndDrop off: %q %+v", text(), st.Primary())
	}
	st.Options.DragAndDrop = true
}

// TestDragReleasePos checks that a frame holding several pointer events
// is read in order through Context.ReleasePos: the drop goes where the
// button was released, not where the pointer ended the frame.
func TestDragReleasePos(t *testing.T) {
	goText(t)
	doc := "hello world\nsecond line\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	l1 := st.Snapshot().LineStart(1)
	moved := " world\nsecondhello line\n"

	// Press, then Move+Release in one frame with no drag frame between:
	// a drop at the release.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	press, target := ed.at(t, st, 0, 2), ed.at(t, st, 1, 6)
	ed.dndPress(press)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: target})
	ed.Settle()
	ed.Advance(time.Second)
	if got := text(); got != moved {
		t.Fatalf("move+release right after the press: %q, want %q", got, moved)
	}
	if want := sel(l1+1, l1+6); st.Primary() != want {
		t.Errorf("selection after the drop: %+v, want %+v", st.Primary(), want)
	}
	st.Undo()

	// Release+Move in one frame: a click at the release point.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(press)
	ed.Inject(input.Pointer{Kind: input.Release, Pos: press})
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target})
	ed.Settle()
	ed.Advance(time.Second)
	if text() != doc || st.Primary() != sel(2, 2) {
		t.Fatalf("release+move: %q %+v, want a caret at 2", text(), st.Primary())
	}

	// A drag in progress, then Release(P)+Move(Q) in one frame: the drop
	// is at P.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	q := ed.at(t, st, 1, 2)
	ed.dndPress(press, ed.at(t, st, 0, 8))
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: target})
	ed.Inject(input.Pointer{Kind: input.Move, Pos: q})
	ed.Settle()
	ed.Advance(time.Second)
	if got := text(); got != moved {
		t.Fatalf("drag then release(P)+move(Q): %q, want %q", got, moved)
	}
	st.Undo()
}

// TestDragCaptureEndsWithoutRelease checks that a drag whose pointer
// capture ends without a release (no drag, no release, no cancel this
// frame) is cancelled and drops nothing.
func TestDragCaptureEndsWithoutRelease(t *testing.T) {
	goText(t)
	doc := "hello world\nsecond line\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	target := ed.at(t, st, 1, 6)
	ed.dndPress(ed.at(t, st, 0, 2), target)
	w := *ed.vw
	if w.vs.dnd == nil {
		t.Fatal("no drag in progress")
	}
	w.dndInput(pending{})
	if w.vs.dnd != nil {
		t.Fatal("drag kept after its capture ended without a release")
	}
	ed.dndRelease(target)
	if st.Snapshot().String() != doc {
		t.Errorf("text changed: %q", st.Snapshot().String())
	}
}

func TestDragCopyWithAlt(t *testing.T) {
	goText(t)
	doc := "hello world\nsecond line\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	l1 := st.Snapshot().LineStart(1)
	alt := func(down bool) {
		m := input.Modifiers(0)
		if down {
			m = input.ModAlt
		}
		ed.Inject(input.KeyEvent{Key: input.KeyAltLeft, Down: down, Mods: m})
		ed.Frame()
	}

	// Alt (Option) pressed during the drag copies.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	target := ed.at(t, st, 1, 6)
	ed.dndPress(ed.at(t, st, 0, 2), target)
	if c := ed.Cursor(); c != input.CursorDefault {
		t.Errorf("pointer while moving: %v, want the arrow", c)
	}
	alt(true)
	if c := ed.Cursor(); c != input.CursorCopy {
		t.Errorf("pointer with Alt held: %v, want the copy arrow", c)
	}
	ed.dndRelease(target, input.ModAlt)
	alt(false)
	if got, want := text(), "hello world\nsecondhello line\n"; got != want {
		t.Fatalf("copy: %q, want %q", got, want)
	}
	if want := sel(l1+6, l1+11); st.Primary() != want {
		t.Errorf("selection after copy: %+v, want %+v", st.Primary(), want)
	}
	st.Undo()
	if text() != doc {
		t.Fatalf("undo copy: %q", text())
	}

	// A copy may go right after (or before) the selection.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2), ed.at(t, st, 0, 8))
	alt(true)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: ed.at(t, st, 0, 5)})
	ed.Frame()
	ed.dndRelease(ed.at(t, st, 0, 5), input.ModAlt)
	alt(false)
	if got, want := text(), "hellohello world\nsecond line\n"; got != want {
		t.Fatalf("copy beside the selection: %q, want %q", got, want)
	}
	if st.Primary() != sel(5, 10) {
		t.Errorf("selection after copy beside: %+v", st.Primary())
	}
	st.Undo()

	// Alt reported only by the pointer events (no key event) copies too.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2))
	ed.Inject(input.Pointer{Kind: input.Move, Pos: target, Mods: input.ModAlt})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Release, Pos: target, Mods: input.ModAlt})
	ed.Settle()
	ed.Advance(time.Second)
	if got, want := text(), "hello world\nsecondhello line\n"; got != want {
		t.Fatalf("copy with Alt on the pointer events: %q, want %q", got, want)
	}
	st.Undo()

	// Releasing Alt before the drop moves again.
	st.SetSelections([]Selection{sel(0, 5)})
	ed.Frame()
	ed.dndPress(ed.at(t, st, 0, 2), target)
	alt(true)
	alt(false)
	ed.dndRelease(target)
	if got, want := text(), " world\nsecondhello line\n"; got != want {
		t.Fatalf("move after releasing Alt: %q, want %q", got, want)
	}
	st.Undo()

	// Windows and Linux copy with Ctrl, not Alt.
	win := keymap.New(keymap.Windows, keymap.DefaultRules(keymap.Windows))
	st2 := NewState(doc, "a.txt", theme.Base("vs"))
	ed2 := newEditDriver(st2, 500, 300, func(w *ViewW) { w.Keymap(win) })
	ed2.focus(t)
	st2.SetSelections([]Selection{sel(0, 5)})
	ed2.Frame()
	tg := ed2.at(t, st2, 1, 6)
	ed2.dndPress(ed2.at(t, st2, 0, 2), tg)
	ed2.Inject(input.KeyEvent{Key: input.KeyCtrlLeft, Down: true, Mods: input.ModCtrl})
	ed2.Frame()
	ed2.dndRelease(tg, input.ModCtrl)
	if got, want := st2.Snapshot().String(), "hello world\nsecondhello line\n"; got != want {
		t.Errorf("Ctrl copy on Windows: %q, want %q", got, want)
	}

	// Read-only: a press on the selection selects as usual, nothing moves.
	st3 := NewState(doc, "a.txt", theme.Base("vs"))
	ed3 := newEditDriver(st3, 500, 300, func(w *ViewW) { pinMac(w); w.ReadOnly(true) })
	ed3.focus(t)
	st3.SetSelections([]Selection{sel(0, 5)})
	ed3.Frame()
	ed3.Drag(ed3.at(t, st3, 0, 2), ed3.at(t, st3, 1, 6))
	if st3.Snapshot().String() != doc || st3.Primary() != sel(2, l1+6) {
		t.Errorf("drag in read-only: %q %+v", st3.Snapshot().String(), st3.Primary())
	}
}

// TestDropText checks DragAndDropCommand's resulting selection and the
// other cursors following the edit.
func TestDropText(t *testing.T) {
	st := NewState("abcdef\nxyz", "a.txt", theme.Base("vs"))
	st.SetSelections([]Selection{sel(1, 3), sel(9, 9)})
	if !st.dropText(1, 3, 5, false) {
		t.Fatal("move did nothing")
	}
	if got := st.Snapshot().String(); got != "adebcf\nxyz" {
		t.Errorf("move right: %q", got)
	}
	wantSels(t, st, "move right", sel(3, 5), sel(9, 9))
	if st.dropText(3, 5, 4, false) {
		t.Error("move into itself changed the text")
	}
	if !st.dropText(3, 5, 7, true) {
		t.Fatal("copy did nothing")
	}
	if got := st.Snapshot().String(); got != "adebcf\nbcxyz" {
		t.Errorf("copy to the next line: %q", got)
	}
	wantSels(t, st, "copy", sel(7, 9), sel(11, 11))
}
