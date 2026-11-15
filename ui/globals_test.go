package ui

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestGlobalLayersLazyOrderedIsolated(t *testing.T) {
	var c Context
	var got []int
	renderGlobals(&c)
	for _, f := range c.globalLayers {
		if f != nil {
			t.Fatal("new context has a renderer")
		}
	}
	// Install in reverse order; the existing fixed stacking order must win.
	c.useGlobalLayer(globalNotification, func(*Context) { got = append(got, 3) })
	c.useGlobalLayer(globalMessage, func(*Context) { got = append(got, 2) })
	c.useGlobalLayer(globalContextMenu, func(*Context) { got = append(got, 1) })
	c.useGlobalLayer(globalConfirm, func(*Context) { got = append(got, 0) })
	c.useGlobalLayer(globalMessage, func(*Context) { got = append(got, 2) })
	renderGlobals(&c)
	if !reflect.DeepEqual(got, []int{0, 1, 2, 3}) {
		t.Fatalf("order/duplicate: %v", got)
	}
	var other Context
	renderGlobals(&other)
	if len(got) != 4 {
		t.Fatal("renderer leaked into another window")
	}
}

func TestGlobalLayerInstallsLaterLayerDuringRender(t *testing.T) {
	var c Context
	var got []int
	c.useGlobalLayer(globalConfirm, func(c *Context) {
		got = append(got, 0)
		c.useGlobalLayer(globalNotification, func(*Context) { got = append(got, 3) })
	})
	renderGlobals(&c)
	if !reflect.DeepEqual(got, []int{0, 3}) {
		t.Fatalf("later layer did not render in the same pass: %v", got)
	}
}

func TestFeedbackInstallsItsOwnLayerSameFrame(t *testing.T) {
	cases := []struct {
		name  string
		layer globalLayer
		open  func(*Context)
	}{
		{"confirm", globalConfirm, func(c *Context) { Confirm(c, ConfirmOptions{Title: "dialog"}) }},
		{"context", globalContextMenu, func(c *Context) { ContextMenuAt(c, 1, geom.Pt(10, 10), []MenuItem{{Key: "one", Label: "one"}}, nil) }},
		{"message", globalMessage, func(c *Context) { Message(c, "success", "saved") }},
		{"notification", globalNotification, func(c *Context) { Notification(c, "info", "news") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ctx *Context
			d := driver(func(c *Context) {
				ctx = c
				if c.pass == 1 {
					tc.open(c)
				}
			})
			d.Frame()
			if ctx.globalLayers[tc.layer] == nil {
				t.Fatal("opening API did not install renderer in the first frame")
			}
			for i, f := range ctx.globalLayers {
				if i != int(tc.layer) && f != nil {
					t.Fatalf("unrelated layer %d installed", i)
				}
			}
			d.Frame() // remains installed for later animation/close frames
			if ctx.globalLayers[tc.layer] == nil {
				t.Fatal("renderer disappeared on next frame")
			}
		})
	}
}
