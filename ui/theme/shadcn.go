package theme

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/paint"
)

// Illustration holds the colors of the Empty / Result pictures.
type Illustration struct {
	// Ant "presented" empty box.
	Ground, Back, Front, Edge, Inner Color
	// Simple empty outline.
	Line Color
	// Element empty picture: three fills from dark to light and the ink.
	Fill1, Fill2, Fill3, Ink Color
	// Result pictures: soft and mid tints of the primary color.
	Soft, Mid Color
}

func defaultIllustration(dark bool, pr Palette, border, borderSecondary Color) Illustration {
	if dark {
		return Illustration{
			Ground: Hex("#272727"), Back: Hex("#3a3a3a"), Front: Hex("#2b2b2b"), Edge: Hex("#4a4a4a"), Inner: Hex("#1f1f1f"),
			Line:  borderSecondary,
			Fill1: Hex("#3b3c3e"), Fill2: Hex("#2f3032"), Fill3: Hex("#262729"), Ink: Hex("#55575b"),
			Soft: pr.Shades[1], Mid: pr.Shades[3],
		}
	}
	return Illustration{
		Ground: Hex("#f5f5f7"), Back: Hex("#aeb8c2"), Front: Hex("#f5f5f7"), Edge: Hex("#dce0e6"), Inner: Hex("#ffffff"),
		Line:  border,
		Fill1: Hex("#dcdde0"), Fill2: Hex("#e8eaee"), Fill3: Hex("#f5f6f8"), Ink: Hex("#c4c7cc"),
		Soft: pr.Shades[0], Mid: pr.Shades[2],
	}
}

// ShadcnOptions configures Shadcn.
type ShadcnOptions struct {
	// Base is the neutral color family: "neutral" (default), "zinc",
	// "slate", "stone" or "gray".
	Base string
	// Radius is shadcn's --radius in px (default 8, i.e. 0.5rem). Cards and
	// dialogs use it, buttons and inputs Radius-2, small parts Radius-4.
	Radius float32
	// Primary replaces the near-black (near-white in dark mode) primary
	// with a brand color; the focus ring follows it.
	Primary Color
	Dark    bool
	Compact bool
}

// ShadcnArg is accepted by Shadcn: a ShadcnOptions value or a generic
// Option (Dark, WithPrimary, Compact).
type ShadcnArg interface{ applyShadcn(*ShadcnOptions) }

func (o ShadcnOptions) applyShadcn(dst *ShadcnOptions) {
	if o.Base != "" {
		dst.Base = o.Base
	}
	if o.Radius > 0 {
		dst.Radius = o.Radius
	}
	if o.Primary.A > 0 {
		dst.Primary = o.Primary
	}
	dst.Dark = dst.Dark || o.Dark
	dst.Compact = dst.Compact || o.Compact
}

func (f Option) applyShadcn(dst *ShadcnOptions) {
	var o options
	f(&o)
	if o.dark {
		dst.Dark = true
	}
	if o.compact {
		dst.Compact = true
	}
	if o.primary != nil {
		dst.Primary = *o.primary
	}
}

// ShadcnVars are the values of shadcn/ui's CSS variables for a theme.
type ShadcnVars struct {
	Background, Foreground                   Color
	Card, CardForeground                     Color
	Popover, PopoverForeground               Color
	Primary, PrimaryForeground               Color
	Secondary, SecondaryForeground           Color
	Muted, MutedForeground                   Color
	Accent, AccentForeground                 Color
	Destructive, DestructiveForeground       Color
	Border, Input, Ring                      Color
	Chart                                    [5]Color
	Sidebar, SidebarForeground               Color
	SidebarPrimary, SidebarPrimaryForeground Color
	SidebarAccent, SidebarAccentForeground   Color
	SidebarBorder, SidebarRing               Color
	Radius                                   float32 // px
	Options                                  ShadcnOptions
}

// ShadcnBases lists the base colors Shadcn accepts.
var ShadcnBases = []string{"neutral", "zinc", "slate", "stone", "gray"}

