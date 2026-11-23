package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/theme"
)

func TestElementDrawerTransition(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		open := false
		var panel *DrawerW
		d := NewDriver(func(c *Context) {
			panel = Drawer(c, &open, "Drawer").Body(func() {})
		}, DriverOptions{Theme: theme.ElementPlus(theme.ElementPlusControls()), Animations: true, ReduceMotion: reduced})
		d.Frame()
		open = true
		d.Frame()
		if panel.AfterOpenChanged() != reduced {
			t.Fatalf("reduced=%v: wrong initial settled state", reduced)
		}
		if !reduced {
			d.Advance(299 * time.Millisecond)
			d.Frame()
			if panel.AfterOpenChanged() {
				t.Fatal("drawer settled before 300ms")
			}
			d.Advance(time.Millisecond)
			d.Frame()
			if !panel.AfterOpenChanged() {
				t.Fatal("drawer did not settle at 300ms")
			}
		}
		open = false
		d.Frame()
		if panel.AfterOpenChanged() != reduced {
			t.Fatalf("reduced=%v: wrong initial close state", reduced)
		}
	}
}
