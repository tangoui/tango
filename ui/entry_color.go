package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// cpClamp01 clamps a channel to [0,1].
func cpClamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// cpHex formats a color as #RRGGBB, or #RRGGBBAA when it is translucent.
func cpHex(col theme.Color) string { return strings.ToUpper(theme.HexString(col)) }

// ---------------------------------------------------------------------------
// Drawing helpers

// cpSwatch draws a color sample (checkerboard below translucent colors).
func cpSwatch(l *paint.List, r geom.Rect, rad geom.Radii, col theme.Color) {
	if col.A < 255 {
		l.Checker(r, rad, max(3, min(r.W, r.H)/4), theme.Color{}, theme.Color{})
	}
	l.Fill(r, rad, col)
}

// cpEmpty draws the "no color" swatch: white with a red diagonal.
func cpEmpty(l *paint.List, r geom.Rect, rad geom.Radii, red theme.Color) {
	l.Fill(r, rad, theme.Hex("#ffffff"))
	l.Clip(r, rad)
	l.StrokePath(paint.NewPath().MoveTo(r.X, r.Bottom()).LineTo(r.Right(), r.Y), paint.StrokeStyle{Width: 2}, red)
	l.Pop()
}

// cpHueBar draws the hue spectrum along r (vertical when vert; a
// horizontal bar starts at the right when rtl).
func cpHueBar(l *paint.List, r geom.Rect, rad geom.Radii, vert, rtl bool) {
	stops := make([]paint.Stop, 7)
	for i := range stops {
		stops[i] = paint.Stop{Offset: float32(i) / 6, Color: theme.FromHSV(theme.HSV{H: float64(i%6) * 60, S: 1, V: 1})}
	}
	start, end := geom.Pt(r.X, r.Y), geom.Pt(r.Right(), r.Y)
	if vert {
		end = geom.Pt(r.X, r.Bottom())
	} else if rtl {
		start, end = end, start
	}
	l.LinearStops(r, rad, start, end, stops...)
}

// ---------------------------------------------------------------------------
// ColorPicker

type cpState struct {
	pop     entPopup
	hsva    theme.HSVA
	synced  theme.Color
	has     bool
	wasOpen bool
	format  string
	fmtInit bool      // the "format" ConfigProvider default was applied
	bufs    [5]string // hex, r, g, b, alpha% (Ant); [0] is the value text under Element
	ids     [5]ID
	selPop  ID
	panel   ID // the panel content (dialog semantics, Controls)
	// groupFlip marks preset groups the user toggled away from their
	// ColorPresetGroup.Closed default.
	groupFlip map[int]bool
}

// ColorPresetGroup is a titled group of preset swatches (PresetGroups).
type ColorPresetGroup struct {
	Label  string
	Colors []theme.Color
	Closed bool // Ant / shadcn: the group starts collapsed
}

// cpFormatOK reports whether f is a Format value.
func cpFormatOK(f string) bool { return f == "hex" || f == "rgb" || f == "hsb" || f == "hsv" }

// ColorPickerW is a color input with a picker panel.
type ColorPickerW struct {
	c          *Context
	n          *node
	val        *theme.Color
	st         *cpState
	ia         Interaction
	presets    []theme.Color
	groups     []ColorPresetGroup
	presetName string
	loc        *Locale // captured at build; measure/paint run outside ConfigProvider
	showText   bool
	noAlpha    bool
	allowClear bool
	disabled   bool
	status     string
	size       ControlSize
	changed    bool
	cleared    bool
	z          *lazy
	th         *theme.Theme // ColorPicker Alias applied
	tk         *theme.ColorPickerToken
}

// cpTheme captures the ColorPicker theme and tokens at build.
func (w *ColorPickerW) cpTheme(c *Context) {
	w.th = c.ThemeFor(theme.CompColorPicker)
	w.tk = &w.th.Components.ColorPicker
}

