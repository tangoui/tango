package ui

import (
	"math"
	"time"
)

type animKey struct {
	id   ID
	slot string
}

type anim struct {
	from, to, val float32
	start         time.Time
	dur           time.Duration
	seen          uint64
	ease          func(float64) float64
}

// Anim returns a value that eases towards target over dur, retained per
// (id, slot). The first call returns target immediately. While moving it
// keeps frames coming; when settled no frames are requested.
func (c *Context) Anim(id ID, slot string, target float32, dur time.Duration) float32 {
	return c.anim(id, slot, target, target, dur, EaseInOut)
}

// AnimFrom is Anim with an explicit initial value (enter transitions).
func (c *Context) AnimFrom(id ID, slot string, initial, target float32, dur time.Duration) float32 {
	return c.anim(id, slot, initial, target, dur, EaseOut)
}

func (c *Context) anim(id ID, slot string, initial, target float32, dur time.Duration, ease func(float64) float64) float32 {
	k := animKey{id, slot}
	a := c.anims[k]
	if a == nil {
		a = &anim{from: initial, to: target, val: initial, start: c.now, dur: dur, ease: ease}
		c.anims[k] = a
	}
	a.seen = c.pass
	if a.to != target {
		a.from, a.to, a.start, a.dur, a.ease = a.val, target, c.now, dur, ease
	}
	if c.noAnim || c.reduceMotion || a.dur <= 0 || a.val == a.to {
		a.val = a.to
		return a.val
	}
	t := float64(c.now.Sub(a.start)) / float64(a.dur)
	if t >= 1 {
		a.val = a.to
		return a.val
	}
	if t < 0 {
		t = 0
	}
	a.val = a.from + (a.to-a.from)*float32(a.ease(t))
	c.animating = true
	return a.val
}

// ReduceMotion reports whether the user asked for reduced motion
// (RunnerOptions.ReduceMotion). Anim and AnimFrom already jump to their
// targets; widgets check it for motion they drive themselves (shimmer,
// pulsing dots, marquees, autoplay).
func (c *Context) ReduceMotion() bool { return c.reduceMotion }

// still reports whether decorative motion must not run (tests without
// animations or the reduced-motion preference).
func (c *Context) still() bool { return c.noAnim || c.reduceMotion }

// AnimDone reports whether the (id, slot) animation has settled at target.
func (c *Context) AnimDone(id ID, slot string) bool {
	a := c.anims[animKey{id, slot}]
	return a == nil || a.val == a.to
}

// cubicBezier returns an easing function for CSS cubic-bezier(x1,y1,x2,y2).
func cubicBezier(x1, y1, x2, y2 float64) func(float64) float64 {
	bx := func(t float64) float64 { return 3*(1-t)*(1-t)*t*x1 + 3*(1-t)*t*t*x2 + t*t*t }
	by := func(t float64) float64 { return 3*(1-t)*(1-t)*t*y1 + 3*(1-t)*t*t*y2 + t*t*t }
	return func(x float64) float64 {
		if x <= 0 {
			return 0
		}
		if x >= 1 {
			return 1
		}
		lo, hi := 0.0, 1.0
		t := x
		for i := 0; i < 24; i++ {
			v := bx(t)
			if math.Abs(v-x) < 1e-5 {
				break
			}
			if v < x {
				lo = t
			} else {
				hi = t
			}
			t = (lo + hi) / 2
		}
		return by(t)
	}
}

// Ant Design motion curves.
var (
	EaseInOut = cubicBezier(0.645, 0.045, 0.355, 1)
	EaseOut   = cubicBezier(0.215, 0.61, 0.355, 1)
	EaseIn    = cubicBezier(0.55, 0.055, 0.675, 0.19)
)
