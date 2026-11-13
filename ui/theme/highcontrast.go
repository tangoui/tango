package theme

import (
	"reflect"
	"strings"
)

// HighContrast returns a copy of th in which every required contrast pair
// of CheckContrast meets WCAG 2.2 AA — text 4.5:1, large text and the
// non-text parts that identify a control, its state or focus 3:1 —
// including the design-system defaults the stock themes keep (placeholder
// and tertiary text, links, input borders, switch tracks, primary and
// danger fills). Use it when the system asks for increased contrast
// (docs/accessibility.md, "Increase contrast"); it works on any theme,
// including WithPrimary and ShadcnOptions variants and app overrides.
//
// The style is kept: geometry, typography, shadows, focus-ring shapes and
// hues do not change. Each color below its minimum moves the shortest
// distance (2% steps) towards the theme's own text color, then towards
// black (white on dark themes), so Ant Design stays Ant blue on white,
// Element stays Element and shadcn stays neutral — only with darker (in
// dark mode lighter) ink, borders and fills. A solid fill (Primary, Error)
// moves as a whole palette (hover and pressed shades follow) and keeps its
// label color when some shade satisfies both the label and the fill's own
// contrast on the page. In dark themes the danger color is both text on the
// page and the fill under a white label, and no red is light enough for
// one and dark enough for the other; the label then switches to the page's
// dark ink (TextOnSolid), the usual dark high-contrast arrangement.
//
// Component tokens (Theme.Components) are processed after the global
// ones, overrides included: colors copied from a moved palette follow it,
// every field with a contrast tag reaches its minimum (components_contrast.go)
// and the Alias themes of Theme.For go through HighContrast too.
//
// Exempt pairs (disabled controls, decorative rules) are left alone. The
// result has HighContrast set and reports label it "<style>-hc".
func HighContrast(th *Theme) *Theme {
	if th == nil {
		return nil
	}
	cp := *th
	h := &cp
	h.HighContrast = true
	if th.Shadcn != nil {
		v := *th.Shadcn
		h.Shadcn = &v
	}

	white := Hex("#ffffff")
	base := h.BgContainer
	if base.A < 255 {
		base = Over(base, white)
	}
	flat := func(c Color) Color {
		if c.A < 255 {
			return Over(c, base)
		}
		return c
	}
	pages := []Color{base, flat(h.BgElevated), flat(h.BgLayout)}

	// Solid fills first: tokens derived from them (links, selected items,
	// focus borders) follow before they are lifted themselves.
	oldP, oldE := h.Primary, h.Error
	infoIsPrimary := h.Info == h.Primary
	h.Primary, h.TextOnPrimary = solidFill(h.Primary, h.TextOnPrimary, base, 3)
	h.Error, h.TextOnSolid = solidFill(h.Error, h.TextOnSolid, base, 4.5)
	if infoIsPrimary {
		h.Info = h.Primary
	}
	moves := paletteMoves(oldP, h.Primary)
	for k, v := range paletteMoves(oldE, h.Error) {
		moves[k] = v
	}
	remapColors(h, moves, base)

	// Text: towards the theme's text color, then pure ink.
	lift := func(c *Color, min float64, bgs ...Color) {
		for _, bg := range bgs {
			*c = reach(*c, bg, min, flatOr(h.Text, bg), away(*c, bg))
		}
	}
	// Body text also sits on input fills and sorted-table tints; the
	// placeholder on input fills.
	ctrl := []Color{flat(h.ControlBg), flat(h.ControlBgHover)}
	surfaces := append(append(append([]Color{}, pages...), ctrl...), flat(h.TableSortedBg), flat(h.TableHeaderActiveBg))
	lift(&h.Text, 4.5, surfaces...)
	lift(&h.TextHeading, 4.5, pages...)
	lift(&h.TextSecondary, 4.5, surfaces...)
	lift(&h.TextTertiary, 4.5, pages...)
	lift(&h.TextQuaternary, 4.5, append(append([]Color{}, pages...), ctrl[0])...)
	lift(&h.TableHeaderText, 4.5, flat(h.TableHeaderBg), base)
	lift(&h.AddonText, 4.5, flat(h.AddonBg), base)
	lift(&h.TextOnSpotlight, 4.5, flat(h.BgSpotlight))
	// Colored text: straight towards black / white keeps the hue.
	for _, c := range []*Color{&h.TextLink, &h.TextLinkHover, &h.TextLinkActive} {
		for _, bg := range pages {
			*c = reach(*c, bg, 4.5, away(*c, bg))
		}
	}

	// Non-text boundaries and states (3:1 on the page and in popups).
	ink := flat(h.Text)
	border := func(c *Color, min float64, more ...Color) {
		for _, bg := range append(append([]Color{}, pages[:2]...), more...) {
			*c = reach(*c, bg, min, ink, away(*c, bg))
		}
	}
	// Inputs and unchecked boxes stroke Border over ControlBg as well.
	border(&h.Border, 3, ctrl[0])
	// Hover must not make a frame fainter than at rest.
	border(&h.BorderHover, max(3, ContrastRatio(h.Border, base)))
	border(&h.SwitchOffBg, 3)
	border(&h.SwitchOffBgHover, max(3, ContrastRatio(h.SwitchOffBg, base)))
	// The off thumb must stand out from its (possibly lifted) track.
	h.SwitchHandleOff = reach(h.SwitchHandleOff, flat(h.SwitchOffBg), 3, away(h.SwitchHandleOff, flat(h.SwitchOffBg)))
	// Rate stars: filled ones keep their hue (straight to black / white),
	// voids go towards the ink like borders.
	for _, bg := range pages[:2] {
		h.RateStar = reach(h.RateStar, bg, 3, away(h.RateStar, bg))
	}
	border(&h.RateStarEmpty, 3)
	// The danger button fill (shadcn dark: destructive/60) carries the
	// label chosen by solidFill.
	h.ButtonDangerBg = reach(flat(h.ButtonDangerBg), h.TextOnSolid, 4.5, away(flat(h.ButtonDangerBg), h.TextOnSolid))
	// Typography Mark: dark or light ink, whichever the highlight needs.
	switch mbg := flat(h.MarkBg); {
	case h.MarkText.A > 0:
		h.MarkText = reach(h.MarkText, mbg, 4.5, away(h.MarkText, mbg))
	case ContrastRatio(h.Text, mbg) < 4.5:
		h.MarkText = reach(flatOr(h.Text, mbg), mbg, 4.5, away(h.Text, mbg))
	}
	for _, c := range []*Color{&h.ControlFocusBorder, &h.FocusRing, &h.FocusOutline} {
		for _, bg := range pages[:2] {
			*c = reach(*c, bg, 3, h.Primary.Active, away(*c, bg))
		}
	}

	if v := h.Shadcn; v != nil {
		v.Foreground, v.CardForeground, v.PopoverForeground = h.Text, h.Text, h.Text
		v.MutedForeground = h.TextTertiary
		v.Primary, v.PrimaryForeground = h.Primary.Base, h.TextOnPrimary
		v.SidebarPrimary, v.SidebarPrimaryForeground = v.Primary, v.PrimaryForeground
		v.Destructive = h.Error.Base
		if h.TextOnSolid != th.TextOnSolid {
			v.DestructiveForeground = h.TextOnSolid
		}
		v.Input = h.Border
		v.Ring, v.SidebarRing = h.ControlFocusBorder, h.ControlFocusBorder
	}
	// Component tokens last, user overrides included (no exemption).
	if th.comp != nil {
		// Re-derive from the high-contrast globals first, so tokens that
		// follow a global (Border, Text…) get its HC value, then lift what
		// still falls short (user overrides included).
		h.deriveComponents(th.Overrides())
		h.contrastComponents(th)
	}
	return h
}

