package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/cpuexec"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// fpHost runs a page through a gpu.Renderer on a cpuexec.Device in strict
// verify mode: every partial frame is compared with a full redraw and a
// difference fails the frame.
type fpHost struct {
	t   *testing.T
	r   *Runner
	dev *cpuexec.Device
	now time.Time
}

func newFPHost(t *testing.T, page func(*Context), size geom.Size) *fpHost {
	t.Helper()
	h := &fpHost{t: t, dev: cpuexec.NewDevice(), now: time.Unix(1000, 0)}
	h.dev.SampledOnly = true
	ren := gpu.NewRenderer(h.dev)
	ren.SetVerify(true)
	ren.SetVerifyStrict(true)
	h.r = NewRunner(page, RunnerOptions{Renderer: ren, NoAnimations: true, Clock: func() time.Time { return h.now }})
	h.r.Attach(&damageWin{size: size, scale: 1})
	return h
}

// frame runs a frame and reports whether the device drew it partially.
func (h *fpHost) frame() bool {
	h.t.Helper()
	before := h.dev.Stats().Partial
	if err := h.r.Frame(); err != nil {
		h.t.Fatalf("frame: %v", err)
	}
	return h.dev.Stats().Partial > before
}

// declarer is a widget that declares damage for its own area every frame
// (the way a code editor's caret does), which makes every frame partial.
func declarer(c *Context) {
	Div(c, layout.Size(40, 20)).Bg(c.Theme.Primary.Base)
	if r, ok := c.RectOf(c.lastID()); ok {
		c.Damage(r)
	} else {
		c.Damage(geom.Rect{W: 40, H: 20})
	}
}

// lastID is the ID of the node built last under the current parent.
func (c *Context) lastID() ID {
	k := c.cur.kids
	return k[len(k)-1].id
}

// TestFingerprintStatusBar: a widget declares damage for its own area every
// frame; a Text elsewhere changes without declaring. Its old and new rects
// are damaged and the partial frame equals a full redraw.
func TestFingerprintStatusBar(t *testing.T) {
	status := "Ln 1, Col 1"
	var textID ID
	page := func(c *Context) {
		Column(c, func() {
			declarer(c)
			Div(c, layout.H(100))
			tx := Text(c, status)
			textID = tx.n.id
		})
	}
	h := newFPHost(t, page, geom.Size{W: 320, H: 200})
	h.frame()
	h.frame()
	for i := 2; i < 8; i++ {
		status = fmt.Sprintf("Ln %d, Col %d", i*7, i*13)
		if !h.frame() {
			t.Fatalf("frame %d not partial", i)
		}
		tr, _ := h.r.c.RectOf(textID)
		if dam, _ := hints(h.r); !covered(dam, tr) {
			t.Fatalf("frame %d: damage %v misses the status text %v", i, dam, tr)
		}
	}
	// Nothing changed: only the declared area.
	if !h.frame() {
		t.Fatal("idle frame not partial")
	}
	if dam, _ := hints(h.r); len(dam) != 1 || dam[0] != (geom.Rect{W: 40, H: 20}) {
		t.Fatalf("idle frame damage %v, want only the declared rect", dam)
	}
}

// TestFingerprintInsideDeclaration: a node that changes inside the rect
// its widget declared adds no damage beyond the declaration (the frame
// stays as small as declared).
func TestFingerprintInsideDeclaration(t *testing.T) {
	red := false
	page := func(c *Context) {
		col := c.Theme.Primary.Base
		if red {
			col = c.Theme.Error.Base
		}
		Div(c, layout.Size(40, 40)).Bg(col)
		c.Damage(geom.Rect{W: 40, H: 40})
	}
	h := newFPHost(t, page, geom.Size{W: 200, H: 100})
	h.frame()
	h.frame()
	for i := range 3 {
		red = !red
		if !h.frame() {
			t.Fatalf("frame %d not partial", i)
		}
		if dam, _ := hints(h.r); len(dam) != 1 || dam[0] != (geom.Rect{W: 40, H: 40}) {
			t.Fatalf("frame %d: damage %v, want only the declared rect", i, dam)
		}
	}
}

