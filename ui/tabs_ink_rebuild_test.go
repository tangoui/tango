package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/layout"
)

// The ink bar slides only when the active tab changes. A bar whose tabs
// move without a new active tab (a page switched back to while its sidebar
// settles, a resize, stretched tabs reflowing) draws the bar in place at
// once: a 300 ms slide there kept the window drawing at display rate, and
// the switch-back frame queued behind it.
func TestTabsInkBarSlidesOnlyOnActiveChange(t *testing.T) {
	width := float32(320)
	act := "active"
	items := []TabItem{{Key: "active", Label: "Active"}, {Key: "archived", Label: "Archived"}}
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			// As MinCode's AI sidebar scope tabs: stretched over the sidebar.
			Tabs(c, &act, items).Size(Small).Stretch().TabBarGutter(0).Indicator(22, "start").
				NoMargin().TabItemHeight(30).ActiveTabFontWeight(600).TabBarStyle(layout.H(30)).Show()
		}, layout.W(width))
	}, DriverOptions{Animations: true})
	d.Settle()
	if d.Runner().Animating() {
		t.Fatal("ink bar still animating after the first settle")
	}
	frame := func() {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}

	// The tabs reflow with the same active tab: no slide.
	width = 260
	frame()
	frame()
	if d.Runner().Animating() {
		t.Error("the ink bar slides after a reflow without a new active tab")
	}

	// A new active tab slides, and a reflow during the slide does not cut it.
	act = "archived"
	frame()
	frame()
	if !d.Runner().Animating() {
		t.Fatal("changing the active tab no longer animates the ink bar")
	}
	width = 300
	frame()
	if !d.Runner().Animating() {
		t.Error("a reflow during the slide ended it")
	}
	d.Advance(time.Second)
	d.Settle()
	if d.Runner().Animating() {
		t.Error("the slide did not end")
	}
}