// ColorPicker adds a color picker bound to *val. The zero Color means "no
// color" (cleared).
func ColorPicker(c *Context, val *theme.Color) *ColorPickerW {
	w := &ColorPickerW{c: c, val: val, status: c.itemStatus, loc: c.Locale(), size: c.defaultSize()}
	w.cpTheme(c)
	w.presetName = w.loc.ColorPicker.Presets
	w.n = c.add("colorpicker", "")
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.st = State[cpState](c, w.n.id, func() cpState { return cpState{format: "hex"} })
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(w)
	w.configDefaults()
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (default: the Locale's A11y.Color;
// inside a FormItem the item label).
func (w *ColorPickerW) A11yLabel(s string) *ColorPickerW { w.n.setName(s); return w }

// semantics: the color (hex / rgb text, or "none") as the value; while
// open the panel dialog it controls.
func (w *ColorPickerW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = w.loc.A11y.Color
	}
	s.Value = w.text()
	if s.Value == "" {
		s.Value = w.loc.ColorPicker.None
	}
	if w.st.pop.open {
		semExpanded(s)
		s.Controls = a11y.NodeID(w.st.panel)
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
}

// Presets adds preset swatches (Ant presets, Element predefine).
func (w *ColorPickerW) Presets(cols ...theme.Color) *ColorPickerW { w.presets = cols; return w }

// PresetGroups shows several titled preset groups instead of Presets. In
// Ant / shadcn the titles are buttons that collapse / expand their group
// (Closed starts collapsed); Element shows each title above its swatches.
func (w *ColorPickerW) PresetGroups(groups ...ColorPresetGroup) *ColorPickerW {
	w.groups = groups
	return w
}

// PresetLabel names the preset group (Ant; default "推荐").
func (w *ColorPickerW) PresetLabel(s string) *ColorPickerW { w.presetName = s; return w }

// ShowText shows the color value next to the swatch.
func (w *ColorPickerW) ShowText() *ColorPickerW { w.showText = true; return w }

// NoAlpha hides the alpha slider and input.
func (w *ColorPickerW) NoAlpha() *ColorPickerW { w.noAlpha = true; return w }

// AllowClear adds a clear control (Element always has one).
func (w *ColorPickerW) AllowClear() *ColorPickerW { w.allowClear = true; return w }

// Format sets the value text and input format: "hex", "rgb", or "hsb" /
// "hsv" (the same color space, written "hsb(220, 75%, 80%)" /
// "hsv(...)"; Ant's panel then edits hue, saturation and brightness).
// Typed text is accepted in any of these forms.
func (w *ColorPickerW) Format(f string) *ColorPickerW {
	if cpFormatOK(f) {
		w.st.format = f
	}
	return w
}
func (w *ColorPickerW) Status(s string) *ColorPickerW    { w.status = s; return w }
func (w *ColorPickerW) Size(s ControlSize) *ColorPickerW { w.size = s; return w }
func (w *ColorPickerW) Disabled(on bool) *ColorPickerW {
	if on {
		w.disabled, w.n.disabled, w.n.focus, w.n.cursor = true, true, false, input.CursorNotAllowed
	}
	return w
}

// Open forces the panel open (screenshots, tests).
func (w *ColorPickerW) Open() *ColorPickerW { w.st.pop.open = true; return w }

// IsOpen reports whether the panel is shown.
func (w *ColorPickerW) IsOpen() bool { return w.st.pop.open }

// Changed reports a new color this frame (Element: on confirm or clear).
func (w *ColorPickerW) Changed() bool { w.z.run(); return w.changed }

// Cleared reports that the value was cleared this frame.
func (w *ColorPickerW) Cleared() bool { w.z.run(); return w.cleared }

func (w *ColorPickerW) el() bool { return w.c.Theme.Flavor == theme.Element }

func (w *ColorPickerW) text() string {
	if *w.val == (theme.Color{}) {
		return ""
	}
	h := theme.ToHSVA(*w.val)
	if w.st.has && w.st.synced == *w.val {
		h = w.st.hsva // keeps the hue of greys
	}
	return w.formatHSVA(h)
}

// formatHSVA writes h in the current format.
func (w *ColorPickerW) formatHSVA(h theme.HSVA) string {
	switch w.st.format {
	case "rgb":
		return theme.RGBString(theme.FromHSVA(h))
	case "hsb", "hsv":
		return theme.HSBAString(w.st.format, h)
	}
	return cpHex(theme.FromHSVA(h))
}

func (w *ColorPickerW) measure(_, _ float32) geom.Size {
	c := w.c
	h := c.controlHeight(w.size)
	if !w.showText {
		return geom.Size{W: h, H: h}
	}
	s := w.text()
	if s == "" {
		s = w.loc.ColorPicker.None
	}
	return geom.Size{W: h + entTextW(c, s, c.controlFont(w.size), text.Regular) + 6, H: h}
}

func (w *ColorPickerW) sync(force bool) {
	st := w.st
	if !force && st.has && *w.val == st.synced {
		return
	}
	h := theme.ToHSVA(*w.val)
	if *w.val == (theme.Color{}) {
		h = theme.HSVA{H: st.hsva.H, S: 1, V: 1, A: 1}
		if !st.has {
			h.H = 0
		}
	} else if st.has && (h.S == 0 || h.V == 0) {
		h.H = st.hsva.H // keep hue on greys
		if h.V == 0 {
			h.S = st.hsva.S
		}
	}
	st.hsva, st.synced, st.has = h, *w.val, true
}

// apply sets the working color; Ant commits it immediately, Element keeps
// a draft until confirmed.
func (w *ColorPickerW) apply(h theme.HSVA) {
	h.H = math.Max(0, math.Min(359.999, h.H))
	h.S, h.V, h.A = cpClamp01(h.S), cpClamp01(h.V), cpClamp01(h.A)
	if w.noAlpha {
		h.A = 1
	}
	w.st.hsva = h
	if !w.el() {
		col := theme.FromHSVA(h)
		if col != *w.val {
			*w.val = col
			w.changed = true
		}
		w.st.synced = *w.val
	}
	w.c.invalid = true
}

func (w *ColorPickerW) applyColor(col theme.Color) {
	h := theme.ToHSVA(col)
	if h.S == 0 || h.V == 0 {
		h.H = w.st.hsva.H
	}
	w.apply(h)
}

func (w *ColorPickerW) finish() {
	c := w.c
	st := w.st
	if w.disabled || w.n.disabled {
		st.pop.open = false
		w.sync(false)
		return
	}
	if w.ia.Clicked {
		st.pop.open = !st.pop.open
		c.invalid = true
	}
	st.pop.dismiss(c, w.n.id, st.selPop)
	if st.pop.open && !st.wasOpen {
		w.sync(true) // Element: start the draft from the value
	} else if !w.el() || !st.pop.open {
		w.sync(false)
	}
	st.wasOpen = st.pop.open
	if !st.pop.open {
		return
	}
	entPanel(c, w.n.id, &st.pop, false, w.panel)
}

func (w *ColorPickerW) panel() {
	c, tk := w.c, w.tk
	var b *Box
	if w.el() {
		b = Column(c, w.panelElement, layout.Pad(tk.PanelPadding), layout.Gap(8), layout.W(tk.PanelWidth))
	} else {
		b = Column(c, w.panelAnt, layout.Pad(tk.PanelPadding), layout.Gap(12), layout.W(tk.PanelWidth))
	}
	// A non-modal dialog: the trigger keeps focus, Tab enters the panel and
	// Escape closes it (focus returns to the trigger).
	b.n.setRole(a11y.RoleDialog).setName(w.loc.A11y.Color)
	w.st.panel = b.n.id
}

func (w *ColorPickerW) panelAnt() {
	c, th, tk := w.c, w.th, w.tk
	st := w.st
	if w.allowClear {
		Row(c, func() {
			Spacer(c)
			n := c.add("cp-clear", w.loc.ColorPicker.ClearColor)
			n.ln.Width, n.ln.Height = 24, 24
			n.setRole(a11y.RoleButton)
			ia := c.interact(n, true, input.CursorPointer)
			n.over = func(l *paint.List, r geom.Rect) {
				if ia.FocusVisible {
					entFocusRing(c, l, r, th.RadiusSM)
				}
			}
			n.paint = func(l *paint.List, r geom.Rect) {
				cpEmpty(l, r, geom.Rad(th.RadiusSM), tk.ClearColor)
				l.Stroke(r, geom.Rad(th.RadiusSM), 1, tk.BorderColor)
			}
			if ia.Clicked {
				w.clear(false)
			}
		})
	}
	w.svPanel(tk.PanelWidth-2*tk.PanelPadding, tk.PaletteHeight)
	Row(c, func() {
		Column(c, func() {
			w.bar("hue", false, 0, tk.BarHeight)
			if !w.noAlpha {
				w.bar("alpha", false, 0, tk.BarHeight)
			}
		}, layout.Grow(1), layout.Gap(6), layout.JustifyContent(layout.JustifyCenter))
		n := c.add("cp-current", "")
		n.ln.Width, n.ln.Height = 28, 28
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(th.RadiusSM)
			cpSwatch(l, r, rad, theme.FromHSVA(st.hsva))
			l.Stroke(r, rad, 1, tk.BorderColor)
		}
	}, layout.Gap(12), layout.Middle())
	w.inputsAnt()
	if len(w.groups) > 0 {
		w.presetGroupsAnt()
	} else if len(w.presets) > 0 {
		Column(c, func() {
			Row(c, func() {
				Icon(c, icon.Down).Size(12).Color(th.TextTertiary)
				Text(c, w.presetName).Size(th.FontSizeSM).Color(th.TextTertiary)
			}, layout.Gap(4), layout.Middle())
			w.swatches(w.presets, w.presetName, tk.SwatchSize, tk.SwatchGap)
		}, layout.Gap(8)).Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X, Y: r.Y - 6, W: r.W, H: 1}, geom.Radii{}, th.Split)
		}).Style(layout.Padding(geom.Insets{Top: 6}))
	}
}

