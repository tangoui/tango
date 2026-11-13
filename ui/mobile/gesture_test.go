package mobile

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// finger drives a Driver with timed pointer events, one frame per step.
type finger struct {
	t  *testing.T
	d  *ui.Driver
	at geom.Point
}

func newFinger(t *testing.T, d *ui.Driver) *finger { return &finger{t: t, d: d} }

func (f *finger) down(p geom.Point) {
	f.at = p
	f.d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	f.d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	f.d.Frame()
}

// move travels to p in steps frames of dt each.
func (f *finger) move(p geom.Point, steps int, dt time.Duration) {
	from := f.at
	for i := 1; i <= steps; i++ {
		k := float32(i) / float32(steps)
		f.d.Advance(dt)
		f.d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X+(p.X-from.X)*k, from.Y+(p.Y-from.Y)*k)})
		f.d.Frame()
	}
	f.at = p
}

func (f *finger) up() {
	f.d.Inject(input.Pointer{Kind: input.Release, Pos: f.at})
	f.d.Frame()
}

func (f *finger) hold(d time.Duration) {
	f.d.Advance(d)
	f.d.Frame()
}

// run advances simulated time until the predicate holds or limit passes.
func run(d *ui.Driver, limit time.Duration) {
	for el := time.Duration(0); el < limit; el += 16 * time.Millisecond {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
}

func phone(page func(c *ui.Context), anim bool) *ui.Driver {
	return ui.NewDriver(page, ui.DriverOptions{Width: 390, Height: 844, Animations: anim})
}

func TestGestureTapLongSwipe(t *testing.T) {
	var g Gesture
	taps, longs, swipes := 0, 0, []Swipe{}
	d := phone(func(c *ui.Context) {
		b := ui.Div(c, layout.Size(300, 300))
		g = Area(c, b, AxisBoth, nil)
		if g.Tap {
			taps++
		}
		if g.LongPress {
			longs++
		}
		if g.Swiped {
			swipes = append(swipes, g.Swipe)
		}
	}, true)
	d.Frame()
	d.Frame()
	f := newFinger(t, d)

	// Tap: press and release in place.
	f.down(geom.Pt(100, 100))
	f.up()
	d.Frame()
	if taps != 1 || longs != 0 {
		t.Fatalf("tap: taps=%d longs=%d", taps, longs)
	}
	// Instant click (press+release in one frame) also taps.
	d.Click(geom.Pt(120, 120))
	if taps != 2 {
		t.Fatalf("instant tap: %d", taps)
	}
	// Long press: hold without moving.
	f.down(geom.Pt(100, 100))
	f.hold(300 * time.Millisecond)
	if longs != 0 {
		t.Fatal("long press too early")
	}
	f.hold(300 * time.Millisecond)
	f.up()
	d.Frame()
	if longs != 1 || taps != 2 {
		t.Fatalf("long press: longs=%d taps=%d", longs, taps)
	}
	// Fast flick to the left: a swipe with high speed.
	f.down(geom.Pt(250, 150))
	f.move(geom.Pt(150, 150), 5, 16*time.Millisecond)
	f.up()
	d.Frame()
	if len(swipes) != 1 || swipes[0].Dir != DirLeft || swipes[0].Speed < 1000 {
		t.Fatalf("flick: %+v", swipes)
	}
	// Slow drag down then rest: not fast, but far enough to be a swipe;
	// the release velocity is ~0 because the finger rested.
	f.down(geom.Pt(150, 50))
	f.move(geom.Pt(150, 200), 10, 50*time.Millisecond)
	f.hold(200 * time.Millisecond)
	f.up()
	d.Frame()
	if len(swipes) != 2 || swipes[1].Dir != DirDown || swipes[1].Speed > 1 {
		t.Fatalf("slow drag: %+v", swipes)
	}
	if taps != 2 {
		t.Fatalf("drag produced a tap")
	}
}

func TestArenaTapReachesRowInsideScroll(t *testing.T) {
	clicked := -1
	d := phone(func(c *ui.Context) {
		ScrollView(c).Layout(layout.H(400)).Body(func() {
			for i := 0; i < 30; i++ {
				ui.Key(c, i, func() {
					if Cell(c, "row").Clickable().Clicked() {
						clicked = i
					}
				})
			}
		})
	}, false)
	d.Frame()
	d.Frame()
	d.Click(geom.Pt(100, 50*2+25))
	if clicked != 2 {
		t.Fatalf("clicked %d", clicked)
	}
}

func TestDecayAndRubber(t *testing.T) {
	// Exponential deceleration: velocity halves after ln2/k seconds and the
	// fling comes to rest at p0 + v0/k.
	k := -math.Log(DecelerationRate) * 1000
	_, v := Decay(0, 1000, math.Ln2/k)
	if math.Abs(v-500) > 1 {
		t.Fatalf("half-life velocity %v", v)
	}
	if p, _ := Decay(0, 1000, 20); math.Abs(p-DecayTarget(0, 1000)) > 0.01 {
		t.Fatalf("rest %v vs %v", p, DecayTarget(0, 1000))
	}
	if r := Rubber(100, 600); r >= 100 || r <= 0 {
		t.Fatalf("rubber %v", r)
	}
	if inv := RubberInverse(Rubber(80, 600), 600); math.Abs(inv-80) > 1e-6 {
		t.Fatalf("rubber inverse %v", inv)
	}
	// Positions along a fling are monotonic and decelerating.
	prev, prevStep := 0.0, math.Inf(1)
	for i := 1; i <= 50; i++ {
		p, _ := Decay(0, 2000, float64(i)*0.016)
		step := p - prev
		if step <= 0 || step > prevStep+1e-9 {
			t.Fatalf("step %d: %v after %v", i, step, prevStep)
		}
		prev, prevStep = p, step
	}
}

func TestScrollerFlingBounce(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := Scroller{Min: 0, Max: 1000, Dim: 600}
	s.BeginDrag()
	s.DragBy(-100) // pull past the top: rubber band
	if s.Pos >= 0 || s.Pos < -100 {
		t.Fatalf("rubber pos %v", s.Pos)
	}
	s.Release(0, t0)
	for i := 1; i < 200 && s.Step(t0.Add(time.Duration(i)*16*time.Millisecond)); i++ {
	}
	if s.Pos != 0 || s.Mode != Idle {
		t.Fatalf("bounce back to %v (%v)", s.Pos, s.Mode)
	}
	// A fast fling towards the end overshoots and settles on Max.
	s.Pos = 900
	s.BeginDrag()
	s.DragBy(0)
	s.Release(3000, t0)
	over := false
	for i := 1; i < 400 && s.Step(t0.Add(time.Duration(i)*16*time.Millisecond)); i++ {
		if s.Pos > 1000.5 {
			over = true
		}
	}
	if !over || s.Pos != 1000 {
		t.Fatalf("fling edge: over=%v pos=%v", over, s.Pos)
	}
}
