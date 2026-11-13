package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Escape goes to the top-most layer: a Confirm over a Modal closes first,
// and a dropdown inside the Modal closes before the Modal.
func TestEscapeRoutesToTopLayer(t *testing.T) {
	open := true
	val := ""
	var sel *SelectW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			if Button(c, "确认").Clicked() {
				Confirm(c, ConfirmOptions{Title: "二次确认"})
			}
		}, layout.Pad(20))
		Modal(c, &open, "对话框").Body(func() {
			sel = Select(c, &val, Opts("甲", "乙")).Width(200)
			if Button(c, "再确认").Clicked() {
				Confirm(c, ConfirmOptions{Title: "二次确认"})
			}
		})
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.ClickLabel("再确认")
	if OpenDialogs(d.r.c) != 1 {
		t.Fatal("confirm not open")
	}
	d.Key(input.KeyEscape, 0)
	if OpenDialogs(d.r.c) != 0 || !open {
		t.Fatalf("escape: confirm open=%d modal=%v", OpenDialogs(d.r.c), open)
	}
	// Dropdown inside the modal.
	d.Click(d.r.c.rects[sel.n.id].Center())
	if !sel.IsOpen() {
		t.Fatal("select not open")
	}
	d.Key(input.KeyEscape, 0)
	if sel.IsOpen() || !open {
		t.Fatalf("escape: select open=%v modal=%v", sel.IsOpen(), open)
	}
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("modal did not close on the next Escape")
	}
}

func TestEscapeDrawer(t *testing.T) {
	open := true
	d := driver(func(c *Context) {
		Drawer(c, &open, "抽屉").Body(func() { Text(c, "内容") })
	})
	d.Frame()
	d.Frame()
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("drawer stayed open")
	}
}
