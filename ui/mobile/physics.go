package mobile

import (
	"math"
	"time"
)

// Physics constants of the scroll model.
const (
	// DecelerationRate is the fraction of velocity kept per millisecond of
	// a fling (iOS "normal" deceleration).
	DecelerationRate = 0.998
	// RubberBand is the resistance of dragging past an edge: the content
	// follows by dim·(1 − 1/(d·RubberBand/dim + 1)).
	RubberBand = 0.55
	// SpringOmega is the angular frequency (1/s) of the critically damped
	// spring that pulls an over-scrolled view back to its edge.
	SpringOmega      = 13.0
	minFlingVelocity = 50.0 // px/s; slower releases just stop (or spring back)
	restVelocity     = 12.0 // px/s; below this a fling ends
	restDistance     = 0.4  // px
)

// decayK is the exponential decay constant (1/s) for DecelerationRate.
var decayK = -math.Log(DecelerationRate) * 1000

// Rubber maps an overshoot d (px past the edge, ≥ 0) to the displayed
// overshoot for a viewport of size dim.
func Rubber(d, dim float64) float64 {
	if d <= 0 || dim <= 0 {
		return 0
	}
	return dim * (1 - 1/(d*RubberBand/dim+1))
}

// RubberInverse is the inverse of Rubber: the finger overshoot that
// displays as v.
func RubberInverse(v, dim float64) float64 {
	if v <= 0 || dim <= 0 {
		return 0
	}
	v = min(v, dim*0.999)
	return (1/(1-v/dim) - 1) * dim / RubberBand
}

// Decay returns the position and velocity t seconds into a fling that
// started at p0 with velocity v0 (px/s).
func Decay(p0, v0, t float64) (p, v float64) {
	e := math.Exp(-decayK * t)
	return p0 + v0/decayK*(1-e), v0 * e
}

// DecayTarget is where a fling from p0 with velocity v0 comes to rest.
func DecayTarget(p0, v0 float64) float64 { return p0 + v0/decayK }

// Spring returns the displacement and velocity t seconds after a
// critically damped spring was released at displacement x0 with velocity
// v0 (target is 0).
func Spring(x0, v0, omega, t float64) (x, v float64) {
	e := math.Exp(-omega * t)
	b := v0 + omega*x0
	return (x0 + b*t) * e, (b - omega*(x0+b*t)) * e
}

// ScrollMode is what a Scroller is doing.
type ScrollMode uint8

const (
	Idle      ScrollMode = iota
	Dragging             // following the finger
	Flinging             // inertial deceleration
	Springing            // bouncing back to an edge or settling on a snap point
)

// Scroller is the scroll physics of one axis: finger tracking with rubber
// band past the edges, inertial fling with exponential deceleration,
// critically damped bounce back, and optional snapping (picker wheels,
// pages). Positions grow as content moves up/left (like scroll offsets).
// It is pure: callers pass the time and read Pos.
type Scroller struct {
	Pos      float64 // current offset
	Min, Max float64 // scroll range
	Dim      float64 // viewport size on this axis (rubber band scale)
	Snap     float64 // > 0: rest only on multiples of Snap (from Min)
	// NoBounce clamps at the edges instead of rubber-banding.
	NoBounce bool

	Mode ScrollMode
	// motion parameters
	t0             time.Time
	p0, v0, target float64
	dragBase       float64 // raw (finger) position at drag start
	glide          bool    // Springing is an exponential glide onto a snap point
}

func (s *Scroller) dim() float64 {
	if s.Dim > 0 {
		return s.Dim
	}
	return 600
}

// display maps a raw finger position to the shown position.
func (s *Scroller) display(raw float64) float64 {
	switch {
	case raw < s.Min:
		if s.NoBounce {
			return s.Min
		}
		return s.Min - Rubber(s.Min-raw, s.dim())
	case raw > s.Max:
		if s.NoBounce {
			return s.Max
		}
		return s.Max + Rubber(raw-s.Max, s.dim())
	}
	return raw
}

// raw is the inverse of display.
func (s *Scroller) raw(p float64) float64 {
	switch {
	case p < s.Min:
		return s.Min - RubberInverse(s.Min-p, s.dim())
	case p > s.Max:
		return s.Max + RubberInverse(p-s.Max, s.dim())
	}
	return p
}

// BeginDrag stops any motion and starts following the finger.
func (s *Scroller) BeginDrag() {
	s.Mode = Dragging
	s.dragBase = s.raw(s.Pos)
}

// DragBy moves to the drag-start position plus delta (finger travel in
// content direction, i.e. negative finger movement for natural scrolling).
func (s *Scroller) DragBy(delta float64) {
	if s.Mode != Dragging {
		s.BeginDrag()
	}
	s.Pos = s.display(s.dragBase + delta)
}

