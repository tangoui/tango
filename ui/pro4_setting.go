package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// SettingDrawer (Ant Design Pro SettingDrawer)
//
//	var settings = ui.DefaultProSettings()
//	ui.ProSettingsProvider(c, &settings, func() { // dark, primary, compact, color weak
//		cfg := ui.ProLayoutConfig{Title: "Admin", Menu: menu, Selected: &sel}
//		settings.Apply(&cfg) // layout, sider theme, content width, fixed header
//		ui.ProLayout(c, cfg, page)
//		ui.SettingDrawer(c, &settings).Show()
//	})
//
// A floating settings button opens a right drawer that edits ProSettings:
// overall style (light menu / dark menu / dark mode), theme color, layout
// (side / top / mix), content width, fixed header and side menu, color
// weak and compact modes. ProSettingsProvider turns the settings into a
// ConfigProvider (dark algorithm, primary color, compact sizes) and Apply
// copies the layout ones into a ProLayoutConfig, so every change applies
// at once.
//
// Differences from Pro: there is no CSS filter, so the color weak mode
// switches to the high-contrast variant of the theme (theme.HighContrast)
// instead of desaturating the page; the sider of a Tango ProLayout always
// stays fixed (it fills the window height), so FixSiderbar is kept for
// round-tripping Pro settings and reported through RouteContext only.

// ProSettings are the shell settings edited by SettingDrawer.
type ProSettings struct {
	NavTheme     string      // "light", "dark" (default) or "realDark"
	PrimaryColor theme.Color // zero: the theme's brand color
	Layout       string      // "side" (default), "top" or "mix"
	ContentWidth string      // "Fluid" (default) or "Fixed"
	FixedHeader  bool
	FixSiderbar  bool
	ColorWeak    bool
	Compact      bool
}

// DefaultProSettings returns Pro's defaults: dark menu, side layout,
// fluid content, fixed header and side menu.
func DefaultProSettings() ProSettings {
	return ProSettings{NavTheme: "dark", Layout: "side", ContentWidth: "Fluid", FixedHeader: true, FixSiderbar: true}
}

// Apply copies the layout settings into cfg.
func (s ProSettings) Apply(cfg *ProLayoutConfig) {
	if s.Layout != "" {
		cfg.Layout = s.Layout
	}
	cfg.LightSider = s.NavTheme == "light"
	if s.ContentWidth != "" {
		cfg.ContentWidth = s.ContentWidth
	}
	cfg.NoFixedHeader = !s.FixedHeader
}

// ProSettingsPresetColors are the theme colors offered by SettingDrawer
// (Pro's daybreak, dust, volcano, sunset, cyan, green, geekblue, purple).
var ProSettingsPresetColors = []theme.Color{
	theme.Hex("#1677ff"), theme.Hex("#f5222d"), theme.Hex("#fa541c"), theme.Hex("#faad14"),
	theme.Hex("#13c2c2"), theme.Hex("#52c41a"), theme.Hex("#2f54eb"), theme.Hex("#722ed1"),
}

type pro4ThemeCache struct {
	base *theme.Theme
	weak bool
	th   *theme.Theme
}

// ProSettingsProvider builds fn inside a ConfigProvider following s:
// realDark selects the dark algorithm, PrimaryColor the brand color,
// Compact the compact sizes, and ColorWeak the high-contrast theme.
// Like ConfigProvider.Theme, the theme applies to the whole window: call
// it at the root.
func ProSettingsProvider(c *Context, s *ProSettings, fn func()) {
	cp := ConfigProvider(c).Dark(s.NavTheme == "realDark").Compact(s.Compact)
	if s.PrimaryColor.A > 0 {
		cp.PrimaryColor(s.PrimaryColor)
	}
	cp.Body(func() {
		if !s.ColorWeak {
			fn()
			return
		}
		cache := State[pro4ThemeCache](c, hashID(c.scope, "pro4-weak"), nil)
		if cache.base != c.Theme || cache.th == nil {
			cache.base, cache.th = c.Theme, theme.HighContrast(c.Theme)
		}
		if c.Theme != cache.th && !c.Theme.HighContrast {
			c.Theme = cache.th
			c.invalid = true
		}
		fn()
	})
}

