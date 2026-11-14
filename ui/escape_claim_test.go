package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// escWidget is a focusable box that claims Esc (and optionally Tab) and
// counts the Esc presses it receives through KeysFor / EventsFor.
type escWidget struct {
	id         ID
	claim, tab bool
	keys, evs  int
	backs      int // Esc presses with Code KeyBack that reached it
}

func (w *escWidget) build(c *Context) {
	b := Div(c, layout.Size(200, 40))
	b.Focusable(input.CursorText)
	w.id = b.ID()
	if w.claim {
		c.ClaimEscape(w.id)
	}
	if w.tab {
		c.CaptureTab(w.id)
	}
	for _, k := range c.KeysFor(w.id) {
		if k.Down && k.Key == input.KeyEscape {
			w.keys++
			if k.Code == input.KeyBack {
				w.backs++
			}
		}
	}
	for _, e := range c.EventsFor(w.id) {
		if k, ok := e.(input.KeyEvent); ok && k.Down && k.Key == input.KeyEscape {
			w.evs++
		}
	}
}

// back delivers the system Back the way the Android host does.
func back(d *Driver) {
	d.Inject(input.KeyEvent{Key: input.KeyEscape, Code: input.KeyBack, Down: true})
	d.Inject(input.KeyEvent{Key: input.KeyEscape, Code: input.KeyBack})
	d.Settle()
}

func focusWidget(t *testing.T, d *Driver, w *escWidget) {
	t.Helper()
	d.Settle()
	d.r.c.SetFocus(w.id)
	d.Settle()
	if d.r.c.focus != w.id {
		t.Fatal("widget not focused")
	}
}

func TestClaimEscapeReachesWidget(t *testing.T) {
	w := &escWidget{claim: true}
	d := NewDriver(func(c *Context) { w.build(c) }, DriverOptions{Width: 300, Height: 120})
	focusWidget(t, d, w)
	d.Key(input.KeyEscape, 0)
	if w.keys != 1 || w.evs != 1 {
		t.Fatalf("claimed Esc: KeysFor %d EventsFor %d, want 1 1", w.keys, w.evs)
	}
	// Without the claim Esc stays out of KeysFor (overlay key).
	w.claim = false
	d.Settle()
	d.Key(input.KeyEscape, 0)
	if w.keys != 1 || w.evs != 2 {
		t.Fatalf("unclaimed Esc: KeysFor %d EventsFor %d, want 1 2", w.keys, w.evs)
	}
}

func TestClaimEscapeKeepsModalOpen(t *testing.T) {
	open := true
	w := &escWidget{claim: true}
	d := NewDriver(func(c *Context) {
		Modal(c, &open, "对话框").Body(func() { w.build(c) })
	}, DriverOptions{Width: 800, Height: 600})
	focusWidget(t, d, w)
	d.Key(input.KeyEscape, 0)
	if !open {
		t.Fatal("claimed Esc closed the modal")
	}
	if w.keys != 1 {
		t.Fatalf("widget got %d Esc, want 1", w.keys)
	}
}

func TestEscapeWithoutClaimClosesModal(t *testing.T) {
	open := true
	var val string
	var in ID
	d := NewDriver(func(c *Context) {
		Modal(c, &open, "对话框").Body(func() { in = Input(c, &val).ID() })
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	d.r.c.SetFocus(in)
	d.Settle()
	if d.r.c.focus != in {
		t.Fatal("input not focused")
	}
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("Esc in a plain Input did not close the modal")
	}
}

func TestClaimEscapeCtrlTabLeaves(t *testing.T) {
	w := &escWidget{claim: true, tab: true}
	var other ID
	d := NewDriver(func(c *Context) {
		w.build(c)
		o := Div(c, layout.Size(200, 40))
		o.Focusable(input.CursorPointer)
		other = o.ID()
	}, DriverOptions{Width: 300, Height: 120})
	focusWidget(t, d, w)
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyTab, 0)
	if d.r.c.focus != w.id {
		t.Fatal("Esc, Tab left a widget that claims Esc")
	}
	d.Key(input.KeyTab, input.ModCtrl)
	if d.r.c.focus != other {
		t.Fatalf("Ctrl+Tab: focus %v, want %v", d.r.c.focus, other)
	}
}

