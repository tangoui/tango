package diffview

import (
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// region is a foldable run of unchanged lines (diff.UnchangedRegion) and
// how much of it the user revealed: the first top and the last bottom lines
// show, the lines between are hidden on both sides (Monaco's
// UnchangedRegion with visibleLineCountTop/Bottom).
type region struct {
	orig, mod   int // first line on each side
	n           int // line count, the same on both sides
	top, bottom int // revealed lines at the top and the bottom; top+bottom <= n
}

func (g region) start(s diff.Side) int {
	if s == diff.Original {
		return g.orig
	}
	return g.mod
}

// hidden returns the hidden lines of g on side s (empty when all show).
func (g region) hidden(s diff.Side) diff.LineRange {
	a := g.start(s)
	return diff.LineRange{Start: a + g.top, End: a + g.n - g.bottom}
}

// showMoreAbove reveals k more lines at the top ("show 20 more lines
// above" of the placeholder; RevealLineCount).
func (g *region) showMoreAbove(k int) { g.top = min(g.top+max(k, 0), g.n-g.bottom) }

// showMoreBelow reveals k more lines at the bottom.
func (g *region) showMoreBelow(k int) { g.bottom = min(g.bottom+max(k, 0), g.n-g.top) }

// showAll reveals the whole region.
func (g *region) showAll() { g.bottom = g.n - g.top }

// collapse hides the whole region again.
func (g *region) collapse() { g.top, g.bottom = 0, 0 }

// revealPreference says from which end showLine reveals lines.
type revealPreference uint8

const (
	revealCloser     revealPreference = iota // from the end nearer to the line (Monaco FromCloserSide)
	revealFromTop                            // grow the top part
	revealFromBottom                         // grow the bottom part
)

// showLine reveals line of side s, if hidden, by growing the top or the
// bottom part just enough (Monaco showModifiedLine/showOriginalLine; their
// two versions disagree on the arithmetic, this one is the same for both
// sides).
func (g *region) showLine(s diff.Side, line int, pref revealPreference) {
	h := g.hidden(s)
	if !h.Contains(line) {
		return
	}
	fromTop := line - h.Start + 1
	fromBottom := h.End - line
	if pref == revealFromTop || pref == revealCloser && fromTop < fromBottom {
		g.top += fromTop
	} else {
		g.bottom += fromBottom
	}
}

// regionsOf returns the foldable regions of r, all collapsed.
func regionsOf(r diff.Result, context, minHidden int) []region {
	urs := r.UnchangedRegions(context, minHidden)
	out := make([]region, len(urs))
	for i, u := range urs {
		out[i] = region{orig: u.Original.Start, mod: u.Modified.Start, n: u.Original.Len()}
	}
	return out
}

// hiddenMap is a hidden run of unchanged lines on both sides (equal
// lengths), the state that carries over from one diff result to the next.
type hiddenMap struct{ orig, mod diff.LineRange }

// regionSet is the folding state of a diff view's unchanged regions.
type regionSet struct {
	regions []region
}

// hiddenMaps returns the non-empty hidden runs, in order.
func (rs *regionSet) hiddenMaps() []hiddenMap {
	var out []hiddenMap
	for _, g := range rs.regions {
		if ho := g.hidden(diff.Original); !ho.Empty() {
			out = append(out, hiddenMap{orig: ho, mod: g.hidden(diff.Modified)})
		}
	}
	return out
}

// hidden returns the hidden line ranges of side s, in order (what the
// view passes to editor.State.SetHiddenAreas).
func (rs *regionSet) hidden(s diff.Side) []diff.LineRange {
	var out []diff.LineRange
	for _, g := range rs.regions {
		if h := g.hidden(s); !h.Empty() {
			out = append(out, h)
		}
	}
	return out
}

// ensureVisible reveals line of side s if a region hides it (a selection,
// a search match or "go to line" landed there; Monaco
// ensureModifiedLineIsVisible). It reports whether anything changed.
func (rs *regionSet) ensureVisible(s diff.Side, line int, pref revealPreference) bool {
	for i := range rs.regions {
		if rs.regions[i].hidden(s).Contains(line) {
			rs.regions[i].showLine(s, line, pref)
			return true
		}
	}
	return false
}

func (rs *regionSet) showAll() {
	for i := range rs.regions {
		rs.regions[i].showAll()
	}
}

func (rs *regionSet) collapseAll() {
	for i := range rs.regions {
		rs.regions[i].collapse()
	}
}

// mapHidden maps hidden runs through edits (sorted, non-overlapping, in
// old's offsets) applied to side s, whose text before them was old. Lines
// an edit touches leave the run, which may split in two; the rest moves
// with the text. The other side does not change.
func mapHidden(hs []hiddenMap, s diff.Side, old buffer.Snapshot, edits []buffer.Edit) []hiddenMap {
	les := lineEditsOf(old, edits)
	if len(les) == 0 {
		return hs
	}
	var out []hiddenMap
	for _, h := range hs {
		lines, other := h.mod, h.orig
		if s == diff.Original {
			lines, other = h.orig, h.mod
		}
		off := other.Start - lines.Start
		a := lines.Start
		emit := func(a, b int) {
			if a >= b {
				return
			}
			d := shiftBefore(les, a)
			sl, ol := diff.LineRange{Start: a + d, End: b + d}, diff.LineRange{Start: a + off, End: b + off}
			if s == diff.Original {
				out = append(out, hiddenMap{orig: sl, mod: ol})
			} else {
				out = append(out, hiddenMap{orig: ol, mod: sl})
			}
		}
		for _, e := range les {
			if e.end <= a {
				continue
			}
			if e.start >= lines.End {
				break
			}
			emit(a, e.start)
			a = e.end
		}
		emit(a, lines.End)
	}
	return out
}

// migrate carries the folding state over to the regions of a new diff
// result (Monaco's updateUnchangedRegions): a region keeps hidden what was
// hidden before in the same place (hidden, already mapped to the current
// texts, with the same line offset between the sides); a region with
// several such runs splits into one region per run. A region where nothing
// was hidden shows completely, so editing never folds away lines the user
// was looking at. With first set (no earlier state) every region starts
// collapsed.
func migrate(regions []region, hidden []hiddenMap, first bool) []region {
	if first {
		return regions
	}
	var out []region
	for _, g := range regions {
		end := g.mod + g.n
		var runs []diff.LineRange
		for _, h := range hidden {
			if h.orig.Start-h.mod.Start != g.orig-g.mod {
				continue
			}
			a, b := max(h.mod.Start, g.mod), min(h.mod.End, end)
			if a < b {
				runs = append(runs, diff.LineRange{Start: a, End: b})
			}
		}
		if len(runs) == 0 {
			g.top, g.bottom = 0, g.n
			out = append(out, g)
			continue
		}
		start := g.mod
		for i, h := range runs {
			stop := h.End
			if i == len(runs)-1 {
				stop = end
			}
			out = append(out, region{
				orig: start + g.orig - g.mod, mod: start, n: stop - start,
				top: h.Start - start, bottom: stop - h.End,
			})
			start = stop
		}
	}
	return out
}