// Release ends a drag with velocity v (px/s, content direction) at time
// now and starts the fling, bounce or snap that follows.
func (s *Scroller) Release(v float64, now time.Time) {
	s.t0, s.p0 = now, s.Pos
	switch {
	case s.Pos < s.Min || s.Pos > s.Max:
		s.startSpring(s.edge(), v, now)
	case s.Snap > 0:
		s.SnapTo(s.snapPoint(DecayTarget(s.Pos, v)), now)
	case math.Abs(v) >= minFlingVelocity:
		s.Mode, s.v0 = Flinging, v
	default:
		s.Mode = Idle
	}
}

// edge returns the nearest bound when the position is out of range.
func (s *Scroller) edge() float64 {
	if s.Pos < s.Min {
		return s.Min
	}
	return s.Max
}

func (s *Scroller) snapPoint(p float64) float64 {
	p = max(s.Min, min(p, s.Max))
	if s.Snap <= 0 {
		return p
	}
	k := math.Round((p - s.Min) / s.Snap)
	return max(s.Min, min(s.Min+k*s.Snap, s.Max))
}

// SnapTo glides to target (decelerating so it lands exactly there).
func (s *Scroller) SnapTo(target float64, now time.Time) {
	s.Mode, s.t0, s.p0, s.target, s.glide = Springing, now, s.Pos, target, true
	s.v0 = (target - s.Pos) * decayKSnap
}

// decayKSnap is the faster decay used to glide onto snap points.
const decayKSnap = 9.0

func (s *Scroller) startSpring(target, v float64, now time.Time) {
	s.Mode, s.t0, s.target, s.glide = Springing, now, target, false
	s.p0, s.v0 = s.Pos, v
}

// Stop halts the motion where it is (a finger caught a fling).
func (s *Scroller) Stop() {
	if s.Mode != Dragging {
		s.Mode = Idle
	}
}

// Settle jumps to the rest position of the current motion.
func (s *Scroller) Settle() {
	switch s.Mode {
	case Flinging:
		s.Pos = DecayTarget(s.p0, s.v0)
	case Springing:
		s.Pos = s.target
	case Dragging:
		return
	}
	s.Pos = max(s.Min, min(s.Pos, s.Max))
	if s.Snap > 0 {
		s.Pos = s.snapPoint(s.Pos)
	}
	s.Mode = Idle
}

// Active reports whether the scroller is moving on its own.
func (s *Scroller) Active() bool { return s.Mode == Flinging || s.Mode == Springing }

// Step advances the motion to now and reports whether it is still moving.
func (s *Scroller) Step(now time.Time) bool {
	t := now.Sub(s.t0).Seconds()
	if t < 0 {
		t = 0
	}
	switch s.Mode {
	case Flinging:
		p, v := Decay(s.p0, s.v0, t)
		if p < s.Min || p > s.Max {
			if s.NoBounce {
				s.Pos, s.Mode = max(s.Min, min(p, s.Max)), Idle
				return false
			}
			// Hit an edge: find the crossing and bounce from there.
			edge := s.Min
			if p > s.Max {
				edge = s.Max
			}
			r := 1 - (edge-s.p0)*decayK/s.v0
			tc := t
			if r > 0 && r < 1 {
				tc = -math.Log(r) / decayK
			}
			_, vc := Decay(s.p0, s.v0, tc)
			// Limit the overshoot to a fraction of the viewport.
			lim := s.dim() / 4 * SpringOmega * math.E
			vc = max(-lim, min(vc, lim))
			s.glide = false
			s.Mode, s.t0, s.target, s.p0, s.v0 = Springing, s.t0.Add(time.Duration(tc*float64(time.Second))), edge, edge, vc
			return s.Step(now)
		}
		s.Pos = p
		if math.Abs(v) < restVelocity {
			s.Mode = Idle
			return false
		}
		return true
	case Springing:
		if s.glide {
			// Snap glide: exponential approach onto the target.
			e := math.Exp(-decayKSnap * t)
			s.Pos = s.target + (s.p0-s.target)*e
			if math.Abs(s.Pos-s.target) < restDistance {
				s.Pos, s.Mode = s.target, Idle
				return false
			}
			return true
		}
		x, v := Spring(s.p0-s.target, s.v0, SpringOmega, t)
		s.Pos = s.target + x
		if math.Abs(x) < restDistance && math.Abs(v) < restVelocity {
			s.Pos, s.Mode = s.target, Idle
			return false
		}
		return true
	}
	return false
}
