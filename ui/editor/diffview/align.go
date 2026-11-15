package diffview

import (
	"sort"

	"github.com/tangoui/tango/ui/editor/diff"
)

// block is one aligned block of a diff: an unchanged run, whose lines
// correspond one to one, or a changed block. The blocks of an alignment
// cover both texts in order.
type block struct {
	orig, mod diff.LineRange
	// change is the index into Result.Changes of a changed block, -1 for
	// an unchanged run. A block past the last change that only exists
	// because the result's line counts disagree gets len(Changes).
	change int
}

func (b block) lines(s diff.Side) diff.LineRange {
	if s == diff.Original {
		return b.orig
	}
	return b.mod
}

// pad is a blank view zone that keeps the two sides aligned: rows rows
// after model line afterLine (-1: before the first line) of side.
type pad struct {
	side      diff.Side
	afterLine int
	rows      int
	// id identifies the zone across recomputations: change*2+side for the
	// padding of a changed block (only rows change when a wrapped line is
	// measured), 1<<63 | line<<1 | side for the padding of an unchanged
	// wrapped line. A changed block's padding that falls after the same
	// line as a wrapped line's padding absorbs it and keeps the block id.
	id    uint64
	block int // index into alignment.blocks
}

// alignment is the block structure of a diff and the padding zones that
// give both sides the same height block by block (Monaco's
// diffEditorViewZones, without its pixel heights: zones are whole rows,
// which is exact because both editors of a diff view share a line height).
type alignment struct {
	blocks []block
	pads   [2][]pad // per side, sorted by afterLine, at most one per line
	// changePad[s][b] is the number of padding rows block b has on side s
	// (0 for unchanged runs).
	changePad [2][]int
}

// align computes the alignment of r. rows reports how many text rows a
// model line of a side takes (nil: one each, no wrapping); for lines far
// from the viewport it may return an estimate. With wrapped set, unchanged
// line pairs whose row counts differ are padded too; otherwise only
// changed blocks are, and rows is only called for their lines.
func align(r diff.Result, rows func(diff.Side, int) int, wrapped bool) *alignment {
	if rows == nil {
		rows = func(diff.Side, int) int { return 1 }
	}
	al := &alignment{}
	o, m := 0, 0
	addUnchanged := func(n int) {
		if n > 0 {
			al.blocks = append(al.blocks, block{orig: diff.LineRange{Start: o, End: o + n}, mod: diff.LineRange{Start: m, End: m + n}, change: -1})
			o, m = o+n, m+n
		}
	}
	for i, c := range r.Changes {
		addUnchanged(c.Modified.Start - m)
		al.blocks = append(al.blocks, block{orig: c.Original, mod: c.Modified, change: i})
		o, m = c.Original.End, c.Modified.End
	}
	addUnchanged(min(r.OriginalLines-o, r.ModifiedLines-m))
	if o < r.OriginalLines || m < r.ModifiedLines {
		al.blocks = append(al.blocks, block{
			orig:   diff.LineRange{Start: o, End: max(o, r.OriginalLines)},
			mod:    diff.LineRange{Start: m, End: max(m, r.ModifiedLines)},
			change: len(r.Changes),
		})
	}

	al.changePad[0] = make([]int, len(al.blocks))
	al.changePad[1] = make([]int, len(al.blocks))
	for bi, b := range al.blocks {
		if b.change < 0 {
			if !wrapped {
				continue
			}
			for k := 0; k < b.orig.Len(); k++ {
				lo, lm := b.orig.Start+k, b.mod.Start+k
				ro, rm := rows(diff.Original, lo), rows(diff.Modified, lm)
				switch {
				case ro < rm:
					al.add(pad{side: diff.Original, afterLine: lo, rows: rm - ro, id: 1<<63 | uint64(lo)<<1, block: bi})
				case rm < ro:
					al.add(pad{side: diff.Modified, afterLine: lm, rows: ro - rm, id: 1<<63 | uint64(lm)<<1 | 1, block: bi})
				}
			}
			continue
		}
		h := [2]int{}
		for s := diff.Original; s <= diff.Modified; s++ {
			lr := b.lines(s)
			for l := lr.Start; l < lr.End; l++ {
				h[s] += rows(s, l)
			}
		}
		short := diff.Original
		if h[diff.Modified] < h[diff.Original] {
			short = diff.Modified
		}
		n := h[1-short] - h[short]
		if n == 0 {
			continue
		}
		lr := b.lines(short)
		after := lr.End - 1 // also lr.Start-1 for an empty range
		al.changePad[short][bi] = n
		al.add(pad{side: short, afterLine: after, rows: n, id: uint64(b.change)<<1 | uint64(short), block: bi})
	}
	return al
}