// presetGroupsAnt adds the PresetGroups under a divider: each title is a
// disclosure button over its swatches.
func (w *ColorPickerW) presetGroupsAnt() {
	c, th, tk := w.c, w.th, w.tk
	st := w.st
	Column(c, func() {
		for gi, g := range w.groups {
			gi, g := gi, g
			Key(c, gi, func() {
				open := !g.Closed != st.groupFlip[gi]
				hd := Row(c, func() {
					ic := icon.Down
					if !open {
						ic = icon.Right // mirrored in RTL
					}
					Icon(c, ic).Size(12).Color(th.TextTertiary)
					Text(c, g.Label).Size(th.FontSizeSM).Color(th.TextTertiary)
				}, layout.Gap(4), layout.Middle())
				hd.n.ln.NoStretch = true
				ia := hd.Focusable(input.CursorPointer)
				hd.n.setRole(a11y.RoleButton).setName(g.Label).setState(a11y.Expandable).stateIf(open, a11y.Expanded)
				if ia.FocusVisible {
					hd.PaintOver(func(l *paint.List, r geom.Rect) { entFocusRing(c, l, r, th.RadiusSM) })
				}
				if ia.Clicked {
					if st.groupFlip == nil {
						st.groupFlip = map[int]bool{}
					}
					st.groupFlip[gi] = !st.groupFlip[gi]
					open = !open
					c.invalid = true
				}
				if open && len(g.Colors) > 0 {
					w.swatches(g.Colors, g.Label, tk.SwatchSize, tk.SwatchGap)
				}
			})
		}
	}, layout.Gap(8)).Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Y - 6, W: r.W, H: 1}, geom.Radii{}, th.Split)
	}).Style(layout.Padding(geom.Insets{Top: 6}))
}

