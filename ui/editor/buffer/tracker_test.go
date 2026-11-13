package buffer

import (
	"math/rand"
	"testing"
)

// TestTrackerMatchesTransform checks Tracker.Map against the reference
// batch.transform (all spans, last to first) on random edit sets,
// including adjacent edits and inserts at the same offset.
func TestTrackerMatchesTransform(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	texts := []string{"", "x", "xy", "xyz\n"}
	for iter := 0; iter < 3000; iter++ {
		n := 30
		var edits []Edit
		pos := 0
		for pos <= n && len(edits) < 8 {
			pos += r.Intn(4)
			if pos > n {
				break
			}
			end := min(n, pos+r.Intn(4))
			edits = append(edits, Edit{Start: pos, End: end, Text: texts[r.Intn(len(texts))]})
			pos = end
		}
		tr := NewTracker(edits)
		bt := newBatch(edits)
		for off := 0; off <= n; off++ {
			for _, b := range []Bias{Left, Right} {
				if got, want := tr.Map(off, b), bt.transform(off, b); got != want {
					t.Fatalf("edits %+v off %d %v: Map %d, transform %d", edits, off, b, got, want)
				}
			}
		}
	}
}
