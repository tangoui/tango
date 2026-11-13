package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// countingBack records SetBackHandling calls on top of the Driver window.
type countingBack struct {
	*driverWindow
	calls []bool
}

func (w *countingBack) SetBackHandling(on bool) { w.calls = append(w.calls, on) }

// Back (delivered as Escape) closes the top-most layer first, and
// WantBack follows what is open: on with a Confirm over a Modal, still on
// after Escape closed the Confirm, off after the Modal closed too.
func TestWantBackFollowsLayers(t *testing.T) {
	modal := false
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			if Button(c, "打开").Clicked() {
				modal = true
			}
		}, layout.Pad(20))
		Modal(c, &modal, "对话框").Body(func() {
			if Button(c, "再确认").Clicked() {
				Confirm(c, ConfirmOptions{Title: "二次确认"})
			}
		})
	}, DriverOptions{Width: 800, Height: 600})
	w := &countingBack{driverWindow: d.w}
	d.r.Attach(w)
	d.Settle()
	if d.r.c.WantBack() || len(w.calls) != 0 {
		t.Fatalf("closed page: WantBack=%v calls=%v", d.r.c.WantBack(), w.calls)
	}
	if err := d.ClickLabel("打开"); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !modal || !d.r.c.WantBack() {
		t.Fatalf("modal open=%v WantBack=%v", modal, d.r.c.WantBack())
	}
	if err := d.ClickLabel("再确认"); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if OpenDialogs(d.r.c) != 1 {
		t.Fatal("confirm not open")
	}
	d.Key(input.KeyEscape, 0)
	if OpenDialogs(d.r.c) != 0 || !modal {
		t.Fatalf("first Back: confirm open=%d modal=%v", OpenDialogs(d.r.c), modal)
	}
	if !d.r.c.WantBack() {
		t.Fatal("WantBack off while the modal is still open")
	}
	d.Key(input.KeyEscape, 0)
	if modal {
		t.Fatal("second Back did not close the modal")
	}
	if d.r.c.WantBack() {
		t.Fatal("WantBack still on with nothing open")
	}
	// Only changes are reported: on once, off once.
	if len(w.calls) != 2 || !w.calls[0] || w.calls[1] {
		t.Fatalf("SetBackHandling calls = %v, want [true false]", w.calls)
	}
	// A third Back with nothing open reaches the page (no overlay takes it).
	d.Key(input.KeyEscape, 0)
	if len(w.calls) != 2 {
		t.Fatalf("calls after idle Back = %v", w.calls)
	}
}

// Drawers count as well; the Driver window reports the state.
func TestWantBackDrawerDriver(t *testing.T) {
	open := true
	d := driver(func(c *Context) {
		Drawer(c, &open, "抽屉").Body(func() { Text(c, "内容") })
	})
	d.Settle()
	d.Frame()
	if !d.BackHandling() {
		t.Fatal("open drawer: BackHandling off")
	}
	d.Key(input.KeyEscape, 0)
	if open || d.BackHandling() {
		t.Fatalf("after Back: open=%v BackHandling=%v", open, d.BackHandling())
	}
}

// ClaimBack lets the page take Back; the page reads it with Escape, and
// an overlay above the page still takes it first.
func TestClaimBack(t *testing.T) {
	depth, modal := 2, false
	d := driver(func(c *Context) {
		if depth > 0 {
			c.ClaimBack()
			if c.Escape() {
				depth--
			}
		}
		Modal(c, &modal, "对话框").Body(func() { Text(c, "内容") })
	})
	d.Settle()
	if !d.BackHandling() {
		t.Fatal("ClaimBack: BackHandling off")
	}
	modal = true
	d.Settle()
	d.Frame()
	d.Key(input.KeyEscape, 0)
	if modal || depth != 2 {
		t.Fatalf("Back with modal: modal=%v depth=%d", modal, depth)
	}
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyEscape, 0)
	if depth != 0 {
		t.Fatalf("depth = %d", depth)
	}
	if d.BackHandling() {
		t.Fatal("BackHandling on after the page stopped claiming")
	}
}