// TestFingerprintNodeAddedRemoved: a node that appears or vanishes without
// a declaration is damaged.
func TestFingerprintNodeAddedRemoved(t *testing.T) {
	show := false
	box := geom.Rect{X: 100, Y: 100, W: 60, H: 30}
	page := func(c *Context) {
		declarer(c)
		if show {
			Div(c, layout.Abs(box.X, box.Y, layout.Unset, layout.Unset), layout.Size(box.W, box.H)).Bg(c.Theme.Error.Base)
		}
	}
	h := newFPHost(t, page, geom.Size{W: 320, H: 200})
	h.frame()
	h.frame()
	for i, s := range []bool{true, false, true, false} {
		show = s
		if !h.frame() {
			t.Fatalf("step %d not partial", i)
		}
		if dam, _ := hints(h.r); !covered(dam, box) {
			t.Fatalf("step %d (show %v): damage %v misses %v", i, s, dam, box)
		}
	}
}

// TestFingerprintMovedNode: a node that moves (layout change) without a
// declaration is damaged at both places.
func TestFingerprintMovedNode(t *testing.T) {
	top := float32(40)
	page := func(c *Context) {
		declarer(c)
		Div(c, layout.Abs(100, top, layout.Unset, layout.Unset), layout.Size(50, 20)).Bg(c.Theme.Success.Base)
	}
	h := newFPHost(t, page, geom.Size{W: 320, H: 200})
	h.frame()
	h.frame()
	for _, y := range []float32{90, 140, 10} {
		old := geom.Rect{X: 100, Y: top, W: 50, H: 20}
		top = y
		if !h.frame() {
			t.Fatalf("move to %v not partial", y)
		}
		dam, _ := hints(h.r)
		if !covered(dam, old) || !covered(dam, geom.Rect{X: 100, Y: y, W: 50, H: 20}) {
			t.Fatalf("move to %v: damage %v", y, dam)
		}
	}
}

// TestFingerprintPureScroll: a ScrollShift of unchanged content adds only
// the uncovered strip (and the scrollbar), not the whole region; changing
// a row inside the scrolled content adds that row.
func TestFingerprintPureScroll(t *testing.T) {
	view := geom.Rect{W: 300, H: 200}
	var st *scrollState
	var prev float64
	label := "row 7"
	page := func(c *Context) {
		declarer(c)
		sb := Scroll(c, func() {
			for i := range 40 {
				Row(c, func() {
					if i == 7 {
						Text(c, label)
					} else {
						Text(c, fmt.Sprintf("row %d", i))
					}
				}, layout.H(24)).Bg(c.Theme.BgContainer)
			}
			if st != nil {
				c.ScrollShift(view, geom.Point{Y: float32(prev - st.off.Y)})
			}
		}, layout.Abs(0, 0, layout.Unset, layout.Unset), layout.Size(view.W, view.H))
		st = sb.s
	}
	// The renderer merges the scrollbar strip with the exposed strip
	// into the whole region; a large window keeps that under half the
	// frame, so the scrolling frames stay partial (and verified).
	h := newFPHost(t, page, geom.Size{W: 800, H: 600})
	h.frame()
	h.frame()
	scroll := func(dy float64) {
		prev = st.off.Y
		st.off.Y += dy
		if !h.frame() {
			d, sc := hints(h.r)
			t.Fatalf("scroll %v not partial: %v %v", dy, d, sc)
		}
	}
	for _, dy := range []float64{10, 24, 7} {
		scroll(dy)
		dam, sc := hints(h.r)
		if len(sc) != 1 {
			t.Fatalf("scroll %v: regions %v", dy, sc)
		}
		mid := geom.Rect{X: 20, Y: 60, W: 200, H: 80}
		if touches(dam, mid) {
			t.Fatalf("scroll %v: pure scroll damaged the middle of the region: %v", dy, dam)
		}
	}
	// Row 7 changes its text while scrolling.
	label = "row seven"
	scroll(5)
	dam, _ := hints(h.r)
	if !touches(dam, geom.Rect{X: 20, Y: 7*24 - float32(st.off.Y) + 4, W: 10, H: 10}) {
		t.Fatalf("changed row not damaged: %v", dam)
	}
	scroll(-30)
	scroll(0) // a declared zero shift redraws the region
}

