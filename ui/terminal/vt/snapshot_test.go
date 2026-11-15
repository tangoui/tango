package vt

import (
	"strings"
	"testing"
)

func TestSnapshotIsIndependentAfterWrite(t *testing.T) {
	tm := New(Options{Cols: 12, Rows: 3, Scrollback: 8})
	tm.Write([]byte("before"))
	old := tm.Snapshot()
	tm.Write([]byte("\r\nafter"))
	defer old.Release()
	if got := snapshotRowText(old.Rows[0]); got != "before" {
		t.Fatalf("old snapshot row = %q, want before", got)
	}
}

func TestSnapshotVersionsOnlyChangeDirtyRows(t *testing.T) {
	tm := New(Options{Cols: 12, Rows: 3, Scrollback: 8})
	tm.Write([]byte("one\r\ntwo"))
	first := tm.Snapshot()
	tm.Write([]byte("\r\nthree"))
	second := tm.Snapshot()
	defer first.Release()
	defer second.Release()
	if len(first.Rows) != len(second.Rows) {
		t.Fatalf("rows = %d and %d", len(first.Rows), len(second.Rows))
	}
	changed := 0
	for i := range first.Rows {
		if first.Rows[i].Version != second.Rows[i].Version {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("changed row versions = %d, want one", changed)
	}
}

func TestSnapshotStyleCompactionInvalidatesGeneration(t *testing.T) {
	tm := New(Options{Cols: 12, Rows: 3, Scrollback: 8})
	first := tm.Snapshot()
	tm.Write([]byte("\x1b[31mred\x1b[0m"))
	second := tm.Snapshot()
	defer first.Release()
	defer second.Release()
	if first.StyleGeneration == second.StyleGeneration {
		t.Fatalf("style generation stayed %d after style change", first.StyleGeneration)
	}
}

func TestSnapshotReleaseAndClose(t *testing.T) {
	tm := New(Options{Cols: 12, Rows: 3})
	s := tm.Snapshot()
	s.Release()
	s.Release()
	// Release is idempotent and the terminal remains usable after releasing.
	tm.Write([]byte("ok"))
}

func snapshotRowText(row SnapshotRow) string {
	out := make([]rune, 0, len(row.Cells))
	for _, c := range row.Cells {
		if c.Width == 0 {
			continue
		}
		if c.Rune == 0 {
			out = append(out, ' ')
		} else {
			out = append(out, c.Rune)
		}
	}
	return strings.TrimRight(string(out), " ")
}
