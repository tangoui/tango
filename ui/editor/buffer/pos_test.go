package buffer

import (
	"math"
	"testing"
)

// Huge columns must clamp to the line length, not overflow the read-ahead
// arithmetic and come back as 0.
func TestColumnConversionsHugeInputs(t *testing.T) {
	s := NewSnapshot("first\na中😀b\nlast")
	for _, col := range []int{math.MaxInt, math.MaxInt - 1, math.MaxInt / 3, math.MaxInt/3 + 1} {
		if got := s.UTF16Col(1, col); got != 5 {
			t.Errorf("UTF16Col(1, %d) = %d, want 5", col, got)
		}
		if got := s.ByteCol(1, col); got != 9 {
			t.Errorf("ByteCol(1, %d) = %d, want 9", col, got)
		}
		if got := s.UTF16Col(0, col); got != 5 {
			t.Errorf("UTF16Col(0, %d) = %d, want 5", col, got)
		}
		if got := s.ByteCol(0, col); got != 5 {
			t.Errorf("ByteCol(0, %d) = %d, want 5", col, got)
		}
		if got := s.OffsetOf(Pos{Line: 1, Col: col}); got != 15 {
			t.Errorf("OffsetOf(1, %d) = %d, want 15", col, got)
		}
	}
	for _, col := range []int{math.MinInt, -1} {
		if got := s.UTF16Col(1, col); got != 0 {
			t.Errorf("UTF16Col(1, %d) = %d, want 0", col, got)
		}
		if got := s.ByteCol(1, col); got != 0 {
			t.Errorf("ByteCol(1, %d) = %d, want 0", col, got)
		}
	}
	if got := s.OffsetOf(Pos{Line: math.MaxInt, Col: math.MaxInt}); got != s.Len() {
		t.Errorf("OffsetOf(MaxInt, MaxInt) = %d, want %d", got, s.Len())
	}
	if got := s.OffsetOf(Pos{Line: math.MinInt, Col: math.MinInt}); got != 0 {
		t.Errorf("OffsetOf(MinInt, MinInt) = %d, want 0", got)
	}
}

// OffsetOf never returns an offset inside a multi-byte character: a column
// that lands mid-character snaps back to the character's first byte.
func TestOffsetOfSnapsToRuneStart(t *testing.T) {
	s := NewSnapshot("x\na中😀b") // line 1 starts at 2: a=[2,3) 中=[3,6) 😀=[6,10) b=[10,11)
	cases := []struct{ col, want int }{
		{0, 2}, {1, 3}, {2, 3}, {3, 3}, {4, 6}, {5, 6}, {6, 6}, {7, 6}, {8, 10}, {9, 11}, {100, 11},
	}
	for _, c := range cases {
		got := s.OffsetOf(Pos{Line: 1, Col: c.col})
		if got != c.want {
			t.Errorf("OffsetOf(1, %d) = %d, want %d", c.col, got, c.want)
		}
		if _, err := s.Apply([]Edit{{Start: got, End: got, Text: "!"}}); err != nil {
			t.Errorf("OffsetOf(1, %d) = %d is not editable: %v", c.col, got, err)
		}
	}
	// Invalid UTF-8 bytes are characters of their own; no snapping.
	inv := NewSnapshot("a\x80\x80b")
	for col := range 5 {
		if got := inv.OffsetOf(Pos{Col: col}); got != col {
			t.Errorf("invalid UTF-8: OffsetOf(0, %d) = %d", col, got)
		}
	}
}
