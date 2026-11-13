package main

import (
	"io"
	"log"
	"os"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Back (Escape) closes the top-most overlay first: a dialog opened over
// the popup, then the popup; the window's Back handling follows.
func TestBackClosesOverlays(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	t.Cleanup(func() { popupOpen, dialogOpen, sheetOpen = false, false, false })
	d := phone(t, "list")
	if d.BackHandling() {
		t.Fatal("Back handled with nothing open")
	}
	popupOpen = true
	d.Settle()
	dialogOpen = true
	d.Settle()
	frames(d, 2)
	if !d.BackHandling() {
		t.Fatal("Back not handled with a popup and a dialog open")
	}
	d.Key(input.KeyEscape, 0)
	frames(d, 2)
	if dialogOpen || !popupOpen {
		t.Fatalf("first Back: dialog %v popup %v", dialogOpen, popupOpen)
	}
	d.Key(input.KeyEscape, 0)
	frames(d, 2)
	if popupOpen || d.BackHandling() {
		t.Fatalf("second Back: popup %v handling %v", popupOpen, d.BackHandling())
	}
}

// The attached overlay page draws (on a transparent background) and its
// popup takes Back.
func TestWebOverlayPage(t *testing.T) {
	t.Cleanup(func() { webPopup = false })
	th := theme.AntDesign()
	th.BgLayout = theme.Color{}
	d := ui.NewDriver(webOverlay, ui.DriverOptions{Width: 390, Height: 844, Theme: th, Accessibility: true})
	img := d.Settle()
	if a := img.RGBAAt(195, 100).A; a != 0 {
		t.Fatalf("overlay not transparent above the card: alpha %d", a)
	}
	if err := d.A11yPerform(a11y.RoleButton, "打开弹出层", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !webPopup || !d.BackHandling() {
		t.Fatalf("popup %v handling %v", webPopup, d.BackHandling())
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()
	if webPopup {
		t.Fatal("Back did not close the popup")
	}
}
