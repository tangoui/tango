package quartz

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// bulkGrid is a grid of cells, each a background then a label. Cells are far
// enough apart that same-row coalescing never joins two labels.
func bulkGrid(n int, id uint32, overlapAfter bool) *paint.List {
	var ops []paint.Op
	for i := 0; i < n; i++ {
		x, y := float32(i%8)*90, float32(i/8)*30
		ops = append(ops, paint.FillRect{Rect: geom.Rect{X: x, Y: y, W: 80, H: 20}, Color: color.NRGBA{R: 200, A: 255}})
		t := planTestText(x+2, y+2)
		t.Glyphs[0].ID = id + uint32(i)
		ops = append(ops, t)
	}
	if overlapAfter {
		ops = append(ops, paint.FillRect{Rect: geom.Rect{X: 0, Y: 0, W: 10, H: 10}, Color: color.NRGBA{G: 255, A: 255}})
	}
	return &paint.List{Ops: ops}
}

// labels counts per-label text items; bulks counts merged bulk text items.
func labels(s *scene) (labels, bulks int) {
	for _, it := range s.items {
		if it.bulkOps != nil {
			bulks++
			continue
		}
		if it.kind != itemText {
			continue
		}
		labels++
	}
	return
}

func countKind(s *scene, k itemKind) int {
	n := 0
	for _, it := range s.items {
		if it.kind == k {
			n++
		}
	}
	return n
}

func TestPlanBulkMergesSustainedChangingText(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	if e := s.plan(bulkGrid(40, 100, false), size, 1); e != nil {
		t.Fatal(e)
	}
	if l, b := labels(s); l != 40 || b != 0 || s.bulk {
		t.Fatalf("first frame must stay per-layer: labels=%d bulks=%d", l, b)
	}
	if e := s.plan(bulkGrid(40, 1000, false), size, 1); e != nil {
		t.Fatal(e)
	}
	if l, b := labels(s); !s.bulk || l != 0 || b != 1 {
		t.Fatalf("second changing frame must merge: labels=%d bulks=%d bulk=%v", l, b, s.bulk)
	}
	last := s.items[len(s.items)-1]
	if last.kind != itemPatch || len(last.bulkOps) != 40 || last.frame != s.bounds {
		t.Fatalf("bulk text must be one window item drawn last: kind=%v ops=%d frame=%v", last.kind, len(last.bulkOps), last.frame)
	}
	if countKind(s, itemSolid) != 40 {
		t.Fatal("backgrounds must keep their own layers")
	}
}

func TestPlanBulkRespectsLaterOverlappingPaint(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	for _, id := range []uint32{100, 1000, 2000} {
		if e := s.plan(bulkGrid(40, id, true), size, 1); e != nil {
			t.Fatal(e)
		}
	}
	if l, b := labels(s); l != 40 || b != 0 {
		t.Fatalf("text was moved above later overlapping paint: labels=%d bulks=%d", l, b)
	}
}

func TestPlanBulkExitsAfterCalmFrames(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	s.plan(bulkGrid(40, 100, false), size, 1)
	s.plan(bulkGrid(40, 1000, false), size, 1)
	if !s.bulk {
		t.Fatal("bulk not entered")
	}
	for i := 0; i < bulkCalmFrames-1; i++ {
		s.plan(bulkGrid(40, 1000, false), size, 1)
		if !s.bulk {
			t.Fatalf("left bulk after %d calm frames", i+1)
		}
	}
	s.plan(bulkGrid(40, 1000, false), size, 1)
	if l, _ := labels(s); s.bulk || l != 40 {
		t.Fatalf("calm content must return to per-layer text: bulk=%v labels=%d", s.bulk, l)
	}
}

func TestPlanBulkIgnoresFewChanges(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	for i := 0; i < 5; i++ {
		l := bulkGrid(40, 100, false)
		// one cell changes per frame: a ticking clock, not a dashboard
		l.Ops[1] = func() paint.Op { t := l.Ops[1].(paint.Text); t.Glyphs[0].ID = uint32(5000 + i); return t }()
		if e := s.plan(l, size, 1); e != nil {
			t.Fatal(e)
		}
	}
	if s.bulk {
		t.Fatal("a few changing labels must not merge the whole window")
	}
}