// presetsElement adds Element's presets: the single Presets grid, or each
// PresetGroups title above its grid.
func (w *ColorPickerW) presetsElement() {
	c, th, tk := w.c, w.th, w.tk
	if len(w.groups) == 0 {
		if len(w.presets) > 0 {
			w.swatches(w.presets, w.presetName, tk.SwatchSize, tk.SwatchGap)
		}
		return
	}
	for gi, g := range w.groups {
		gi, g := gi, g
		Key(c, gi, func() {
			Column(c, func() {
				if g.Label != "" {
					Text(c, g.Label).Size(th.FontSizeSM).Color(th.TextSecondary)
				}
				w.swatches(g.Colors, g.Label, tk.SwatchSize, tk.SwatchGap)
			}, layout.Gap(6))
		})
	}
}

func (w *ColorPickerW) inputsAnt() {
	c := w.c
	st := w.st
	col := theme.FromHSVA(st.hsva)
	refresh := func(i int, s string) {
		if st.ids[i] == 0 || c.focus != st.ids[i] {
			st.bufs[i] = s
		}
	}
	hsb := st.format == "hsb" || st.format == "hsv"
	refresh(0, strings.TrimPrefix(cpHex(theme.Color{R: col.R, G: col.G, B: col.B, A: 255}), "#"))
	if hsb {
		refresh(1, strconv.Itoa(int(math.Round(st.hsva.H))%360))
		refresh(2, strconv.Itoa(int(math.Round(st.hsva.S*100))))
		refresh(3, strconv.Itoa(int(math.Round(st.hsva.V*100))))
	} else {
		refresh(1, strconv.Itoa(int(col.R)))
		refresh(2, strconv.Itoa(int(col.G)))
		refresh(3, strconv.Itoa(int(col.B)))
	}
	refresh(4, strconv.Itoa(int(math.Round(st.hsva.A*100))))
	Row(c, func() {
		// Format names are color notation, not interface text.
		third := Option{Label: "HSB", Value: "hsb"}
		if st.format == "hsv" {
			third = Option{Label: "HSV", Value: "hsv"}
		}
		sel := Select(c, &st.format, []Option{{Label: "HEX", Value: "hex"}, {Label: "RGB", Value: "rgb"}, third}).Size(Small).Width(76).A11yLabel(w.loc.ColorPicker.Format)
		st.selPop = sel.st.panel
		if hsb {
			w.inputsHSB()
		} else if st.format == "hex" {
			in := Input(c, &st.bufs[0]).Size(Small).PrefixText("#").MaxLength(6).Layout(layout.Grow(1), layout.MinW(0)).A11yLabel("HEX")
			st.ids[0] = in.ID()
			if in.Changed() {
				if v, err := theme.ParseHex(st.bufs[0]); err == nil && len(st.bufs[0]) == 6 {
					v.A = col.A
					w.applyColor(v)
				}
			}
		} else {
			for i := 1; i <= 3; i++ {
				// Channel names are color notation, not interface text.
				in := Input(c, &st.bufs[i]).Size(Small).MaxLength(3).Layout(layout.Grow(1), layout.MinW(0)).A11yLabel("RGB"[i-1 : i])
				st.ids[i] = in.ID()
				if in.Changed() {
					if v, err := strconv.Atoi(st.bufs[i]); err == nil && v >= 0 && v <= 255 {
						nc := col
						switch i {
						case 1:
							nc.R = uint8(v)
						case 2:
							nc.G = uint8(v)
						default:
							nc.B = uint8(v)
						}
						w.applyColor(nc)
					}
				}
			}
		}
		if !w.noAlpha {
			in := Input(c, &st.bufs[4]).Size(Small).SuffixText("%").MaxLength(3).Width(52).A11yLabel(w.loc.ColorPicker.Alpha)
			st.ids[4] = in.ID()
			if in.Changed() {
				if v, err := strconv.Atoi(st.bufs[4]); err == nil && v >= 0 && v <= 100 {
					h := st.hsva
					h.A = float64(v) / 100
					w.apply(h)
				}
			}
		}
	}, layout.Gap(4), layout.Middle())
}

