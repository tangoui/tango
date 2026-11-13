package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

// TestA11yDirection: semantic nodes carry the direction of the scope they
// were built in, nested scopes included; the tree carries the window's.
func TestA11yDirection(t *testing.T) {
	page := func(c *Context) {
		Button(c, "Plain")
		ConfigProvider(c).Direction(RTL).Body(func() {
			Button(c, "Arabic")
			ConfigProvider(c).Direction(LTR).Body(func() {
				Button(c, "Code")
			})
		})
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 300})
	tr := d.EnableAccessibility()
	if tr.RTL || tr.Node(tr.Root).RTL {
		t.Fatal("window default should be left to right")
	}
	for name, want := range map[string]bool{"Plain": false, "Arabic": true, "Code": false} {
		n, ok := d.A11yFind(a11y.RoleButton, name)
		if !ok {
			t.Fatalf("%s not found:\n%s", name, tr.Dump())
		}
		if n.RTL != want {
			t.Errorf("%s: RTL=%v want %v", name, n.RTL, want)
		}
	}
	// The RTL button's bounds are its mirrored screen rectangle: the
	// same rectangle the hit test uses.
	n, _ := d.A11yFind(a11y.RoleButton, "Arabic")
	r := d.r.c.rects[ID(n.ID)]
	if n.Bounds != toA11yRect(r) {
		t.Errorf("bounds %v, rect %v", n.Bounds, r)
	}
}