// SettingDrawerW builds a SettingDrawer.
type SettingDrawerW struct {
	c         *Context
	s         *ProSettings
	open      *bool
	trigger   bool
	colors    []theme.Color
	hideItems map[string]bool
	changed   bool
}

type pro4DrawerState struct{ open bool }

// SettingDrawer starts the settings drawer editing *s; chain options, then
// Show.
//
// ConfigProvider.Defaults("SettingDrawer", ...) keys: "hideColors",
// "hideLayout", "hideOther" (bool) hide those sections.
func SettingDrawer(c *Context, s *ProSettings) *SettingDrawerW {
	w := &SettingDrawerW{c: c, s: s, trigger: true, colors: ProSettingsPresetColors, hideItems: map[string]bool{}}
	for _, k := range []string{"hideColors", "hideLayout", "hideOther"} {
		if Prop(c, "SettingDrawer", k, false) {
			w.hideItems[k] = true
		}
	}
	return w
}

// Open binds the open state (otherwise the drawer keeps its own).
func (w *SettingDrawerW) Open(v *bool) *SettingDrawerW { w.open = v; return w }

// NoTrigger hides the floating button (open it with Open).
func (w *SettingDrawerW) NoTrigger() *SettingDrawerW { w.trigger = false; return w }

// Colors replaces the preset theme colors.
func (w *SettingDrawerW) Colors(cols []theme.Color) *SettingDrawerW { w.colors = cols; return w }

// Changed reports a settings change this frame.
func (w *SettingDrawerW) Changed() bool { return w.changed }

// Show builds the trigger and, when open, the drawer.
func (w *SettingDrawerW) Show() *SettingDrawerW {
	c := w.c
	t := c.Locale().SettingDrawer
	open := w.open
	if open == nil {
		open = &State[pro4DrawerState](c, hashID(c.scope, "setting-drawer"), nil).open
	}
	if w.trigger {
		if FloatButton(c).Icon(icon.Setting).A11yLabel(t.Open).Tooltip(t.Title).Clicked() {
			*open = !*open
			c.invalid = true
		}
	}
	if !*open {
		return w
	}
	Drawer(c, open, t.Title).Size(320).Body(func() {
		Column(c, func() { w.sections(t) }, layout.Gap(24))
	})
	return w
}

func (w *SettingDrawerW) sections(t SettingDrawerLocale) {
	c, s := w.c, w.s
	w.section(t.NavTheme, func() {
		w.choice(t.NavTheme, &s.NavTheme, []pro4Choice{
			{"light", t.Light, pro4PreviewStyle(false, false)},
			{"dark", t.Dark, pro4PreviewStyle(true, false)},
			{"realDark", t.RealDark, pro4PreviewStyle(true, true)},
		})
	})
	if !w.hideItems["hideColors"] {
		w.section(t.PrimaryColor, func() { w.swatches(t) })
	}
	if !w.hideItems["hideLayout"] {
		w.section(t.Layout, func() {
			w.choice(t.Layout, &s.Layout, []pro4Choice{
				{"side", t.Side, pro4PreviewLayout("side")},
				{"top", t.Top, pro4PreviewLayout("top")},
				{"mix", t.Mix, pro4PreviewLayout("mix")},
			})
		})
		Column(c, func() {
			w.row(t.ContentWidth, func() {
				v := s.ContentWidth
				if v == "" {
					v = "Fluid"
				}
				if Select(c, &v, []Option{{Label: t.Fluid, Value: "Fluid"}, {Label: t.Fixed, Value: "Fixed"}}).
					Size(Small).Width(96).A11yLabel(t.ContentWidth).Changed() {
					s.ContentWidth = v
					w.changed = true
				}
			})
			w.switchRow(t.FixedHeader, &s.FixedHeader)
			w.switchRow(t.FixSiderbar, &s.FixSiderbar)
		}, layout.Gap(4))
	}
	if !w.hideItems["hideOther"] {
		w.section(t.Other, func() {
			Column(c, func() {
				w.switchRow(t.ColorWeak, &s.ColorWeak)
				w.switchRow(t.Compact, &s.Compact)
			}, layout.Gap(4))
		})
	}
}

