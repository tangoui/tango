package terminal

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/terminal/vt"
)

func TestViewSearch(t *testing.T) {
	g := newViewRig(t, 400, 200, nil)
	g.output("alpha\r\nbeta alpha\r\ngamma\r\n", func(v *vt.Terminal) bool { return rowText(v, 2) == "gamma" })
	g.focus()

	count, at := g.vw.Search("alpha", false)
	if count != 2 || at != 1 {
		t.Fatalf("Search = %d,%d want 2,1", count, at)
	}
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "alpha" {
		t.Fatalf("current match copy = %q, want alpha", got)
	}

	if count, at = g.vw.SearchStep(1); count != 2 || at != 2 {
		t.Fatalf("step next = %d,%d want 2,2", count, at)
	}
	if count, at = g.vw.SearchStep(1); count != 2 || at != 1 {
		t.Fatalf("step wrap = %d,%d want 2,1", count, at)
	}
	if count, at = g.vw.SearchStep(-1); count != 2 || at != 2 {
		t.Fatalf("step prev = %d,%d want 2,2", count, at)
	}
	if count, at = g.vw.SearchInfo(); count != 2 || at != 2 {
		t.Fatalf("SearchInfo = %d,%d want 2,2", count, at)
	}

	if count, at = g.vw.Search("zzz", false); count != 0 || at != 0 {
		t.Fatalf("no-match = %d,%d want 0,0", count, at)
	}
	g.vw.ClearSearch()
	if count, at = g.vw.SearchInfo(); count != 0 || at != 0 {
		t.Fatalf("after clear = %d,%d want 0,0", count, at)
	}
}

func TestViewSearchScrollsToScrollback(t *testing.T) {
	g := newViewRig(t, 400, 64, nil) // few rows so early lines scroll off
	g.output("needle one\r\n", func(v *vt.Terminal) bool { return rowText(v, 0) == "needle one" })
	for i := 0; i < 20; i++ {
		g.output("filler\r\n", func(v *vt.Terminal) bool { return true })
	}
	g.focus()
	count, at := g.vw.Search("needle", false)
	if count != 1 || at != 1 {
		t.Fatalf("Search = %d,%d want 1,1", count, at)
	}
	v := g.term.Lock()
	off := v.Screen().ViewOffset()
	g.term.Unlock()
	if off == 0 {
		t.Fatalf("viewport not scrolled to scrollback hit (offset 0)")
	}
	g.d.Key(input.KeyC, copyMods())
	if got := g.d.Clipboard(); got != "needle" {
		t.Fatalf("scrollback match copy = %q, want needle", got)
	}
}
