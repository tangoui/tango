package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEventsForKeepsOrder(t *testing.T) {
	var got []input.Event
	var id ID
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(200, 40))
		b.Focusable(input.CursorText)
		id = b.ID()
		got = append(got, c.EventsFor(id)...)
	}, DriverOptions{Width: 300, Height: 100})
	d.Frame()
	d.Click(geom.Pt(20, 20))
	d.Frame()
	got = nil
	// One frame: Backspace, then "a", then a composition, then Enter.
	d.Inject(input.KeyEvent{Key: input.KeyBackspace, Down: true})
	d.Inject(input.TextInput{Text: "a"})
	d.Inject(input.Composition{Text: "ni"})
	d.Inject(input.KeyEvent{Key: input.KeyNumpadEnter, Down: true})
	d.Frame()
	want := []string{"key:Backspace", "text:a", "comp:ni", "key:Enter"}
	var seq []string
	for _, e := range got {
		switch e := e.(type) {
		case input.KeyEvent:
			name := "Backspace"
			if e.Key == input.KeyEnter {
				name = "Enter"
				if e.Code != input.KeyNumpadEnter {
					t.Errorf("Code = %v, want the physical NumpadEnter", e.Code)
				}
			}
			seq = append(seq, "key:"+name)
		case input.TextInput:
			seq = append(seq, "text:"+e.Text)
		case input.Composition:
			seq = append(seq, "comp:"+e.Text)
		}
	}
	if len(seq) != len(want) {
		t.Fatalf("events %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("events %v, want %v", seq, want)
		}
	}
}

func TestPressModsAndWindowFocused(t *testing.T) {
	var mods input.Modifiers
	var focused bool
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(100, 40))
		if b.Interact(input.CursorText).PressedNow {
			mods = c.PressMods()
		}
		focused = c.WindowFocused()
	}, DriverOptions{Width: 200, Height: 100})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(10, 10)})
	d.Inject(input.Pointer{Kind: input.Press, Pos: geom.Pt(10, 10), Mods: input.ModShift})
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(10, 10), Mods: input.ModShift})
	d.Frame()
	if !mods.Has(input.ModShift) {
		t.Fatal("PressMods lost Shift")
	}
	if !focused {
		t.Fatal("window should start focused")
	}
	d.Inject(input.WindowFocus{Focused: false})
	d.Frame()
	if focused {
		t.Fatal("WindowFocused should be false after the window lost focus")
	}
}
