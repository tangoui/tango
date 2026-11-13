package theme

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/paint"
)

// CSSPrefix starts the name of every variable returned by CSSVars.
const CSSPrefix = "--tango-"

// CSSVars exports the main tokens as CSS custom properties, so a WebView
// page of the same application can reuse the colors and metrics of the
// self-drawn UI:
//
//	--tango-color-primary, --tango-color-primary-hover, -active, -bg,
//	  -bg-hover, -border, -border-hover, -text, -text-hover, -text-active,
//	  -1 … -10                           (also success, warning, error, info)
//	--tango-color-text, -text-heading, -text-secondary, -text-tertiary,
//	  -text-quaternary, -text-disabled, -text-on-primary
//	--tango-color-link, -link-hover, -link-active
//	--tango-color-border, -border-hover, -border-secondary, -split,
//	  -focus-ring
//	--tango-color-fill, -fill-secondary, -fill-tertiary, -fill-quaternary,
//	  -fill-alter
//	--tango-color-bg-container, -bg-elevated, -bg-layout, -bg-spotlight,
//	  -bg-mask, -bg-container-disabled, -bg-text-hover, -bg-text-active
//	--tango-radius, -radius-xs, -radius-sm, -radius-lg      (px)
//	--tango-font-size, -font-size-sm, -lg, -xl, -font-size-heading-1 … -5
//	--tango-line-height (unitless), --tango-font-weight-strong
//	--tango-control-height, -sm, -lg, --tango-control-padding-h, -sm,
//	  --tango-line-width
//	--tango-padding-xxs … -xl, --tango-margin-xs … -xl
//	--tango-shadow, -shadow-secondary, -shadow-tertiary     (box-shadow)
//	--tango-motion-duration-fast, -mid, -slow               (seconds)
//	--tango-color-scheme ("light" / "dark")
//
// Shadcn themes add shadcn/ui's own names (--background, --primary,
// --ring, --chart-1 …, --radius; see ShadcnVars.Vars).
//
// Opaque colors are "#rrggbb", translucent ones "rgba(r, g, b, a)".
// Lengths are logical px (CSS px).
func (t *Theme) CSSVars() map[string]string {
	v := map[string]string{}
	col := func(name string, c Color) { v[CSSPrefix+"color-"+name] = cssColor(c) }
	px := func(name string, f float32) { v[CSSPrefix+name] = cssPx(f) }

	for _, p := range []struct {
		name string
		pal  Palette
	}{{"primary", t.Primary}, {"success", t.Success}, {"warning", t.Warning}, {"error", t.Error}, {"info", t.Info}} {
		n := p.name
		col(n, p.pal.Base)
		col(n+"-hover", p.pal.Hover)
		col(n+"-active", p.pal.Active)
		col(n+"-bg", p.pal.Bg)
		col(n+"-bg-hover", p.pal.BgHover)
		col(n+"-border", p.pal.Border)
		col(n+"-border-hover", p.pal.BorderHover)
		col(n+"-text", p.pal.Text)
		col(n+"-text-hover", p.pal.TextHover)
		col(n+"-text-active", p.pal.TextActive)
		for i, s := range p.pal.Shades {
			col(n+"-"+strconv.Itoa(i+1), s)
		}
	}

	col("text", t.Text)
	col("text-heading", t.TextHeading)
	col("text-secondary", t.TextSecondary)
	col("text-tertiary", t.TextTertiary)
	col("text-quaternary", t.TextQuaternary)
	col("text-disabled", t.TextDisabled)
	col("text-on-primary", t.TextOnPrimary)
	col("link", t.TextLink)
	col("link-hover", t.TextLinkHover)
	col("link-active", t.TextLinkActive)

	col("border", t.Border)
	col("border-hover", t.BorderHover)
	col("border-secondary", t.BorderSecondary)
	col("split", t.Split)
	col("focus-ring", t.FocusRing)

	col("fill", t.Fill)
	col("fill-secondary", t.FillSecondary)
	col("fill-tertiary", t.FillTertiary)
	col("fill-quaternary", t.FillQuaternary)
	col("fill-alter", t.FillAlter)

	col("bg-container", t.BgContainer)
	col("bg-elevated", t.BgElevated)
	col("bg-layout", t.BgLayout)
	col("bg-spotlight", t.BgSpotlight)
	col("bg-mask", t.BgMask)
	col("bg-container-disabled", t.BgContainerDisabled)
	col("bg-text-hover", t.BgTextHover)
	col("bg-text-active", t.BgTextActive)

	px("radius", t.Radius)
	px("radius-xs", t.RadiusXS)
	px("radius-sm", t.RadiusSM)
	px("radius-lg", t.RadiusLG)

	px("font-size", t.FontSize)
	px("font-size-sm", t.FontSizeSM)
	px("font-size-lg", t.FontSizeLG)
	px("font-size-xl", t.FontSizeXL)
	for i, h := range t.Heading {
		px("font-size-heading-"+strconv.Itoa(i+1), h)
	}
	v[CSSPrefix+"line-height"] = cssNum(t.LineHeight)
	v[CSSPrefix+"font-weight-strong"] = strconv.Itoa(t.FontWeightStrong)

	px("control-height", t.ControlHeight)
	px("control-height-sm", t.ControlHeightSM)
	px("control-height-lg", t.ControlHeightLG)
	px("control-padding-h", t.ControlPaddingH)
	px("control-padding-h-sm", t.ControlPaddingHSM)
	px("line-width", t.LineWidth)

	for _, s := range []struct {
		name string
		val  float32
	}{
		{"padding-xxs", t.PaddingXXS}, {"padding-xs", t.PaddingXS}, {"padding-sm", t.PaddingSM}, {"padding", t.Padding},
		{"padding-md", t.PaddingMD}, {"padding-lg", t.PaddingLG}, {"padding-xl", t.PaddingXL},
		{"margin-xs", t.MarginXS}, {"margin-sm", t.MarginSM}, {"margin", t.Margin},
		{"margin-md", t.MarginMD}, {"margin-lg", t.MarginLG}, {"margin-xl", t.MarginXL},
	} {
		px(s.name, s.val)
	}

	v[CSSPrefix+"shadow"] = cssShadow(t.Shadow)
	v[CSSPrefix+"shadow-secondary"] = cssShadow(t.ShadowSecondary)
	v[CSSPrefix+"shadow-tertiary"] = cssShadow(t.ShadowTertiary)

	v[CSSPrefix+"motion-duration-fast"] = cssDuration(t.DurationFast)
	v[CSSPrefix+"motion-duration-mid"] = cssDuration(t.DurationMid)
	v[CSSPrefix+"motion-duration-slow"] = cssDuration(t.DurationSlow)

	v[CSSPrefix+"color-scheme"] = map[bool]string{false: "light", true: "dark"}[t.Dark]
	if t.Shadcn != nil {
		for k, s := range t.Shadcn.Vars(false) {
			v[k] = s
		}
	}
	return v
}

