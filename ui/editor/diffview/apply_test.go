package diffview

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// randEdits returns a few sorted, non-overlapping random edits of s.
func randEdits(r *rand.Rand, s buffer.Snapshot) []buffer.Edit {
	pieces := []string{"", "q", "\n", "w\n", "\r\nk", "a\nb\n", "  "}
	var out []buffer.Edit
	pos := 0
	for k := 1 + r.IntN(3); k > 0 && pos <= s.Len(); k-- {
		start := pos + r.IntN(s.Len()-pos+1)
		end := min(s.Len(), start+r.IntN(6))
		out = append(out, buffer.Edit{Start: start, End: end, Text: pieces[r.IntN(len(pieces))]})
		pos = end + 1
	}
	return out
}

// checkMapped verifies that r is a valid diff of orig and mod: the
// structure is sound and every unchanged line pair really is equal.
func checkMapped(t *testing.T, r diff.Result, orig, mod []string) {
	t.Helper()
	if r.OriginalLines != len(orig) || r.ModifiedLines != len(mod) {
		t.Fatalf("line counts %d/%d, want %d/%d", r.OriginalLines, r.ModifiedLines, len(orig), len(mod))
	}
	checkStructure(t, r)
	o, m := 0, 0
	pairs := func(n int) {
		for k := 0; k < n; k++ {
			if orig[o+k] != mod[m+k] {
				t.Fatalf("unchanged pair %d/%d differs: %q vs %q", o+k, m+k, orig[o+k], mod[m+k])
			}
		}
	}
	for _, c := range r.Changes {
		pairs(c.Original.Start - o)
		o, m = c.Original.End, c.Modified.End
	}
	pairs(len(orig) - o)
}

func TestApplyEditsRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for iter := 0; iter < 3000; iter++ {
		a := randLines(r, 1+r.IntN(25))
		b := mutate(r, a)
		res := diffLines(a, b)
		side := diff.Side(iter % 2)
		texts := [2][]string{a, b}
		old := buffer.NewSnapshot(strings.Join(texts[side], "\n"))
		edits := randEdits(r, old)
		ns, err := old.Apply(edits)
		if err != nil {
			t.Fatal(err)
		}
		got := applyEdits(res, side, old, edits)
		texts[side] = split(ns.String())
		checkMapped(t, got, texts[0], texts[1])
		// Blocks only grow: every old change's range on the other side
		// (whose text did not change) lies inside a new change.
		for _, c := range res.Changes {
			oc := c.Original
			if side == diff.Original {
				oc = c.Modified
			}
			if !slices.ContainsFunc(got.Changes, func(n diff.Change) bool {
				nr := n.Original
				if side == diff.Original {
					nr = n.Modified
				}
				return nr.Start <= oc.Start && oc.End <= nr.End
			}) {
				t.Fatalf("iter %d: old change %v lost in %v", iter, c, got.Changes)
			}
		}
		// Applying the edits to a fresh diff of the edited text changes nothing
		// it should not: an exact re-diff is also valid, so compare sizes only
		// loosely (the mapped result never claims fewer changed lines on the
		// unedited side than are really different).
		fresh := diffLines(texts[0], texts[1])
		if len(fresh.Changes) > 0 && len(got.Changes) == 0 {
			t.Fatalf("iter %d: mapped result has no changes, fresh diff has %v", iter, fresh.Changes)
		}
	}
}

func TestApplyEditsCases(t *testing.T) {
	a := split("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7")
	b := split("l0\nl1\nX\nl3\nl4\nl5\nl6\nl7")
	res := diffLines(a, b)
	if len(res.Changes) != 1 {
		t.Fatalf("setup: %v", res.Changes)
	}
	mod := buffer.NewSnapshot(strings.Join(b, "\n"))
	cases := []struct {
		name  string
		edits []buffer.Edit
		want  []diff.Change // Inner ignored
		inner []bool        // whether each change keeps its inner ranges
	}{
		{"no edits", nil, []diff.Change{{Original: lr(2, 3), Modified: lr(2, 3)}}, []bool{true}},
		{"insert line far below shifts nothing above",
			[]buffer.Edit{{Start: mod.LineStart(6), End: mod.LineStart(6), Text: "n\n"}},
			[]diff.Change{
				{Original: lr(2, 3), Modified: lr(2, 3)},
				{Original: lr(6, 7), Modified: lr(6, 8)},
			}, []bool{true, false}},
		{"insert line above shifts the change",
			[]buffer.Edit{{Start: 0, End: 0, Text: "n\n"}},
			[]diff.Change{
				{Original: lr(0, 1), Modified: lr(0, 2)},
				{Original: lr(2, 3), Modified: lr(3, 4)},
			}, []bool{false, true}},
		{"typing next to the change merges into it",
			[]buffer.Edit{{Start: mod.LineStart(3), End: mod.LineStart(3), Text: "y"}},
			[]diff.Change{{Original: lr(2, 4), Modified: lr(2, 4)}}, []bool{false}},
		{"typing inside the change",
			[]buffer.Edit{{Start: mod.LineStart(2) + 1, End: mod.LineStart(2) + 1, Text: "y\nz"}},
			[]diff.Change{{Original: lr(2, 3), Modified: lr(2, 4)}}, []bool{false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyEdits(res, diff.Modified, mod, c.edits)
			if len(got.Changes) != len(c.want) {
				t.Fatalf("changes %v, want %v", got.Changes, c.want)
			}
			for i, ch := range got.Changes {
				if ch.Original != c.want[i].Original || ch.Modified != c.want[i].Modified {
					t.Fatalf("changes %v, want %v", got.Changes, c.want)
				}
				if (len(ch.Inner) > 0) != c.inner[i] {
					t.Fatalf("change %d inner %v, want kept=%v", i, ch.Inner, c.inner[i])
				}
			}
		})
	}
	// Inner ranges of a change below an edit move with it.
	got := applyEdits(res, diff.Modified, mod, []buffer.Edit{{Start: 0, End: 0, Text: "n\n"}})
	in := got.Changes[1].Inner[0]
	if want := res.Changes[0].Inner[0]; in.Original != want.Original || in.Modified.Start.Line != want.Modified.Start.Line+1 || in.Modified.Start.Col != want.Modified.Start.Col {
		t.Fatalf("inner %v, want %v shifted one line on the modified side", in, want)
	}
}
