package diffview

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
)

func TestRevertUndo(t *testing.T) {
	useTestText(t)
	orig := docLines(120)
	mod := modify(orig, 20)
	d := newTestDiff(t, orig, mod)
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newHost(t, d, geom.Size{W: 1200, H: 500}, 1, false)
	h.settle()
	r, _ := d.Result()
	n := len(r.Changes)
	before := d.Modified.Snapshot().String()
	if err := d.Revert(1); err != nil {
		t.Fatal(err)
	}
	h.settle()
	r2, ok := d.Result()
	if !ok || len(r2.Changes) != n-1 {
		t.Fatalf("after revert: %d changes (up to date %v), had %d", len(r2.Changes), ok, n)
	}
	// One undo step brings the change back.
	if !d.Modified.Undo() {
		t.Fatal("undo")
	}
	if got := d.Modified.Snapshot().String(); got != before {
		t.Fatal("undo did not restore the text")
	}
	h.settle()
	if r3, _ := d.Result(); len(r3.Changes) != n {
		t.Fatalf("after undo: %d changes", len(r3.Changes))
	}
	// The arrow at the top of a change reverts it.
	c := r.Changes[0]
	col := h.d.Modified
	er := h.editorRect(diff.Modified)
	y := er.Y + float32(d.changeTop(0)-col.ScrollTop()) + col.Viewport.LineHeight/2
	h.click(geom.Pt(er.X-revertW/2, y))
	h.frame()
	h.settle()
	if r4, _ := d.Result(); len(r4.Changes) != n-1 {
		t.Fatalf("after clicking the arrow of %v: %d changes", c, len(r4.Changes))
	}
	if !strings.Contains(d.Modified.Snapshot().String(), orig[c.Original.Start]) {
		t.Fatal("original line not restored")
	}
	// A read-only modified side cannot be reverted and has no arrows.
	d.Modified.Options.ReadOnly = true
	h.frame()
	if err := d.Revert(0); err == nil {
		t.Fatal("reverted a read-only side")
	}
	if h.vw.revertID != 0 {
		t.Fatal("revert arrows on a read-only side")
	}
}

func TestHunkActions(t *testing.T) {
	useTestText(t)
	orig := docLines(120)
	d := newTestDiff(t, orig, modify(orig, 20))
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newHost(t, d, geom.Size{W: 1200, H: 600}, 1, true)
	var ran []int
	h.mods = func(w *ViewW) {
		w.HunkActions(func(i int, ch diff.Change) []HunkAction {
			return []HunkAction{
				{Label: "接受", Run: func() { ran = append(ran, i) }},
				{Label: "拒绝", Run: func() { _ = d.Revert(i) }},
			}
		})
	}
	h.settle()
	r, _ := d.Result()
	var bars [2]int
	for s, st := range d.sides() {
		for _, z := range st.ViewZones(owner) {
			if z.ID&idBar != 0 && z.ID>>63 == 0 {
				bars[s]++
			}
		}
	}
	if bars[0] != len(r.Changes) || bars[1] != len(r.Changes) {
		t.Fatalf("bars %v for %d changes", bars, len(r.Changes))
	}
	if h.vw.revertID != 0 {
		t.Fatal("revert arrows with hunk actions")
	}
	checkAligned(t, d, "bars")
	h.shot("hunk-actions")
	// Click "接受" of the first change.
	b := d.lay.bars[idBar|0<<1|uint64(diff.Modified)]
	st := d.Modified
	er := h.editorRect(diff.Modified)
	y := er.Y + float32(st.LineTop(r.Changes[0].Modified.Start)-st.ScrollTop()) - st.Viewport.LineHeight/2
	h.click(geom.Pt(b.x+(b.btn[0][0]+b.btn[0][1])/2, y))
	h.frame()
	if len(ran) != 1 || ran[0] != 0 {
		t.Fatalf("ran %v", ran)
	}
	// "拒绝" reverts.
	h.frame()
	h.click(geom.Pt(b.x+(b.btn[1][0]+b.btn[1][1])/2, y))
	h.frame()
	h.settle()
	if r2, _ := d.Result(); len(r2.Changes) != len(r.Changes)-1 {
		t.Fatalf("%d changes after rejecting one of %d", len(r2.Changes), len(r.Changes))
	}
	checkAligned(t, d, "after reject")
}
