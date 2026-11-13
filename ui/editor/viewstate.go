package editor

import (
	"slices"
)

// ViewState is the per-editor state worth keeping while an editor shows
// another Model, and restoring when it comes back (Monaco's
// saveViewState / restoreViewState).
type ViewState struct {
	// TopLine and OffsetY are the vertical scroll position: TopLine is the
	// document (model) line at the top, so it stays meaningful when
	// wrapping or folding changes (Viewport.TopLine is a view line; a top
	// inside a wrapped line is saved as that line's first row).
	TopLine int
	OffsetY float32
	// ScrollX is the horizontal scroll in pixels.
	ScrollX float32
	// Selections are the selections, the primary first.
	Selections []Selection
	// Folded are the start lines of the folded regions. Restoring folds
	// the folding ranges of the current text starting there (Monaco keeps
	// their end lines and a checksum too; a start line that no longer
	// starts a range is not folded).
	Folded []int
}

// SaveViewState returns the editor's scroll position, selections and
// folded regions.
func (s *State) SaveViewState() ViewState {
	return ViewState{
		TopLine:    s.topModelLine(),
		OffsetY:    s.Viewport.OffsetY,
		ScrollX:    s.Viewport.ScrollX,
		Selections: slices.Clone(s.sels()),
		Folded:     s.foldedStarts(),
	}
}

// RestoreViewState restores a state SaveViewState returned, possibly for an
// earlier version of the text: out-of-range values are clamped to the
// document (lines to the last line, offsets to the text, OffsetY below the
// line height, scroll offsets to >= 0). Overlapping selections merge;
// without selections the caret goes to the document start.
func (s *State) RestoreViewState(v ViewState) {
	snap := s.Model.Snapshot()
	n := snap.LineCount()
	vp := &s.Viewport
	// The folded regions first, so the top line is anchored in the view
	// lines they leave.
	s.restoreFolds(v.Folded)
	top := max(0, min(v.TopLine, n-1))
	s.anchorTop(top, 0) // to a view line
	vp.OffsetY = max(0, v.OffsetY)
	if top == 0 {
		vp.OffsetY = max(-vp.PadTop, v.OffsetY) // the top padding
	}
	if lh := vp.LineHeight; lh > 0 && vp.OffsetY >= lh {
		vp.OffsetY = 0
	}
	if top != v.TopLine {
		vp.OffsetY = 0
	}
	vp.ScrollX = max(0, v.ScrollX)
	s.clampViewport()
	sels := []Selection{{}}
	if len(v.Selections) > 0 {
		sels = v.Selections
	}
	s.Sels, _ = mergeSels(clampSels(snap, slices.Clone(sels), true))
	s.lastAdded = len(s.Sels) - 1
	s.goals = nil
	s.lastKind = kindNone
	s.revealFolds()
}

// topModelLine returns the model line at the top of the viewport.
func (s *State) topModelLine() int {
	l, _ := s.topAnchor()
	return l
}
