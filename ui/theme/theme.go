// Package theme defines the design tokens of the Tango self-drawn UI.
//
// Three token sets are provided: AntDesign (default, Ant Design v5 / Pro),
// ElementPlus and Shadcn (shadcn/ui, with its base colors, radius and brand
// color options). All expose the same Theme struct so every component reads
// a single vocabulary. Visual recipes that differ between the systems (focus
// rings, input halos, button weight and padding, switch geometry, popup
// borders, card shadows, line heights) are tokens too; Flavor is left for
// the few structural differences. Semantic colors are derived from one base
// color with each system's own algorithm (see palette.go, shadcn.go), and
// every constructor accepts Dark() to build the dark variant.
package theme

import (
	"time"

	"github.com/tangoui/tango/ui/paint"
)

// Flavor names the design system a theme imitates.
type Flavor int

const (
	Ant Flavor = iota
	Element
	// StyleShadcn is the shadcn/ui look (Shadcn constructor). Components
	// branch on it only for structural differences that tokens cannot
	// express (segmented tabs, Sonner toasts); colors, focus rings, heights
	// and shadows come from tokens.
	StyleShadcn
)

// Style aliases name the three design systems uniformly
// (th.Flavor == theme.StyleShadcn reads like th.Style).
const (
	StyleAnt     = Ant
	StyleElement = Element
)

// String returns "antd", "element-plus" or "shadcn".
func (f Flavor) String() string {
	switch f {
	case Element:
		return "element-plus"
	case StyleShadcn:
		return "shadcn"
	}
	return "antd"
}

