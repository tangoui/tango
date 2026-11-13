package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestCaptureTab(t *testing.T) {
	var editor, other ID
	var tabs int
	capture := true
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(200, 40))
		b.Focusable(input.CursorText)
		editor = b.ID()
		if capture {
			c.CaptureTab(editor)
		}
		for _, k := range c.KeysFor(editor) {
			if k.Down && k.Key == input.KeyTab {
				tabs++
			}
		}
		o := Div(c, layout.Size(200, 40))
		o.Focusable(input.CursorPointer)
		other = o.ID()
	}, DriverOptions{Width: 300, Height: 120})
	d.Frame()
	d.Click(geom.Pt(20, 20))
	d.Frame()
	if d.r.c.focus != editor {
		t.Fatal("editor not focused")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, input.ModShift)
	if tabs != 2 || d.r.c.focus != editor {
		t.Fatalf("captured Tab: tabs=%d focus moved=%v", tabs, d.r.c.focus != editor)
	}
	d.Key(input.KeyTab, input.ModCtrl)
	if d.r.c.focus != other {
		t.Fatal("Ctrl+Tab should move focus out of a Tab-capturing widget")
	}
	d.Click(geom.Pt(20, 20))
	d.Frame()
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyTab, 0)
	if d.r.c.focus != other {
		t.Fatal("Esc then Tab should move focus out")
	}
	capture = false
	d.Click(geom.Pt(20, 20))
	d.Frame()
	d.Key(input.KeyTab, 0)
	if d.r.c.focus != other {
		t.Fatal("without CaptureTab, Tab moves focus")
	}
}

func TestPressCount(t *testing.T) {
	var counts []int
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(100, 40))
		if ia := b.Interact(input.CursorText); ia.PressedNow {
			counts = append(counts, c.PressCount())
		}
	}, DriverOptions{Width: 200, Height: 100})
	d.Frame()
	d.Click(geom.Pt(10, 10))
	d.Click(geom.Pt(10, 10))
	d.Click(geom.Pt(10, 10))
	if len(counts) != 3 || counts[2] != 3 {
		t.Fatalf("press counts %v, want [1 2 3]", counts)
	}
}
