package ui

import (
	"slices"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Multi-touch input.
//
// Hosts report fingers as input.TouchEvent. When a finger goes down the
// Runner routes it, using the last frame's geometry, to every box under it
// that asked for touches (Box.Touches) and is not covered by a widget
// painted above it; the finger stays bound to those boxes until it lifts,
// even when it leaves them. Several fingers can change in one frame.
//
// The first finger of a gesture (no other finger down) is also turned
// into pointer events: Move + Press when it goes down, Move while it
// moves, Release and Leave when it lifts, Cancel when the system cancels
// it. Widgets without touch handling (buttons, inputs, sliders) therefore
// work on touch screens unchanged. Boxes that handle the finger entirely
// by themselves (a drawing canvas, a map) opt out with Box.RawTouch; a host
// that emulates the pointer itself turns emulation off for the whole
// window with Runner.SetTouchPointerEmulation(false).

// touchFinger is a finger that is down.
type touchFinger struct {
	id      int64
	pos     geom.Point
	to      []ID // boxes receiving it
	primary bool // turned into pointer events
}

// routedTouch is a touch event of this frame and its receivers.
type routedTouch struct {
	ev input.TouchEvent
	to []ID
}

type touchState struct {
	live    []touchFinger
	frame   []routedTouch
	noEmu   bool          // SetTouchPointerEmulation(false)
	scratch []input.Event // expandTouches output buffer
}

// SetTouchPointerEmulation turns the pointer events synthesized from the
// first finger of a touch gesture on (the default) or off. Hosts that
// already deliver emulated mouse events for touches turn it off.
func (r *Runner) SetTouchPointerEmulation(on bool) { r.c.ptr.touch.noEmu = !on }

// expandTouches routes the queued touch events (against the last frame's
// hits) and inserts the pointer events synthesized for the primary
// finger right after each one. Other events pass through unchanged.
func (r *Runner) expandTouches(q []input.Event) []input.Event {
	c := r.c
	ts := &c.ptr.touch
	has := false
	for _, e := range q {
		if _, ok := e.(input.TouchEvent); ok {
			has = true
			break
		}
	}
	if !has {
		return q
	}
	out := ts.scratch[:0]
	for _, e := range q {
		ev, ok := e.(input.TouchEvent)
		if !ok {
			out = append(out, e)
			continue
		}
		if ev.Time.IsZero() {
			ev.Time = c.now
		}
		out = append(out, ev)
		i := slices.IndexFunc(ts.live, func(f touchFinger) bool { return f.id == ev.ID })
		if ev.Phase == input.TouchBegan {
			if i >= 0 { // a reused ID without an end: drop the old finger
				ts.live = slices.Delete(ts.live, i, i+1)
			}
			f := touchFinger{id: ev.ID, pos: ev.Pos}
			raw := false
			idx, top := c.hitAt(ev.Pos)
			for k := range c.ptr.regions {
				g := &c.ptr.regions[k]
				if g.flags&(ptrTouch|ptrRawTouch) == 0 || !g.covers(ev.Pos, idx, top) {
					continue
				}
				if g.flags&ptrTouch != 0 {
					f.to = append(f.to, g.id)
				}
				if g.flags&ptrRawTouch != 0 {
					raw = true
				}
			}
			f.primary = !ts.noEmu && !raw && len(ts.live) == 0
			ts.live = append(ts.live, f)
			i = len(ts.live) - 1
		}
		if i < 0 {
			continue // never saw it go down (began before Attach): ignore
		}
		f := &ts.live[i]
		f.pos = ev.Pos
		ts.frame = append(ts.frame, routedTouch{ev: ev, to: f.to})
		if f.primary {
			switch ev.Phase {
			case input.TouchBegan:
				out = append(out, input.Pointer{Kind: input.Move, Pos: ev.Pos, Source: input.SourceTouch}, input.Pointer{Kind: input.Press, Pos: ev.Pos, Source: input.SourceTouch})
			case input.TouchMoved:
				out = append(out, input.Pointer{Kind: input.Move, Pos: ev.Pos, Source: input.SourceTouch})
			case input.TouchEnded:
				out = append(out, input.Pointer{Kind: input.Move, Pos: ev.Pos, Source: input.SourceTouch}, input.Pointer{Kind: input.Release, Pos: ev.Pos, Source: input.SourceTouch},
					input.Pointer{Kind: input.Leave, Pos: ev.Pos, Source: input.SourceTouch})
			case input.TouchCancelled:
				out = append(out, input.Pointer{Kind: input.Cancel, Pos: ev.Pos, Source: input.SourceTouch}, input.Pointer{Kind: input.Leave, Pos: ev.Pos, Source: input.SourceTouch})
			}
		}
		if ev.Done() {
			ts.live = slices.Delete(ts.live, i, i+1)
		}
	}
	// Hand the caller's slice back as the next scratch buffer.
	ts.scratch = q[:0]
	return out
}

// clearTouchFrame forgets the touch events delivered this frame.
func (c *Context) clearTouchFrame() {
	clear(c.ptr.touch.frame)
	c.ptr.touch.frame = c.ptr.touch.frame[:0]
}

// Touches makes b receive touch events and returns those of this frame
// for the fingers bound to b (see the notes on multi-touch above), in the
// order they happened. Call it every frame: fingers are routed by the
// last frame's geometry, so b receives fingers that go down inside it
// from the frame after its first call. Single-finger pointer emulation
// keeps running unless b also sets RawTouch.
func (b *Box) Touches() []input.TouchEvent {
	b.n.ptr |= ptrTouch
	c := b.c
	var out []input.TouchEvent
	for _, t := range c.ptr.touch.frame {
		if slices.Contains(t.to, b.n.id) {
			out = append(out, t.ev)
		}
	}
	return out
}

// RawTouch stops the fingers that go down inside b from being turned into
// pointer events: b handles them through Touches alone (drawing surfaces,
// maps, custom multi-finger gestures).
func (b *Box) RawTouch() *Box { b.n.ptr |= ptrRawTouch | ptrTouch; return b }

// TouchCount returns how many fingers are on the screen.
func (c *Context) TouchCount() int { return len(c.ptr.touch.live) }
