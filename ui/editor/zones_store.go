package editor

import (
	"slices"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// This file holds the storage of a view's view zones (docs/specs/
// diff-view.md, 2.2): per owner, zones placed after model lines, following
// the text through edits. It is not wired to State or viewLines yet.

// zoneSpec is one zone as its owner sets and reads it. Z, the zone's
// paint and input description, is opaque to the store.
type zoneSpec[Z any] struct {
	id           uint64
	afterLine    int // model line the zone follows; -1: before the first line
	rows         int // height in rows
	showInHidden bool
	z            *Z
}

// zoneItem is a stored zone: its spec (afterLine kept current) and the
// offset that tracks its line.
type zoneItem[Z any] struct {
	spec zoneSpec[Z]
	off  int // end of afterLine (unused for -1)
}

// zoneLineRows is the number of zone rows after one model line.
type zoneLineRows struct{ line, rows int }

// zoneStore holds the zones of one view, per owner.
//
// A zone's line is tracked by an offset at the end of the line with Left
// bias, like a buffer.Anchor: typing at the end of the line, or pressing
// Enter there, leaves the zone after that line (the new line goes below
// the zone, as in Monaco); deleting the line moves it to the line the
// deletion joined into. After every change the offset moves back to the
// end of the zone's line. A zone before the first line stays there.
type zoneStore[Z any] struct {
	owners  map[string][]zoneItem[Z]
	version uint64 // changes whenever a zone or its line changes
}

func newZoneStore[Z any]() *zoneStore[Z] {
	return &zoneStore[Z]{owners: map[string][]zoneItem[Z]{}}
}

// lineEndOffset returns the offset of the end of line l of s (before its
// line break); for l < 0 it returns 0.
func lineEndOffset(s buffer.Snapshot, l int) int {
	switch {
	case l < 0:
		return 0
	case l+1 < s.LineCount():
		return s.LineStart(l+1) - 1
	}
	return s.Len()
}

// set replaces the zones of owner with zs (afterLine in snap's lines,
// clamped to [-1, last line]; negative rows count as 0); empty zs removes
// the owner.
func (st *zoneStore[Z]) set(owner string, zs []zoneSpec[Z], snap buffer.Snapshot) {
	st.version++
	if len(zs) == 0 {
		delete(st.owners, owner)
		return
	}
	items := make([]zoneItem[Z], len(zs))
	last := snap.LineCount() - 1
	for i, z := range zs {
		z.afterLine = min(max(z.afterLine, -1), last)
		z.rows = max(z.rows, 0)
		items[i] = zoneItem[Z]{spec: z, off: lineEndOffset(snap, z.afterLine)}
	}
	st.owners[owner] = items
}

// get returns owner's zones with their current lines, in the order set.
func (st *zoneStore[Z]) get(owner string) []zoneSpec[Z] {
	items := st.owners[owner]
	if items == nil {
		return nil
	}
	out := make([]zoneSpec[Z], len(items))
	for i, it := range items {
		out[i] = it.spec
	}
	return out
}

// apply moves the zones through the edits of t into snap, the text after
// them.
func (st *zoneStore[Z]) apply(t *buffer.Tracker, snap buffer.Snapshot) {
	changed := false
	for _, items := range st.owners {
		for i := range items {
			it := &items[i]
			if it.spec.afterLine < 0 {
				continue
			}
			l := snap.LineOf(snapToRune(snap, t.Map(it.off, buffer.Left), buffer.Left))
			it.off = lineEndOffset(snap, l) // the zone belongs to the line, not a column
			if l != it.spec.afterLine {
				it.spec.afterLine = l
				changed = true
			}
		}
	}
	if changed {
		st.version++
	}
}

// resolve returns the zone layout for the hidden lines hidden (sorted,
// disjoint, inclusive spans): the rows before the first line, and the rows
// after each line that has zones, sorted by line. A zone after a hidden
// line moves up to the nearest visible line above (or before the first
// line), or is left out unless it shows in hidden areas.
func (st *zoneStore[Z]) resolve(hidden []lineSpan) (lead int, rows []zoneLineRows) {
	for _, items := range st.owners {
		for _, it := range items {
			l := it.spec.afterLine
			if l >= 0 {
				i, found := slices.BinarySearchFunc(hidden, l, func(s lineSpan, l int) int {
					switch {
					case s.b < l:
						return -1
					case s.a > l:
						return 1
					}
					return 0
				})
				if found {
					if !it.spec.showInHidden {
						continue
					}
					l = hidden[i].a - 1 // spans are merged: the line above is visible
				}
			}
			if it.spec.rows == 0 {
				continue
			}
			if l < 0 {
				lead += it.spec.rows
				continue
			}
			rows = append(rows, zoneLineRows{l, it.spec.rows})
		}
	}
	slices.SortFunc(rows, func(a, b zoneLineRows) int { return a.line - b.line })
	out := rows[:0]
	for _, r := range rows {
		if k := len(out); k > 0 && out[k-1].line == r.line {
			out[k-1].rows += r.rows
			continue
		}
		out = append(out, r)
	}
	return lead, out
}
