package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestDrawerResizeDrag: dragging the page-facing edge resizes the drawer
// within its limits, writes the bound size and shows a resize cursor.
func TestDrawerResizeDrag(t *testing.T) {
	open := true
	var size float32 = 300
	var dw *DrawerW
	resized := 0
	place := DrawerRight
	d := navDriver(func(c *Context) {
		dw = Drawer(c, &open, "抽屉").Placement(place).Resizable(true).SizeLimits(200, 500).BindSize(&size)
		dw.Body(func() { Text(c, "内容") })
		if dw.Resized() {
			resized++
		}
	})
	d.Frame()
	d.Frame()
	h := mustFind(t, d, "drawer-resize:抽屉")
	if !near(h.X, 960-300) {
		t.Fatalf("handle not on the left edge of a right drawer: %v", h)
	}
	d.Move(h.Center())
	if d.Cursor() != input.CursorResizeEW {
		t.Fatalf("cursor = %v", d.Cursor())
	}
	navDrag(d, h.Center(), geom.Pt(h.Center().X-100, h.Center().Y))
	d.Frame()
	if !near(size, 400) || !near(dw.CurrentSize(), 400) || resized == 0 {
		t.Fatalf("after drag size=%v cur=%v resized=%d", size, dw.CurrentSize(), resized)
	}
	// Past the maximum the size stops at the limit.
	h = mustFind(t, d, "drawer-resize:抽屉")
	navDrag(d, h.Center(), geom.Pt(h.Center().X-400, h.Center().Y))
	d.Frame()
	if !near(size, 500) {
		t.Fatalf("max not honored: %v", size)
	}
	h = mustFind(t, d, "drawer-resize:抽屉")
	navDrag(d, h.Center(), geom.Pt(h.Center().X+600, h.Center().Y))
	d.Frame()
	if !near(size, 200) {
		t.Fatalf("min not honored: %v", size)
	}

	// Bottom drawer: dragging the top edge up grows the height.
	place, size = DrawerBottom, 250
	d.Frame()
	d.Frame()
	h = mustFind(t, d, "drawer-resize:抽屉")
	if !near(h.Y, 600-250) {
		t.Fatalf("bottom handle %v", h)
	}
	d.Move(h.Center())
	if d.Cursor() != input.CursorResizeNS {
		t.Fatalf("bottom cursor = %v", d.Cursor())
	}
	navDrag(d, h.Center(), geom.Pt(h.Center().X, h.Center().Y-50))
	d.Frame()
	if !near(size, 300) {
		t.Fatalf("bottom drag size=%v", size)
	}
}

// TestDrawerResizeKeyboard: the edge is a focusable separator; arrows step
// the size, Home / End reach the limits, and the close button keeps the
// initial focus.
func TestDrawerResizeKeyboard(t *testing.T) {
	open := true
	var dw *DrawerW
	d := navDriver(func(c *Context) {
		dw = Drawer(c, &open, "抽屉").Size(300).Resizable(true).SizeLimits(200, 400)
		dw.Body(func() { Button(c, "确定") })
	})
	d.EnableAccessibility()
	d.Settle()
	sep, ok := d.A11yFind(a11y.RoleSeparator, ZhCN.A11y.Resize)
	if !ok {
		t.Fatal("no resize separator")
	}
	if f := d.A11yFocused(); f != nil && f.ID == sep.ID {
		t.Fatal("the resize edge took the initial focus")
	}
	if sep.ValueNow != 300 || sep.ValueMin != 200 || sep.ValueMax != 400 {
		t.Fatalf("range %v %v %v", sep.ValueNow, sep.ValueMin, sep.ValueMax)
	}
	d.PerformAction(sep.ID, a11y.Focus, "")
	d.Settle()
	d.Key(input.KeyLeft, 0) // towards the page: grows a right drawer
	if !near(dw.CurrentSize(), 310) {
		t.Fatalf("left arrow: %v", dw.CurrentSize())
	}
	d.Key(input.KeyRight, input.ModShift)
	if !near(dw.CurrentSize(), 260) {
		t.Fatalf("shift+right: %v", dw.CurrentSize())
	}
	d.Key(input.KeyEnd, 0)
	if !near(dw.CurrentSize(), 400) {
		t.Fatalf("end: %v", dw.CurrentSize())
	}
	d.Key(input.KeyHome, 0)
	if !near(dw.CurrentSize(), 200) {
		t.Fatalf("home: %v", dw.CurrentSize())
	}
	// Escape still closes from the handle.
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("escape did not close")
	}
}
