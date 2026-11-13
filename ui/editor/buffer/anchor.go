package buffer

import (
	"fmt"
	"slices"
	"unicode/utf8"
)

// Bias says which way an Anchor moves when text is inserted exactly at it.
type Bias int

const (
	// Left keeps the anchor before text inserted at its offset (it sticks
	// to the character on its left).
	Left Bias = iota
	// Right moves the anchor after text inserted at its offset (it sticks
	// to the character on its right).
	Right
)

func (b Bias) String() string {
	switch b {
	case Left:
		return "Left"
	case Right:
		return "Right"
	}
	return fmt.Sprintf("Bias(%d)", int(b))
}

// Anchor is a position that follows the text through edits, undo and redo.
// It records an offset and the point in its Buffer's edit log where the
// offset was valid; Buffer.Resolve replays the later edits to find the
// current offset. Anchors are small values and need no registration or
// cleanup.
//
// A range is two anchors, and the bias of each end expresses Monaco's
// TrackedRangeStickiness:
//
//	AlwaysGrowsWhenTypingAtEdges  start Left,  end Right
//	NeverGrowsWhenTypingAtEdges   start Right, end Left
//	GrowsOnlyWhenTypingBefore     start Left,  end Left
//	GrowsOnlyWhenTypingAfter      start Right, end Right
//
// An anchor belongs to the Buffer that created it; resolving it against
// another Buffer gives meaningless results.
type Anchor struct {
	off  int
	bias Bias
	seq  int // len(Buffer.log) when off was valid
}

// Bias returns the anchor's bias.
func (a Anchor) Bias() Bias { return a.bias }

// Anchor returns an anchor at byte offset off of the current snapshot,
// clamped to [0, Len()].
func (b *Buffer) Anchor(off int, bias Bias) Anchor {
	return Anchor{off: clamp(off, 0, b.snap.Len()), bias: bias, seq: len(b.log)}
}

// Resolve returns a's offset in the current snapshot. Each edit since the
// anchor was made moves it by Monaco's rule for decoration markers
// (intervalTree.ts nodeAcceptEdit, without forceMoveMarkers). For an edit
// replacing bytes [start, start+del) with ins bytes and an anchor at off:
//   - off < start: unchanged;
//   - off == start: a replace or delete (del > 0) keeps it at start; a pure
//     insertion keeps a Left anchor and moves a Right one after the text;
//   - inside the replaced range, offsets in the overlapping prefix
//     off < start+min(del, ins) keep their offset; at off == start+min(del,
//     ins) the anchor stays when the range shrinks (del > ins), otherwise
//     Left stays and Right moves on;
//   - any other offset up to and including the end of the range moves to
//     start+ins (the end of the new text);
//   - off > start+del: shifted by ins-del.
//
// So a whole-document replace (LSP formatting, reload from disk) keeps
// anchors at their offsets, clamped to the new length, rather than
// collapsing them to 0. Offsets are bytes where Monaco counts UTF-16 units;
// a kept offset that falls inside a multi-byte character of the new text is
// snapped to the character's start (Left) or end (Right).
//
// The cost is linear in the number of edits since the anchor was made; to
// keep a long-lived anchor cheap, re-anchor it at the resolved offset (for
// example whenever the history has grown by many edits).
func (b *Buffer) Resolve(a Anchor) int {
	if a.seq < 0 || a.seq > len(b.log) {
		panic(fmt.Sprintf("buffer: anchor from edit %d resolved against a buffer with %d edits", a.seq, len(b.log)))
	}
	off := a.off
	for _, bt := range b.log[a.seq:] {
		off = bt.transform(off, a.bias)
	}
	off = clamp(off, 0, b.snap.Len())
	if b.snap.splitsRune(off) {
		off = b.snap.runeStart(off)
		if a.bias == Right {
			_, size := utf8.DecodeRuneInString(b.snap.slice(off, min(b.snap.Len(), off+utf8.UTFMax)))
			off += size
		}
	}
	return off
}

// span is one edit of a batch in pre-edit offsets: bytes [start, end)
// replaced by n bytes of (EOL-normalised) text.
type span struct{ start, end, n int }

// batch is the shape of one Snapshot.Apply call: sorted, non-overlapping
// spans in the coordinates of the snapshot it was applied to. The text is not
// needed; undo restores stored snapshots.
type batch []span

