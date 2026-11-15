package editor

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestScrollDamageStrip locks down the large-file scroll path: two new
// rows plus a small fixed allowance (scrollbar / integer rounding), not
// a full gutter, overview or minimap repaint. Strict verification compares
// every partial frame's pixels to a full redraw (including line numbers).
func TestScrollDamageStrip(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 2} {
		st := NewState(strings.Repeat("const a = 42;\n", LargeFileLines+100), "a.ts", theme.Base("vs"))
		h := newHintHost(t, st, geom.Size{W: 640, H: 300}, scale)
		h.frame()
		st.MoveCaret(st.Snapshot().LineStart(100_000), false)
		st.RevealCaret()
		for range 3 {
			h.frame()
		}
		p := geom.Pt(320, 150)
		// Warm the first wheel frame (BenchmarkScrollPartial likewise
		// records one initial full frame), then check steady scrolling.
		h.r.HandleEvent(input.Scroll{Pos: p, DY: 2 * st.Viewport.LineHeight})
		h.frame()
		for range 8 {
			h.r.HandleEvent(input.Scroll{Pos: p, DY: 2 * st.Viewport.LineHeight})
			r := h.requirePartialStep("two-line scroll")
			f := r.frames[0]
			if f.copies != 1 {
				t.Fatalf("scale %g: copies %d, want 1", scale, f.copies)
			}
			area := 0
			for _, d := range f.dirty {
				area += d.Dx() * d.Dy()
			}
			limit := int((2*st.Viewport.LineHeight+2)*scale) * f.size.X
			if area > limit || len(f.dirty) > 3 {
				t.Fatalf("scale %g: dirty %v area %d, limit %d (exposed two rows + 2px)", scale, f.dirty, area, limit)
			}
		}
		st.Close()
	}
}

// TestStyleRunsScratch: styleRuns runs for every visible row every frame
// (a scroll frame lays out all of them); its style boundaries live in the
// view's scratch, so a call allocates only the runs and paints it returns
// (editor M2 follow-up Task C: per-frame allocations of a scroll frame).
// The result is the same as from a fresh call.
func TestStyleRunsScratch(t *testing.T) {
	st := NewState("x", "a.ts", theme.Base("vs"))
	d, vw := viewDriver(st, 400, 200, nil)
	d.Frame()
	w := *vw
	th := themeOf(st)
	line := strings.Repeat(`const answer = foo(42, "str") + bar; // note `, 4)
	hl := st.Highlighter()
	if hl == nil {
		t.Fatal("no highlighter")
	}
	toks := hl.TokenizeSpan(st.Snapshot(), 0, line, nil)
	if len(toks) < 20 {
		t.Fatalf("%d tokens", len(toks))
	}
	runs0, paints0 := w.styleRuns(line, 0, toks, th, nil)
	if n := testing.AllocsPerRun(50, func() { w.styleRuns(line, 0, toks, th, nil) }); n > 2 {
		t.Fatalf("styleRuns: %.1f allocations per call, want ≤ 2 (runs, paints)", n)
	}
	runs1, paints1 := w.styleRuns(line, 0, toks, th, nil)
	if len(runs0) != len(runs1) || len(paints0) != len(paints1) {
		t.Fatalf("runs %d/%d paints %d/%d", len(runs0), len(runs1), len(paints0), len(paints1))
	}
	for k := range runs0 {
		if runs0[k] != runs1[k] || paints0[k] != paints1[k] {
			t.Fatalf("run %d: %+v %+v vs %+v %+v", k, runs0[k], paints0[k], runs1[k], paints1[k])
		}
	}
	// With override spans (colored brackets) too.
	over := []brSpan{{b0: 18, b1: 19}, {b0: 28, b1: 29}}
	runs2, _ := w.styleRuns(line, 0, toks, th, over)
	if len(runs2) <= len(runs0) {
		t.Fatalf("override spans: %d runs, want more than %d", len(runs2), len(runs0))
	}
}
