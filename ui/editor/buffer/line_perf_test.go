package buffer

import (
	"fmt"
	"strings"
	"testing"
)

var queriedLine string

// BenchmarkShortLineRead isolates the repeated line reads used by editing,
// indentation and tokenization. It excludes loading and snapshot edits.
func BenchmarkShortLineRead(b *testing.B) {
	for _, width := range []int{10, 100, 3000} {
		b.Run(fmt.Sprintf("width%d", width), func(b *testing.B) {
			s := NewSnapshot(strings.Repeat(strings.Repeat("x", width-1)+"\n", 10000))
			// Keep the previous three-pass lookup in the same binary, so
			// scheduler/load changes do not masquerade as algorithm gains.
			for _, arm := range []struct {
				name string
				read func(Snapshot, int) string
			}{
				{"previous", func(s Snapshot, i int) string { return s.slice(s.LineStart(i), s.lineEnd(i)) }},
				{"combined", Snapshot.Line},
			} {
				b.Run(arm.name, func(b *testing.B) {
					b.ReportAllocs()
					i := 0
					for b.Loop() {
						i = (i + 7919) % 10000
						queriedLine = arm.read(s, i)
					}
				})
			}
		})
	}
}

// TestLineAtLeafBoundaries pins the combined lookup's byte/rune/line
// semantics. In particular a line may begin at the end of a leaf, span
// several leaves, or be the final empty line. Non-UTF-8 bytes are data.
func TestLineAtLeafBoundaries(t *testing.T) {
	for _, text := range []string{
		"a\n\n世😀\nlast", "last", "\n", "\xff\n\xfe\x00\n",
		strings.Repeat("\n", leafMax*2),
		strings.Repeat("a", leafTarget-1) + "\n" + strings.Repeat("世😀", leafMax) + "\nlast\n",
		strings.Repeat("x\n", leafMax) + strings.Repeat("z", leafMax*3),
	} {
		s := NewSnapshot(text)
		want := strings.Split(s.String(), "\n")
		for i, w := range want {
			if got := lineAt(s.root, i); got != w {
				t.Fatalf("len=%d line=%d: got %q, want %q", len(text), i, got, w)
			}
		}
		// Edits change leaf boundaries without changing the old snapshot.
		next := mustApply(t, s, Edit{Start: 0, Text: "prefix\n"})
		for _, snap := range []Snapshot{s, next} {
			for i, w := range strings.Split(snap.String(), "\n") {
				if got := lineAt(snap.root, i); got != w {
					t.Fatalf("after edit line=%d: got %q, want %q", i, got, w)
				}
			}
		}
	}
}

// Editing can split, merge and rebalance leaves. Query both old and new
// snapshots against an independent string reference after every edit.
func TestLineReadAfterRebalance(t *testing.T) {
	s := NewSnapshot(strings.Repeat("短\n\n"+strings.Repeat("x", 3000)+"\n", 30))
	check := func(s Snapshot) {
		t.Helper()
		for i, want := range strings.Split(s.String(), "\n") {
			if got := s.Line(i); got != want {
				t.Fatalf("version %d line %d: got %q, want %q", s.Version(), i, got, want)
			}
		}
	}
	for k := range 100 {
		old := s
		// ASCII offsets are selected within a known line to avoid splitting
		// UTF-8; varying deletes cover lines and several leaf boundaries.
		line := (k * 37) % s.LineCount()
		a := s.LineStart(line)
		endLine := min(line+k%7, s.LineCount()-1)
		b := s.LineStart(endLine)
		s = mustApply(t, s, Edit{Start: a, End: b, Text: strings.Repeat("世😀\n", k%5) + "\xfftail\n"})
		check(old)
		check(s)
	}
	check(Snapshot{})
}
