package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// g4Scoped builds fn twice: once inside a nested provider overriding the
// component tokens cs, once outside it.
func g4Scoped(c *Context, cs theme.Components, fn func(scoped bool)) {
	Column(c, func() {
		ConfigProvider(c).Components(cs).Body(func() { fn(true) })
		fn(false)
	}, layout.Gap(40), layout.Pad(40))
}

// g4CountFill counts FillRect and FillPath ops of color col.
func g4CountFill(d *Driver, col theme.Color) int {
	n := 0
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col {
				n++
			}
		case paint.FillPath:
			if o.Color == col {
				n++
			}
		}
	}
	return n
}

var g4Mark = theme.Hex("#12ab34")

// TestG4TokenOverridesAreScoped overrides one color token per component in
// a nested ConfigProvider and checks that only the scoped instance paints
// it, while the instance outside keeps the derived value.
func TestG4TokenOverridesAreScoped(t *testing.T) {
	th := theme.AntDesign()
	for _, tc := range []struct {
		name string
		cs   theme.Components
		def  theme.Color
		fn   func(c *Context)
	}{
		{"Tag", theme.Components{Tag: theme.TagToken{DefaultBg: g4Mark}}, th.Components.Tag.DefaultBg,
			func(c *Context) { Tag(c, "tag") }},
		{"Card", theme.Components{Card: theme.CardToken{Bg: g4Mark}}, th.Components.Card.Bg,
			func(c *Context) { Card(c, "card").Body(func() { Text(c, "x") }) }},
		{"Avatar", theme.Components{Avatar: theme.AvatarToken{Bg: g4Mark}}, th.Components.Avatar.Bg,
			func(c *Context) { Avatar(c, "A") }},
		{"Skeleton", theme.Components{Skeleton: theme.SkeletonToken{GradientFromColor: g4Mark}}, th.Components.Skeleton.GradientFromColor,
			func(c *Context) { SkeletonButton(c, Middle, "default", false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(func(c *Context) {
				g4Scoped(c, tc.cs, func(bool) { tc.fn(c) })
			}, DriverOptions{Width: 600, Height: 600, Theme: th})
			d.Settle()
			if n := g4CountFill(d, g4Mark); n != 1 {
				t.Errorf("override painted %d times, want once (inside the scope)", n)
			}
			if n := g4CountFill(d, tc.def); n < 1 {
				t.Errorf("default %v not painted outside the scope", tc.def)
			}
		})
	}
}

// TestG4OverlayTokenOverrides: Modal, Drawer, Popover and Tooltip opened
// inside a provider use its tokens; the same widget outside does not.
func TestG4OverlayTokenOverrides(t *testing.T) {
	th := theme.AntDesign()
	for _, tc := range []struct {
		name string
		cs   theme.Components
		fn   func(c *Context, scoped bool)
	}{
		{"Modal", theme.Components{Modal: theme.ModalToken{ContentBg: g4Mark}}, func(c *Context, scoped bool) {
			open := scoped
			Modal(c, &open, "Modal").Body(func() { Text(c, "m") })
		}},
		{"Drawer", theme.Components{Drawer: theme.DrawerToken{Bg: g4Mark}}, func(c *Context, scoped bool) {
			open := scoped
			Drawer(c, &open, "Drawer").Body(func() { Text(c, "d") })
		}},
		{"Popover", theme.Components{Popover: theme.PopoverToken{Bg: g4Mark}}, func(c *Context, scoped bool) {
			open := true
			Popover(c).Text("p").Trigger("manual").Open(&open).Body(func() { Text(c, "trigger") })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(func(c *Context) { g4Scoped(c, tc.cs, func(s bool) { tc.fn(c, s) }) },
				DriverOptions{Width: 800, Height: 600, Theme: th})
			d.Settle()
			d.Settle()
			if n := g4CountFill(d, g4Mark); n < 1 {
				t.Errorf("override not painted")
			}
			if tc.name == "Popover" {
				// Two open popovers: one panel (and its arrow) in the mark color.
				if n := g4CountFill(d, th.Components.Popover.Bg); n < 1 {
					t.Errorf("popover outside the scope lost the default background")
				}
			}
		})
	}
	// Tooltip: hover the scoped trigger.
	var in *TooltipW
	d := NewDriver(func(c *Context) {
		g4Scoped(c, theme.Components{Tooltip: theme.TooltipToken{Bg: g4Mark}}, func(s bool) {
			tt := Tooltip(c, "tip", func() { Text(c, "hover me") })
			if s {
				in = tt
			}
		})
	}, DriverOptions{Width: 600, Height: 400, Theme: th})
	d.Settle()
	r := d.r.c.rects[in.ID()]
	d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
	d.Settle()
	if g4CountFill(d, g4Mark) < 1 {
		t.Error("tooltip override not painted")
	}
}

// TestG4GlobalTokenOverrides: Message and Notification opened inside a
// provider keep its tokens although they render in a global layer.
func TestG4GlobalTokenOverrides(t *testing.T) {
	th := theme.AntDesign()
	d := NewDriver(func(c *Context) {
		s := State[bool](c, hashID("g4-fire"), nil)
		g4Scoped(c, theme.Components{
			Message:      theme.MessageToken{ContentBg: g4Mark},
			Notification: theme.NotificationToken{Bg: g4Mark},
		}, func(scoped bool) {
			if *s {
				return
			}
			Message(c, "info", "msg").Duration(0)
			Notification(c, "info", "note").Duration(0)
			if !scoped {
				*s = true
			}
		})
	}, DriverOptions{Width: 900, Height: 600, Theme: th})
	d.Settle()
	if n := g4CountFill(d, g4Mark); n != 2 {
		t.Errorf("scoped message + notification: %d fills in the override color, want 2", n)
	}
	if n := g4CountFill(d, th.Components.Message.ContentBg); n < 2 {
		t.Errorf("unscoped message and notification lost the default background (%d)", n)
	}
}
