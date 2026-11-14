package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Spin

// drawSpinner paints the default loading indicator in the primary color.
func drawSpinner(c *Context, l *paint.List, r geom.Rect) {
	drawSpinDots(c, l, r, c.Theme.Primary.Base)
}

// drawSpinDots paints the Ant four-dot indicator.
func drawSpinDots(c *Context, l *paint.List, r geom.Rect, col theme.Color) {
	ang := spinAngle(c) * 1000 / 1200
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rad := r.W * 9 / 40          // four dots, each 9/20 of the box
	off := (r.W/2 - rad) * 1.414 // dots sit in the corners
	for i := 0; i < 4; i++ {
		a := ang + float64(i)*math.Pi/2 + math.Pi/4
		x := cx + float32(math.Cos(a))*off
		y := cy + float32(math.Sin(a))*off
		op := 0.3 + 0.7*float64(i)/3
		l.Path(paint.NewPath().Circle(x, y, rad), theme.Fade(col, op))
	}
}

// drawSpinIndicator paints the flavor's indicator, or a progress ring when
// percent >= 0.
func drawSpinIndicator(c *Context, l *paint.List, r geom.Rect, percent float64, col theme.Color) {
	th := c.Theme
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if percent >= 0 {
		w := max(2, r.W/8)
		rad := r.W/2 - w/2
		l.StrokePath(paint.NewPath().Arc(cx, cy, rad, 0, 2*math.Pi), paint.StrokeStyle{Width: w}, th.FillSecondary)
		if percent > 0 {
			l.StrokePath(paint.NewPath().Arc(cx, cy, rad, -math.Pi/2, 2*math.Pi*min(percent, 100)/100), paint.StrokeStyle{Width: w, Cap: paint.CapRound}, col)
		}
		return
	}
	if th.Flavor != theme.Ant { // Element and shadcn (lucide loader) draw a rotating arc
		w := max(2, r.W/22)
		if th.Flavor == theme.StyleShadcn {
			w = max(1.5, r.W/12)
		}
		l.StrokePath(paint.NewPath().Arc(cx, cy, r.W/2-w, spinAngle(c)*0.8, 1.4*math.Pi), paint.StrokeStyle{Width: w, Cap: paint.CapRound}, col)
		return
	}
	drawSpinDots(c, l, r, col)
}

type spinState struct {
	since time.Time
}

// SpinW is a loading indicator: sizes, tip, delayed show, progress percent
// and a nested mode (Wrap) that dims the wrapped content.
type SpinW struct {
	_ [0]func()

	c        *Context
	n        *node
	z        *lazy
	size     ControlSize
	sizeSet  bool
	custom   float32
	tip      string
	delay    time.Duration
	spinning bool
	percent  float64 // <0: none
	shown    bool
}

// Spin adds a loading indicator; chain options. Wrap turns it into a
// container that covers its content while spinning:
//
//	ui.Spin(c).Size(ui.Large).Tip("加载中")
//	ui.Spin(c).Spinning(loading).Wrap(func() { ... })
func Spin(c *Context) *SpinW {
	s := &SpinW{c: c, spinning: true, percent: -1}
	s.defaults() // ConfigProvider.Defaults("Spin", ...): size, delay, tip
	s.n = c.add("spin", "")
	s.n.ln.NoStretch = true
	s.n.ln.Measure = func(_, _ float32) geom.Size {
		if !s.shown {
			return geom.Size{}
		}
		return s.measure()
	}
	s.n.paint = func(l *paint.List, r geom.Rect) {
		if s.shown {
			s.paint(l, r, false)
		}
	}
	s.z = c.lazy(s.n, func() {
		s.shown = s.visible(s.n.id)
		if s.shown {
			s.n.label = s.tip // lets tests locate a visible spin by its tip
			s.annotate(s.n)
		}
	})
	return s
}

// Size sets small, middle (default) or large.
func (s *SpinW) Size(sz ControlSize) *SpinW { s.size, s.sizeSet = sz, true; return s }

// IndicatorSize sets a custom indicator size in px.
func (s *SpinW) IndicatorSize(px float32) *SpinW { s.custom = px; return s }

// Tip shows a text under the indicator.
func (s *SpinW) Tip(t string) *SpinW { s.tip = t; return s }

// Delay shows the indicator only after spinning lasted d (avoids flashes).
func (s *SpinW) Delay(d time.Duration) *SpinW { s.delay = d; return s }

// Spinning toggles the indicator (default on).
func (s *SpinW) Spinning(on bool) *SpinW { s.spinning = on; return s }

// Percent replaces the indicator with a progress ring (0..100).
func (s *SpinW) Percent(p float64) *SpinW { s.percent = p; return s }

func (s *SpinW) px() float32 {
	if s.custom > 0 {
		return s.custom
	}
	sz := s.size
	if !s.sizeSet {
		sz, _ = s.c.ComponentSize()
	}
	el := navEl(s.c)
	switch sz {
	case Small:
		if el {
			return 28
		}
		return 14
	case Large:
		if el {
			return 50
		}
		return 32
	}
	if el {
		return 42
	}
	return 20
}

func (s *SpinW) visible(id ID) bool {
	c := s.c
	st := State[spinState](c, hashID(id, "spin"), nil)
	if !s.spinning {
		st.since = time.Time{}
		return false
	}
	if st.since.IsZero() {
		st.since = c.now
	}
	if s.delay > 0 && !c.noAnim && c.now.Sub(st.since) < s.delay {
		c.WakeAt(st.since.Add(s.delay))
		return false
	}
	return true
}

func (s *SpinW) tipStyle() text.Style {
	th := s.c.Theme
	fsz := th.FontSize
	if !navEl(s.c) && s.size == Small && s.sizeSet {
		fsz = th.FontSizeSM
	}
	return s.c.style(fsz, text.Regular)
}

func (s *SpinW) measure() geom.Size {
	px := s.px()
	if s.tip != "" {
		st := s.tipStyle()
		return geom.Size{W: max(px, text.Width(s.tip, st)), H: px + 8 + st.LineHeight}
	}
	return geom.Size{W: px, H: px}
}

func (s *SpinW) paint(l *paint.List, r geom.Rect, onDark bool) {
	th := s.c.Theme
	col := th.Primary.Base
	if onDark {
		col = th.TextOnPrimary
	}
	px := s.px()
	ir := geom.Rect{X: r.X + (r.W-px)/2, Y: r.Y, W: px, H: px}
	drawSpinIndicator(s.c, l, ir, s.percent, col)
	if s.tip != "" {
		lay := text.Shape(s.tip, s.tipStyle(), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, ir.Bottom()+8), lay.Glyphs, col)
	}
}

// annotate marks n as the busy indicator: a status named by the tip (or
// "loading"), or a progress bar when Percent is set.
func (s *SpinW) annotate(n *node) {
	name := s.tip
	if name == "" {
		name = s.c.Locale().A11y.Loading
	}
	n.setName(name).setState(a11y.Busy)
	if s.percent >= 0 {
		n.setRole(a11y.RoleProgressBar).fill(s)
		return
	}
	n.setRole(a11y.RoleStatus)
}

func (s *SpinW) semantics(n *a11y.Node) {
	if s.percent >= 0 {
		rangeSem(n, min(max(s.percent, 0), 100), 0, 100, 0)
		n.Value = FormatNumber(s.percent, 0) + "%"
	}
}

// indicator adds a separate indicator leaf (used inside masks).
func (s *SpinW) indicator(onDark bool) *node {
	n := s.c.add("spin-indicator", s.tip)
	s.annotate(n)
	n.ln.NoStretch = true
	n.ln.Measure = func(_, _ float32) geom.Size { return s.measure() }
	n.paint = func(l *paint.List, r geom.Rect) { s.paint(l, r, onDark) }
	return n
}

