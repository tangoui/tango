package ui

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func carouselDriver(cur *int, mod func(w *CarouselW)) (*Driver, **Box) {
	var box *Box
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			w := Carousel(c, 3, func(i int) {
				Column(c, func() {
					Text(c, fmt.Sprintf("slide-%d", i))
					Button(c, fmt.Sprintf("btn-%d", i))
				}, layout.Grow(1))
			}).Current(cur).Draggable(true)
			if mod != nil {
				mod(w)
			}
			box = w.Show()
		}, layout.W(400))
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	d.Settle()
	return d, &box
}

// TestCarouselSwipe: slides follow the pointer; a drag past the threshold
// turns the page, a short one springs back, and a quick touch flick turns
// it whatever the travel.
func TestCarouselSwipe(t *testing.T) {
	cur := 0
	d, box := carouselDriver(&cur, nil)
	r := (*box).Rect()
	mid := geom.Pt(r.X+r.W/2, r.Y+r.H-20) // below the content, above the dots
	mid.Y = r.Y + r.H/2 + 30

	// Mid-drag the current slide follows the pointer.
	s0, _ := d.FindText("slide-0")
	d.Inject(input.Pointer{Kind: input.Move, Pos: mid})
	d.Inject(input.Pointer{Kind: input.Press, Pos: mid})
	d.Frame()
	d.Move(geom.Pt(mid.X-50, mid.Y))
	d.Frame()
	s0b, _ := d.FindText("slide-0")
	if !near(s0b.X, s0.X-50) {
		t.Fatalf("slide did not follow the pointer: %v -> %v", s0.X, s0b.X)
	}
	// Released short of the threshold (80px of 400): springs back.
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(mid.X-50, mid.Y)})
	d.Settle()
	if cur != 0 {
		t.Fatalf("short drag turned the page: cur=%d", cur)
	}
	if s, _ := d.FindText("slide-0"); !near(s.X, s0.X) {
		t.Fatalf("did not spring back: %v", s.X)
	}
	// Past the threshold to the left: next slide.
	d.Drag(mid, geom.Pt(mid.X-150, mid.Y))
	if cur != 1 {
		t.Fatalf("swipe left: cur=%d", cur)
	}
	// To the right: previous.
	d.Drag(mid, geom.Pt(mid.X+150, mid.Y))
	if cur != 0 {
		t.Fatalf("swipe right: cur=%d", cur)
	}
	// A drag that starts on a button inside the slide is taken over.
	b, ok := d.Find("btn-0")
	if !ok {
		t.Fatal("btn-0 missing")
	}
	d.Drag(b.Center(), geom.Pt(b.Center().X-200, b.Center().Y))
	if cur != 1 {
		t.Fatalf("takeover swipe: cur=%d", cur)
	}
	// A quick touch flick (60px in ~130ms) turns the page.
	d.TouchDrag(mid, geom.Pt(mid.X-60, mid.Y))
	if cur != 2 {
		t.Fatalf("flick: cur=%d", cur)
	}
	// Looping: swiping on from the last slide wraps to the first.
	d.Drag(mid, geom.Pt(mid.X-150, mid.Y))
	if cur != 0 {
		t.Fatalf("loop wrap: cur=%d", cur)
	}
}

// TestCarouselSwipeRTL: in RTL the slides run right to left, so a swipe to
// the right shows the next slide.
func TestCarouselSwipeRTL(t *testing.T) {
	cur := 0
	var box *Box
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Column(c, func() {
				box = Carousel(c, 3, func(i int) { Text(c, fmt.Sprintf("slide-%d", i)) }).Current(&cur).Draggable(true).Show()
			}, layout.W(400))
		})
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	d.Settle()
	r := box.Rect()
	mid := r.Center()
	d.Drag(mid, geom.Pt(mid.X+150, mid.Y))
	if cur != 1 {
		t.Fatalf("RTL swipe right: cur=%d", cur)
	}
}

// TestCarouselNoLoop: Loop(false) stops at both ends for arrows, keys,
// swipes and autoplay, and the arrows at the ends are disabled.
func TestCarouselNoLoop(t *testing.T) {
	cur := 0
	d, box := carouselDriver(&cur, func(w *CarouselW) { w.Loop(false).Arrows("always").Dots(false) })
	prev := mustFind(t, d, "上一张")
	d.Click(prev.Center())
	if cur != 0 {
		t.Fatalf("prev at the first slide wrapped: cur=%d", cur)
	}
	d.EnableAccessibility()
	d.Settle()
	if a, ok := d.A11yFind(a11y.RoleButton, "上一张"); !ok || !a.States.Has(a11y.Disabled) {
		t.Fatal("prev arrow should be disabled at the first slide")
	}
	if a, ok := d.A11yFind(a11y.RoleButton, "下一张"); !ok || a.States.Has(a11y.Disabled) {
		t.Fatal("next arrow should be enabled")
	}
	r := (*box).Rect()
	mid := geom.Pt(r.X+r.W/2, r.Y+r.H/2+30)
	// Swiping right at the first slide only rubber-bands.
	d.Drag(mid, geom.Pt(mid.X+200, mid.Y))
	if cur != 0 {
		t.Fatalf("swipe before the first slide: cur=%d", cur)
	}
	next := mustFind(t, d, "下一张")
	d.Click(next.Center())
	d.Click(next.Center())
	if cur != 2 {
		t.Fatalf("next twice: cur=%d", cur)
	}
	next = mustFind(t, d, "下一张")
	d.Click(next.Center())
	if cur != 2 {
		t.Fatalf("next at the last slide wrapped: cur=%d", cur)
	}
	// Keys stop at the end too (the frame is focusable without dots).
	d.Click(geom.Pt(r.X+r.W/2, r.Y+10))
	d.Key(input.KeyRight, 0)
	if cur != 2 {
		t.Fatalf("right key at the last slide: cur=%d", cur)
	}
	d.Key(input.KeyLeft, 0)
	if cur != 1 {
		t.Fatalf("left key: cur=%d", cur)
	}
	d.Drag(mid, geom.Pt(mid.X-200, mid.Y))
	d.Drag(mid, geom.Pt(mid.X-200, mid.Y))
	if cur != 2 {
		t.Fatalf("swipe past the last slide: cur=%d", cur)
	}
}

// TestCarouselDotsRingedOnLightSlides: the light dots drawn on the slides
// get a dark ring, so they stay visible on light slides (here the slides are
// the white page); dots placed outside the frame, on the page, get none.
func TestCarouselDotsRingedOnLightSlides(t *testing.T) {
	for _, tc := range []struct {
		pos   string
		rings int
	}{{"bottom", 3}, {"outside", 0}} {
		cur := 0
		d, _ := carouselDriver(&cur, func(w *CarouselW) { w.DotPosition(tc.pos) })
		ring := d.Runner().Context().ThemeFor(theme.CompCarousel).Components.Carousel.DotRingColor
		n := 0
		for _, op := range d.Runner().List().Ops {
			if s, ok := op.(paint.StrokeRect); ok && s.Color == color.NRGBA(ring) {
				n++
			}
		}
		if n != tc.rings {
			t.Errorf("%s: %d ringed dots, want %d", tc.pos, n, tc.rings)
		}
		if tc.rings > 0 {
			white := theme.Hex("#ffffff")
			if l := theme.Luma(theme.Over(ring, white)); l > 0.8 {
				t.Errorf("ring over white has luma %.2f: not visible", l)
			}
		}
	}
}
