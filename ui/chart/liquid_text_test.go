package chart

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// liquidTextColors returns the colors of the text drawn by a still liquid
// chart filled to 78% (the label sits on the fill).
func liquidTextColors(t *testing.T, th *theme.Theme) []theme.Color {
	t.Helper()
	d := ui.NewDriver(func(c *ui.Context) { Liquid(c, 0.78).Title("Capacity").Still().Show() },
		ui.DriverOptions{Width: 300, Height: 300, Theme: th})
	d.Settle()
	var cols []theme.Color
	for _, op := range d.Runner().List().Ops {
		if tx, ok := op.(paint.Text); ok {
			cols = append(cols, theme.Color(tx.Color))
		}
	}
	if len(cols) == 0 {
		t.Fatal("liquid chart drew no text")
	}
	return cols
}

// The label on a high fill follows the fill's lightness: shadcn/ui v4's
// neutral chart-1 is a light gray, where white text is unreadable.
func TestLiquidLabelOnLightFill(t *testing.T) {
	white := theme.Hex("#ffffff")
	for _, c := range liquidTextColors(t, theme.Shadcn()) {
		if c == white {
			t.Errorf("white label on the light gray shadcn fill")
		}
	}
	for _, c := range liquidTextColors(t, theme.AntDesign()) {
		if c != white {
			t.Errorf("label on the Ant Design blue fill is %v, want white", c)
		}
	}
}
