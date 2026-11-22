package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestMarkdownReadOnlyTasks(t *testing.T) {
	var toggled bool
	d := NewDriver(func(c *Context) {
		m := Markdown(c, "- [x] Finished\n\n```go\nexample\n```", layout.Fill()).
			ReadOnlyTasks(true).CopyCode(false).ContentWidth(300)
		_, changed := m.TaskToggled()
		toggled = toggled || changed
	}, DriverOptions{Width: 500, Height: 200, Accessibility: true})
	d.Frame()
	check, ok := d.A11yFind(a11y.RoleCheckbox, "Finished")
	if !ok || check.States&(a11y.Checked|a11y.ReadOnly|a11y.Disabled) != a11y.Checked|a11y.ReadOnly|a11y.Disabled {
		t.Fatalf("task semantics: %+v", check)
	}
	d.Click(geom.Pt(check.Bounds.X+check.Bounds.W/2, check.Bounds.Y+check.Bounds.H/2))
	if toggled {
		t.Fatal("read-only task toggled")
	}
	if _, ok := d.Find(defaultLocaleNow().Markdown.CopyCode); ok {
		t.Fatal("hidden code copy control was built")
	}
}