// Wrap builds content and, while spinning, dims it and centers the
// indicator above it (Ant nested Spin).
func (s *SpinW) Wrap(fn func()) *Box {
	c, th := s.c, s.c.Theme
	s.z.done = true // a container now, not a leaf
	n := s.n
	n.ln.NoStretch = false
	n.ln.Measure = nil
	n.ln.Dir = layout.Stack
	b := &Box{c: c, n: n}
	n.paint = b.paintBox
	on := s.visible(n.id)
	n.stateIf(on, a11y.Busy)
	if on {
		n.setRole(a11y.RoleGroup)
	}
	a := c.Anim(n.id, "on", b2f(on), th.DurationMid)
	b.Body(func() {
		content := Column(c, fn)
		if a > 0 {
			content.Opacity(1 - 0.5*a)
		}
		if on || a > 0 {
			mask := Column(c, func() { s.indicator(false) }, layout.Center())
			mask.Bg(theme.Fade(th.BgContainer, 0.5)).Opacity(a)
			mask.n.hit, mask.n.blocks = true, true
		}
	})
	return b
}

// ---------------------------------------------------------------------------
// Loading (a region or the whole window covered by a mask and spinner;
// Element v-loading)

// LoadingW covers a region or the window while loading.
type LoadingW struct {
	_ [0]func()

	c       *Context
	on      bool
	tip     string
	bg      theme.Color
	bgSet   bool
	full    bool
	size    ControlSize
	sizeSet bool
}

// Loading starts a loading mask shown while on; chain options, then Body
// (cover a region) or Fullscreen (cover the window):
//
//	ui.Loading(c, busy).Tip("加载中").Body(func() { ... })
//	ui.Loading(c, busy).Fullscreen()
//
// ConfigProvider.Defaults("Spin", ...) "size" and "tip" apply.
func Loading(c *Context, on bool) *LoadingW {
	m := &LoadingW{c: c, on: on}
	m.configDefaults()
	return m
}

// Tip shows a caption under the spinner.
func (m *LoadingW) Tip(s string) *LoadingW { m.tip = s; return m }

// Background overrides the mask color.
func (m *LoadingW) Background(col theme.Color) *LoadingW { m.bg, m.bgSet = col, true; return m }

// Size sets the spinner size.
func (m *LoadingW) Size(s ControlSize) *LoadingW { m.size, m.sizeSet = s, true; return m }

func (m *LoadingW) maskColor() (theme.Color, bool) {
	th := m.c.Theme
	switch {
	case m.bgSet:
		return m.bg, false
	case navEl(m.c):
		return theme.Fade(th.BgContainer, 0.9), false
	case m.full:
		return th.BgMask, true // Ant fullscreen Spin: dark mask, white indicator
	}
	return theme.Fade(th.BgContainer, 0.6), false
}

func (m *LoadingW) spin() *SpinW {
	return &SpinW{c: m.c, size: m.size, sizeSet: m.sizeSet, tip: m.tip, spinning: true, percent: -1}
}

// Body builds the region and covers it while loading.
func (m *LoadingW) Body(fn func()) *Box {
	c, th := m.c, m.c.Theme
	b := c.box("loading", layout.Stack, nil)
	if m.on {
		b.n.setRole(a11y.RoleGroup).setState(a11y.Busy)
	}
	a := c.Anim(b.n.id, "on", b2f(m.on), th.DurationMid)
	b.Body(func() {
		Column(c, fn)
		if a > 0.01 {
			col, dark := m.maskColor()
			mask := Column(c, func() { m.spin().indicator(dark) }, layout.Center())
			mask.Bg(col).Opacity(a)
			mask.n.hit, mask.n.blocks = true, true
			mask.n.cursor = input.CursorDefault
		}
	})
	return b
}

// Fullscreen covers the whole window while loading.
func (m *LoadingW) Fullscreen() *LoadingW {
	c, th := m.c, m.c.Theme
	m.full = true
	id := hashID(c.scope, "loading-fullscreen")
	a := c.Anim(id, "on", b2f(m.on), th.DurationMid)
	if a <= 0.01 {
		return m
	}
	col, dark := m.maskColor()
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() { m.spin().indicator(dark) }, layout.Center())
	ov.Bg(col).Opacity(a)
	return m
}

// ---------------------------------------------------------------------------
// Alert

// AlertW is an inline status banner.
type AlertW struct {
	_ [0]func()

	c        *Context
	status   string
	title    string
	desc     string
	showIcon bool
	closable bool
	banner   bool
	closed   bool
	center   bool
	effect   AlertEffect
	action   func()
	th       *theme.Theme
	tk       *theme.AlertToken
}

// AlertEffect selects how an Alert is filled (Element's effect).
type AlertEffect string

const (
	// AlertEffectLight tints the box with the light status background
	// (the default).
	AlertEffectLight AlertEffect = "light"
	// AlertEffectDark fills the box with the solid status color and
	// draws the text and icon on it in a contrasting color.
	AlertEffectDark AlertEffect = "dark"
)

// Alert adds a banner; call Show after options. status: success, info,
// warning, error.
//
// ConfigProvider.Defaults("Alert", ...) keys: "showIcon", "closable",
// "banner" (bool).
//
// Under shadcn/ui it is the bordered Alert card with an outlined
// (lucide-style) icon; "error" is the destructive variant.
func Alert(c *Context, status, title string) *AlertW {
	th := c.ThemeFor(theme.CompAlert)
	return &AlertW{c: c, status: status, title: title, showIcon: Prop(c, "Alert", "showIcon", true), closable: Prop(c, "Alert", "closable", false),
		banner: Prop(c, "Alert", "banner", false), th: th, tk: &th.Components.Alert}
}

// alertLook is the box and icon colors of an alert status.
type alertLook struct{ bg, border, icon theme.Color }

// look returns the token colors of the alert's status (other statuses
// take the theme palette of that name).
func (a *AlertW) look() alertLook {
	tk := a.tk
	if a.effect == AlertEffectDark {
		return a.darkLook()
	}
	switch a.status {
	case "success":
		return alertLook{tk.SuccessBg, tk.SuccessBorderColor, tk.SuccessIconColor}
	case "info":
		return alertLook{tk.InfoBg, tk.InfoBorderColor, tk.InfoIconColor}
	case "warning":
		return alertLook{tk.WarningBg, tk.WarningBorderColor, tk.WarningIconColor}
	case "error", "danger":
		return alertLook{tk.ErrorBg, tk.ErrorBorderColor, tk.ErrorIconColor}
	}
	p := a.th.ColorOf(a.status)
	lk := alertLook{p.Bg, p.Border, p.Base}
	if a.th.Flavor == theme.StyleShadcn {
		lk.bg, lk.border, lk.icon = tk.InfoBg, tk.InfoBorderColor, tk.InfoIconColor
	} else if tk.InfoBorderColor.A == 0 {
		lk.border = theme.Color{}
	}
	return lk
}

// darkLook returns the dark effect colors: the solid status fill, no
// border, and the contrasting text color as the icon (and text) color.
func (a *AlertW) darkLook() alertLook {
	tk := a.tk
	var bg theme.Color
	switch a.status {
	case "success":
		bg = tk.SuccessDarkBg
	case "info":
		bg = tk.InfoDarkBg
	case "warning":
		bg = tk.WarningDarkBg
	case "error", "danger":
		bg = tk.ErrorDarkBg
	default:
		bg = a.th.ColorOf(a.status).Base
	}
	return alertLook{bg: bg, icon: a.th.TextOn(bg, tk.DarkTextColor)}
}

// alertIcon adds the filled status circle of size px in col.
func alertIcon(c *Context, status string, size float32, col, fg theme.Color) *node {
	n := c.add("status-icon", status)
	n.ln.Width, n.ln.Height = size, size
	n.paint = func(l *paint.List, r geom.Rect) { drawStatusIcon(l, status, r, col, fg) }
	return n
}

// Action builds an operation area at the right of the text (Ant action),
// e.g. a small button.
func (a *AlertW) Action(fn func()) *AlertW { a.action = fn; return a }

