package editor

import "math"

// Scrolling in pixels and model lines (docs/specs/diff-view.md, 2.4), for
// hosts that keep views in step (the diff view's two sides) or show their
// own rulers. Pixels are logical pixels; ScrollTop 0 is the very top of
// the document, the top padding included (Monaco's getScrollTop).
//
// While wrapping, the rows of lines far from the viewport are estimates
// (viewlines.go): LineTop of such a line may change when rows above it
// become known, the picture does not move (the top model line keeps its
// place), and OnDidScroll does not report it.

// ScrollTop returns the vertical scroll position: the pixels of the
// document above the viewport's top edge, the top padding included.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) ScrollTop() float64 {
	s.clampViewport()
	return s.Viewport.scrollY() + float64(max(s.Viewport.PadTop, 0))
}

// SetScrollTop scrolls vertically to px (see ScrollTop), clamped to the
// document (the last line may scroll up to the top).
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) SetScrollTop(px float64) {
	s.Viewport.setScrollY(px-float64(max(s.Viewport.PadTop, 0)), s.viewCount())
}

// ScrollLeft returns the horizontal scroll position in pixels.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) ScrollLeft() float32 { return s.Viewport.ScrollX }

// SetScrollLeft scrolls horizontally to px, clamped to the content width
// the View last measured.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) SetScrollLeft(px float32) {
	s.Viewport.ScrollX = px
	s.Viewport.clampX()
}

// ScrollHeight returns the scrollable height in pixels: the view lines,
// the top padding and a viewport's height (the last line can scroll to
// the top), as the overview ruler measures the document.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) ScrollHeight() float64 {
	v := s.Viewport
	return float64(max(0, s.viewCount()-1))*float64(v.LineHeight) + float64(max(v.PadTop, 0)) + float64(v.Height)
}

// LineTop returns the top of the first text row of model line line (0
// based, clamped) in ScrollTop's coordinates (Monaco's
// getTopForLineNumber). A hidden line's top is the bottom of the block of
// the shown line before it (its text rows and zones). It measures no line.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) LineTop(line int) float64 {
	vl := s.lines()
	line = max(0, min(line, vl.n-1))
	return float64(vl.viewOf(line))*float64(s.Viewport.LineHeight) + float64(max(s.Viewport.PadTop, 0))
}

// LineBlock returns the top of model line line (LineTop), its text rows
// and the rows of the view zones after it: its block of
// (textRows+zoneRows) rows. A hidden line has no rows.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) LineBlock(line int) (top float64, textRows, zoneRows int) {
	vl := s.lines()
	line = max(0, min(line, vl.n-1))
	if vl.hidden(line) {
		return s.LineTop(line), 0, 0
	}
	return s.LineTop(line), vl.rowsOf(line), vl.zoneRows(line)
}

// LineAtTop returns the model line at the viewport's top edge (whose
// block, or the zones before the first line, the edge is in) and the
// pixels from that line's top to the edge: ScrollTop() == LineTop(line) +
// offset. The offset is negative over the top padding and the zones before
// the first line.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) LineAtTop() (line int, offset float32) {
	s.clampViewport()
	line, row := s.topAnchor()
	v := s.Viewport
	return line, float32(row)*v.LineHeight + v.OffsetY
}

// SetLineAtTop scrolls so that the viewport's top edge is offset pixels
// below the top of model line line (LineAtTop's inverse), clamped to the
// document.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) SetLineAtTop(line int, offset float32) {
	vl := s.lines()
	line = max(0, min(line, vl.n-1))
	v := &s.Viewport
	v.setScrollY(float64(vl.viewOf(line))*float64(v.LineHeight)+float64(offset), vl.Count())
}

// RowsOf returns the text rows of model line line: 1 without wrapping, its
// measured rows while wrapping (it measures the line, keeping the
// picture in place), 0 when hidden.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) RowsOf(line int) int {
	vl := s.lines()
	if line < 0 || line >= vl.n || vl.hidden(line) {
		return 0
	}
	if vl.on() {
		return s.breaks(line).rows()
	}
	return 1
}

// RevealWhere is where RevealLine and RevealRange put what they reveal.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
type RevealWhere uint8

const (
	// RevealNearest scrolls the least amount that shows it.
	RevealNearest RevealWhere = iota
	// RevealCenter centres it vertically.
	RevealCenter
	// RevealCenterIfOutside centres it unless it is fully shown already.
	RevealCenterIfOutside
	// RevealTop puts it at the viewport's top.
	RevealTop
)