// inputsHSB adds Ant's hue (degrees), saturation and brightness (percent)
// inputs. No "%" suffix: the row has no room for it next to the format
// select and the alpha input.
func (w *ColorPickerW) inputsHSB() {
	c := w.c
	st := w.st
	names := "HSB" // channel names are color notation, not interface text
	if st.format == "hsv" {
		names = "HSV"
	}
	for i := 1; i <= 3; i++ {
		in := Input(c, &st.bufs[i]).Size(Small).MaxLength(3).Layout(layout.Grow(1), layout.MinW(0)).A11yLabel(names[i-1 : i])
		st.ids[i] = in.ID()
		if !in.Changed() {
			continue
		}
		v, err := strconv.Atoi(st.bufs[i])
		if err != nil || v < 0 || (i == 1 && v > 360) || (i > 1 && v > 100) {
			continue
		}
		h := st.hsva
		switch i {
		case 1:
			h.H = float64(v % 360)
		case 2:
			h.S = float64(v) / 100
		default:
			h.V = float64(v) / 100
		}
		w.apply(h)
	}
}

func (w *ColorPickerW) panelElement() {
	c, tk := w.c, w.tk
	st := w.st
	w.elPalette()
	if !w.noAlpha {
		w.bar("alpha", false, 0, tk.BarHeight)
	}
	w.presetsElement()
	if st.ids[0] == 0 || c.focus != st.ids[0] {
		st.bufs[0] = w.formatHSVA(st.hsva)
	}
	Row(c, func() {
		in := Input(c, &st.bufs[0]).Size(Small).Width(160).A11yLabel(w.loc.A11y.Color)
		st.ids[0] = in.ID()
		if in.Submitted() || in.Changed() {
			if v, err := theme.ParseColor(st.bufs[0]); err == nil {
				w.applyColor(v)
			}
		}
		Spacer(c)
		if Button(c, w.loc.ColorPicker.Clear).Link().Small().Clicked() {
			w.clear(true)
		}
		if Button(c, w.loc.OK).Small().Clicked() {
			col := theme.FromHSVA(st.hsva)
			if col != *w.val {
				*w.val = col
				w.changed = true
			}
			st.synced = *w.val
			st.pop.open = false
			c.invalid = true
		}
	}, layout.Gap(8), layout.Middle())
}

func (w *ColorPickerW) clear(close bool) {
	if *w.val != (theme.Color{}) {
		*w.val = theme.Color{}
		w.changed = true
	}
	w.cleared = true
	w.st.synced = *w.val
	if close {
		w.st.pop.open = false
	}
	w.c.invalid = true
}

// elPalette adds Element's palette row: the square and the vertical hue
// bar beside it, 8px apart.
func (w *ColorPickerW) elPalette() {
	tk := w.tk
	Row(w.c, func() {
		w.svPanel(tk.PanelWidth-2*tk.PanelPadding-tk.BarHeight-8, tk.PaletteHeight)
		w.bar("hue", true, tk.BarHeight, tk.PaletteHeight)
	}, layout.Gap(8))
}