// actionArea builds the Action content.
func (a *AlertW) actionArea() {
	if a.action == nil {
		return
	}
	b := Row(a.c, a.action, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Left: 8}))
	if a.desc == "" {
		b.n.ln.AlignSelf = layout.AlignCenter
	}
}

func (a *AlertW) Description(d string) *AlertW { a.desc = d; return a }
func (a *AlertW) NoIcon() *AlertW              { a.showIcon = false; return a }
func (a *AlertW) Closable() *AlertW            { a.closable = true; return a }
func (a *AlertW) Banner() *AlertW              { a.banner = true; return a }

// Effect selects the fill: AlertEffectLight (default) or AlertEffectDark,
// the solid status color with contrasting text (Element effect="dark").
func (a *AlertW) Effect(e AlertEffect) *AlertW { a.effect = e; return a }

// Center centers the icon and text horizontally (Element center); the
// close button stays at the end.
func (a *AlertW) Center() *AlertW { a.center = true; return a }

// darkText returns the text color under the dark effect, and ok false
// for the light effect.
func (a *AlertW) darkText(lk alertLook) (theme.Color, bool) {
	if a.effect != AlertEffectDark {
		return theme.Color{}, false
	}
	return lk.icon, true
}

// leadSpacer opens a centered alert: a growing spacer, plus room matching
// the close button so the content stays centered on the box.
func (a *AlertW) leadSpacer() {
	if !a.center {
		return
	}
	Spacer(a.c)
	if a.closable {
		Div(a.c, layout.W(16))
	}
}

// trailSpacer closes a centered alert before the action and close button.
func (a *AlertW) trailSpacer() {
	if a.center {
		Spacer(a.c)
	}
}

// closeButton adds the close icon button, in fg under the dark effect.
func (a *AlertW) closeButton(lk alertLook) {
	if !a.closable {
		return
	}
	ib := IconButton(a.c, icon.Close, a.c.Locale().Close).Size(16, 12).NoHoverBg()
	if fg, ok := a.darkText(lk); ok {
		ib.Color(fg)
	}
	if ib.Clicked() {
		a.closed = true
	}
}

// grow is the text column's main-axis option: it fills the row unless the
// alert is centered.
func (a *AlertW) grow() layout.Option {
	if a.center {
		return layout.Grow(0)
	}
	return layout.Grow(1)
}

// textAlign is the alignment of title and description.
func (a *AlertW) textAlign() TextAlign {
	if a.center {
		return AlignCenter
	}
	return AlignStart
}

// Closed reports a click on the close button.
func (a *AlertW) Closed() bool { return a.closed }

