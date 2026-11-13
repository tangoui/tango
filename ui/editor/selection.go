package editor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Selection is the selection: Anchor is where it started, Head is the caret
// (byte offsets). A caret has Anchor == Head.
type Selection = buffer.Selection

// Direction is the direction of a Move or Delete. For the vertical units
// (Line, Page) Backward is up and Forward is down.
type Direction int

const (
	Backward Direction = iota
	Forward
)

// Unit is how far a Move or Delete goes.
type Unit int

const (
	// Char is one grapheme cluster; at a line edge it crosses the line
	// break.
	Char Unit = iota
	// Word moves to the start (Backward) or end (Forward) of the previous or
	// next word, by the language's wordPattern or, without one, runs of
	// Unicode letters, digits, marks and '_'. At a line edge it continues on
	// the adjacent line.
	Word
	// Line moves up or down one line (one view line while wrapping),
	// keeping the goal column.
	Line
	// LineEdge is Home / End. Home goes to the first non-blank character,
	// or to column 0 when already there. On a wrapped line Home and End
	// first go to the edges of the view line (Monaco).
	LineEdge
	// Page moves up or down by the number of lines that fit in the viewport
	// and scrolls by the same amount.
	Page
	// Doc is the start or end of the document.
	Doc
)

// MoveCaret puts the caret at offset to (clamped to the document, and moved
// back to the start of a UTF-8 character it falls inside) and removes the
// secondary cursors, as a click does in Monaco. With extend the primary
// selection's anchor stays and the selection grows or shrinks; otherwise
// it collapses to the caret. It is what a click or a drag maps to.
func (s *State) MoveCaret(to int, extend bool) {
	snap := s.Snapshot()
	off := snap.OffsetOf(snap.PosOf(to))
	sel := Selection{Anchor: off, Head: off}
	if extend {
		sel.Anchor = s.Primary().Anchor
	}
	s.setSels([]Selection{sel}, nil)
}

// Move moves every caret by unit in dir; with extend it extends each
// selection. Without extend, moving by Char from a non-empty selection
// collapses it to its start (Backward) or end (Forward). Vertical moves
// keep each cursor's goal column. Cursors that meet merge. Move does not
// scroll (except Page); call RevealCaret after keyboard moves.
//
// Deviation from Monaco: a vertical move without extend starts from the
// caret (Head) of a non-empty selection; Monaco's moveUp / moveDown start
// from the selection's start (up) or end (down).
func (s *State) Move(dir Direction, unit Unit, extend bool) {
	snap := s.Snapshot()
	sels := s.sels()
	out := make([]Selection, len(sels))
	moved := func(sel Selection, to int) Selection {
		if extend {
			return Selection{Anchor: sel.Anchor, Head: to}
		}
		return Selection{Anchor: to, Head: to}
	}
	switch unit {
	case Line, Page:
		n := 1
		if unit == Page {
			n = s.pageLines()
		}
		goals := s.goals
		if len(goals) != len(sels) {
			goals = make([]int, len(sels))
			for k, sel := range sels {
				goals[k] = s.viewColumn(snap, clampOff(snap, sel.Head), s.headUp(k))
			}
		}
		ups := make([]bool, len(sels))
		for k, sel := range sels {
			to, up := s.verticalUp(snap, clampOff(snap, sel.Head), s.headUp(k), dir, n, goals[k])
			out[k], ups[k] = moved(sel, to), up
		}
		s.setSels(out, goals)
		s.setUps(out, ups)
		if unit == Page {
			dy := float32(n) * s.Viewport.LineHeight
			if dir == Backward {
				dy = -dy
			}
			s.ScrollBy(dy, 0)
		}
		return
	}
	ups := make([]bool, len(sels))
	for k, sel := range sels {
		if unit == LineEdge {
			// Wrapped lines: the row's edges first (Monaco).
			if to, toUp, ok := s.lineEdgeTarget(dir, clampOff(snap, sel.Head), s.headUp(k)); ok {
				out[k] = moved(sel, to)
				ups[k] = toUp
				continue
			}
		}
		if unit == Char && !extend && sel.Anchor != sel.Head {
			a, b := ordered(Selection{Anchor: clampOff(snap, sel.Anchor), Head: clampOff(snap, sel.Head)})
			if dir == Backward {
				out[k] = Selection{Anchor: a, Head: a}
			} else {
				out[k] = Selection{Anchor: b, Head: b}
			}
			continue
		}
		out[k] = moved(sel, s.target(dir, unit, clampOff(snap, sel.Head)))
	}
	s.setSels(out, nil)
	s.setUps(out, ups)
}

