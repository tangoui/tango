package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Remote-desktop style apps forward keys by their physical position, so the
// HID and native codes from the backend must survive the runner's Key folding.
func TestKeyEventKeepsPhysicalCodes(t *testing.T) {
	var got []input.Event
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(200, 40))
		b.Focusable(input.CursorText)
		got = append(got, c.EventsFor(b.ID())...)
	}, DriverOptions{Width: 300, Height: 100})
	d.Frame()
	d.Click(geom.Pt(20, 20))
	d.Frame()
	got = nil
	d.Inject(input.KeyEvent{Key: input.KeyNumpadEnter, Down: true, HID: 0x58, Native: 0xE01C})
	d.Frame()
	if len(got) != 1 {
		t.Fatalf("events %v, want one key", got)
	}
	k, ok := got[0].(input.KeyEvent)
	if !ok {
		t.Fatalf("event %T, want KeyEvent", got[0])
	}
	if k.Key != input.KeyEnter || k.Code != input.KeyNumpadEnter {
		t.Errorf("Key, Code = %v, %v; want Enter, NumpadEnter", k.Key, k.Code)
	}
	if k.HID != 0x58 || k.Native != 0xE01C {
		t.Errorf("HID, Native = %#x, %#x; want 0x58, 0xe01c", k.HID, k.Native)
	}
}

func TestPointerEventsReturnsEveryMoveInOrder(t *testing.T) {
	var got []input.Pointer
	var last geom.Point
	d := NewDriver(func(c *Context) {
		Div(c, layout.Size(200, 40))
		got = append(got, c.PointerEvents()...)
		last, _ = c.Pointer()
	}, DriverOptions{Width: 300, Height: 100})
	d.Frame()
	got = nil
	for i := 1; i <= 3; i++ {
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(float32(10*i), 10), Time: time.Duration(i) * time.Millisecond})
	}
	d.Frame()
	if len(got) != 3 {
		t.Fatalf("PointerEvents() = %d events, want 3", len(got))
	}
	for i, p := range got {
		want := geom.Pt(float32(10*(i+1)), 10)
		if p.Kind != input.Move || p.Pos != want || p.Time != time.Duration(i+1)*time.Millisecond {
			t.Errorf("event %d = %+v, want Move at %v, time %dms", i, p, want, i+1)
		}
	}
	if last != geom.Pt(30, 10) {
		t.Errorf("Pointer() = %v, want the last move (30,10)", last)
	}
	got = nil
	d.Frame()
	if len(got) != 0 {
		t.Errorf("PointerEvents() after an idle frame = %d events, want 0", len(got))
	}
}
