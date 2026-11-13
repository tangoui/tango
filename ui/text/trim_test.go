package text

import "testing"

// TrimCaches empties the Shape cache without changing what Shape returns,
// and leaves LineCache entries alone.
func TestTrimCaches(t *testing.T) {
	st := Style{Size: 14}
	a := Shape("trim caches 文本", st, Options{MaxWidth: 60})
	lc := NewLineCache(16)
	runs := []Run{{Start: 0, End: 8, Style: st}}
	la := lc.Shape("line one", runs, LineOptions{})
	TrimCaches()
	mu.Lock()
	n := len(cache) + len(runsCache)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("%d cached layouts after TrimCaches", n)
	}
	b := Shape("trim caches 文本", st, Options{MaxWidth: 60})
	if a == b {
		t.Fatal("Shape returned the trimmed layout")
	}
	if a.Width != b.Width || a.Height != b.Height || len(a.Lines) != len(b.Lines) || len(a.Glyphs) != len(b.Glyphs) {
		t.Fatalf("layout changed: %v×%v %d lines %d glyphs, then %v×%v %d lines %d glyphs",
			a.Width, a.Height, len(a.Lines), len(a.Glyphs), b.Width, b.Height, len(b.Lines), len(b.Glyphs))
	}
	if lb := lc.Shape("line one", runs, LineOptions{}); lb != la {
		t.Fatal("TrimCaches cleared a LineCache")
	}
}
