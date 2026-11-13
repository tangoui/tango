package mobile

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
)

// Gesture thresholds.
const (
	TouchSlop        = 8                      // px a finger may wander before a tap becomes a drag
	LongPressDelay   = 500 * time.Millisecond // hold time of a long press
	SwipeMinVelocity = 250                    // px/s for a release to count as a swipe (flick)
	SwipeMinDistance = 30                     // px of travel that counts as a swipe at any speed
	velocityWindow   = 100 * time.Millisecond // samples considered for the release velocity
)

// Direction is the dominant direction of a movement.
type Direction uint8

const (
	DirNone Direction = iota
	DirLeft
	DirRight
	DirUp
	DirDown
)

func (d Direction) String() string {
	return [...]string{"none", "left", "right", "up", "down"}[d]
}

// Horizontal reports whether d is left or right.
func (d Direction) Horizontal() bool { return d == DirLeft || d == DirRight }

// DirectionOf returns the dominant direction of v.
func DirectionOf(v geom.Point) Direction {
	switch {
	case v.X == 0 && v.Y == 0:
		return DirNone
	case math.Abs(float64(v.X)) >= math.Abs(float64(v.Y)):
		if v.X < 0 {
			return DirLeft
		}
		return DirRight
	case v.Y < 0:
		return DirUp
	}
	return DirDown
}

// Sample is one pointer position at a time.
type Sample struct {
	Pos geom.Point
	T   time.Time
}

// Track records one finger from down to up and classifies it. It is pure
// (no Context): feed Begin / Move / End with timestamps.
type Track struct {
	Start, Last Sample
	samples     []Sample // recent samples for velocity
	Moved       bool     // travelled more than TouchSlop
	Ended       bool
	LongFired   bool
}

// Begin starts the track at s.
func (t *Track) Begin(s Sample) {
	*t = Track{Start: s, Last: s, samples: append(t.samples[:0], s)}
}

// Move adds a sample; it reports whether this move crossed the slop.
func (t *Track) Move(s Sample) (crossed bool) {
	t.Last = s
	t.samples = append(t.samples, s)
	if len(t.samples) > 32 {
		t.samples = append(t.samples[:0], t.samples[len(t.samples)-16:]...)
	}
	if !t.Moved && dist(s.Pos, t.Start.Pos) > TouchSlop {
		t.Moved = true
		return true
	}
	return false
}

// End finishes the track at s.
func (t *Track) End(s Sample) {
	if s.Pos != t.Last.Pos || s.T != t.Last.T {
		t.Move(s)
	}
	t.Ended = true
}

// Delta is the travel from the start.
func (t *Track) Delta() geom.Point { return t.Last.Pos.Sub(t.Start.Pos) }

// Velocity is the release velocity (px/s) over the last 100 ms of
// samples; zero when the finger rested or time did not advance.
func (t *Track) Velocity() geom.Point {
	if len(t.samples) < 2 {
		return geom.Point{}
	}
	last := t.samples[len(t.samples)-1]
	first := last
	for i := len(t.samples) - 2; i >= 0; i-- {
		s := t.samples[i]
		if last.T.Sub(s.T) > velocityWindow {
			break
		}
		first = s
	}
	dt := last.T.Sub(first.T).Seconds()
	if dt <= 0 {
		return geom.Point{}
	}
	return geom.Pt(float32(float64(last.Pos.X-first.Pos.X)/dt), float32(float64(last.Pos.Y-first.Pos.Y)/dt))
}

// Held is how long the finger has been down at now.
func (t *Track) Held(now time.Time) time.Duration { return now.Sub(t.Start.T) }

// IsTap reports whether an ended track was a tap: short travel and not
// turned into a long press.
func (t *Track) IsTap() bool { return t.Ended && !t.Moved && !t.LongFired }

// Swipe classifies an ended track as a swipe: fast enough or far enough
// along its dominant direction.
func (t *Track) Swipe() (Swipe, bool) {
	if !t.Ended || !t.Moved {
		return Swipe{}, false
	}
	v := t.Velocity()
	d := t.Delta()
	dir := DirectionOf(d)
	speed := axisOf(v, dir)
	travel := axisOf(d, dir)
	if speed >= SwipeMinVelocity || travel >= SwipeMinDistance {
		return Swipe{Dir: dir, Velocity: v, Distance: travel, Speed: speed}, true
	}
	return Swipe{}, false
}

// axisOf returns the component of v along dir (positive when moving that way).
func axisOf(v geom.Point, dir Direction) float32 {
	switch dir {
	case DirLeft:
		return -v.X
	case DirRight:
		return v.X
	case DirUp:
		return -v.Y
	case DirDown:
		return v.Y
	}
	return 0
}

// Swipe is a quick directional movement.
type Swipe struct {
	Dir      Direction
	Velocity geom.Point // px/s
	Speed    float32    // px/s along Dir
	Distance float32    // px along Dir
}

func dist(a, b geom.Point) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

// ---------------------------------------------------------------------------
// Pinch (multi-touch)

// TouchPoint is one finger of a multi-touch frame.
type TouchPoint struct {
	ID  int64
	Pos geom.Point
}

// Pinch is the state of a two-finger gesture relative to its start.
type Pinch struct {
	Scale    float32    // current finger distance / start distance
	Rotation float64    // radians, positive clockwise
	Center   geom.Point // midpoint of the fingers
	Pan      geom.Point // movement of the midpoint since the start
	Active   bool
}

// PinchRecognizer turns two-finger frames into scale / rotation / pan.
// Gesture areas feed it from the ui layer's touch events (Gesture.Pinch);
// custom code can call Update with the fingers of a ui.Box.Touches
// stream.
type PinchRecognizer struct {
	ids      [2]int64
	d0, a0   float64
	c0       geom.Point
	tracking bool
	cur      Pinch
}

// Update consumes the current set of touching fingers and returns the
// pinch state (Active false with fewer than two fingers).
func (r *PinchRecognizer) Update(pts []TouchPoint) Pinch {
	if len(pts) < 2 {
		r.tracking = false
		r.cur = Pinch{}
		return r.cur
	}
	a, b := pts[0], pts[1]
	if r.tracking {
		// Keep following the same two fingers when possible.
		var fa, fb *TouchPoint
		for i := range pts {
			switch pts[i].ID {
			case r.ids[0]:
				fa = &pts[i]
			case r.ids[1]:
				fb = &pts[i]
			}
		}
		if fa == nil || fb == nil {
			r.tracking = false
		} else {
			a, b = *fa, *fb
		}
	}
	d := math.Hypot(float64(b.Pos.X-a.Pos.X), float64(b.Pos.Y-a.Pos.Y))
	ang := math.Atan2(float64(b.Pos.Y-a.Pos.Y), float64(b.Pos.X-a.Pos.X))
	c := geom.Pt((a.Pos.X+b.Pos.X)/2, (a.Pos.Y+b.Pos.Y)/2)
	if !r.tracking {
		r.tracking, r.ids, r.d0, r.a0, r.c0 = true, [2]int64{a.ID, b.ID}, max(d, 1), ang, c
	}
	rot := ang - r.a0
	for rot > math.Pi {
		rot -= 2 * math.Pi
	}
	for rot < -math.Pi {
		rot += 2 * math.Pi
	}
	r.cur = Pinch{Scale: float32(d / r.d0), Rotation: rot, Center: c, Pan: c.Sub(r.c0), Active: true}
	return r.cur
}
