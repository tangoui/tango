package editor

import "math"

// Viewport is the visible part of the document. The scroll position is
// TopLine plus OffsetY pixels (an integer line index keeps it exact on
// documents with millions of lines, where a float32 pixel offset would not
// be). Lines here are view lines: with word wrap a model line may take
// several (viewlines.go); without it view and model lines are the same.
// The View sets the sizes it measured: Height and Width of the text area,
// LineHeight, and ContentWidth, the width of the widest line it knows (0
// when unknown).
//
// Vertical scrolling follows Monaco's default scrollBeyondLastLine: the
// last line may scroll up to the top of the viewport. Options.Padding adds
// PadTop pixels above the first line (scrolled to the top, OffsetY is
// -PadTop on line 0) and lets the last line scroll PadBottom pixels
// further, up to a line height.
type Viewport struct {
	// TopLine is the first view line at least partly visible. While
	// wrapping it may move without the picture moving, when the rows of
	// lines above it become known (see viewLines); SaveViewState records
	// the model line instead.
	TopLine int
	// OffsetY is how many pixels of TopLine are scrolled above the top
	// edge, in [0, LineHeight); on line 0 it goes down to -PadTop, the top
	// padding showing above it.
	OffsetY float32
	// ScrollX is the horizontal scroll in pixels, >= 0.
	ScrollX float32

	Height, Width float32
	LineHeight    float32
	ContentWidth  float32
	// PadTop and PadBottom are Options.Padding, set by the View.
	PadTop, PadBottom float32
}

// scrollY returns the vertical scroll position in pixels.
func (v Viewport) scrollY() float64 {
	return float64(v.TopLine)*float64(v.LineHeight) + float64(v.OffsetY)
}

// setScrollY sets the vertical position to y pixels, clamped to a document
// of n view lines.
func (v *Viewport) setScrollY(y float64, n int) {
	lh := float64(v.LineHeight)
	if lh <= 0 {
		v.TopLine, v.OffsetY = max(0, min(v.TopLine, n-1)), 0
		return
	}
	pb := max(0, min(float64(v.PadBottom), lh-1))
	y = max(-float64(max(v.PadTop, 0)), min(y, float64(n-1)*lh+pb))
	if y < 0 {
		v.TopLine, v.OffsetY = 0, float32(y)
		return
	}
	top := int(math.Floor(y / lh))
	off := y - float64(top)*lh
	if off >= lh { // rounding
		top, off = top+1, 0
	}
	if top > n-1 {
		top, off = n-1, y-float64(n-1)*lh
	}
	v.TopLine, v.OffsetY = top, float32(off)
}

func (v *Viewport) clampX() {
	if v.ContentWidth > 0 {
		v.ScrollX = min(v.ScrollX, max(0, v.ContentWidth-v.Width))
	}
	v.ScrollX = max(0, v.ScrollX)
}

// clampViewport keeps the viewport inside the current document (after an
// edit shrank it).
func (s *State) clampViewport() {
	s.Viewport.setScrollY(s.Viewport.scrollY(), s.viewCount())
	s.Viewport.clampX()
}

// VisibleLines returns the first and last (inclusive) view lines that are
// at least partly visible (the same as model lines without word wrap).
// Without a viewport size it returns TopLine twice.
func (s *State) VisibleLines() (first, last int) {
	s.clampViewport()
	v := s.Viewport
	n := s.viewCount()
	if v.LineHeight <= 0 || v.Height <= 0 {
		return v.TopLine, v.TopLine
	}
	rows := int(math.Ceil(float64(v.OffsetY+v.Height)/float64(v.LineHeight))) - 1
	return v.TopLine, min(n-1, v.TopLine+max(rows, 0))
}

// ScrollBy scrolls by dy pixels down and dx pixels right, clamped to the
// document (vertically to the last line at the top, horizontally to
// ContentWidth when known).
func (s *State) ScrollBy(dy, dx float32) {
	n := s.viewCount()
	s.Viewport.setScrollY(s.Viewport.scrollY()+float64(dy), n)
	s.Viewport.ScrollX += dx
	s.Viewport.clampX()
}

// RevealCaret scrolls vertically the least amount that makes the caret's
// view line fully visible (its top when the viewport is shorter than a
// line). Horizontal reveal needs the caret's x, which only the View can
// measure: see RevealX.
func (s *State) RevealCaret() {
	s.revealAt(s.Primary().Head, s.caretUp())
}

// revealLine scrolls vertically the least amount that makes the first view
// line of model line l fully visible.
func (s *State) revealLine(l int) {
	snap := s.Snapshot()
	l = max(0, min(l, snap.LineCount()-1))
	s.revealAt(snap.LineStart(l), false)
}

// revealAt scrolls vertically the least amount that makes the view line of
// offset off (up: see caretUp) fully visible. While wrapping, the lines up
// to a viewport above it are measured first, so the rows between the new
// top and off are known.
func (s *State) revealAt(off int, up bool) {
	if s.lines().on() {
		snap := s.Snapshot()
		l := snap.LineOf(clampOff(snap, off))
		s.measure(l-s.pageLines()-1, l)
	}
	s.revealRow(s.rowOf(off, up))
}

// revealRow scrolls vertically the least amount that makes view line r
// fully visible.
func (s *State) revealRow(l int) {
	v := &s.Viewport
	if v.LineHeight <= 0 || v.Height <= 0 {
		v.TopLine, v.OffsetY = l, 0
		s.clampViewport()
		return
	}
	lh := float64(v.LineHeight)
	top := float64(l) * lh
	y := v.scrollY()
	switch {
	case top < y:
		y = top
	case top+lh > y+float64(v.Height):
		y = max(top+lh-float64(v.Height), 0)
		if v.Height < v.LineHeight {
			y = top
		}
	}
	v.setScrollY(y, s.viewCount())
}

// RevealX scrolls horizontally the least amount that makes content x range
// [x0, x1] (the caret, as measured by the View) visible.
func (s *State) RevealX(x0, x1 float32) {
	v := &s.Viewport
	switch {
	case x0 < v.ScrollX:
		v.ScrollX = x0
	case x1 > v.ScrollX+v.Width:
		v.ScrollX = x1 - v.Width
	}
	v.clampX()
}