// reach returns fg, or the first 2% step from fg towards the colors in
// toward, in order, that reaches min contrast on the opaque bg.
func reach(fg, bg Color, min float64, toward ...Color) Color {
	if ContrastRatio(fg, bg) >= min {
		return fg
	}
	cur := fg
	for _, t := range toward {
		from := cur
		for i := 1; i <= 50; i++ {
			cur = Mix(from, t, float64(i)/50)
			if ContrastRatio(cur, bg) >= min {
				return cur
			}
		}
	}
	return cur
}

// away is white when fg is lighter than bg, else black: the extreme that
// increases their contrast.
func away(fg, bg Color) Color {
	if fg.A < 255 {
		fg = Over(fg, bg)
	}
	if RelativeLuminance(fg) > RelativeLuminance(bg) {
		return Hex("#ffffff")
	}
	return Hex("#000000")
}

// flatOr composites a translucent c over bg.
func flatOr(c, bg Color) Color {
	if c.A < 255 {
		return Over(c, bg)
	}
	return c
}

// solidFill adjusts palette p so that label reads at 4.5:1 on p.Base and
// p.Base itself reaches fillMin on the page bg. The smallest shift towards
// black or white that satisfies both wins; when none does with label, the
// label flips (white ↔ the page's dark ink) and the search repeats.
func solidFill(p Palette, label, bg Color, fillMin float64) (Palette, Color) {
	ok := func(f, l Color) bool { return ContrastRatio(l, f) >= 4.5 && ContrastRatio(f, bg) >= fillMin }
	if ok(p.Base, label) {
		return p, label
	}
	white, black := Hex("#ffffff"), Hex("#000000")
	search := func(l Color) (Color, float64, bool) {
		for i := 0; i <= 50; i++ { // i = 0: the flipped label may suffice as is
			w := float64(i) / 50
			for _, dir := range []Color{black, white} {
				if ok(Mix(p.Base, dir, w), l) {
					return dir, w, true
				}
			}
		}
		return Color{}, 0, false
	}
	dir, w, found := search(label)
	if !found {
		alt := white
		if Luma(label) > .5 {
			alt = black
			if RelativeLuminance(bg) < 0.05 {
				alt = bg // the page's own near-black
			}
		}
		if dir, w, found = search(alt); !found {
			return p, label
		}
		label = alt
	}
	q := p
	mv := func(c Color) Color { return Mix(c, dir, w) }
	q.Base = mv(p.Base)
	// Hover and pressed fills keep their offset and carry the label too.
	q.Hover = reach(mv(p.Hover), label, 4.5, dir)
	q.Active = reach(mv(p.Active), label, 4.5, dir)
	q.Text, q.TextHover, q.TextActive = mv(p.Text), mv(p.TextHover), mv(p.TextActive)
	for i, s := range p.Shades {
		switch s {
		case p.Base:
			q.Shades[i] = q.Base
		case p.Hover:
			q.Shades[i] = q.Hover
		case p.Active:
			q.Shades[i] = q.Active
		}
	}
	return q, label
}