// Show builds the alert.
func (a *AlertW) Show() *AlertW {
	c, th, tk := a.c, a.th, a.tk
	lk := a.look()
	if th.Flavor == theme.StyleShadcn {
		return a.showShadcn(lk)
	}
	el := th.Flavor == theme.Element
	big := a.desc != ""
	fg, dark := a.darkText(lk)
	pad := geom.VH(tk.DefaultPaddingBlock, tk.DefaultPaddingInline)
	if big {
		pad = geom.VH(tk.WithDescriptionPaddingBlock, tk.WithDescriptionPaddingInline)
	}
	b := Row(c, func() {
		a.leadSpacer()
		if a.showIcon {
			is, mr := tk.IconSize, tk.IconMarginInlineEnd
			if big {
				is, mr = tk.WithDescriptionIconSize, tk.WithDescriptionIconMarginEnd
			}
			glyph := th.BgContainer
			if dark {
				glyph = lk.bg
			}
			n := alertIcon(c, a.status, is, lk.icon, glyph)
			n.ln.Margin.Right = mr
			if !big {
				n.ln.AlignSelf = layout.AlignCenter
			}
		}
		Column(c, func() {
			t := Text(c, a.title).Align(a.textAlign())
			if big {
				t.Size(tk.TitleFontSize).Weight(text.Weight(tk.TitleFontWeight))
				d := Text(c, a.desc).Size(tk.DescriptionFontSize).Color(tk.DescriptionColor).Align(a.textAlign())
				if dark {
					d.Color(fg)
				}
				if !el {
					t.Layout(layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
				}
			}
			switch {
			case dark:
				t.Color(fg)
			case tk.TitleColor.A > 0:
				t.Color(tk.TitleColor)
			default:
				t.Color(lk.icon)
			}
		}, a.grow())
		a.trailSpacer()
		a.actionArea()
		a.closeButton(lk)
	}, layout.Padding(pad), layout.AlignItems(layout.AlignStart))
	a.annotate(b)
	b.Bg(lk.bg).Radius(tk.Radius)
	if lk.border.A > 0 {
		b.Border(1, lk.border)
		if a.banner {
			b.Radius(0).Border(0, theme.Color{})
		}
	}
	return a
}

// annotate exposes the alert box as a live region: errors and warnings as
// an alert (assertive), the rest as a status; the title is its name and
// the description its description.
func (a *AlertW) annotate(b *Box) {
	b.n.setRole(liveRole(a.status)).setName(a.title)
	if a.desc != "" && a.c.a11yOn {
		d := a.desc
		b.n.fill(semFunc(func(s *a11y.Node) { s.Description = d }))
	}
}

// showShadcn draws shadcn's Alert: a bordered card (rounded-lg, px-4 py-3)
// with the icon at the top left, a medium title and a muted description;
// the destructive (error) variant colors text and border red.
func (a *AlertW) showShadcn(lk alertLook) *AlertW {
	c, tk := a.c, a.tk
	danger := a.status == "error" || a.status == "danger"
	fg, dark := a.darkText(lk)
	big := a.desc != ""
	pad := geom.VH(tk.DefaultPaddingBlock, tk.DefaultPaddingInline)
	if big {
		pad = geom.VH(tk.WithDescriptionPaddingBlock, tk.WithDescriptionPaddingInline)
	}
	b := Row(c, func() {
		a.leadSpacer()
		if a.showIcon {
			is, mr := tk.IconSize, tk.IconMarginInlineEnd
			if big {
				is, mr = tk.WithDescriptionIconSize, tk.WithDescriptionIconMarginEnd
			}
			n := shad2cStatusNodeColor(c, a.status, is, lk.icon) // lucide-style outline glyph
			n.ln.Margin = geom.Insets{Right: mr, Top: 2}
		}
		Column(c, func() {
			t := Text(c, a.title).Size(tk.TitleFontSize).Weight(text.Weight(tk.TitleFontWeight)).Color(tk.TitleColor).Align(a.textAlign())
			switch {
			case dark:
				t.Color(fg)
			case danger:
				t.Color(lk.icon)
			}
			if big {
				d := Text(c, a.desc).Size(tk.DescriptionFontSize).Color(tk.DescriptionColor).Align(a.textAlign())
				switch {
				case dark:
					d.Color(theme.Fade(fg, .9))
				case danger:
					d.Color(theme.Fade(lk.icon, .9))
				}
			}
		}, a.grow(), layout.Gap(tk.TitleMarginBottom))
		a.trailSpacer()
		a.actionArea()
		a.closeButton(lk)
	}, layout.Padding(pad), layout.AlignItems(layout.AlignStart))
	a.annotate(b)
	b.Bg(lk.bg).Radius(tk.Radius)
	if lk.border.A > 0 {
		b.Border(1, lk.border)
	}
	if a.banner {
		b.Radius(0)
	}
	return a
}

// ---------------------------------------------------------------------------
// Motion of Modal / Confirm / Drawer.
//
// Enter animations of the dialog-like layers live here so their style can be
// swapped in one place (Modal and the Confirm family share modalMotion,
// Drawer uses drawerMotion).

// Ant Design motion curves used by the zoom / mask transitions.
var (
	easeOutCirc   = cubicBezier(0.08, 0.82, 0.17, 1)
	easeInOutCirc = cubicBezier(0.78, 0.14, 0.15, 0.86)
)

// motionTo eases slot of id towards 1 (open) or 0 (closed) with separate
// enter / leave curves. A fresh slot starts at 0, so the first open frame
// begins the enter transition.
func motionTo(c *Context, id ID, slot string, open bool, dur time.Duration, in, out func(float64) float64) float32 {
	if open {
		return c.anim(id, slot, 0, 1, dur, in)
	}
	return c.anim(id, slot, 0, 0, dur, out)
}

// motionLive reports whether a dialog-like layer must still be drawn: open,
// or closed while its leave transition runs.
func motionLive(c *Context, id ID, open bool) bool {
	if open {
		return true
	}
	if c.anims[animKey{id, "open"}] == nil {
		return false
	}
	return motionTo(c, id, "open", false, c.Theme.DurationSlow, nil, motionOut(c)) > 0
}

// motionOut is the leave curve of dialog-like layers.
func motionOut(c *Context) func(float64) float64 {
	if c.Theme.Flavor != theme.Ant {
		return EaseIn
	}
	return easeInOutCirc
}

// modalAnim is the enter / leave state of a centered dialog.
type modalAnim struct {
	a, m    float32 // panel and mask progress, 0 hidden .. 1 shown
	el      bool
	shad    bool       // shadcn: fade + zoom 95% → 100%
	origin  geom.Point // zoom origin (screen); valid when hasOrig
	hasOrig bool
	leaving bool
	opening bool // first frame of the enter transition
}

type modalOrigin struct {
	p  geom.Point
	ok bool
}

// modalMotion advances the transition of dialog id towards open. ok is
// false once a closed dialog has finished leaving (nothing to draw).
//
// Ant zooms the panel from 0.2 to 1 about the click that opened it (or its
// centre) while fading it in; the mask fades separately. Element fades the
// panel in while it moves down 20px.
func modalMotion(c *Context, id ID, open bool) (modalAnim, bool) {
	if !motionLive(c, id, open) {
		return modalAnim{}, false
	}
	th := c.Theme
	el := th.Flavor == theme.Element
	og := State[modalOrigin](c, id^hashID("modal-origin"), nil)
	opening := false
	if open {
		if a := c.anims[animKey{id, "open"}]; a == nil || a.val == 0 && a.to == 0 {
			// Opening now: zoom from a click made just before (the trigger).
			og.ok = !c.lastClick.IsZero() && c.now.Sub(c.lastClick) < 500*time.Millisecond
			og.p = c.pressPos
			opening = true
		}
	}
	shad := th.Flavor == theme.StyleShadcn
	in, out := easeOutCirc, motionOut(c)
	if el || shad {
		in = EaseOut
	}
	m := modalAnim{el: el, shad: shad, origin: og.p, hasOrig: og.ok, leaving: !open, opening: opening}
	dur := th.DurationSlow
	if shad {
		dur = th.DurationMid
	}
	m.a = motionTo(c, id, "open", open, dur, in, out)
	m.m = motionTo(c, id, "mask", open, dur, EaseOut, EaseInOut)
	return m, true
}

// mask returns the opacity of the dimming mask.
func (m modalAnim) mask() float64 { return float64(m.m) }

// place adjusts the panel's overlay options; the motion itself is a paint
// transform (see apply), so placement is unchanged.
func (m modalAnim) place(o OverlayOptions) OverlayOptions { return o }

// apply styles the dialog panel for the current progress: opacity plus a
// zoom (Ant) or a short downward slide (Element), drawn as a transform so
// the panel's corners, shadow and text scale together without relayout.
func (m modalAnim) apply(dlg *Box) {
	dlg.Opacity(m.a)
	if m.a >= 1 {
		return
	}
	if m.el {
		dlg.Transform(geom.Translate(0, (1-m.a)*-20))
		return
	}
	if m.shad {
		dlg.Scale(0.95 + 0.05*m.a)
		return
	}
	k := 0.2 + 0.8*m.a
	if !m.hasOrig {
		dlg.Scale(k)
		return
	}
	o := m.origin
	dlg.n.xf = func(geom.Rect) geom.Affine { return geom.ScaleAround(k, k, o) }
}

// drawerMotion advances drawer id towards open and returns its progress
// (0..1; the panel is hidden by (1-a)*size beyond its edge). ok is false
// once a closed drawer has finished sliding out.
func drawerMotion(c *Context, id ID, open bool) (a float32, ok bool) {
	if !motionLive(c, id, open) {
		return 0, false
	}
	return motionTo(c, id, "open", open, c.Theme.DurationSlow, EaseOut, motionOut(c)), true
}

// ---------------------------------------------------------------------------
// Modal

type modalState struct {
	off      geom.Point
	from     geom.Point
	dragFrom geom.Point
	full     bool
}

// ModalW is a dialog over a mask: centered or top placement, draggable,
// fullscreen, custom footer and close options.
type ModalW struct {
	_ [0]func()

	c          *Context
	open       *bool
	title      string
	width      float32
	okText     string
	cancelText string
	footer     func()
	noFooter   bool
	ok, cancel bool
	maskClose  bool
	closable   bool
	danger     bool
	loading    bool
	centered   bool
	draggable  bool
	full       *bool
	fullToggle bool
	top        float32
	mask       bool
}

// Modal starts a dialog shown while *open; chain options, then Body.
func Modal(c *Context, open *bool, title string) *ModalW {
	w := float32(520)
	if navEl(c) {
		w = max(320, c.size.W*0.5)
	}
	m := &ModalW{c: c, open: open, title: title, width: w, maskClose: true, closable: true, mask: true}
	m.defaults() // ConfigProvider.Defaults("Modal", ...), see shad2c_defaults.go
	return m
}

func (m *ModalW) Width(px float32) *ModalW       { m.width = px; return m }
func (m *ModalW) OkText(s string) *ModalW        { m.okText = s; return m }
func (m *ModalW) CancelText(s string) *ModalW    { m.cancelText = s; return m }
func (m *ModalW) Footer(fn func()) *ModalW       { m.footer = fn; return m }
func (m *ModalW) NoFooter() *ModalW              { m.noFooter = true; return m }
func (m *ModalW) MaskClosable(on bool) *ModalW   { m.maskClose = on; return m }
func (m *ModalW) Closable(on bool) *ModalW       { m.closable = on; return m }
func (m *ModalW) Danger() *ModalW                { m.danger = true; return m }
func (m *ModalW) ConfirmLoading(on bool) *ModalW { m.loading = on; return m }

// Mask(false) hides the dimming mask (the page stays visible but blocked).
func (m *ModalW) Mask(on bool) *ModalW { m.mask = on; return m }

// Centered centers the dialog vertically.
func (m *ModalW) Centered() *ModalW { m.centered = true; return m }

// Top sets the distance from the window top (Ant 100, Element 15%).
func (m *ModalW) Top(px float32) *ModalW { m.top = px; return m }

// Draggable lets the user move the dialog by its header.
func (m *ModalW) Draggable() *ModalW { m.draggable = true; return m }

// Fullscreen binds the fullscreen state.
func (m *ModalW) Fullscreen(on *bool) *ModalW { m.full = on; return m }

// FullscreenToggle shows a maximize button in the header.
func (m *ModalW) FullscreenToggle() *ModalW { m.fullToggle = true; return m }

// Ok reports the OK button this frame.
func (m *ModalW) Ok() bool { return m.ok }

// Cancelled reports cancel / close / mask click / Escape this frame.
func (m *ModalW) Cancelled() bool { return m.cancel }

// Body builds the dialog content.
func (m *ModalW) Body(fn func()) *ModalW {
	c := m.c
	th := c.ThemeFor(theme.CompModal)
	tk := &th.Components.Modal
	id := hashID(c.scope, "modal", m.title)
	mo, live := modalMotion(c, id, *m.open)
	if !live {
		return m
	}
	leaving := mo.leaving
	el := navEl(c)
	shad := th.Flavor == theme.StyleShadcn
	loc := c.Locale()
	if m.okText == "" {
		m.okText = loc.OK
	}
	if m.cancelText == "" {
		m.cancelText = loc.Cancel
	}
	st := State[modalState](c, id, nil)
	if mo.opening {
		st.off = geom.Point{} // a dragged dialog reopens at its home position
	}
	full := st.full
	if m.full != nil {
		full = *m.full
	}
	mask := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {})
	if m.mask {
		mask.Bg(theme.Fade(tk.MaskBg, mo.mask()))
	}
	if m.maskClose && c.clicked[mask.n.id] > 0 {
		m.cancel = true
	}
	top := m.top
	if top == 0 {
		top = 100
		if el {
			top = c.size.H * 0.15
		}
	}
	place := ScreenTopCenter
	if m.centered || (shad && m.top == 0) {
		place = ScreenCenter // shadcn dialogs are always centered
	}
	var header *node
	width := m.width
	if full {
		place, top, width = ScreenFill, 0, c.size.W
	}
	dlg := Overlay(c, mo.place(OverlayOptions{Place: place, Margin: top, ShiftX: st.off.X, ShiftY: st.off.Y}), func() {
		pad := geom.VH(tk.PaddingBlock, tk.Padding)
		col := Column(c, func() {
			hb := Row(c, func() {
				t := Text(c, m.title).Layout(layout.Grow(1)).NoWrap().Ellipsis().Size(tk.TitleFontSize).Color(tk.TitleColor)
				if tk.TitleLineHeight > 0 {
					t.LineHeight(tk.TitleLineHeight)
				}
				if tk.TitleFontWeight != 0 {
					t.Weight(text.Weight(tk.TitleFontWeight))
				}
				if m.fullToggle {
					label := loc.Fullscreen
					g := icon.Fullscreen
					if full {
						label, g = loc.ExitFullscreen, icon.FullscreenExit
					}
					if navIconButton(c, g, label).Clicked {
						st.full = !full
						if m.full != nil {
							*m.full = st.full
						}
						st.off = geom.Point{}
						c.invalid = true
					}
				}
				if m.closable {
					ib := IconButton(c, icon.Close, loc.Close)
					if shad {
						ib.NoHoverBg()
					}
					if tk.CloseIconColor.A > 0 {
						ib.Color(tk.CloseIconColor)
					}
					if ib.Clicked() {
						m.cancel = true
					}
				}
			}, layout.Middle(), layout.Gap(4))
			header = hb.n
			if tk.HeaderBg.A > 0 {
				hb.Bg(tk.HeaderBg)
			}
			hb.n.ln.Margin.Bottom = tk.HeaderMarginBottom
			body := Column(c, fn)
			if full {
				body.n.ln.Grow = 1
			}
			if !m.noFooter {
				fr := Row(c, func() {
					if m.footer != nil {
						m.footer()
						return
					}
					if Button(c, m.cancelText).Clicked() {
						m.cancel = true
					}
					ok := Button(c, m.okText).Primary().Loading(m.loading)
					if m.danger {
						ok.Danger()
					}
					if ok.Clicked() {
						m.ok = true
					}
				}, layout.End(), layout.Gap(8), layout.Margin(geom.Insets{Top: tk.FooterMarginTop}))
				if tk.FooterBg.A > 0 {
					fr.Bg(tk.FooterBg)
				}
			}
		}, layout.Padding(pad))
		col.n.ln.Width = width
		if full {
			col.n.ln.Height = c.size.H
		}
	})
	dlg.Bg(tk.ContentBg).Radius(tk.Radius).Shadow(tk.Shadow)
	dlg.n.setRole(a11y.RoleDialog).setName(m.title).setState(a11y.Modal)
	if c.a11yOn {
		dlg.n.fill(semFunc(dialogSem))
	}
	if !leaving {
		c.TrapFocus(dlg.n.id)
	}
	if tk.BorderColor.A > 0 && !full {
		dlg.Border(1, tk.BorderColor)
	}
	mo.apply(dlg)
	if full {
		dlg.Radius(0)
	}
	if leaving {
		// The panel is fading out: it still blocks the page, but its
		// buttons, Escape and the mask no longer act.
		m.ok, m.cancel = false, false
		return m
	}
	if m.draggable && !full && header != nil {
		header.label = m.title // lets tests and screenshots locate the drag handle
		ia := c.interact(header, false, input.CursorMove)
		if ia.PressedNow {
			st.from, st.dragFrom = st.off, c.pointer
		}
		if c.Dragging(header.id) {
			nx := st.from.X + c.pointer.X - st.dragFrom.X
			ny := st.from.Y + c.pointer.Y - st.dragFrom.Y
			// Keep the header reachable.
			if r, ok := c.rects[dlg.n.id]; ok {
				baseX, baseY := r.X-st.off.X, r.Y-st.off.Y
				nx = max(-baseX, min(nx, c.size.W-baseX-r.W))
				ny = max(-baseY, min(ny, c.size.H-baseY-40))
			}
			if nx != st.off.X || ny != st.off.Y {
				st.off = geom.Point{X: nx, Y: ny}
				c.invalid = true
			}
		}
	}
	if c.EscapeAt(dlg.n.id) {
		m.cancel = true
	}
	if m.cancel {
		*m.open = false
		c.invalid = true
	}
	return m
}