// selRange returns the primary selection as an ordered, clamped range.
func (s *State) selRange() (int, int) {
	snap := s.Snapshot()
	p := s.Primary()
	a, b := clampOff(snap, p.Anchor), clampOff(snap, p.Head)
	return min(a, b), max(a, b)
}

func clampOff(snap buffer.Snapshot, off int) int { return max(0, min(off, snap.Len())) }

// clampSel clamps both ends into snap and onto character boundaries.
func clampSel(snap buffer.Snapshot, sel Selection) Selection {
	return Selection{
		Anchor: snap.OffsetOf(snap.PosOf(sel.Anchor)),
		Head:   snap.OffsetOf(snap.PosOf(sel.Head)),
	}
}

// lineEnd returns the offset of the end of line i (before its line break).
func lineEnd(snap buffer.Snapshot, i int) int {
	if i+1 < snap.LineCount() {
		return snap.LineStart(i+1) - 1
	}
	return snap.Len()
}

// target returns where a horizontal move (Char, Word, LineEdge, Doc) from
// off ends; Line and Page are vertical and go through Move (Delete treats
// Page like Line moves without the goal column).
func (s *State) target(dir Direction, unit Unit, off int) int {
	snap := s.Snapshot()
	off = clampOff(snap, off)
	l := snap.LineOf(off)
	start := snap.LineStart(l)
	switch unit {
	case Char:
		text := snap.Line(l)
		col := off - start
		if dir == Backward {
			if col == 0 {
				return max(off-1, 0) // the previous line's break
			}
			return start + prevGrapheme(text, col)
		}
		if col >= len(text) {
			return min(off+1, snap.Len())
		}
		return start + nextGrapheme(text, col)
	case Word:
		return s.wordTarget(snap, dir, l, off-start)
	case LineEdge:
		if dir == Forward {
			return lineEnd(snap, l)
		}
		text := snap.Line(l)
		indent := len(text) - len(strings.TrimLeft(text, " \t"))
		if off-start == indent {
			return start
		}
		return start + indent
	case Doc:
		if dir == Backward {
			return 0
		}
		return snap.Len()
	case Line, Page:
		n := 1
		if unit == Page {
			n = s.pageLines()
		}
		return s.vertical(snap, off, dir, n, -1)
	}
	return off
}

// wordTarget implements Move by Word from column col of line l.
func (s *State) wordTarget(snap buffer.Snapshot, dir Direction, l, col int) int {
	text := snap.Line(l)
	if dir == Forward {
		if col >= len(text) {
			if l+1 >= snap.LineCount() {
				return snap.Len()
			}
			l, col, text = l+1, 0, snap.Line(l+1)
		}
		for _, w := range s.words(text) {
			if w[1] > col {
				return snap.LineStart(l) + w[1]
			}
		}
		return snap.LineStart(l) + len(text)
	}
	if col == 0 {
		if l == 0 {
			return 0
		}
		l--
		text = snap.Line(l)
		col = len(text)
	}
	ws := s.words(text)
	for i := len(ws) - 1; i >= 0; i-- {
		if ws[i][0] < col {
			return snap.LineStart(l) + ws[i][0]
		}
	}
	return snap.LineStart(l)
}

