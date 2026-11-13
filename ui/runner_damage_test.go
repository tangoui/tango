package ui

import (
	"errors"
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// damageWin is a minimal Window for the damage tests.
type damageWin struct {
	size      geom.Size
	scale     float32
	presented int
}

func (w *damageWin) Size() geom.Size               { return w.size }
func (w *damageWin) Scale() float32                { return w.scale }
func (w *damageWin) Invalidate()                   {}
func (w *damageWin) InvalidateAt(time.Time)        {}
func (w *damageWin) SetCursor(input.Cursor)        {}
func (w *damageWin) ReadClipboard() string         { return "" }
func (w *damageWin) WriteClipboard(string)         {}
func (w *damageWin) SetTextInput(bool, geom.Rect)  {}
func (w *damageWin) Present(img *image.RGBA) error { w.presented++; return nil }

// damagePresenterWin also implements DamagePresenter.
type damagePresenterWin struct {
	damageWin
	partial int
	dirty   []image.Rectangle
}

func (w *damagePresenterWin) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	w.partial++
	w.dirty = append(w.dirty[:0], dirty...)
	return nil
}

type damageRig struct {
	r   *Runner
	w   Window
	now time.Time
}

func newDamageRig(t *testing.T, page func(*Context), w Window, o RunnerOptions) *damageRig {
	t.Helper()
	g := &damageRig{now: time.Unix(1000, 0), w: w}
	o.Clock = func() time.Time { return g.now }
	g.r = NewRunner(page, o)
	g.r.Attach(w)
	return g
}

func (g *damageRig) frame(t *testing.T) {
	t.Helper()
	if err := g.r.Frame(); err != nil {
		t.Fatal(err)
	}
}

func newWin() *damageWin { return &damageWin{size: geom.Size{W: 400, H: 300}, scale: 1} }

// hints returns the Damage rects and ScrollRegion ops of the last frame.
func hints(r *Runner) (dam []geom.Rect, sc []paint.ScrollRegion) {
	for _, op := range r.List().Ops {
		switch o := op.(type) {
		case paint.Damage:
			dam = append(dam, o.Rect)
		case paint.ScrollRegion:
			sc = append(sc, o)
		}
	}
	return
}

// covered reports whether the union of dam covers want (sampled at
// quarter-pixel steps, edges included).
func covered(dam []geom.Rect, want geom.Rect) bool {
	in := func(p geom.Point) bool {
		for _, d := range dam {
			if p.X >= d.X && p.X <= d.Right() && p.Y >= d.Y && p.Y <= d.Bottom() {
				return true
			}
		}
		return false
	}
	for y := want.Y; y <= want.Bottom(); y += 0.25 {
		for x := want.X; x <= want.Right(); x += 0.25 {
			if !in(geom.Pt(x, y)) {
				return false
			}
		}
	}
	return true
}

func touches(dam []geom.Rect, want geom.Rect) bool {
	for _, d := range dam {
		if !d.Intersect(want).Empty() {
			return true
		}
	}
	return false
}

var smallDamage = geom.Rect{X: 390, Y: 290, W: 4, H: 4}

