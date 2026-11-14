package diffview

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/editor/diff"
)

// fakeLayout lays out one side the way the editor will: every line takes
// rows(line) rows, followed by the rows of the zones placed after it;
// zones after line -1 come before the first line. Units are rows (rowH 1).
type fakeLayout struct {
	top []float64 // top of each line's first text row
	end float64
}

func layoutSide(lines int, rows func(int) int, pads []pad) fakeLayout {
	after := map[int]int{}
	for _, p := range pads {
		after[p.afterLine] += p.rows
	}
	l := fakeLayout{top: make([]float64, lines)}
	y := float64(after[-1])
	for i := 0; i < lines; i++ {
		l.top[i] = y
		y += float64(rows(i) + after[i])
	}
	l.end = y
	return l
}

func (l fakeLayout) geom() sideGeom {
	return sideGeom{lineTop: func(i int) float64 { return l.top[i] }, rowH: 1, lines: len(l.top), end: l.end}
}

func sideRows(s diff.Side, rows func(diff.Side, int) int) func(int) int {
	if rows == nil {
		return func(int) int { return 1 }
	}
	return func(l int) int { return rows(s, l) }
}

func TestAlignBlocksAndPads(t *testing.T) {
	cases := []struct {
		name     string
		a, b     string
		blocks   []block
		orig     []pad
		modified []pad
	}{
		{
			name: "identical",
			a:    "a\nb", b: "a\nb",
			blocks: []block{{orig: lr(0, 2), mod: lr(0, 2), change: -1}},
		},
		{
			name: "insert at start",
			a:    "a\nb", b: "x\ny\na\nb",
			blocks: []block{
				{orig: lr(0, 0), mod: lr(0, 2), change: 0},
				{orig: lr(0, 2), mod: lr(2, 4), change: -1},
			},
			orig: []pad{{side: diff.Original, afterLine: -1, rows: 2, id: 0, block: 0}},
		},
		{
			name: "delete at end",
			a:    "a\nb\nc\nd", b: "a\nb",
			blocks: []block{
				{orig: lr(0, 2), mod: lr(0, 2), change: -1},
				{orig: lr(2, 4), mod: lr(2, 2), change: 0},
			},
			modified: []pad{{side: diff.Modified, afterLine: 1, rows: 2, id: 1, block: 1}},
		},
		{
			name: "replace 1 by 3 in the middle",
			a:    "a\nb\nc", b: "a\nx\ny\nz\nc",
			blocks: []block{
				{orig: lr(0, 1), mod: lr(0, 1), change: -1},
				{orig: lr(1, 2), mod: lr(1, 4), change: 0},
				{orig: lr(2, 3), mod: lr(4, 5), change: -1},
			},
			orig: []pad{{side: diff.Original, afterLine: 1, rows: 2, id: 0, block: 1}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := diffLines(split(c.a), split(c.b))
			al := align(r, nil, false)
			if !slices.Equal(al.blocks, c.blocks) {
				t.Fatalf("blocks %+v, want %+v", al.blocks, c.blocks)
			}
			if !slices.Equal(al.pads[diff.Original], c.orig) || !slices.Equal(al.pads[diff.Modified], c.modified) {
				t.Fatalf("pads %+v / %+v, want %+v / %+v", al.pads[0], al.pads[1], c.orig, c.modified)
			}
		})
	}
}

// TestAlignMismatchedCounts covers a result whose unchanged tail is not the
// same length on both sides (which a correct diff never produces): the
// leftover lines become a changed block so the alignment stays total.
func TestAlignMismatchedCounts(t *testing.T) {
	r := diff.Result{OriginalLines: 3, ModifiedLines: 5}
	al := align(r, nil, false)
	want := []block{
		{orig: lr(0, 3), mod: lr(0, 3), change: -1},
		{orig: lr(3, 3), mod: lr(3, 5), change: 0},
	}
	if !slices.Equal(al.blocks, want) {
		t.Fatalf("blocks %+v, want %+v", al.blocks, want)
	}
}

