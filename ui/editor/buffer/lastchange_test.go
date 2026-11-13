package buffer

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// checkLastChange verifies that LastChange describes the transition from
// prev to the buffer's current snapshot: old is prev, and applying the edits
// to it yields the current text.
func checkLastChange(t *testing.T, b *Buffer, prev Snapshot, ctx string) []Edit {
	t.Helper()
	old, edits := b.LastChange()
	if old.Version() != prev.Version() || old.String() != prev.String() {
		t.Fatalf("%s: LastChange old = v%d %q, want v%d %q", ctx, old.Version(), old.String(), prev.Version(), prev.String())
	}
	got, err := old.Apply(edits)
	if err != nil {
		t.Fatalf("%s: applying LastChange edits %+v: %v", ctx, edits, err)
	}
	if want := b.Snapshot().String(); got.String() != want {
		t.Fatalf("%s: LastChange edits %+v give %q, want %q", ctx, edits, got.String(), want)
	}
	return edits
}

func TestLastChange(t *testing.T) {
	b := New("hello world")
	if old, edits := b.LastChange(); old.Version() != b.Snapshot().Version() || edits != nil {
		t.Fatalf("fresh buffer: LastChange = v%d %+v", old.Version(), edits)
	}
	// One explicit step with two edits far apart (and an EOL-normalised insert).
	b.PushUndoStop()
	prev := b.Snapshot()
	mustEdit(t, b, []Edit{{Start: 0, End: 5, Text: "HELLO"}}, caret(0), caret(5))
	checkLastChange(t, b, prev, "edit 1")
	prev = b.Snapshot()
	mustEdit(t, b, []Edit{{Start: 6, End: 6, Text: "big\r\n"}, {Start: 11, End: 11, Text: "!"}}, caret(5), caret(5))
	checkLastChange(t, b, prev, "edit 2")
	b.PushUndoStop()

	prev = b.Snapshot()
	b.Undo()
	edits := checkLastChange(t, b, prev, "undo")
	// The composed undo keeps the two far-apart regions separate.
	if len(edits) < 2 {
		t.Fatalf("undo edits %+v: want the separate regions, not one covering span", edits)
	}
	prev = b.Snapshot()
	b.Redo()
	checkLastChange(t, b, prev, "redo")

	// Undo/Redo with nothing to do leave LastChange describing the last change.
	prev = b.Snapshot()
	b.Redo()
	if old, _ := b.LastChange(); old.Version() == prev.Version() {
		t.Fatal("failed Redo replaced LastChange")
	}
}

// Random histories: every Edit, Undo and Redo is described by LastChange.
func TestLastChangeRandom(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	words := []string{"", "a", "xyz", "\n", "é", "日本", "\r\n", "line\nbreak"}
	for round := range 50 {
		b, clk := newClockBuffer(strings.Repeat("abc\n", 5))
		for op := range 60 {
			prev := b.Snapshot()
			ctx := ""
			switch k := rng.IntN(10); {
			case k < 6:
				if rng.IntN(4) == 0 {
					b.PushUndoStop()
				}
				n := b.Snapshot().Len()
				var edits []Edit
				pos := 0
				for range 1 + rng.IntN(3) {
					if pos > n {
						break
					}
					s := pos + rng.IntN(n-pos+1)
					e := s + rng.IntN(min(3, n-s)+1)
					edits = append(edits, Edit{Start: s, End: e, Text: words[rng.IntN(len(words))]})
					pos = e
				}
				if _, err := b.Edit(edits, caret(0), caret(0)); err != nil {
					continue // split rune: buffer unchanged
				}
				ctx = "edit"
			case k < 8:
				if _, _, ok := b.Undo(); !ok {
					continue
				}
				ctx = "undo"
			default:
				if _, _, ok := b.Redo(); !ok {
					continue
				}
				ctx = "redo"
			}
			clk.advance(10 * 1e6)
			if b.Snapshot().Version() == prev.Version() {
				continue
			}
			checkLastChange(t, b, prev, ctx+" round "+string(rune('0'+round%10))+" op "+string(rune('0'+op%10)))
		}
	}
}