// paletteMoves maps the solid colors of old to their replacements in q.
func paletteMoves(old, q Palette) map[Color]Color {
	m := map[Color]Color{}
	for _, pr := range [][2]Color{{old.Base, q.Base}, {old.Hover, q.Hover}, {old.Active, q.Active},
		{old.Text, q.Text}, {old.TextHover, q.TextHover}, {old.TextActive, q.TextActive}} {
		if pr[0] != pr[1] {
			m[pr[0]] = pr[1]
		}
	}
	return m
}

// remapColors replaces, in every top-level Color field of t, the colors
// that were copied from a palette that moved (TextLink, ItemSelected,
// ControlFocusBorder, BorderHover, SegmentedSelectedBg …). Sider* fields
// sit on the sider's own (dark) background, where a darker brand color
// would read worse, and the labels on fills were chosen by solidFill.
// Colors drawn on the page (links, selected text, focus borders) take the
// move only when it does not lower their contrast with the page: in dark
// themes a fill darkened for its white label would make them dimmer.
// Fills (…Bg) always follow, since they carry the label.
func remapColors(t *Theme, moves map[Color]Color, page Color) {
	if len(moves) == 0 {
		return
	}
	v := reflect.ValueOf(t).Elem()
	ct := reflect.TypeOf(Color{})
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.Type != ct || strings.HasPrefix(f.Name, "Sider") || f.Name == "TextOnPrimary" || f.Name == "TextOnSolid" {
			continue
		}
		c := v.Field(i).Interface().(Color)
		if n, ok := moves[c]; ok && (strings.HasSuffix(f.Name, "Bg") || ContrastRatio(n, page) >= ContrastRatio(c, page)) {
			v.Field(i).Set(reflect.ValueOf(n))
		}
	}
}