func TestClaimEscapeKeepsWantBack(t *testing.T) {
	open := true
	w := &escWidget{}
	d := NewDriver(func(c *Context) {
		Modal(c, &open, "对话框").Body(func() { w.build(c) })
	}, DriverOptions{Width: 800, Height: 600})
	focusWidget(t, d, w)
	without := d.r.c.WantBack()
	w.claim = true
	d.Settle()
	if got := d.r.c.WantBack(); got != without || !got {
		t.Fatalf("WantBack with claim %v, without %v (want both true)", got, without)
	}
	if !d.BackHandling() {
		t.Fatal("back handling off with a modal open")
	}
	// Keyboard Esc goes to the widget, the system Back closes the modal.
	d.Key(input.KeyEscape, 0)
	if !open || w.keys != 1 {
		t.Fatalf("keyboard Esc: open=%v widget Esc=%d", open, w.keys)
	}
	back(d)
	if open {
		t.Fatal("Back did not close the modal under a widget claiming Esc")
	}
	if w.backs != 0 {
		t.Fatal("Back reached the claiming widget")
	}
}

// A page using ClaimBack still sees Back through Escape while a widget that
// claims Esc has focus; a keyboard Esc goes to the widget instead.
func TestClaimEscapeKeepsClaimBack(t *testing.T) {
	w := &escWidget{claim: true}
	pageBacks := 0
	d := NewDriver(func(c *Context) {
		c.ClaimBack()
		w.build(c)
		if c.Escape() {
			pageBacks++
		}
	}, DriverOptions{Width: 300, Height: 120})
	focusWidget(t, d, w)
	if !d.r.c.WantBack() {
		t.Fatal("WantBack off with ClaimBack")
	}
	d.Key(input.KeyEscape, 0)
	if pageBacks != 0 || w.keys != 1 {
		t.Fatalf("keyboard Esc: page %d widget %d, want 0 1", pageBacks, w.keys)
	}
	back(d)
	if pageBacks != 1 || w.keys != 1 {
		t.Fatalf("Back: page %d widget %d, want 1 1", pageBacks, w.keys)
	}
}

// Back with no claim keeps Code KeyBack in the ordered stream (the runner
// does not overwrite a set Code).
func TestBackKeepsCode(t *testing.T) {
	var codes []input.Key
	var id ID
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(200, 40))
		b.Focusable(input.CursorText)
		id = b.ID()
		for _, e := range c.EventsFor(id) {
			if k, ok := e.(input.KeyEvent); ok && k.Down {
				codes = append(codes, k.Code)
			}
		}
	}, DriverOptions{Width: 300, Height: 120})
	d.Settle()
	d.Click(geom.Pt(20, 20))
	d.Settle()
	back(d)
	if len(codes) != 1 || codes[0] != input.KeyBack {
		t.Fatalf("codes %v, want [Back]", codes)
	}
}

// Back's release follows its press away from a widget that claims Esc: the
// claimer sees no KeyBack event at all, while a keyboard Esc reaches it
// both down and up.
func TestClaimEscapeDropsBackRelease(t *testing.T) {
	open := true
	var id ID
	var keys, evs []input.KeyEvent
	d := NewDriver(func(c *Context) {
		Modal(c, &open, "对话框").Body(func() {
			b := Div(c, layout.Size(200, 40))
			b.Focusable(input.CursorText)
			id = b.ID()
			c.ClaimEscape(id)
			keys = append(keys, c.KeysFor(id)...)
			for _, e := range c.EventsFor(id) {
				if k, ok := e.(input.KeyEvent); ok {
					evs = append(evs, k)
				}
			}
		})
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	d.r.c.SetFocus(id)
	d.Settle()
	d.Key(input.KeyEscape, 0)
	if !open || len(keys) != 2 || !keys[0].Down || keys[1].Down || len(evs) != 2 {
		t.Fatalf("keyboard Esc: open=%v keys %+v evs %+v, want press and release", open, keys, evs)
	}
	keys, evs = nil, nil
	back(d)
	if open {
		t.Fatal("Back did not close the modal")
	}
	for _, k := range append(keys, evs...) {
		if k.Code == input.KeyBack {
			t.Fatalf("claimer saw a Back event: %+v", k)
		}
	}
}