// section is a titled block.
func (w *SettingDrawerW) section(title string, fn func()) {
	c, th := w.c, w.c.Theme
	Column(c, func() {
		Text(c, title).Weight(text.Medium).Color(th.TextHeading)
		fn()
	}, layout.Gap(12)).Semantics(a11y.RoleGroup, title)
}

// row is a label with a control at the right.
func (w *SettingDrawerW) row(label string, fn func()) {
	c := w.c
	Row(c, func() {
		Text(c, label).Layout(layout.Grow(1), layout.MinW(0))
		fn()
	}, layout.Middle(), layout.MinH(36))
}

func (w *SettingDrawerW) switchRow(label string, v *bool) {
	w.row(label, func() {
		if Switch(w.c, v).Small().A11yLabel(label).Changed() {
			w.changed = true
		}
	})
}

// pro4Choice is one option of a picture choice.
type pro4Choice struct {
	value, label string
	paint        func(l *paint.List, r geom.Rect, th *theme.Theme, rtl bool)
}

// choice is a radio group of 48×40 preview pictures (arrow keys move,
// Enter / Space pick).
func (w *SettingDrawerW) choice(name string, cur *string, opts []pro4Choice) {
	c, th := w.c, w.c.Theme
	var nodes []*node
	var ids []ID
	sel := 0
	grp := Row(c, func() {
		for i, o := range opts {
			o := o
			on := *cur == o.value || (*cur == "" && i == 0)
			if on {
				sel = i
			}
			n := c.add("setting-choice", o.value)
			n.setRole(a11y.RoleRadio).setName(o.label).stateIf(on, a11y.Checked|a11y.Selected)
			n.ln.Width, n.ln.Height = 48, 40
			n.ownKeys = true
			ia := c.interact(n, true, input.CursorPointer)
			nodes, ids = append(nodes, n), append(ids, n.id)
			if ia.Clicked && !on {
				*cur = o.value
				w.changed = true
				c.invalid = true
			}
			n.paint = func(l *paint.List, r geom.Rect) {
				l.Clip(r, geom.Rad(th.RadiusSM))
				o.paint(l, r, th, c.IsRTL())
				l.Pop()
				col := th.BorderSecondary
				if on {
					col = th.Primary.Base
				} else if ia.Hovered {
					col = th.Primary.BorderHover
				}
				l.Stroke(r, geom.Rad(th.RadiusSM), map[bool]float32{true: 2, false: 1}[on], col)
				if on {
					d := geom.Rect{X: c.inline(r).X(r.W-16, 12), Y: r.Bottom() - 16, W: 12, H: 12} // bottom-end corner
					l.Fill(d, geom.Rad(6), th.Primary.Base)
					shad2dIconCheck(l, d, th.TextOnPrimary)
				}
				if ia.FocusVisible {
					drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
				}
			}
		}
	}, layout.Gap(12), layout.Middle())
	grp.Semantics(a11y.RoleRadioGroup, name)
	rovingTabStop(nodes, sel)
	if to := rovingStep(c, ids, nil, rovingHorizontal); to >= 0 {
		*cur = opts[to].value
		w.changed = true
		navFocus(c, ids[to])
	}
}