// TestFingerprintHoverAndAnimation: hover on a button and its transition
// still produce frames equal to full redraws while another widget declares
// damage.
func TestFingerprintHoverAndAnimation(t *testing.T) {
	var btn ID
	page := func(c *Context) {
		declarer(c)
		Column(c, func() {
			b := Button(c, "Press me")
			btn = b.n.id
		}, layout.Abs(100, 80, layout.Unset, layout.Unset))
	}
	h := newFPHost(t, page, geom.Size{W: 320, H: 200})
	h.frame()
	h.frame()
	br, _ := h.r.c.RectOf(btn)
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: br.Center()})
	h.frame()
	h.frame()
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: br.Center(), Button: input.ButtonLeft})
	h.frame()
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: br.Center(), Button: input.ButtonLeft})
	h.frame()
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(5, 190)})
	h.frame()
	h.frame()
}

// TestFingerprintNoHintsNoHashing: frames without hints only record; the
// first partial frame after them hashes the last frame lazily.
func TestFingerprintNoHintsNoHashing(t *testing.T) {
	declare := false
	text := "a"
	page := func(c *Context) {
		if declare {
			c.Damage(geom.Rect{W: 4, H: 4})
		}
		Column(c, func() { Text(c, text) }, layout.Abs(50, 50, layout.Unset, layout.Unset))
	}
	h := newFPHost(t, page, geom.Size{W: 200, H: 100})
	h.frame()
	h.frame()
	if h.r.c.fp.prevHashed {
		t.Fatal("a frame without hints hashed its records")
	}
	declare, text = true, "b" // first hinted frame after an unhinted one
	if !h.frame() {
		t.Fatal("not partial")
	}
	if dam, _ := hints(h.r); len(dam) < 2 {
		t.Fatalf("changed text not damaged against the lazily hashed baseline: %v", dam)
	}
}

// fpBenchPage builds about 1000 visible nodes: 50 rows of 6 cells, each a
// label and two boxes.
func fpBenchPage(declare bool) func(*Context) {
	return func(c *Context) {
		if declare {
			c.Damage(geom.Rect{W: 4, H: 4})
		}
		Column(c, func() {
			for i := range 50 {
				Row(c, func() {
					for j := range 6 {
						Row(c, func() {
							Text(c, fmt.Sprintf("Cell %d.%d", i, j))
							Div(c, layout.Size(16, 16)).Bg(c.Theme.Primary.Base).Radius(4)
							Div(c, layout.Size(40, 16)).Bg(c.Theme.Success.Base)
						}, layout.W(200))
					}
				}, layout.H(22))
			}
		})
	}
}

// BenchmarkFingerprint measures, on a page of about 1000 nodes, a whole
// frame without hints, a whole frame with hints (fingerprinting included)
// and the fingerprint work alone (hash the frame's records, and hash plus
// compare against the previous frame).
func BenchmarkFingerprint(b *testing.B) {
	run := func(b *testing.B, declare bool) *Runner {
		r := NewRunner(fpBenchPage(declare), RunnerOptions{Renderer: nopRenderer{}, NoAnimations: true,
			Clock: func() time.Time { return time.Unix(1000, 0) }})
		r.Attach(&damageWin{size: geom.Size{W: 1200, H: 1100}, scale: 1})
		for range 3 {
			if err := r.Frame(); err != nil {
				b.Fatal(err)
			}
		}
		return r
	}
	b.Run("frame-nohints", func(b *testing.B) {
		r := run(b, false)
		b.ReportAllocs()
		for b.Loop() {
			r.Frame()
		}
		b.ReportMetric(float64(len(r.c.fp.prev)), "nodes")
	})
	b.Run("frame-hinted", func(b *testing.B) {
		r := run(b, true)
		b.ReportAllocs()
		for b.Loop() {
			r.Frame()
		}
		b.ReportMetric(float64(len(r.c.fp.prev)), "nodes")
		b.ReportMetric(float64(len(r.list.Ops)), "ops")
	})
	b.Run("hash", func(b *testing.B) {
		r := run(b, true)
		s := &r.c.fp
		b.ReportAllocs()
		for b.Loop() {
			s.hash(s.prev, r.list.Ops)
		}
	})
	b.Run("hash+compare", func(b *testing.B) {
		r := run(b, true)
		c := r.c
		s := &c.fp
		// Compare the last frame's records with themselves.
		s.cur = append(s.cur[:0], s.prev...)
		var dam []geom.Rect
		b.ReportAllocs()
		for b.Loop() {
			s.prevHashed = true
			dam = c.fingerprintDamage(dam[:0], r.list.Ops)
		}
		if len(dam) != 0 {
			b.Fatalf("unchanged frame damaged %v", dam)
		}
	})
}
