package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
)

// phone opens the page like the Android device run that found the tab
// bar bug: portrait 411×914 with a 52pt status bar and a 48pt navigation
// bar.
func phone(t *testing.T, which string) *ui.Driver {
	t.Helper()
	tab = which
	t.Cleanup(func() { tab = "list" })
	d := ui.NewDriver(page, ui.DriverOptions{Width: 411, Height: 914, Accessibility: true})
	d.SetSafeArea(geom.Insets{Top: 52, Bottom: 48})
	frames(d, 3) // mirrored parts (tab bar, title) follow one frame behind
	return d
}

func node(t *testing.T, d *ui.Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %s %q in tree:\n%s", role, name, d.AccessibilityTree().Dump())
	}
	return n
}

// List rows hidden under the tab bar are offscreen for screen readers,
// and showing one scrolls it above the tab bar.
func TestListRowsUnderTabBar(t *testing.T) {
	d := phone(t, "list")
	bar := node(t, d, a11y.RoleTabList, "")
	top := bar.Bounds.Y
	if bar.States.Has(a11y.Offscreen) || top+bar.Bounds.H != 914 {
		t.Fatalf("tab bar %+v %s", bar.Bounds, bar.States)
	}
	rows := 0
	for _, n := range d.A11yFindAll(a11y.RoleButton, "") {
		if !strings.Contains(n.Name, "Row") {
			continue
		}
		rows++
		if hidden := n.Bounds.Y >= top; n.States.Has(a11y.Offscreen) != hidden {
			t.Errorf("%q at %+v (tab bar at %v): offscreen %v", n.Name, n.Bounds, top, !hidden)
		}
		if !n.Actions.Has(a11y.ScrollIntoView) {
			t.Errorf("%q lacks ScrollIntoView", n.Name)
		}
	}
	if rows != 12 {
		t.Fatalf("%d rows in tree:\n%s", rows, d.AccessibilityTree().Dump())
	}
	const name = "第 10 行 · Row 10"
	if n := node(t, d, a11y.RoleButton, name); !n.States.Has(a11y.Offscreen) {
		t.Fatalf("row 10 visible at %+v", n.Bounds)
	}
	if err := d.A11yPerform(a11y.RoleButton, name, a11y.ScrollIntoView, ""); err != nil {
		t.Fatal(err)
	}
	frames(d, 2)
	if n := node(t, d, a11y.RoleButton, name); n.States.Has(a11y.Offscreen) || n.Bounds.Y+n.Bounds.H > top+0.5 {
		t.Fatalf("row 10 not shown above the tab bar (%v): %+v %s", top, n.Bounds, n.States)
	}
}

// The settings tab's stepper and slider follow a screen reader's
// increment and decrement, and the app logs the stepper's value.
func TestSettingsAdjustable(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	qty0, vol0 := qty, volume
	defer func() { qty, volume = qty0, vol0 }()
	d := phone(t, "settings")
	// The group starts below the fold. Its controls are in the tree all
	// the same (Offscreen), so a screen reader reaches them by swiping;
	// moving to one shows its group.
	for _, c := range []struct {
		role a11y.Role
		name string
	}{{a11y.RoleSpinButton, "数量"}, {a11y.RoleSlider, "音量"}} {
		if n := node(t, d, c.role, c.name); !n.States.Has(a11y.Offscreen) || !n.Actions.Has(a11y.Increment|a11y.Decrement) {
			t.Fatalf("%s %q below the fold: %+v %s %s", c.role, c.name, n.Bounds, n.States, n.Actions)
		}
	}
	if err := d.A11yPerform(a11y.RoleForm, "调节 Adjust", a11y.ScrollIntoView, ""); err != nil {
		t.Fatal(err)
	}
	frames(d, 2)
	if n := node(t, d, a11y.RoleSpinButton, "数量"); n.States.Has(a11y.Offscreen) {
		t.Fatalf("stepper offscreen after showing its group: %+v", n.Bounds)
	}
	if err := d.A11yPerform(a11y.RoleSpinButton, "数量", a11y.Increment, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.A11yPerform(a11y.RoleSpinButton, "数量", a11y.Increment, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.A11yPerform(a11y.RoleSpinButton, "数量", a11y.Decrement, ""); err != nil {
		t.Fatal(err)
	}
	if qty != qty0+1 || !strings.Contains(out.String(), "mobile-ui: stepper 3") {
		t.Fatalf("stepper %v, log:\n%s", qty, out.String())
	}
	if n := node(t, d, a11y.RoleSpinButton, "数量"); n.ValueNow != qty || !n.Actions.Has(a11y.Increment|a11y.Decrement) {
		t.Fatalf("stepper node %s value %v actions %s", n, n.ValueNow, n.Actions)
	}
	if err := d.A11yPerform(a11y.RoleSlider, "音量", a11y.Increment, ""); err != nil {
		t.Fatal(err)
	}
	if volume <= vol0 || !strings.Contains(out.String(), "mobile-ui: slider") {
		t.Fatalf("slider %v, log:\n%s", volume, out.String())
	}
	if p := os.Getenv("MOBILEUI_SETTINGS_PNG"); p != "" {
		if err := d.SavePNG(p); err != nil {
			t.Fatal(err)
		}
	}
}