// add appends p to its side, merging it into a pad after the same line
// (the padding of a changed block whose range is empty on this side lands
// after the last line of the preceding unchanged run, which may itself be
// padded for wrapping). The changed block's identity wins.
func (al *alignment) add(p pad) {
	ps := al.pads[p.side]
	if k := len(ps); k > 0 && ps[k-1].afterLine == p.afterLine {
		ps[k-1].rows += p.rows
		ps[k-1].id, ps[k-1].block = p.id, p.block
		return
	}
	al.pads[p.side] = append(ps, p)
}

// blockAt returns the index of the block containing line of side s (a
// line past the end belongs to the last block).
func (al *alignment) blockAt(s diff.Side, line int) int {
	i := sort.Search(len(al.blocks), func(i int) bool { return al.blocks[i].lines(s).End > line })
	// Blocks with an empty range on s end where the next one starts; the
	// search above skips them, which is what a line lookup wants.
	return min(i, len(al.blocks)-1)
}

// sideGeom is the vertical geometry of one side's editor, in any unit
// (pixels in the view, rows in tests).
type sideGeom struct {
	// lineTop returns the top of the first text row of a model line
	// (editor.State.LineTop).
	lineTop func(line int) float64
	rowH    float64 // height of one row
	lines   int     // model line count
	end     float64 // bottom of the content: below the last line and its zones
}

// blockTop returns where block bi starts on side s. A block whose range is
// empty on s is its padding zone, which sits right above the next line (or
// at the end of the content).
func (al *alignment) blockTop(s diff.Side, bi int, g sideGeom) float64 {
	if bi >= len(al.blocks) {
		return g.end
	}
	lr := al.blocks[bi].lines(s)
	if !lr.Empty() {
		return g.lineTop(lr.Start)
	}
	below := g.end
	if lr.Start < g.lines {
		below = g.lineTop(lr.Start)
	}
	return below - float64(al.changePad[s][bi])*g.rowH
}

// syncTop returns the scroll position of the follower side for the leader
// scrolled to top (both in the units of the geometries). Instead of making
// the two positions equal, as Monaco does, it anchors on the aligned block
// at the top of the leader: the follower shows the same block, at the same
// offset from its top, clamped to the follower's block height. With exact
// row counts on both sides the two agree with Monaco; when far-away
// wrapping is only estimated the viewport still lines up.
func (al *alignment) syncTop(lead diff.Side, top float64, gl, gf sideGeom) float64 {
	n := len(al.blocks)
	if n == 0 {
		return top
	}
	// The last block whose top is at or above top.
	bi := sort.Search(n, func(i int) bool { return al.blockTop(lead, i, gl) > top }) - 1
	if bi < 0 {
		// Above the first block (top padding of the editor): keep the offset.
		return al.blockTop(1-lead, 0, gf) - (al.blockTop(lead, 0, gl) - top)
	}
	off := top - al.blockTop(lead, bi, gl)
	ft := al.blockTop(1-lead, bi, gf)
	fh := al.blockTop(1-lead, bi+1, gf) - ft
	if bi == n-1 {
		return ft + off // below the last block start: content end padding scrolls along
	}
	return ft + min(off, fh)
}