func TestRunnerCollectsDamage(t *testing.T) {
	declare := false
	page := func(c *Context) {
		Div(c, layout.Size(100, 40)).Bg(c.Theme.Primary.Base)
		if declare {
			c.Damage(geom.Rect{X: 10, Y: 10, W: 20, H: 20})
			c.Damage(geom.Rect{X: 50, Y: 10, W: 5, H: 5})
		}
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	if d, s := hints(g.r); len(d)+len(s) != 0 {
		t.Fatalf("first frame wrote hints %v %v", d, s)
	}
	declare = true
	g.frame(t)
	d, s := hints(g.r)
	if len(s) != 0 {
		t.Fatalf("unexpected scroll regions %v", s)
	}
	if len(d) != 2 || d[0] != (geom.Rect{X: 10, Y: 10, W: 20, H: 20}) || d[1] != (geom.Rect{X: 50, Y: 10, W: 5, H: 5}) {
		t.Fatalf("damage = %v", d)
	}
	ops := g.r.List().Ops
	if _, ok := ops[len(ops)-1].(paint.Damage); !ok {
		t.Fatalf("hints not at the end of the list: %T", ops[len(ops)-1])
	}
	// Empty and off-window rects are dropped; partly visible ones clipped.
	page2 := func(c *Context) {
		Div(c, layout.Size(100, 40)).Bg(c.Theme.Primary.Base) // unchanged: not damaged
		c.Damage(geom.Rect{X: 10, Y: 10})
		c.Damage(geom.Rect{X: 380, Y: -10, W: 40, H: 20})
		c.Damage(geom.Rect{X: 500, Y: 10, W: 5, H: 5})
	}
	g.r.page = page2
	g.frame(t)
	d, _ = hints(g.r)
	if len(d) != 1 || d[0] != (geom.Rect{X: 380, Y: 0, W: 20, H: 10}) {
		t.Fatalf("clipped damage = %v", d)
	}
}

func TestRunnerAutoDamageAnimation(t *testing.T) {
	wide := false
	var boxID ID
	page := func(c *Context) {
		Gap(c, 50)
		b := Div(c, layout.H(20))
		boxID = b.ID()
		target := float32(40)
		if wide {
			target = 120
		}
		b.Style(layout.W(c.Anim(b.ID(), "w", target, 200*time.Millisecond))).Bg(c.Theme.Primary.Base)
		c.Damage(smallDamage)
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(boxID)
	if d, _ := hints(g.r); len(d) != 1 || d[0] != smallDamage {
		t.Fatalf("settled frame damage = %v", d)
	}
	wide = true
	g.now = g.now.Add(100 * time.Millisecond)
	g.frame(t) // target changes: still at 40, but now in flight
	if d, _ := hints(g.r); !covered(d, old) {
		t.Fatalf("start: damage %v misses animated box %v", d, old)
	}
	g.now = g.now.Add(100 * time.Millisecond)
	g.frame(t) // moving: the clock advanced after an animating frame -> full
	cur, _ := g.r.Context().RectOf(boxID)
	if cur.W <= old.W {
		t.Fatalf("not animating: %v -> %v", old, cur)
	}
	if d, s := hints(g.r); len(d)+len(s) != 0 {
		t.Fatalf("timed frame wrote partial hints %v %v", d, s)
	}
	g.frame(t) // same instant (e.g. an input frame): partial, anim still damaged
	if d, _ := hints(g.r); !covered(d, cur) || covered(d, geom.Rect{X: 200, Y: 150, W: 100, H: 100}) {
		t.Fatalf("damage %v misses animated box %v (or is too wide)", d, cur)
	}
	g.now = g.now.Add(time.Second)
	g.frame(t) // lands on 120 (full: clock moved after animating)
	g.frame(t) // settled: nothing moves
	if d, _ := hints(g.r); len(d) != 1 || d[0] != smallDamage {
		t.Fatalf("settled frame damage = %v", d)
	}
}

// siblingShiftPage stacks a box whose height animates from 20 to 60 (tall
// selects the target) above a sibling that moves down with it, and a
// static box to the right of both.
func siblingShiftPage(tall *bool, boxID, sibID *ID) func(*Context) {
	return func(c *Context) {
		Row(c, func() {
			Div(c, layout.W(200)).Body(func() {
				b := Div(c, layout.W(200))
				*boxID = b.ID()
				target := float32(20)
				if *tall {
					target = 60
				}
				b.Style(layout.H(c.Anim(b.ID(), "h", target, 200*time.Millisecond))).Bg(c.Theme.Primary.Base)
				s := Div(c, layout.Size(200, 60)).Bg(c.Theme.Primary.Hover)
				*sibID = s.ID()
			})
			Div(c, layout.Size(100, 40)).Bg(c.Theme.Primary.Active)
		})
		c.Damage(smallDamage)
	}
}

func TestRunnerAutoDamageSiblingShift(t *testing.T) {
	tall := false
	var boxID, sibID ID
	g := newDamageRig(t, siblingShiftPage(&tall, &boxID, &sibID), newWin(), RunnerOptions{})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(sibID)
	tall = true
	g.now = g.now.Add(100 * time.Millisecond)
	g.frame(t) // target changes: still at 20, now in flight
	g.now = g.now.Add(100 * time.Millisecond)
	// The Runner redraws a frame whole when the clock moved after an
	// animating frame; drop that rule for this frame to see the hints of a
	// frame in which the animation moves the layout.
	g.r.dmg.timed = false
	g.frame(t)
	cur, _ := g.r.Context().RectOf(sibID)
	if cur.Y <= old.Y {
		t.Fatalf("sibling did not move: %v -> %v", old, cur)
	}
	d, s := hints(g.r)
	if len(d)+len(s) == 0 {
		t.Fatal("frame drawn whole")
	}
	if !covered(d, old) || !covered(d, cur) {
		t.Fatalf("damage %v misses the shifted sibling (%v -> %v)", d, old, cur)
	}
	if touches(d, geom.Rect{X: 200, Y: 160, W: 100, H: 100}) {
		t.Fatalf("damage %v reaches far from the moving nodes", d)
	}
	// Settled: nothing moves, nothing is damaged besides the declaration.
	g.now = g.now.Add(time.Second)
	g.frame(t)
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 || d[0] != smallDamage {
		t.Fatalf("settled frame damage = %v", d)
	}
}

func TestRunnerAutoDamageSiblingShiftReduceMotion(t *testing.T) {
	tall := false
	var boxID, sibID ID
	g := newDamageRig(t, siblingShiftPage(&tall, &boxID, &sibID), newWin(), RunnerOptions{ReduceMotion: true})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(sibID)
	box, _ := g.r.Context().RectOf(boxID)
	tall = true
	g.now = g.now.Add(100 * time.Millisecond)
	g.frame(t) // jumps to 60 in this frame, not animating
	cur, _ := g.r.Context().RectOf(sibID)
	if cur.Y != old.Y+40 {
		t.Fatalf("sibling %v -> %v, want a 40px jump", old, cur)
	}
	d, s := hints(g.r)
	if len(d)+len(s) == 0 {
		t.Fatal("frame drawn whole")
	}
	if !covered(d, old) || !covered(d, cur) || !covered(d, box) {
		t.Fatalf("damage %v misses the shifted sibling (%v -> %v)", d, old, cur)
	}
	if touches(d, geom.Rect{X: 200, Y: 160, W: 100, H: 100}) {
		t.Fatalf("damage %v reaches far from the moving nodes", d)
	}
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 || d[0] != smallDamage {
		t.Fatalf("settled frame damage = %v", d)
	}
}

// TestRunnerAutoDamageShiftWithAnimation: nodes that move with the
// content of a ScrollShift are left to the copy when an animation in the
// same frame triggers the layout comparison.
func TestRunnerAutoDamageShiftWithAnimation(t *testing.T) {
	off, wide := float32(0), false
	var boxID ID
	page := func(c *Context) {
		Div(c, layout.Size(200, 100)).Clip().Body(func() {
			Div(c, layout.Abs(0, off, layout.Unset, layout.Unset), layout.W(200)).Body(func() {
				for range 20 {
					Div(c, layout.H(20)).Bg(c.Theme.BgContainer)
				}
				c.ScrollShift(geom.Rect{W: 200, H: 100}, geom.Point{Y: -10})
			})
		})
		Gap(c, 50)
		w := float32(40)
		if wide {
			w = 120
		}
		b := Div(c, layout.H(20))
		boxID = b.ID()
		b.Style(layout.W(c.Anim(b.ID(), "w", w, 200*time.Millisecond))).Bg(c.Theme.Primary.Base)
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(boxID)
	off, wide = -10, true
	g.frame(t) // content moves by the declared delta, the box jumps to 120
	dam, sc := hints(g.r)
	if len(sc) != 1 {
		t.Fatalf("scroll regions = %v (damage %v)", sc, dam)
	}
	cur, _ := g.r.Context().RectOf(boxID)
	if !covered(dam, old) || !covered(dam, cur) {
		t.Fatalf("damage %v misses the animated box %v -> %v", dam, old, cur)
	}
	if touches(dam, geom.Rect{Y: 20, W: 150, H: 40}) {
		t.Fatalf("shifted rows redrawn instead of copied: %v", dam)
	}
}

// TestRunnerAutoDamageShiftGhost: a node in scrolled content that an
// animation moves by something else than the shift delta leaves a copy of
// its old pixels at old box + delta; that place is damaged too.
func TestRunnerAutoDamageShiftGhost(t *testing.T) {
	off, moved := float32(0), false
	var blockID ID
	page := func(c *Context) {
		Div(c, layout.Size(200, 200)).Clip().Body(func() {
			Div(c, layout.Abs(0, off, layout.Unset, layout.Unset), layout.W(200)).Body(func() {
				for i := range 20 {
					row := Div(c, layout.H(20)).Bg(c.Theme.BgContainer)
					if i == 5 {
						row.Body(func() {
							x := float32(0)
							if moved {
								x = 150
							}
							b := Div(c, layout.Size(40, 20))
							blockID = b.ID()
							b.Style(layout.Abs(c.Anim(b.ID(), "x", x, 200*time.Millisecond), 0, layout.Unset, layout.Unset)).Bg(c.Theme.Primary.Base)
						})
					}
				}
				c.ScrollShift(geom.Rect{W: 200, H: 200}, geom.Point{Y: -60})
			})
		})
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(blockID)
	if old != (geom.Rect{X: 0, Y: 100, W: 40, H: 20}) {
		t.Fatalf("block at %v", old)
	}
	off, moved = -60, true
	g.frame(t)
	cur, _ := g.r.Context().RectOf(blockID)
	if cur != (geom.Rect{X: 150, Y: 40, W: 40, H: 20}) {
		t.Fatalf("block moved to %v", cur)
	}
	dam, sc := hints(g.r)
	if len(sc) != 1 {
		t.Fatalf("scroll regions = %v (damage %v)", sc, dam)
	}
	ghost := geom.Rect{X: 0, Y: 40, W: 40, H: 20}
	if !covered(dam, old) || !covered(dam, cur) || !covered(dam, ghost) {
		t.Fatalf("damage %v misses old %v, new %v or the copied ghost %v", dam, old, cur, ghost)
	}
}

// shiftGhostPage scrolls 20 rows by -60 when step is set, in a 200×200
// clip. With block, row 5 holds a 40×20 block (removed when step is set);
// with sibling, an absolutely placed 40×20 block over the region at y=100
// moves from x=0 to 150. A box below the region animates its width.
func shiftGhostPage(step *bool, block, sibling bool, id *ID) func(*Context) {
	return func(c *Context) {
		Div(c, layout.Size(200, 200)).Clip().Body(func() {
			off := float32(0)
			if *step {
				off = -60
			}
			Div(c, layout.Abs(0, off, layout.Unset, layout.Unset), layout.W(200)).Body(func() {
				for i := range 20 {
					row := Div(c, layout.H(20)).Bg(c.Theme.BgContainer)
					if i == 5 && block && !*step {
						row.Body(func() {
							b := Div(c, layout.Size(40, 20)).Bg(c.Theme.Primary.Base)
							*id = b.ID()
						})
					}
				}
				c.ScrollShift(geom.Rect{W: 200, H: 200}, geom.Point{Y: -60})
			})
			if sibling {
				x := float32(0)
				if *step {
					x = 150
				}
				b := Div(c, layout.Size(40, 20))
				*id = b.ID()
				b.Style(layout.Abs(c.Anim(b.ID(), "x", x, 200*time.Millisecond), 100, layout.Unset, layout.Unset)).Bg(c.Theme.Primary.Base)
			}
		})
		w := float32(40)
		if *step {
			w = 120
		}
		b := Div(c, layout.H(20))
		b.Style(layout.W(c.Anim(b.ID(), "w", w, 200*time.Millisecond))).Bg(c.Theme.Primary.Base)
	}
}

// TestRunnerAutoDamageShiftGhostVanished: a node removed inside a shift
// region leaves a copy of its old pixels at old box + delta.
func TestRunnerAutoDamageShiftGhostVanished(t *testing.T) {
	step := false
	var id ID
	g := newDamageRig(t, shiftGhostPage(&step, true, false, &id), newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(id)
	if old != (geom.Rect{X: 0, Y: 100, W: 40, H: 20}) {
		t.Fatalf("block at %v", old)
	}
	step = true
	g.frame(t)
	if _, ok := g.r.Context().RectOf(id); ok {
		t.Fatal("block still laid out")
	}
	dam, sc := hints(g.r)
	if len(sc) != 1 {
		t.Fatalf("scroll regions = %v (damage %v)", sc, dam)
	}
	ghost := geom.Rect{X: 0, Y: 40, W: 40, H: 20}
	if !covered(dam, old) || !covered(dam, ghost) {
		t.Fatalf("damage %v misses old %v or the copied ghost %v", dam, old, ghost)
	}
}

// TestRunnerAutoDamageShiftGhostSibling: a node over the region that is not
// part of the scrolled content and moves leaves a ghost at old box + delta.
func TestRunnerAutoDamageShiftGhostSibling(t *testing.T) {
	step := false
	var id ID
	g := newDamageRig(t, shiftGhostPage(&step, false, true, &id), newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	old, _ := g.r.Context().RectOf(id)
	if old != (geom.Rect{X: 0, Y: 100, W: 40, H: 20}) {
		t.Fatalf("block at %v", old)
	}
	step = true
	g.frame(t)
	cur, _ := g.r.Context().RectOf(id)
	if cur != (geom.Rect{X: 150, Y: 100, W: 40, H: 20}) {
		t.Fatalf("block moved to %v", cur)
	}
	dam, sc := hints(g.r)
	if len(sc) != 1 {
		t.Fatalf("scroll regions = %v (damage %v)", sc, dam)
	}
	ghost := geom.Rect{X: 0, Y: 40, W: 40, H: 20}
	if !covered(dam, old) || !covered(dam, cur) || !covered(dam, ghost) {
		t.Fatalf("damage %v misses old %v, new %v or the copied ghost %v", dam, old, cur, ghost)
	}
}

func TestRunnerAutoDamageOverlay(t *testing.T) {
	open := false
	var ovID ID
	page := func(c *Context) {
		if open {
			ovID = Overlay(c, OverlayOptions{Place: ScreenCenter}, func() {
				Div(c, layout.Size(80, 60)).Bg(c.Theme.Primary.Base)
			}).ID()
		}
		c.Damage(smallDamage)
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	open = true
	g.frame(t)
	r, ok := g.r.Context().RectOf(ovID)
	if !ok || r.Empty() {
		t.Fatal("overlay not painted")
	}
	d, _ := hints(g.r)
	if !covered(d, r) {
		t.Fatalf("open: damage %v misses overlay %v", d, r)
	}
	g.frame(t)
	if d, _ := hints(g.r); touches(d, r.Inset(geom.All(1))) {
		t.Fatalf("static overlay damaged again: %v", d)
	}
	open = false
	g.frame(t)
	d, _ = hints(g.r)
	if !covered(d, r) {
		t.Fatalf("close: damage %v misses overlay %v", d, r)
	}
}

func TestRunnerAutoDamageFocusRing(t *testing.T) {
	var aID, bID ID
	page := func(c *Context) {
		Row(c, func() {
			aID = Button(c, "A").ID()
			Gap(c, 100)
			bID = Button(c, "B").ID()
		}, layout.Pad(20))
		c.Damage(smallDamage)
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	ra, _ := g.r.Context().RectOf(aID)
	rb, _ := g.r.Context().RectOf(bID)
	g.r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true})
	g.frame(t)
	if g.r.Context().Focused() != aID {
		t.Fatal("Tab did not focus A")
	}
	d, _ := hints(g.r)
	if !covered(d, ra.Expand(2)) {
		t.Fatalf("ring shown: damage %v misses %v", d, ra)
	}
	if touches(d, rb) {
		t.Fatalf("B damaged without change: %v", d)
	}
	g.r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true})
	g.frame(t)
	d, _ = hints(g.r)
	if !covered(d, ra.Expand(2)) || !covered(d, rb.Expand(2)) {
		t.Fatalf("ring moved: damage %v misses %v or %v", d, ra, rb)
	}
	// A pointer press hides the ring (focus stays where the press lands).
	g.r.HandleEvent(input.Pointer{Kind: input.Press, Button: input.ButtonLeft, Pos: geom.Pt(390, 200)})
	g.r.HandleEvent(input.Pointer{Kind: input.Release, Button: input.ButtonLeft, Pos: geom.Pt(390, 200)})
	g.frame(t)
	d, _ = hints(g.r)
	if !covered(d, rb.Expand(2)) {
		t.Fatalf("ring hidden: damage %v misses %v", d, rb)
	}
}

// stickyPage models a table with a sticky header: the rows scroll inside a
// viewport while the header (not a descendant of the scrolled content)
// stays put on top of them.
func stickyPage(shift *geom.Point, headerTop float32) func(*Context) {
	return func(c *Context) {
		StackBox(c, func() {
			Scroll(c, func() {
				for i := 0; i < 30; i++ {
					Div(c, layout.H(20)).Bg(c.Theme.BgContainer)
				}
				if shift != nil {
					c.ScrollShift(geom.Rect{W: 200, H: 100}, *shift)
				}
			}, layout.Size(200, 100))
			Div(c, layout.Abs(0, headerTop, layout.Unset, layout.Unset), layout.Size(200, 20)).Bg(c.Theme.Primary.Base)
		}, layout.Size(200, 100))
	}
}

func TestScrollShiftStickyHeaderDamaged(t *testing.T) {
	d := geom.Point{Y: -10}
	g := newDamageRig(t, stickyPage(&d, 0), newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	dam, sc := hints(g.r)
	r := geom.Rect{W: 200, H: 100}
	if len(sc) != 1 || sc[0].Rect != r || sc[0].Delta != d {
		t.Fatalf("scroll regions = %v", sc)
	}
	header := geom.Rect{W: 200, H: 20}
	if !covered(dam, header) {
		t.Fatalf("damage %v misses the sticky header %v", dam, header)
	}
	exposed := geom.Rect{Y: 90, W: 200, H: 10}
	if !covered(dam, exposed) {
		t.Fatalf("damage %v misses the exposed strip %v", dam, exposed)
	}
	for _, x := range dam {
		if x.Intersect(r) != x {
			t.Fatalf("damage %v leaves the scroll region", x)
		}
	}
	if covered(dam, geom.Rect{Y: 40, W: 150, H: 40}) {
		t.Fatalf("scrolled rows redrawn instead of shifted: %v", dam)
	}
	// Input makes the frame build twice; the shift re-declared by the
	// final pass survives.
	g.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(390, 290)})
	g.frame(t)
	if _, sc := hints(g.r); len(sc) != 1 {
		t.Fatalf("rebuilt frame lost its shift: %v", sc)
	}
	// Downwards: the header's shifted copy lands on the rows below it.
	d = geom.Point{Y: 15}
	g.frame(t)
	dam, _ = hints(g.r)
	if !covered(dam, geom.Rect{Y: 0, W: 200, H: 35}) {
		t.Fatalf("damage %v misses the header's shifted copy", dam)
	}
	if !covered(dam, geom.Rect{W: 200, H: 15}) {
		t.Fatalf("damage %v misses the exposed strip", dam)
	}
}

func TestScrollShiftUncertainFallsBack(t *testing.T) {
	r := geom.Rect{W: 200, H: 100}
	d := geom.Point{Y: -10}
	// The header sticks out of the scroll region: it is neither inside nor
	// outside, so the shift is withdrawn and the whole region redrawn.
	g := newDamageRig(t, stickyPage(&d, -8), newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	dam, sc := hints(g.r)
	if len(sc) != 0 {
		t.Fatalf("uncertain shift kept: %v", sc)
	}
	if !covered(dam, r) {
		t.Fatalf("damage %v does not redraw the region %v", dam, r)
	}
	// Content that was never laid out / painted (culled) cannot be shifted.
	page := func(c *Context) {
		Div(c, layout.Abs(1000, 1000, layout.Unset, layout.Unset), layout.Size(10, 10)).Body(func() {
			c.ScrollShift(r, d)
		})
	}
	g = newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	dam, sc = hints(g.r)
	if len(sc) != 0 || !covered(dam, r) {
		t.Fatalf("culled content: scroll %v damage %v", sc, dam)
	}
	// An overlay crossing the region edge is uncertain too.
	page = func(c *Context) {
		stickyPage(&d, 0)(c)
		Overlay(c, OverlayOptions{Place: ScreenFill}, func() {}, layout.Size(50, 50))
	}
	g = newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	dam, sc = hints(g.r)
	if len(sc) != 0 || !covered(dam, r) {
		t.Fatalf("overlay across region: scroll %v damage %v", sc, dam)
	}
}

func TestRunnerFullFrameTriggers(t *testing.T) {
	page := func(c *Context) {
		Div(c, layout.Size(100, 40)).Bg(c.Theme.Primary.Base)
		c.Damage(smallDamage)
	}
	w := newWin()
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("baseline damage = %v", d)
	}
	full := func(name string) {
		t.Helper()
		g.frame(t)
		if d, s := hints(g.r); len(d)+len(s) != 0 {
			t.Fatalf("%s: wrote partial hints %v %v", name, d, s)
		}
		g.frame(t)
		if d, _ := hints(g.r); len(d) != 1 {
			t.Fatalf("%s: next frame damage = %v", name, d)
		}
	}
	g.r.SetReduceMotion(true)
	full("reduce motion")
	g.r.SetTheme(theme.AntDesign())
	full("theme")
	w.size = geom.Size{W: 500, H: 300}
	full("resize")
	w.scale = 2
	full("scale")
}

func TestRunnerUsesDamagePresenter(t *testing.T) {
	declare := true
	page := func(c *Context) {
		Div(c, layout.Size(100, 40)).Bg(c.Theme.Primary.Base)
		if declare {
			c.Damage(geom.Rect{X: 10.5, Y: 10, W: 20, H: 20})
		}
	}
	w := &damagePresenterWin{damageWin: damageWin{size: geom.Size{W: 400, H: 300}, scale: 2}}
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t) // first frame: full
	if w.presented != 1 || w.partial != 0 {
		t.Fatalf("first frame: present %d partial %d", w.presented, w.partial)
	}
	g.frame(t)
	if w.presented != 1 || w.partial != 1 {
		t.Fatalf("damaged frame: present %d partial %d", w.presented, w.partial)
	}
	if len(w.dirty) != 1 || w.dirty[0] != image.Rect(21, 20, 61, 60) {
		t.Fatalf("dirty = %v", w.dirty)
	}
	declare = false
	g.frame(t)
	if w.presented != 2 || w.partial != 1 {
		t.Fatalf("undeclared frame: present %d partial %d", w.presented, w.partial)
	}
	// GPU renderers never get PresentDamage (the hints travel in the List).
	gpu := &damagePresenterWin{damageWin: damageWin{size: geom.Size{W: 400, H: 300}, scale: 1}}
	declare = true
	g2 := newDamageRig(t, page, gpu, RunnerOptions{NoAnimations: true, Renderer: nopRenderer{}})
	g2.frame(t)
	g2.frame(t)
	if gpu.partial != 0 || gpu.presented != 2 {
		t.Fatalf("gpu path: present %d partial %d", gpu.presented, gpu.partial)
	}
}

type nopRenderer struct{}

func (nopRenderer) Render(*paint.List, geom.Size, float32) error { return nil }

func TestRunnerWithoutDamageUnchanged(t *testing.T) {
	open := false
	page := func(c *Context) {
		b := Div(c, layout.H(20))
		target := float32(40)
		if open {
			target = 120
			Overlay(c, OverlayOptions{Place: ScreenCenter}, func() {
				Div(c, layout.Size(80, 60)).Bg(c.Theme.Primary.Base)
			})
		}
		b.Style(layout.W(c.Anim(b.ID(), "w", target, 200*time.Millisecond))).Bg(c.Theme.Primary.Base)
		Button(c, "A")
	}
	w := &damagePresenterWin{damageWin: damageWin{size: geom.Size{W: 400, H: 300}, scale: 1}}
	g := newDamageRig(t, page, w, RunnerOptions{})
	for i := 0; i < 6; i++ {
		if i == 2 {
			open = true
			g.r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true})
		}
		g.now = g.now.Add(50 * time.Millisecond)
		g.frame(t)
		if d, s := hints(g.r); len(d)+len(s) != 0 {
			t.Fatalf("frame %d: hints without declarations %v %v", i, d, s)
		}
	}
	if w.partial != 0 || w.presented != 6 {
		t.Fatalf("present %d partial %d", w.presented, w.partial)
	}
}

