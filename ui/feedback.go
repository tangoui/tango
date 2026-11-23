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

// spinTokens returns the Spin token of the current scope.
func spinTokens(c *Context) *theme.SpinToken {
	return &c.ThemeFor(theme.CompSpin).Components.Spin
}

// drawSpinner paints the theme's loading indicator (SpinToken.Indicator)
// in the Spin color.
func drawSpinner(c *Context, l *paint.List, r geom.Rect) {
	drawSpinIndicator(c, l, r, -1, spinTokens(c).Color)
}

// drawSpinDots paints the Ant four-dot indicator (antd es/spin/style
// genIndicatorStyle): four items of (size − marginXXS/2) / 2 in the
// corners, scaled to 0.75, fading from 0.3 to 1.
func drawSpinDots(c *Context, l *paint.List, r geom.Rect, col theme.Color) {
	ang := spinAngle(c) * 1000 / 1200
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	item := max(0, r.W-2) / 2
	rad := item / 2 * 0.75
	off := (r.W/2 - item/2) * math.Sqrt2 // item centers sit in the corners
	for i := 0; i < 4; i++ {
		a := ang + float64(i)*math.Pi/2 + math.Pi/4
		x := cx + float32(math.Cos(a))*off
		y := cy + float32(math.Sin(a))*off
		op := 0.3 + 0.7*float64(i)/3
		l.Path(paint.NewPath().Circle(x, y, rad), theme.Fade(col, op))
	}
}

// drawSpinIndicator paints the theme's indicator, or a progress ring when
// percent > 0 (antd shows the dots until the percent leaves 0).
func drawSpinIndicator(c *Context, l *paint.List, r geom.Rect, percent float64, col theme.Color) {
	th := c.Theme
	tk := spinTokens(c)
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if percent > 0 {
		// antd Indicator/Progress.js: stroke viewSize / 5 on a
		// colorFillSecondary track, starting at the top.
		w := max(2, r.W*float32(tk.ProgressStrokeRatio))
		rad := r.W/2 - w/2
		l.StrokePath(paint.NewPath().Arc(cx, cy, rad, 0, 2*math.Pi), paint.StrokeStyle{Width: w}, th.FillSecondary)
		l.StrokePath(paint.NewPath().Arc(cx, cy, rad, -math.Pi/2, 2*math.Pi*min(percent, 100)/100), paint.StrokeStyle{Width: w, Cap: paint.CapRound}, col)
		return
	}
	switch tk.Indicator {
	case theme.SpinCircular:
		// EP loading.scss: r=20 circle in a 50 viewBox, stroke-width 2,
		// dash 90/150 rotating.
		w := max(1, r.W*2/50)
		l.StrokePath(paint.NewPath().Arc(cx, cy, r.W*20/50, spinAngle(c)*0.8, 1.4*math.Pi), paint.StrokeStyle{Width: w, Cap: paint.CapRound}, col)
	case theme.SpinIcon:
		c.paintIcon(l, icon.Loading, r, col, spinAngle(c)) // shadcn Spinner: lucide Loader2
	default:
		drawSpinDots(c, l, r, col)
	}
}

type spinState struct {
	since time.Time
}

// throttleState is the state of throttledOn.
type throttleState struct {
	inited bool
	last   bool // latest requested value
	val    bool // value shown
	pend   bool
	at     time.Time
}

// throttledOn delays a boolean the way Element's useThrottleRender does
// (and antd Spin delay): after on turns true it reports true only once
// leading has passed, after it turns false only once trailing has passed
// (trailing <= 0: at once). The first frame starts from initVal and
// takes on after the leading delay whatever on is (EP onMounted
// dispatches "leading"). Delays are skipped when animations are off
// (screenshots), unless the Driver runs with animations.
func throttledOn(c *Context, key ID, on bool, leading, trailing time.Duration, initVal bool) bool {
	st := State[throttleState](c, key, nil)
	schedule := func(d time.Duration) {
		if d <= 0 || c.noAnim {
			st.val, st.pend = st.last, false
			return
		}
		st.pend, st.at = true, c.now.Add(d)
	}
	switch {
	case !st.inited:
		st.inited, st.val, st.last = true, initVal, on
		schedule(leading)
	case st.last != on:
		st.last = on
		if on {
			schedule(leading)
		} else {
			schedule(trailing)
		}
	}
	if st.pend {
		if !c.now.Before(st.at) {
			st.val, st.pend = st.last, false
		} else {
			c.WakeAt(st.at)
		}
	}
	return st.val
}

// SpinW is a loading indicator: sizes, description, delayed show,
// progress percent and a nested mode (Wrap) that dims the wrapped content.
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
	auto     bool       // PercentAuto
	ic       *icon.Icon // Indicator
	full     bool       // fullscreen indicator size
}