// dialogSem lets screen readers close a dialog (Escape).
func dialogSem(s *a11y.Node) { s.Actions |= a11y.Dismiss }

// navIconButton is a small borderless icon button.
func navIconButton(c *Context, g *icon.Icon, label string) Interaction {
	th := c.Theme
	n := c.add("glyph-button", label)
	n.setRole(a11y.RoleButton)
	n.ln.Width, n.ln.Height = 22, 22
	n.ln.NoStretch = true
	ia := c.interact(n, true, input.CursorPointer)
	id := n.id
	n.paint = func(l *paint.List, r geom.Rect) {
		h := c.Anim(id, "hover", b2f(ia.Hovered), th.DurationMid)
		if h > 0 {
			l.Fill(r, geom.Rad(th.RadiusSM), theme.Fade(th.BgTextHover, float64(h)))
		}
		c.paintIcon(l, g, r.Expand(-4), theme.Lerp(th.TextTertiary, th.Text, h), 0)
		if ia.FocusVisible {
			l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
	return ia
}

// ---------------------------------------------------------------------------
// Drawer

// DrawerPlacement selects the edge a Drawer slides from. DrawerEnd (the
// zero value and default) and DrawerStart are logical: in RTL the end is
// the left edge. DrawerRight and DrawerLeft always mean that physical edge.
//
// Append-only: never reorder or insert; the zero value stays DrawerEnd.
type DrawerPlacement uint8

const (
	DrawerEnd DrawerPlacement = iota
	DrawerRight
	DrawerLeft
	DrawerTop
	DrawerBottom
	DrawerStart
)

// resolve maps a logical placement to the physical edge for the given
// direction.
func (p DrawerPlacement) resolve(rtl bool) DrawerPlacement {
	switch p {
	case DrawerEnd:
		if rtl {
			return DrawerLeft
		}
		return DrawerRight
	case DrawerStart:
		if rtl {
			return DrawerRight
		}
		return DrawerLeft
	}
	return p
}

type drawerCtx struct {
	childOpen bool
}

var drawerStackKey = hashID("drawer-stack")

// DrawerW is a panel sliding from a window edge: four placements, nested
// drawers push their parent aside, header extra, footer, sizes.
type DrawerW struct {
	_ [0]func()

	c         *Context
	open      *bool
	title     string
	place     DrawerPlacement
	size      float32
	footer    func()
	extra     func()
	closable  bool
	mask      bool
	maskClose bool
	closed    bool
	noHeader  bool

	resizable        bool
	minSize, maxSize float32
	bind             *float32
	resized          bool
	cur              float32 // size used this frame
}

// drawerResize is the retained state of a resizable drawer.
type drawerResize struct {
	size      float32 // user-chosen size (0: the configured Size)
	dragStart float32
	dragFrom  geom.Point
}

// Drawer starts a drawer shown while *open; chain options, then Body.
// A Drawer opened inside another one's Body pushes the parent aside.
func Drawer(c *Context, open *bool, title string) *DrawerW {
	d := &DrawerW{c: c, open: open, title: title, closable: true, mask: true, maskClose: true}
	d.defaults() // ConfigProvider.Defaults("Drawer", ...), see shad2c_defaults.go
	return d
}

// Placement selects the edge (DrawerEnd default: right, left in RTL).
func (d *DrawerW) Placement(p DrawerPlacement) *DrawerW { d.place = p; return d }

// Size sets the width (left/right) or height (top/bottom); default 378
// (Ant) or 30% of the window (Element).
func (d *DrawerW) Size(px float32) *DrawerW { d.size = px; return d }

// Large uses the large preset (736).
func (d *DrawerW) Large() *DrawerW { d.size = 736; return d }

func (d *DrawerW) Footer(fn func()) *DrawerW     { d.footer = fn; return d }
func (d *DrawerW) Extra(fn func()) *DrawerW      { d.extra = fn; return d }
func (d *DrawerW) Closable(on bool) *DrawerW     { d.closable = on; return d }
func (d *DrawerW) Mask(on bool) *DrawerW         { d.mask = on; return d }
func (d *DrawerW) MaskClosable(on bool) *DrawerW { d.maskClose = on; return d }
func (d *DrawerW) NoHeader() *DrawerW            { d.noHeader = true; return d }

// Closed reports close / mask / Escape this frame.
func (d *DrawerW) Closed() bool { return d.closed }

// Resizable lets the user drag the edge facing the page to change the
// width (left / right) or height (top / bottom). The edge is also a
// focusable separator: arrow keys step 10px (Shift: 50px), Home / End go
// to the limits. The chosen size is kept while the drawer exists; use
// BindSize to own it.
func (d *DrawerW) Resizable(on bool) *DrawerW { d.resizable = on; return d }

// SizeLimits bounds a resizable drawer (px). Zero min means 100; zero max
// means the window width / height.
func (d *DrawerW) SizeLimits(min, max float32) *DrawerW { d.minSize, d.maxSize = min, max; return d }

// BindSize binds the size to *px: a positive value overrides Size, and a
// resize writes the new size back.
func (d *DrawerW) BindSize(px *float32) *DrawerW { d.bind = px; return d }

// Resized reports a size change by the user this frame.
func (d *DrawerW) Resized() bool { return d.resized }

// CurrentSize returns the width / height used this frame (after Body).
func (d *DrawerW) CurrentSize() float32 { return d.cur }

// sizeLimits resolves SizeLimits against the window extent.
func (d *DrawerW) sizeLimits(horiz bool) (lo, hi float32) {
	ext := d.c.size.H
	if horiz {
		ext = d.c.size.W
	}
	lo, hi = d.minSize, d.maxSize
	if lo <= 0 {
		lo = 100
	}
	if hi <= 0 || hi > ext {
		hi = ext
	}
	return lo, max(lo, hi)
}

// Body builds the drawer content.
func (d *DrawerW) Body(fn func()) *DrawerW {
	c := d.c
	th := c.ThemeFor(theme.CompDrawer)
	tk := &th.Components.Drawer
	el := navEl(c)
	stack := State[[]*drawerCtx](c, drawerStackKey, nil)
	if len(*stack) > 0 && *d.open {
		(*stack)[len(*stack)-1].childOpen = true
	}
	id := hashID(c.scope, "drawer", d.title)
	a, live := drawerMotion(c, id, *d.open)
	if !live {
		return d
	}
	leaving := !*d.open
	place := d.place.resolve(c.IsRTL()) // physical edge
	horiz := place == DrawerLeft || place == DrawerRight
	size := d.size
	if size == 0 {
		size = 378
		if el {
			if horiz {
				size = c.size.W * 0.3
			} else {
				size = c.size.H * 0.3
			}
		}
	}
	var rs *drawerResize
	if d.resizable {
		rs = State[drawerResize](c, hashID(id, "resize"), nil)
		if d.bind != nil && *d.bind > 0 {
			size = *d.bind
		} else if rs.size > 0 {
			size = rs.size
		}
		lo, hi := d.sizeLimits(horiz)
		size = max(lo, min(size, hi))
	}
	if d.mask {
		mask := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {})
		mask.Bg(theme.Fade(tk.MaskBg, float64(a)))
		if d.maskClose && c.clicked[mask.n.id] > 0 {
			d.closed = true
		}
	}
	ctx := &drawerCtx{}
	var panel *node
	holder := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		p := Column(c, func() {
			if !d.noHeader {
				d.header()
			}
			*stack = append(*stack, ctx)
			body := Scroll(c, fn, layout.Grow(1), layout.Pad(tk.Padding))
			body.n.ln.Padding.Top = tk.BodyPaddingTop
			stack = State[[]*drawerCtx](c, drawerStackKey, nil)
			*stack = (*stack)[:len(*stack)-1]
			if d.footer != nil {
				f := Row(c, d.footer, layout.Gap(8), layout.End(), layout.Padding(geom.Insets{
					Top: tk.FooterPaddingTop, Right: tk.FooterPaddingInline, Bottom: tk.FooterPaddingBlock, Left: tk.FooterPaddingInline}))
				if fb := tk.FooterBorderColor; fb.A > 0 {
					f.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Y, r.W, fb) })
				}
			}
			if rs != nil && !leaving {
				// Built last: on top of the content for hits, last in Tab order.
				size = d.resizeHandle(rs, place, size, tk)
			}
		})
		p.Bg(tk.Bg).Shadow(tk.Shadow)
		if bc := tk.BorderColor; bc.A > 0 {
			// A 1px border on the edge facing the page (shadcn Sheet).
			p.PaintOver(func(l *paint.List, r geom.Rect) {
				switch place {
				case DrawerRight:
					navVLine(l, r.X, r.Y, r.H, bc)
				case DrawerLeft:
					navVLine(l, r.Right()-1, r.Y, r.H, bc)
				case DrawerTop:
					navHLine(l, r.X, r.Bottom()-1, r.W, bc)
				default:
					navHLine(l, r.X, r.Y, r.W, bc)
				}
			})
		}
		p.n.hit, p.n.blocks = true, true
		p.n.setRole(a11y.RoleDialog).setName(d.title).stateIf(d.mask, a11y.Modal)
		if c.a11yOn {
			p.n.fill(semFunc(dialogSem))
		}
		panel = p.n
	})
	if !leaving && d.mask {
		c.TrapFocus(holder.n.id)
	}
	holder.n.hit, holder.n.blocks = false, false
	d.cur = size
	push := c.Anim(id, "push", b2f(ctx.childOpen)*180, th.DurationSlow)
	ln := &panel.ln
	ln.Absolute = true
	// The panel is laid out at its open position; the slide (and the push
	// aside by a nested drawer) is a paint transform whose sign follows the
	// physical edge. Pos offsets are logical (mirrored in RTL), so a
	// physical edge swaps Left and Right there.
	shift := (1-a)*size - push // distance towards the panel's own edge
	rtl := c.IsRTL()
	var dx, dy float32
	switch place {
	case DrawerRight:
		ln.Width = size
		ln.Pos = flexbox.Pos{Left: layout.Unset, Top: 0, Right: 0, Bottom: 0}
		if rtl {
			ln.Pos.Left, ln.Pos.Right = 0, layout.Unset
		}
		dx = shift
	case DrawerLeft:
		ln.Width = size
		ln.Pos = flexbox.Pos{Left: 0, Top: 0, Right: layout.Unset, Bottom: 0}
		if rtl {
			ln.Pos.Left, ln.Pos.Right = layout.Unset, 0
		}
		dx = -shift
	case DrawerTop:
		ln.Height = size
		ln.Pos = flexbox.Pos{Left: 0, Top: 0, Right: 0, Bottom: layout.Unset}
		dy = -shift
	case DrawerBottom:
		ln.Height = size
		ln.Pos = flexbox.Pos{Left: 0, Top: layout.Unset, Right: 0, Bottom: 0}
		dy = shift
	}
	if dx != 0 || dy != 0 {
		m := geom.Translate(dx, dy)
		panel.xf = func(geom.Rect) geom.Affine { return m }
	}
	if leaving {
		d.closed = false // sliding out: the panel no longer acts
		return d
	}
	if c.EscapeAt(holder.n.id) {
		d.closed = true
	}
	if d.closed {
		*d.open = false
		c.invalid = true
	}
	return d
}

