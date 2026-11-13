package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// TestPointerModsFollowsDrag: PressMods keeps the modifiers of the press,
// PointerMods follows later moves and key events.
func TestPointerModsFollowsDrag(t *testing.T) {
	var press, ptr input.Modifiers
	d := NewDriver(func(c *Context) {
		press, ptr = c.PressMods(), c.PointerMods()
	}, DriverOptions{Width: 100, Height: 100})
	d.Settle()
	d.Move(geom.Pt(10, 10))
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Press, Pos: geom.Pt(10, 10), Button: input.ButtonLeft})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(20, 10), Button: input.ButtonLeft, Mods: input.ModAlt})
	d.Frame()
	if press != 0 || ptr != input.ModAlt {
		t.Fatalf("after move with Alt: press=%v pointer=%v", press, ptr)
	}
	d.Inject(input.KeyEvent{Key: input.KeyEscape, Down: true})
	d.Frame()
	if ptr != 0 {
		t.Fatalf("after key event without mods: pointer=%v", ptr)
	}
}

// TestReleasePosIsTheReleaseEvent: a move after the release in the same
// frame does not change ReleasePos.
func TestReleasePosIsTheReleaseEvent(t *testing.T) {
	var id ID
	var got geom.Point
	var ok bool
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(80, 80))
		b.Focusable(input.CursorDefault)
		id = b.ID()
		if p, k := c.ReleasePos(id); k {
			got, ok = p, true // latch: a frame may build more than once
		}
	}, DriverOptions{Width: 100, Height: 100})
	d.Settle()
	d.Inject(input.Pointer{Kind: input.Press, Pos: geom.Pt(10, 10), Button: input.ButtonLeft})
	d.Frame()
	if ok {
		t.Fatal("ReleasePos reported before any release")
	}
	ok = false
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(30, 20), Button: input.ButtonLeft})
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(70, 70)})
	d.Frame()
	if !ok || got != geom.Pt(30, 20) {
		t.Fatalf("ReleasePos = %v %v, want (30,20) true", got, ok)
	}
	ok = false
	d.Move(geom.Pt(50, 50))
	d.Frame()
	if ok {
		t.Fatal("ReleasePos must be cleared next frame")
	}
}
