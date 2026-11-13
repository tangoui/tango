package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Touch and safe-area helpers of the Driver. Gesture helpers advance
// simulated time by touchStep per step so recognizers see realistic
// velocities.

const (
	touchSteps = 8
	touchStep  = 16 * time.Millisecond
)

// Finger IDs used by the gesture helpers.
const (
	driverFinger1 int64 = 1001
	driverFinger2 int64 = 1002
)

// Touch injects one finger change and runs a frame.
func (d *Driver) Touch(id int64, phase input.TouchPhase, p geom.Point) {
	d.Inject(input.TouchEvent{ID: id, Phase: phase, Pos: p, Time: d.w.now})
	d.Frame()
}

// Touches injects several finger changes delivered in the same frame and
// runs it.
func (d *Driver) Touches(evs ...input.TouchEvent) {
	for _, e := range evs {
		if e.Time.IsZero() {
			e.Time = d.w.now
		}
		d.Inject(e)
	}
	d.Frame()
}

// Tap touches p with one finger and lifts it in the same frame, then
// settles.
func (d *Driver) Tap(p geom.Point) {
	d.Inject(input.TouchEvent{ID: driverFinger1, Phase: input.TouchBegan, Pos: p, Time: d.w.now})
	d.Inject(input.TouchEvent{ID: driverFinger1, Phase: input.TouchEnded, Pos: p, Time: d.w.now})
	d.Settle()
}

// TouchDrag puts one finger down at from, moves it to to in a few steps
// (one frame and touchStep of simulated time each) and lifts it, then
// settles.
func (d *Driver) TouchDrag(from, to geom.Point) {
	d.Touch(driverFinger1, input.TouchBegan, from)
	for i := 1; i <= touchSteps; i++ {
		d.Advance(touchStep)
		d.Touch(driverFinger1, input.TouchMoved, lerpPt(from, to, float32(i)/touchSteps))
	}
	d.Touch(driverFinger1, input.TouchEnded, to)
	d.Settle()
}

// Pinch puts two fingers down on a horizontal line through center, from
// apart, moves them symmetrically until they are to apart (to > from
// zooms in) and lifts them, then settles.
func (d *Driver) Pinch(center geom.Point, from, to float32) {
	d.twoFingers(center, center, from, to)
}

// TwoFingerPan puts two fingers down around from (spread px apart), moves
// both to to without changing their distance and lifts them, then
// settles.
func (d *Driver) TwoFingerPan(from, to geom.Point, spread float32) {
	d.twoFingers(from, to, spread, spread)
}

func (d *Driver) twoFingers(c0, c1 geom.Point, s0, s1 float32) {
	at := func(c geom.Point, s float32) (geom.Point, geom.Point) {
		return geom.Pt(c.X-s/2, c.Y), geom.Pt(c.X+s/2, c.Y)
	}
	a, b := at(c0, s0)
	d.Touches(input.TouchEvent{ID: driverFinger1, Phase: input.TouchBegan, Pos: a},
		input.TouchEvent{ID: driverFinger2, Phase: input.TouchBegan, Pos: b})
	for i := 1; i <= touchSteps; i++ {
		k := float32(i) / touchSteps
		d.Advance(touchStep)
		a, b = at(lerpPt(c0, c1, k), s0+(s1-s0)*k)
		d.Touches(input.TouchEvent{ID: driverFinger1, Phase: input.TouchMoved, Pos: a},
			input.TouchEvent{ID: driverFinger2, Phase: input.TouchMoved, Pos: b})
	}
	d.Touches(input.TouchEvent{ID: driverFinger2, Phase: input.TouchEnded, Pos: b},
		input.TouchEvent{ID: driverFinger1, Phase: input.TouchEnded, Pos: a})
	d.Settle()
}

func lerpPt(a, b geom.Point, k float32) geom.Point {
	return geom.Pt(a.X+(b.X-a.X)*k, a.Y+(b.Y-a.Y)*k)
}

// SetSafeArea sets the safe-area insets the page sees (Runner.SetSafeArea);
// the next frame picks them up.
func (d *Driver) SetSafeArea(in geom.Insets) { d.r.SetSafeArea(in) }