// newBatch records the shape of edits, dropping no-op edits. The inserted
// length is measured after EOL normalisation, as Snapshot.Apply stores it.
func newBatch(edits []Edit) batch {
	var bt batch
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		bt = append(bt, span{start: e.Start, end: e.End, n: len(normalizeEOL(e.Text))})
	}
	return bt
}

// inverse returns the batch that undoes bt, in the coordinates of the
// snapshot bt produced.
func (bt batch) inverse() batch {
	inv := make(batch, len(bt))
	delta := 0
	for i, sp := range bt {
		start := sp.start + delta
		inv[i] = span{start: start, end: start + sp.n, n: sp.end - sp.start}
		delta += sp.n - (sp.end - sp.start)
	}
	return inv
}

// transform maps offset off through bt. Monaco applies the edits of one
// batch from last to first, each in the coordinates of the original text;
// processing in that order means the edits before an anchor's own edit only
// shift it.
func (bt batch) transform(off int, bias Bias) int {
	for i := len(bt) - 1; i >= 0; i-- {
		sp := bt[i]
		off = acceptEdit(off, sp.start, sp.end-sp.start, sp.n, bias == Left)
	}
	return off
}

// acceptEdit moves one marker through one replace, following Monaco's
// nodeAcceptEdit/adjustMarkerBeforeColumn for a single marker (an
// independent implementation of the same rule; see Resolve).
// stickLeft is Monaco's "stick to previous character".
func acceptEdit(off, start, del, ins int, stickLeft bool) int {
	// staysBefore reports whether a marker at off is left in place by a
	// check at offset at; forceStay decides the tie off == at.
	staysBefore := func(at int, forceStay bool) bool {
		switch {
		case off < at:
			return true
		case off > at:
			return false
		case forceStay:
			return true
		}
		return stickLeft
	}
	if staysBefore(start, del > 0) {
		return off
	}
	if common := min(del, ins); common > 0 && staysBefore(start+common, del > ins) {
		return off
	}
	if staysBefore(start+del, false) {
		return start + ins
	}
	return max(0, off+ins-del)
}

// runeStart returns the start of the character containing byte off (off must
// be inside the text).
func (s Snapshot) runeStart(off int) int {
	lo := max(0, off-(utf8.UTFMax-1))
	window := s.slice(lo, off+1)
	for i := len(window) - 1; i > 0; i-- {
		if utf8.RuneStart(window[i]) {
			return lo + i
		}
	}
	return lo
}

// compose returns the batch equivalent to applying a and then b (b is in the
// coordinates of the text a produced); the result is in a's input
// coordinates. Spans of a that b touches or overlaps merge into one span
// (as do spans of b that touch each other or the same span of a); spans
// that end up replacing nothing with nothing are dropped. A nil a is the
// identity.
//
// It is one merge pass over both batches, O(len(a) + len(b)): undoing a
// step of many batches of many spans (typing at thousands of cursors)
// stays linear in the spans per batch.
func compose(a, b batch) batch {
	if len(a) == 0 {
		return slices.Clone(b)
	}
	out := make(batch, 0, len(a)+len(b))
	i, da := 0, 0 // next span of a; length change of the spans of a before it
	// The open group: [gs, ge) in a's output coordinates, os its start in
	// a's input coordinates, extra the length change of its spans of b.
	var gs, ge, os, extra int
	open := false
	flush := func() {
		if open {
			// da now includes every span of a in the group.
			out = append(out, span{start: os, end: ge - da, n: ge - gs + extra})
			open = false
		}
	}
	for _, y := range b {
		if open && ge < y.start {
			flush()
		}
		if !open {
			// Spans of a ending before y pass through unchanged.
			for i < len(a) && a[i].start+da+a[i].n < y.start {
				out = append(out, a[i])
				da += a[i].n - (a[i].end - a[i].start)
				i++
			}
			gs, ge, os, extra, open = y.start, y.end, y.start-da, 0, true
		}
		// Spans of a that y touches join the group.
		for i < len(a) && a[i].start+da <= y.end {
			as := a[i].start + da
			if as < gs {
				gs, os = as, a[i].start
			}
			ge = max(ge, as+a[i].n)
			da += a[i].n - (a[i].end - a[i].start)
			i++
		}
		ge = max(ge, y.end)
		extra += y.n - (y.end - y.start)
	}
	flush()
	out = append(out, a[i:]...)
	return slices.DeleteFunc(out, func(sp span) bool { return sp.start == sp.end && sp.n == 0 })
}
