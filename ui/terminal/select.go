package terminal

import (
	"runtime"
	"strings"
	"unicode"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/terminal/vt"
)

// Selection.
//
// A selection runs from anchor to head, each a column boundary (0..cols)
// on a line. Lines are numbered stably: the number of lines dropped from
// the top of the screen's history (vt.Screen.Evicted) plus the line's
// position in the screen, 0 being the oldest scrollback line. So a
// selection stays on its text while output scrolls the screen, a full
// scrollback drops lines (also while the view is scrolled up) and the
// view scrolls the scrollback. A selection whose lines were all dropped
// is cleared; one partly dropped keeps the rest. A resize clears it.
//
// A press starts a selection, a double click selects a word (separators:
// white space and ()[]{}'"`,;|), a triple click the line; dragging after
// them extends by characters, words or lines. A wide character is
// selected whole. Shift+click extends the selection.

// Selection units.
const (
	unitChar = iota
	unitWord
	unitLine
)

// selPos is a column boundary on a stable line number.
type selPos struct{ row, col int }

func (a selPos) less(b selPos) bool { return a.row < b.row || a.row == b.row && a.col < b.col }

type selection struct {
	anchor, head selPos
	on           bool // a press or select-all made a selection
	dragging     bool // the primary button is down on the grid
	unit         int
	// unitA..unitB is the word or line the press selected, which a drag
	// by words or lines keeps.
	unitA, unitB selPos
}

func (s *selection) clear() { s.on, s.dragging = false, false }

// bounds returns the selection in order and whether it is non-empty.
func (s *selection) bounds() (a, b selPos, ok bool) {
	if !s.on || s.anchor == s.head {
		return selPos{}, selPos{}, false
	}
	a, b = s.anchor, s.head
	if b.less(a) {
		a, b = b, a
	}
	return a, b, true
}

// barState is the overlay scrollbar's retained state.
type barState struct {
	drag bool
	grab float32 // pointer y minus the thumb's top at the press
}

// wordSeps are the characters besides white space that end a word.
const wordSeps = "()[]{}'\"`,;|"

func isSep(r rune) bool {
	return r == 0 || unicode.IsSpace(r) || strings.ContainsRune(wordSeps, r)
}

// absLine returns the line at position abs of s (0 is the oldest
// scrollback line), or nil. vt has no accessor by absolute position, so
// for scrollback lines it moves the viewport there and back; the offset
// is restored before the view's TakeDirty, which therefore sees no change.
func absLine(s *vt.Screen, abs int) *vt.Line {
	sb := s.ScrollbackLen()
	_, rows := s.Size()
	switch {
	case abs < 0 || abs >= sb+rows:
		return nil
	case abs >= sb:
		return s.Line(abs - sb)
	}
	old := s.ViewOffset()
	s.SetViewOffset(sb - abs)
	l := s.ViewLine(0)
	s.SetViewOffset(old)
	return l
}

// line returns the line with stable number row, or nil.
func (w *ViewW) line(s *vt.Screen, row int) *vt.Line { return absLine(s, row-w.vs.evicted) }

// trackEviction updates the stable numbering from the screen's eviction
// count and clears a selection whose lines were all dropped. s is locked.
func (vs *viewState) trackEviction(s *vt.Screen) {
	vs.evicted = int(s.Evicted())
	if _, b, ok := vs.sel.bounds(); ok && b.row < vs.evicted {
		vs.sel.clear()
	}
}

// cellAt returns the view row and column boundary nearest p in the grid
// at r, and the cell column under p.
func (w *ViewW) cellAt(s *vt.Screen, r geom.Rect, p geom.Point) (row, bound, cell int) {
	cols, rows := s.Size()
	m := w.vs.m
	y := int((p.Y - r.Y) / m.lineH)
	if p.Y < r.Y {
		y = 0
	}
	row = min(max(y, 0), rows-1)
	fx := (p.X - r.X) / m.cellW
	bound = min(max(int(fx+0.5), 0), cols)
	cell = min(max(int(fx), 0), cols-1)
	if fx < 0 {
		cell = 0
	}
	return row, bound, cell
}

// stableRow returns the stable number of view row y.
func (w *ViewW) stableRow(s *vt.Screen, y int) int {
	return w.vs.evicted + s.ScrollbackLen() - s.ViewOffset() + y
}

// unitRange returns the word or line around cell x of stable line row.
func (w *ViewW) unitRange(s *vt.Screen, row, x, unit int) (a, b selPos) {
	cols, _ := s.Size()
	if unit == unitLine {
		return selPos{row, 0}, selPos{row, cols}
	}
	l := w.line(s, row)
	if l == nil {
		return selPos{row, x}, selPos{row, x + 1}
	}
	cells := l.Cells
	runeAt := func(i int) rune {
		for i > 0 && cells[i].Width == 0 {
			i-- // right half: the wide character's rune
		}
		return cells[i].Rune
	}
	r := runeAt(x)
	class := func(c rune) int {
		switch {
		case c == 0 || unicode.IsSpace(c):
			return 0
		case isSep(c):
			return 1
		}
		return 2
	}
	k := class(r)
	lo, hi := x, x+1
	if k != 1 { // a separator other than space is a word of its own
		for lo > 0 && class(runeAt(lo-1)) == k {
			lo--
		}
		for hi < len(cells) && class(runeAt(hi)) == k {
			hi++
		}
	}
	return selPos{row, lo}, selPos{row, hi}
}

