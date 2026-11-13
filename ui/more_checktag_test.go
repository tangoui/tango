package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestMoreCheckableTagToggleKeysDisabled(t *testing.T) {
	a, b := false, true
	disabled := false
	changes := 0
	d := driver(func(c *Context) {
		Row(c, func() {
			if CheckableTag(c, "电影", &a).Changed() {
				changes++
			}
			CheckableTag(c, "音乐", &b).Disabled(disabled)
		}, layout.Pad(20), layout.Gap(8))
	})
	d.Frame()
	d.Click(mustFind(t, d, "电影").Center())
	if !a || changes != 1 {
		t.Fatalf("click: a=%v changes=%d", a, changes)
	}
	d.Click(mustFind(t, d, "电影").Center())
	if a || changes != 2 {
		t.Fatalf("second click: a=%v changes=%d", a, changes)
	}
	// Keyboard: Tab focuses the first tag, Space and Enter toggle it.
	d.r.c.SetFocus(0)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeySpace, 0)
	if !a {
		t.Fatal("space did not toggle")
	}
	d.Key(input.KeyEnter, 0)
	if a {
		t.Fatal("enter did not toggle")
	}
	// Disabled tag ignores clicks and leaves the tab order.
	disabled = true
	d.Frame()
	d.Click(mustFind(t, d, "音乐").Center())
	if !b {
		t.Fatal("disabled tag toggled")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	if f := d.r.c.focus; f != 0 {
		if r, _ := d.r.c.RectOf(f); r == mustFind(t, d, "音乐") {
			t.Fatal("disabled tag focusable")
		}
	}
}

func TestMoreCheckableTagThemes(t *testing.T) {
	on := true
	var w *CheckableTagW
	d := driver(func(c *Context) {
		Row(c, func() { w = CheckableTag(c, "标签", &on) }, layout.Pad(20))
	})
	d.Frame()
	ant := mustFind(t, d, "标签")
	if ant.H != 22 {
		t.Fatalf("ant height %v", ant.H)
	}
	// Checked Ant tag is filled with the primary color.
	px := d.Image().RGBAAt(int(ant.X+2), int(ant.Y+ant.H/2))
	if px.B < 200 || px.R > 100 {
		t.Fatalf("ant checked bg %v", px)
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	el := mustFind(t, d, "标签")
	if el.H != 28 || el.W <= ant.W {
		t.Fatalf("element size %v (ant %v)", el, ant)
	}
	if !w.Checked() {
		t.Fatal("value lost")
	}
}
