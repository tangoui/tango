package ui

import (
	"reflect"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
)

func TestCollapseAccordionAndDisabled(t *testing.T) {
	open := []string{"1"}
	accordion := true
	items := func(c *Context) []CollapseItem {
		body := func(s string) func() { return func() { Text(c, s) } }
		return []CollapseItem{
			{Key: "1", Label: "head 1", Body: body("body 1")},
			{Key: "2", Label: "head 2", Body: body("body 2")},
			{Key: "3", Label: "head 3", Body: body("body 3"), Disabled: true},
		}
	}
	changed := 0
	d := driver(func(c *Context) {
		w := Collapse(c, &open, items(c))
		if accordion {
			w.Accordion()
		}
		if w.Show().Changed() {
			changed++
		}
	})
	d.Frame()
	d.Click(mustFind(t, d, "head 2").Center())
	if !reflect.DeepEqual(open, []string{"2"}) || changed != 1 {
		t.Fatalf("accordion open = %v changed=%d", open, changed)
	}
	d.Click(mustFind(t, d, "head 3").Center())
	if !reflect.DeepEqual(open, []string{"2"}) {
		t.Fatalf("disabled panel toggled: %v", open)
	}
	d.Click(mustFind(t, d, "head 2").Center())
	if len(open) != 0 {
		t.Fatalf("close = %v", open)
	}
	accordion = false
	d.Click(mustFind(t, d, "head 1").Center())
	d.Click(mustFind(t, d, "head 2").Center())
	if !reflect.DeepEqual(open, []string{"1", "2"}) {
		t.Fatalf("multi open = %v", open)
	}
}

func TestCollapseHeightAnimation(t *testing.T) {
	open := []string{}
	var box ID
	d := NewDriver(func(c *Context) {
		w := Collapse(c, &open, []CollapseItem{{Key: "k", Label: "head", Body: func() {
			Text(c, "content line")
			Text(c, "content line 2")
		}}}).Show()
		box = w.box.ID()
	}, DriverOptions{Width: 400, Height: 300, Animations: true})
	d.Settle()
	closed, _ := d.Runner().Context().RectOf(box)
	p := mustFind(t, d, "head").Center()
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Frame()
	for i := 0; i < 4; i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	mid, _ := d.Runner().Context().RectOf(box)
	d.Settle()
	full, _ := d.Runner().Context().RectOf(box)
	if !(closed.H < mid.H && mid.H < full.H) {
		t.Fatalf("heights closed=%v mid=%v open=%v", closed.H, mid.H, full.H)
	}
}