// svPanel adds the saturation/value square.
func (w *ColorPickerW) svPanel(width, height float32) {
	c, tk := w.c, w.tk
	st := w.st
	n := c.add("cp-sv", w.loc.ColorPicker.Saturation)
	n.ln.Width, n.ln.Height = width, height
	n.ln.NoStretch = true
	n.ownKeys = true
	ia := c.interact(n, true, input.CursorDefault)
	// A slider over saturation (Left / Right); Up / Down change the
	// brightness, reported in the value text.
	n.setRole(a11y.RoleSlider).setState(a11y.Horizontal)
	if c.a11yOn {
		n.fill(semFunc(func(s *a11y.Node) {
			h := st.hsva
			rangeSem(s, math.Round(h.S*100), 0, 100, 1)
			s.Actions |= a11y.SetValue
			s.Value = strconv.Itoa(int(math.Round(h.S*100))) + "%, " + strconv.Itoa(int(math.Round(h.V*100))) + "%"
		}))
	}
	if v, ok := c.takeSetNumber(n.id); ok {
		h := st.hsva
		h.S = v / 100
		w.apply(h)
	}
	if r, ok := c.rects[n.id]; ok && (ia.PressedNow || c.Dragging(n.id)) && r.W > 0 {
		h := st.hsva
		h.S = cpClamp01(float64((c.pointer.X - r.X) / r.W))
		h.V = 1 - cpClamp01(float64((c.pointer.Y-r.Y)/r.H))
		w.apply(h)
	}
	for _, k := range c.KeysFor(n.id) {
		if !k.Down {
			continue
		}
		h := st.hsva
		switch k.Key {
		case input.KeyLeft:
			h.S -= 0.01
		case input.KeyRight:
			h.S += 0.01
		case input.KeyUp:
			h.V += 0.01
		case input.KeyDown:
			h.V -= 0.01
		default:
			continue
		}
		w.apply(h)
	}
	el := w.el()
	n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(tk.PaletteRadius)
		l.Clip(r, rad)
		l.Fill(r, geom.Radii{}, theme.FromHSV(theme.HSV{H: st.hsva.H, S: 1, V: 1}))
		l.LinearStops(r, geom.Radii{}, geom.Pt(r.X, r.Y), geom.Pt(r.Right(), r.Y), paint.Stop{Offset: 0, Color: theme.RGBA(255, 255, 255, 1)}, paint.Stop{Offset: 1, Color: theme.RGBA(255, 255, 255, 0)})
		l.LinearStops(r, geom.Radii{}, geom.Pt(r.X, r.Y), geom.Pt(r.X, r.Bottom()), paint.Stop{Offset: 0, Color: theme.RGBA(0, 0, 0, 0)}, paint.Stop{Offset: 1, Color: theme.RGBA(0, 0, 0, 1)})
		l.Pop()
		x := r.X + float32(st.hsva.S)*r.W
		y := r.Y + float32(1-st.hsva.V)*r.H
		cur := theme.FromHSVA(st.hsva)
		cur.A = 255
		if el {
			l.StrokePath(paint.NewPath().Circle(x, y, 4.5), paint.StrokeStyle{Width: 1}, theme.RGBA(0, 0, 0, .3))
			l.StrokePath(paint.NewPath().Circle(x, y, 3.5), paint.StrokeStyle{Width: 1.2}, theme.Hex("#ffffff"))
		} else {
			cpHandle(l, tk, x, y, cur)
		}
		if ia.FocusVisible {
			entFocusRing(c, l, r, rad.TL)
		}
	}
}

// cpHandle draws Ant's round slider / palette handle filled with col.
func cpHandle(l *paint.List, tk *theme.ColorPickerToken, x, y float32, col theme.Color) {
	r := tk.HandleSize / 2
	l.Path(paint.NewPath().Circle(x, y, r+1), tk.HandleShadow)
	l.Path(paint.NewPath().Circle(x, y, r), tk.HandleColor)
	l.Path(paint.NewPath().Circle(x, y, r-2), col)
}

