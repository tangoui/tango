package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

// TestStickyCandidatesMatchFullWalk: the sticky lines, their positions and
// clips chosen from the ranges around the view's top (stickyCandidates)
// are the ones the walk over every folding range from the document's
// start chooses, at any scroll position (fractional, near the top, past
// nested and sibling ranges, with folded ranges above and in the view).
func TestStickyCandidatesMatchFullWalk(t *testing.T) {
	goText(t)
	const n = 3000
	st := NewState(strings.Repeat("func scope() {\n", n), "a.go", theme.Base("vs"))
	defer st.Close()
	// Nested ranges: blocks of 200 lines holding blocks of 40 holding
	// blocks of 8, and a few long ranges around them.
	var rs []FoldingRange
	rs = append(rs, FoldingRange{Start: 0, End: n - 2})
	for a := 10; a+200 < n; a += 210 {
		rs = append(rs, FoldingRange{Start: a, End: a + 200})
		for b := a + 1; b+40 <= a+200; b += 45 {
			rs = append(rs, FoldingRange{Start: b, End: b + 40})
			for c := b + 1; c+8 <= b+40; c += 10 {
				rs = append(rs, FoldingRange{Start: c, End: c + 8})
			}
		}
	}
	p := &testProvider{rs: rs}
	d, vw := viewDriver(st, 640, 300, func(w *ViewW) { w.FoldingProvider(p) })
	d.Frame()
	// Fold a few ranges, above and inside the positions scrolled to.
	for _, l := range []int{12, 432, 1272, 2112} {
		st.Sels = []Selection{{Anchor: st.Snapshot().LineStart(l), Head: st.Snapshot().LineStart(l)}}
		st.Execute("editor.fold", nil)
	}
	d.Frame()
	if got := len(st.Folded()); got != 4 || st.viewCount() < n-40 {
		t.Fatalf("%d folded ranges, %d view lines", got, st.viewCount())
	}
	lh := st.Viewport.LineHeight
	r := rand.New(rand.NewPCG(1, 2))
	checked, chained := 0, 0
	for k := range 400 {
		y := float64(r.IntN(st.viewCount()*int(lh))) + float64(r.IntN(4))*0.25
		if k < 20 {
			y = float64(k) * float64(lh) * 0.5 // near the top
		}
		st.Viewport.setScrollY(y, st.viewCount())
		d.Frame()
		w := *vw
		if w.st.fold.rsUp == nil {
			t.Fatal("no enclosing range index")
		}
		got := [3]any{slices.Clone(w.vs.stickyLines), slices.Clone(w.vs.stickyY), slices.Clone(w.vs.stickyClips)}
		if chain, _ := w.stickyCandidates(float32(st.Viewport.scrollY()), lh); len(chain) > 0 {
			chained++
		}
		up := w.st.fold.rsUp
		w.st.fold.rsUp = nil // the full walk
		w.prepareSticky(st.Snapshot())
		w.st.fold.rsUp = up
		want := [3]any{w.vs.stickyLines, w.vs.stickyY, w.vs.stickyClips}
		if !slices.Equal(got[0].([]int), want[0].([]int)) || !slices.Equal(got[1].([]float32), want[1].([]float32)) ||
			!slices.Equal(got[2].([]geom.Rect), want[2].([]geom.Rect)) {
			t.Fatalf("scroll %v: sticky %v %v %v, full walk %v %v %v", y, got[0], got[1], got[2], want[0], want[1], want[2])
		}
		checked++
	}
	if chained < 100 {
		t.Fatalf("only %d of %d positions started the walk at the view", chained, checked)
	}
}
