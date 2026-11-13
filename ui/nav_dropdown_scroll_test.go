package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestDropdownLongMenuScrollsToKeyboardItem(t *testing.T) {
	var items []MenuItem
	for i := 0; i < 40; i++ {
		items = append(items, MenuItem{Key: fmt.Sprint(i), Label: fmt.Sprintf("选项 %02d", i)})
	}
	var dd *DropdownW
	d := NewDriver(func(c *Context) {
		dd = Dropdown(c, items).Trigger(TriggerClick).MaxHeight(240).Body(func() { Button(c, "长菜单") })
	}, DriverOptions{Width: 400, Height: 600})
	d.Frame()
	d.ClickLabel("长菜单")
	if !dd.IsOpen() {
		t.Fatal("not open")
	}
	panel, _ := d.r.c.RectOf(dd.st.panels[0])
	if panel.H > 241 || panel.H < 200 {
		t.Fatalf("panel not height limited: %v", panel)
	}
	if r, ok := d.Find("选项 30"); ok && !r.Empty() {
		t.Fatalf("item 30 should start hidden below the fold: %v", r)
	}
	// Tab to the trigger; Down enters the menu on its first item.
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	if !dd.IsOpen() {
		t.Fatal("Down did not open")
	}
	for i := 0; i < 30; i++ /* 选项 00 → 选项 30 */ {
		d.Key(input.KeyDown, 0)
	}
	r, ok := d.Find("选项 30")
	if !ok {
		t.Fatal("item 30 not painted")
	}
	vis := r.Intersect(panel)
	if vis.H < r.H-0.5 {
		t.Fatalf("keyboard item not scrolled into view: item %v panel %v", r, panel)
	}
	// Wheel scrolls the panel too.
	d.Scroll(panel.Center(), -2000)
	if r, _ := d.Find("选项 00"); r.Intersect(panel).H < r.H-0.5 {
		t.Fatalf("wheel did not scroll back to top: %v", r)
	}
}