// A focused text field blinks its caret through WakeAt timers the Runner
// does not track: once another widget declares damage, the frame at the
// blink must still be redrawn whole.
func TestRunnerCaretBlinkFullFrame(t *testing.T) {
	val := "hello"
	var inID ID
	page := func(c *Context) {
		inID = Input(c, &val).ID()
		c.Damage(smallDamage) // some other widget declares damage every frame
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.r.Context().SetFocus(inID)
	g.frame(t)
	g.frame(t)
	if !g.r.c.wakeAt.After(g.now) {
		t.Fatal("focused input did not request a caret blink")
	}
	if d, _ := hints(g.r); len(d) != 1 || d[0] != smallDamage {
		t.Fatalf("frame before the blink: damage %v", d)
	}
	g.now = g.r.c.wakeAt // the host wakes us for the blink
	g.frame(t)
	if d, s := hints(g.r); len(d)+len(s) != 0 {
		t.Fatalf("blink frame wrote partial hints %v %v (caret would freeze)", d, s)
	}
	g.now = g.now.Add(time.Millisecond) // early frame: timer not due yet
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("frame before the next blink: damage %v", d)
	}
}

type failWin struct {
	damagePresenterWin
	fail bool
}

func (w *failWin) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	w.partial++
	if w.fail {
		return errPresent
	}
	return nil
}

