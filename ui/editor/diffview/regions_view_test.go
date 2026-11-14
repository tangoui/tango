package diffview

import (
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
)

// placeholderOf returns the placeholder of region ri on side s.
func placeholderOf(t *testing.T, d *State, ri int, s diff.Side) *placeholder {
	t.Helper()
	for _, p := range d.lay.places {
		if p.region == ri && p.side == s {
			return p
		}
	}
	t.Fatalf("no placeholder for region %d on side %v", ri, s)
	return nil
}

// clickPlaceholder clicks button b of region ri's placeholder on side s
// (shown, its line on screen).
func (h *host) clickPlaceholder(ri int, s diff.Side, b int) {
	h.t.Helper()
	d := h.d
	p := placeholderOf(h.t.(*testing.T), d, ri, s)
	st := d.side(s)
	after := d.regions.regions[ri].hidden(s).Start - 1
	st.RevealLine(max(after, 0), editor.RevealCenterIfOutside)
	h.frame()
	h.frame()
	var y float64
	if after < 0 {
		y = st.LineTop(0) - float64(st.Viewport.LineHeight)
	} else {
		top, rows, _ := st.LineBlock(after)
		y = top + float64(rows)*float64(st.Viewport.LineHeight)
	}
	r := h.editorRect(s)
	wy := r.Y + float32(y-st.ScrollTop()) + st.Viewport.LineHeight/2
	wx := p.x + (p.btn[b][0]+p.btn[b][1])/2
	h.click(geom.Pt(wx, wy))
	h.frame()
	h.frame()
}

func hiddenOf(st *editor.State) []editor.LineRange { return st.HiddenAreas(owner) }

func TestUnchangedRegionsView(t *testing.T) {
	useTestText(t)
	orig := docLines(400)
	d := newTestDiff(t, orig, modify(orig, 80))
	d.Options.HideUnchangedRegions.Enabled = true
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newHost(t, d, geom.Size{W: 1200, H: 600}, 1, false)
	h.settle()
	rs := d.regions.regions
	if len(rs) < 3 {
		t.Fatalf("%d regions", len(rs))
	}
	ho, hm := hiddenOf(d.Original), hiddenOf(d.Modified)
	if len(ho) != len(rs) || len(hm) != len(rs) {
		t.Fatalf("hidden areas %v / %v for %d regions", ho, hm, len(rs))
	}
	for i := range ho {
		if ho[i].End-ho[i].Start != hm[i].End-hm[i].Start {
			t.Fatalf("hidden %v / %v", ho[i], hm[i])
		}
	}
	if d.Modified.Options.StickyScroll.Enabled {
		t.Fatal("sticky scroll on while folding unchanged regions")
	}
	checkAligned(t, d, "folded")
	h.shot("regions-folded")

	// The second region's placeholder is on screen: show 20 more lines
	// above it.
	ri := 1
	d.Modified.RevealLine(d.regions.regions[ri].hidden(diff.Modified).Start, editor.RevealCenter)
	h.frame()
	h.frame()
	before := d.regions.regions[ri].hidden(diff.Modified)
	h.clickPlaceholder(ri, diff.Modified, btnAbove)
	after := d.regions.regions[ri].hidden(diff.Modified)
	if after.Start != before.Start+20 || after.End != before.End {
		t.Fatalf("show above: %v -> %v", before, after)
	}
	if got := hiddenOf(d.Original)[ri]; got.End-got.Start != after.Len() {
		t.Fatalf("original hidden %v, modified %v", got, after)
	}
	checkAligned(t, d, "show above")
	h.shot("regions-above")
	// Then all of it (from the original side's placeholder).
	h.clickPlaceholder(ri, diff.Original, btnAll)
	if g := d.regions.regions[ri]; !g.hidden(diff.Modified).Empty() {
		t.Fatalf("show all: %+v", g)
	}
	if n := len(hiddenOf(d.Modified)); n != len(rs)-1 {
		t.Fatalf("%d hidden areas after showing one of %d", n, len(rs))
	}
	checkAligned(t, d, "show all")

	// A selection that goes into a folded region (a search match, go to
	// line) reveals its line.
	last := len(d.regions.regions) - 1
	hl := d.regions.regions[last].hidden(diff.Modified)
	line := (hl.Start + hl.End) / 2
	snap := d.Modified.Snapshot()
	d.Modified.SetSelections([]editor.Selection{{Anchor: snap.LineStart(line), Head: snap.LineStart(line)}})
	h.frame()
	h.frame()
	if _, ok := d.Modified.HiddenAreaAt(line); ok {
		t.Fatalf("line %d still hidden", line)
	}
	if g := d.regions.regions[last]; g.hidden(diff.Modified).Contains(line) {
		t.Fatalf("region %+v hides %d", g, line)
	}
	if _, ok := d.Original.HiddenAreaAt(line - d.regions.regions[last].mod + d.regions.regions[last].orig); ok {
		t.Fatal("original line still hidden")
	}
	checkAligned(t, d, "selection")

	// Commands.
	d.ShowAllUnchanged()
	h.frame()
	if n := len(hiddenOf(d.Modified)) + len(hiddenOf(d.Original)); n != 0 {
		t.Fatalf("%d hidden areas after ShowAllUnchanged", n)
	}
	d.CollapseAllUnchanged()
	h.frame()
	if n := len(hiddenOf(d.Modified)); n != len(d.regions.regions) {
		t.Fatalf("%d hidden areas after CollapseAllUnchanged", n)
	}
	// Turning the option off shows everything.
	d.Options.HideUnchangedRegions.Enabled = false
	h.frame()
	if n := len(hiddenOf(d.Modified)); n != 0 {
		t.Fatalf("%d hidden areas with the option off", n)
	}
	checkAligned(t, d, "off")
}

// TestUnchangedRegionsSurviveEdits: typing keeps what the user revealed
// and what stays folded (the state migrates to the new result).
func TestUnchangedRegionsSurviveEdits(t *testing.T) {
	useTestText(t)
	orig := docLines(300)
	d := newTestDiff(t, orig, modify(orig, 70))
	d.Options.HideUnchangedRegions.Enabled = true
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newHost(t, d, geom.Size{W: 1200, H: 600}, 1, false)
	h.settle()
	n := len(d.regions.regions)
	d.placeholderAction(0, btnAll)
	h.frame()
	// Type on a visible line of the second change.
	c := d.res.Changes[1]
	snap := d.Modified.Snapshot()
	off := snap.LineStart(c.Modified.Start)
	d.Modified.SetSelections([]editor.Selection{{Anchor: off, Head: off}})
	d.Modified.Type("x")
	h.frame()
	d.recompute()
	h.settle()
	if got := len(d.regions.regions); got != n {
		t.Fatalf("%d regions, had %d", got, n)
	}
	if !d.regions.regions[0].hidden(diff.Modified).Empty() {
		t.Fatalf("region 0 folded again: %+v", d.regions.regions[0])
	}
	if d.regions.regions[1].hidden(diff.Modified).Empty() {
		t.Fatalf("region 1 not folded: %+v", d.regions.regions[1])
	}
	checkAligned(t, d, "after edit")
}