// Vars returns shadcn/ui's own CSS variables (--background, --foreground,
// --card, --popover, --primary, --secondary, --muted, --accent,
// --destructive with their -foreground pairs, --border, --input, --ring,
// --chart-1 … --chart-5, --sidebar-*, --radius). Colors are CSS colors
// ("#rrggbb"), the Tailwind v4 convention (var(--primary)); with hsl true
// they are bare HSL channels ("240 5.9% 10%") for Tailwind v3 projects that
// write hsl(var(--primary)). CSSVars includes the first form for shadcn
// themes, so a WebView page using shadcn/ui shares the self-drawn theme.
func (s *ShadcnVars) Vars(hsl bool) map[string]string {
	out := map[string]string{}
	col := func(name string, c Color) {
		if hsl {
			out["--"+name] = hslChannels(c)
		} else {
			out["--"+name] = cssColor(c)
		}
	}
	for _, p := range []struct {
		name string
		c    Color
	}{
		{"background", s.Background}, {"foreground", s.Foreground},
		{"card", s.Card}, {"card-foreground", s.CardForeground},
		{"popover", s.Popover}, {"popover-foreground", s.PopoverForeground},
		{"primary", s.Primary}, {"primary-foreground", s.PrimaryForeground},
		{"secondary", s.Secondary}, {"secondary-foreground", s.SecondaryForeground},
		{"muted", s.Muted}, {"muted-foreground", s.MutedForeground},
		{"accent", s.Accent}, {"accent-foreground", s.AccentForeground},
		{"destructive", s.Destructive}, {"destructive-foreground", s.DestructiveForeground},
		{"border", s.Border}, {"input", s.Input}, {"ring", s.Ring},
		{"sidebar", s.Sidebar}, {"sidebar-foreground", s.SidebarForeground},
		{"sidebar-primary", s.SidebarPrimary}, {"sidebar-primary-foreground", s.SidebarPrimaryForeground},
		{"sidebar-accent", s.SidebarAccent}, {"sidebar-accent-foreground", s.SidebarAccentForeground},
		{"sidebar-border", s.SidebarBorder}, {"sidebar-ring", s.SidebarRing},
	} {
		col(p.name, p.c)
	}
	for i, c := range s.Chart {
		col("chart-"+strconv.Itoa(i+1), c)
	}
	out["--radius"] = cssNum(s.Radius/16) + "rem"
	return out
}

