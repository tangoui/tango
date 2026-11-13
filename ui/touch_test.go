package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// touchPage has a tappable box at (0,0,100,40) and a touch-observing pad
// below it at (0,40,300,200) with its own tappable child at (0,40,100,40).
type touchPage struct {
	clicks, padClicks, childClicks int
	evs                            []input.TouchEvent
	raw                            bool
}

func (p *touchPage) build(c *Context) {
	b := Div(c, layout.Size(100, 40))
	if b.Interact(input.CursorPointer).Clicked {
		p.clicks++
	}
	pad := Div(c, layout.Size(300, 200))
	if p.raw {
		pad.RawTouch()
	}
	p.evs = append(p.evs, pad.Touches()...)
	pad.Body(func() {
		k := Div(c, layout.Size(100, 40))
		if k.Interact(input.CursorPointer).Clicked {
			p.childClicks++
		}
	})
}

func TestTouchEmulatesPointer(t *testing.T) {
	p := &touchPage{}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Tap(geom.Pt(50, 20))
	if p.clicks != 1 {
		t.Fatalf("tap on a widget without touch handling: clicks=%d", p.clicks)
	}
	if _, in := d.r.c.Pointer(); in {
		t.Fatal("pointer still inside after the finger lifted (stuck hover)")
	}
	// The pad observes touches; its child still gets the emulated click.
	d.Tap(geom.Pt(50, 60))
	if p.childClicks != 1 {
		t.Fatalf("child of an observing box: clicks=%d", p.childClicks)
	}
	if len(p.evs) != 2 || p.evs[0].Phase != input.TouchBegan || p.evs[1].Phase != input.TouchEnded {
		t.Fatalf("pad touches %+v", p.evs)
	}
	// A tap on the first box is not delivered to the pad.
	p.evs = nil
	d.Tap(geom.Pt(50, 20))
	if len(p.evs) != 0 {
		t.Fatalf("pad received a touch outside it: %+v", p.evs)
	}
}

func TestTouchSecondFingerNotEmulated(t *testing.T) {
	p := &touchPage{}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	// Finger 1 rests on the pad background, finger 2 taps the button.
	d.Touch(1, input.TouchBegan, geom.Pt(200, 150))
	d.Touch(2, input.TouchBegan, geom.Pt(50, 20))
	if n := d.r.c.TouchCount(); n != 2 {
		t.Fatalf("TouchCount %d", n)
	}
	d.Touch(2, input.TouchEnded, geom.Pt(50, 20))
	d.Touch(1, input.TouchEnded, geom.Pt(200, 150))
	d.Settle()
	if p.clicks != 0 {
		t.Fatal("second finger was emulated as a click")
	}
	// Both fingers down together, several changes in one frame.
	p.evs = nil
	d.Touches(input.TouchEvent{ID: 7, Phase: input.TouchBegan, Pos: geom.Pt(150, 100), Pressure: 0.5},
		input.TouchEvent{ID: 8, Phase: input.TouchBegan, Pos: geom.Pt(250, 100)})
	if len(p.evs) != 2 || p.evs[0].ID != 7 || p.evs[0].Pressure != 0.5 || p.evs[1].ID != 8 || p.evs[0].Time.IsZero() {
		t.Fatalf("multi-touch frame %+v", p.evs)
	}
	// Fingers stay bound to the pad when they leave it.
	d.Touch(8, input.TouchMoved, geom.Pt(390, 390))
	if last := p.evs[len(p.evs)-1]; last.ID != 8 || last.Phase != input.TouchMoved {
		t.Fatalf("captured finger: %+v", last)
	}
	d.Touches(input.TouchEvent{ID: 7, Phase: input.TouchCancelled}, input.TouchEvent{ID: 8, Phase: input.TouchEnded})
	if d.r.c.TouchCount() != 0 {
		t.Fatal("fingers left over")
	}
}

func TestRawTouchAndEmulationSwitch(t *testing.T) {
	p := &touchPage{raw: true}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Tap(geom.Pt(50, 60))
	if p.childClicks != 0 || len(p.evs) != 2 {
		t.Fatalf("raw touch: child clicks=%d touches=%d", p.childClicks, len(p.evs))
	}
	d.Tap(geom.Pt(50, 20))
	if p.clicks != 1 {
		t.Fatal("raw touch leaked outside its box")
	}
	d.Runner().SetTouchPointerEmulation(false)
	d.Tap(geom.Pt(50, 20))
	if p.clicks != 1 {
		t.Fatal("emulation off still clicked")
	}
}