// RevealLine scrolls vertically to show the first row of model line line
// (clamped; a hidden line shows where it would be) as where says. While
// wrapping, the lines up to a viewport above it are measured first.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) RevealLine(line int, where RevealWhere) {
	snap := s.Snapshot()
	line = max(0, min(line, snap.LineCount()-1))
	s.RevealRange(snap.LineStart(line), snap.LineStart(line), where)
}

// RevealRange scrolls vertically to show the rows of the offsets [start,
// end] as where says: the whole range when it fits in the viewport,
// otherwise its start. While wrapping, the lines up to a viewport above it
// are measured first.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) RevealRange(start, end int, where RevealWhere) {
	snap := s.Snapshot()
	start, end = clampOff(snap, start), clampOff(snap, end)
	start, end = min(start, end), max(start, end)
	if s.lines().on() {
		l := snap.LineOf(start)
		s.measure(l-s.pageLines()-1, snap.LineOf(end))
	}
	r0, r1 := s.rowOf(start, false), s.rowOf(end, false)
	v := &s.Viewport
	if v.LineHeight <= 0 || v.Height <= 0 {
		v.TopLine, v.OffsetY = r0, 0
		s.clampViewport()
		return
	}
	lh := float64(v.LineHeight)
	h := float64(v.Height)
	top, bottom := float64(r0)*lh, float64(r1+1)*lh
	if bottom-top > h {
		bottom = top + lh // too tall: its start
	}
	y := v.scrollY()
	inside := top >= y && bottom <= y+h
	switch where {
	case RevealNearest:
		switch {
		case top < y:
			y = top
		case bottom > y+h:
			y = bottom - h
		}
	case RevealCenterIfOutside:
		if inside {
			return
		}
		fallthrough
	case RevealCenter:
		y = (top+bottom)/2 - h/2
	case RevealTop:
		y = top
	}
	v.setScrollY(math.Max(y, -float64(max(v.PadTop, 0))), s.viewCount())
}

// ScrollEvent is a change of a view's scroll position (OnDidScroll).
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
type ScrollEvent struct {
	// ScrollTop and ScrollLeft are the new position.
	ScrollTop, ScrollLeft float64
	// TopChanged: the vertical position changed (the model line at the
	// top or the offset from its top); LeftChanged: the horizontal one.
	TopChanged, LeftChanged bool
	// Line and Offset are LineAtTop.
	Line   int
	Offset float32
}

// scrollWatch is a State's OnDidScroll listeners and the position they
// last saw.
type scrollWatch struct {
	subs     listeners[func(ScrollEvent)]
	ok       bool
	line     int
	offset   float32
	left     float32
	notified bool // a callback is running
}

// OnDidScroll calls fn when the view's scroll position changed, compared
// as the model line at the top and the pixels from its top (LineAtTop) and
// ScrollLeft: rows above becoming known while wrapping move no picture
// and are not reported. The View checks after its build (the wheel),
// after applying its input (keys, clicks, reveals) and at the end of its
// paint; changes made directly between frames (SetScrollTop, Viewport)
// are reported at the next check. fn may scroll other views, not this one.
// cancel removes fn.
//
// Experimental: part of the scrolling API (docs/specs/diff-view.md).
func (s *State) OnDidScroll(fn func(ScrollEvent)) (cancel func()) {
	if !s.scroll.ok {
		s.scroll.line, s.scroll.offset = s.LineAtTop()
		s.scroll.left, s.scroll.ok = s.Viewport.ScrollX, true
	}
	return s.scroll.subs.add(fn)
}

// checkScroll reports a scroll position change to the OnDidScroll
// listeners.
func (s *State) checkScroll() {
	sw := &s.scroll
	if len(sw.subs.list) == 0 || sw.notified {
		return
	}
	line, off := s.LineAtTop()
	left := s.Viewport.ScrollX
	top := line != sw.line || off != sw.offset
	lc := left != sw.left
	if !top && !lc {
		return
	}
	sw.line, sw.offset, sw.left = line, off, left
	ev := ScrollEvent{ScrollTop: s.ScrollTop(), ScrollLeft: float64(left), TopChanged: top, LeftChanged: lc, Line: line, Offset: off}
	sw.notified = true
	defer func() { sw.notified = false }()
	sw.subs.each(func(fn func(ScrollEvent)) { fn(ev) })
}
