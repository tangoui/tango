package theme

import (
	"fmt"
	"math"
	"strings"
)

// Contrast checking (WCAG 2.2, success criteria 1.4.3 and 1.4.11).

// RelativeLuminance is the WCAG relative luminance of an opaque color.
func RelativeLuminance(c Color) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// ContrastRatio returns the WCAG contrast ratio (1..21) of fg drawn on bg.
// A translucent fg is composited over bg first; a translucent bg is
// composited over white (light themes) — pass an opaque bg for dark ones.
func ContrastRatio(fg, bg Color) float64 {
	if bg.A < 255 {
		bg = Over(bg, Color{R: 255, G: 255, B: 255, A: 255})
	}
	if fg.A < 255 {
		fg = Over(fg, bg)
	}
	a, b := RelativeLuminance(fg), RelativeLuminance(bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

// ContrastKind is the WCAG requirement a token pair is held to.
//
// Append-only: never reorder or insert; the zero value stays ContrastText.
type ContrastKind uint8

const (
	ContrastText      ContrastKind = iota // body text: 4.5:1 (AA)
	ContrastLargeText                     // ≥ 18.66px bold or 24px: 3:1
	ContrastNonText                       // component boundaries, focus indicators, states: 3:1
	// ContrastExempt pairs are measured and reported but not required:
	// inactive (disabled) controls and their text (WCAG 1.4.3 "text or
	// images of text that are part of an inactive user interface
	// component", 1.4.11 "inactive components"), purely decorative lines
	// that do not identify a control (dividers, card and table rules: the
	// content inside identifies itself) and logotypes (1.4.3). Logos are
	// app images, not theme tokens, so no pair checks them.
	ContrastExempt
)

// Min returns the ratio the kind requires (0 for exempt pairs).
func (k ContrastKind) Min() float64 {
	switch k {
	case ContrastText:
		return 4.5
	case ContrastLargeText, ContrastNonText:
		return 3
	}
	return 0
}

func (k ContrastKind) String() string {
	return [...]string{"text", "large-text", "non-text", "exempt"}[k]
}

// ContrastStatus classifies a measured pair for reports and audit gates.
//
// Append-only: never reorder or insert; the zero value stays ContrastStatusPass.
type ContrastStatus uint8

const (
	// ContrastStatusPass: the pair meets its minimum.
	ContrastStatusPass ContrastStatus = iota
	// ContrastStatusFail: a required pair (text; a non-text element that
	// identifies a control, its state or keyboard focus) below its minimum
	// whose colors are not the published values of the imitated design
	// system. Audits fail on these.
	ContrastStatusFail
	// ContrastStatusDesignDefault: a required pair below its minimum whose
	// colors are exactly the published values of the design system the
	// theme imitates (Spec names the variable). It is reported with its
	// ratio and the HighContrast replacement but is not a failure: changing
	// it would make the default theme stop looking like Ant Design, Element
	// Plus or shadcn/ui. Apps that need WCAG AA use HighContrast.
	ContrastStatusDesignDefault
	// ContrastStatusExempt: a ContrastExempt pair, reported only.
	ContrastStatusExempt
)

func (s ContrastStatus) String() string {
	return [...]string{"ok", "FAIL", "DesignDefault", "exempt"}[s]
}

// ContrastCheck is one token pair of a theme and its measured ratio.
type ContrastCheck struct {
	Theme string // theme name ("antd", "antd-dark" …)
	Token string // foreground token, e.g. "TextSecondary"
	On    string // background token, e.g. "BgContainer"
	Use   string // where the pair appears
	FG    Color
	BG    Color
	Kind  ContrastKind
	Ratio float64

	Status ContrastStatus
	Spec   string // published design-system variable (DesignDefault only)
	// SuggestFG/SuggestBG/SuggestRatio are the colors HighContrast(theme)
	// uses for this pair; set for required pairs below their minimum.
	SuggestFG, SuggestBG Color
	SuggestRatio         float64
}

// Pass reports whether the pair meets its requirement.
func (c ContrastCheck) Pass() bool { return c.Ratio+1e-9 >= c.Kind.Min() }

func (c ContrastCheck) String() string {
	min := ""
	if m := c.Kind.Min(); m > 0 {
		min = fmt.Sprintf(" (min %.1f)", m)
	}
	s := fmt.Sprintf("%-13s %-20s %-18s on %-19s %5.2f:1 %s%s — %s", c.Status, c.Theme, c.Token, c.On, c.Ratio, c.Kind, min, c.Use)
	if c.Status == ContrastStatusFail || c.Status == ContrastStatusDesignDefault {
		s += fmt.Sprintf(" | %s on %s", HexString(c.FG), HexString(c.BG))
		if c.Spec != "" {
			s += " [" + c.Spec + "]"
		}
		if c.SuggestRatio > 0 {
			s += fmt.Sprintf(" → HighContrast %s on %s %.2f:1", HexString(c.SuggestFG), HexString(c.SuggestBG), c.SuggestRatio)
		}
	}
	return s
}

// themeLabel names a theme for reports ("antd", "element-plus-dark",
// "shadcn-dark-hc").
func themeLabel(t *Theme) string {
	s := t.Flavor.String()
	if t.Dark {
		s += "-dark"
	}
	if t.HighContrast {
		s += "-hc"
	}
	return s
}

// designSpec names, per design system, the published variable a checked
// token takes its color from. Only these tokens can be DesignDefault, and
// only while their colors equal the stock theme's (a WithPrimary brand
// color or an app override is the app's choice, not the design system's,
// and fails). Keyboard focus indicators (ControlFocusBorder, FocusRing,
// FocusOutline) are deliberately absent: WCAG 2.4.7 / 1.4.11 focus
// visibility is not traded for fidelity, and the built-in themes already
// lift them to 3:1 (theme.go, shadcn.go). Body and secondary text are absent
// because every design system's values pass.
//
// Review of the 37 pairs the six stock themes failed before this
// classification (ratios on BgContainer unless noted):
//
//	DesignDefault, text 4.5:1 (WCAG 1.4.3 applies, the value is the spec's):
//	  TextTertiary    antd 3.36, element-plus 3.08           help text / --el-text-color-secondary
//	  TextQuaternary  antd 1.84, antd-dark 2.25, element 2.30 placeholder (1.4.3 covers placeholders)
//	  TextLink        antd 4.10, antd-dark 3.55, element 2.78 colorLink / el-link = brand color
//	  TextOnPrimary   antd 4.10, element 2.78, element-dark 2.78 white label on the brand fill
//	  TextOnSolid     all six (2.90–4.24)                     white label on the danger fill
//	  Error           antd 3.27, antd-dark 4.35, element 2.90, shadcn 3.76 validation text
//	  (shadcn/ui v4, 2026-11-14: the light --destructive #e7000b passes, so
//	  shadcn's TextOnSolid / Error / danger-button pairs dropped out; dark
//	  text-white on --destructive #ff6467 is 2.89 and dark --input is 1.48.)
//	  TableHeaderText element 3.08                            --el-table-header-text-color
//	DesignDefault, non-text 3:1 (WCAG 1.4.11: identifies a control or state):
//	  Border          all six (1.26–2.18)  input / select / checkbox frame; the
//	                  only cue of an empty input, so required (labelled buttons
//	                  that share it would not need it)
//	  SwitchOffBg     all six (1.26–2.25)  the off track is the switch's boundary
//	  Primary         element 2.78         checked checkbox / radio fill, active tab
//	Fail (required, not eligible), fixed in the stock themes:
//	  ControlFocusBorder element 2.78 (#409eff → #3c94f0, 3.14), shadcn 2.52
//	                  (--ring #a3a3a3 → #949494, 3.03): the focus indicator of
//	                  text inputs; lifted to 3:1 like FocusRing (see
//	                  ElementPlus, Shadcn).
//	Exempt: none of the 37 (disabled text and decorative rules pass or are
//	checked separately as ContrastExempt).
//
// Review of the data-entry, sorted-table, switch thumb, rate and mark pairs
// added later (23 more DesignDefault):
//
//	DesignDefault (same tokens and spec values as above, on new backdrops):
//	  TextQuaternary on ControlBg     antd 1.84, antd-dark 2.25, element 2.30
//	  TextOnSolid on ButtonDangerBg   antd 3.27, antd-dark 4.24, element and
//	                                  element-dark 2.90, shadcn 3.76
//	  Border on ControlBg             all six (1.24–2.18)
//	DesignDefault, non-text 3:1 (new spec variables):
//	  SwitchHandleOff on SwitchOffBg  antd 1.84, element 1.33, shadcn 1.26
//	                                  (white / --background thumb)
//	  RateStar                        antd 1.38 (#fadb14), element 1.75 (#f7ba2a)
//	  RateStarEmpty                   antd 1.14, antd-dark 1.42, element 1.52,
//	                                  element-dark 2.83
//	Fixed in the stock themes (not spec values):
//	  MarkText on MarkBg  antd-dark 1.20, element-dark 1.03: the gold-3
//	                      highlight stays light in dark mode, so MarkText is
//	                      dark ink there (13.68).
//	  RateStarEmpty shadcn 1.26, shadcn-dark 1.31 (Rate has no shadcn
//	                      counterpart): muted-foreground, which the void
//	                      outline already used (4.74 / 7.85).
//	Text / TextSecondary on ControlBg, ControlBgHover, TableSortedBg and
//	TableHeaderActiveBg pass in all six.
var designSpec = map[Flavor]map[string]string{
	Ant: {
		"TextTertiary":    "colorTextTertiary",
		"TextQuaternary":  "colorTextPlaceholder",
		"TextLink":        "colorLink",
		"TextOnPrimary":   "colorTextLightSolid on colorPrimary",
		"TextOnSolid":     "colorTextLightSolid on colorError",
		"Error":           "colorError",
		"Border":          "colorBorder",
		"SwitchOffBg":     "Switch off track (colorTextQuaternary)",
		"Primary":         "colorPrimary",
		"SwitchHandleOff": "Switch handle (colorWhite) on the off track",
		"RateStar":        "Rate starColor (yellow-6)",
		"RateStarEmpty":   "Rate starBg (colorFillContent)",
	},
	Element: {
		"TextTertiary":    "--el-text-color-secondary",
		"TextQuaternary":  "--el-text-color-placeholder",
		"TextLink":        "--el-color-primary (el-link)",
		"TextOnPrimary":   "--el-color-white on --el-color-primary",
		"TextOnSolid":     "--el-color-white on --el-color-danger",
		"Error":           "--el-color-danger",
		"TableHeaderText": "--el-table-header-text-color",
		"Border":          "--el-border-color",
		"SwitchOffBg":     "--el-switch-off-color",
		"Primary":         "--el-color-primary",
		"SwitchHandleOff": "--el-color-white thumb on --el-switch-off-color",
		"RateStar":        "--el-rate-fill-color",
		"RateStarEmpty":   "--el-rate-void-color",
	},
	StyleShadcn: {
		"TextTertiary":    "--muted-foreground",
		"TextQuaternary":  "--muted-foreground",
		"TextOnPrimary":   "--primary-foreground on --primary",
		"TextOnSolid":     "text-white on --destructive",
		"Error":           "--destructive",
		"Border":          "--input",
		"SwitchOffBg":     "--input (switch unchecked)",
		"Primary":         "--primary",
		"SwitchHandleOff": "--background thumb on --input (switch unchecked)",
	},
}

// referenceTheme is the stock theme of t's design system (same base color
// and dark mode, no brand override): the colors DesignDefault compares with.
func referenceTheme(t *Theme) *Theme {
	var opts []Option
	if t.Dark {
		opts = append(opts, Dark())
	}
	switch t.Flavor {
	case Element:
		return ElementPlus(opts...)
	case StyleShadcn:
		o := ShadcnOptions{Dark: t.Dark}
		if t.Shadcn != nil {
			o.Base = t.Shadcn.Options.Base
		}
		return Shadcn(o)
	}
	return AntDesign(opts...)
}

// CheckContrast measures the text, placeholder, disabled, border and focus
// indicator tokens of t against the backgrounds they are drawn on and
// classifies each pair (Status), followed by the component token pairs
// (contrast struct tags) that overrides touched — every tagged pair in a
// HighContrast theme. Every pair is returned; ContrastFailures
// gives the required pairs that fail, ContrastIssues those and the design
// defaults.
func CheckContrast(t *Theme) []ContrastCheck {
	out := contrastPairs(t)
	var ref, hc []ContrastCheck
	for i := range out {
		c := &out[i]
		switch {
		case c.Kind == ContrastExempt:
			c.Status = ContrastStatusExempt
		case c.Pass():
			c.Status = ContrastStatusPass
		default:
			if ref == nil {
				ref = contrastPairs(referenceTheme(t))
				hc = contrastPairs(HighContrast(t))
			}
			c.Status = ContrastStatusFail
			if spec := designSpec[t.Flavor][c.Token]; spec != "" && !t.HighContrast && ref[i].FG == c.FG && ref[i].BG == c.BG {
				c.Status, c.Spec = ContrastStatusDesignDefault, spec
			}
			c.SuggestFG, c.SuggestBG, c.SuggestRatio = hc[i].FG, hc[i].BG, hc[i].Ratio
		}
	}
	return append(out, componentChecks(t)...)
}

// contrastPairs measures the checked pairs of t (always the same list, in
// the same order) without classifying them.
func contrastPairs(t *Theme) []ContrastCheck {
	name := themeLabel(t)
	base := t.BgContainer
	if base.A < 255 {
		base = Over(base, Color{R: 255, G: 255, B: 255, A: 255})
	}
	on := func(bg Color) Color {
		if bg.A < 255 {
			return Over(bg, base)
		}
		return bg
	}
	var out []ContrastCheck
	add := func(token, onName, use string, fg, bg Color, k ContrastKind) {
		bg = on(bg)
		out = append(out, ContrastCheck{Theme: name, Token: token, On: onName, Use: use, FG: fg, BG: bg, Kind: k, Ratio: ContrastRatio(fg, bg)})
	}
	// Text (WCAG 1.4.3): 4.5:1 for body-size text, 3:1 for headings, which
	// are at least 16px semibold … 38px (large text is ≥ 18.66px bold or
	// 24px; the 16px h5 is covered because TextHeading equals Text).
	add("Text", "BgContainer", "body text", t.Text, t.BgContainer, ContrastText)
	add("Text", "BgLayout", "page text", t.Text, t.BgLayout, ContrastText)
	add("Text", "BgElevated", "popup / dialog text", t.Text, t.BgElevated, ContrastText)
	add("TextHeading", "BgContainer", "titles (large text)", t.TextHeading, t.BgContainer, ContrastLargeText)
	add("TextSecondary", "BgContainer", "secondary text, labels", t.TextSecondary, t.BgContainer, ContrastText)
	add("TextSecondary", "BgElevated", "secondary text in popups", t.TextSecondary, t.BgElevated, ContrastText)
	add("TextTertiary", "BgContainer", "descriptions, help text", t.TextTertiary, t.BgContainer, ContrastText)
	// Placeholders are text: WCAG's 1.4.3 understanding document lists no
	// exception for them.
	add("TextQuaternary", "BgContainer", "placeholder", t.TextQuaternary, t.BgContainer, ContrastText)
	// Exempt: inactive components (1.4.3 / 1.4.11).
	add("TextDisabled", "BgContainerDisabled", "disabled control text", t.TextDisabled, t.BgContainerDisabled, ContrastExempt)
	add("TextDisabled", "BgContainer", "disabled links, menu items", t.TextDisabled, t.BgContainer, ContrastExempt)
	add("TextLink", "BgContainer", "links", t.TextLink, t.BgContainer, ContrastText)
	// Labels on solid fills are body-size text (14px regular / medium).
	add("TextOnPrimary", "Primary", "primary button label", t.TextOnPrimary, t.Primary.Base, ContrastText)
	add("TextOnSpotlight", "BgSpotlight", "tooltip text", t.TextOnSpotlight, t.BgSpotlight, ContrastText)
	add("TextOnSolid", "Error", "danger button / badge text", t.TextOnSolid, t.Error.Base, ContrastText)
	add("Error", "BgContainer", "validation messages", t.Error.Base, t.BgContainer, ContrastText)
	add("TableHeaderText", "TableHeaderBg", "table header", t.TableHeaderText, t.TableHeaderBg, ContrastText)
	// Data-entry surfaces: inputs, selects, outline buttons and unchecked
	// boxes are filled with ControlBg (shadcn dark: input/30 over the page).
	add("Text", "ControlBg", "input / select value", t.Text, t.ControlBg, ContrastText)
	add("Text", "ControlBgHover", "outline button label (hover)", t.Text, t.ControlBgHover, ContrastText)
	add("TextQuaternary", "ControlBg", "input placeholder", t.TextQuaternary, t.ControlBg, ContrastText)
	add("TextOnSolid", "ButtonDangerBg", "danger button label", t.TextOnSolid, t.ButtonDangerBg, ContrastText)
	// Sorted table columns: cells and header keep body / secondary text.
	add("Text", "TableSortedBg", "sorted column cells", t.Text, t.TableSortedBg, ContrastText)
	add("TextSecondary", "TableSortedBg", "secondary text in sorted cells", t.TextSecondary, t.TableSortedBg, ContrastText)
	add("Text", "TableHeaderActiveBg", "sorted / hovered header cell", t.Text, t.TableHeaderActiveBg, ContrastText)
	add("TextSecondary", "TableHeaderActiveBg", "secondary text in active header", t.TextSecondary, t.TableHeaderActiveBg, ContrastText)
	// Typography Mark: MarkText (zero: the span keeps Text) on MarkBg.
	mark := t.MarkText
	if mark.A == 0 {
		mark = t.Text
	}
	add("MarkText", "MarkBg", "highlighted (mark) text", mark, t.MarkBg, ContrastText)
	// Non-text (1.4.11): visual information required to identify a control
	// or its state. Border frames inputs, selects and checkboxes — an empty
	// outlined input on the page has no other cue — so it is required even
	// though labelled default buttons that share it would not need it.
	add("Border", "BgContainer", "input / checkbox boundary", t.Border, t.BgContainer, ContrastNonText)
	add("SwitchOffBg", "BgContainer", "switch track (off)", t.SwitchOffBg, t.BgContainer, ContrastNonText)
	add("Primary", "BgContainer", "checked controls, active indicators", t.Primary.Base, t.BgContainer, ContrastNonText)
	// Inputs and unchecked checkboxes both stroke Border over ControlBg
	// (Border on BgContainer is checked above).
	add("Border", "ControlBg", "input / checkbox boundary on its fill", t.Border, t.ControlBg, ContrastNonText)
	// The off thumb is the switch's state indicator on its track; a
	// translucent track is composited over the page first.
	add("SwitchHandleOff", "SwitchOffBg", "switch thumb (off)", t.SwitchHandleOff, on(t.SwitchOffBg), ContrastNonText)
	// Rate stars carry the value (filled) and the scale (voids).
	add("RateStar", "BgContainer", "rate star (filled)", t.RateStar, t.BgContainer, ContrastNonText)
	add("RateStarEmpty", "BgContainer", "rate star (empty)", t.RateStarEmpty, t.BgContainer, ContrastNonText)
	// Focus indicators (2.4.7, 1.4.11).
	add("ControlFocusBorder", "BgContainer", "focused input border", t.ControlFocusBorder, t.BgContainer, ContrastNonText)
	add("FocusOutline", "BgContainer", "focus ring of buttons, checkboxes", t.FocusOutline, t.BgContainer, ContrastNonText)
	add("FocusRing", "BgContainer", "focus ring of icon buttons, tags", t.FocusRing, t.BgContainer, ContrastNonText)
	// Exempt: decorative rules. Cards, table cells, list rows and dividers
	// separate content that identifies itself; the line carries no control
	// boundary or state (1.4.11 applies to "visual information required to
	// identify user interface components and states").
	add("BorderSecondary", "BgContainer", "card / table / list rules (decorative)", t.BorderSecondary, t.BgContainer, ContrastExempt)
	add("Split", "BgContainer", "dividers (decorative)", t.Split, t.BgContainer, ContrastExempt)
	return out
}

// ContrastIssues returns the required pairs of t below their minimum:
// failures and design defaults (exempt pairs are never issues). ui's
// Driver.Audit lists them; gate on ContrastFailures.
func ContrastIssues(t *Theme) []ContrastCheck {
	var out []ContrastCheck
	for _, c := range CheckContrast(t) {
		if c.Status == ContrastStatusFail || c.Status == ContrastStatusDesignDefault {
			out = append(out, c)
		}
	}
	return out
}

// ContrastFailures returns the required pairs of t that fail and are not
// design-system defaults: what an audit of t must keep at zero. For a
// HighContrast theme every required pair below its minimum is a failure.
func ContrastFailures(t *Theme) []ContrastCheck {
	var out []ContrastCheck
	for _, c := range CheckContrast(t) {
		if c.Status == ContrastStatusFail {
			out = append(out, c)
		}
	}
	return out
}

// ContrastReport formats the checks of several themes grouped by status:
// failures, design defaults, exempt pairs, then passing pairs, after a
// summary line.
func ContrastReport(themes ...*Theme) string {
	var groups [4][]string
	for _, t := range themes {
		for _, c := range CheckContrast(t) {
			groups[c.Status] = append(groups[c.Status], c.String())
		}
	}
	head := fmt.Sprintf("contrast: %d fail, %d design-default, %d exempt, %d ok",
		len(groups[ContrastStatusFail]), len(groups[ContrastStatusDesignDefault]), len(groups[ContrastStatusExempt]), len(groups[ContrastStatusPass]))
	lines := []string{head}
	for _, s := range []ContrastStatus{ContrastStatusFail, ContrastStatusDesignDefault, ContrastStatusExempt, ContrastStatusPass} {
		lines = append(lines, groups[s]...)
	}
	return strings.Join(lines, "\n")
}

// BuiltinThemes returns the six built-in themes checked by the contrast
// tests: Ant Design, Element Plus and shadcn/ui, light and dark.
func BuiltinThemes() []*Theme {
	return []*Theme{AntDesign(), AntDesign(Dark()), ElementPlus(), ElementPlus(Dark()), Shadcn(), Shadcn(Dark())}
}

// EnsureContrast returns fg, or the smallest step (10% increments) from fg
// towards the colors in toward, in order, that reaches min contrast
// against bg. Themes use it to lift focus indicators that would otherwise
// be too faint (WCAG 1.4.11) while keeping their hue.
func EnsureContrast(fg, bg Color, min float64, toward ...Color) Color {
	if ContrastRatio(fg, bg) >= min {
		return fg
	}
	cur := fg
	for _, t := range toward {
		from := cur
		for w := 0.1; w <= 1.0001; w += 0.1 {
			cur = Mix(from, t, w)
			if ContrastRatio(cur, bg) >= min {
				return cur
			}
		}
	}
	return cur
}