// Spin adds a loading indicator; chain options. Wrap turns it into a
// container that covers its content while spinning:
//
//	ui.Spin(c).Size(ui.Large).Description("加载中")
//	ui.Spin(c).Spinning(loading).Wrap(func() { ... })
//
// ConfigProvider.Defaults("Spin", ...) keys: "size", "delay",
// "description" ("tip" before antd 6).
func Spin(c *Context) *SpinW {
	s := &SpinW{c: c, spinning: true, percent: -1}
	s.defaults() // ConfigProvider.Defaults("Spin", ...): size, delay, tip
	if d, ok := p2bStr(c, "Spin", "description"); ok {
		s.tip = d
	}
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
			s.paint(l, r, spinTokens(c).Color)
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

// Description shows a text under the indicator (antd 6 description).
func (s *SpinW) Description(t string) *SpinW { s.tip = t; return s }

// Tip shows a text under the indicator.
//
// Deprecated: use Description (antd 6 renamed tip to description).
func (s *SpinW) Tip(t string) *SpinW { s.tip = t; return s }

// Indicator replaces the indicator with a spinning icon (antd indicator,
// EP spinner).
func (s *SpinW) Indicator(ic *icon.Icon) *SpinW { s.ic = ic; return s }

// Delay shows the indicator only after spinning lasted d (avoids flashes);
// it hides at once when spinning stops (antd delay).
func (s *SpinW) Delay(d time.Duration) *SpinW { s.delay = d; return s }

// Spinning toggles the indicator (default on).
func (s *SpinW) Spinning(on bool) *SpinW { s.spinning = on; return s }

// Percent replaces the indicator with a progress ring (0..100); at 0 the
// indicator still spins (antd).
func (s *SpinW) Percent(p float64) *SpinW { s.percent, s.auto = p, false; return s }

// PercentAuto shows a simulated progress that slows down as it nears 100
// while spinning (antd percent="auto": every 200ms it advances 5% of the
// rest up to 30, 3% up to 70, 1% up to 96).
func (s *SpinW) PercentAuto() *SpinW { s.auto, s.percent = true, 0; return s }

func (s *SpinW) px() float32 {
	if s.custom > 0 {
		return s.custom
	}
	tk := spinTokens(s.c)
	sz := s.size
	if !s.sizeSet {
		if s.full && tk.FullscreenDotSize > 0 {
			return tk.FullscreenDotSize
		}
		sz, _ = s.c.ComponentSize()
	}
	switch sz {
	case Small:
		return tk.DotSizeSM
	case Large:
		return tk.DotSizeLG
	}
	return tk.DotSize
}

func (s *SpinW) visible(id ID) bool {
	c := s.c
	st := State[spinState](c, hashID(id, "spin"), nil)
	if !s.spinning {
		st.since = time.Time{}
	} else if st.since.IsZero() {
		st.since = c.now
	}
	on := throttledOn(c, hashID(id, "spin-delay"), s.spinning, s.delay, 0, false)
	if on && s.auto {
		s.percent = spinAutoPercent(c, st.since)
	}
	return on
}

// spinAutoPercent is the simulated progress of PercentAuto after spinning
// since start (antd spin/usePercent.js).
func spinAutoPercent(c *Context, start time.Time) float64 {
	const step = 200 * time.Millisecond
	if c.noAnim {
		return 0 // deterministic screenshots; reduced motion still advances (it is information)
	}
	n := int(c.now.Sub(start) / step)
	p := 0.0
	for i := 0; i < n && i < 2000; i++ {
		rest := 100 - p
		switch {
		case p <= 30:
			p += rest * 0.05
		case p <= 70:
			p += rest * 0.03
		case p <= 96:
			p += rest * 0.01
		default:
			return p
		}
	}
	c.WakeAt(start.Add(time.Duration(n+1) * step))
	return p
}

func (s *SpinW) tipStyle() text.Style {
	return s.c.style(spinTokens(s.c).DescriptionFontSize, text.Regular)
}

func (s *SpinW) measure() geom.Size {
	px := s.px()
	if s.tip != "" {
		st := s.tipStyle()
		return geom.Size{W: max(px, text.Width(s.tip, st)), H: px + spinTokens(s.c).DescriptionGap + st.LineHeight}
	}
	return geom.Size{W: px, H: px}
}

func (s *SpinW) paint(l *paint.List, r geom.Rect, col theme.Color) {
	px := s.px()
	ir := geom.Rect{X: r.X + (r.W-px)/2, Y: r.Y, W: px, H: px}
	if s.ic != nil && s.percent <= 0 {
		s.c.paintIcon(l, s.ic, ir, col, spinAngle(s.c))
	} else {
		drawSpinIndicator(s.c, l, ir, s.percent, col)
	}
	if s.tip != "" {
		lay := text.Shape(s.tip, s.tipStyle(), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, ir.Bottom()+spinTokens(s.c).DescriptionGap), lay.Glyphs, col)
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

// indicator adds a separate indicator leaf in col (used inside masks).
func (s *SpinW) indicator(col theme.Color) *node {
	n := s.c.add("spin-indicator", s.tip)
	s.annotate(n)
	n.ln.NoStretch = true
	n.ln.Measure = func(_, _ float32) geom.Size { return s.measure() }
	n.paint = func(l *paint.List, r geom.Rect) { s.paint(l, r, col) }
	return n
}

// Wrap builds content and, while spinning, dims it and centers the
// indicator above it (Ant nested Spin).
func (s *SpinW) Wrap(fn func()) *Box {
	c := s.c
	th := c.Theme
	tk := spinTokens(c)
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
		if a > 0 && tk.ContainerOpacity < 1 {
			content.Opacity(1 - float32(1-tk.ContainerOpacity)*a)
		}
		if on || a > 0 {
			mask := Column(c, func() { s.indicator(tk.Color) }, layout.Center())
			mask.Bg(tk.MaskBg).Opacity(a)
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
	ic      *icon.Icon
}

// Loading starts a loading mask shown while on; chain options, then Body
// (cover a region) or Fullscreen (cover the window):
//
//	ui.Loading(c, busy).Text("加载中").Body(func() { ... })
//	ui.Loading(c, busy).Fullscreen()
//
// ConfigProvider.Defaults("Spin", ...) "size" and "tip" apply.
func Loading(c *Context, on bool) *LoadingW {
	m := &LoadingW{c: c, on: on}
	m.configDefaults()
	if d, ok := p2bStr(c, "Spin", "description"); ok {
		m.tip = d
	}
	return m
}

// Text shows a caption under the spinner (EP element-loading-text, antd
// Spin description).
func (m *LoadingW) Text(s string) *LoadingW { m.tip = s; return m }

// Tip shows a caption under the spinner.
//
// Deprecated: use Text.
func (m *LoadingW) Tip(s string) *LoadingW { m.tip = s; return m }

// Background overrides the mask color.
func (m *LoadingW) Background(col theme.Color) *LoadingW { m.bg, m.bgSet = col, true; return m }

// Size sets the spinner size.
func (m *LoadingW) Size(s ControlSize) *LoadingW { m.size, m.sizeSet = s, true; return m }

// Spinner replaces the indicator with a spinning icon (EP
// element-loading-spinner, antd indicator).
func (m *LoadingW) Spinner(ic *icon.Icon) *LoadingW { m.ic = ic; return m }

// look returns the mask color and the indicator color.
func (m *LoadingW) look() (bg, fg theme.Color) {
	tk := spinTokens(m.c)
	bg, fg = tk.MaskBg, tk.Color
	if m.full {
		bg, fg = tk.FullscreenMaskBg, tk.FullscreenColor
	}
	if m.bgSet {
		bg = m.bg
	}
	return bg, fg
}

func (m *LoadingW) spin() *SpinW {
	return &SpinW{c: m.c, size: m.size, sizeSet: m.sizeSet, tip: m.tip, spinning: true, percent: -1, ic: m.ic, full: m.full}
}

// Body builds the region and covers it while loading.
func (m *LoadingW) Body(fn func()) *Box {
	c, th := m.c, m.c.Theme
	tk := spinTokens(c)
	b := c.box("loading", layout.Stack, nil)
	if m.on {
		b.n.setRole(a11y.RoleGroup).setState(a11y.Busy)
	}
	a := c.Anim(b.n.id, "on", b2f(m.on), th.DurationMid)
	b.Body(func() {
		content := Column(c, fn)
		if a > 0.01 && tk.ContainerOpacity < 1 {
			content.Opacity(1 - float32(1-tk.ContainerOpacity)*a)
		}
		if a > 0.01 {
			bg, fg := m.look()
			mask := Column(c, func() { m.spin().indicator(fg) }, layout.Center())
			mask.Bg(bg).Opacity(a)
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
	bg, fg := m.look()
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() { m.spin().indicator(fg) }, layout.Center())
	ov.Bg(bg).Opacity(a)
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

	iconSet   bool // ShowIcon / NoIcon / Defaults "showIcon" decided
	variant   string
	ic        *icon.Icon
	closeIc   *icon.Icon
	closeText string
	closeOff  bool
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
// warning, error. title is antd's title (v6; message before).
//
// As in antd and Element Plus the icon is off unless ShowIcon(true) (or
// Banner, which shows it by default).
//
// ConfigProvider.Defaults("Alert", ...) keys: "showIcon", "closable",
// "banner" (bool), "variant" (string: "outlined" | "filled").
//
// Under shadcn/ui it is the bordered Alert card with an outlined
// (lucide-style) icon; "error" is the destructive variant.
func Alert(c *Context, status, title string) *AlertW {
	th := c.ThemeFor(theme.CompAlert)
	_, iconSet := c.ConfigProp("Alert", "showIcon")
	return &AlertW{c: c, status: status, title: title, showIcon: Prop(c, "Alert", "showIcon", false), iconSet: iconSet,
		closable: Prop(c, "Alert", "closable", false), banner: Prop(c, "Alert", "banner", false),
		variant: Prop(c, "Alert", "variant", "outlined"), th: th, tk: &th.Components.Alert}
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

// NoIcon hides the status icon (same as ShowIcon(false)).
func (a *AlertW) NoIcon() *AlertW { a.showIcon, a.iconSet = false, true; return a }

// Closable adds the close button.
func (a *AlertW) Closable() *AlertW { a.closable = true; return a }

// Banner styles the alert as a page-top banner: no border or radius, and
// the icon on unless ShowIcon(false) (antd banner).
func (a *AlertW) Banner() *AlertW { a.banner = true; return a }

// ShowIcon shows or hides the status icon (antd showIcon, EP show-icon;
// default off, on for a Banner).
func (a *AlertW) ShowIcon(on bool) *AlertW { a.showIcon, a.iconSet = on, true; return a }

// Icon replaces the status icon (antd icon) and shows it.
func (a *AlertW) Icon(ic *icon.Icon) *AlertW { a.ic, a.showIcon, a.iconSet = ic, true, true; return a }

// Variant selects "outlined" (default, status border) or "filled" (no
// border) (antd 6.4 variant). Element and shadcn alerts are drawn
// without / with their own border either way.
func (a *AlertW) Variant(v string) *AlertW { a.variant = v; return a }

// CloseIcon replaces the close icon and makes the alert closable (antd
// closable.closeIcon).
func (a *AlertW) CloseIcon(ic *icon.Icon) *AlertW { a.closeIc, a.closable = ic, true; return a }

// CloseText shows text instead of the close icon and makes the alert
// closable (EP close-text).
func (a *AlertW) CloseText(s string) *AlertW { a.closeText, a.closable = s, true; return a }

// CloseDisabled disables the close button (antd closable.disabled);
// ConfigProvider.ComponentDisabled disables it too.
func (a *AlertW) CloseDisabled(on bool) *AlertW { a.closeOff = on; return a }

// iconShown reports whether the status icon is drawn: explicit settings
// win, a banner shows it by default.
func (a *AlertW) iconShown() bool {
	if a.iconSet {
		return a.showIcon
	}
	return a.showIcon || a.banner
}

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

// closeButton adds the close icon (or CloseText) button, in fg under the
// dark effect.
func (a *AlertW) closeButton(lk alertLook) {
	if !a.closable {
		return
	}
	c, tk := a.c, a.tk
	fg, dark := a.darkText(lk)
	Disabled(c, a.closeOff, func() {
		if a.closeText != "" {
			var t *Label
			b := Row(c, func() { t = Text(c, a.closeText).Size(tk.CloseFontSize).NoWrap() }, layout.Margin(geom.Insets{Left: 8}))
			b.Semantics(a11y.RoleButton, a.closeText)
			ia := b.Focusable(input.CursorPointer)
			col := tk.CloseColor
			switch {
			case b.n.disabled:
				col = a.th.TextDisabled
			case dark:
				col = fg
			case ia.Hovered:
				col = tk.CloseHoverColor
			}
			t.Color(col)
			if ia.FocusVisible {
				th := a.th
				b.PaintOver(func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(th.RadiusSM)) })
			}
			if ia.Clicked {
				a.closed = true
			}
			return
		}
		ic := a.closeIc
		if ic == nil {
			ic = icon.Close
		}
		ib := IconButton(c, ic, c.Locale().Close).Size(max(16, tk.CloseIconSize), tk.CloseIconSize).NoHoverBg()
		ib.n.ln.Margin.Left = 8
		col := tk.CloseColor
		if ib.Hovered() {
			col = tk.CloseHoverColor
		}
		if dark {
			col = fg
		}
		if ib.n.disabled {
			col = a.th.TextDisabled
		}
		ib.Color(col)
		if ib.Clicked() {
			a.closed = true
		}
	})
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
	lk := a.frame()
	if th.Flavor == theme.StyleShadcn {
		return a.showShadcn(lk)
	}
	big := a.desc != ""
	fg, dark := a.darkText(lk)
	pad := geom.VH(tk.DefaultPaddingBlock, tk.DefaultPaddingInline)
	if big {
		pad = geom.VH(tk.WithDescriptionPaddingBlock, tk.WithDescriptionPaddingInline)
	}
	b := Row(c, func() {
		a.leadSpacer()
		if a.iconShown() {
			is, mr := tk.IconSize, tk.IconMarginInlineEnd
			if big {
				is, mr = tk.WithDescriptionIconSize, tk.WithDescriptionIconMarginEnd
			}
			glyph := th.BgContainer
			if dark {
				glyph = lk.bg
			}
			var n *node
			if a.ic != nil {
				n = alertCustomIcon(c, a.ic, is, lk.icon)
			} else {
				n = alertIcon(c, a.status, is, lk.icon, glyph)
			}
			n.ln.Margin.Right = mr
			if !big || tk.WithDescriptionIconCenter {
				n.ln.AlignSelf = layout.AlignCenter
			}
		}
		Column(c, func() {
			t := Text(c, a.title).Align(a.textAlign())
			if big {
				t.Size(tk.TitleFontSize).Weight(text.Weight(tk.TitleFontWeight))
				t.Layout(layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
				d := Text(c, a.desc).Size(tk.DescriptionFontSize).Align(a.textAlign())
				switch {
				case dark:
					d.Color(fg)
				case tk.DescriptionColor.A > 0:
					d.Color(tk.DescriptionColor)
				default:
					d.Color(lk.icon) // Element: the status color
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
	switch {
	case a.banner:
		b.Radius(0) // antd banner: border 0, radius 0
	case lk.border.A > 0:
		b.Border(1, lk.border)
	}
	return a
}

// frame is the look with the variant applied: "filled" drops the border
// (antd .ant-alert-filled: border-color transparent).
func (a *AlertW) frame() alertLook {
	lk := a.look()
	if a.variant == "filled" {
		lk.border = theme.Color{}
	}
	return lk
}

// alertCustomIcon adds the Icon replacement of size px in col.
func alertCustomIcon(c *Context, ic *icon.Icon, size float32, col theme.Color) *node {
	n := c.add("alert-icon", "")
	n.aHidden = true
	n.ln.Width, n.ln.Height = size, size
	n.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, ic, r, col, 0) }
	return n
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
// the destructive (error) variant colors text and icon red.
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
		if a.iconShown() {
			is, mr := tk.IconSize, tk.IconMarginInlineEnd
			if big {
				is, mr = tk.WithDescriptionIconSize, tk.WithDescriptionIconMarginEnd
			}
			var n *node
			if a.ic != nil {
				n = alertCustomIcon(c, a.ic, is, lk.icon)
			} else {
				n = shad2cStatusNodeColor(c, a.status, is, lk.icon) // lucide-style outline glyph
			}
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
	shown    bool // the open transition finished (AfterOpenChanged)
	opened   bool // shown since the last close (AfterOpenChanged on close)
}

// ModalW is a dialog over a mask: centered or top placement, draggable,
// fullscreen, custom footer and close options.
type ModalW struct {
	_ [0]func()

	c          *Context
	open       *bool
	title      string
	header     func()
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
	keyboard   bool
	skeleton   bool
	settled    bool
}

// Modal starts a dialog shown while *open; chain options, then Body.
//
// The default width, top and centering come from theme.ModalToken (Ant
// 520 / 100, Element 50% / 15%, shadcn 512 centered). Besides the keys
// read in shad2c_defaults.go, ConfigProvider.Defaults("Modal", ...) reads
// "keyboard" (bool).
func Modal(c *Context, open *bool, title string) *ModalW {
	tk := &c.ThemeFor(theme.CompModal).Components.Modal
	w := tk.Width
	if tk.WidthRatio > 0 {
		w = max(w, c.size.W*float32(tk.WidthRatio))
	}
	m := &ModalW{c: c, open: open, title: title, width: w, maskClose: true, closable: true, mask: true,
		keyboard: Prop(c, "Modal", "keyboard", true)}
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

// Header replaces the visible title with custom content. The title passed
// to Modal remains the accessible dialog name. The built-in fullscreen
// and close buttons, drag handle and focus management remain available.
func (m *ModalW) Header(fn func()) *ModalW { m.header = fn; return m }

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

// Keyboard sets whether Escape closes the dialog (default true; antd
// keyboard, Element close-on-press-escape).
func (m *ModalW) Keyboard(on bool) *ModalW { m.keyboard = on; return m }

// Loading shows an active Skeleton instead of the content and hides the
// footer (antd loading).
func (m *ModalW) Loading(on bool) *ModalW { m.skeleton = on; return m }

// AfterOpenChanged reports, after Body, that the open or close transition
// finished this frame (antd afterOpenChange; read *open for the new
// state, false is antd afterClose).
func (m *ModalW) AfterOpenChanged() bool { return m.settled }

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
	st := State[modalState](c, id, nil)
	if !live {
		if st.shown || st.opened {
			// Also after a close during the open transition (antd
			// afterClose fires on every close).
			st.shown, st.opened, m.settled = false, false, true
		}
		return m
	}
	if *m.open {
		st.opened = true
		if mo.a >= 1 && !st.shown {
			st.shown, m.settled = true, true
		}
	}
	leaving := mo.leaving
	loc := c.Locale()
	if m.okText == "" {
		m.okText = loc.OK
	}
	if m.cancelText == "" {
		m.cancelText = loc.Cancel
	}
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
		top = tk.Top
		if tk.TopRatio > 0 {
			top = c.size.H * float32(tk.TopRatio)
		}
	}
	place := ScreenTopCenter
	if m.centered || (tk.Centered && m.top == 0) {
		place = ScreenCenter // shadcn dialogs are always centered
	}
	var header *node
	// antd max-width calc(100vw - 2 * margin), shadcn
	// max-w-[calc(100%-2rem)].
	width := min(m.width, max(0, c.size.W-32))
	if th.UsesElementPlusControls() {
		width = min(m.width, c.size.W)
	}
	if full {
		place, top, width = ScreenFill, 0, c.size.W
	}
	// The close button sits over the header's inline end; the header
	// keeps clear of it.
	closeReserve := float32(0)
	if m.closable {
		closeReserve = max(0, tk.CloseInsetEnd+tk.CloseBtnSize-tk.Padding)
	}
	dlg := Overlay(c, mo.place(OverlayOptions{Place: place, Margin: top, ShiftX: st.off.X, ShiftY: st.off.Y, exactTop: th.UsesElementPlusControls()}), func() {
		pad := geom.VH(tk.PaddingBlock, tk.Padding)
		col := Column(c, func() {
			hb := Row(c, func() {
				if m.header != nil {
					Row(c, m.header, layout.Grow(1), layout.Middle())
				} else {
					t := Text(c, m.title).Layout(layout.Grow(1)).NoWrap().Ellipsis().Size(tk.TitleFontSize).Color(tk.TitleColor)
					if tk.TitleLineHeight > 0 {
						t.LineHeight(tk.TitleLineHeight)
					}
					if tk.TitleFontWeight != 0 {
						t.Weight(text.Weight(tk.TitleFontWeight))
					}
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
			}, layout.Middle(), layout.Gap(4))
			header = hb.n
			if tk.HeaderBg.A > 0 {
				hb.Bg(tk.HeaderBg)
			}
			hb.n.ln.Margin.Bottom = tk.HeaderMarginBottom
			hb.n.ln.Margin.Right = closeReserve // inline end (mirrored in RTL)
			if m.closable && modalCloseButton(c, tk) {
				// First in Tab order, as in the header before.
				m.cancel = true
			}
			body := Column(c, func() {
				if m.skeleton {
					Skeleton(c).Active().Show()
					return
				}
				fn()
			})
			if full {
				body.n.ln.Grow = 1
			}
			if !m.noFooter && !m.skeleton {
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
	if c.EscapeAt(dlg.n.id) && m.keyboard {
		m.cancel = true
	}
	if m.cancel {
		*m.open = false
		c.invalid = true
	}
	return m
}

// modalCloseButton builds the close button of a Modal, absolutely placed
// CloseInsetTop / CloseInsetEnd from the panel's corner, and reports a
// click.
func modalCloseButton(c *Context, tk *theme.ModalToken) bool {
	ib := IconButton(c, icon.Close, c.Locale().Close).Size(tk.CloseBtnSize, tk.CloseIconSize)
	if tk.CloseNoHoverBg {
		ib.NoHoverBg() // Element headerbtn / shadcn: transparent
	}
	switch {
	case tk.CloseHoverColor.A > 0 && (ib.Hovered() || ib.ia.FocusVisible):
		ib.Color(tk.CloseHoverColor) // Element primary, shadcn opacity-100
	case tk.CloseIconColor.A > 0:
		ib.Color(tk.CloseIconColor)
	}
	ln := &ib.n.ln
	ln.Absolute = true
	// Pos is relative to the panel column's outer box; Right is the
	// inline end (mirrored in RTL).
	ln.Pos = flexbox.Pos{Left: layout.Unset, Top: tk.CloseInsetTop, Right: tk.CloseInsetEnd, Bottom: layout.Unset}
	return ib.Clicked()
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

	closePlace string // "start" / "end"; "": the theme's (DrawerToken.CloseAtEnd)
	keyboard   bool
	skeleton   bool
	push       float32
	settled    bool
}

// drawerResize is the retained state of a resizable drawer.
type drawerResize struct {
	size      float32 // user-chosen size (0: the configured Size)
	dragStart float32
	dragFrom  geom.Point
}

// Drawer starts a drawer shown while *open; chain options, then Body.
// A Drawer opened inside another one's Body pushes the parent aside.
//
// Besides the keys read in shad2c_defaults.go, ConfigProvider.Defaults(
// "Drawer", ...) reads "keyboard" (bool) and "closablePlacement"
// ("start" / "end").
func Drawer(c *Context, open *bool, title string) *DrawerW {
	d := &DrawerW{c: c, open: open, title: title, closable: true, mask: true, maskClose: true, push: 180,
		keyboard:   Prop(c, "Drawer", "keyboard", true),
		closePlace: Prop(c, "Drawer", "closablePlacement", "")}
	d.defaults() // ConfigProvider.Defaults("Drawer", ...), see shad2c_defaults.go
	return d
}

// Placement selects the edge (DrawerEnd default: right, left in RTL).
func (d *DrawerW) Placement(p DrawerPlacement) *DrawerW { d.place = p; return d }

// Size sets the width (left/right) or height (top/bottom); default from
// theme.DrawerToken: 378 (Ant), 30% of the window (Element), 384 but at
// most 75% (shadcn Sheet).
func (d *DrawerW) Size(px float32) *DrawerW { d.size = px; return d }

// Large uses the large preset (736).
func (d *DrawerW) Large() *DrawerW { d.size = 736; return d }

func (d *DrawerW) Footer(fn func()) *DrawerW     { d.footer = fn; return d }
func (d *DrawerW) Extra(fn func()) *DrawerW      { d.extra = fn; return d }
func (d *DrawerW) Closable(on bool) *DrawerW     { d.closable = on; return d }
func (d *DrawerW) Mask(on bool) *DrawerW         { d.mask = on; return d }
func (d *DrawerW) MaskClosable(on bool) *DrawerW { d.maskClose = on; return d }
func (d *DrawerW) NoHeader() *DrawerW            { d.noHeader = true; return d }

// ClosablePlacement puts the close button "start" (before the title) or
// "end" (after the title and extra); default from the theme (Ant start,
// Element and shadcn end; antd closable.placement).
func (d *DrawerW) ClosablePlacement(p string) *DrawerW { d.closePlace = p; return d }

// Keyboard sets whether Escape closes the drawer (default true; antd
// keyboard, Element close-on-press-escape).
func (d *DrawerW) Keyboard(on bool) *DrawerW { d.keyboard = on; return d }

// Loading shows an active Skeleton instead of the content (antd loading).
func (d *DrawerW) Loading(on bool) *DrawerW { d.skeleton = on; return d }

// Push sets how far a nested drawer pushes this one aside (default 180;
// 0 turns it off; antd push.distance).
func (d *DrawerW) Push(px float32) *DrawerW { d.push = px; return d }

// AfterOpenChanged reports, after Body, that the open or close slide
// finished this frame (antd afterOpenChange; read *open for the state).
func (d *DrawerW) AfterOpenChanged() bool { return d.settled }

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
	stack := State[[]*drawerCtx](c, drawerStackKey, nil)
	if len(*stack) > 0 && *d.open {
		(*stack)[len(*stack)-1].childOpen = true
	}
	id := hashID(c.scope, "drawer", d.title)
	a, live := drawerMotion(c, id, *d.open)
	// 1: open since the last close, 2: the open slide finished.
	shown := State[uint8](c, hashID(id, "shown"), nil)
	if !live {
		if *shown != 0 {
			*shown, d.settled = 0, true // every close, as antd afterOpenChange(false)
		}
		return d
	}
	if *d.open {
		if a >= 1 && *shown != 2 {
			*shown, d.settled = 2, true
		} else if *shown == 0 {
			*shown = 1
		}
	}
	leaving := !*d.open
	place := d.place.resolve(c.IsRTL()) // physical edge
	horiz := place == DrawerLeft || place == DrawerRight
	size := d.size
	if size == 0 {
		ext := c.size.H
		if horiz {
			ext = c.size.W
		}
		size = tk.Size
		if r := float32(tk.SizeRatio) * ext; tk.SizeRatio > 0 && (size == 0 || r < size) {
			size = r
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
			content := fn
			if d.skeleton {
				content = func() { Skeleton(c).Active().Show() }
			}
			body := Scroll(c, content, layout.Grow(1), layout.Pad(tk.Padding))
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
	push := c.Anim(id, "push", b2f(ctx.childOpen)*d.push, th.DurationSlow)
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
	if c.EscapeAt(holder.n.id) && d.keyboard {
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
			col := tk.ResizeLineColor
			if c.Dragging(id) && tk.ResizeLineActiveColor.A > 0 {
				col = tk.ResizeLineActiveColor
			}
			col = theme.Fade(col, float64(on))
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
		ib := IconButton(c, icon.Close, c.Locale().Close).Size(tk.CloseBtnSize, tk.CloseIconSize)
		if shad || th.UsesElementPlusControls() {
			ib.NoHoverBg()
		}
		if th.UsesElementPlusControls() && (ib.Hovered() || ib.ia.Focused) {
			ib.Color(th.Primary.Base)
		} else if tk.CloseIconColor.A > 0 {
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
	atEnd := tk.CloseAtEnd
	switch d.closePlace {
	case "end":
		atEnd = true
	case "start":
		atEnd = false
	}
	hd := Row(c, func() {
		if !atEnd {
			close() // Ant: before the title
		}
		title()
		if d.extra != nil {
			Row(c, d.extra, layout.Gap(8), layout.Middle())
		}
		if atEnd {
			close()
		}
	}, pad, layout.Gap(tk.HeaderGap), layout.Middle())
	if th.UsesElementPlusControls() {
		hd.n.ln.Margin.Bottom = 32 // drawer.scss .el-drawer__header
	} else if el {
		hd.n.ln.Margin.Bottom = 12
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
// "placement", "okType", "cancelType" (string), "showCancel", "arrow",
// "disabled" (bool), "width" (float32).
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
	arrow                bool
	width                float32
	okType, cancelType   string
	z                    *lazy
	confirmed, cancelled bool
	isOpen               bool
}

// Popconfirm wraps trigger content; clicking it opens a confirm bubble.
// The default placement, width, arrow and cancel button type come from
// theme.PopoverToken (Ant top; Element bottom, 150 wide, text cancel
// button; shadcn no arrow).
func Popconfirm(c *Context, title, desc string, trigger func()) *PopconfirmW {
	b := c.box("popconfirm", layout.Row, nil)
	b.n.ln.NoStretch = true
	b.Body(trigger)
	tk := &c.ThemeFor(theme.CompPopover).Components.Popover
	place, cancelType := "top", ""
	if tk.PopconfirmBottom {
		place = "bottom"
	}
	if tk.PopconfirmCancelText {
		cancelType = "text"
	}
	p := &PopconfirmW{c: c, b: b, title: title, desc: desc,
		okText:     Prop(c, "Popconfirm", "okText", ""),
		cancelText: Prop(c, "Popconfirm", "cancelText", ""),
		place:      Prop(c, "Popconfirm", "placement", place),
		noCancel:   !Prop(c, "Popconfirm", "showCancel", true),
		arrow:      Prop(c, "Popconfirm", "arrow", tk.Arrow),
		disabled:   Prop(c, "Popconfirm", "disabled", false),
		width:      Prop(c, "Popconfirm", "width", tk.PopconfirmWidth),
		okType:     Prop(c, "Popconfirm", "okType", "primary"),
		cancelType: Prop(c, "Popconfirm", "cancelType", cancelType),
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

// Arrow shows or hides the bubble arrow (default from the theme; antd
// arrow).
func (p *PopconfirmW) Arrow(on bool) *PopconfirmW { p.arrow = on; return p }

// Width fixes the bubble width (Element width, default 150 there; zero
// fits the content).
func (p *PopconfirmW) Width(px float32) *PopconfirmW { p.width = px; return p }

// OkType sets the confirm button type: "primary" (default), "default",
// "dashed", "text", "link" or "danger" (antd okType, Element
// confirm-button-type).
func (p *PopconfirmW) OkType(t string) *PopconfirmW { p.okType = t; return p }

// CancelType sets the cancel button type, as OkType (default "default";
// Element "text", its cancel-button-type).
func (p *PopconfirmW) CancelType(t string) *PopconfirmW { p.cancelType = t; return p }

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
	place, _ := c.popPlacement(p.place)
	off := tk.OffsetNoArrow // as Popover (antd Popconfirm renders a Popover)
	if p.arrow {
		off = tk.Offset
	}
	ov := Overlay(c, OverlayOptions{Anchor: b.n.id, Place: place, Offset: off, Flip: true}, func() {
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
			if !p.noCancel && popButtonType(Button(c, cancelText).Small(), p.cancelType).Clicked() {
				p.cancelled = true
				st.open = false
				popRestore(c, st)
			}
			ok := popButtonType(Button(c, okText).Small(), p.okType)
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
	if p.width > 0 {
		ov.n.ln.Width = p.width
	}
	ov.n.setRole(a11y.RoleAlertDialog).setName(title)
	if c.a11yOn {
		ov.n.fill(semFunc(func(s *a11y.Node) {
			s.Description = desc
			dialogSem(s)
		}))
	}
	// The Popover panel: radius, border (shadcn, Element) and arrow.
	ov.Bg(tk.Bg).Radius(tk.Radius).Shadow(tk.Shadow).Opacity(a)
	if tk.BorderColor.A > 0 {
		ov.Border(1, tk.BorderColor)
	}
	st.panel = ov.n.id
	if !p.arrow {
		return
	}
	ov.n.over = func(l *paint.List, r geom.Rect) {
		if ar, ok := c.anchorRect(b.n.id); ok {
			drawPopupArrow(l, r, ar, tk.Bg, tk.BorderColor)
		}
	}
}

// popButtonType styles a Popconfirm button by its antd / Element type
// name: "danger" is a primary danger button, Element "success" /
// "warning" / "info" are the solid status buttons.
func popButtonType(b *ButtonW, t string) *ButtonW {
	switch t {
	case "danger":
		return b.Primary().Danger()
	case "success":
		return b.Success()
	case "warning":
		return b.Warning()
	case "info":
		return b.Info()
	}
	return b.Variant(buttonVariantOf(t, VariantDefault))
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
