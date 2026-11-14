package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func TestMessageBoxDistinguishCancelAndClose(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		cancels, closes := 0, 0
		var ref *ConfirmRef
		d := NewDriver(moreBoxPage(func(c *Context) {
			ref = Confirm(c, ConfirmOptions{Title: "Leave", Content: "Unsaved changes", DistinguishCancelAndClose: true,
				MaskClosable: true, OnCancel: func() { cancels++ }, OnClose: func() { closes++ }})
		}), DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Frame()
		d.ClickLabel("打开")
		d.ClickLabel("取消")
		if cancels != 1 || closes != 0 || ref.Action() != ConfirmActionCancel {
			t.Fatalf("%s cancel: cancels=%d closes=%d action=%q", th.Name, cancels, closes, ref.Action())
		}
		d.ClickLabel("打开")
		d.Key(input.KeyEscape, 0)
		if cancels != 1 || closes != 1 || ref.Action() != ConfirmActionClose {
			t.Fatalf("%s escape: cancels=%d closes=%d action=%q", th.Name, cancels, closes, ref.Action())
		}
		d.ClickLabel("打开")
		d.Click(geom.Pt(5, 395))
		if closes != 2 || ref.IsOpen() {
			t.Fatalf("%s mask: closes=%d", th.Name, closes)
		}
		if th.Flavor == theme.Element {
			d.ClickLabel("打开")
			d.ClickLabel(ZhCN.Close)
			if closes != 3 || cancels != 1 {
				t.Fatalf("element close button: closes=%d cancels=%d", closes, cancels)
			}
		}
		d.ClickLabel("打开")
		d.ClickLabel("确定")
		if ref.Action() != ConfirmActionConfirm {
			t.Fatalf("%s ok: action=%q", th.Name, ref.Action())
		}
	}
	// Without the flag Escape stays a cancel.
	cancels := 0
	var ref *ConfirmRef
	d := driver(moreBoxPage(func(c *Context) {
		ref = Confirm(c, ConfirmOptions{Title: "Plain", OnCancel: func() { cancels++ }, OnClose: func() { t.Fatal("OnClose without flag") }})
	}))
	d.Frame()
	d.ClickLabel("打开")
	d.Key(input.KeyEscape, 0)
	if cancels != 1 || ref.Action() != ConfirmActionCancel {
		t.Fatalf("escape without flag: %d %q", cancels, ref.Action())
	}
}

func TestMessageBoxCenter(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(moreBoxPage(func(c *Context) {
			MessageBox(c, ConfirmOptions{Title: "Centered title", Content: "Body", Center: true})
		}), DriverOptions{Width: 800, Height: 500, Theme: th})
		d.Frame()
		d.ClickLabel("打开")
		title, ok := d.FindText("Centered title")
		ok2r, ok2 := d.Find("确定")
		if !ok || !ok2 {
			if !ok2 {
				ok2r, ok2 = d.Find(ZhCN.GotIt)
			}
			if !ok || !ok2 {
				t.Fatalf("%s: dialog parts missing", th.Name)
			}
		}
		// Dialogs are centered horizontally in the 800px window.
		if c := cxOf(title); c < 360 || c > 440 {
			t.Errorf("%s: title center %v", th.Name, c)
		}
		if c := cxOf(ok2r); c < 370 || c > 430 {
			t.Errorf("%s: button center %v", th.Name, c)
		}
	}
}

func TestMessageBoxDraggable(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(moreBoxPage(func(c *Context) {
			MessageBox(c, ConfirmOptions{Title: "Drag me", Content: "Body", Draggable: true})
		}), DriverOptions{Width: 800, Height: 600, Theme: th})
		d.Frame()
		d.ClickLabel("打开")
		before, ok := d.FindText("Drag me")
		if !ok {
			t.Fatal("title missing")
		}
		from := before.Center()
		d.Drag(from, geom.Pt(from.X-120, from.Y+80))
		d.Settle()
		after, _ := d.FindText("Drag me")
		if dx, dy := after.X-before.X, after.Y-before.Y; dx > -100 || dy < 60 {
			t.Fatalf("%s: dragged by %v,%v", th.Name, dx, dy)
		}
	}
	// Not draggable by default.
	d := NewDriver(moreBoxPage(func(c *Context) {
		MessageBox(c, ConfirmOptions{Title: "Fixed", Content: "Body"})
	}), DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.ClickLabel("打开")
	before, _ := d.FindText("Fixed")
	d.Drag(before.Center(), geom.Pt(before.X-100, before.Y+80))
	d.Settle()
	if after, _ := d.FindText("Fixed"); after != before {
		t.Fatalf("non-draggable dialog moved: %v -> %v", before, after)
	}
}

func TestMessageBoxCustomIcon(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(moreBoxPage(func(c *Context) {
			MessageBox(c, ConfirmOptions{Title: "Custom", Content: "Body", Status: "success", Icon: icon.Star})
		}), DriverOptions{Width: 800, Height: 600, Theme: th})
		d.Frame()
		d.ClickLabel("打开")
		if _, ok := d.Find("success"); ok {
			t.Errorf("%s: status icon drawn despite Icon", th.Name)
		}
		if t2bCount(d, th.Success.Base) == 0 {
			t.Errorf("%s: custom icon not drawn in the status color", th.Name)
		}
	}
}
