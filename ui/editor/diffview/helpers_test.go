package diffview

import (
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/diff"
)

// randLines returns n lines drawn from a small alphabet, so that random
// texts share many lines and diffs have unchanged runs.
func randLines(r *rand.Rand, n int) []string {
	words := []string{"a", "b", "c", "d", "e", "f", "  a", "x y", ""}
	out := make([]string, n)
	for i := range out {
		out[i] = words[r.IntN(len(words))]
	}
	return out
}

// mutate returns a copy of lines with a few random line insertions,
// deletions and replacements.
func mutate(r *rand.Rand, lines []string) []string {
	out := slices.Clone(lines)
	for k := r.IntN(5); k >= 0; k-- {
		i := r.IntN(len(out) + 1)
		switch r.IntN(3) {
		case 0:
			out = slices.Insert(out, i, randLines(r, 1+r.IntN(4))...)
		case 1:
			if i < len(out) {
				out = slices.Delete(out, i, min(len(out), i+1+r.IntN(3)))
			}
		default:
			if i < len(out) {
				out[i] += "z"
			}
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func diffLines(a, b []string) diff.Result {
	return diff.Lines(context.Background(), a, b, diff.Options{})
}

func split(s string) []string { return strings.Split(s, "\n") }

// checkStructure verifies the invariants of a diff.Result that do not
// depend on the texts: changes are sorted, disjoint on both sides and
// separated by unchanged runs of equal length on both sides.
func checkStructure(t *testing.T, r diff.Result) {
	t.Helper()
	o, m := 0, 0
	for i, c := range r.Changes {
		if c.Original.Start < o || c.Modified.Start < m || c.Original.End < c.Original.Start || c.Modified.End < c.Modified.Start {
			t.Fatalf("change %d %v out of order (after %d/%d)", i, c, o, m)
		}
		if c.Original.Start-o != c.Modified.Start-m {
			t.Fatalf("change %d %v: unequal unchanged gap before it", i, c)
		}
		if i > 0 && c.Original.Start == o {
			t.Fatalf("change %d %v touches the previous change", i, c)
		}
		if c.Original.Empty() && c.Modified.Empty() {
			t.Fatalf("change %d is empty on both sides", i)
		}
		o, m = c.Original.End, c.Modified.End
	}
	if r.OriginalLines-o != r.ModifiedLines-m || o > r.OriginalLines {
		t.Fatalf("unequal unchanged tail: %d/%d of %d/%d", o, m, r.OriginalLines, r.ModifiedLines)
	}
}