func TestTouchGestureHelpers(t *testing.T) {
	var evs []input.TouchEvent
	d := NewDriver(func(c *Context) {
		evs = append(evs, Div(c, layout.Size(400, 400)).Touches()...)
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Pinch(geom.Pt(200, 200), 50, 150)
	ids := map[int64]geom.Point{}
	for _, e := range evs {
		ids[e.ID] = e.Pos
	}
	if len(ids) != 2 || ids[driverFinger2].X-ids[driverFinger1].X != 150 {
		t.Fatalf("pinch ends %v", ids)
	}
	evs = nil
	d.TwoFingerPan(geom.Pt(100, 100), geom.Pt(100, 300), 40)
	if e := evs[len(evs)-1]; e.Phase != input.TouchEnded || e.Pos.Y != 300 {
		t.Fatalf("two-finger pan end %+v", e)
	}
}

// takeoverPage: a vertical scroller-like container with a focusable
// widget inside; drags that start on the widget go to the container.
type takeoverPage struct {
	axes                       PanAxis
	keep                       bool
	outer, inner               ID
	taken, cancelled, innerClk int
	outerDrag, innerDrag       bool
	from                       geom.Point
	focusBefore                ID
}

func (p *takeoverPage) build(c *Context) {
	o := Div(c, layout.Size(300, 300)).TakePointer(p.axes)
	o.Interact(input.CursorDefault)
	p.outer = o.ID()
	o.Body(func() {
		Gap(c, 50)
		in := Div(c, layout.Size(200, 40))
		ia := in.Focusable(input.CursorText)
		p.inner = in.ID()
		if ia.Clicked {
			p.innerClk++
		}
		if p.keep {
			c.KeepPointer(in.ID())
		}
		if c.PointerCancelled(in.ID()) {
			p.cancelled++
		}
		p.innerDrag = c.Dragging(in.ID())
	})
	if from, ok := c.PointerTaken(o.ID()); ok {
		p.taken++
		p.from = from
	}
	p.outerDrag = c.Dragging(o.ID())
}

func TestPointerTakeover(t *testing.T) {
	p := &takeoverPage{axes: PanY}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Frame()
	start := geom.Pt(100, 70)
	d.Inject(input.Pointer{Kind: input.Move, Pos: start})
	d.Inject(input.Pointer{Kind: input.Press, Pos: start})
	d.Frame()
	if !p.innerDrag || d.r.c.Focused() != p.inner {
		t.Fatal("press did not reach the inner widget")
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(101, 74)}) // within the slop
	d.Frame()
	if !p.innerDrag || p.taken != 0 {
		t.Fatal("taken over within the slop")
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(102, 120)})
	d.Frame()
	if p.taken != 1 || p.cancelled != 1 || !p.outerDrag || p.innerDrag || p.from != start {
		t.Fatalf("takeover: taken=%d cancelled=%d outer=%v inner=%v from=%v", p.taken, p.cancelled, p.outerDrag, p.innerDrag, p.from)
	}
	if d.r.c.Focused() == p.inner {
		t.Fatal("focus given by the cancelled press was kept")
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(100, 70)}) // back over the widget
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(100, 70)})
	d.Settle()
	if p.innerClk != 0 || p.taken != 1 || p.cancelled != 1 {
		t.Fatalf("after release: clicks=%d taken=%d cancelled=%d", p.innerClk, p.taken, p.cancelled)
	}

	// A horizontal drag is not the container's: the widget keeps it (and,
	// released over it, clicks as before).
	d.Drag(start, geom.Pt(180, 72))
	if p.taken != 1 || p.innerClk != 1 {
		t.Fatalf("horizontal drag: taken=%d clicks=%d", p.taken, p.innerClk)
	}
	// A plain click still clicks.
	d.Click(start)
	if p.innerClk != 2 {
		t.Fatalf("click after takeover: %d", p.innerClk)
	}
	// KeepPointer refuses the takeover.
	p.keep = true
	d.Drag(start, geom.Pt(100, 200))
	if p.taken != 1 {
		t.Fatal("KeepPointer ignored")
	}
	// Without declarations nothing changes (default behaviour).
	p.keep, p.axes = false, 0
	d.Drag(start, geom.Pt(100, 200))
	if p.taken != 1 {
		t.Fatal("container without TakePointer took the pointer")
	}
}

func TestPointerTakeoverByTouch(t *testing.T) {
	p := &takeoverPage{axes: PanXY}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.TouchDrag(geom.Pt(100, 70), geom.Pt(160, 70))
	if p.taken != 1 || p.cancelled != 1 || p.innerClk != 0 {
		t.Fatalf("touch drag: taken=%d cancelled=%d clicks=%d", p.taken, p.cancelled, p.innerClk)
	}
	d.Tap(geom.Pt(100, 70))
	if p.innerClk != 1 {
		t.Fatal("touch tap on the inner widget")
	}
}

func TestPointerCancelEvent(t *testing.T) {
	p := &takeoverPage{}
	d := NewDriver(p.build, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Touch(1, input.TouchBegan, geom.Pt(100, 70))
	d.Touch(1, input.TouchCancelled, geom.Pt(100, 70))
	if p.cancelled != 1 || p.innerClk != 0 {
		t.Fatalf("system cancel: cancelled=%d clicks=%d", p.cancelled, p.innerClk)
	}
}

type safeWin struct {
	damageWin
	in geom.Insets
}

func (w *safeWin) SafeAreaInsets() geom.Insets { return w.in }

func TestSafeArea(t *testing.T) {
	var got geom.Insets
	page := func(c *Context) {
		got = c.SafeArea()
		Div(c, layout.Size(100, 40))
		c.Damage(smallDamage)
	}
	w := &safeWin{damageWin: *newWin(), in: geom.Insets{Top: 44, Bottom: 34}}
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	if got != w.in {
		t.Fatalf("window insets %v", got)
	}
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("baseline damage %v", d)
	}
	w.in.Top = 20
	g.frame(t)
	if d, s := hints(g.r); got.Top != 20 || len(d)+len(s) != 0 {
		t.Fatalf("change: insets %v, partial hints %v %v", got, d, s)
	}
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("after change damage %v", d)
	}
	g.r.SetSafeArea(geom.Insets{Left: 10})
	g.frame(t)
	if got != (geom.Insets{Left: 10}) {
		t.Fatalf("override %v", got)
	}
	g.r.ClearSafeArea()
	g.frame(t)
	if got.Top != 20 {
		t.Fatalf("cleared override %v", got)
	}

	var dg geom.Insets
	d := NewDriver(func(c *Context) { dg = c.SafeArea() }, DriverOptions{})
	d.SetSafeArea(geom.Insets{Bottom: 34})
	d.Frame()
	if dg.Bottom != 34 {
		t.Fatalf("driver safe area %v", dg)
	}
}