// tailwindScales are the Tailwind CSS gray families (50, 100 … 900, 950)
// that shadcn/ui's base colors are built from.
var tailwindScales = map[string][11]string{
	"neutral": {"#fafafa", "#f5f5f5", "#e5e5e5", "#d4d4d4", "#a3a3a3", "#737373", "#525252", "#404040", "#262626", "#171717", "#0a0a0a"},
	"zinc":    {"#fafafa", "#f4f4f5", "#e4e4e7", "#d4d4d8", "#a1a1aa", "#71717a", "#52525b", "#3f3f46", "#27272a", "#18181b", "#09090b"},
	"slate":   {"#f8fafc", "#f1f5f9", "#e2e8f0", "#cbd5e1", "#94a3b8", "#64748b", "#475569", "#334155", "#1e293b", "#0f172a", "#020617"},
	"stone":   {"#fafaf9", "#f5f5f4", "#e7e5e4", "#d6d3d1", "#a8a29e", "#78716c", "#57534e", "#44403c", "#292524", "#1c1917", "#0c0a09"},
	"gray":    {"#f9fafb", "#f3f4f6", "#e5e7eb", "#d1d5db", "#9ca3af", "#6b7280", "#4b5563", "#374151", "#1f2937", "#111827", "#030712"},
}

// scale indexes a Tailwind family by its step name (50, 100, … 950).
type scale [11]Color

func (s scale) at(step int) Color {
	switch step {
	case 50:
		return s[0]
	case 950:
		return s[10]
	}
	return s[step/100]
}

// shadcnPalette derives interaction shades the shadcn way: hover is the
// color at 90% opacity over the page, tints are the color mixed into it.
func shadcnPalette(c, bg Color, dark bool) Palette {
	over := func(a float64) Color { return Over(Fade(c, a), bg) }
	tint := func(w float64) Color { return Mix(bg, c, w) }
	far := Hex("#000000")
	if dark {
		far = Hex("#ffffff")
	}
	deep := func(w float64) Color { return Mix(c, far, w) }
	s := [10]Color{tint(.08), tint(.16), tint(.3), tint(.5), tint(.75), c, deep(.2), deep(.4), deep(.6), deep(.8)}
	return Palette{
		Base: c, Hover: over(.9), Active: over(.8),
		Bg: s[0], BgHover: s[1], Border: s[2], BorderHover: s[3],
		TextHover: over(.8), Text: c, TextActive: deep(.2),
		Shades: s,
	}
}

func shTW(y, blur, spread float32, a float64) paint.Shadow {
	return paint.Shadow{Y: y, Blur: blur, Spread: spread, Color: RGBA(0, 0, 0, a)}
}

