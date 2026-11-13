package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

// TestInputNumberControlsDefaultByFlavor: el-input-number puts − and + at
// the sides by default, so Element's InputNumber does (150px wide, like
// the el-input-number default width); ControlsPosition("right") and
// ConfigProvider Defaults still choose the stacked buttons. Ant and shadcn
// keep the stacked buttons.
func TestInputNumberControlsDefaultByFlavor(t *testing.T) {
	for _, c := range []struct {
		th   *theme.Theme
		want string
	}{
		{theme.ElementPlus(), "sides"},
		{theme.ElementPlus(theme.Dark()), "sides"},
		{theme.AntDesign(), ""},
		{theme.Shadcn(), ""},
	} {
		v := 1.0
		var def, right, cfg *InputNumberW
		d := p2dDriver(c.th, func(cx *Context) {
			def = InputNumber(cx, &v).A11yLabel("a")
			right = InputNumber(cx, &v).ControlsPosition("right").A11yLabel("b")
			ConfigProvider(cx).Defaults("InputNumber", Props{"controlsPosition": "right"}).Body(func() {
				cfg = InputNumber(cx, &v).A11yLabel("c")
			})
		})
		if def.ctrlPos != c.want {
			t.Errorf("%v: default controls %q, want %q", c.th.Flavor, def.ctrlPos, c.want)
		}
		if right.ctrlPos != "right" || cfg.ctrlPos != "right" {
			t.Errorf("%v: explicit %q, config %q, want right", c.th.Flavor, right.ctrlPos, cfg.ctrlPos)
		}
		if c.want == "sides" {
			if w := def.box.abs.W; w != 150 {
				t.Errorf("%v: sides width %v, want 150", c.th.Flavor, w)
			}
		}
		_ = d
	}
}

// TestInputNumberElementRightHandlesShown: Element's stacked step buttons
// (ControlsPosition "right") are always shown on --el-fill-color-light, as
// el-input-number's; Ant's appear only on hover or focus.
func TestInputNumberElementRightHandlesShown(t *testing.T) {
	for _, c := range []struct {
		th    *theme.Theme
		shown bool
	}{{theme.ElementPlus(), true}, {theme.AntDesign(), false}} {
		v := 1.0
		d := p2dDriver(c.th, func(cx *Context) { InputNumber(cx, &v).ControlsPosition("right").A11yLabel("n") })
		d.Move(geom.Pt(800, 600)) // away from the control
		d.Settle()
		tk := c.th.Components.InputNumber
		bg := tk.HandleBg.A > 0 && t2aCount(d, tk.HandleBg) > 0
		arrows := t2aCount(d, tk.HandleColor) > 0
		if c.shown && (!bg || !arrows) {
			t.Errorf("%v: resting handles background %v, arrows %v", c.th.Flavor, bg, arrows)
		}
		if !c.shown && arrows {
			t.Errorf("%v: handles shown without hover", c.th.Flavor)
		}
	}
}
