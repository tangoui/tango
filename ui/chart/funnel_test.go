package chart

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

func TestFunnelTooltipUsesPreviousStage(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		start, previous, count int
		conversion             string
	}{
		{name: "positive-over-zero", start: 10, count: 2},
		{name: "zero-over-zero", start: 10},
		{name: "leading-zero", previous: 8, count: 2, conversion: "25.0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []Row{{"stage": "Start", "count": tc.start}, {"stage": "Previous", "count": tc.previous}, {"stage": "Target", "count": tc.count}}
			var v View
			d := ui.NewDriver(func(c *ui.Context) {
				v = Funnel(c, rows).X("stage").Y("count").Legend(LegendNone).Show()
			}, ui.DriverOptions{Width: 400, Height: 400})
			d.Settle()
			r := v.Rect()
			d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H*5/6))
			d.Frame()
			tip, ok := v.Tooltip()
			if !ok || tip.Title != "Target" {
				t.Fatalf("fixture did not hover Target: %+v", tip)
			}
			if tc.conversion == "" {
				if len(tip.Items) != 1 {
					t.Errorf("tooltip items = %v, want only the count", tip.Items)
				}
			} else if len(tip.Items) != 2 || tip.Items[1].Value != tc.conversion {
				t.Errorf("tooltip items = %v, want the count and %s", tip.Items, tc.conversion)
			}
		})
	}
}
