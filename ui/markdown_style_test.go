package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func TestMarkdownCodeStyle(t *testing.T) {
	showLanguage, copyable := true, true
	h := &fakeHL{}
	bg := theme.RGBA(20, 50, 80, 1)
	d := NewDriver(func(c *Context) {
		m := Markdown(c, "```fake\nkw x\n```", layout.Fill()).Virtual().Highlighter(h).CopyCode(copyable)
		if !showLanguage {
			m.CodeLanguage(false).CodeBackground(bg)
		}
	}, DriverOptions{Width: 500, Height: 200, Accessibility: true})
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleNone, "fake"); !ok {
		t.Fatal("default language label missing")
	}
	if h.asked == 0 {
		t.Fatal("fence language did not reach the highlighter")
	}
	showLanguage = false
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleNone, "fake"); ok {
		t.Fatal("language label still present")
	}
	if _, ok := d.Find(defaultLocaleNow().Markdown.CopyCode); !ok {
		t.Fatal("copy control should be independent of language label")
	}
	copyable = false
	d.Settle()
	if _, ok := d.Find(defaultLocaleNow().Markdown.CopyCode); ok {
		t.Fatal("copy control still present")
	}
	for _, op := range d.r.List().Ops {
		if fill, ok := op.(paint.FillRect); ok && fill.Color == bg {
			return
		}
	}
	t.Fatal("configured code background was not painted")
}

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
