package ui

import (
	"slices"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
)

// Bounded tables (ScrollY) build only the rows in the window. With
// keepEdit (EditableProTable and its presets given ScrollY) rows being
// edited (EditableRows, the cell editor) are built even outside it, so
// their editors keep their drafts and focus while scrolled away; focus
// moving into one of them, or typing in it, scrolls it into view. Plain
// TableW.ScrollY is unchanged: such a row is destroyed when it leaves the
// window.

// keeping reports the keep-edited-rows mode (bounded body and keepEdit).
func (f *tblFrame) keeping() bool { return f.bounded && f.t.keepEdit }

// editRow reports a row being edited (row edit mode or the edited cell).
func (f *tblFrame) editRow(r tblRow) bool {
	if f.editSet[r.key] {
		return true
	}
	e := &f.st.edit
	return e.on && e.key == r.key
}

// keepRows returns the sorted positions of the rows being edited that lie
// outside the window [first, last] (bounded tables only).
func (f *tblFrame) keepRows() []int {
	if !f.keeping() || f.n == 0 {
		return nil
	}
	var keys []string
	if f.t.editRows != nil {
		keys = append(keys, *f.t.editRows...)
	}
	if e := &f.st.edit; e.on {
		keys = append(keys, e.key)
	}
	var out []int
	for _, k := range keys {
		if p := f.posOfKey(k); p >= 0 && (p < f.first || p > f.last) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// posOfKey returns the position of the row with key k on the page (-1:
// not shown). The last position found is tried first and checked against
// the live key, so reordered or edited rows are found again by a scan. A
// key the scan did not find (a row deleted outside the table, or on
// another page, left in EditableRows) is not scanned for again while the
// page data stays the same (tblMissKey), so it does not cost a scan of
// the whole page every frame.
func (f *tblFrame) posOfKey(k string) int {
	st := f.st
	if p, ok := st.keepPos[k]; ok && p >= 0 && p < f.n && f.row(p).key == k {
		return p
	}
	mk, cacheable := f.missKey()
	if m, ok := st.keepMiss[k]; ok && cacheable && m == mk {
		return -1
	}
	for p := 0; p < f.n; p++ {
		if f.row(p).key == k {
			if st.keepPos == nil {
				st.keepPos = map[string]int{}
			}
			if len(st.keepPos) > 64 {
				clear(st.keepPos)
			}
			st.keepPos[k] = p
			delete(st.keepMiss, k)
			return p
		}
	}
	delete(st.keepPos, k)
	if cacheable {
		if st.keepMiss == nil || len(st.keepMiss) > 64 {
			st.keepMiss = map[string]tblMissKey{}
		}
		st.keepMiss[k] = mk
	}
	return -1
}

// tblMissKey identifies the page data a key was not found in: the row
// count, Revision, the page bounds and the cached sort / filter order. A
// key that later appears without any of them changing (a row re-keyed in
// place at the same row count) is found once the row count or Revision
// changes, as the table's cached sorting already requires. Tree data is
// flattened every frame and is not cached (cacheable false).
type tblMissKey struct {
	rows     int
	revision uint64
	start, n int
	ids      *int
	nids     int
}

func (f *tblFrame) missKey() (tblMissKey, bool) {
	if f.tree != nil {
		return tblMissKey{}, false
	}
	k := tblMissKey{rows: f.t.rows, revision: f.t.revision, start: f.start, n: f.n, nids: len(f.ids)}
	if len(f.ids) > 0 {
		k.ids = &f.ids[0] // the table's own cached order, not caller data
	}
	return k, true
}

// buildKeep builds the kept row at position pos outside the window,
// absolutely placed at its own content position (out of view, clipped by
// the body).
func (f *tblFrame) buildKeep(pos int) {
	r := f.row(pos)
	cells := f.spanCells(pos, pos)
	Key(f.c, r.key, func() {
		n := f.buildRow(pos, r, cells)
		n.keepFocus = true
		n.ln.Absolute = true
		n.ln.Pos = flexbox.Pos{Left: 0, Top: float32(f.offsetOf(pos) - f.scroll.base.Y), Right: layout.Unset, Bottom: layout.Unset}
		f.noteFocus(n, r)
	})
}

// noteFocus asks for the edited row r (node n, just built) to be scrolled
// into view on the next build when the focus moved into it since the last
// build (Tab between its editors, an editor focused by Edit or AddRow) or
// the user types in it this frame (keys, text or IME input to an editor
// scrolled out of view, as a browser reveals the caret).
func (f *tblFrame) noteFocus(n *node, r tblRow) {
	c, st := f.c, f.st
	if !f.keeping() || n == nil || c.focus == 0 || !f.editRow(r) {
		return
	}
	if (c.focus == st.focusSeen && !f.typing) || !c.focusWithin(n) {
		return
	}
	leaf := -1
	for _, k := range n.kids {
		if c.focusWithin(k) {
			if i := slices.Index(f.rowIDs, k.id); i >= 0 {
				leaf = f.rowLeaf[i]
			}
			break
		}
	}
	st.revKey, st.revLeaf, st.revOn = r.key, leaf, true
	c.invalid = true
}

// pendingReveal scrolls a row asked for by noteFocus or revealRow into
// view before the window is built.
func (f *tblFrame) pendingReveal() {
	st := f.st
	if !st.revOn {
		return
	}
	st.revOn = false
	if !f.keeping() {
		return
	}
	if p := f.posOfKey(st.revKey); p >= 0 {
		f.ensureVisible(p, st.revLeaf)
	}
}

// tblTyping reports keyboard, text or IME input this frame. It is read
// before the rows are built (tblFrame.typing): an editor consumes the
// frame's events while it builds (a TextEdit, from a mobile IME's delete
// or autocorrect, clears them).
func tblTyping(c *Context) bool {
	if len(c.ime.edits) > 0 {
		return true
	}
	for _, ev := range c.events {
		switch ev := ev.(type) {
		case input.KeyEvent:
			if ev.Down {
				return true
			}
		case input.TextInput, input.Composition, input.TextEdit:
			return true
		}
	}
	return false
}

// revealRow scrolls the row with key k into view on the next build of a
// keepEdit table (no-op otherwise or before the first Show).
func (t *TableW) revealRow(k string) {
	if t == nil || t.st == nil || k == "" || !t.keepEdit {
		return
	}
	t.st.revKey, t.st.revLeaf, t.st.revOn = k, -1, true
	t.c.invalid = true
}
