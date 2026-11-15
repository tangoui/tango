package diffview

import (
	"testing"

	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestInlineMode(t *testing.T) {
	useTestText(t)
	orig := docLines(200)
	d := newTestDiff(t, orig, modify(orig, 15))
	d.Options.Mode = Inline
	h := newHost(t, d, geom.Size{W: 1000, H: 500}, 1, true)
	h.settle()
	if h.vw.mode != Inline || h.vw.ed[diff.Original] != nil || h.vw.ed[diff.Modified] == nil {
		t.Fatalf("mode %v, editors %v", h.vw.mode, h.vw.ed)
	}
	r, _ := d.Result()
	want := 0
	for _, c := range r.Changes {
		if !c.Original.Empty() {
			want++
		}
	}
	zs := d.Modified.ViewZones(owner)
	var deleted int
	for _, z := range zs {
		if z.ID&idDeleted != 0 && z.ID>>61 == 0 {
			deleted++
			c := r.Changes[z.ID&^idDeleted]
			if z.Rows != c.Original.Len() || z.AfterLine != c.Modified.Start-1 || z.Paint == nil || z.PaintGutter == nil {
				t.Fatalf("deleted zone %+v for %v", z, c)
			}
		}
	}
	if deleted != want || deleted == 0 {
		t.Fatalf("%d deleted zones, want %d", deleted, want)
	}
	if d.Modified.Options.OverviewRulerLanes == 0 {
		t.Fatal("no overview ruler lanes inline")
	}
	h.shot("inline")
	// The deleted lines are drawn with their syntax colors once the
	// tokens are final: the zones then get a Version.
	for range 5 {
		h.frame()
	}
	visibleVersioned := false
	first, _ := d.Modified.LineAtTop()
	for _, z := range d.Modified.ViewZones(owner) {
		if z.ID&idDeleted != 0 && z.ID>>61 == 0 && z.AfterLine >= first && z.AfterLine < first+20 && z.Version != 0 {
			visibleVersioned = true
		}
	}
	if !visibleVersioned {
		t.Fatal("no visible deleted zone with a Version")
	}
	// Strictly verified scrolling and navigation over the deleted lines.
	for _, dy := range []float32{20, 40, 13, 60, -20} {
		h.wheel(diff.Modified, dy)
		h.frame()
	}
	for range 3 {
		d.Next()
		h.frame()
	}
	h.key(input.KeyF5, input.ModAlt)
	h.frame()
}

func TestInlineBreakpoint(t *testing.T) {
	useTestText(t)
	orig := docLines(80)
	d := newTestDiff(t, orig, modify(orig, 15))
	h := newHost(t, d, geom.Size{W: 800, H: 400}, 1, false)
	h.settle()
	h.frame()
	if h.vw.mode != Inline {
		t.Fatalf("mode %v at width 800", h.vw.mode)
	}
	h.w.size = geom.Size{W: 1300, H: 400}
	h.frame()
	h.frame()
	if h.vw.mode != SideBySide {
		t.Fatalf("mode %v at width 1300", h.vw.mode)
	}
	checkAligned(t, d, "back to side by side")
	// The ViewW.Mode override wins.
	h.mods = func(w *ViewW) { w.Mode(Inline) }
	h.frame()
	if h.vw.mode != Inline {
		t.Fatalf("mode %v with the override", h.vw.mode)
	}
}
