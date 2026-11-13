package mobile

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Style names the mobile design language the components imitate.
type Style uint8

const (
	AntMobile Style = iota // Ant Design Mobile (default)
	Vant                   // Vant
	// Shadcn derives a mobile look from the shadcn/ui design language
	// (neutral colors, thin borders, rounded-lg/xl, near-black primary,
	// Drawer-style bottom sheets, Sonner toasts). The default AntMobile
	// resolves to Shadcn under a shadcn ui theme (theme.Shadcn).
	Shadcn
)

// Theme holds the mobile design tokens. It is derived from the ui theme
// (dark mode, brand color, mask) and the selected Style (sizes, neutrals).
// Components capture it at build time; read it with ThemeOf.
type Theme struct {
	Style Style
	Dark  bool
	Base  *theme.Theme

	// Brand and status colors.
	Primary, Success, Warning, Danger theme.Color
	PrimaryLight                      theme.Color // tinted background (tags, notice bars)

	// Neutrals.
	Text, TextSecondary, TextWeak, TextLight theme.Color
	Border                                   theme.Color // hairlines between cells
	PageBg                                   theme.Color // page background behind lists
	Box                                      theme.Color // cells, bars, panels
	Fill                                     theme.Color // stepper buttons, search fields, skeleton
	Press                                    theme.Color // overlay drawn over a pressed item
	Mask                                     theme.Color
	ToastBg                                  theme.Color
	OnPrimary                                theme.Color // text on solid colored controls

	// Typography (logical px; 1pt = 1px at the mobile reference density).
	FontTiny, FontSmall, FontMain, FontBody, FontTitle, FontLarge float32

	// Geometry.
	TouchMin                      float32 // minimum touch target (44pt)
	PadH                          float32 // horizontal page padding
	Radius, RadiusLarge, PopupRad float32
	NavBarHeight, TabBarHeight    float32
	CellPadV, CellMinHeight       float32
	ButtonHeight                  [4]float32 // Mini, Small, Middle, Large
	ButtonFont                    [4]float32
	ButtonRadius                  float32
	PickerItemHeight              float32
	PickerVisible                 int // rows shown in a picker wheel
	TabsHeight                    float32
	TabsLineWidth, TabsLineHeight float32 // 0 width = as wide as the title
	DialogWidth                   float32
	ActionHeight                  float32
	ToastPad                      float32
	SearchHeight, SearchRadius    float32
	StepperHeight, StepperInputW  float32
	BadgeColor                    theme.Color
	NoticeBg, NoticeText          theme.Color
	TitleWeight                   text.Weight
	FormLabelWidth                float32

	// Tokens of the shadcn derivation; the Ant Mobile and Vant values keep
	// their looks (zero colors mean "none").
	OnSolid       theme.Color // text on solid success / warning / danger fills
	ButtonWeight  text.Weight // button label weight
	CardBorder    theme.Color // outline of lists, cards, sheets and dialogs
	DrawerHandle  theme.Color // grab handle on top of bottom sheets
	ToastText     theme.Color // toast message color (on ToastBg)
	ToastBorder   theme.Color // toast outline
	NoticeBorder  theme.Color // notice bar top / bottom lines
	SwitchOff     theme.Color // switch track when off
	SwitchThumb   theme.Color // switch thumb when off
	SwitchThumbOn theme.Color // switch thumb when on
	SwitchW       float32     // switch track size
	SwitchH       float32
	Ring          theme.Color // keyboard focus outline
	SegmentThumb  theme.Color // selected tab of the segmented (shadcn) Tabs
}

// Font returns a text style of px size and weight with a mobile line
// height (1.5×, rounded).
func (t *Theme) Font(px float32, w text.Weight) text.Style {
	return text.Style{Size: px, Weight: w, LineHeight: float32(int(px*1.5 + 0.5))}
}

// ColorOf maps a status name ("primary", "success", "warning", "danger",
// "default") to a color.
func (t *Theme) ColorOf(name string) theme.Color {
	switch name {
	case "primary", "info", "processing":
		return t.Primary
	case "success":
		return t.Success
	case "warning":
		return t.Warning
	case "danger", "error":
		return t.Danger
	}
	return t.TextSecondary
}

// NewTheme derives the mobile tokens for style from the ui theme base.
// Under a high-contrast base (theme.HighContrast) the neutrals are lifted
// to WCAG AA like the ui theme's: text 4.5:1, arrows, borders and switch
// tracks 3:1 (see highContrast).
func NewTheme(base *theme.Theme, style Style) *Theme {
	if base == nil {
		base = theme.AntDesign()
	}
	t := newTheme(base, style)
	if base.HighContrast {
		t.highContrast()
	}
	return t
}

