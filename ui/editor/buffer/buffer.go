package buffer

import (
	"slices"
	"time"
)

// typingMergeWindow is how close together two edits must be for the
// automatic grouping to merge them into one undo step.
const typingMergeWindow = 300 * time.Millisecond

// Selection is a selection or caret as byte offsets: Anchor is where it
// started, Head is where the caret is. A caret has Anchor == Head.
type Selection struct{ Anchor, Head int }

// Buffer is the mutable handle on a document: it owns the current Snapshot,
// the undo/redo history and the edit log used to resolve Anchors.
//
// A Buffer is not safe for concurrent use; drive it from one goroutine (the
// UI thread). The Snapshots it hands out are immutable and may be passed to
// any goroutine.
type Buffer struct {
	// Now is the clock used to group typing into undo steps. It defaults to
	// time.Now; tests inject a fake clock.
	Now func() time.Time

	snap Snapshot
	alt  uint64 // Monaco alternativeVersionId

	undo []*step // oldest first; the last one may be open
	redo []*step // most recently undone last

	// stop is set by PushUndoStop: the next edit starts a new step, and
	// that step is explicit (it absorbs every edit until the next stop).
	stop bool

	log []batch // every change to the text, in order; see Anchor

	// The most recent change (Edit, Undo or Redo): lastOld is the snapshot
	// before it, lastSpans its shape in lastOld's coordinates; see LastChange.
	lastOld   Snapshot
	lastSpans batch
	hasLast   bool
}

// step is one undo step: one or more consecutive Edit calls.
type step struct {
	before, after Snapshot // text before the first and after the last edit
	beforeAlt     uint64
	afterAlt      uint64
	selBefore     []Selection
	selAfter      []Selection
	batches       []batch // forward edits, in order
	last          time.Time
	open          bool // more edits may still merge into it
	explicit      bool // opened after PushUndoStop: merges until the next stop
}

// New returns a Buffer holding text as version 1 (see NewSnapshot).
func New(text string) *Buffer {
	s := NewSnapshot(text)
	return &Buffer{snap: s, alt: s.Version()}
}

// Snapshot returns the current version of the document.
func (b *Buffer) Snapshot() Snapshot { return b.snap }

// AltVersion returns Monaco's alternativeVersionId: it equals
// Snapshot().Version() after an edit, but undo and redo set it back to the
// value the restored state had, so "is the document back to its saved
// state" is a simple comparison even though Version keeps increasing.
func (b *Buffer) AltVersion() uint64 { return b.alt }

// Edit applies edits (as Snapshot.Apply) and records them for undo.
// cursorsBefore and cursorsAfter are the selections before and after the
// edit; Undo and Redo hand them back. Both slices are copied.
//
// Grouping into undo steps:
//   - PushUndoStop always ends the current step. Edits made between two
//     PushUndoStop calls form a single step regardless of timing.
//   - Without an explicit stop, an edit merges into the previous step when it
//     comes within 300 ms of the previous edit (by b.Now) and cursorsBefore
//     equals the previous cursorsAfter (and is not empty), i.e. continuous
//     typing.
//
// On error, or when edits change nothing, the buffer is left unchanged and
// the current snapshot is returned.
func (b *Buffer) Edit(edits []Edit, cursorsBefore, cursorsAfter []Selection) (Snapshot, error) {
	ns, err := b.snap.Apply(edits)
	if err != nil {
		return b.snap, err
	}
	bt := newBatch(edits)
	if len(bt) == 0 {
		return b.snap, nil // valid but changes nothing: no new version
	}
	now := b.now()
	before, beforeAlt := b.snap, b.alt
	b.snap = ns
	b.alt = ns.Version()
	b.log = append(b.log, bt)
	b.redo = nil
	b.setLast(before, bt)

	if st := b.top(); st != nil && st.open && !b.stop &&
		(st.explicit || b.continuesTyping(st, cursorsBefore, now)) {
		st.after = ns
		st.afterAlt = b.alt
		st.selAfter = slices.Clone(cursorsAfter)
		st.batches = append(st.batches, bt)
		st.last = now
		return ns, nil
	}
	if st := b.top(); st != nil {
		st.open = false
	}
	b.undo = append(b.undo, &step{
		before: before, after: ns,
		beforeAlt: beforeAlt, afterAlt: b.alt,
		selBefore: slices.Clone(cursorsBefore),
		selAfter:  slices.Clone(cursorsAfter),
		batches:   []batch{bt},
		last:      now,
		open:      true,
		explicit:  b.stop,
	})
	b.stop = false
	return ns, nil
}

func (b *Buffer) continuesTyping(st *step, cursorsBefore []Selection, now time.Time) bool {
	return len(cursorsBefore) > 0 &&
		slices.Equal(cursorsBefore, st.selAfter) &&
		now.Sub(st.last) <= typingMergeWindow
}