// resizeHandle builds the drag edge of a resizable drawer and returns the
// size after this frame's drag or key presses.
func (d *DrawerW) resizeHandle(rs *drawerResize, place DrawerPlacement, size float32, tk *theme.DrawerToken) float32 {
	c := d.c
	th := c.ThemeFor(theme.CompDrawer)
	horiz := place == DrawerLeft || place == DrawerRight
	lo, hi := d.sizeLimits(horiz)
	n := c.add("drawer-resize", "drawer-resize:"+d.title)
	n.ln.Absolute = true
	hw := tk.ResizeHandleSize
	u := layout.Unset
	// The handle straddles the edge facing the page. Positions are
	// physical (the panel itself is placed by physical edge).
	switch place {
	case DrawerRight:
		n.ln.Width, n.ln.Pos = hw, flexbox.Pos{Left: 0, Top: 0, Right: u, Bottom: 0}
	case DrawerLeft:
		n.ln.Width, n.ln.Pos = hw, flexbox.Pos{Left: u, Top: 0, Right: 0, Bottom: 0}
	case DrawerTop:
		n.ln.Height, n.ln.Pos = hw, flexbox.Pos{Left: 0, Top: u, Right: 0, Bottom: 0}
	default:
		n.ln.Height, n.ln.Pos = hw, flexbox.Pos{Left: 0, Top: 0, Right: 0, Bottom: u}
	}
	n.ln.NoMirror = true
	cur := input.CursorResizeEW
	if !horiz {
		cur = input.CursorResizeNS
	}
	n.ownKeys = true
	ia := c.interact(n, true, cur)
	n.setRole(a11y.RoleSeparator).setName(c.Locale().A11y.Resize)
	if horiz {
		n.setState(a11y.Vertical)
	} else {
		n.setState(a11y.Horizontal)
	}
	set := func(v float32) {
		v = max(lo, min(v, hi))
		if v != size {
			size = v
			rs.size = v
			if d.bind != nil {
				*d.bind = v
			}
			d.resized = true
			c.invalid = true
		}
	}
	if ia.Focused {
		target, moved := size, false
		if act, _, ok := c.A11yAction(n.id); ok && (act == a11y.Increment || act == a11y.Decrement) {
			c.KeysFor(n.id) // the synthetic arrow key of the action
			target, moved = size+10, true
			if act == a11y.Decrement {
				target = size - 10
			}
		} else if v, ok := c.takeSetNumber(n.id); ok {
			target, moved = float32(v), true
		}
		for _, k := range c.KeysFor(n.id) {
			if !k.Down {
				continue
			}
			step := float32(10)
			if k.Mods&input.ModShift != 0 {
				step = 50
			}
			// The edge follows the arrow: towards the page grows the drawer.
			grow := false
			shrink := false
			switch k.Key {
			case input.KeyLeft:
				grow, shrink = place == DrawerRight, place == DrawerLeft
			case input.KeyRight:
				grow, shrink = place == DrawerLeft, place == DrawerRight
			case input.KeyUp:
				grow, shrink = place == DrawerBottom, place == DrawerTop
			case input.KeyDown:
				grow, shrink = place == DrawerTop, place == DrawerBottom
			case input.KeyHome:
				target, moved = lo, true
			case input.KeyEnd:
				target, moved = hi, true
			}
			if grow {
				target, moved = target+step, true
			} else if shrink {
				target, moved = target-step, true
			}
		}
		if moved {
			set(target)
		}
	}
	if ia.PressedNow && rs.dragStart == 0 { // PressedNow holds for every build of the frame
		rs.dragStart, rs.dragFrom = size, c.pointer
	}
	if c.Dragging(n.id) && rs.dragStart > 0 {
		c.KeepPointer(n.id)
		dx, dy := c.pointer.X-rs.dragFrom.X, c.pointer.Y-rs.dragFrom.Y
		switch place {
		case DrawerRight:
			set(rs.dragStart - dx)
		case DrawerLeft:
			set(rs.dragStart + dx)
		case DrawerTop:
			set(rs.dragStart + dy)
		default:
			set(rs.dragStart - dy)
		}
	} else if !c.Dragging(n.id) {
		rs.dragStart = 0
	}
	if c.a11yOn {
		now := size
		n.fill(semFunc(func(s *a11y.Node) {
			rangeSem(s, float64(now), float64(lo), float64(hi), 10)
			s.Actions |= a11y.Increment | a11y.Decrement | a11y.SetValue
		}))
	}
	id := n.id
	n.paint = func(l *paint.List, r geom.Rect) {
		on := c.Anim(id, "hover", b2f(ia.Hovered || c.Dragging(id)), th.DurationMid)
		if on > 0.01 {
			w := tk.ResizeLineSize
			col := theme.Fade(tk.ResizeLineColor, float64(on))
			var lr geom.Rect
			switch place {
			case DrawerRight:
				lr = geom.Rect{X: r.X, Y: r.Y, W: w, H: r.H}
			case DrawerLeft:
				lr = geom.Rect{X: r.Right() - w, Y: r.Y, W: w, H: r.H}
			case DrawerTop:
				lr = geom.Rect{X: r.X, Y: r.Bottom() - w, W: r.W, H: w}
			default:
				lr = geom.Rect{X: r.X, Y: r.Y, W: r.W, H: w}
			}
			l.Fill(lr, geom.Rad(0), col)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(0))
		}
	}
	return size
}

