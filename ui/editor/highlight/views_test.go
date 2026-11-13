package highlight

import (
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// frame draws one view: Highlight up to the last visible line, then the
// visible lines' tokens (as the editor View does).
func frame(t *testing.T, h *Highlighter, v ViewKey, s buffer.Snapshot, first, last int) {
	t.Helper()
	h.HighlightFor(v, s, last+1, time.Hour)
	var dst []monarch.Token
	for i := first; i <= last; i++ {
		var final bool
		if dst, final = h.TokensFor(v, s, i, dst); !final {
			t.Fatalf("view %d line %d not final", v, i)
		}
	}
}

func slotsID(h *Highlighter, v ViewKey) *slot {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w := h.st.window(v); w != nil && len(w.slots) > 0 {
		return &w.slots[0]
	}
	return nil
}

// Two views far apart in one document keep a token window each: after the
// first frame the windows neither move nor reallocate, and neither view's
// visible lines are tokenized again.
func TestTwoViewsKeepTheirWindows(t *testing.T) {
	lx := tsLexer(t)
	h := New(lx)
	h.TokenWindow = 200
	s := buffer.NewSnapshot(tsDoc(t, 20000))
	highlightAll(t, h, s)
	a, b := NewViewKey(), NewViewKey()
	draw := func() {
		frame(t, h, a, s, 100, 140)
		frame(t, h, b, s, 15000, 15040)
	}
	draw() // warm-up
	alo, ahi := h.WindowRangeFor(a)
	blo, bhi := h.WindowRangeFor(b)
	if alo > 100 || ahi <= 140 || blo > 15000 || bhi <= 15040 {
		t.Fatalf("windows A %d..%d B %d..%d", alo, ahi, blo, bhi)
	}
	aid, bid := slotsID(h, a), slotsID(h, b)
	n0 := h.tokenized.Load()
	for range 10 {
		draw()
		if lo, hi := h.WindowRangeFor(a); lo != alo || hi != ahi {
			t.Fatalf("A's window moved: %d..%d -> %d..%d", alo, ahi, lo, hi)
		}
		if lo, hi := h.WindowRangeFor(b); lo != blo || hi != bhi {
			t.Fatalf("B's window moved: %d..%d -> %d..%d", blo, bhi, lo, hi)
		}
		if slotsID(h, a) != aid || slotsID(h, b) != bid {
			t.Fatal("a window was reallocated")
		}
	}
	if d := h.tokenized.Load() - n0; d != 0 {
		t.Fatalf("visible lines re-tokenized %d times across frames, want 0 (cached)", d)
	}

	// Edits above both windows shift both; the tokens stay right.
	ns, err := s.Apply([]buffer.Edit{{Start: s.LineStart(50), End: s.LineStart(50), Text: "/* x\n*/\n"}})
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, []buffer.Edit{{Start: s.LineStart(50), End: s.LineStart(50), Text: "/* x\n*/\n"}})
	highlightAll(t, h, ns)
	want := fullTokenize(lx, ns)
	for _, c := range []struct {
		v           ViewKey
		first, last int
	}{{a, 102, 142}, {b, 15002, 15042}} {
		for i := c.first; i <= c.last; i++ {
			got, final := h.TokensFor(c.v, ns, i, nil)
			if !final || !slices.Equal(got, want[i]) {
				t.Fatalf("view %d line %d after edit: final=%v\n got  %+v\n want %+v", c.v, i, final, got, want[i])
			}
		}
	}
	if lo, _ := h.WindowRangeFor(b); lo != blo+2 {
		t.Fatalf("B's window not shifted with the text: %d, want %d", lo, blo+2)
	}
}

// Overlapping windows stay consistent: a line re-tokenized while two windows
// hold it updates both.
func TestOverlappingWindowsConsistent(t *testing.T) {
	lx := tsLexer(t)
	h := New(lx)
	h.TokenWindow = 50
	s := buffer.NewSnapshot(tsDoc(t, 500))
	a, b := NewViewKey(), NewViewKey()
	frame(t, h, a, s, 0, 40)
	frame(t, h, b, s, 20, 60)
	e := []buffer.Edit{{Start: s.LineStart(30), End: s.LineStart(30), Text: "`"}}
	ns, _ := s.Apply(e)
	h.Edit(s, ns, e)
	frame(t, h, a, ns, 0, 40)
	want := fullTokenize(lx, ns)
	for _, v := range []ViewKey{a, b} {
		for i := 20; i <= 40; i++ {
			if got, _ := h.TokensFor(v, ns, i, nil); !slices.Equal(got, want[i]) {
				t.Fatalf("view %d line %d\n got  %+v\n want %+v", v, i, got, want[i])
			}
		}
	}
}

// At most maxWindows windows; the least recently used goes first.
func TestWindowsLRU(t *testing.T) {
	h := New(tsLexer(t))
	h.TokenWindow = 10
	s := buffer.NewSnapshot(tsDoc(t, 1000))
	keys := []ViewKey{NewViewKey(), NewViewKey(), NewViewKey(), NewViewKey(), NewViewKey()}
	h.HighlightFor(keys[0], s, 1, time.Hour) // adopt s
	for i, k := range keys[:4] {
		h.TokensFor(k, s, i*100, nil)
	}
	h.TokensFor(keys[0], s, 5, nil) // keys[0] used again: keys[1] is now the oldest
	h.TokensFor(keys[4], s, 900, nil)
	if lo, hi := h.WindowRangeFor(keys[1]); lo != 0 || hi != 0 {
		t.Fatalf("least recently used window kept: %d..%d", lo, hi)
	}
	for _, k := range []ViewKey{keys[0], keys[2], keys[3], keys[4]} {
		if _, hi := h.WindowRangeFor(k); hi == 0 {
			t.Fatalf("window %d evicted", k)
		}
	}
}