// bar adds the hue or alpha slider. Vertical bars (Element hue) take
// width/height; horizontal ones stretch and take height.
func (w *ColorPickerW) bar(kind string, vert bool, width, height float32) {
	c, tk := w.c, w.tk
	st := w.st
	label := map[string]string{"hue": w.loc.ColorPicker.Hue, "alpha": w.loc.ColorPicker.Alpha}[kind]
	n := c.add("cp-bar", label)
	n.ln.Height = height
	if vert {
		n.ln.Width = width
	}
	n.ownKeys = true
	ia := c.interact(n, true, input.CursorPointer)
	el := w.el()
	n.setRole(a11y.RoleSlider)
	if vert {
		n.setState(a11y.Vertical)
	} else {
		n.setState(a11y.Horizontal)
	}
	get := func() float64 {
		if kind == "hue" {
			return st.hsva.H / 360
		}
		return st.hsva.A
	}
	set := func(t float64) {
		h := st.hsva
		t = cpClamp01(t)
		if kind == "hue" {
			h.H = t * 360
		} else {
			h.A = t
		}
		w.apply(h)
	}
	// Hue in degrees, alpha in percent.
	scale := 100.0
	if kind == "hue" {
		scale = 360
	}
	if v, ok := c.takeSetNumber(n.id); ok {
		set(v / scale)
	}
	if c.a11yOn {
		n.fill(semFunc(func(s *a11y.Node) {
			now := math.Round(get() * scale)
			rangeSem(s, now, 0, scale, 1)
			s.Actions |= a11y.SetValue
			if kind == "alpha" {
				s.Value = strconv.Itoa(int(now)) + "%"
			}
		}))
	}
	// Ant handles (16px) overhang the ends; keep their centres inside.
	inset := tk.HandleSize / 2
	if el {
		inset = 0
	}
	// A horizontal bar runs from its inline start (0 at the right in RTL,
	// like Slider); vertical bars and the saturation palette (a color
	// space, not reading order) stay physical.
	rtl := c.IsRTL() && !vert
	if r, ok := c.rects[n.id]; ok && (ia.PressedNow || c.Dragging(n.id)) {
		if vert && r.H > 0 {
			set(float64((c.pointer.Y - r.Y) / r.H))
		} else if r.W > 2*inset {
			px := c.pointer.X
			if rtl {
				px = 2*r.X + r.W - px
			}
			set(float64((px - r.X - inset) / (r.W - 2*inset)))
		}
	}
	for _, k := range c.KeysFor(n.id) {
		if !k.Down {
			continue
		}
		step := 0.01
		if kind == "hue" {
			step = 1.0 / 360
		}
		// The arrow toward the max side increases (RTL: Left).
		switch {
		case k.Key == input.KeyUp, k.Key == input.KeyLeft && !rtl, k.Key == input.KeyRight && rtl:
			set(get() - step)
		case k.Key == input.KeyDown, k.Key == input.KeyRight && !rtl, k.Key == input.KeyLeft && rtl:
			set(get() + step)
		}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		rail := r
		if !el {
			rail = geom.Rect{X: r.X, Y: r.Y + (r.H-tk.RailHeight)/2, W: r.W, H: tk.RailHeight}
		}
		rad := geom.Rad(min(rail.W, rail.H) / 2)
		if el {
			rad = geom.Radii{}
		}
		cur := theme.FromHSVA(st.hsva)
		opaque := cur
		opaque.A = 255
		if kind == "hue" {
			cpHueBar(l, rail, rad, vert, rtl)
		} else {
			l.Checker(rail, rad, 4, theme.Color{}, theme.Color{})
			tr := opaque
			tr.A = 0
			p0, p1 := geom.Pt(rail.X, rail.Y), geom.Pt(rail.Right(), rail.Y)
			if rtl {
				p0, p1 = p1, p0
			}
			l.LinearStops(rail, rad, p0, p1, paint.Stop{Offset: 0, Color: tr}, paint.Stop{Offset: 1, Color: opaque})
		}
		t := float32(get())
		if el {
			// Element: a white bar thumb with a soft shadow.
			if vert {
				y := r.Y + t*(r.H-4)
				tr := geom.Rect{X: r.X, Y: y, W: r.W, H: 4}
				l.Shadows(tr, geom.Rad(1), []paint.Shadow{{Blur: 2, Color: theme.RGBA(0, 0, 0, .6)}})
				l.Fill(tr, geom.Rad(1), theme.Hex("#ffffff"))
			} else {
				x := r.X + t*(r.W-4)
				tr := dtMirrorRect(rtl, geom.Rect{X: x, Y: r.Y, W: 4, H: r.H}, r)
				l.Shadows(tr, geom.Rad(1), []paint.Shadow{{Blur: 2, Color: theme.RGBA(0, 0, 0, .6)}})
				l.Fill(tr, geom.Rad(1), theme.Hex("#ffffff"))
			}
		} else {
			x := dtMirror(rtl, r, r.X+inset+t*(r.W-2*inset), 0)
			hc := theme.FromHSV(theme.HSV{H: st.hsva.H, S: 1, V: 1})
			if kind == "alpha" {
				hc = cur
			}
			cpHandle(l, tk, x, r.Y+r.H/2, hc)
		}
		if ia.FocusVisible {
			entFocusRing(c, l, rail, rad.TL)
		}
	}
}