// Theme is the complete token set read by components.
type Theme struct {
	Name   string
	Flavor Flavor
	Dark   bool
	// HighContrast is set on themes built by the HighContrast function.
	HighContrast bool

	Primary, Success, Warning, Error, Info Palette

	// Neutral text.
	Text           Color // colorText
	TextHeading    Color
	TextSecondary  Color
	TextTertiary   Color
	TextQuaternary Color // placeholder / disabled text
	TextDisabled   Color
	TextOnPrimary  Color // text on solid colored controls
	TextLink       Color
	TextLinkHover  Color
	TextLinkActive Color

	// Borders and fills.
	Border          Color // colorBorder (inputs, default buttons)
	BorderHover     Color // input border under the pointer
	BorderSecondary Color // colorBorderSecondary (cards, tables, dividers)
	Split           Color
	Fill            Color // strongest neutral fill
	FillSecondary   Color
	FillTertiary    Color
	FillQuaternary  Color
	FillAlter       Color // table header / zebra rows

	// Backgrounds.
	BgContainer         Color
	BgElevated          Color // popovers, dropdowns, modals
	BgLayout            Color
	BgSpotlight         Color // tooltip background
	BgMask              Color
	BgContainerDisabled Color
	BgTextHover         Color
	BgTextActive        Color

	// Selection inside menus, selects, trees.
	ItemHoverBg    Color
	ItemSelectedBg Color
	ItemSelected   Color // text color of selected item

	// Focus ring: Ant draws an outer shadow, Element a tinted border,
	// shadcn a ring separated from the control by an offset gap.
	ControlOutline        Color   // focused input halo (zero: none)
	ControlOutlineWidth   float32 // halo width
	ControlOutlineOffset  float32 // gap between the control edge and the halo (0: halo touches the border)
	ControlOutlineError   Color   // halo of a focused input in error status
	ControlOutlineWarning Color   // halo of a focused input in warning status
	ControlFocusBorder    Color   // border of a focused input
	FocusRing             Color   // :focus-visible outline of small controls (icon buttons, tags)
	FocusOutline          Color   // :focus-visible ring of buttons, checkboxes, switches
	FocusRingWidth        float32 // width of the FocusOutline ring
	FocusRingOffset       float32 // gap between the control and the FocusOutline ring

	// Text on special surfaces.
	TextOnSpotlight Color // tooltip text on BgSpotlight
	TextOnSolid     Color // text on solid success/warning/error fills (badges, danger buttons)

	// Buttons.
	ButtonFontWeight int // label weight (400 regular, 500 medium)
	// Horizontal padding, middle size.
	//
	// Deprecated: use Components.Button.PaddingInline (the field remains the source of the derived default).
	ButtonPaddingH float32
	// Deprecated: use Components.Button.PaddingInlineSM and Components.Button.PaddingInlineLG (the field remains the source of the derived default).
	ButtonPaddingHSM, ButtonPaddingHLG float32
	SecondaryBg, SecondaryText         Color // "secondary" button / neutral filled controls
	SecondaryBgHover                   Color

	// Selection controls.

	// Checkbox / radio label weight.
	//
	// Deprecated: use Components.Checkbox.LabelFontWeight (the field remains the source of the derived default).
	LabelFontWeight int
	// Switch thumb.
	//
	// Deprecated: use Components.Switch.HandleBg (the field remains the source of the derived default).
	SwitchHandle Color
	// Switch track when off.
	//
	// Deprecated: use Components.Switch.OffBg (the field remains the source of the derived default).
	SwitchOffBg Color
	// Deprecated: use Components.Switch.OffHoverBg (the field remains the source of the derived default).
	SwitchOffBgHover Color
	// Middle-size track height (width follows the design system)
	//
	// Deprecated: use Components.Switch.TrackHeight (the field remains the source of the derived default).
	SwitchHeight float32
	// Deprecated: use Components.Switch.TrackMinWidth (the field remains the source of the derived default).
	SwitchWidth float32
	// Deprecated: use Components.Checkbox.ControlInteractiveSize / Components.Radio.RadioSize (the field remains the source of the derived default).
	CheckboxSize   float32
	CheckboxRadius float32
	// Data-entry surfaces (shad2a). ControlBg fills inputs, selects, outline
	// buttons and unchecked boxes (shadcn dark: input/30); ControlBgHover is
	// the outline button under the pointer. ButtonDangerBg is the solid
	// danger button (shadcn dark: destructive/60). SwitchHandleOff is the
	// thumb of an off switch (shadcn dark: foreground). RateStar /
	// RateStarEmpty color the Rate stars.
	ControlBg, ControlBgHover Color
	// Deprecated: use Components.Button.DangerBg (the field remains the source of the derived default).
	ButtonDangerBg Color
	// Deprecated: use Components.Switch.HandleOffBg (the field remains the source of the derived default).
	SwitchHandleOff Color
	// Deprecated: use Components.Rate.StarColor and Components.Rate.StarBg (the field remains the source of the derived default).
	RateStar, RateStarEmpty Color

	// Deprecated: use Components.Slider.RailSize (the field remains the source of the derived default).
	SliderRailHeight float32

	// Cards and overlays.
	CardBorder Color // card outline (zero: none)
	CardShadow []paint.Shadow
	// Card corner radius (zero: the design system default, RadiusLG / Element Radius)
	//
	// Deprecated: use Components.Card.Radius (the field remains the source of the derived default).
	CardRadius  float32
	PopupBorder Color // dropdown / popover / select panel outline (zero: none)
	// Dialog outline (zero: none)
	//
	// Deprecated: use Components.Modal.BorderColor / Components.Drawer.BorderColor (the field remains the source of the derived default).
	ModalBorder   Color
	TooltipShadow []paint.Shadow // small floating tips (FloatButton, Slider)
	// Deprecated: use Components.Table.HeaderColor (the field remains the source of the derived default).
	TableHeaderText Color
	// Deprecated: use Components.Table.HeaderFontWeight (the field remains the source of the derived default).
	TableHeaderWeight int
	// Sorter / filter icon when inactive.
	//
	// Deprecated: use Components.Table.SorterIdleColor (the field remains the source of the derived default).
	TableSorterIdle Color
	// Opacity of the shadow beside fixed columns.
	//
	// Deprecated: use Components.Table.FixedShadowAlpha (the field remains the source of the derived default).
	TableFixedShadowAlpha float64
	// Body cells of the sorted column (zero: no column tint)
	//
	// Deprecated: use Components.Table.BodySortBg (the field remains the source of the derived default).
	TableSortedBg Color
	// Header cell of the sorted / hovered sortable column (zero: none)
	//
	// Deprecated: use Components.Table.HeaderSortActiveBg (the field remains the source of the derived default).
	TableHeaderActiveBg Color

	// Small surfaces whose colors used to be picked per dark/light.

	// Badge status="default" dot.
	//
	// Deprecated: use Components.Badge.StatusDefaultColor (the field remains the source of the derived default).
	StatusDefault Color
	// Deprecated: use Components.Avatar.Bg (the field remains the source of the derived default).
	AvatarBg Color
	// Deprecated: use Components.Avatar.Color (the field remains the source of the derived default).
	AvatarText Color
	// Deprecated: use Components.Skeleton.GradientFromColor (the field remains the source of the derived default).
	SkeletonBg Color
	// Deprecated: use Components.Skeleton.GradientToColor (the field remains the source of the derived default).
	SkeletonHighlight Color
	// Picture placeholder icon of Skeleton.Image.
	//
	// Deprecated: use Components.Skeleton.IconColor (the field remains the source of the derived default).
	SkeletonIcon Color
	// Input addon / Compact addon background.
	//
	// Deprecated: use Components.Input.AddonBg (the field remains the source of the derived default).
	AddonBg Color
	// Deprecated: use Components.Input.AddonColor (the field remains the source of the derived default).
	AddonText Color
	// Text over MarkBg (zero: keep the span color)
	//
	// Deprecated: use Components.Typography.MarkColor (the field remains the source of the derived default).
	MarkText Color
	// Typography Mark highlight.
	//
	// Deprecated: use Components.Typography.MarkBg (the field remains the source of the derived default).
	MarkBg       Color
	Illustration Illustration

	// LineHeights maps a font size to its line height in px; nil uses
	// Ant's rule (size + 8, rounded to even). shadcn uses Tailwind's
	// leading (14 → 20).
	LineHeights map[float32]float32

	// Shadcn holds the shadcn/ui CSS variable values and the options the
	// theme was built with; nil for the other design systems.
	Shadcn *ShadcnVars

	// Geometry.
	Radius, RadiusSM, RadiusLG, RadiusXS float32
	ControlHeight                        float32
	ControlHeightSM                      float32
	ControlHeightLG                      float32
	ControlPaddingH                      float32 // horizontal padding inside inputs/buttons
	ControlPaddingHSM                    float32
	LineWidth                            float32

	// Typography (logical px).
	FontSize, FontSizeSM, FontSizeLG, FontSizeXL float32
	Heading                                      [5]float32 // h1..h5
	LineHeight                                   float32    // ratio: 22/14 for Ant
	FontWeightStrong                             int

	// Spacing scale.
	PaddingXXS, PaddingXS, PaddingSM, Padding, PaddingMD, PaddingLG, PaddingXL float32
	MarginXS, MarginSM, Margin, MarginMD, MarginLG, MarginXL                   float32

	// Shadows.
	Shadow          []paint.Shadow // boxShadow: dropdowns, popovers
	ShadowSecondary []paint.Shadow // modals, message, notification
	ShadowTertiary  []paint.Shadow // cards
	ButtonShadow    []paint.Shadow
	InputShadow     []paint.Shadow // inputs and selects (shadcn shadow-xs)
	// Deprecated: use Components.Button.PrimaryShadow (the field remains the source of the derived default).
	PrimaryShadow []paint.Shadow
	// Deprecated: use Components.Button.DangerShadow (the field remains the source of the derived default).
	DangerShadow []paint.Shadow

	// Motion.
	DurationFast, DurationMid, DurationSlow time.Duration

	// Layout chrome (ProLayout).

	// Deprecated: use Components.Layout.ProHeaderBg (the field remains the source of the derived default).
	HeaderBg Color
	// Deprecated: use Components.Layout.ProHeaderHeight (the field remains the source of the derived default).
	HeaderHeight float32
	SiderBg      Color // dark sider
	SiderLightBg Color
	// Deprecated: use Components.Layout.SiderWidth (the field remains the source of the derived default).
	SiderWidth      float32
	SiderCollapsed  float32
	SiderText       Color // menu text on dark sider
	SiderTextActive Color
	// Deprecated: use Components.Menu.DarkSubMenuItemBg / Components.Layout.TriggerBg (the field remains the source of the derived default).
	SiderSubmenuBg   Color
	SiderItemHoverBg Color
	// Deprecated: use Components.Menu.ItemHeight (the field remains the source of the derived default).
	MenuItemHeight float32
	// Deprecated: use Components.Table.HeaderBg (the field remains the source of the derived default).
	TableHeaderBg Color
	// Deprecated: use Components.Table.RowHoverBg (the field remains the source of the derived default).
	TableRowHoverBg Color
	// Deprecated: use Components.Table.CellPaddingBlock (the field remains the source of the derived default).
	TablePaddingV float32
	// Deprecated: use Components.Table.CellPaddingInline (the field remains the source of the derived default).
	TablePaddingH float32

	// Segmented control.
	SegmentedBg         Color // track behind the items
	SegmentedSelectedBg Color // sliding thumb under the selected item

	// Tabs.
	// Header strip of a border-card Tabs (Element fill-color-light)
	//
	// Deprecated: use Components.Tabs.HeaderBg (the field remains the source of the derived default).
	TabsHeaderBg Color

	// Components holds the resolved component tokens (components.go).
	Components Components
	comp       *compState // overrides and per-component Alias themes (For)
}

