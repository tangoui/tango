package main

import (
	"testing"

	"github.com/tangoui/tango/macoslite"
)

func click(c *counter, r macoslite.Rect) {
	p := macoslite.Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
	c.Mouse(macoslite.MouseEvent{Kind: macoslite.MouseDown, Pos: p})
	c.Mouse(macoslite.MouseEvent{Kind: macoslite.MouseUp, Pos: p})
}

type recorder struct{ invalidations, stops int }

func (r *recorder) Invalidate() { r.invalidations++ }
func (r *recorder) Stop()       { r.stops++ }

func TestCounterButtons(t *testing.T) {
	rec := &recorder{}
	c := &counter{host: rec}
	click(c, plusRect)
	click(c, plusRect)
	click(c, minusRect)
	if c.n != 1 {
		t.Fatalf("n = %d, want 1", c.n)
	}
	click(c, resetRect)
	if c.n != 0 {
		t.Fatalf("after reset n = %d", c.n)
	}
	if rec.invalidations < 4 {
		t.Fatalf("invalidations = %d, want one per change", rec.invalidations)
	}
}

func TestCounterClickNeedsPressAndReleaseInside(t *testing.T) {
	c := &counter{host: &recorder{}}
	c.Mouse(macoslite.MouseEvent{Kind: macoslite.MouseDown, Pos: macoslite.Point{X: plusRect.X + 1, Y: plusRect.Y + 1}})
	c.Mouse(macoslite.MouseEvent{Kind: macoslite.MouseUp, Pos: macoslite.Point{X: 1, Y: 1}})
	if c.n != 0 {
		t.Fatal("release outside the button must not count")
	}
}

func TestCounterKeys(t *testing.T) {
	rec := &recorder{}
	c := &counter{host: rec}
	c.Key(macoslite.KeyEvent{Key: "+", Down: true})
	c.Key(macoslite.KeyEvent{Key: "=", Down: true})
	c.Key(macoslite.KeyEvent{Key: "-", Down: true})
	if c.n != 1 {
		t.Fatalf("n = %d, want 1", c.n)
	}
	c.Key(macoslite.KeyEvent{Key: "Escape", Down: false})
	if rec.stops != 0 {
		t.Fatal("key release must not stop")
	}
	c.Key(macoslite.KeyEvent{Key: "Escape", Down: true})
	if rec.stops != 1 {
		t.Fatalf("stops = %d, want 1", rec.stops)
	}
}
