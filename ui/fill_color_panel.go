package ui

// ColorPickerPanel: the ColorPicker panel as an embeddable widget without
// trigger or popup (Element Plus el-color-picker-panel). The saturation /
// value square, hue and alpha bars, preset swatches and Ant's inputs are
// the ColorPicker's own builders (entry_color.go), driven through a
// ColorPickerW that is never shown as a trigger.

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// ColorPickerPanelW is an inline color panel bound to a color.
type ColorPickerPanelW struct {
	c      *Context
	cp     *ColorPickerW
	box    *Box
	border bool
	off    bool
	z      *lazy
}

// ColorPickerPanel adds an inline color panel bound to *val; changes apply
// immediately in every design system (there is no confirm step).
//
//	ui.ColorPickerPanel(c, &col).Presets(theme.Hex("#409eff"), theme.Hex("#67c23a"))
func ColorPickerPanel(c *Context, val *theme.Color) *ColorPickerPanelW {
	w := &ColorPickerPanelW{c: c, border: true}
	w.box = c.box("fill-colorpanel", layout.Column, nil)
	w.box.n.ln.NoStretch = true
	cp := &ColorPickerW{c: c, val: val, status: c.itemStatus, loc: c.Locale(), size: c.defaultSize(), n: w.box.n}
	cp.cpTheme(c)
	cp.presetName = cp.loc.ColorPicker.Presets
	cp.st = State(c, hashID(w.box.n.id, "fill-cp"), func() cpState { return cpState{format: "hex"} })
	w.cp = cp
	w.z = c.lazy(w.box.n, w.build)
	return w
}

// Presets adds preset swatches (Element predefine, Ant presets).
func (w *ColorPickerPanelW) Presets(cols ...theme.Color) *ColorPickerPanelW {
	w.cp.presets = cols
	return w
}

// PresetGroups shows several titled preset groups (see
// ColorPickerW.PresetGroups).
func (w *ColorPickerPanelW) PresetGroups(groups ...ColorPresetGroup) *ColorPickerPanelW {
	w.cp.groups = groups
	return w
}

// PresetLabel names the preset group (Ant).
func (w *ColorPickerPanelW) PresetLabel(s string) *ColorPickerPanelW { w.cp.presetName = s; return w }

// NoAlpha hides the alpha bar and input.
func (w *ColorPickerPanelW) NoAlpha() *ColorPickerPanelW { w.cp.noAlpha = true; return w }

// Format sets the value text format: "hex" (default), "rgb", "hsb" or
// "hsv".
func (w *ColorPickerPanelW) Format(f string) *ColorPickerPanelW { w.cp.Format(f); return w }

// AllowClear adds the "no color" control (Ant).
func (w *ColorPickerPanelW) AllowClear() *ColorPickerPanelW { w.cp.allowClear = true; return w }

// Border toggles the panel border (default on).
func (w *ColorPickerPanelW) Border(on bool) *ColorPickerPanelW { w.border = on; return w }

// Disabled turns the panel off.
func (w *ColorPickerPanelW) Disabled(on bool) *ColorPickerPanelW {
	if on {
		w.off = true
	}
	return w
}

// Changed reports a new color this frame.
func (w *ColorPickerPanelW) Changed() bool { w.z.run(); return w.cp.changed }

// ID returns the panel's ID.
func (w *ColorPickerPanelW) ID() ID { return w.box.n.id }

func (w *ColorPickerPanelW) build() {
	c, th, cp := w.c, w.cp.th, w.cp
	cp.sync(false)
	h0 := cp.st.hsva
	el := th.Flavor == theme.Element
	n := w.box.n
	if w.border {
		rad := th.RadiusLG
		if el {
			rad = th.Radius
		}
		w.box.Bg(th.BgElevated).Radius(rad).Border(1, th.BorderSecondary)
	}
	tk := cp.tk
	body := func() {
		if el {
			Column(c, w.element, layout.Pad(tk.PanelPadding), layout.Gap(8), layout.W(tk.PanelWidth))
		} else {
			Column(c, cp.panelAnt, layout.Pad(tk.PanelPadding), layout.Gap(12), layout.W(tk.PanelWidth))
		}
	}
	c.body(n, func() { Disabled(c, w.off, body) })
	if w.off {
		w.box.Opacity(0.55)
	}
	// Element's builders keep a draft; the panel commits it at once.
	if el && cp.st.hsva != h0 {
		if col := theme.FromHSVA(cp.st.hsva); col != *cp.val {
			*cp.val = col
			cp.changed = true
		}
		cp.st.synced = *cp.val
	}
}

// element lays out Element's panel: square + vertical hue, alpha bar,
// presets and the value input (no confirm buttons).
func (w *ColorPickerPanelW) element() {
	c, cp := w.c, w.cp
	st := cp.st
	cp.elPalette()
	if !cp.noAlpha {
		cp.bar("alpha", false, 0, cp.tk.BarHeight)
	}
	cp.presetsElement()
	if st.ids[0] == 0 || c.focus != st.ids[0] {
		st.bufs[0] = cp.formatHSVA(st.hsva)
	}
	Row(c, func() {
		in := Input(c, &st.bufs[0]).Size(Small).Layout(layout.Grow(1))
		st.ids[0] = in.ID()
		fillSem(c, in.ed, a11y.RoleTextbox, c.Locale().ColorPicker.Value)
		if in.Submitted() || in.Changed() {
			if v, err := theme.ParseColor(st.bufs[0]); err == nil {
				cp.applyColor(v)
			}
		}
	}, layout.Middle())
}
