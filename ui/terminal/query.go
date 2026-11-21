package terminal

import (
	"sort"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/terminal/vt"
)

// CellPosition identifies a terminal cell. Rows are stable while retained
// in the buffer, including when the viewport scrolls. Columns are zero-based.
// Requery positions after resizing the terminal or resetting its screen.
type CellPosition struct {
	Row, Column int
}

// CellRange spans cells in row-major order with an exclusive End.
type CellRange struct {
	Start, End CellPosition
}

// LogicalLine is the text of a terminal line including its soft wraps.
// A trailing run of blank cells is omitted.
type LogicalLine struct {
	Text  string
	cells []logicalCell
}

type logicalCell struct {
	from, to int // UTF-8 byte offsets
	span     CellRange
}

// Range maps a nonempty UTF-8 byte interval in Text to a cell range,
// expanding partial grapheme cells to whole cells.
func (l LogicalLine) Range(from, to int) (CellRange, bool) {
	if from < 0 || to > len(l.Text) || from >= to || len(l.cells) == 0 {
		return CellRange{}, false
	}
	a := sort.Search(len(l.cells), func(i int) bool { return l.cells[i].to > from })
	b := sort.Search(len(l.cells), func(i int) bool { return l.cells[i].to >= to })
	return CellRange{l.cells[a].span.Start, l.cells[b].span.End}, true
}

// CellAt returns the cell under p in window coordinates using the view's
// last layout. Both halves of a wide character resolve to its first cell.
func (w *ViewW) CellAt(p geom.Point) (CellPosition, bool) {
	v := w.t.Lock()
	defer w.t.Unlock()
	return w.gridCell(v.Screen(), w.box.Rect(), p)
}

func (w *ViewW) gridCell(s *vt.Screen, r geom.Rect, p geom.Point) (CellPosition, bool) {
	m := w.vs.m
	if m.cellW == 0 || m.lineH == 0 || !r.Contains(p) {
		return CellPosition{}, false
	}
	x, y := int((p.X-r.X)/m.cellW), int((p.Y-r.Y)/m.lineH)
	cols, rows := s.Size()
	if x >= cols || y >= rows {
		return CellPosition{}, false
	}
	if x > 0 && s.ViewLine(y).Cells[x].Width == 0 {
		x--
	}
	return CellPosition{int(s.Evicted()) + s.ScrollbackLen() - s.ViewOffset() + y, x}, true
}

// LogicalLineAt returns the complete retained logical line under p in
// window coordinates, even if part of it is outside the viewport.
func (w *ViewW) LogicalLineAt(p geom.Point) (LogicalLine, bool) {
	return w.logicalLineAt(w.box.Rect(), p)
}

func (w *ViewW) logicalLineAt(r geom.Rect, p geom.Point) (LogicalLine, bool) {
	v := w.t.Lock()
	defer w.t.Unlock()
	s := v.Screen()
	cell, ok := w.gridCell(s, r, p)
	if !ok {
		return LogicalLine{}, false
	}
	return logicalLine(s, cell.Row), true
}

func logicalLine(s *vt.Screen, row int) LogicalLine {
	evicted := int(s.Evicted())
	row -= evicted
	for row > 0 && s.BufferLine(row-1).Wrapped {
		row--
	}
	var buf strings.Builder
	var cells []logicalCell
	for ; ; row++ {
		line := s.BufferLine(row)
		if line == nil {
			break
		}
		limit := wrappedEnd(line)
		if !line.Wrapped {
			for limit > 0 && line.Cells[limit-1].Width == 1 &&
				(line.Cells[limit-1].Rune == 0 || line.Cells[limit-1].Rune == ' ' && len(line.Combining(limit-1)) == 0) {
				limit--
			}
		}
		for x := 0; x < limit; {
			width := max(int(line.Cells[x].Width), 1)
			start := buf.Len()
			appendCellText(&buf, line, x)
			cells = append(cells, logicalCell{start, buf.Len(),
				CellRange{CellPosition{row + evicted, x}, CellPosition{row + evicted, x + width}}})
			x += width
		}
		if !line.Wrapped {
			break
		}
	}
	return LogicalLine{Text: buf.String(), cells: cells}
}

// wrappedEnd skips the unwritten cell left by a wide glyph wrapping at
// the last column. Printed spaces are real cells.
func wrappedEnd(line *vt.Line) int {
	n := len(line.Cells)
	if line.Wrapped && n > 0 && line.Cells[n-1].Width == 1 && line.Cells[n-1].Rune == 0 {
		return n - 1
	}
	return n
}

func appendCellText(out *strings.Builder, line *vt.Line, x int) {
	cell := line.Cells[x]
	if cell.Rune == 0 {
		out.WriteByte(' ')
		return
	}
	out.WriteRune(cell.Rune)
	for _, r := range line.Combining(x) {
		out.WriteRune(r)
	}
}

// Selection returns the current range, widened to whole glyph cells and
// clipped to retained history. It includes selected blank cells.
func (w *ViewW) Selection() (CellRange, bool) {
	v := w.t.Lock()
	defer w.t.Unlock()
	s := v.Screen()
	w.vs.trackEviction(s)
	a, b, ok := w.vs.sel.bounds()
	if !ok {
		return CellRange{}, false
	}
	if a.row < w.vs.evicted {
		a = selPos{w.vs.evicted, 0}
	}
	start, end := w.line(s, a.row), w.line(s, b.row)
	if start == nil || end == nil {
		return CellRange{}, false
	}
	lo, _, _ := rowRange(start, a.row, a, b)
	_, hi, _ := rowRange(end, b.row, a, b)
	return CellRange{CellPosition{a.row, lo}, CellPosition{b.row, hi}}, true
}

// SelectedText returns the text that CopySelection would copy, joining
// soft wraps and using the platform's newline for hard line breaks.
func (w *ViewW) SelectedText() string {
	v := w.t.Lock()
	defer w.t.Unlock()
	w.vs.trackEviction(v.Screen())
	return w.selectedText(v.Screen())
}

// CopySelection copies the current selection if it has text.
func (w *ViewW) CopySelection() {
	if txt := w.SelectedText(); txt != "" && w.c.Window() != nil {
		w.c.Window().WriteClipboard(txt)
	}
}

// SelectAll selects retained history and the active rows through the last
// content or cursor row.
func (w *ViewW) SelectAll() {
	v := w.t.Lock()
	w.vs.trackEviction(v.Screen())
	w.selectAll(v)
	w.t.Unlock()
	w.c.Invalidator()()
}

// Clear clears scrollback and keeps the cursor's current line at the top.
func (w *ViewW) Clear() {
	v := w.t.Lock()
	w.clear(v)
	w.t.Unlock()
	w.c.Invalidator()()
}

func (w *ViewW) clear(v *vt.Terminal) {
	clearScreen(v)
	w.vs.sel.clear()
	w.vs.trackEviction(v.Screen())
	w.vs.needFull = true
	w.t.echo = echoState{}
}