func newTheme(base *theme.Theme, style Style) *Theme {
	if style == AntMobile && base.Flavor == theme.StyleShadcn {
		style = Shadcn
	}
	t := &Theme{Style: style, Dark: base.Dark, Base: base, TouchMin: 44, OnPrimary: theme.Hex("#ffffff")}
	switch style {
	case Vant:
		t.vant()
	case Shadcn:
		t.shadcn()
		return t
	default:
		t.antMobile()
	}
	t.OnSolid, t.ButtonWeight = t.OnPrimary, text.Regular
	t.SwitchThumb, t.SwitchThumbOn = theme.Hex("#ffffff"), theme.Hex("#ffffff")
	t.SwitchOff = t.Fill
	t.SwitchW, t.SwitchH = 51, 31
	if style == Vant {
		t.SwitchW, t.SwitchH = 52, 30
	} else if !t.Dark {
		t.SwitchOff = theme.Hex("#e5e5e5")
	}
	t.ToastText = theme.Hex("#ffffff")
	t.Ring = base.FocusOutline
	// A customized brand color in the ui theme wins over the style default
	// (the stock Ant Design / Element Plus blues mean "not customized").
	if p := base.Primary.Base; !isStockPrimary(p) {
		t.Primary = p
	}
	t.PrimaryLight = theme.Fade(t.Primary, 0.1)
	if t.Dark {
		t.PrimaryLight = theme.Fade(t.Primary, 0.2)
	}
	return t
}

// Contrast targets of a high-contrast mobile theme (WCAG 2.2 AA).
const (
	hcText    = 4.5 // text and its weaker shades
	hcNonText = 3.0 // arrows, hairlines, switch tracks
)

// highContrast lifts the style's hard-coded neutrals to the WCAG targets
// on the surfaces they are drawn on (cells, the page, fills). Each color
// moves the shortest distance (theme.EnsureContrast's 10% steps) towards
// the mobile text color, then towards black (white in dark mode), so the
// style keeps its hues and only gets darker (lighter) ink.
func (t *Theme) highContrast() {
	ink := theme.Hex("#000000")
	if t.Dark {
		ink = theme.Hex("#ffffff")
	}
	lift := func(c *theme.Color, min float64, toward theme.Color, bgs ...theme.Color) {
		for _, bg := range bgs {
			*c = theme.EnsureContrast(*c, opaque(bg, t.Box), min, toward, ink)
		}
	}
	surfaces := []theme.Color{t.Box, t.PageBg, t.Fill}
	lift(&t.Text, hcText, ink, surfaces...)
	lift(&t.TextSecondary, hcText, t.Text, surfaces...)
	lift(&t.TextWeak, hcText, t.Text, surfaces...)
	lift(&t.TextLight, hcNonText, t.Text, t.Box, t.PageBg) // arrows, disabled rows
	lift(&t.Border, hcNonText, t.Text, t.Box, t.PageBg)
	lift(&t.SwitchOff, hcNonText, t.Text, t.Box)
	lift(&t.NoticeText, hcText, ink, t.NoticeBg)
	if t.CardBorder.A > 0 {
		lift(&t.CardBorder, hcNonText, t.Text, t.PageBg)
	}
}

// opaque composites a translucent color over bg.
func opaque(c, bg theme.Color) theme.Color {
	if c.A < 255 {
		return theme.Over(c, bg)
	}
	return c
}

// shad reports the shadcn derivation (structural branches: Drawer sheets,
// grouped action sheets, Sonner toasts, segmented tabs).
func (t *Theme) shad() bool { return t.Style == Shadcn }

