package mobile

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
)

// Every mobile overlay asks for the system Back while it is open
// (ui.Context.WantBack through the Escape routing) and Back, delivered as
// Escape, closes it.
func TestBackClosesMobileOverlays(t *testing.T) {
	cases := map[string]func(c *ui.Context, open *bool){
		"Popup": func(c *ui.Context, open *bool) {
			Popup(c, open).Body(func() { ui.Text(c, "body") })
		},
		"ActionSheet": func(c *ui.Context, open *bool) {
			ActionSheet(c, open, []Action{{Text: "Copy"}}).Cancel("Cancel").Show()
		},
		"Dialog": func(c *ui.Context, open *bool) {
			Dialog(c, open, "Delete?").Content("Gone").Confirm("Yes", "No").Confirmed()
		},
		"Picker": func(c *ui.Context, open *bool) {
			val := []string{"x"}
			Picker(c, open, "Pick").Columns([][]ui.Option{ui.Opts("x", "y")}, &val).Show()
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			open := true
			d := phone(func(c *ui.Context) { build(c, &open) }, false)
			d.Settle()
			d.Frame()
			if !d.BackHandling() {
				t.Fatal("open: BackHandling off")
			}
			d.Key(input.KeyEscape, 0)
			d.Settle()
			if open || d.BackHandling() {
				t.Fatalf("after Back: open=%v BackHandling=%v", open, d.BackHandling())
			}
		})
	}
}