// TestAlignRandom checks, for random diffs with and without wrapped lines,
// that padding makes every block start at the same height on both sides,
// that both sides end at the same height, that unchanged line pairs start
// at the same height, and that scroll synchronisation round-trips every
// position.
func TestAlignRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for iter := 0; iter < 400; iter++ {
		a := randLines(r, 1+r.IntN(30))
		b := mutate(r, a)
		res := diffLines(a, b)
		var rows func(diff.Side, int) int
		wrapped := iter%2 == 1
		if wrapped {
			ro := make([]int, len(a))
			rm := make([]int, len(b))
			for i := range ro {
				ro[i] = 1 + r.IntN(3)
			}
			for i := range rm {
				rm[i] = 1 + r.IntN(3)
			}
			rows = func(s diff.Side, l int) int {
				if s == diff.Original {
					return ro[l]
				}
				return rm[l]
			}
		}
		al := align(res, rows, wrapped)
		for s := range al.pads {
			ps := al.pads[s]
			for i := 1; i < len(ps); i++ {
				if ps[i].afterLine <= ps[i-1].afterLine {
					t.Fatalf("iter %d side %d: pads not sorted/unique: %+v", iter, s, ps)
				}
			}
		}
		lo := layoutSide(len(a), sideRows(diff.Original, rows), al.pads[diff.Original])
		lm := layoutSide(len(b), sideRows(diff.Modified, rows), al.pads[diff.Modified])
		if lo.end != lm.end {
			t.Fatalf("iter %d: heights %v / %v (wrapped %v)\n%v\n%v", iter, lo.end, lm.end, wrapped, a, b)
		}
		go_, gm := lo.geom(), lm.geom()
		for bi, blk := range al.blocks {
			if to, tm := al.blockTop(diff.Original, bi, go_), al.blockTop(diff.Modified, bi, gm); to != tm {
				t.Fatalf("iter %d block %d %+v: tops %v / %v", iter, bi, blk, to, tm)
			}
			if blk.change < 0 {
				for k := 0; k < blk.orig.Len(); k++ {
					if lo.top[blk.orig.Start+k] != lm.top[blk.mod.Start+k] {
						t.Fatalf("iter %d: unchanged lines %d/%d at %v / %v", iter, blk.orig.Start+k, blk.mod.Start+k, lo.top[blk.orig.Start+k], lm.top[blk.mod.Start+k])
					}
				}
			}
		}
		// Every row (and a fractional position inside it) maps across and back.
		for y := 0.0; y < lm.end; y += 0.5 {
			yo := al.syncTop(diff.Modified, y, gm, go_)
			if yo != y {
				t.Fatalf("iter %d: modified %v -> original %v", iter, y, yo)
			}
			if back := al.syncTop(diff.Original, yo, go_, gm); back != y {
				t.Fatalf("iter %d: %v -> %v -> %v", iter, y, yo, back)
			}
		}
		// blockAt agrees with a linear scan.
		for s := diff.Original; s <= diff.Modified; s++ {
			n := res.OriginalLines
			if s == diff.Modified {
				n = res.ModifiedLines
			}
			for l := 0; l < n; l++ {
				bi := al.blockAt(s, l)
				if !al.blocks[bi].lines(s).Contains(l) {
					t.Fatalf("iter %d: blockAt(%v, %d) = %d %+v", iter, s, l, bi, al.blocks[bi])
				}
			}
		}
	}
}

// TestSyncUnequalEstimates checks the anchor-based synchronisation when
// the two sides' heights above the viewport disagree (lazily estimated
// wrapping): positions inside an unchanged block keep their offset from
// the block top, positions past the shorter changed block clamp to its end.
func TestSyncUnequalEstimates(t *testing.T) {
	res := diffLines(split("a\nb\nc\nd"), split("a\nX\nY\nc\nd"))
	al := align(res, nil, false)
	// Original: line 0 at 0, the change [1,2) at 1 (estimated 4 rows tall,
	// with its padding), unchanged [2,4) at 5.
	lo := fakeLayout{top: []float64{0, 1, 5, 6}, end: 7}
	lm := layoutSide(5, func(int) int { return 1 }, al.pads[diff.Modified])
	if got := al.syncTop(diff.Modified, lm.top[3]+0.25, lm.geom(), lo.geom()); got != 5.25 {
		t.Fatalf("unchanged line c: synced to %v, want 5.25", got)
	}
	if got := al.syncTop(diff.Original, 4.5, lo.geom(), lm.geom()); got != lm.top[3] {
		t.Fatalf("past the modified change: synced to %v, want %v", got, lm.top[3])
	}
}