func (t *Theme) antMobile() {
	// Ant Design Mobile 5 tokens (--adm-color-*, --adm-font-size-*).
	t.Primary, t.Success, t.Warning, t.Danger = theme.Hex("#1677ff"), theme.Hex("#00b578"), theme.Hex("#ff8f1f"), theme.Hex("#ff3141")
	t.Text, t.TextSecondary, t.TextWeak, t.TextLight = theme.Hex("#333333"), theme.Hex("#666666"), theme.Hex("#999999"), theme.Hex("#cccccc")
	t.Border, t.PageBg, t.Box, t.Fill = theme.Hex("#eeeeee"), theme.Hex("#f5f5f5"), theme.Hex("#ffffff"), theme.Hex("#f5f5f5")
	t.Press = theme.RGBA(0, 0, 0, .08)
	t.Mask = theme.RGBA(0, 0, 0, .55)
	t.ToastBg = theme.RGBA(0, 0, 0, .7)
	t.NoticeBg, t.NoticeText = theme.Hex("#fff9ed"), theme.Hex("#ff6010")
	if t.Dark {
		t.Primary, t.Success, t.Warning, t.Danger = theme.Hex("#3086ff"), theme.Hex("#34b368"), theme.Hex("#ffa930"), theme.Hex("#ff4a58")
		t.Text, t.TextSecondary, t.TextWeak, t.TextLight = theme.Hex("#e6e6e6"), theme.Hex("#b3b3b3"), theme.Hex("#808080"), theme.Hex("#4d4d4d")
		t.Border, t.PageBg, t.Box, t.Fill = theme.Hex("#2b2b2b"), theme.Hex("#000000"), theme.Hex("#1a1a1a"), theme.Hex("#2b2b2b")
		t.Press = theme.RGBA(255, 255, 255, .08)
		t.ToastBg = theme.RGBA(60, 60, 60, .92)
		t.NoticeBg, t.NoticeText = theme.Hex("#302217"), theme.Hex("#ff8a3d")
	}
	t.BadgeColor = t.Danger
	t.FontTiny, t.FontSmall, t.FontMain, t.FontBody, t.FontTitle, t.FontLarge = 10, 13, 15, 17, 18, 21
	t.PadH = 12
	t.Radius, t.RadiusLarge, t.PopupRad = 4, 8, 8
	t.NavBarHeight, t.TabBarHeight = 45, 50
	t.CellPadV, t.CellMinHeight = 12, 50
	t.ButtonHeight = [4]float32{24, 30, 40, 50}
	t.ButtonFont = [4]float32{13, 15, 17, 18}
	t.ButtonRadius = 4
	t.PickerItemHeight, t.PickerVisible = 34, 7
	t.TabsHeight, t.TabsLineWidth, t.TabsLineHeight = 42, 0, 2
	t.DialogWidth = 300
	t.ActionHeight = 55
	t.ToastPad = 12
	t.SearchHeight, t.SearchRadius = 32, 6
	t.StepperHeight, t.StepperInputW = 28, 44
	t.TitleWeight = text.Regular
	t.FormLabelWidth = 90
}

func (t *Theme) vant() {
	// Vant 4 tokens (--van-*).
	t.Primary, t.Success, t.Warning, t.Danger = theme.Hex("#1989fa"), theme.Hex("#07c160"), theme.Hex("#ff976a"), theme.Hex("#ee0a24")
	t.Text, t.TextSecondary, t.TextWeak, t.TextLight = theme.Hex("#323233"), theme.Hex("#646566"), theme.Hex("#969799"), theme.Hex("#c8c9cc")
	t.Border, t.PageBg, t.Box, t.Fill = theme.Hex("#ebedf0"), theme.Hex("#f7f8fa"), theme.Hex("#ffffff"), theme.Hex("#f2f3f5")
	t.Press = theme.RGBA(0, 0, 0, .1)
	t.Mask = theme.RGBA(0, 0, 0, .7)
	t.ToastBg = theme.RGBA(0, 0, 0, .7)
	t.NoticeBg, t.NoticeText = theme.Hex("#fffbe8"), theme.Hex("#ed6a0c")
	if t.Dark {
		t.Text, t.TextSecondary, t.TextWeak, t.TextLight = theme.Hex("#f5f5f5"), theme.Hex("#c9c9c9"), theme.Hex("#707070"), theme.Hex("#4d4d4d")
		t.Border, t.PageBg, t.Box, t.Fill = theme.Hex("#3a3a3c"), theme.Hex("#000000"), theme.Hex("#1c1c1e"), theme.Hex("#2c2c2e")
		t.Press = theme.RGBA(255, 255, 255, .1)
		t.ToastBg = theme.RGBA(60, 60, 60, .92)
		t.NoticeBg, t.NoticeText = theme.Hex("#2b2416"), theme.Hex("#ff9a4d")
	}
	t.BadgeColor = t.Danger
	t.FontTiny, t.FontSmall, t.FontMain, t.FontBody, t.FontTitle, t.FontLarge = 10, 12, 14, 14, 16, 20
	t.PadH = 16
	t.Radius, t.RadiusLarge, t.PopupRad = 2, 8, 16
	t.NavBarHeight, t.TabBarHeight = 46, 50
	t.CellPadV, t.CellMinHeight = 10, 44
	t.ButtonHeight = [4]float32{24, 32, 44, 50}
	t.ButtonFont = [4]float32{10, 12, 14, 16}
	t.ButtonRadius = 4
	t.PickerItemHeight, t.PickerVisible = 44, 6
	t.TabsHeight, t.TabsLineWidth, t.TabsLineHeight = 44, 40, 3
	t.DialogWidth = 320
	t.ActionHeight = 50
	t.ToastPad = 12
	t.SearchHeight, t.SearchRadius = 34, 2
	t.StepperHeight, t.StepperInputW = 28, 32
	t.TitleWeight = text.Semibold
	t.FormLabelWidth = 88
}

