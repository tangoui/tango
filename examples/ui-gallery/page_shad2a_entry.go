package main

// shad2a: missing theme variants of the basic data-entry pages
// (docs/specs/ui-coverage.md §5) and a shadcn-focused Button / choice /
// input states page.
//
// The variants copy pages registered by other files: Go runs init
// functions file by file in name order, and page_shad2a_* sorts after the
// page_entry_*, page_fill_*, page_general and page_more_* files that
// register them.

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// shad2aVariant registers Name-<suffix> for an already registered page,
// rendered with mk's theme.
func shad2aVariant(name, suffix, title string, mk func() *theme.Theme) {
	for _, p := range gallery.Pages() {
		if p.Name != name || p.Variant {
			continue
		}
		for _, q := range gallery.Pages() {
			if q.Name == name+suffix {
				return // already provided by the page's own file
			}
		}
		v := p
		v.Variant = true
		v.Name, v.Title = p.Name+suffix, p.Title+title
		inner := p.Interact
		if inner == nil {
			inner = p.Setup
		}
		v.Interact = nil
		v.Setup = func(d *ui.Driver) {
			t := mk()
			t.BgLayout = t.BgContainer
			d.SetTheme(t)
			d.Settle()
			if inner != nil {
				inner(d)
			}
		}
		gallery.Register(v)
		return
	}
}

func init() {
	shad2aStatesPage()
	el := func() *theme.Theme { return theme.ElementPlus() }
	antDark := func() *theme.Theme { return theme.AntDesign(theme.Dark()) }
	elDark := func() *theme.Theme { return theme.ElementPlus(theme.Dark()) }
	for _, n := range []string{"Button", "Checkbox", "Input", "InputNumber", "Select", "Switch", "ShadButtonStates"} {
		shad2aVariant(n, "-Element", "（Element Plus）", el)
	}
	for _, n := range []string{"Button", "Checkbox", "Input", "InputNumber", "Select", "Switch", "Radio",
		"CheckableTag", "ColorPickerPanel", "VirtualSelect", "IconButton", "ShadButtonStates"} {
		shad2aVariant(n, "-Dark", "（暗色）", antDark)
	}
	for _, n := range []string{"Button", "Input", "Select"} {
		shad2aVariant(n, "-element-dark", "（Element Plus 暗色）", elDark)
	}
}

var (
	shad2aChk     = true
	shad2aChkOff  bool
	shad2aOn      = true
	shad2aOff     bool
	shad2aText    = "tango"
	shad2aBad     = "not-an-email"
	shad2aRadio   = "b"
	shad2aNum     = 3.0
	shad2aSel     = "b"
	shad2aTagOn   = true
	shad2aTagOff  bool
	shad2aRateVal = 2.5
)

// shad2aStatesPage shows every shadcn button variant and size and the
// focus, error and disabled states of the choice and input controls.
func shad2aStatesPage() {
	p := gallery.Page{Name: "ShadButtonStates", Title: "Button / 选择控件状态", Group: gallery.General, Order: 1, Parent: "Button",
		Render: func(c *ui.Context) { ui.Column(c, func() { shad2aStates(c) }, layout.Gap(16)) },
		Interact: func(d *ui.Driver) {
			d.Key(input.KeyTab, 0) // keyboard focus on "Focused" shows the ring
			d.Settle()
		},
	}
	gallery.Register(p)
}

func shad2aStates(c *ui.Context) {
	row := func(fn func()) { ui.Row(c, fn, layout.Gap(12), layout.WrapItems(), layout.Middle()) }
	row(func() {
		ui.Button(c, "Focused") // first Tab stop: shows the focus ring
		ui.Button(c, "Destructive").Primary().Danger()
		ui.Button(c, "Disabled").Disabled(true)
		ui.Button(c, "Disabled").Primary().Disabled(true)
		ui.Button(c, "Loading").Loading(true)
	})
	row(func() {
		ui.Button(c, "Default").Primary()
		ui.Button(c, "Secondary").Secondary()
		ui.Button(c, "Outline")
		ui.Button(c, "Ghost").Text()
		ui.Button(c, "Link").Link()
		ui.Button(c, "Destructive").Primary().Danger()
	})
	row(func() {
		ui.Button(c, "Small").Small()
		ui.Button(c, "Middle")
		ui.Button(c, "Large").Large()
		ui.Button(c, "").Icon(icon.Plus).Small().A11yLabel("add")
		ui.Button(c, "").Icon(icon.Plus).A11yLabel("add")
		ui.Button(c, "").Icon(icon.Plus).Large().A11yLabel("add")
		ui.Button(c, "Mail").Icon(icon.Mail).Small()
		ui.Button(c, "Mail").Icon(icon.Mail)
		ui.Button(c, "Mail").Icon(icon.Mail).Large()
	})
	row(func() {
		ui.Checkbox(c, "Checked", &shad2aChk)
		ui.Checkbox(c, "Invalid", &shad2aChkOff).Status("error")
		ui.Checkbox(c, "Disabled", &shad2aChk).Disabled(true)
		ui.RadioGroup(c, &shad2aRadio, ui.Opts("a", "b", "c"))
		ui.Radio(c, "Invalid", false).Status("error")
		ui.Switch(c, &shad2aOn).A11yLabel("on")
		ui.Switch(c, &shad2aOff).A11yLabel("off")
		ui.Switch(c, &shad2aOn).Small().A11yLabel("small")
		ui.Switch(c, &shad2aOn).Disabled(true).A11yLabel("disabled")
	})
	row(func() {
		ui.Input(c, &shad2aText).Width(200).A11yLabel("text")
		ui.Input(c, &shad2aBad).Width(200).Status("error").A11yLabel("email")
		ui.Input(c, &shad2aText).Width(200).Disabled(true).A11yLabel("disabled")
		ui.InputNumber(c, &shad2aNum).Width(120).A11yLabel("number")
	})
	row(func() {
		ui.Select(c, &shad2aSel, ui.Opts("a", "b", "c")).Width(180).A11yLabel("select")
		ui.Select(c, &shad2aSel, ui.Opts("a", "b", "c")).Width(180).Status("error").A11yLabel("select error")
		ui.CheckableTag(c, "Selected", &shad2aTagOn)
		ui.CheckableTag(c, "Outline", &shad2aTagOff)
		ui.Rate(c, &shad2aRateVal).AllowHalf()
	})
}
