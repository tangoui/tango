// The documentation playground renders the public Tango components unchanged.
package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"golang.org/x/image/font/gofont/goregular"
)

func init() {
	// Browser hosts cannot discover desktop fonts. Go Regular is distributed
	// with x/image under its existing license; no font download is required.
	if err := text.RegisterFont("Go", goregular.TTF); err != nil {
		panic(err)
	}
}

type demoState struct {
	component string
	clicks    int
	name      string
	enabled   bool
	level     float64
	language  string
	tab       string
}

func newDemo(name string) *demoState {
	return &demoState{component: name, level: 35, tab: "overview"}
}

func (s *demoState) page(c *ui.Context) {
	if strings.HasPrefix(s.component, "gallery:") {
		name := strings.TrimPrefix(s.component, "gallery:")
		p, ok := gallery.Find(name)
		if !ok || p.Render == nil {
			ui.Text(c, "Unknown component")
			return
		}
		if p.Bare {
			// A bare scene omits the title frame, not the scroll viewport. Honor
			// its authored height so complete dashboards remain reachable.
			ui.Scroll(c, func() { ui.Div(c, layout.H(max(p.Height, c.Size().H))).Body(func() { p.Render(c) }) }, layout.Grow(1))
			return
		}
		ui.Scroll(c, func() { gallery.Frame(c, p.Title, func() { p.Render(c) }) }, layout.Grow(1))
		return
	}
	ui.Column(c, func() {
		switch s.component {
		case "input":
			ui.Title(c, "Input", 4)
			ui.Input(c, &s.name).Placeholder("Type here...").AllowClear().Layout(layout.MaxW(440))
			ui.Text(c, "Value: "+s.name)
			ui.Text(c, "Supports typing, selection, paste and IME.").Secondary()
		case "switch":
			ui.Title(c, "Switch", 4)
			ui.Switch(c, &s.enabled)
			ui.Text(c, fmt.Sprintf("Enabled: %t", s.enabled))
			ui.Text(c, "Click the switch or focus it and press Space.").Secondary()
		case "slider":
			ui.Title(c, "Slider", 4)
			ui.Slider(c, &s.level, 0, 100)
			ui.Text(c, fmt.Sprintf("Value: %.0f", s.level))
			ui.Text(c, "Drag the handle or use the arrow keys.").Secondary()
		case "select":
			ui.Title(c, "Select", 4)
			ui.Select(c, &s.language, ui.Opts("Go", "TypeScript", "Python")).Placeholder("Choose a language").Searchable().AllowClear().Layout(layout.MaxW(440))
			ui.Text(c, "Selected: "+s.language)
		case "tabs":
			ui.Title(c, "Tabs", 4)
			ui.Tabs(c, &s.tab, []ui.TabItem{{Key: "overview", Label: "Overview"}, {Key: "events", Label: "Events"}, {Key: "settings", Label: "Settings"}}).Show()
			switch s.tab {
			case "events":
				ui.Text(c, "Calls, events and streams")
			case "settings":
				ui.Text(c, "Theme and application settings")
			default:
				ui.Text(c, "Build your interface with Tango")
			}
		default:
			ui.Title(c, "Button", 4)
			ui.Row(c, func() {
				if ui.Button(c, "Click me").Primary().Clicked() {
					s.clicks++
				}
				if ui.Button(c, "Default").Clicked() {
					s.clicks++
				}
				if ui.Button(c, "Danger").Danger().Clicked() {
					s.clicks++
				}
				if ui.Button(c, "Disabled").Disabled(true).Clicked() {
					s.clicks++
				}
			}, layout.Gap(12), layout.WrapItems())
			ui.Text(c, fmt.Sprintf("Clicks: %d", s.clicks))
			ui.Text(c, "These are real Go controls rendered in your browser.").Secondary()
		}
	}, layout.Pad(24), layout.Gap(18))
}