// ---------------------------------------------------------------------------
// Provider

// Options configure the mobile components inside a Provider.
type Options struct {
	Style Style
	// SafeArea overrides the window's safe-area insets (ui.Context.SafeArea,
	// set by the host through ui.SafeAreaWindow or Runner.SetSafeArea).
	SafeArea *geom.Insets
}

type providerFrame struct {
	th    *Theme
	style Style // as requested (AntMobile may have resolved to Shadcn)
	safe  *geom.Insets
}

type providerStack struct {
	frames []providerFrame
	cache  map[themeKey]*Theme
}

type themeKey struct {
	base  *theme.Theme
	style Style
}

var keyProvider = ui.ID(0x6d6f62696c650001)

func providers(c *ui.Context) *providerStack {
	return ui.State(c, keyProvider, func() providerStack { return providerStack{cache: map[themeKey]*Theme{}} })
}

func (s *providerStack) themeFor(base *theme.Theme, style Style) *Theme {
	k := themeKey{base, style}
	if t, ok := s.cache[k]; ok {
		return t
	}
	if len(s.cache) > 16 {
		clear(s.cache)
	}
	t := NewTheme(base, style)
	s.cache[k] = t
	return t
}

// Provider runs fn with the mobile options o (design style, safe area) and
// then renders imperative toasts queued with ShowToast. Wrap a mobile page
// in one Provider; without it components use the Ant Design Mobile style.
func Provider(c *ui.Context, o Options, fn func()) {
	s := providers(c)
	s.frames = append(s.frames, providerFrame{th: s.themeFor(c.Theme, o.Style), style: o.Style, safe: o.SafeArea})
	fn()
	renderToasts(c)
	s.frames = s.frames[:len(s.frames)-1]
}

// ThemeOf returns the mobile theme in effect.
func ThemeOf(c *ui.Context) *Theme {
	s := providers(c)
	if n := len(s.frames); n > 0 {
		f := s.frames[n-1]
		if f.th.Base == c.Theme {
			return f.th
		}
		return s.themeFor(c.Theme, f.style)
	}
	return s.themeFor(c.Theme, AntMobile)
}

// ---------------------------------------------------------------------------
// Safe area

// SafeAreaInsets returns the insets in effect: the innermost Provider
// override, else the window's (ui.Context.SafeArea).
func SafeAreaInsets(c *ui.Context) geom.Insets {
	s := providers(c)
	for i := len(s.frames) - 1; i >= 0; i-- {
		if s.frames[i].safe != nil {
			return *s.frames[i].safe
		}
	}
	return c.SafeArea()
}

// Edge selects a side of the screen.
type Edge uint8

const (
	Top Edge = iota
	Bottom
	Left
	Right
)

// SafeArea adds a blank of the safe-area inset of edge (Top: status bar /
// notch, Bottom: home indicator), filled with the box color.
func SafeArea(c *ui.Context, edge Edge) *ui.Box {
	in := SafeAreaInsets(c)
	th := ThemeOf(c)
	v := in.Top
	switch edge {
	case Bottom:
		v = in.Bottom
	case Left:
		v = in.Left
	case Right:
		v = in.Right
	}
	b := ui.Div(c)
	if edge == Left || edge == Right {
		b.Style(func(s *layoutStyle) { s.Width = v })
	} else {
		b.Style(func(s *layoutStyle) { s.Height = v })
	}
	_ = th
	return b
}

var stockPrimaries = func() []theme.Color {
	var out []theme.Color
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.ElementPlus(), theme.ElementPlus(theme.Dark())} {
		out = append(out, th.Primary.Base)
	}
	return out
}()

func isStockPrimary(c theme.Color) bool {
	for _, s := range stockPrimaries {
		if s == c {
			return true
		}
	}
	return false
}