// PushUndoStop ends the current undo step: the next edit starts a new one,
// which then collects every edit until the next PushUndoStop.
func (b *Buffer) PushUndoStop() {
	if st := b.top(); st != nil {
		st.open = false
	}
	b.stop = true
}

// SetEOL changes the document's line ending to eol ("\n" or "\r\n") as an
// undo step of its own; the text (always stored LF-normalised) does not
// change, only EOL and TextWithEOL do. cursors are recorded as the
// selections before and after the step. It returns the new snapshot and
// false when eol is invalid or already the document's line ending (nothing
// changes then). LastChange afterwards reports no edits.
func (b *Buffer) SetEOL(eol string, cursors []Selection) (Snapshot, bool) {
	if (eol != "\n" && eol != "\r\n") || eol == b.snap.EOL() {
		return b.snap, false
	}
	before, beforeAlt := b.snap, b.alt
	b.snap = Snapshot{root: before.root, eol: eol, version: before.version + 1}
	b.alt = b.snap.version
	b.redo = nil
	b.setLast(before, nil)
	if st := b.top(); st != nil {
		st.open = false
	}
	b.undo = append(b.undo, &step{
		before: before, after: b.snap,
		beforeAlt: beforeAlt, afterAlt: b.alt,
		selBefore: slices.Clone(cursors),
		selAfter:  slices.Clone(cursors),
	})
	return b.snap, true
}

// Undo reverts the most recent undo step and ends the current step, so the
// next edit starts a new one. It returns the new snapshot (with a
// new, higher Version but the text of the state before the step), the
// selections from before the step, and false when there is nothing to undo.
func (b *Buffer) Undo() (Snapshot, []Selection, bool) {
	if len(b.undo) == 0 {
		return b.snap, nil, false
	}
	st := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	st.open = false
	var change batch
	for i := len(st.batches) - 1; i >= 0; i-- {
		inv := st.batches[i].inverse()
		b.log = append(b.log, inv)
		change = compose(change, inv)
	}
	b.setLast(b.snap, change)
	b.restore(st.before, st.beforeAlt)
	b.redo = append(b.redo, st)
	return b.snap, slices.Clone(st.selBefore), true
}

// Redo re-applies the most recently undone step, returning the new snapshot,
// the selections from after the step, and false when there is nothing to
// redo.
func (b *Buffer) Redo() (Snapshot, []Selection, bool) {
	if len(b.redo) == 0 {
		return b.snap, nil, false
	}
	st := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.log = append(b.log, st.batches...)
	var change batch
	for _, bt := range st.batches {
		change = compose(change, bt)
	}
	b.setLast(b.snap, change)
	b.restore(st.after, st.afterAlt)
	b.undo = append(b.undo, st)
	return b.snap, slices.Clone(st.selAfter), true
}

// LastChange describes the most recent successful Edit, Undo or Redo: old is
// the snapshot it started from, and edits (sorted, non-overlapping, in old's
// coordinates, with EOL-normalised text) turn old into the current snapshot,
// i.e. old.Apply(edits) has the current text. It is what an incremental
// consumer such as a highlighter needs after Undo and Redo, which return only
// the resulting snapshot. The edits of a multi-edit undo step are composed
// into one batch: nearby or overlapping edits merge, edits far apart stay
// separate. Composition is linear in the step's spans per span (quadratic in
// the worst case), which is fine for typing-sized steps; a single-batch step
// (the usual replace-all) is passed through as is.
//
// Before any change it returns the current snapshot and no edits. Edits that
// cancel out within a step (typing then deleting a character) yield no edit.
func (b *Buffer) LastChange() (old Snapshot, edits []Edit) {
	if !b.hasLast {
		return b.snap, nil
	}
	edits = make([]Edit, 0, len(b.lastSpans))
	delta := 0
	for _, sp := range b.lastSpans {
		ns := sp.start + delta
		edits = append(edits, Edit{Start: sp.start, End: sp.end, Text: b.snap.slice(ns, ns+sp.n)})
		delta += sp.n - (sp.end - sp.start)
	}
	return b.lastOld, edits
}

func (b *Buffer) setLast(old Snapshot, change batch) {
	b.lastOld, b.lastSpans, b.hasLast = old, change, true
}

// restore makes the text of s current under a new version. Snapshots are
// immutable, so the stored one is reused as is: restoring is O(1) and
// reproduces the earlier text exactly.
func (b *Buffer) restore(s Snapshot, alt uint64) {
	b.snap = Snapshot{root: s.root, eol: s.eol, version: b.snap.version + 1}
	b.alt = alt
}

func (b *Buffer) top() *step {
	if len(b.undo) == 0 {
		return nil
	}
	return b.undo[len(b.undo)-1]
}

func (b *Buffer) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}