// words returns the byte ranges of the words of a line.
func (s *State) words(text string) [][]int {
	if s.Model.wordRe != nil {
		ms := s.Model.wordRe.FindAllStringIndex(text, -1)
		out := ms[:0]
		for _, m := range ms {
			if m[1] > m[0] {
				out = append(out, m)
			}
		}
		return out
	}
	var out [][]int
	start := -1
	for i, r := range text {
		w := r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
		switch {
		case w && start < 0:
			start = i
		case !w && start >= 0:
			out = append(out, []int{start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, []int{start, len(text)})
	}
	return out
}

// pageLines is the number of (view) lines a Page move covers.
func (s *State) pageLines() int {
	v := s.Viewport
	if v.LineHeight <= 0 || v.Height <= 0 {
		return 1
	}
	return max(1, int(v.Height/v.LineHeight))
}

// vertical moves off n lines up or down to visual column goal (off's own
// column when goal < 0); while wrapping it moves by view lines, the column
// counted within the row (State.viewColumn). Above the first line it goes
// to the document start, below the last to its end.
func (s *State) vertical(snap buffer.Snapshot, off int, dir Direction, n, goal int) int {
	to, _ := s.verticalUp(snap, off, false, dir, n, goal)
	return to
}

// verticalUp is vertical from a caret at off with row affinity up
// (headUp); it also reports the affinity of the result: the end of a row
// that continues on the next one when the goal column lies past it.
func (s *State) verticalUp(snap buffer.Snapshot, off int, up bool, dir Direction, n, goal int) (int, bool) {
	if s.lines().on() {
		line, row := s.rowPos(off, up)
		if goal < 0 {
			goal = s.viewColumn(snap, off, up)
		}
		l, r, ok := s.stepRows(line, row, dir, n)
		switch {
		case ok:
			return s.offsetInRow(snap, l, r, goal)
		case dir == Backward:
			return 0, false
		}
		return snap.Len(), false
	}
	return s.verticalModel(snap, off, dir, n, goal), false
}

// verticalModel is vertical by model lines, wrapped or not (Monaco's
// logical lines).
func (s *State) verticalModel(snap buffer.Snapshot, off int, dir Direction, n, goal int) int {
	l := snap.LineOf(off)
	if dir == Backward {
		l -= n
	} else {
		l += n
	}
	switch {
	case l < 0:
		return 0
	case l >= snap.LineCount():
		return snap.Len()
	}
	if goal < 0 {
		goal = s.column(snap, off)
	}
	return snap.LineStart(l) + s.offsetForColumn(snap.Line(l), goal)
}

// column returns the visual column of off on its line: tabs advance to the
// next multiple of the tab size (State.tabSize), wide characters take two columns, every other
// grapheme cluster one.
func (s *State) column(snap buffer.Snapshot, off int) int {
	l := snap.LineOf(off)
	return s.lineColumn(snap.Line(l), off-snap.LineStart(l))
}

// lineColumn returns the visual column of byte column end of text (see
// column).
func (s *State) lineColumn(text string, end int) int {
	col := 0
	for i := 0; i < end; {
		j := nextGrapheme(text, i)
		col = s.advance(col, text[i:j])
		i = j
	}
	return col
}

// offsetForColumn returns the byte column of the last cluster boundary of
// text at or before visual column goal.
//
// Deviation from Monaco: vertical moves round down (a goal in the middle
// of a tab or a wide character lands before it), where Monaco's
// columnFromVisibleColumn takes the nearer boundary (ties before it).
// Column selection uses the nearer boundary, as Monaco (nearestColumn).
func (s *State) offsetForColumn(text string, goal int) int {
	col := 0
	for i := 0; i < len(text); {
		j := nextGrapheme(text, i)
		next := s.advance(col, text[i:j])
		if next > goal {
			return i
		}
		col, i = next, j
	}
	return len(text)
}

func (s *State) advance(col int, g string) int { return advanceCol(col, g, s.tabSize()) }

// columnWith is column with tab stops every ts columns (the Model's tab
// size for Tab, which ignores the View's display override).
func (s *State) columnWith(snap buffer.Snapshot, off, ts int) int {
	l := snap.LineOf(off)
	text := snap.Line(l)
	col := 0
	for i, end := 0, off-snap.LineStart(l); i < end; {
		j := nextGrapheme(text, i)
		col = advanceCol(col, text[i:j], ts)
		i = j
	}
	return col
}

// advanceCol returns the visual column after grapheme g starting at col,
// with tab stops every ts columns.
func advanceCol(col int, g string, ts int) int {
	if g == "\t" {
		return (col/ts + 1) * ts
	}
	if r, _ := utf8.DecodeRuneInString(g); r < utf8.RuneSelf && len(g) == 1 {
		return col + 1
	}
	return col + graphemeWidth(g)
}
