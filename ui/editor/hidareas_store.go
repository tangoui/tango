package editor

import (
	"slices"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// This file holds the storage of a view's hidden areas per owner
// (docs/specs/diff-view.md, 2.3), which the view will hide together with
// its folds. It is not wired to State yet.

// hidArea is one hidden line range, tracked like a decoration that
// always grows at its edges (Monaco's hidden areas): s at the start of the
// first line with Left bias, e at the end of the last line with Right
// bias, moved back to the line edges after every change. lines caches the
// current inclusive line span.
type hidArea struct {
	s, e  int
	lines lineSpan
}

// hidAreaStore holds the hidden areas of one view.
type hidAreaStore struct {
	owners  map[string][]hidArea
	names   []string // owners, sorted: at() reports the first by name
	version uint64   // changes whenever an area or its lines change
}

func newHidAreaStore() *hidAreaStore {
	return &hidAreaStore{owners: map[string][]hidArea{}}
}

// set replaces the hidden areas of owner with spans (inclusive model
// lines of snap, clamped to the text; empty spans are dropped); no spans
// removes the owner.
func (st *hidAreaStore) set(owner string, spans []lineSpan, snap buffer.Snapshot) {
	st.version++
	last := snap.LineCount() - 1
	var areas []hidArea
	for _, sp := range spans {
		a, b := max(sp.a, 0), min(sp.b, last)
		if a > b {
			continue
		}
		areas = append(areas, hidArea{s: snap.LineStart(a), e: lineEndOffset(snap, b), lines: lineSpan{a, b}})
	}
	i, found := slices.BinarySearch(st.names, owner)
	if len(areas) == 0 {
		delete(st.owners, owner)
		if found {
			st.names = slices.Delete(st.names, i, i+1)
		}
		return
	}
	st.owners[owner] = areas
	if !found {
		st.names = slices.Insert(st.names, i, owner)
	}
}

// get returns owner's current hidden lines, in the order set.
func (st *hidAreaStore) get(owner string) []lineSpan {
	areas := st.owners[owner]
	if areas == nil {
		return nil
	}
	out := make([]lineSpan, len(areas))
	for i, a := range areas {
		out[i] = a.lines
	}
	return out
}

// apply moves the areas through the edits of t into snap, the text after
// them.
func (st *hidAreaStore) apply(t *buffer.Tracker, snap buffer.Snapshot) {
	changed := false
	for _, areas := range st.owners {
		for i := range areas {
			h := &areas[i]
			ls := lineSpan{
				snap.LineOf(snapToRune(snap, t.Map(h.s, buffer.Left), buffer.Left)),
				snap.LineOf(snapToRune(snap, t.Map(h.e, buffer.Right), buffer.Right)),
			}
			// Areas hide whole lines: the ends go back to the line edges.
			h.s, h.e = snap.LineStart(ls.a), lineEndOffset(snap, ls.b)
			if ls != h.lines {
				h.lines = ls
				changed = true
			}
		}
	}
	if changed {
		st.version++
	}
}

// at returns the owner (the first by name) hiding line.
func (st *hidAreaStore) at(line int) (owner string, ok bool) {
	for _, name := range st.names {
		for _, h := range st.owners[name] {
			if h.lines.a <= line && line <= h.lines.b {
				return name, true
			}
		}
	}
	return "", false
}

// union returns, appended to dst, the lines hidden by folds (sorted,
// disjoint spans, as foldState.hidden returns them) or by any owner's
// areas: sorted spans with overlapping and adjacent ones merged, what
// viewLines.setHidden takes.
func (st *hidAreaStore) union(folds []lineSpan, dst []lineSpan) []lineSpan {
	if len(st.owners) == 0 {
		return append(dst, folds...)
	}
	all := slices.Clone(folds)
	for _, areas := range st.owners {
		for _, h := range areas {
			all = append(all, h.lines)
		}
	}
	slices.SortFunc(all, func(x, y lineSpan) int { return x.a - y.a })
	base := len(dst)
	for _, sp := range all {
		if k := len(dst); k > base && dst[k-1].b >= sp.a-1 {
			dst[k-1].b = max(dst[k-1].b, sp.b)
			continue
		}
		dst = append(dst, sp)
	}
	return dst
}