// swatches is the theme color picker (radio group of round swatches).
func (w *SettingDrawerW) swatches(t SettingDrawerLocale) {
	c, th, s := w.c, w.c.Theme, w.s
	var nodes []*node
	var ids []ID
	sel := -1
	grp := Row(c, func() {
		for i, col := range w.colors {
			col := col
			on := s.PrimaryColor == col || (s.PrimaryColor.A == 0 && col == th.Primary.Base)
			if on {
				sel = i
			}
			hex := theme.HexString(col)
			n := c.add("setting-color", hex)
			n.setRole(a11y.RoleRadio).setName(locFmt(t.ColorFmt, hex)).stateIf(on, a11y.Checked|a11y.Selected)
			n.ln.Width, n.ln.Height = 22, 22
			n.ownKeys = true
			ia := c.interact(n, true, input.CursorPointer)
			nodes, ids = append(nodes, n), append(ids, n.id)
			if ia.Clicked {
				s.PrimaryColor = col
				w.changed = true
				c.invalid = true
			}
			n.paint = func(l *paint.List, r geom.Rect) {
				l.Fill(r, geom.Rad(4), col)
				if on {
					shad2dIconCheck(l, r.Inset(geom.All(4)), theme.Hex("#ffffff"))
				}
				if ia.FocusVisible {
					drawFocusRing(l, th, r, geom.Rad(4))
				}
			}
		}
	}, layout.Gap(8), layout.WrapItems())
	grp.Semantics(a11y.RoleRadioGroup, t.PrimaryColor)
	rovingTabStop(nodes, max(0, sel))
	if to := rovingStep(c, ids, nil, rovingHorizontal); to >= 0 {
		s.PrimaryColor = w.colors[to]
		w.changed = true
		navFocus(c, ids[to])
	}
}

// pro4PreviewStyle paints a light / dark menu or dark mode thumbnail (the
// sider on the start side: on the right when rtl).
func pro4PreviewStyle(darkMenu, darkAll bool) func(*paint.List, geom.Rect, *theme.Theme, bool) {
	return func(l *paint.List, r geom.Rect, th *theme.Theme, rtl bool) {
		page, side, top := theme.Hex("#f0f2f5"), theme.Hex("#ffffff"), theme.Hex("#ffffff")
		if darkMenu {
			side = theme.Hex("#001529")
		}
		if darkAll {
			page, side, top = theme.Hex("#141414"), theme.Hex("#1f1f1f"), theme.Hex("#1f1f1f")
		}
		l.Fill(r, geom.Radii{}, page)
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H * .25}, geom.Radii{}, top)
		sr := geom.Rect{X: r.X, Y: r.Y, W: r.W * .3, H: r.H}
		if rtl {
			sr = mirrorRect(sr, r)
		}
		l.Fill(sr, geom.Radii{}, side)
	}
}

// pro4PreviewLayout paints a side / top / mix layout thumbnail (the sider
// on the start side: on the right when rtl).
func pro4PreviewLayout(kind string) func(*paint.List, geom.Rect, *theme.Theme, bool) {
	return func(l *paint.List, r geom.Rect, th *theme.Theme, rtl bool) {
		dark := theme.Hex("#001529")
		l.Fill(r, geom.Radii{}, theme.Hex("#f0f2f5"))
		hdr := geom.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H * .25}
		side := geom.Rect{X: r.X, Y: r.Y, W: r.W * .3, H: r.H}
		if rtl {
			side = mirrorRect(side, r)
		}
		switch kind {
		case "top":
			l.Fill(hdr, geom.Radii{}, dark)
		case "mix":
			l.Fill(hdr, geom.Radii{}, dark)
			side.Y, side.H = hdr.Bottom(), r.H-hdr.H
			l.Fill(side, geom.Radii{}, theme.Hex("#ffffff"))
		default:
			l.Fill(hdr, geom.Radii{}, theme.Hex("#ffffff"))
			l.Fill(side, geom.Radii{}, dark)
		}
	}
}
