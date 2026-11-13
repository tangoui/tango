package main

import (
	"testing"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

func TestMobileGalleryKeepsInteractiveStateAcrossFrames(t *testing.T) {
	page, ok := gallery.Find("MobileList")
	if !ok {
		t.Fatal("MobileList missing")
	}
	d := ui.NewDriver(page.Render, ui.DriverOptions{Width: 440, Height: 940, Accessibility: true})
	defer d.Runner().Close()
	d.Frame()
	initial, ok := d.A11yFind(a11y.RoleSwitch, "无线局域网")
	if !ok {
		t.Fatal("Wi-Fi switch is missing")
	}
	before := initial.States & a11y.Checked
	if err := d.A11yPerform(a11y.RoleSwitch, "无线局域网", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	current, ok := d.A11yFind(a11y.RoleSwitch, "无线局域网")
	if !ok || current.States&a11y.Checked == before {
		t.Fatal("mobile gallery discarded the changed state on the next frame")
	}
	if err := d.A11yPerform(a11y.RoleSwitch, "无线局域网", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
}
