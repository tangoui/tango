package editor

import (
	"image/color"
	"math"
	"slices"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// Dragging text inside the editor (Monaco's dragAndDrop option and its
// DragAndDropController / DragAndDropCommand):
//
// A single press (no Alt, which adds cursors) on the text of the primary
// selection, in a writable view with Options.DragAndDrop, does not move
// the caret: it picks the selection up. Dragging shows the insertion point
// as a dotted bar under the pointer (none while the pointer is over the
// selection itself) and the pointer turns into an arrow. Releasing over
// the editor moves the text there, or copies it while the trigger
// modifier is held (Alt / Option on macOS, Ctrl on Windows and Linux, as
// Monaco's), as one undo step; the moved or copied text is selected.
// Releasing inside the dragged selection does nothing, except that a copy
// may go right before or after it. Releasing without having moved is a
// click: the selection under the pointer becomes a caret there (Shift:
// the primary selection extends to it). Esc (which still runs its own
// keybinding, removeSecondaryCursors with several cursors), losing focus,
// a cancelled pointer, an edit of the text or a change of the primary
// selection (Shift+Arrow while dragging) cancel the drag; releasing
// outside the editor drops nothing.
//
// The drop point is where the button was released (Context.ReleasePos),
// not where the pointer is at the end of the frame, and the drag counts as
// moved when the pointer was held away from the press in an earlier frame
// or was released away from it. So a frame holding several pointer events
// (a busy machine) is read in order: a move then the release drops at the
// release, even right after the press with no frame in between; a release
// then a move is a click at the release point.
//
// Dropping files or text from other applications is not handled (Monaco
// inserts no file content by default either).

// dndState is a drag of selected text in progress.
type dndState struct {
	a, b  int    // the dragged selection, ordered
	ver   uint64 // text version at the press: an edit cancels the drag
	from  geom.Point
	shift bool
	moved bool
	// target is the insertion point shown, -1 for none (the pointer is
	// over the selection, or has not moved).
	target int
	// mac: the trigger modifier is Alt (macOS keymap), else Ctrl.
	mac bool
}

// dndStart starts a drag when the press p (at offset off) is a single
// click on the text of the primary selection (Monaco's MouseDownOperation
// conditions) and reports whether it did.
func (w *ViewW) dndStart(p pending, snap buffer.Snapshot, off int) bool {
	st, vs := w.st, w.vs
	vs.dnd = nil
	if w.opts.ReadOnly || !w.opts.DragAndDrop || p.alt || p.clicks != 1 {
		return false
	}
	a, b := ordered(st.Primary())
	a, b = clampOff(snap, a), clampOff(snap, b)
	if a == b || off < a || off > b || !w.overText(snap, p.pressPos) {
		return false
	}
	vs.dnd = &dndState{a: a, b: b, ver: snap.Version(), from: p.pressPos, shift: p.shift, target: -1, mac: w.keymap().Platform() == keymap.Mac}
	vs.dragMode = dragNone
	return true
}

// overText reports whether window point pt is over the text of a line
// (Monaco's CONTENT_TEXT: not the gutter, not past the end of the line or
// below the last line).
func (w *ViewW) overText(snap buffer.Snapshot, pt geom.Point) bool {
	vs, v := w.vs, w.st.Viewport
	if !vs.text.Contains(pt) {
		return false
	}
	r := w.pointRow(pt)
	if r < 0 || r >= w.st.viewCount() {
		return false
	}
	line, row := w.st.lines().find(r)
	lv := w.rowAt(snap, line, row)
	endX, ok := lv.caretX(lv.runeAt(lv.b1))
	if !ok {
		endX = lv.endX
	}
	return pt.X-vs.text.X+v.ScrollX <= endX
}

// dndInput applies this frame's input to the drag in progress: Esc from
// the keys (the copy modifier is read from Context.PointerMods, see
// dndCopy), then the pointer (still held: drag; released: drop at the
// release position). A capture that ended without a release cancels.
func (w *ViewW) dndInput(p pending) {
	st, vs := w.st, w.vs
	d := vs.dnd
	if a, b := ordered(st.Primary()); !w.ia.Focused || p.cancel || st.Snapshot().Version() != d.ver || a != d.a || b != d.b {
		vs.dnd = nil
		return
	}
	for _, e := range p.events {
		if k, ok := e.(input.KeyEvent); ok && k.Down && k.Key == input.KeyEscape {
			vs.dnd = nil
			return
		}
	}
	if p.drag {
		w.c.KeepPointer(w.box.ID())
		w.dndDrag(p.dragPos)
		return
	}
	if !p.release {
		vs.dnd = nil
		return
	}
	w.dndDrop(p.relPos)
}

// dndCopy reports whether the drag copies: the trigger modifier (Alt on
// macOS, Ctrl elsewhere) is held now (Context.PointerMods: the latest
// pointer or key event, so a bare Alt press counts).
func (w *ViewW) dndCopy(d *dndState) bool {
	m := w.c.PointerMods()
	if d.mac {
		return m.Has(input.ModAlt)
	}
	return m.Has(input.ModCtrl)
}

// dndDrag follows the pointer at pt: the insertion point under it, none
// over the dragged selection.
func (w *ViewW) dndDrag(pt geom.Point) {
	d := w.vs.dnd
	if !w.hasGeom() {
		return
	}
	if pt != d.from {
		d.moved = true
	}
	if !d.moved {
		return
	}
	off := w.offsetAt(w.st.Snapshot(), pt)
	if d.a <= off && off <= d.b {
		d.target = -1
		return
	}
	d.target = off
	w.revealOff(off)
}

// dndDrop ends the drag with the pointer released at pt.
func (w *ViewW) dndDrop(pt geom.Point) {
	st, vs := w.st, w.vs
	d := vs.dnd
	vs.dnd = nil
	if !w.hasGeom() {
		return
	}
	snap := st.Snapshot()
	if !d.moved && pt != d.from {
		d.moved = true // moved and released in one frame
	}
	if !d.moved {
		// A click (Monaco's drop without a drag), at the release point,
		// which is the press point.
		off := w.offsetAt(snap, pt)
		if d.shift {
			st.setSels([]Selection{{Anchor: st.Primary().Anchor, Head: off}}, nil)
			return
		}
		sels := slices.Clone(st.sels())
		for i, x := range sels {
			if a, b := ordered(x); a <= off && off <= b {
				sels[i] = Selection{Anchor: off, Head: off}
			}
		}
		st.setSels(sels, nil)
		return
	}
	if !vs.rect.Contains(pt) {
		return
	}
	off := w.offsetAt(snap, pt)
	cp := w.dndCopy(d)
	inside := d.a <= off && off <= d.b
	if inside && !(cp && (off == d.a || off == d.b)) {
		return
	}
	// The primary selection must still be the dragged text (dropText
	// replaces the primary selection).
	if a, b := ordered(st.Primary()); a != d.a || b != d.b || st.readOnly() {
		return
	}
	if st.dropText(d.a, d.b, off, cp) {
		w.reveal()
	}
}

// dropText moves the text [a, b) to offset t (outside it), or copies it
// there (t may then be a or b), as one undo step, and selects it at its
// new place in place of the primary selection; the other selections
// follow the edit (Monaco's DragAndDropCommand). It reports whether the
// text changed.
func (s *State) dropText(a, b, t int, copy bool) bool {
	snap := s.Snapshot()
	a, b, t = clampOff(snap, a), clampOff(snap, b), clampOff(snap, t)
	if a >= b || (!copy && t >= a && t <= b) || (copy && t > a && t < b) {
		return false
	}
	text := snap.Slice(a, b)
	n := b - a
	edits := []buffer.Edit{{Start: t, End: t, Text: text}}
	var sel Selection
	switch {
	case copy:
		sel = Selection{Anchor: t, Head: t + n}
	case t > b:
		edits = append(edits, buffer.Edit{Start: a, End: b})
		sel = Selection{Anchor: t - n, Head: t}
	default:
		edits = append(edits, buffer.Edit{Start: a, End: b})
		sel = Selection{Anchor: t, Head: t + n}
	}
	s.clampedSels(snap)
	cmds := []opCmd{{edits: edits, cursor: func(*opResult, Selection) Selection { return sel }}}
	return s.runOps(cmds)
}

// dndColor is the insertion point's color: Monaco's .dnd-target border
// (dnd.css) for the theme type.
func dndColor(themeType int) color.NRGBA {
	switch themeType {
	case 1:
		return color.NRGBA{0xAE, 0xAF, 0xAD, 0xFF}
	case 2:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	}
	return color.NRGBA{0, 0, 0, 0xFF}
}

// paintDnd draws the insertion point of a drag in progress: a 2px dotted
// bar at the caret position of the target (Monaco's "border-right: 2px
// dotted"). rowOf returns the laid out row showing an offset (nil when it
// is not laid out), lineY a view line's top. It returns the bar's
// rectangle when it drew one (a mark for the redraw hints, damage.go).
func (w *ViewW) paintDnd(l *paint.List, snap buffer.Snapshot, rowOf func(off int, up bool) *lineView, tx float32, lineY func(int) float32, lh float32, c color.NRGBA) (geom.Rect, bool) {
	d := w.vs.dnd
	if d == nil || d.target < 0 {
		return geom.Rect{}, false
	}
	off := clampOff(snap, d.target)
	lv := rowOf(off, false)
	if lv == nil {
		return geom.Rect{}, false
	}
	x, ok := lv.caretX(lv.runeAt(off - snap.LineStart(lv.i)))
	if !ok {
		return geom.Rect{}, false
	}
	cx := float32(math.Round(float64(tx + x)))
	y0 := lineY(lv.vi)
	for y := y0; y < y0+lh; y += 4 {
		l.Fill(rectXYWH(cx, y, 2, min(2, y0+lh-y)), geom.Radii{}, c)
	}
	return rectXYWH(cx, y0, 2, lh), true
}