// Option tweaks a theme while it is being constructed.
type Option func(*options)

type options struct {
	dark       bool
	primary    *Color
	compact    bool
	components Components
}

// Dark selects the dark algorithm.
func Dark() Option { return func(o *options) { o.dark = true } }

// WithPrimary overrides the brand color; all derived shades follow.
func WithPrimary(c Color) Option { return func(o *options) { o.primary = &c } }

// Compact reduces control heights (Ant's compact algorithm, simplified).
func Compact() Option { return func(o *options) { o.compact = true } }

func collect(opts []Option) options {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return o
}

func sh(x, y, blur, spread float32, c Color) paint.Shadow {
	return paint.Shadow{X: x, Y: y, Blur: blur, Spread: spread, Color: c}
}

// AntDesign returns the Ant Design v5 token set.
func AntDesign(opts ...Option) *Theme {
	o := collect(opts)
	primary := Hex("#1677ff")
	if o.primary != nil {
		primary = *o.primary
	}
	t := &Theme{Name: "antd", Flavor: Ant, Dark: o.dark}
	bgDark := Hex("#141414")
	t.Primary = AntPalette(primary, o.dark, bgDark)
	t.Success = AntPalette(Hex("#52c41a"), o.dark, bgDark)
	t.Warning = AntPalette(Hex("#faad14"), o.dark, bgDark)
	t.Error = AntPalette(Hex("#ff4d4f"), o.dark, bgDark)
	t.Info = t.Primary

	t.Radius, t.RadiusSM, t.RadiusLG, t.RadiusXS = 6, 4, 8, 2
	t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 32, 24, 40
	if o.compact {
		t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 28, 20, 36
	}
	t.ControlPaddingH, t.ControlPaddingHSM = 11, 7
	t.LineWidth = 1
	t.FontSize, t.FontSizeSM, t.FontSizeLG, t.FontSizeXL = 14, 12, 16, 20
	t.Heading = [5]float32{38, 30, 24, 20, 16}
	t.LineHeight = 22.0 / 14.0
	t.FontWeightStrong = 600
	t.PaddingXXS, t.PaddingXS, t.PaddingSM, t.Padding, t.PaddingMD, t.PaddingLG, t.PaddingXL = 4, 8, 12, 16, 20, 24, 32
	t.MarginXS, t.MarginSM, t.Margin, t.MarginMD, t.MarginLG, t.MarginXL = 8, 12, 16, 20, 24, 32
	t.DurationFast, t.DurationMid, t.DurationSlow = 100*time.Millisecond, 200*time.Millisecond, 300*time.Millisecond
	t.HeaderHeight, t.SiderWidth, t.SiderCollapsed, t.MenuItemHeight = 56, 208, 64, 40
	t.TablePaddingV, t.TablePaddingH = 16, 16
	t.ControlOutlineWidth = 2

	if !o.dark {
		t.Text = RGBA(0, 0, 0, .88)
		t.TextHeading = t.Text
		t.TextSecondary = RGBA(0, 0, 0, .65)
		t.TextTertiary = RGBA(0, 0, 0, .45)
		t.TextQuaternary = RGBA(0, 0, 0, .25)
		t.TextDisabled = RGBA(0, 0, 0, .25)
		t.Border = Hex("#d9d9d9")
		t.BorderSecondary = Hex("#f0f0f0")
		t.Split = RGBA(5, 5, 5, .06)
		t.Fill = RGBA(0, 0, 0, .15)
		t.FillSecondary = RGBA(0, 0, 0, .06)
		t.FillTertiary = RGBA(0, 0, 0, .04)
		t.FillQuaternary = RGBA(0, 0, 0, .02)
		t.FillAlter = RGBA(0, 0, 0, .02)
		t.BgContainer = Hex("#ffffff")
		t.BgElevated = Hex("#ffffff")
		t.BgLayout = Hex("#f5f5f5")
		t.BgSpotlight = RGBA(0, 0, 0, .85)
		t.BgMask = RGBA(0, 0, 0, .45)
		t.BgContainerDisabled = RGBA(0, 0, 0, .04)
		t.BgTextHover = RGBA(0, 0, 0, .06)
		t.BgTextActive = RGBA(0, 0, 0, .15)
		t.ItemHoverBg = RGBA(0, 0, 0, .04)
		t.ControlOutline = RGBA(5, 145, 255, .1)
		t.Shadow = []paint.Shadow{sh(0, 6, 16, 0, RGBA(0, 0, 0, .08)), sh(0, 3, 6, -4, RGBA(0, 0, 0, .12)), sh(0, 9, 28, 8, RGBA(0, 0, 0, .05))}
		t.ShadowSecondary = t.Shadow
		t.ShadowTertiary = []paint.Shadow{sh(0, 1, 2, 0, RGBA(0, 0, 0, .03)), sh(0, 1, 6, -1, RGBA(0, 0, 0, .02)), sh(0, 2, 4, 0, RGBA(0, 0, 0, .02))}
		t.ButtonShadow = []paint.Shadow{sh(0, 2, 0, 0, RGBA(0, 0, 0, .02))}
		t.PrimaryShadow = []paint.Shadow{sh(0, 2, 0, 0, RGBA(5, 145, 255, .1))}
		t.DangerShadow = []paint.Shadow{sh(0, 2, 0, 0, RGBA(255, 38, 5, .06))}
		t.HeaderBg = Hex("#ffffff")
		t.SiderLightBg = Hex("#ffffff")
		t.TableHeaderBg = Hex("#fafafa")
		t.TableRowHoverBg = Hex("#fafafa")
		t.TextOnPrimary = Hex("#ffffff")
	} else {
		t.Text = RGBA(255, 255, 255, .85)
		t.TextHeading = t.Text
		t.TextSecondary = RGBA(255, 255, 255, .65)
		t.TextTertiary = RGBA(255, 255, 255, .45)
		t.TextQuaternary = RGBA(255, 255, 255, .25)
		t.TextDisabled = RGBA(255, 255, 255, .25)
		t.Border = Hex("#424242")
		t.BorderSecondary = Hex("#303030")
		t.Split = RGBA(253, 253, 253, .12)
		t.Fill = RGBA(255, 255, 255, .18)
		t.FillSecondary = RGBA(255, 255, 255, .12)
		t.FillTertiary = RGBA(255, 255, 255, .08)
		t.FillQuaternary = RGBA(255, 255, 255, .04)
		t.FillAlter = RGBA(255, 255, 255, .04)
		t.BgContainer = Hex("#141414")
		t.BgElevated = Hex("#1f1f1f")
		t.BgLayout = Hex("#000000")
		t.BgSpotlight = Hex("#424242")
		t.BgMask = RGBA(0, 0, 0, .45)
		t.BgContainerDisabled = RGBA(255, 255, 255, .08)
		t.BgTextHover = RGBA(255, 255, 255, .12)
		t.BgTextActive = RGBA(255, 255, 255, .18)
		t.ItemHoverBg = RGBA(255, 255, 255, .08)
		t.ControlOutline = RGBA(0, 60, 180, .15)
		t.Shadow = []paint.Shadow{sh(0, 6, 16, 0, RGBA(0, 0, 0, .32)), sh(0, 3, 6, -4, RGBA(0, 0, 0, .48)), sh(0, 9, 28, 8, RGBA(0, 0, 0, .2))}
		t.ShadowSecondary = t.Shadow
		t.ShadowTertiary = []paint.Shadow{sh(0, 1, 2, 0, RGBA(0, 0, 0, .12))}
		t.ButtonShadow = []paint.Shadow{sh(0, 2, 0, 0, RGBA(0, 0, 0, .02))}
		t.PrimaryShadow = []paint.Shadow{sh(0, 2, 0, 0, RGBA(0, 0, 0, .02))}
		t.DangerShadow = t.PrimaryShadow
		t.HeaderBg = Hex("#141414")
		t.SiderLightBg = Hex("#141414")
		t.TableHeaderBg = Hex("#1d1d1d")
		t.TableRowHoverBg = Hex("#262626")
		t.TextOnPrimary = Hex("#ffffff")
	}
	t.ItemSelectedBg = t.Primary.Bg
	t.ItemSelected = t.Primary.Base
	t.BorderHover = t.Primary.Hover
	t.TextLink, t.TextLinkHover, t.TextLinkActive = t.Primary.Base, t.Primary.Shades[4], t.Primary.Active
	t.FocusRing = t.Primary.Border
	t.ControlOutlineError = RGBA(255, 38, 5, .06)
	t.ControlOutlineWarning = RGBA(255, 215, 5, .1)
	t.ControlFocusBorder = t.Primary.Base
	t.FocusOutline, t.FocusRingWidth, t.FocusRingOffset = t.FocusRing, 4, 1
	// a11y: Ant's colorPrimaryBorder ring is ~1.7:1 on the page; lift it
	// towards the brand color until it reaches WCAG 1.4.11 (3:1).
	t.FocusRing = EnsureContrast(t.FocusRing, t.BgContainer, 3, t.Primary.Base, t.Primary.Active, t.Text)
	t.FocusOutline = t.FocusRing
	t.TextOnSpotlight = Hex("#ffffff")
	t.TextOnSolid = Hex("#ffffff")
	t.ButtonFontWeight = 400
	t.ButtonPaddingH, t.ButtonPaddingHSM, t.ButtonPaddingHLG = 15, 7, 15
	t.SecondaryBg, t.SecondaryBgHover, t.SecondaryText = t.FillTertiary, t.FillSecondary, t.Text
	t.SwitchOffBg, t.SwitchOffBgHover = t.TextQuaternary, t.TextTertiary
	t.SwitchHeight, t.SwitchWidth = 22, 44
	t.LabelFontWeight, t.SwitchHandle = 400, Hex("#ffffff")
	t.CheckboxSize, t.CheckboxRadius = 16, 4
	t.ControlBg, t.ControlBgHover = t.BgContainer, t.BgContainer
	t.ButtonDangerBg, t.SwitchHandleOff = t.Error.Base, t.SwitchHandle
	t.RateStar, t.RateStarEmpty = Hex("#fadb14"), t.FillSecondary
	t.SliderRailHeight = 4
	t.CardShadow = nil
	t.TableHeaderText, t.TableHeaderWeight = t.TextHeading, t.FontWeightStrong
	t.TooltipShadow = t.Shadow
	t.TableSorterIdle, t.TableFixedShadowAlpha = RGBA(0, 0, 0, .29), .16
	t.TableSortedBg, t.TableHeaderActiveBg = t.FillQuaternary, t.FillTertiary
	t.StatusDefault = t.Fill
	t.AvatarBg, t.AvatarText = t.TextQuaternary, Hex("#ffffff")
	t.SkeletonBg, t.SkeletonHighlight = RGBA(0, 0, 0, .06), RGBA(0, 0, 0, .15)
	t.SkeletonIcon = t.Fill
	t.AddonBg, t.AddonText = t.FillAlter, t.Text
	t.MarkBg = Hex("#ffe58f") // gold-3
	if o.dark {
		t.TableSorterIdle, t.TableFixedShadowAlpha = t.TextQuaternary, .6
		t.StatusDefault = t.TextQuaternary
		t.SkeletonBg, t.SkeletonHighlight = RGBA(255, 255, 255, .12), RGBA(255, 255, 255, .2)
		t.AddonBg = t.FillQuaternary
		// a11y: the gold-3 highlight stays light in dark mode, so the
		// page's light text would read ~1.2:1 on it; use dark ink.
		t.MarkText = Hex("#000000e0")
	}
	t.Illustration = defaultIllustration(o.dark, t.Primary, t.Border, t.BorderSecondary)
	t.SiderBg = Hex("#001529")
	t.SiderSubmenuBg = Hex("#000c17")
	t.SiderText = RGBA(255, 255, 255, .65)
	t.SiderTextActive = Hex("#ffffff")
	t.SiderItemHoverBg = RGBA(255, 255, 255, .08)
	if o.dark {
		t.SiderBg = Hex("#141414")
		t.SiderSubmenuBg = Hex("#000000")
	}
	// Ant: trackBg = colorBgLayout, itemSelectedBg = colorBgElevated. Pages
	// often repurpose the light layout color (white), so the light track
	// comes from the fill scale instead.
	t.SegmentedBg = Over(t.FillTertiary, t.BgContainer)
	if o.dark {
		t.SegmentedBg = t.BgLayout
	}
	t.SegmentedSelectedBg = t.BgElevated
	t.TabsHeaderBg = t.FillAlter
	t.deriveComponents(o.components)
	return t
}

