package editor

import (
	"slices"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Hidden areas per owner (docs/specs/diff-view.md, 2.3): line ranges a
// view hides for an owner (the diff view's unchanged regions), on top of
// its folded regions. The view hides the union of both (State.hiddenLines
// replaces foldState.hidden where the view lines are hidden); the areas
// follow the text like decorations that always grow at their edges
// (hidareas_store.go). The user's fold commands never show them, and the
// editor never shows them on its own: a selection that moves into one is
// drawn at the end of the visible line before it (as in a folded region),
// Left and Right step over it, and the owner decides whether to show it.

// LineRange is the model lines [Start, End) (zero-based, half open), as
// diff.LineRange.
//
// Experimental: part of the hidden areas API (docs/specs/diff-view.md).
type LineRange struct{ Start, End int }

// SetHiddenAreas replaces the lines owner hides in this view; no ranges
// remove the owner. Ranges are lines of the current text, clamped to it;
// empty ones are dropped. They follow the text from then on (lines
// inserted at their edges join them). The view keeps at least one line
// visible: ranges hiding every line leave the first one shown.
//
// Experimental: part of the hidden areas API (docs/specs/diff-view.md).
func (s *State) SetHiddenAreas(owner string, rs []LineRange) {
	snap := s.Snapshot()
	var spans []lineSpan
	for _, r := range rs {
		if r.End > r.Start {
			spans = append(spans, lineSpan{r.Start, r.End - 1})
		}
	}
	if s.hid == nil {
		if len(spans) == 0 {
			return
		}
		s.hid = newHidAreaStore()
	}
	s.hid.set(owner, spans, snap)
	s.hidGen++
	s.track()
	s.applyHidden()
}

// HiddenAreas returns the lines owner hides in this view, moved with the
// edits since they were set, in the order set.
//
// Experimental: part of the hidden areas API (docs/specs/diff-view.md).
func (s *State) HiddenAreas(owner string) []LineRange {
	if s.hid == nil {
		return nil
	}
	spans := s.hid.get(owner)
	if spans == nil {
		return nil
	}
	out := make([]LineRange, len(spans))
	for i, sp := range spans {
		out[i] = LineRange{sp.a, sp.b + 1}
	}
	return out
}

// HiddenAreaAt returns the owner hiding model line line in this view (the
// first by name when several do); false when none does (the line may
// still be inside a folded region).
//
// Experimental: part of the hidden areas API (docs/specs/diff-view.md).
func (s *State) HiddenAreaAt(line int) (owner string, ok bool) {
	if s.hid == nil {
		return "", false
	}
	return s.hid.at(line)
}

// hasHidAreas reports whether some owner hides lines in this view.
func (s *State) hasHidAreas() bool { return s.hid != nil && len(s.hid.owners) > 0 }

// hiddenLines returns the lines the view hides in a document of n lines:
// the folded regions' and the owners' (merged), never every line.
func (s *State) hiddenLines(n int) []lineSpan {
	folds := s.fold.hidden(n)
	if !s.hasHidAreas() {
		return folds
	}
	h := s.hid.union(folds, nil)
	if len(h) > 0 && h[0].a <= 0 && h[0].b >= n-1 {
		h[0].a = 1 // keep the first line
		if h[0].a > h[0].b {
			h = h[1:]
		}
	}
	// Spans are clamped to the text (a stale area of a shorter text).
	for i := range h {
		h[i].b = min(h[i].b, n-1)
	}
	return slices.DeleteFunc(h, func(sp lineSpan) bool { return sp.a > sp.b })
}

// applyHidden hides the view's lines after a change of the hidden areas,
// keeping the top model line at the top (the visible line before it when
// it got hidden).
func (s *State) applyHidden() {
	vl := s.lines()
	line, row := vl.find(s.Viewport.TopLine)
	vl.setHidden(s.hiddenLines(vl.n))
	s.anchorTop(line, row)
	s.clampViewport()
}

// --- tracking ---

// tracks reports whether the view has anything following the text through
// the Model's changes (Model.tracked): hidden areas.
func (s *State) tracks() bool { return s.hasHidAreas() }

// track registers the view with its Model for tracking when it has
// something to track, and unregisters it when it has nothing left.
func (s *State) track() {
	m := s.Model
	i := slices.Index(m.trackers, s)
	switch on := s.tracks() && s.cancel != nil; {
	case on && i < 0:
		m.trackers = append(m.trackers, s)
	case !on && i >= 0:
		m.trackers = slices.Delete(m.trackers, i, i+1)
	}
}

// trackEdits moves the view's hidden areas through a change of its Model
// (t, into ns).
func (s *State) trackEdits(t *buffer.Tracker, ns buffer.Snapshot) {
	if s.hid != nil {
		s.hid.apply(t, ns)
	}
}