// hslChannels formats c as shadcn's bare "H S% L%" triplet (alpha dropped).
func hslChannels(c Color) string {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l := (mx + mn) / 2
	var h, s float64
	if d := mx - mn; d > 0 {
		if l > 0.5 {
			s = d / (2 - mx - mn)
		} else {
			s = d / (mx + mn)
		}
		switch mx {
		case r:
			h = (g - b) / d
			if g < b {
				h += 6
			}
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h *= 60
	}
	f := func(x float64) string {
		s := strconv.FormatFloat(x, 'f', 1, 64)
		return strings.TrimSuffix(s, ".0")
	}
	return f(h) + " " + f(s*100) + "% " + f(l*100) + "%"
}

// CSS returns the variables of CSSVars as a ":root" rule.
func (t *Theme) CSS() string { return CSSRule(":root", t.CSSVars()) }

// CSSRule formats vars as one CSS rule for selector, one declaration per
// line sorted by name, so the output is stable:
//
//	:root {
//	  --tango-color-primary: #1677ff;
//	  ...
//	}
func CSSRule(selector string, vars map[string]string) string {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(selector)
	b.WriteString(" {\n")
	for _, k := range names {
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(vars[k])
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func cssColor(c Color) string {
	if c.A == 255 {
		return HexString(c)
	}
	return RGBString(c)
}

func cssNum(f float32) string {
	s := strconv.FormatFloat(float64(f), 'f', 4, 32)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func cssPx(f float32) string {
	if f == 0 {
		return "0"
	}
	return cssNum(f) + "px"
}

func cssDuration(d time.Duration) string { return cssNum(float32(d.Seconds())) + "s" }

func cssShadow(ss []paint.Shadow) string {
	if len(ss) == 0 {
		return "none"
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = cssPx(s.X) + " " + cssPx(s.Y) + " " + cssPx(s.Blur) + " " + cssPx(s.Spread) + " " + cssColor(s.Color)
	}
	return strings.Join(parts, ", ")
}

// CSSOptions selects what CSSVarsWith exports besides the CSSVars set.
type CSSOptions struct {
	// Components adds one variable per component token,
	// --tango-<component>-<token> in kebab case (--tango-button-padding-inline,
	// --tango-button-color-primary for the Alias, --tango-table-header-bg); a
	// css:"name" tag on a field replaces the <token> part.
	Components bool
	// OnlyChanged keeps only the component variables whose value differs
	// from the theme built without component overrides (empty when there are
	// none), instead of several hundred.
	OnlyChanged bool
	// Ref writes var(--tango-…) for a component token that equals the
	// global variable named by its ref tag (the Alias fields), so a page
	// changing the global variable cascades to the component.
	Ref bool
}

// CSSVarsWith is CSSVars plus the component tokens selected by o. Component
// variables are opt-in: CSSVars itself never includes them.
func (t *Theme) CSSVarsWith(o CSSOptions) map[string]string {
	v := t.CSSVars()
	if !o.Components {
		return v
	}
	var base map[string]string
	if o.OnlyChanged {
		base = map[string]string{}
		t.withOverrides(Components{}).componentVars(base, nil)
	}
	raw, out := map[string]string{}, map[string]string{}
	t.componentVars(raw, nil)
	if o.Ref {
		t.componentVars(out, v)
	} else {
		out = raw
	}
	for k, s := range out {
		if b, ok := base[k]; ok && b == raw[k] {
			continue
		}
		v[k] = s
	}
	return v
}

// componentVars writes the component token variables of t into out. With
// global set, a field whose ref tag names a global variable of the same
// value is written as var(--tango-<ref>).
func (t *Theme) componentVars(out, global map[string]string) {
	rv := reflect.ValueOf(t.Components)
	for k := range numComps {
		prefix := CSSPrefix + kebab(Comp(k).String()) + "-"
		var walk func(v reflect.Value)
		walk = func(v reflect.Value) {
			for i := range v.NumField() {
				f, sf := v.Field(i), v.Type().Field(i)
				if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
					walk(f)
					continue
				}
				val, ok := cssValue(f)
				if !ok {
					continue
				}
				if ref := sf.Tag.Get("ref"); ref != "" && global != nil && global[CSSPrefix+ref] == val {
					val = "var(" + CSSPrefix + ref + ")"
				}
				name := sf.Tag.Get("css")
				if name == "" {
					name = kebab(sf.Name)
				}
				out[prefix+name] = val
			}
		}
		walk(rv.Field(int(k)))
	}
}

// cssValue formats a token field: colors, px lengths, unitless float64
// ratios, integers (font weights), shadows and durations.
func cssValue(f reflect.Value) (string, bool) {
	switch {
	case f.Type() == colorType:
		return cssColor(f.Interface().(Color)), true
	case f.Type() == reflect.TypeFor[time.Duration]():
		return cssDuration(time.Duration(f.Int())), true
	case f.Type() == reflect.TypeFor[[]paint.Shadow]():
		return cssShadow(f.Interface().([]paint.Shadow)), true
	}
	switch f.Kind() {
	case reflect.Float32:
		return cssPx(float32(f.Float())), true
	case reflect.Float64:
		return cssNum(float32(f.Float())), true
	case reflect.Int:
		return strconv.Itoa(int(f.Int())), true
	}
	return "", false
}

// kebab converts a Go field name to CSS kebab case: PaddingInlineSM →
// padding-inline-sm, HeaderBg → header-bg.
func kebab(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i, c := range r {
		up := c >= 'A' && c <= 'Z'
		if up && i > 0 {
			prev := r[i-1]
			prevLower := (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9')
			nextLower := i+1 < len(r) && r[i+1] >= 'a' && r[i+1] <= 'z'
			if prevLower || (prev >= 'A' && prev <= 'Z' && nextLower) {
				b.WriteByte('-')
			}
		}
		if up {
			c += 'a' - 'A'
		}
		b.WriteRune(c)
	}
	return b.String()
}
