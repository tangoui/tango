package diffview

import (
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
)

func TestSideBySideAlignedScroll(t *testing.T) {
	useTestText(t)
	orig := docLines(400)
	orig[1] += " // a long line, wider than both editors, so that both sides scroll horizontally ..........................."
	d := newTestDiff(t, orig, modify(orig, 20))
	h := newHost(t, d, geom.Size{W: 1400, H: 700}, 1, false)
	h.settle()
	if r, ok := d.Result(); !ok || len(r.Changes) == 0 {
		t.Fatalf("no result: %v %d", ok, len(r.Changes))
	}
	if h.vw.mode != SideBySide {
		t.Fatalf("mode %v", h.vw.mode)
	}
	if len(d.Original.ViewZones(owner)) == 0 || len(d.Modified.ViewZones(owner)) == 0 {
		t.Fatalf("zones: %d / %d", len(d.Original.ViewZones(owner)), len(d.Modified.ViewZones(owner)))
	}
	if a, b := d.Original.ScrollHeight(), d.Modified.ScrollHeight(); a != b {
		t.Fatalf("scroll heights %v / %v", a, b)
	}
	if n := len(d.Modified.Decorations(owner)); n == 0 {
		t.Fatal("no decorations")
	}
	checkAligned(t, d, "start")
	h.shot("sbs-start")
	for i, dy := range []float32{37, 130, 211, 7, 400, -55, 999, -321, 18, 3000} {
		s := diff.Modified
		if i%2 == 1 {
			s = diff.Original
		}
		h.wheel(s, dy)
		h.frame()
		checkAligned(t, d, "wheel")
		if d.Original.ScrollTop() == 0 && d.Modified.ScrollTop() == 0 {
			t.Fatal("did not scroll")
		}
	}
	// Scrolling directly between frames is reported at the next frame.
	d.Original.SetScrollTop(2345)
	h.frame()
	h.frame()
	checkAligned(t, d, "SetScrollTop")
	h.shot("sbs-scrolled")
	// Horizontal scrolling follows too.
	d.Modified.SetScrollLeft(40)
	h.frame()
	if d.Original.ScrollLeft() != d.Modified.ScrollLeft() {
		t.Fatalf("scroll left %v / %v", d.Original.ScrollLeft(), d.Modified.ScrollLeft())
	}
}

func TestSideBySideOptionsOverrides(t *testing.T) {
	useTestText(t)
	orig := docLines(50)
	d := newTestDiff(t, orig, modify(orig, 10))
	d.Modified.Options.FontSize = 16
	h := newHost(t, d, geom.Size{W: 1400, H: 600}, 1, false)
	h.settle()
	for _, st := range d.sides() {
		if st.Options.Folding || st.Options.Minimap.Enabled || st.Options.OverviewRulerLanes != 0 {
			t.Fatalf("options not overridden: %+v", st.Options)
		}
	}
	if !d.Original.Options.ReadOnly || d.Modified.Options.ReadOnly {
		t.Fatal("read-only")
	}
	if d.Original.Viewport.LineHeight != d.Modified.Viewport.LineHeight {
		t.Fatalf("line heights %v / %v", d.Original.Viewport.LineHeight, d.Modified.Viewport.LineHeight)
	}
}

func TestSideBySideWrappedAligned(t *testing.T) {
	useTestText(t)
	orig := docLines(300)
	mod := modify(orig, 15)
	for i := range mod {
		if i%7 == 3 {
			mod[i] += " // a long comment that makes this line wrap around in the narrow editor of the modified side"
		}
	}
	d := newTestDiff(t, orig, mod)
	d.Options.DiffWordWrap = editor.WrapOn
	h := newHost(t, d, geom.Size{W: 1000, H: 600}, 1, false)
	d.Options.UseInlineWhenSpaceIsLimited = false
	h.settle()
	checkAligned(t, d, "wrapped start")
	h.shot("sbs-wrapped")
	for i, dy := range []float32{90, 333, 47, 1200, -200, 640} {
		s := diff.Modified
		if i%2 == 1 {
			s = diff.Original
		}
		h.wheel(s, dy)
		h.frame()
		checkAligned(t, d, "wrapped wheel")
	}
}