// ElementPlus returns the Element Plus token set.
func ElementPlus(opts ...Option) *Theme {
	o := collect(opts)
	primary := Hex("#409eff")
	if o.primary != nil {
		primary = *o.primary
	}
	t := &Theme{Name: "element-plus", Flavor: Element, Dark: o.dark}
	bgDark := Hex("#141414")
	t.Primary = ElementPalette(primary, o.dark, bgDark)
	t.Success = ElementPalette(Hex("#67c23a"), o.dark, bgDark)
	t.Warning = ElementPalette(Hex("#e6a23c"), o.dark, bgDark)
	t.Error = ElementPalette(Hex("#f56c6c"), o.dark, bgDark)
	t.Info = ElementPalette(Hex("#909399"), o.dark, bgDark)

	t.Radius, t.RadiusSM, t.RadiusLG, t.RadiusXS = 4, 2, 8, 2
	t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 32, 24, 40
	if o.compact {
		t.ControlHeight, t.ControlHeightSM, t.ControlHeightLG = 28, 20, 36
	}
	t.ControlPaddingH, t.ControlPaddingHSM = 11, 7
	t.LineWidth = 1
	t.FontSize, t.FontSizeSM, t.FontSizeLG, t.FontSizeXL = 14, 12, 16, 20
	t.Heading = [5]float32{32, 28, 24, 20, 16}
	t.LineHeight = 22.0 / 14.0
	t.FontWeightStrong = 500
	t.PaddingXXS, t.PaddingXS, t.PaddingSM, t.Padding, t.PaddingMD, t.PaddingLG, t.PaddingXL = 4, 8, 12, 16, 20, 24, 32
	t.MarginXS, t.MarginSM, t.Margin, t.MarginMD, t.MarginLG, t.MarginXL = 8, 12, 16, 20, 24, 32
	t.DurationFast, t.DurationMid, t.DurationSlow = 100*time.Millisecond, 200*time.Millisecond, 300*time.Millisecond
	t.HeaderHeight, t.SiderWidth, t.SiderCollapsed, t.MenuItemHeight = 60, 210, 64, 56
	t.TablePaddingV, t.TablePaddingH = 12, 12
	t.ControlOutlineWidth = 0

	if !o.dark {
		t.Text = Hex("#303133")
		t.TextHeading = Hex("#303133")
		t.TextSecondary = Hex("#606266")
		t.TextTertiary = Hex("#909399")
		t.TextQuaternary = Hex("#a8abb2")
		t.TextDisabled = Hex("#a8abb2")
		t.Border = Hex("#dcdfe6")
		t.BorderSecondary = Hex("#ebeef5")
		t.Split = Hex("#e4e7ed")
		t.Fill = Hex("#f0f2f5")
		t.FillSecondary = Hex("#f0f2f5")
		t.FillTertiary = Hex("#f5f7fa")
		t.FillQuaternary = Hex("#fafafa")
		t.FillAlter = Hex("#fafafa")
		t.BgContainer = Hex("#ffffff")
		t.BgElevated = Hex("#ffffff")
		t.BgLayout = Hex("#f2f3f5")
		t.BgSpotlight = Hex("#303133")
		t.BgMask = RGBA(0, 0, 0, .5)
		t.BgContainerDisabled = Hex("#f5f7fa")
		t.BgTextHover = Hex("#f5f7fa")
		t.BgTextActive = Hex("#ebeef5")
		t.ItemHoverBg = Hex("#f5f7fa")
		t.Shadow = []paint.Shadow{sh(0, 0, 12, 0, RGBA(0, 0, 0, .12))}
		t.ShadowSecondary = []paint.Shadow{sh(0, 12, 32, 4, RGBA(0, 0, 0, .04)), sh(0, 8, 20, 0, RGBA(0, 0, 0, .08))}
		t.ShadowTertiary = []paint.Shadow{sh(0, 0, 12, 0, RGBA(0, 0, 0, .06))}
		t.HeaderBg = Hex("#ffffff")
		t.SiderLightBg = Hex("#ffffff")
		t.TableHeaderBg = Hex("#ffffff")
		t.TableRowHoverBg = Hex("#f5f7fa")
		t.SiderBg = Hex("#304156")
		t.SiderSubmenuBg = Hex("#1f2d3d")
		t.SiderText = Hex("#bfcbd9")
		t.SiderTextActive = Hex("#409eff")
		t.SiderItemHoverBg = Hex("#263445")
	} else {
		t.Text = Hex("#e5eaf3")
		t.TextHeading = Hex("#e5eaf3")
		t.TextSecondary = Hex("#cfd3dc")
		t.TextTertiary = Hex("#a3a6ad")
		t.TextQuaternary = Hex("#8d9095")
		t.TextDisabled = Hex("#6c6e72")
		t.Border = Hex("#4c4d4f")
		t.BorderSecondary = Hex("#363637")
		t.Split = Hex("#414243")
		t.Fill = Hex("#303030")
		t.FillSecondary = Hex("#262727")
		t.FillTertiary = Hex("#1d1d1d")
		t.FillQuaternary = Hex("#191919")
		t.FillAlter = Hex("#191919")
		t.BgContainer = Hex("#141414")
		t.BgElevated = Hex("#1d1e1f")
		t.BgLayout = Hex("#0a0a0a")
		t.BgSpotlight = Hex("#e5eaf3")
		t.BgMask = RGBA(0, 0, 0, .8)
		t.BgContainerDisabled = Hex("#262727")
		t.BgTextHover = Hex("#262727")
		t.BgTextActive = Hex("#303030")
		t.ItemHoverBg = Hex("#262727")
		t.Shadow = []paint.Shadow{sh(0, 0, 12, 0, RGBA(0, 0, 0, .72))}
		t.ShadowSecondary = []paint.Shadow{sh(0, 12, 32, 4, RGBA(0, 0, 0, .36)), sh(0, 8, 20, 0, RGBA(0, 0, 0, .72))}
		t.ShadowTertiary = []paint.Shadow{sh(0, 0, 12, 0, RGBA(0, 0, 0, .36))}
		t.HeaderBg = Hex("#141414")
		t.SiderLightBg = Hex("#141414")
		t.TableHeaderBg = Hex("#141414")
		t.TableRowHoverBg = Hex("#262727")
		t.SiderBg = Hex("#141414")
		t.SiderSubmenuBg = Hex("#0a0a0a")
		t.SiderText = Hex("#cfd3dc")
		t.SiderTextActive = t.Primary.Base
		t.SiderItemHoverBg = Hex("#262727")
	}
	t.TextOnPrimary = Hex("#ffffff")
	t.BorderHover = Hex("#c0c4cc")
	if o.dark {
		t.BorderHover = Hex("#6c6e72")
	}
	t.ItemSelectedBg = t.Primary.Bg
	t.ItemSelected = t.Primary.Base
	t.TextLink, t.TextLinkHover, t.TextLinkActive = t.Primary.Base, t.Primary.Hover, t.Primary.Active
	t.ControlOutline = Color{}
	t.FocusRing = t.Primary.Hover
	t.ControlFocusBorder = t.Primary.Base
	t.FocusOutline, t.FocusRingWidth, t.FocusRingOffset = t.Primary.Shades[3], 2, 0
	// a11y: Element's light-3 ring is ~1.6-2:1; lift both focus colors to
	// 3:1 (WCAG 1.4.11) towards the darker brand shade.
	t.FocusRing = EnsureContrast(t.FocusRing, t.BgContainer, 3, t.Primary.Active, t.Text)
	t.FocusOutline = EnsureContrast(t.FocusOutline, t.BgContainer, 3, t.Primary.Active, t.Text)
	// a11y: the focused-input border is the text inputs' focus indicator
	// (Element has no halo). #409eff is 2.78:1 on white; lift it to 3:1 the
	// same way (one 10% step towards dark-2: #3c94f0, 3.14:1).
	// Dark mode (6.5:1) is unchanged.
	t.ControlFocusBorder = EnsureContrast(t.ControlFocusBorder, t.BgContainer, 3, t.Primary.Active, t.Text)
	// Element's tooltip ("dark" effect) inverts with the theme: page color
	// on BgSpotlight.
	t.TextOnSpotlight = t.BgContainer
	t.TextOnSolid = Hex("#ffffff")
	t.ButtonFontWeight = 500
	t.ButtonPaddingH, t.ButtonPaddingHSM, t.ButtonPaddingHLG = 15, 7, 15
	t.SecondaryBg, t.SecondaryBgHover, t.SecondaryText = t.Info.Bg, t.Info.BgHover, t.Info.Base
	t.SwitchOffBg, t.SwitchOffBgHover = t.Border, t.Border
	t.SwitchHeight, t.SwitchWidth = 20, 40
	t.LabelFontWeight, t.SwitchHandle = 500, Hex("#ffffff")
	t.CheckboxSize, t.CheckboxRadius = 14, 2
	t.ControlBg, t.ControlBgHover = t.BgContainer, t.BgContainer
	t.ButtonDangerBg, t.SwitchHandleOff = t.Error.Base, t.SwitchHandle
	// Element rate: void color between the border and its darker tone.
	t.RateStar, t.RateStarEmpty = Hex("#f7ba2a"), Lerp(t.Border, t.BorderHover, 0.5)
	t.SliderRailHeight = 6
	t.TableHeaderText, t.TableHeaderWeight = t.TextTertiary, 600
	t.TableSorterIdle, t.TableFixedShadowAlpha = t.TextQuaternary, .2
	t.StatusDefault = t.TextQuaternary
	t.AvatarBg, t.AvatarText = Hex("#c0c4cc"), Hex("#ffffff")
	t.SkeletonBg, t.SkeletonHighlight = Hex("#f0f2f5"), Hex("#e6e8eb")
	t.SkeletonIcon = Hex("#dcdde0")
	t.AddonBg, t.AddonText = t.FillTertiary, t.TextTertiary
	t.MarkBg = Hex("#ffe58f")
	t.PopupBorder = t.Split
	if o.dark {
		t.TooltipShadow = t.Shadow
		t.TableFixedShadowAlpha = .6
		t.AvatarBg = Hex("#6c6e72")
		t.SkeletonBg, t.SkeletonHighlight = Hex("#363637"), Hex("#4c4d4f")
		t.SkeletonIcon = t.TextQuaternary
		t.MarkText = Hex("#000000e0") // dark ink on the light highlight (a11y)
		t.PopupBorder = t.Border
	}
	t.Illustration = defaultIllustration(o.dark, t.Primary, t.Border, t.BorderSecondary)
	t.SegmentedBg = t.FillTertiary
	t.SegmentedSelectedBg = t.Primary.Base
	t.TabsHeaderBg = t.BgContainerDisabled // fill-color-light: #f5f7fa / #262727
	t.deriveComponents(o.components)
	return t
}

// ColorOf returns the palette for a status name ("primary", "success",
// "warning", "error"/"danger", "info"); unknown names give Primary.
func (t *Theme) ColorOf(status string) Palette {
	switch status {
	case "success":
		return t.Success
	case "warning":
		return t.Warning
	case "error", "danger":
		return t.Error
	case "info":
		return t.Info
	}
	return t.Primary
}

// LineHeightFor returns the theme-appropriate line height in px for a font size.
func (t *Theme) LineHeightFor(size float32) float32 {
	if t.LineHeights != nil {
		if h, ok := t.LineHeights[size]; ok {
			return h
		}
		return float32(int(size*t.LineHeight + 0.5))
	}
	if size == t.FontSize {
		return 22
	}
	// Ant: lineHeight = (fontSize + 8) / fontSize, rounded to even px.
	switch size {
	case 12:
		return 20
	case 16:
		return 24
	case 20:
		return 28
	}
	return float32(int(size*t.LineHeight + 0.5))
}