func (d *DrawerW) header() {
	c := d.c
	th := c.ThemeFor(theme.CompDrawer)
	tk := &th.Components.Drawer
	el := navEl(c)
	shad := th.Flavor == theme.StyleShadcn
	close := func() {
		if !d.closable {
			return
		}
		ib := IconButton(c, icon.Close, c.Locale().Close)
		if shad {
			ib.NoHoverBg()
		}
		if tk.CloseIconColor.A > 0 {
			ib.Color(tk.CloseIconColor)
		}
		if ib.Clicked() {
			d.closed = true
		}
	}
	title := func() {
		t := Text(c, d.title).Size(tk.TitleFontSize).Color(tk.TitleColor).Layout(layout.Grow(1)).NoWrap().Ellipsis()
		if tk.TitleFontWeight != 0 {
			t.Weight(text.Weight(tk.TitleFontWeight))
		}
	}
	pad := layout.Padding(geom.Insets{Top: tk.HeaderPaddingTop, Right: tk.HeaderPaddingInline, Bottom: tk.HeaderPaddingBottom, Left: tk.HeaderPaddingInline})
	var hd *Box
	if el || shad {
		// Title first, close button at the end.
		hd = Row(c, func() {
			title()
			if d.extra != nil {
				Row(c, d.extra, layout.Gap(8), layout.Middle())
			}
			close()
		}, pad, layout.Gap(8), layout.Middle())
		if el {
			hd.n.ln.Margin.Bottom = 12
		}
	} else {
		// Ant: close button before the title.
		hd = Row(c, func() {
			close()
			title()
			if d.extra != nil {
				Row(c, d.extra, layout.Gap(8), layout.Middle())
			}
		}, pad, layout.Gap(12), layout.Middle())
	}
	if hb := tk.HeaderBorderColor; hb.A > 0 {
		hd.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Bottom()-1, r.W, hb) })
	}
}

