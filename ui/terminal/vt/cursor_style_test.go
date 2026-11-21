package vt

import "testing"

func TestDECSCUSR(t *testing.T) {
	v := New(Options{Cols: 10, Rows: 2})
	for _, tc := range []struct {
		seq  string
		want CursorStyle
	}{
		{"", CursorDefault}, {"\x1b[ q", CursorBlinkBlock}, {"\x1b[2 q", CursorSteadyBlock},
		{"\x1b[3 q", CursorBlinkUnderline}, {"\x1b[4 q", CursorSteadyUnderline},
		{"\x1b[5 q", CursorBlinkBar}, {"\x1b[6 q", CursorSteadyBar},
		{"\x1b[7 q", CursorSteadyBar}, {"\x1b[2q", CursorSteadyBar},
		{"\x1b[?2 q", CursorSteadyBar}, {"\x1b[0 q", CursorBlinkBlock},
		{"\x1bc", CursorDefault},
	} {
		for i := range len(tc.seq) {
			v.Write([]byte(tc.seq[i : i+1]))
		}
		if got := v.Snapshot().CursorStyle; got != tc.want {
			t.Fatalf("%q: cursor style = %v, want %v", tc.seq, got, tc.want)
		}
	}
}