// swatches adds a preset color grid named name.
func (w *ColorPickerW) swatches(presets []theme.Color, name string, size, gap float32) {
	c, th, tk := w.c, w.th, w.tk
	st := w.st
	el := w.el()
	// A radio group named by the preset label: each swatch is a radio
	// named by its hex value; arrow keys move and pick (roving Tab stop).
	cur := theme.FromHSVA(st.hsva)
	nodes := make([]*node, 0, len(presets))
	g := Row(c, func() {
		for i, col := range presets {
			col := col
			Key(c, i, func() {
				n := c.add("cp-preset", cpHex(col))
				n.ln.Width, n.ln.Height = size, size
				n.setRole(a11y.RoleRadio).setState(a11y.Checkable).stateIf(cur == col, a11y.Checked)
				ia := c.interact(n, true, input.CursorPointer)
				nodes = append(nodes, n)
				if ia.Clicked {
					w.applyColor(col)
				}
				n.over = func(l *paint.List, r geom.Rect) {
					if ia.FocusVisible {
						entFocusRing(c, l, r, th.RadiusSM)
					}
				}
				n.paint = func(l *paint.List, r geom.Rect) {
					rad := geom.Rad(tk.SwatchRadius)
					sel := theme.FromHSVA(st.hsva) == col
					if el && sel {
						l.Stroke(r.Expand(2), rad.Grow(2), 2, tk.SwatchActiveBorder)
					}
					cpSwatch(l, r, rad, col)
					if tk.SwatchBorder.A > 0 {
						l.Stroke(r, rad, 1, tk.SwatchBorder)
					}
					if !el && sel {
						mark := theme.Hex("#ffffff")
						if col.A < 128 || theme.Luma(col) > 0.75 {
							mark = theme.RGBA(0, 0, 0, .88)
						}
						is := size * 0.6
						drawIcon(l, icon.Check, r.X+(r.W-is)/2, r.Y+(r.H-is)/2, is, mark)
					}
					if ia.Hovered && !sel {
						l.Stroke(r, rad, 1, tk.SwatchHoverBorder)
					}
				}
			})
		}
	}, layout.Gap(gap), layout.WrapItems())
	g.n.setRole(a11y.RoleRadioGroup).setName(name)
	ids := make([]ID, len(nodes))
	sel := -1
	for i, n := range nodes {
		ids[i] = n.id
		if i < len(presets) && presets[i] == cur {
			sel = i
		}
	}
	rovingTabStop(nodes, sel)
	if to := rovingStep(c, ids, nil, rovingBoth); to >= 0 {
		w.applyColor(presets[to])
		navFocus(c, ids[to])
	}
}

func (w *ColorPickerW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	dis := w.disabled || w.n.disabled
	rad := geom.Rad(c.controlRadius(w.size))
	focus := w.st.pop.open || w.ia.Focused
	c.paintFrame(l, r, rad, w.n.id, frameState{hover: w.ia.Hovered, focus: focus && !w.el(), disabled: dis, status: w.status})
	if w.el() && focus && !dis && w.status == "" {
		l.Stroke(r, rad, 1, th.Primary.Base)
	}
	h := r.H
	// The swatch block sits at the inline start, the text after it.
	rtl := c.IsRTL()
	block := dtMirrorRect(rtl, geom.Rect{X: r.X + 4, Y: r.Y + 4, W: h - 8, H: h - 8}, r)
	empty := *w.val == (theme.Color{})
	if dis {
		l.Opacity(0.5)
	}
	if w.el() {
		bRad := geom.Rad(tk.TriggerSwatchRadius)
		in := block.Expand(-1)
		if empty {
			l.Stroke(block, bRad, 1, tk.TriggerSwatchBorder)
			drawIcon(l, icon.Close, block.X+(block.W-12)/2, block.Y+(block.H-12)/2, 12, th.TextTertiary)
		} else {
			cpSwatch(l, in, geom.Rad(1), *w.val)
			l.Stroke(block, bRad, 1, tk.TriggerSwatchBorder)
			mark := theme.Hex("#ffffff")
			if theme.Luma(*w.val) > 0.8 || w.val.A < 100 {
				mark = th.TextSecondary
			}
			drawIcon(l, icon.Down, block.X+(block.W-12)/2, block.Y+(block.H-12)/2, 12, mark)
		}
	} else {
		bRad := geom.Rad(tk.TriggerSwatchRadius)
		if empty {
			cpEmpty(l, block, bRad, tk.ClearColor)
		} else {
			cpSwatch(l, block, bRad, *w.val)
		}
		l.Stroke(block, bRad, 1, tk.TriggerSwatchBorder)
	}
	if dis {
		l.Pop()
	}
	if w.showText {
		s := w.text()
		col := th.Text
		if s == "" {
			s, col = w.loc.ColorPicker.None, th.TextQuaternary
		}
		if dis {
			col = th.TextDisabled
		}
		x := block.Right() + 6
		if rtl {
			x = block.X - 6 - entTextW(c, s, c.controlFont(w.size), text.Regular)
		}
		entText(c, l, s, x, r, c.controlFont(w.size), text.Regular, col, 0)
	}
	if w.ia.FocusVisible {
		entFocusRing(c, l, r, rad.TL)
	}
}
