package mobile

import (
	"slices"
	"strconv"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// The gesture arena turns the ui layer's single pointer into touch
// gestures for mobile widgets.
//
// The ui input layer reports one pointer and gives the press to the
// top-most hit widget; there is no event bubbling. Mobile widgets
// therefore split into two kinds:
//
//   - gesture areas (scroll views, swipe cells, swipers, picker wheels…)
//     register a hit on their own box and capture every press that lands
//     on them or on non-interactive content inside;
//   - touchables (cells, buttons, grid items…) register a hit only when
//     they are not inside an area. Inside an area they have no hit; the
//     arena resolves taps and long presses to the deepest touchable under
//     the finger, so a tap on a row inside a scroll view still reaches the
//     row while a drag on the same row scrolls.
//
// Once a finger moves past TouchSlop the arena picks the owner of the
// drag: walking from the innermost area outwards, the first area whose
// axis accepts the movement direction wins (a horizontal swipe on a swipe
// cell opens it, a vertical one scrolls the list around it).
//
// Plain ui widgets inside an area (ui.Input, ui.Switch) keep their own
// hits and work as usual. Every area also declares its axes to the ui
// layer's pointer takeover (ui.Box.TakePointer): a drag that starts on
// such a widget and moves along an area's axis is handed to the area,
// which then scrolls or swipes, and the widget sees the press cancelled.
//
// Areas also receive the ui layer's touch events (ui.Box.Touches) and
// report two-finger gestures as Pinch; single fingers keep arriving as
// the pointer events the ui layer synthesizes from them.

// Axis is the set of drag directions an area accepts.
type Axis uint8

const (
	AxisNone       Axis = 0
	AxisHorizontal Axis = 1 << 0
	AxisVertical   Axis = 1 << 1
	AxisBoth            = AxisHorizontal | AxisVertical
)

type entry struct {
	id, parent ui.ID
	area       bool
	axis       Axis
	accept     func(Direction) bool
	tappable   bool
	reg        uint64
}

type touch struct {
	id          ui.ID // widget holding the pointer capture
	seq         uint64
	tr          Track
	owner       ui.ID // area that claimed the drag
	claim       geom.Point
	dir         Direction
	pressTarget ui.ID
	tapped      bool
	tapTarget   ui.ID
	longTarget  ui.ID
}

type consumer struct {
	start, began, ended, tap, long uint64
	pinch                          PinchRecognizer
	fingers                        []TouchPoint // touches bound to the area that are down
}

type arena struct {
	entries map[ui.ID]*entry
	stack   []ui.ID
	t       touch
	has     bool // t is valid
	seq     uint64
	regs    uint64
	seen    map[ui.ID]*consumer
}

var keyArena = ui.ID(0x6d6f62696c650002)

func arenaOf(c *ui.Context) *arena {
	a := ui.State(c, keyArena, func() arena {
		return arena{entries: map[ui.ID]*entry{}, seen: map[ui.ID]*consumer{}}
	})
	a.sync(c)
	return a
}

func (a *arena) top() ui.ID {
	if n := len(a.stack); n > 0 {
		return a.stack[n-1]
	}
	return 0
}

func (a *arena) consumer(id ui.ID) *consumer {
	k := a.seen[id]
	if k == nil {
		if len(a.seen) > 1024 {
			clear(a.seen)
		}
		k = &consumer{}
		a.seen[id] = k
	}
	return k
}

func (a *arena) register(id, parent ui.ID, area, tappable bool, axis Axis, accept func(Direction) bool) *entry {
	a.regs++
	e := a.entries[id]
	if e == nil {
		if len(a.entries) > 2048 {
			for k, v := range a.entries {
				if a.regs-v.reg > 4096 {
					delete(a.entries, k)
				}
			}
		}
		e = &entry{id: id}
		a.entries[id] = e
	}
	e.parent, e.area, e.tappable, e.axis, e.accept, e.reg = parent, area, tappable, axis, accept, a.regs
	return e
}

// observe detects a press captured by id (which registered a hit) and
// presses released within the same frame.
func (a *arena) observe(c *ui.Context, id ui.ID, ia ui.Interaction) {
	now := c.Now()
	p, _ := c.Pointer()
	if c.Dragging(id) && (!a.has || a.t.tr.Ended || a.t.id != id) {
		if a.has && !a.t.tr.Ended {
			a.end(c, now)
		}
		from, taken := c.PointerTaken(id)
		if !taken {
			a.begin(c, id, Sample{p, now})
			return
		}
		// Taken over from a plain widget inside (a drag that started on a
		// ui.Input): the finger went down at from and has already moved
		// past the slop, so the drag is claimed in this pass.
		a.begin(c, id, Sample{from, now})
		a.sync(c)
		return
	}
	if ia.PressedNow && ia.Clicked && !c.Dragging(id) {
		// Pressed and released between two frames: an instant tap.
		a.begin(c, id, Sample{p, now})
		a.end(c, now)
		c.Invalidate() // let widgets built earlier in this pass see it
	}
}

func (a *arena) begin(c *ui.Context, id ui.ID, s Sample) {
	a.seq++
	a.t = touch{id: id, seq: a.seq}
	a.t.tr.Begin(s)
	a.has = true
	a.t.pressTarget = a.resolve(c, s.Pos)
}

func (a *arena) end(c *ui.Context, now time.Time) {
	t := &a.t
	p, _ := c.Pointer()
	t.tr.End(Sample{p, now})
	if t.tr.IsTap() {
		t.tapped = true
		t.tapTarget = a.resolve(c, t.tr.Start.Pos)
	}
}

// sync updates the current touch from the pointer: movement, slop
// crossing (drag ownership), release and long press. It is idempotent
// within a frame.
func (a *arena) sync(c *ui.Context) {
	if !a.has || a.t.tr.Ended {
		return
	}
	t := &a.t
	now := c.Now()
	if !c.Dragging(t.id) {
		a.end(c, now)
		return
	}
	p, _ := c.Pointer()
	if p != t.tr.Last.Pos {
		if t.tr.Move(Sample{p, now}) && !t.tr.LongFired {
			t.dir = DirectionOf(t.tr.Delta())
			t.claim = p
			t.owner = a.claimOwner(t.id, t.dir)
		}
	}
	if !t.tr.Moved && !t.tr.LongFired {
		if t.tr.Held(now) >= LongPressDelay {
			t.tr.LongFired = true
			t.longTarget = a.resolve(c, t.tr.Start.Pos)
		} else {
			c.WakeAt(t.tr.Start.T.Add(LongPressDelay))
		}
	}
}

func (a *arena) claimOwner(from ui.ID, dir Direction) ui.ID {
	e := a.entries[from]
	for depth := 0; e != nil && depth < 64; depth++ {
		if e.area {
			ok := (dir.Horizontal() && e.axis&AxisHorizontal != 0) || (!dir.Horizontal() && e.axis&AxisVertical != 0)
			if ok && e.accept != nil {
				ok = e.accept(dir)
			}
			if ok {
				return e.id
			}
		}
		e = a.entries[e.parent]
	}
	return 0
}

// within reports whether id is root or nested inside root.
func (a *arena) within(id, root ui.ID) bool {
	e := a.entries[id]
	for depth := 0; e != nil && depth < 64; depth++ {
		if e.id == root {
			return true
		}
		e = a.entries[e.parent]
	}
	return false
}

// resolve returns the deepest touchable under p inside the captured
// widget (the smallest rectangle wins), or the captured widget itself.
func (a *arena) resolve(c *ui.Context, p geom.Point) ui.ID {
	root := a.t.id
	var best ui.ID
	bestArea := float32(-1)
	for _, e := range a.entries {
		if !e.tappable || !a.within(e.id, root) {
			continue
		}
		r, ok := c.RectOf(e.id)
		if !ok || !r.Contains(p) {
			continue
		}
		if ar := r.W * r.H; bestArea < 0 || ar < bestArea {
			best, bestArea = e.id, ar
		}
	}
	if best == 0 {
		if e := a.entries[root]; e != nil && e.tappable {
			return root
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Touchable

// Press is the per-frame touch state of a touchable widget.
type Press struct {
	Pressed   bool       // a finger is down on it and has not moved away
	PressPos  geom.Point // where the finger went down
	Tap       bool       // tapped this frame
	TapPos    geom.Point
	LongPress bool // held for LongPressDelay this frame
	LongPos   geom.Point
	// KeyTap reports that Tap came from the keyboard (Enter / Space on the
	// focused widget) or a screen reader (Press action); TapPos is then
	// the centre of the widget.
	KeyTap bool
}

// touchMode selects how a touchable takes part in keyboard navigation.
type touchMode uint8

const (
	touchFocus   touchMode = iota // a Tab stop (outside gesture areas)
	touchPointer                  // pointer target only; keyboard access is provided elsewhere
	touchPassive                  // swallows taps without looking clickable (masks, form rows)
)

// Touchable registers box b as a tappable widget and returns its touch
// state for this frame. Call it once per frame per widget. Inside a
// gesture area b gets no ui hit (the area captures the finger and the
// arena forwards taps); elsewhere b becomes hit-testable and a keyboard
// focus stop (Enter / Space tap it). In both cases a screen reader's Press
// action on b reports a Tap, so give b a role (b.Semantics) to expose it.
func Touchable(c *ui.Context, b *ui.Box) Press { return touchAs(c, b, touchFocus) }

func touchAs(c *ui.Context, b *ui.Box, mode touchMode) Press {
	a := arenaOf(c)
	id := b.ID()
	parent := a.top()
	a.register(id, parent, false, true, AxisNone, nil)
	var pr Press
	key := false
	if parent == 0 {
		var ia ui.Interaction
		switch mode {
		case touchFocus:
			ia = b.Focusable(input.CursorPointer)
			// Pointer presses clear FocusVisible: a visible-focus click is
			// Enter / Space.
			key = ia.Clicked && ia.FocusVisible
			if ia.FocusVisible {
				b.PaintOver(focusRing(ThemeOf(c)))
			}
		case touchPointer:
			ia = b.Interact(input.CursorPointer)
		default:
			ia = b.Interact(input.CursorDefault)
		}
		a.observe(c, id, ia)
	} else if c.A11yEnabled() {
		b.A11y(pressSem) // no ui hit: offer the Press action ourselves
	}
	if act, _, ok := c.A11yAction(id); ok && act == a11y.Press {
		key = true
	}
	if key {
		pr.Tap, pr.KeyTap = true, true
		if r, ok := c.RectOf(id); ok {
			pr.TapPos = r.Center()
		}
	}
	if !a.has {
		return pr
	}
	t := &a.t
	k := a.consumer(id)
	if !t.tr.Ended && !t.tr.Moved && !t.tr.LongFired && t.pressTarget == id {
		pr.Pressed, pr.PressPos = true, t.tr.Start.Pos
	}
	if t.tapped && t.tapTarget == id && k.tap < t.seq {
		k.tap = t.seq
		pr.Tap, pr.TapPos, pr.KeyTap = true, t.tr.Start.Pos, false
	}
	if t.tr.LongFired && t.longTarget == id && k.long < t.seq {
		k.long = t.seq
		pr.LongPress, pr.LongPos = true, t.tr.Start.Pos
	}
	return pr
}

// sink makes b swallow presses (dialog panels over a tappable mask).
func sink(c *ui.Context, b *ui.Box) {
	a := arenaOf(c)
	a.register(b.ID(), a.top(), false, false, AxisNone, nil)
	if a.top() == 0 {
		b.Interact(input.CursorDefault)
	}
}

// ---------------------------------------------------------------------------
// Gesture areas

// Pan is the drag state of a gesture area that owns the current drag.
type Pan struct {
	Active   bool       // the area owns a drag in progress
	Began    bool       // ownership started this frame
	Ended    bool       // the finger lifted this frame
	Delta    geom.Point // travel since the drag was claimed
	Velocity geom.Point // px/s (release velocity when Ended)
	Dir      Direction  // direction at claim time
}

// Gesture is the per-frame gesture state of an area.
type Gesture struct {
	ID         ui.ID
	Pan        Pan
	Tap        bool // a tap landed inside the area (bubbles from touchables)
	TapPos     geom.Point
	LongPress  bool
	LongPos    geom.Point
	TouchStart bool // a finger went down inside the area this frame
	Outside    bool // a finger went down outside the area this frame
	Swipe      Swipe
	Swiped     bool // the drag owned by this area ended as a swipe
	Pinch      Pinch
}

// Area makes box b a gesture area accepting drags along axis (accept, if
// non-nil, can refuse a direction at claim time) and returns its state.
// Build the area's content with Within so nested touchables and areas
// attach to it.
func Area(c *ui.Context, b *ui.Box, axis Axis, accept func(Direction) bool) Gesture {
	a := arenaOf(c)
	id := b.ID()
	parent := a.top()
	a.register(id, parent, true, false, axis, accept)
	b.TakePointer(takeAxes(axis, accept))
	ia := b.Interact(input.CursorDefault)
	a.observe(c, id, ia)
	g := Gesture{ID: id}
	g.Pinch = a.pinch(c, b)
	if !a.has {
		return g
	}
	t := &a.t
	k := a.consumer(id)
	inside := a.within(t.id, id)
	if k.start < t.seq {
		k.start = t.seq
		g.TouchStart, g.Outside = inside, !inside
	}
	if t.owner == id {
		g.Pan.Active = !t.tr.Ended
		g.Pan.Delta = t.tr.Last.Pos.Sub(t.claim)
		g.Pan.Dir = t.dir
		g.Pan.Velocity = t.tr.Velocity()
		if k.began < t.seq {
			k.began = t.seq
			g.Pan.Began = true
		}
		if t.tr.Ended && k.ended < t.seq {
			k.ended = t.seq
			g.Pan.Ended = true
			g.Swipe, g.Swiped = t.tr.Swipe()
		}
	}
	if t.tapped && inside && k.tap < t.seq {
		k.tap = t.seq
		g.Tap, g.TapPos = true, t.tr.Start.Pos
	}
	if t.tr.LongFired && inside && k.long < t.seq {
		k.long = t.seq
		g.LongPress, g.LongPos = true, t.tr.Start.Pos
	}
	return g
}

// Within builds fn as the content of area id: touchables and areas
// created inside attach to it.
func Within(c *ui.Context, id ui.ID, fn func()) {
	a := arenaOf(c)
	a.stack = append(a.stack, id)
	fn()
	a.stack = a.stack[:len(a.stack)-1]
}

// detached builds fn outside any area (overlay layers).
func detached(c *ui.Context, fn func()) {
	Within(c, 0, fn)
}

// takeAxes is the part of axis that accept currently allows, as the ui
// layer's takeover axes.
func takeAxes(axis Axis, accept func(Direction) bool) ui.PanAxis {
	ok := func(a, b Direction) bool { return accept == nil || accept(a) || accept(b) }
	var p ui.PanAxis
	if axis&AxisHorizontal != 0 && ok(DirLeft, DirRight) {
		p |= ui.PanX
	}
	if axis&AxisVertical != 0 && ok(DirUp, DirDown) {
		p |= ui.PanY
	}
	return p
}

// ---------------------------------------------------------------------------
// Multi-touch

// pinch follows the fingers bound to area b (ui touch events) and returns
// the two-finger gesture state.
func (a *arena) pinch(c *ui.Context, b *ui.Box) Pinch {
	evs := b.Touches()
	k := a.seen[b.ID()]
	if k == nil {
		if len(evs) == 0 {
			return Pinch{}
		}
		k = a.consumer(b.ID())
	}
	if c.TouchCount() == 0 && len(evs) == 0 {
		k.fingers = k.fingers[:0] // lost an end event (area rebuilt mid-gesture)
	}
	for _, e := range evs {
		i := slices.IndexFunc(k.fingers, func(f TouchPoint) bool { return f.ID == e.ID })
		switch {
		case e.Done():
			if i >= 0 {
				k.fingers = slices.Delete(k.fingers, i, i+1)
			}
		case i >= 0:
			k.fingers[i].Pos = e.Pos
		default:
			k.fingers = append(k.fingers, TouchPoint{ID: e.ID, Pos: e.Pos})
		}
	}
	if len(k.fingers) < 2 {
		if k.pinch.tracking {
			k.pinch.Update(nil)
		}
		return Pinch{}
	}
	return k.pinch.Update(k.fingers)
}

// ---------------------------------------------------------------------------
// Frame pump

// pump keeps frames coming while a physics motion runs (generation gen of
// widget id). It returns false when the runner has animations disabled
// (tests, screenshots): callers then jump to the end state.
func pump(c *ui.Context, id ui.ID, gen uint64) bool {
	return c.AnimFrom(id, "mobile.pump."+strconv.FormatUint(gen, 10), 0, 1, time.Hour) < 1
}
