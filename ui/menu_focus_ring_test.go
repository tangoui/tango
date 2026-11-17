package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// TestMenuHorizontalFocusRingOutsideActiveBar: the keyboard focus ring of
// a horizontal menu item is an outline around the whole item (antd
// genFocusOutline on .ant-menu-item), so it never cuts the item's 2px
// active bar off at the bottom. The ring used to be inset 2px from the top
// and bottom, which drew its lower edge right above the bar and left a
// short stub of underline sticking out below the box.
func TestMenuHorizontalFocusRingOutsideActiveBar(t *testing.T) {
	sel := "dash"
	items := []MenuItem{{Key: "dash", Label: "Dashboard"}, {Key: "form", Label: "Forms"}}
	d := NewDriver(func(c *Context) {
		Column(c, func() { Menu(c, items, &sel).Mode(MenuHorizontal).Show() }, layout.Pad(16))
	}, DriverOptions{Width: 600, Height: 200, Theme: theme.AntDesign()})
	d.EnableAccessibility()
	d.Settle()
	n, ok := d.A11yFind(a11y.RoleMenuItem, "Dashboard")
	if !ok {
		t.Fatal("menu item missing")
	}
	// Keyboard focus, as Tab would give it: the ring is :focus-visible only.
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.ID != n.ID {
		if err := d.PerformAction(n.ID, a11y.Focus, ""); err != nil {
			t.Fatal(err)
		}
		d.Settle()
	}
	tk := d.r.c.Theme.Components.Menu
	var bar, ring *paint.FillRect
	var ringStroke *paint.StrokeRect
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Rect.H == tk.HorizontalActiveBarHeight && o.Rect.Y > n.Bounds.Y && o.Rect.X >= n.Bounds.X-1 && o.Rect.Right() <= n.Bounds.X+n.Bounds.W+1 {
				oo := o
				bar = &oo
			}
		case paint.StrokeRect:
			if o.Color == d.r.c.Theme.FocusOutline || o.Color == d.r.c.Theme.FocusRing {
				oo := o
				ringStroke = &oo
			}
		}
	}
	_ = ring
	if bar == nil {
		t.Fatal("active bar not painted under the selected item")
	}
	if ringStroke == nil {
		t.Fatal("no focus ring painted for the keyboard-focused item")
	}
	if ringStroke.Rect.Bottom() < bar.Rect.Bottom() {
		t.Fatalf("focus ring bottom %v cuts across the active bar (bar %v..%v)", ringStroke.Rect.Bottom(), bar.Rect.Y, bar.Rect.Bottom())
	}
	if ringStroke.Rect.X > bar.Rect.X || ringStroke.Rect.Right() < bar.Rect.Right() {
		t.Fatalf("focus ring %v does not enclose the active bar %v", ringStroke.Rect, bar.Rect)
	}
}