// Shadcn returns a shadcn/ui token set. Without arguments it is the light
// "neutral" theme with a 0.5rem radius; pass ShadcnOptions to pick another
// base color, radius or brand color, and Dark() (or ShadcnOptions.Dark) for
// the dark variant:
//
//	theme.Shadcn()
//	theme.Shadcn(theme.Dark())
//	theme.Shadcn(theme.ShadcnOptions{Base: "zinc", Radius: 8})
//	theme.Shadcn(theme.ShadcnOptions{Primary: theme.Hex("#2563eb")}, theme.Dark())
//
// Colors follow the published shadcn/ui variables (background, foreground,
// primary, secondary, muted, accent, destructive, border, input, ring,
// chart-1..5); th.Shadcn keeps them and CSSVars exports them under the
// same names.
func Shadcn(args ...ShadcnArg) *Theme {
	o := ShadcnOptions{Base: "neutral", Radius: 8}
	for _, a := range args {
		if a != nil {
			a.applyShadcn(&o)
		}
	}
	o.Base = strings.ToLower(o.Base)
	hexes, ok := tailwindScales[o.Base]
	if !ok {
		o.Base = "neutral"
		hexes = tailwindScales["neutral"]
	}
	var s scale
	for i, h := range hexes {
		s[i] = Hex(h)
	}
	dark := o.Dark
	white := Hex("#ffffff")
	v := &ShadcnVars{Radius: o.Radius, Options: o}
	if !dark {
		v.Background, v.Foreground = white, s.at(950)
		v.Card, v.CardForeground = white, s.at(950)
		v.Popover, v.PopoverForeground = white, s.at(950)
		v.Primary, v.PrimaryForeground = s.at(900), s.at(50)
		v.Secondary, v.SecondaryForeground = s.at(100), s.at(900)
		v.Muted, v.MutedForeground = s.at(100), s.at(500)
		v.Accent, v.AccentForeground = s.at(100), s.at(900)
		v.Border, v.Input, v.Ring = s.at(200), s.at(200), s.at(400)
		v.Chart = [5]Color{Hex("#e76e50"), Hex("#2a9d90"), Hex("#274754"), Hex("#e8c468"), Hex("#f4a462")}
		v.Sidebar, v.SidebarForeground = s.at(50), s.at(700)
		v.SidebarAccent, v.SidebarAccentForeground = s.at(100), s.at(900)
		v.SidebarBorder = s.at(200)
	} else {
		v.Background, v.Foreground = s.at(950), s.at(50)
		v.Card, v.CardForeground = s.at(950), s.at(50)
		v.Popover, v.PopoverForeground = s.at(950), s.at(50)
		v.Primary, v.PrimaryForeground = s.at(50), s.at(900)
		v.Secondary, v.SecondaryForeground = s.at(800), s.at(50)
		v.Muted, v.MutedForeground = s.at(800), s.at(400)
		v.Accent, v.AccentForeground = s.at(800), s.at(50)
		v.Border, v.Input, v.Ring = s.at(800), s.at(800), s.at(300)
		v.Chart = [5]Color{Hex("#2662d9"), Hex("#2eb88a"), Hex("#e88c30"), Hex("#af57db"), Hex("#e23670")}
		v.Sidebar, v.SidebarForeground = s.at(900), s.at(100)
		v.SidebarAccent, v.SidebarAccentForeground = s.at(800), s.at(100)
		v.SidebarBorder = s.at(800)
	}
	v.Destructive, v.DestructiveForeground = Hex("#ef4444"), s.at(50)
	if o.Primary.A > 0 {
		v.Primary = o.Primary
		v.PrimaryForeground = s.at(50)
		if Luma(o.Primary) > 0.6 {
			v.PrimaryForeground = s.at(900)
		}
		v.Ring = o.Primary
	}
	v.SidebarPrimary, v.SidebarPrimaryForeground = v.Primary, v.PrimaryForeground
	v.SidebarRing = v.Ring

	t := &Theme{Name: "shadcn", Flavor: StyleShadcn, Dark: dark, Shadcn: v, Icons: LucideIcons()}
	bg := v.Background
	t.Primary = shadcnPalette(v.Primary, bg, dark)
	if o.Primary.A == 0 {
		// The neutral primary has no tint: selections use the accent gray.
		t.Primary.Bg, t.Primary.BgHover = v.Accent, Mix(v.Accent, v.Border, .5)
		t.Primary.Shades[0], t.Primary.Shades[1] = t.Primary.Bg, t.Primary.BgHover
		t.Primary.Border, t.Primary.Shades[2] = v.Border, v.Border
	}
	success, warning, info := Hex("#16a34a"), Hex("#f59e0b"), Hex("#3b82f6")
	if dark {
		success = Hex("#22c55e")
	}
	t.Success = shadcnPalette(success, bg, dark)
	t.Warning = shadcnPalette(warning, bg, dark)
	t.Error = shadcnPalette(v.Destructive, bg, dark)
	t.Info = shadcnPalette(info, bg, dark)

	r := o.Radius
	t.RadiusLG, t.Radius, t.RadiusSM, t.RadiusXS = r, max(0, r-2), max(0, r-4), max(0, r-6)
	t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 36, 32, 40
	if o.Compact {
		t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 32, 28, 36
	}
	t.ControlPaddingH, t.ControlPaddingHSM = 12, 8
	t.LineWidth = 1
	t.FontSize, t.FontSizeSM, t.FontSizeLG, t.FontSizeXL = 14, 12, 16, 20
	t.Heading = [5]float32{36, 30, 24, 20, 18}
	t.LineHeight = 20.0 / 14.0
	// Tailwind leading for text-xs … text-4xl.
	t.LineHeights = map[float32]float32{12: 16, 13: 18, 14: 20, 16: 24, 18: 28, 20: 28, 24: 32, 30: 36, 36: 40}
	t.FontWeightStrong = 600
	t.PaddingXXS, t.PaddingXS, t.PaddingSM, t.Padding, t.PaddingMD, t.PaddingLG, t.PaddingXL = 4, 8, 12, 16, 20, 24, 32
	t.MarginXS, t.MarginSM, t.Margin, t.MarginMD, t.MarginLG, t.MarginXL = 8, 12, 16, 20, 24, 32
	t.DurationFast, t.DurationMid, t.DurationSlow = 150*time.Millisecond, 200*time.Millisecond, 300*time.Millisecond
	t.HeaderHeight, t.SiderWidth, t.SiderCollapsed, t.MenuItemHeight = 56, 256, 48, 32
	t.TablePaddingV, t.TablePaddingH = 8, 8

	t.Text, t.TextHeading = v.Foreground, v.Foreground
	t.TextTertiary, t.TextQuaternary = v.MutedForeground, v.MutedForeground
	if !dark {
		t.TextSecondary = s.at(600)
		t.TextDisabled = s.at(400)
		t.Fill, t.FillSecondary, t.FillTertiary, t.FillQuaternary = s.at(200), s.at(100), s.at(100), s.at(50)
		t.BgLayout = s.at(50)
		t.BgContainerDisabled = s.at(50)
		t.BgTextActive = s.at(200)
		t.BorderHover = s.at(300)
	} else {
		t.TextSecondary = s.at(300)
		t.TextDisabled = s.at(600)
		t.Fill, t.FillSecondary, t.FillTertiary, t.FillQuaternary = s.at(700), s.at(800), s.at(800), s.at(900)
		t.BgLayout = s.at(950)
		t.BgContainerDisabled = s.at(900)
		t.BgTextActive = s.at(700)
		t.BorderHover = s.at(700)
	}
	t.FillAlter = Mix(bg, v.Muted, .5)
	t.TextOnPrimary = v.PrimaryForeground
	t.TextLink, t.TextLinkHover, t.TextLinkActive = v.Primary, t.Primary.TextHover, t.Primary.Active
	if o.Primary.A == 0 {
		t.TextLink, t.TextLinkHover, t.TextLinkActive = v.Foreground, v.Foreground, v.Foreground
	}
	t.Border, t.BorderSecondary, t.Split = v.Input, v.Border, v.Border
	t.BgContainer, t.BgElevated = v.Background, v.Popover
	t.BgSpotlight, t.TextOnSpotlight = v.Primary, v.PrimaryForeground
	if o.Primary.A > 0 {
		t.BgSpotlight, t.TextOnSpotlight = v.Foreground, v.Background
	}
	t.BgMask = RGBA(0, 0, 0, .8)
	t.BgTextHover = v.Accent
	t.ItemHoverBg, t.ItemSelectedBg, t.ItemSelected = v.Accent, v.Accent, v.AccentForeground
	t.TextOnSolid = Hex("#ffffff")

	t.ControlFocusBorder = v.Ring
	t.ControlOutline, t.ControlOutlineWidth, t.ControlOutlineOffset = Fade(v.Ring, .5), 3, 0
	t.ControlOutlineError, t.ControlOutlineWarning = Fade(v.Destructive, .2), Fade(warning, .2)
	t.FocusRing = v.Ring
	t.FocusOutline, t.FocusRingWidth, t.FocusRingOffset = v.Ring, 2, 2
	// a11y: the light --ring (zinc-400 …) is ~2.5:1; darken the keyboard
	// focus ring just enough for WCAG 1.4.11 (3:1). The focused-input
	// border is the inputs' focus indicator (the ring/50 halo beside it is
	// fainter still), so it takes the same lifted color; the halo keeps
	// --ring.
	t.FocusRing = EnsureContrast(t.FocusRing, t.BgContainer, 3, v.Foreground)
	t.FocusOutline = t.FocusRing
	t.ControlFocusBorder = t.FocusRing

	xs := []paint.Shadow{shTW(1, 2, 0, .05)}
	t.Shadow = []paint.Shadow{shTW(4, 6, -1, .1), shTW(2, 4, -2, .1)}            // shadow-md
	t.ShadowSecondary = []paint.Shadow{shTW(10, 15, -3, .1), shTW(4, 6, -4, .1)} // shadow-lg
	t.ShadowTertiary = xs                                                        // shadow-sm
	t.ButtonShadow, t.PrimaryShadow, t.DangerShadow = xs, xs, xs
	t.InputShadow = xs
	t.CardShadow = xs
	t.CardRadius = r + 4 // rounded-xl
	t.CardBorder, t.PopupBorder, t.ModalBorder = v.Border, v.Border, v.Border
	t.TooltipShadow = nil

	t.ButtonFontWeight = 500
	t.ButtonPaddingH, t.ButtonPaddingHSM, t.ButtonPaddingHLG = 16, 12, 32
	t.SecondaryBg, t.SecondaryText = v.Secondary, v.SecondaryForeground
	t.SecondaryBgHover = Over(Fade(v.Secondary, .8), bg)
	if dark {
		t.SecondaryBgHover = s.at(700)
	}
	t.SwitchOffBg, t.SwitchOffBgHover = v.Input, v.Input
	t.SwitchHeight, t.SwitchWidth = 18.4, 32 // new-york v4: h-[1.15rem] w-8
	t.LabelFontWeight, t.SwitchHandle = 500, v.Background
	t.CheckboxSize, t.CheckboxRadius = 16, 4
	// new-york v4: inputs and outline buttons are transparent (light) or
	// input/30 (dark, hover input/50); dark destructive buttons are
	// destructive/60; the dark off switch thumb is foreground. Rate has no
	// shadcn counterpart: neutral foreground stars, voids drawn as
	// muted-foreground outlines (an input-colored void would be ~1.3:1).
	t.ControlBg, t.ControlBgHover = bg, v.Accent
	t.ButtonDangerBg, t.SwitchHandleOff = v.Destructive, t.SwitchHandle
	if dark {
		t.ControlBg, t.ControlBgHover = Over(Fade(v.Input, .3), bg), Over(Fade(v.Input, .5), bg)
		t.ButtonDangerBg, t.SwitchHandleOff = Over(Fade(v.Destructive, .6), bg), v.Foreground
	}
	t.RateStar, t.RateStarEmpty = v.Primary, v.MutedForeground
	t.SliderRailHeight = 6

	t.HeaderBg = v.Background
	t.SiderBg, t.SiderLightBg = v.Sidebar, v.Sidebar
	t.SiderSubmenuBg = v.Sidebar
	t.SiderText, t.SiderTextActive = v.SidebarForeground, v.SidebarAccentForeground
	t.SiderItemHoverBg = v.SidebarAccent
	t.TableHeaderBg = v.Background
	t.TableRowHoverBg = Mix(bg, v.Muted, .5)
	t.TableHeaderText, t.TableHeaderWeight = v.MutedForeground, 500
	t.TableSorterIdle, t.TableFixedShadowAlpha = v.MutedForeground, .12
	// shadcn data tables tint nothing for sorting; the header sort control
	// is a ghost button (TableHeaderActiveBg zero, accent pill on hover).
	t.TableSortedBg, t.TableHeaderActiveBg = Color{}, Color{}
	if dark {
		t.TableFixedShadowAlpha = .5
	}
	t.SegmentedBg, t.SegmentedSelectedBg = v.Muted, v.Background
	if dark {
		t.SegmentedSelectedBg = s.at(950)
	}
	t.TabsHeaderBg = v.Muted

	t.StatusDefault = v.MutedForeground
	t.AvatarBg, t.AvatarText = v.Muted, v.Foreground
	t.SkeletonBg, t.SkeletonHighlight = v.Accent, Mix(v.Accent, v.Border, .6)
	t.SkeletonIcon = v.MutedForeground
	t.AddonBg, t.AddonText = Mix(bg, v.Muted, .5), v.MutedForeground
	t.MarkBg = t.Warning.BgHover // soft amber tint, dark in dark mode
	if dark {
		t.MarkText = v.Foreground
	}
	t.Illustration = Illustration{
		Ground: v.Muted, Back: s.at(300), Front: v.Muted, Edge: v.Border, Inner: v.Background,
		Line:  v.Border,
		Fill1: s.at(300), Fill2: s.at(200), Fill3: v.Muted, Ink: s.at(400),
		Soft: v.Muted, Mid: v.Border,
	}
	if dark {
		t.Illustration.Back, t.Illustration.Fill1, t.Illustration.Fill2, t.Illustration.Ink = s.at(700), s.at(700), s.at(800), s.at(600)
	}
	t.deriveComponents(shadcnComponents(args))
	return t
}
