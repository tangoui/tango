package vt

import "hash/fnv"

// Snapshot is an immutable copy of the terminal state needed by a view. Its
// slices are owned by the snapshot and remain valid until Release.
type Snapshot struct {
	Version          uint64
	Cols, Height     int
	ViewOffsetN      int
	EvictedN         uint64
	Scrollback       int
	CursorX, CursorY int
	CursorVisible    bool
	CursorStyle      CursorStyle
	Title            string
	StyleGeneration  uint64
	StylesData       []Style
	Rows             []SnapshotRow
	rowIndex         map[uint64]int
	styles           StyleTable
	released         bool
}

// SnapshotRow is a copied viewport row. ID remains stable while the same
// scrollback line is retained; Version changes when its content changes.
type SnapshotRow struct {
	ID        uint64
	Version   uint64
	Cells     []Cell
	Wrapped   bool
	Combining []SnapshotCombining
	line      Line
}

// SnapshotCombining stores combining marks for one cell.
type SnapshotCombining struct {
	Column int
	Runes  []rune
}

// Snapshot returns a read-only copy of the current terminal state. The
// caller serializes it with Terminal.Write, Resize and other state reads.
func (t *Terminal) Snapshot() Snapshot {
	s := t.s
	if t.snapshot != nil && t.snapTGen == t.generation && t.snapSGen == s.generation {
		return *t.snapshot
	}
	cols, rows := s.Size()
	x, y, visible := t.Cursor()
	styleGen := uint64(s.styles.gen)<<32 | uint64(len(s.styles.styles))
	out := Snapshot{
		Version:         t.generation,
		Cols:            cols,
		Height:          rows,
		ViewOffsetN:     s.ViewOffset(),
		EvictedN:        s.Evicted(),
		Scrollback:      s.ScrollbackLen(),
		CursorX:         x,
		CursorY:         y,
		CursorVisible:   visible,
		CursorStyle:     t.cursorStyle,
		Title:           t.Title(),
		StyleGeneration: styleGen,
		Rows:            make([]SnapshotRow, rows),
		rowIndex:        make(map[uint64]int, rows),
	}
	prev := t.snapshot
	reuseRows := prev != nil && prev.Cols == cols && prev.Height == rows && prev.StyleGeneration == styleGen
	if reuseRows {
		out.StylesData = prev.StylesData
		out.styles = prev.styles
	} else {
		out.StylesData = append([]Style(nil), s.styles.styles...)
		out.styles = StyleTable{styles: append([]Style(nil), s.styles.styles...), gen: s.styles.gen}
	}
	for y := range rows {
		line := s.ViewLine(y)
		abs := s.sbLen - s.offset + y
		id := s.dropped + uint64(abs)
		reused := false
		if reuseRows {
			if i, ok := prev.rowIndex[id]; ok && snapshotLineEqual(line, prev.Rows[i]) {
				out.Rows[y] = prev.Rows[i]
				reused = true
			}
		}
		if !reused {
			row := snapshotRow(line, id)
			out.Rows[y] = row
		}
		out.rowIndex[id] = y
	}
	t.snapshot = &out
	t.snapTGen, t.snapSGen = t.generation, s.generation
	return out
}

func snapshotRow(line *Line, id uint64) SnapshotRow {
	row := SnapshotRow{
		ID:      id,
		Cells:   append([]Cell(nil), line.Cells...),
		Wrapped: line.Wrapped,
	}
	row.line = Line{Cells: append([]Cell(nil), line.Cells...), Wrapped: line.Wrapped}
	for x, c := range line.Cells {
		if c.Rune == 0 {
			continue
		}
		marks := line.Combining(x)
		if len(marks) > 0 {
			copied := append([]rune(nil), marks...)
			row.Combining = append(row.Combining, SnapshotCombining{Column: x, Runes: copied})
			row.line.comb = append(row.line.comb, combMarks{x: x, r: append([]rune(nil), marks...)})
		}
	}
	row.Version = snapshotRowVersion(row)
	return row
}

func snapshotLineEqual(line *Line, row SnapshotRow) bool {
	if line.Wrapped != row.Wrapped || len(line.Cells) != len(row.Cells) {
		return false
	}
	for i := range line.Cells {
		if line.Cells[i] != row.Cells[i] {
			return false
		}
	}
	return snapshotRowVersion(SnapshotRow{Cells: row.Cells, Wrapped: row.Wrapped, Combining: row.Combining}) == row.Version
}

// Size returns the copied viewport dimensions.
func (s *Snapshot) Size() (cols, rows int) { return s.Cols, s.Height }
func (s *Snapshot) ScrollbackLen() int     { return s.Scrollback }
func (s *Snapshot) ViewOffset() int        { return s.ViewOffsetN }
func (s *Snapshot) Evicted() uint64        { return s.EvictedN }
func (s *Snapshot) ViewLine(y int) *Line   { return &s.Rows[y].line }
func (s *Snapshot) Styles() *StyleTable    { return &s.styles }

// Release is retained for callers that own a snapshot value. Snapshot data
// is immutable and the terminal keeps the latest version for reuse; old
// values are reclaimed normally when no caller references them.
func (s *Snapshot) Release() {}

func snapshotRowVersion(row SnapshotRow) uint64 {
	h := fnv.New64a()
	for _, c := range row.Cells {
		_, _ = h.Write([]byte{byte(c.Rune), byte(c.Rune >> 8), byte(c.Rune >> 16), byte(c.Rune >> 24), byte(c.Style), byte(c.Style >> 8), c.Width})
	}
	for _, m := range row.Combining {
		_, _ = h.Write([]byte{byte(m.Column), byte(m.Column >> 8)})
		for _, r := range m.Runes {
			_, _ = h.Write([]byte{byte(r), byte(r >> 8), byte(r >> 16), byte(r >> 24)})
		}
	}
	if row.Wrapped {
		_, _ = h.Write([]byte{1})
	}
	return h.Sum64()
}
