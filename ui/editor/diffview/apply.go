package diffview

import (
	"sort"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// lineEdit is the line-level shape of one or more text edits: old lines
// [start, end) became newLen lines. Edits touching the same line merge.
type lineEdit struct{ start, end, newLen int }

func (e lineEdit) delta() int { return e.newLen - (e.end - e.start) }

// lineEditsOf returns the line-level shape of edits (sorted,
// non-overlapping, in old's offsets) in old.
func lineEditsOf(old buffer.Snapshot, edits []buffer.Edit) []lineEdit {
	var out []lineEdit
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		sl, el := old.LineOf(e.Start), old.LineOf(e.End)
		d := len(diff.SplitLines(e.Text)) - 1 - (el - sl)
		if k := len(out); k > 0 && sl < out[k-1].end {
			prev := &out[k-1]
			d += prev.delta()
			prev.end = el + 1
			prev.newLen = prev.end - prev.start + d
			continue
		}
		out = append(out, lineEdit{start: sl, end: el + 1, newLen: el + 1 - sl + d})
	}
	return out
}

// shiftBefore returns the line delta of the edits ending at or before
// line boundary p (p must not lie strictly inside an edit).
func shiftBefore(les []lineEdit, p int) int {
	d := 0
	for _, e := range les {
		if e.end > p {
			break
		}
		d += e.delta()
	}
	return d
}

// applyEdits maps r, a diff of two texts, onto the texts after edits
// (sorted, non-overlapping, in old's offsets) were applied to side, whose
// text before them was old. It is what a diff view shows while the real
// diff is being recomputed (debounce window, background computation):
// blocks below an edit move with the text, and every line an edit touches
// becomes part of a changed block (merged with the blocks it touches), so
// unchanged line pairs always really are equal and no block lands on the
// wrong lines. The result can only over-report changes, never mis-align
// them. Blocks touched by an edit have no Inner ranges (they show as whole
// changed lines); the others keep theirs, shifted. Moves are dropped.
//
// Monaco 0.56 has the same hook (applyModifiedEdits in
// diffEditorViewModel.js) but disables it; this is an independent version.
func applyEdits(r diff.Result, side diff.Side, old buffer.Snapshot, edits []buffer.Edit) diff.Result {
	les := lineEditsOf(old, edits)
	if len(les) == 0 {
		return r
	}
	sideOf := func(c diff.Change) (s, o diff.LineRange) {
		if side == diff.Original {
			return c.Original, c.Modified
		}
		return c.Modified, c.Original
	}
	// otherAt maps a line boundary p of side, inside or at the edge of an
	// unchanged run, to the other side.
	otherAt := func(p int) int {
		i := sort.Search(len(r.Changes), func(i int) bool {
			s, _ := sideOf(r.Changes[i])
			return s.End > p || (s.End == p && s.Start == p)
		})
		if i == 0 {
			return p
		}
		s, o := sideOf(r.Changes[i-1])
		return p + o.End - s.End
	}

	out := r
	out.Changes = make([]diff.Change, 0, len(r.Changes)+len(les))
	out.Moves = nil
	out.Identical = false
	total := shiftBefore(les, int(^uint(0)>>1))
	if side == diff.Original {
		out.OriginalLines += total
	} else {
		out.ModifiedLines += total
	}
	emit := func(s, o diff.LineRange, inner []diff.RangeMapping) {
		c := diff.Change{Original: s, Modified: o, Inner: inner}
		if side == diff.Modified {
			c.Original, c.Modified = o, s
		}
		out.Changes = append(out.Changes, c)
	}

	ci, ei := 0, 0
	for ci < len(r.Changes) || ei < len(les) {
		// Start a group with the next item by start line.
		var gs, ge int
		var startOther, endOther = -1, -1
		members := 0
		takeChange := func() {
			s, o := sideOf(r.Changes[ci])
			if members == 0 || s.Start < gs {
				gs = s.Start
			}
			if s.Start == gs && (startOther < 0 || o.Start < startOther) {
				startOther = o.Start
			}
			if members == 0 || s.End > ge {
				ge, endOther = s.End, o.End
			} else if s.End == ge {
				endOther = max(endOther, o.End)
			}
			members++
			ci++
		}
		edited := false
		takeEdit := func() {
			e := les[ei]
			if members == 0 || e.start < gs {
				gs, startOther = e.start, -1
			}
			if members == 0 || e.end > ge {
				ge, endOther = e.end, -1
			}
			members++
			edited = true
			ei++
		}
		firstChange := ci < len(r.Changes)
		if firstChange && ei < len(les) {
			s, _ := sideOf(r.Changes[ci])
			firstChange = s.Start <= les[ei].start
		}
		if firstChange {
			takeChange()
		} else {
			takeEdit()
		}
		// Changes are separated by unchanged lines, so only edits join a
		// group to a change (or two changes): an edit that overlaps or
		// touches the group joins it, and once the group holds an edit,
		// so does a change that overlaps or touches it.
		for {
			if ei < len(les) && les[ei].start <= ge {
				takeEdit()
				continue
			}
			if edited && ci < len(r.Changes) {
				if s, _ := sideOf(r.Changes[ci]); s.Start <= ge {
					takeChange()
					continue
				}
			}
			break
		}
		if !edited {
			// A change no edit touches: shift it on the edited side.
			c := r.Changes[ci-1]
			s, o := sideOf(c)
			d := shiftBefore(les, s.Start)
			if d == 0 {
				out.Changes = append(out.Changes, c)
				continue
			}
			inner := make([]diff.RangeMapping, len(c.Inner))
			for i, m := range c.Inner {
				if side == diff.Original {
					m.Original.Start.Line += d
					m.Original.End.Line += d
				} else {
					m.Modified.Start.Line += d
					m.Modified.End.Line += d
				}
				inner[i] = m
			}
			emit(diff.LineRange{Start: s.Start + d, End: s.End + d}, o, inner)
			continue
		}
		if startOther < 0 {
			startOther = otherAt(gs)
		}
		if endOther < 0 {
			endOther = otherAt(ge)
		}
		emit(diff.LineRange{Start: gs + shiftBefore(les, gs), End: ge + shiftBefore(les, ge)},
			diff.LineRange{Start: startOther, End: endOther}, nil)
	}
	return out
}