var errPresent = errors.New("present failed")

// After a failed Render / Present the screen does not show the frame the
// damage state was advanced to: the next frame must be full.
func TestRunnerDamageAfterPresentError(t *testing.T) {
	page := func(c *Context) { c.Damage(smallDamage) }
	w := &failWin{damagePresenterWin: damagePresenterWin{damageWin: damageWin{size: geom.Size{W: 400, H: 300}, scale: 1}}}
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t)
	w.fail = true
	if err := g.r.Frame(); err != errPresent {
		t.Fatalf("err = %v", err)
	}
	w.fail = false
	presented := w.presented
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 0 || w.presented != presented+1 {
		t.Fatalf("frame after failure: damage %v, present %d -> %d", d, presented, w.presented)
	}
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("recovered frame: damage %v", d)
	}
	// Renderer errors too.
	fr := &failRenderer{}
	g2 := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true, Renderer: fr})
	g2.frame(t)
	g2.frame(t)
	fr.fail = true
	if err := g2.r.Frame(); err == nil {
		t.Fatal("render error swallowed")
	}
	fr.fail = false
	g2.frame(t)
	if d, _ := hints(g2.r); len(d) != 0 {
		t.Fatalf("frame after render failure: damage %v", d)
	}
}

type failRenderer struct{ fail bool }

func (f *failRenderer) Render(*paint.List, geom.Size, float32) error {
	if f.fail {
		return errPresent
	}
	return nil
}
