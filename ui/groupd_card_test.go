package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestCardTabs(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		active := "a"
		changed := 0
		extraShown := false
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				cw := Card(c, "Projects").Tabs(&active, []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}).
					TabBarExtra(func() { extraShown = true; Link(c, "All") }).Body(func() {
					Text(c, "content-"+active)
				})
				if cw.TabChanged() {
					changed++
				}
			}, layout.Pad(20))
		}, DriverOptions{Width: 600, Height: 400, Theme: th})
		d.EnableAccessibility()
		d.Settle()
		if !extraShown {
			t.Fatalf("%s: tab bar extra not built", th.Name)
		}
		title, _ := d.FindText("Projects")
		tab, ok := d.Find("Alpha")
		if !ok {
			t.Fatalf("%s: tab missing", th.Name)
		}
		body, _ := d.FindText("content-a")
		if !(title.Y < tab.Y && tab.Y < body.Y) {
			t.Fatalf("%s: order title %v tab %v body %v", th.Name, title.Y, tab.Y, body.Y)
		}
		if _, ok := d.A11yFind(a11y.RoleTabPanel, "Alpha"); !ok {
			t.Fatalf("%s: body is not the tab panel", th.Name)
		}
		d.ClickLabel("Beta")
		if active != "b" || changed != 1 {
			t.Fatalf("%s: active %q changed %d", th.Name, active, changed)
		}
		if _, ok := d.FindText("content-b"); !ok {
			t.Fatalf("%s: content did not switch", th.Name)
		}
	}
	// Without a title the tabs are the header.
	active := "x"
	d := driver(func(c *Context) {
		Card(c, "").Tabs(&active, []TabItem{{Key: "x", Label: "Only"}}).Body(func() { Text(c, "body") })
	})
	d.Settle()
	if _, ok := d.Find("Only"); !ok {
		t.Fatal("tab missing on an untitled card")
	}
}