// pointerInput applies the frame's presses, drags and releases: the
// scrollbar first, then the selection. v is locked.
func (w *ViewW) pointerInput(v *vt.Terminal, p pending, r geom.Rect) {
	vs := w.vs
	s := v.Screen()
	sel := &vs.sel
	if p.press {
		if thumb, ok := w.thumbRect(s, r); ok && p.pressPos.X >= r.Right()-scrollbarWidth {
			vs.bar.drag = true
			if thumb.Contains(p.pressPos) {
				vs.bar.grab = p.pressPos.Y - thumb.Y
			} else {
				vs.bar.grab = thumb.H / 2
				w.dragBar(s, r, p.pressPos.Y)
			}
		} else {
			y, bound, cell := w.cellAt(s, r, p.pressPos)
			row := w.stableRow(s, y)
			sel.dragging = true
			switch {
			case p.clicks == 1 && p.shift && sel.on:
				sel.unit = unitChar
				sel.head = selPos{row, bound}
			case p.clicks == 1:
				sel.unit = unitChar
				sel.anchor, sel.head, sel.on = selPos{row, bound}, selPos{row, bound}, true
			default:
				sel.unit = unitWord
				if p.clicks == 3 {
					sel.unit = unitLine
				}
				a, b := w.unitRange(s, row, cell, sel.unit)
				sel.unitA, sel.unitB = a, b
				sel.anchor, sel.head, sel.on = a, b, true
			}
		}
	}
	if p.drag {
		switch {
		case vs.bar.drag:
			w.dragBar(s, r, p.dragPos.Y)
		case sel.dragging:
			y, bound, cell := w.cellAt(s, r, p.dragPos)
			row := w.stableRow(s, y)
			if sel.unit == unitChar {
				sel.head = selPos{row, bound}
				break
			}
			a, b := w.unitRange(s, row, cell, sel.unit)
			if a.less(sel.unitA) {
				sel.anchor, sel.head = sel.unitB, a
			} else {
				sel.anchor, sel.head = sel.unitA, b
			}
		}
	}
	if p.release {
		vs.bar.drag = false
		sel.dragging = false
	}
}

// thumbRect returns the scrollbar thumb in the grid at r, and false when
// there is no scrollback.
func (w *ViewW) thumbRect(s *vt.Screen, r geom.Rect) (geom.Rect, bool) {
	sb := s.ScrollbackLen()
	if sb == 0 {
		return geom.Rect{}, false
	}
	_, rows := s.Size()
	total := float32(sb + rows)
	h := max(r.H*float32(rows)/total, 20)
	top := r.Y + (r.H-h)*float32(sb-s.ViewOffset())/float32(sb)
	return geom.Rect{X: r.Right() - scrollbarWidth, Y: top, W: scrollbarWidth, H: h}, true
}

// dragBar scrolls so that the thumb's top follows pointer y.
func (w *ViewW) dragBar(s *vt.Screen, r geom.Rect, y float32) {
	thumb, ok := w.thumbRect(s, r)
	if !ok || r.H <= thumb.H {
		return
	}
	sb := s.ScrollbackLen()
	f := (y - w.vs.bar.grab - r.Y) / (r.H - thumb.H)
	f = min(max(f, 0), 1)
	s.SetViewOffset(sb - int(f*float32(sb)+0.5))
}

// rowRange returns the selected columns [lo, hi) of line l with stable
// number row, widened to whole wide characters, or ok=false.
func rowRange(l *vt.Line, row int, a, b selPos) (lo, hi int, ok bool) {
	if l == nil || row < a.row || row > b.row {
		return 0, 0, false
	}
	cols := len(l.Cells)
	lo, hi = 0, cols
	if row == a.row {
		lo = min(a.col, cols)
	}
	if row == b.row {
		hi = min(b.col, cols)
	}
	if lo > 0 && lo < cols && l.Cells[lo].Width == 0 {
		lo--
	}
	if hi > 0 && hi < cols && l.Cells[hi].Width == 0 {
		hi++
	}
	return lo, hi, lo < hi
}

// eol is the line break of copied text.
var eol = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

// selectedText returns the selected text: soft-wrapped lines are joined
// without a line break and trailing blanks of each line are dropped.
// s is locked.
func (w *ViewW) selectedText(s *vt.Screen) string {
	a, b, ok := w.vs.sel.bounds()
	if !ok || b.row < w.vs.evicted {
		return ""
	}
	if a.row < w.vs.evicted {
		a = selPos{w.vs.evicted, 0} // its first lines were dropped
	}
	var out, line strings.Builder
	for row := a.row; row <= b.row; row++ {
		l := w.line(s, row)
		lo, hi, ok := rowRange(l, row, a, b)
		line.Reset()
		if ok {
			for x := lo; x < hi; x++ {
				c := l.Cells[x]
				if c.Width == 0 {
					continue
				}
				if c.Rune == 0 {
					line.WriteByte(' ')
					continue
				}
				line.WriteRune(c.Rune)
				for _, m := range l.Combining(x) {
					line.WriteRune(m)
				}
			}
		}
		wrapped := l != nil && l.Wrapped && row < b.row
		txt := line.String()
		if !wrapped {
			txt = strings.TrimRight(txt, " ")
		}
		out.WriteString(txt)
		if row < b.row && !wrapped {
			out.WriteString(eol)
		}
	}
	return out.String()
}

// selectAll selects the scrollback and the screen down to the cursor's
// line or the last non-blank line below it. v is locked.
func (w *ViewW) selectAll(v *vt.Terminal) {
	s := v.Screen()
	cols, rows := s.Size()
	_, last, _ := v.Cursor()
	for y := rows - 1; y > last; y-- {
		if !blankLine(s.Line(y)) {
			last = y
			break
		}
	}
	sel := &w.vs.sel
	ev := w.vs.evicted
	sel.anchor, sel.head = selPos{ev, 0}, selPos{ev + s.ScrollbackLen() + last, cols}
	sel.on, sel.dragging = true, false
}
