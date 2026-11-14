package diffview

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func (h *host) lastSaid() string {
	if h.aw == nil || len(h.aw.said) == 0 {
		return ""
	}
	return h.aw.said[len(h.aw.said)-1]
}

func TestNavigationKeys(t *testing.T) {
	useTestText(t)
	orig := docLines(200)
	d := newTestDiff(t, orig, modify(orig, 20))
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newA11yHost(t, d, geom.Size{W: 1200, H: 500})
	h.settle()
	r, _ := d.Result()
	n := len(r.Changes)
	h.clickLine(diff.Modified, 0)
	h.frame()
	// Alt+F5 goes to the changes in turn, on the focused side.
	for i := range n + 1 {
		h.key(input.KeyF5, input.ModAlt)
		h.frame()
		want := i % n
		if got := caretLine(d.Modified); got != r.Changes[want].Modified.Start {
			t.Fatalf("next %d: caret on line %d, want %d", i, got, r.Changes[want].Modified.Start)
		}
		if s := h.lastSaid(); !strings.HasPrefix(s, "第 ") || !strings.Contains(s, "共 ") {
			t.Fatalf("announcement %q", s)
		}
		checkAligned(t, d, "next")
	}
	want := announceChange(announceTexts(nil), 0, n, r.Changes[0])
	if got := h.lastSaid(); got != want {
		t.Fatalf("announced %q, want %q", got, want)
	}
	// Shift+Alt+F5 goes back (wrapping to the last change).
	h.key(input.KeyF5, input.ModAlt|input.ModShift)
	h.frame()
	if got := caretLine(d.Modified); got != r.Changes[n-1].Modified.Start {
		t.Fatalf("prev: caret on line %d, want %d", got, r.Changes[n-1].Modified.Start)
	}
	// On the original side, the original's lines.
	d.Original.SetScrollTop(0)
	h.frame()
	h.clickLine(diff.Original, 1)
	h.frame()
	h.key(input.KeyF5, input.ModAlt)
	h.frame()
	if got := caretLine(d.Original); got != r.Changes[0].Original.Start {
		t.Fatalf("original next: caret on line %d, want %d", got, r.Changes[0].Original.Start)
	}
	if d.focus != diff.Original {
		t.Fatal("focus")
	}
	// Moving the caret into a change with the keyboard announces it.
	h.aw.said = nil
	line := caretLine(d.Original)
	for range r.Changes[1].Original.Start - line {
		h.key(input.KeyDown, 0)
		h.frame()
	}
	if got := h.lastSaid(); got != announceChange(announceTexts(nil), 1, n, r.Changes[1]) {
		t.Fatalf("moving into change 1 announced %q (said %q)", got, h.aw.said)
	}
}

func TestNavigationEnglish(t *testing.T) {
	useTestText(t)
	orig := docLines(60)
	d := newTestDiff(t, orig, modify(orig, 20))
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newA11yHost(t, d, geom.Size{W: 1200, H: 500})
	h.r = ui.NewRunner(func(c *ui.Context) {
		ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() { h.vw = View(c, d) })
	}, ui.RunnerOptions{NoAnimations: true})
	h.r.Attach(h.aw)
	h.settle()
	if !d.Next() {
		t.Fatal("Next")
	}
	h.frame()
	if got := h.lastSaid(); !strings.HasPrefix(got, "Difference 1 of ") {
		t.Fatalf("announced %q", got)
	}
	if got := d.label(diff.Original); !strings.HasPrefix(got, "Original (read-only) — a.go") {
		t.Fatalf("label %q", got)
	}
}

func TestOverviewRuler(t *testing.T) {
	useTestText(t)
	orig := docLines(600)
	d := newTestDiff(t, orig, modify(orig, 30))
	d.Options.UseInlineWhenSpaceIsLimited = false
	h := newHost(t, d, geom.Size{W: 1200, H: 500}, 1, false)
	h.settle()
	ov := h.d.Modified
	er := h.editorRect(diff.Modified)
	x := er.Right() + overviewW/2
	if len(d.lay.ov.rects[0]) == 0 || len(d.lay.ov.rects[1]) == 0 {
		t.Fatalf("marks %v", d.lay.ov.rects)
	}
	// A click centres the viewport there.
	h.click(geom.Pt(x, er.Y+er.H/2))
	h.frame()
	sh := ov.ScrollHeight()
	mid := ov.ScrollTop() + float64(ov.Viewport.Height)/2
	if mid < sh*0.45 || mid > sh*0.55 {
		t.Fatalf("click at the middle: viewport centre %v of %v", mid, sh)
	}
	checkAligned(t, d, "overview click")
	// A drag follows the pointer.
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, er.Y+er.H*0.2)})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: geom.Pt(x, er.Y+er.H*0.2)})
	h.frame()
	top0 := ov.ScrollTop()
	for _, f := range []float32{0.3, 0.5, 0.8} {
		h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, er.Y+er.H*f)})
		h.frame()
		checkAligned(t, d, "overview drag")
	}
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: geom.Pt(x, er.Y+er.H*0.8)})
	h.frame()
	if ov.ScrollTop() <= top0 {
		t.Fatalf("drag down: %v -> %v", top0, ov.ScrollTop())
	}
	mid = ov.ScrollTop() + float64(ov.Viewport.Height)/2
	if mid < sh*0.75 || mid > sh*0.85 {
		t.Fatalf("dragged to 0.8: viewport centre %v of %v", mid, sh)
	}
}