// ---------------------------------------------------------------------------
// Popconfirm

type popState struct {
	open  bool
	panel ID
	ret   ID // focus when the bubble opened (restored when it closes)
}

// PopconfirmW asks for confirmation before an action. Options apply when
// the build finishes (or at Confirmed / Cancelled).
//
// ConfigProvider.Defaults("Popconfirm", ...) keys: "okText", "cancelText",
// "placement" (string), "showCancel" (bool).
type PopconfirmW struct {
	_ [0]func()

	c                    *Context
	b                    *Box
	title, desc          string
	okText, cancelText   string
	ic                   *icon.Icon
	icSet                bool
	icColor              theme.Color
	place                string
	disabled             bool
	okDanger             bool
	noCancel             bool
	z                    *lazy
	confirmed, cancelled bool
	isOpen               bool
}

// Popconfirm wraps trigger content; clicking it opens a confirm bubble.
func Popconfirm(c *Context, title, desc string, trigger func()) *PopconfirmW {
	b := c.box("popconfirm", layout.Row, nil)
	b.n.ln.NoStretch = true
	b.Body(trigger)
	p := &PopconfirmW{c: c, b: b, title: title, desc: desc,
		okText:     Prop(c, "Popconfirm", "okText", ""),
		cancelText: Prop(c, "Popconfirm", "cancelText", ""),
		place:      Prop(c, "Popconfirm", "placement", "top"),
		noCancel:   !Prop(c, "Popconfirm", "showCancel", true),
	}
	p.z = c.lazy(b.n, p.build)
	return p
}

// OkText sets the confirm button text (default Locale().OK).
func (p *PopconfirmW) OkText(s string) *PopconfirmW { p.okText = s; return p }

// CancelText sets the cancel button text (default Locale().Cancel).
func (p *PopconfirmW) CancelText(s string) *PopconfirmW { p.cancelText = s; return p }

// Description sets the text under the title.
func (p *PopconfirmW) Description(s string) *PopconfirmW { p.desc = s; return p }

// Icon replaces the warning icon (nil hides it).
func (p *PopconfirmW) Icon(ic *icon.Icon) *PopconfirmW { p.ic, p.icSet = ic, true; return p }

// IconColor colors a custom Icon (default the warning color).
func (p *PopconfirmW) IconColor(col theme.Color) *PopconfirmW { p.icColor = col; return p }

// Placement as Popover.Placement (default "top").
func (p *PopconfirmW) Placement(s string) *PopconfirmW { p.place = s; return p }

// Disabled keeps the bubble from opening.
func (p *PopconfirmW) Disabled(on bool) *PopconfirmW { p.disabled = on; return p }

// OkDanger styles the confirm button as dangerous (okType="danger").
func (p *PopconfirmW) OkDanger() *PopconfirmW { p.okDanger = true; return p }

// ShowCancel shows or hides the cancel button (default shown).
func (p *PopconfirmW) ShowCancel(on bool) *PopconfirmW { p.noCancel = !on; return p }

// IsOpen reports whether the bubble is shown.
func (p *PopconfirmW) IsOpen() bool { p.z.run(); return p.isOpen }

func (p *PopconfirmW) build() {
	c := p.c
	th := c.ThemeFor(theme.CompPopover)
	tk := &th.Components.Popover
	b := p.b
	st := State[popState](c, b.n.id, nil)
	opened := false
	disabled := p.disabled || b.n.disabled
	if !disabled && (c.released[b.n.id] || c.clickedWithin(b.n) || c.keyActivatedWithin(b.n)) {
		st.open = !st.open
		c.invalid = true
		if st.open {
			opened, st.ret = true, c.focus
		} else {
			popRestore(c, st)
		}
	}
	if st.open && (disabled || c.Dismissed(st.panel, b.n.id)) {
		st.open = false
		popRestore(c, st)
	}
	p.isOpen = st.open
	if !st.open {
		return
	}
	loc := c.Locale()
	okText, cancelText := p.okText, p.cancelText
	if okText == "" {
		okText = loc.OK
	}
	if cancelText == "" {
		cancelText = loc.Cancel
	}
	title, desc := p.title, p.desc
	a := c.AnimFrom(b.n.id, "open", 0, 1, th.DurationMid)
	shad := th.Flavor == theme.StyleShadcn
	place, _ := c.popPlacement(p.place)
	ov := Overlay(c, OverlayOptions{Anchor: b.n.id, Place: place, Offset: map[bool]float32{true: 4, false: 10}[shad], Flip: true}, func() {
		Row(c, func() {
			switch {
			case !p.icSet:
				statusIcon(c, "warning", 14).ln.Margin.Top = 4
			case p.ic != nil:
				col := p.icColor
				if col.A == 0 {
					col = th.Warning.Base
				}
				Icon(c, p.ic).Size(14).Color(col).n.ln.Margin.Top = 4
			}
			Column(c, func() {
				Text(c, title).Strong()
				if desc != "" {
					Text(c, desc)
				}
			}, layout.Gap(4))
		}, layout.Gap(8), layout.AlignItems(layout.AlignStart), layout.Margin(geom.Insets{Bottom: 8}))
		Row(c, func() {
			if !p.noCancel && Button(c, cancelText).Small().Clicked() {
				p.cancelled = true
				st.open = false
				popRestore(c, st)
			}
			ok := Button(c, okText).Small().Primary()
			if p.okDanger {
				ok.Danger()
			}
			if opened {
				// Keyboard focus moves into the bubble (Tab reaches Cancel).
				if c.focusVisible {
					navFocus(c, ok.n.id)
				} else {
					c.SetFocus(ok.n.id)
				}
			}
			if ok.Clicked() {
				p.confirmed = true
				st.open = false
				popRestore(c, st)
			}
		}, layout.Gap(8), layout.End())
	}, layout.Pad(tk.InnerPadding))
	ov.n.setRole(a11y.RoleAlertDialog).setName(title)
	if c.a11yOn {
		ov.n.fill(semFunc(func(s *a11y.Node) {
			s.Description = desc
			dialogSem(s)
		}))
	}
	ov.Bg(tk.Bg).Radius(th.RadiusLG).Shadow(tk.Shadow).Opacity(a)
	st.panel = ov.n.id
	if shad {
		// shadcn Popover: bordered rounded-md panel, p-4, no arrow.
		ov.Radius(tk.Radius)
		if tk.BorderColor.A > 0 {
			ov.Border(1, tk.BorderColor)
		}
		return
	}
	ov.n.over = func(l *paint.List, r geom.Rect) {
		ar, ok := c.anchorRect(b.n.id)
		if !ok {
			return
		}
		if r.Bottom() > ar.Y+1 && r.Y < ar.Bottom()-1 {
			popDrawArrow(l, r, ar, tk.Bg, theme.Color{}) // left / right
			return
		}
		cx := max(r.X+12+7, min(ar.X+ar.W/2, r.Right()-12-7))
		p := paint.NewPath()
		if r.Bottom() <= ar.Y+1 {
			p.MoveTo(cx-7, r.Bottom()-0.5).LineTo(cx, r.Bottom()+6).LineTo(cx+7, r.Bottom()-0.5).Close()
		} else {
			p.MoveTo(cx-7, r.Y+0.5).LineTo(cx, r.Y-6).LineTo(cx+7, r.Y+0.5).Close()
		}
		l.Path(p, tk.Bg)
	}
}

// popRestore gives keyboard focus back to the trigger when the bubble
// closes while focus is inside it.
func popRestore(c *Context, st *popState) {
	if st.ret != 0 && c.focusLayer(st.ret) >= 0 {
		c.SetFocus(st.ret)
	}
	st.ret = 0
}

// Confirmed reports the OK button.
func (p *PopconfirmW) Confirmed() bool { p.z.run(); return p.confirmed }

// Cancelled reports the cancel button.
func (p *PopconfirmW) Cancelled() bool { p.z.run(); return p.cancelled }
