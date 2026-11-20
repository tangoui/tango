package ui

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestMenuAccordionArrowMatchesClick(t *testing.T) {
	items := []MenuItem{
		{Key: "a", Label: "A", Children: []MenuItem{{Key: "a1", Label: "A child"}}},
		{Key: "b", Label: "B", Children: []MenuItem{{Key: "b1", Label: "B child"}}},
	}
	var selected string
	var open []string
	d := newPerfDriver(func(c *Context) {
		Menu(c, items, &selected).Accordion().OpenKeys(&open).Show()
	}, false)
	d.Settle()
	if err := d.ClickLabel("A"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(open, []string{"a"}) {
		t.Fatalf("click A opened %v, want [a]", open)
	}
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyRight, 0)
	if !slices.Contains(open, "b") {
		t.Fatalf("fixture did not open B: %v", open)
	}
	got := slices.Clone(open)
	if err := d.ClickLabel("B"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(open, "b") {
		t.Fatalf("click B did not close B: %v", open)
	}
	if err := d.ClickLabel("B"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(open, []string{"b"}) {
		t.Fatalf("mouse control opened %v, want [b]", open)
	}
	if !slices.Equal(got, open) {
		t.Errorf("arrow opened %v; opening B by click leaves %v", got, open)
	}
}

func TestMenuAccordionKeepsParentOpen(t *testing.T) {
	items := []MenuItem{
		{Key: "parent", Label: "Parent", Children: []MenuItem{
			{Key: "first", Label: "First", Children: []MenuItem{{Key: "first-leaf", Label: "First leaf"}}},
			{Key: "second", Label: "Second", Children: []MenuItem{{Key: "second-leaf", Label: "Second leaf"}}},
		}},
	}
	for _, keyboard := range []bool{false, true} {
		name := "click"
		if keyboard {
			name = "arrow"
		}
		t.Run(name, func(t *testing.T) {
			var open []string
			d := newPerfDriver(func(c *Context) {
				Menu(c, items, nil).Accordion().OpenKeys(&open).Show()
			}, false)
			d.Settle()
			if err := d.ClickLabel("Parent"); err != nil {
				t.Fatal(err)
			}
			if keyboard {
				d.Key(input.KeyRight, 0) // enter Parent
				d.Key(input.KeyRight, 0) // open First
				d.Key(input.KeyEnd, 0)   // focus Second
				d.Key(input.KeyRight, 0)
			} else {
				if err := d.ClickLabel("First"); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(open, []string{"parent", "first"}) {
					t.Fatalf("opening First leaves %v, want [parent first]", open)
				}
				if err := d.ClickLabel("Second"); err != nil {
					t.Fatal(err)
				}
			}
			if !slices.Equal(open, []string{"parent", "second"}) {
				t.Errorf("opening Second leaves %v, want [parent second]", open)
			}
		})
	}
}
