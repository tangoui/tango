package vt

import "hash/fnv"

// Snapshot is an immutable copy of the terminal state needed by a view. Its
// slices are owned by the snapshot and remain valid until Release.
type Snapshot struct {
	Version          uint64
	Cols, Height     int
	ViewOffset       int
	Evicted          uint64
	CursorX, CursorY int
	CursorVisible    bool
	Title            string
	StyleGeneration  uint64
	Styles           []Style
	Rows             []SnapshotRow
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
	cols, rows := s.Size()
	x, y, visible := t.Cursor()
	out := Snapshot{
		Version:         t.generation,
		Cols:            cols,
		Height:          rows,
		ViewOffset:      s.ViewOffset(),
		Evicted:         s.Evicted(),
		CursorX:         x,
		CursorY:         y,
		CursorVisible:   visible,
		Title:           t.Title(),
		StyleGeneration: uint64(s.styles.gen)<<32 | uint64(len(s.styles.styles)),
		Styles:          append([]Style(nil), s.styles.styles...),
		Rows:            make([]SnapshotRow, rows),
	}
	for y := range rows {
		line := s.ViewLine(y)
		abs := s.sbLen - s.offset + y
		row := SnapshotRow{
			ID:      s.dropped + uint64(abs),
			Cells:   append([]Cell(nil), line.Cells...),
			Wrapped: line.Wrapped,
		}
		for x, c := range line.Cells {
			if c.Rune == 0 {
				continue
			}
			marks := line.Combining(x)
			if len(marks) > 0 {
				row.Combining = append(row.Combining, SnapshotCombining{Column: x, Runes: append([]rune(nil), marks...)})
			}
		}
		row.Version = snapshotRowVersion(row)
		out.Rows[y] = row
	}
	return out
}

// Release drops the snapshot's owned memory. It is idempotent.
func (s *Snapshot) Release() {
	if s == nil || s.released {
		return
	}
	s.Styles = nil
	for i := range s.Rows {
		s.Rows[i].Cells = nil
		s.Rows[i].Combining = nil
	}
	s.Rows = nil
	s.released = true
}

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
