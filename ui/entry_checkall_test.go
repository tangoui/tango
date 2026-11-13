package ui

import "testing"

func TestEntryCheckAllToggleAndIndeterminate(t *testing.T) {
	opts := []Option{{Label: "苹果", Value: "a"}, {Label: "香蕉", Value: "b"}, {Label: "橙子", Value: "c", Disabled: true}}
	sel := []string{"c"}
	changes := 0
	d := driver(func(c *Context) {
		if CheckAll(c, &sel, opts) {
			changes++
		}
	})
	d.Frame()
	d.ClickLabel("全选")
	if len(sel) != 3 || changes != 1 {
		t.Fatalf("select all: %v (%d)", sel, changes)
	}
	d.ClickLabel("全选")
	if len(sel) != 1 || sel[0] != "c" {
		t.Fatalf("unselect all keeps disabled: %v", sel)
	}
	d.ClickLabel("苹果")
	if len(sel) != 2 {
		t.Fatalf("group toggle: %v", sel)
	}
	// Partially selected: clicking the master selects the rest.
	d.ClickLabel("全选")
	if len(sel) != 3 {
		t.Fatalf("indeterminate -> all: %v", sel)
	}
	got := caSetAll([]string{"x", "b"}, opts, true)
	if len(got) != 3 || got[0] != "x" || got[1] != "a" || got[2] != "b" {
		t.Fatalf("caSetAll = %v", got)
	}
}

func TestEntryCheckAllDisabled(t *testing.T) {
	opts := Opts("a", "b")
	var sel []string
	d := driver(func(c *Context) {
		Disabled(c, true, func() { CheckAll(c, &sel, opts) })
	})
	d.Frame()
	d.ClickLabel("全选")
	if len(sel) != 0 {
		t.Fatal("disabled check-all toggled")
	}
}
