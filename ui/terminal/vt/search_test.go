package vt

import "testing"

func TestSearchBasic(t *testing.T) {
	s := NewScreen(20, 3, 5)
	putLine(s, 0, "hello world")
	putLine(s, 1, "HELLO again")
	putLine(s, 2, "nothing here")

	hits := s.Search("hello", false)
	if len(hits) != 2 {
		t.Fatalf("case-insensitive hits = %d, want 2: %+v", len(hits), hits)
	}
	// Active row 0 is absolute position sbLen+0.
	sb := s.ScrollbackLen()
	if hits[0] != (Match{Line: sb, From: 0, To: 5}) {
		t.Fatalf("hit0 = %+v", hits[0])
	}
	if hits[1] != (Match{Line: sb + 1, From: 0, To: 5}) {
		t.Fatalf("hit1 = %+v", hits[1])
	}

	if got := s.Search("hello", true); len(got) != 1 || got[0].Line != sb {
		t.Fatalf("case-sensitive hits = %+v, want one on row 0", got)
	}
	if got := s.Search("", false); got != nil {
		t.Fatalf("empty query = %+v, want nil", got)
	}
	if got := s.Search("zzz", false); len(got) != 0 {
		t.Fatalf("no-match hits = %+v", got)
	}
}

func TestSearchMultiplePerLine(t *testing.T) {
	s := NewScreen(20, 1, 0)
	putLine(s, 0, "ababab")
	hits := s.Search("ab", false)
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3: %+v", len(hits), hits)
	}
	for i, h := range hits {
		if h.From != i*2 || h.To != i*2+2 {
			t.Fatalf("hit%d = %+v", i, h)
		}
	}
}

func TestSearchScrollbackOrder(t *testing.T) {
	s := NewScreen(10, 2, 10)
	e := &emitter{s: s}
	for range 5 {
		e.line() // L1..L5; L1..L3 scroll into history
	}
	hits := s.Search("L", false)
	if len(hits) != 5 {
		t.Fatalf("hits = %d, want 5: %+v", len(hits), hits)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Line <= hits[i-1].Line {
			t.Fatalf("not oldest-first: %+v", hits)
		}
	}
	if hits[0].Line != 0 {
		t.Fatalf("first hit line = %d, want 0 (oldest scrollback)", hits[0].Line)
	}
}

func TestSearchWideChar(t *testing.T) {
	s := NewScreen(10, 1, 0)
	l := s.Line(0)
	// "中ab": 中 spans cells 0-1, a at 2, b at 3.
	l.Cells[0] = Cell{Rune: '中', Width: 2}
	l.Cells[1] = Cell{Rune: 0, Width: 0}
	l.Cells[2] = Cell{Rune: 'a', Width: 1}
	l.Cells[3] = Cell{Rune: 'b', Width: 1}
	hits := s.Search("中a", false)
	if len(hits) != 1 || hits[0].From != 0 || hits[0].To != 3 {
		t.Fatalf("wide hit = %+v, want {0 3}", hits)
	}
}