// Cells side by side: the next cell's background touches a label's padded
// native bounds but not its ink. Bulk mode must still apply; real ink overlap
// must still block it (TestPlanBulkRespectsLaterOverlappingPaint).
func TestPlanBulkAllowsAdjacentBackgrounds(t *testing.T) {
	grid := func(id uint32) *paint.List {
		var ops []paint.Op
		for i := 0; i < 40; i++ {
			x, y := float32(i%8)*40, float32(i/8)*20
			ops = append(ops, paint.FillRect{Rect: geom.Rect{X: x, Y: y, W: 40, H: 20}, Color: color.NRGBA{R: 200, A: 255}})
			t := planTestText(x+2, y+2)
			t.Glyphs[0].ID = id + uint32(i)
			ops = append(ops, t)
		}
		return &paint.List{Ops: ops}
	}
	// Like the CoreGraphics provider, pad native bounds for smoothing; the
	// padding spans into the neighbouring cell, the glyph ink does not.
	s := newScene(paddedGlyphs{})
	size := geom.Size{W: 400, H: 200}
	for _, id := range []uint32{100, 1000} {
		if e := s.plan(grid(id), size, 1); e != nil {
			t.Fatal(e)
		}
	}
	if l, b := labels(s); l != 0 || b != 1 {
		t.Fatalf("adjacent cell backgrounds blocked bulk text: labels=%d bulks=%d", l, b)
	}
}

type paddedGlyphs struct{ planGlyphs }

func (paddedGlyphs) Bounds(run nativeglyph.Run) (image.Rectangle, bool) {
	b, ok := planGlyphs{}.Bounds(run)
	return b.Inset(-40), ok
}

// A later overlapping paint appearing while bulk mode is on must re-plan
// the frame with per-label native text, in that same frame.
func TestPlanBulkReplansWhenLiftBecomesUnsafe(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	s.plan(bulkGrid(40, 100, false), size, 1)
	s.plan(bulkGrid(40, 1000, false), size, 1)
	if !s.bulk || !s.bulkMerged {
		t.Fatal("bulk not entered")
	}
	if e := s.plan(bulkGrid(40, 2000, true), size, 1); e != nil {
		t.Fatal(e)
	}
	if l, b := labels(s); l != 40 || b != 0 || s.bulkMerged {
		t.Fatalf("unsafe frame kept bulk text: labels=%d bulks=%d", l, b)
	}
	for _, it := range s.items {
		if it.kind == itemText && (it.key == "" || len(it.runs) == 0) {
			t.Fatal("re-planned text has no native layer content")
		}
	}
	if e := s.plan(bulkGrid(40, 3000, false), size, 1); e != nil {
		t.Fatal(e)
	}
	if _, b := labels(s); b != 1 {
		t.Fatal("bulk did not resume once lifting was safe again")
	}
}

// A changing grid whose text can never be lifted (a later overlay covers a
// label) must not plan every frame twice: after a failed lift it stops
// planning lazily for a while, and retries later.
func TestPlanBulkBacksOffWhenLiftKeepsFailing(t *testing.T) {
	s := newScene(planGlyphs{})
	size := geom.Size{W: 800, H: 600}
	s.plan(bulkGrid(40, 100, true), size, 1)
	replans := 0
	for f := 0; f < 40; f++ {
		before := s.serial
		if e := s.plan(bulkGrid(40, uint32(1000*(f+1)), true), size, 1); e != nil {
			t.Fatal(e)
		}
		if s.serial-before > 1 {
			replans++
		}
		if l, _ := labels(s); l != 40 {
			t.Fatalf("frame %d: %d per-label items, want 40", f, l)
		}
	}
	if replans > 2 {
		t.Fatalf("planned twice on %d of 40 frames with an unliftable grid", replans)
	}
}
