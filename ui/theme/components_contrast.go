package theme

import (
	"reflect"
	"strings"
)

// Contrast of component tokens. A Color field of a token struct may carry
//
//	contrast:"text:<bg>"     text drawn on <bg>, 4.5:1 (WCAG 1.4.3)
//	contrast:"nontext:<bg>"  a boundary / state / indicator on <bg>, 3:1 (1.4.11)
//
// where <bg> is a Color field of the same token (a zero or fully
// transparent one means the control shows the page), or @Page
// (BgContainer), @Elevated (BgElevated), @Spotlight (BgSpotlight),
// @Primary (Primary.Base) or @Error (Error.Base) of the component's theme
// (Theme.For). HighContrast lifts every tagged field to its minimum and
// CheckContrast reports the tagged pairs an override touched.

// contrastTag is one parsed contrast tag.
type contrastTag struct {
	field int // index in the token struct
	name  string
	kind  ContrastKind
	on    string
}

// tokenTags returns the contrast tags of a token struct type, non-text
// pairs first (fills and borders settle before the text drawn on them).
func tokenTags(t reflect.Type) []contrastTag {
	var text, non []contrastTag
	for i := range t.NumField() {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("contrast")
		if !ok || f.Type != colorType {
			continue
		}
		kind, on, _ := strings.Cut(tag, ":")
		switch kind {
		case "text":
			text = append(text, contrastTag{i, f.Name, ContrastText, on})
		case "nontext":
			non = append(non, contrastTag{i, f.Name, ContrastNonText, on})
		default:
			panic("theme: bad contrast tag on " + t.Name() + "." + f.Name)
		}
	}
	return append(non, text...)
}

var compTags = func() (tags [numComps][]contrastTag) {
	for k := range numComps {
		tags[k] = tokenTags(componentsType.Field(int(k)).Type)
	}
	return tags
}()

// pageOf is t's opaque page color.
func pageOf(t *Theme) Color {
	return flatOr(t.BgContainer, Hex("#ffffff"))
}

// contrastBg resolves the background of a tagged pair to an opaque color.
func contrastBg(on string, tok reflect.Value, ctx *Theme) Color {
	page := pageOf(ctx)
	var c Color
	switch on {
	case "@Page":
		return page
	case "@Elevated":
		c = ctx.BgElevated
	case "@Spotlight":
		c = ctx.BgSpotlight
	case "@Primary":
		c = ctx.Primary.Base
	case "@Error":
		c = ctx.Error.Base
	default:
		f := tok.FieldByName(on)
		if !f.IsValid() || f.Type() != colorType {
			panic("theme: contrast tag refers to unknown field " + on)
		}
		c = f.Interface().(Color)
	}
	if c.A == 0 {
		return page
	}
	return flatOr(c, page)
}

// chromatic reports whether c carries a hue worth keeping (brand-colored
// text moves straight towards black / white instead of the text color).
func chromatic(c Color) bool { return ToHSV(c).S > .2 }

// contrastComponents makes the component tokens of the high-contrast theme
// h (copied from src) meet their tags: Alias themes go through HighContrast
// themselves, colors copied from a moved palette follow it, then every
// tagged field is lifted. Runs last, so user overrides are covered too.
func (h *Theme) contrastComponents(src *Theme) {
	st := *h.comp
	h.comp = &st
	for k, a := range st.alias {
		if a != nil {
			st.alias[k] = HighContrast(a) // a has no compState: flat tokens only
		}
	}
	rv := reflect.ValueOf(&h.Components).Elem()
	for k := range numComps {
		ctx, sctx := h.For(k), src.For(k)
		page := pageOf(ctx)
		moves := paletteMoves(sctx.Primary, ctx.Primary)
		for c, n := range paletteMoves(sctx.Error, ctx.Error) {
			moves[c] = n
		}
		tok := rv.Field(int(k))
		remapToken(tok, moves, page)
		for _, tg := range compTags[k] {
			f := tok.Field(tg.field)
			fg := f.Interface().(Color)
			if fg.A == 0 {
				continue // not drawn
			}
			bg := contrastBg(tg.on, tok, ctx)
			min := tg.kind.Min()
			orig := fg
			if chromatic(fg) {
				fg = reach(fg, bg, min, away(fg, bg))
			} else {
				fg = reach(fg, bg, min, flatOr(ctx.Text, bg), away(fg, bg))
			}
			if ContrastRatio(fg, bg) < min {
				// A label on a mid-tone fill (dark-mode danger) cannot get
				// there in its own direction: flip it the way solidFill
				// flips TextOnSolid.
				fg = reach(orig, bg, min, flipLabel(away(orig, bg), page)...)
			}
			f.Set(reflect.ValueOf(fg))
		}
		tok.Field(0).Set(reflect.ValueOf(aliasOf(ctx)))
	}
	for _, a := range st.alias {
		if a != nil {
			a.Components = h.Components
		}
	}
}

// remapToken is remapColors for a token struct, recursing into nested
// structs (the Alias is reset from the component's theme afterwards).
// Dark* foregrounds sit on the dark sider and are left to their tags.
func remapToken(v reflect.Value, moves map[Color]Color, page Color) {
	if len(moves) == 0 {
		return
	}
	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		switch {
		case sf.Type == aliasType:
		case sf.Type == colorType:
			if strings.HasPrefix(sf.Name, "Dark") && !strings.HasSuffix(sf.Name, "Bg") {
				continue
			}
			c := f.Interface().(Color)
			if n, ok := moves[c]; ok && (strings.HasSuffix(sf.Name, "Bg") || ContrastRatio(n, page) >= ContrastRatio(c, page)) {
				f.Set(reflect.ValueOf(n))
			}
		case sf.Type.Kind() == reflect.Struct:
			remapToken(f, moves, page)
		}
	}
}

// componentPairs measures the tagged component pairs of t: all of them, or
// (all false) only those an override touched — the component's Alias, the
// foreground or a background field set by the app. Pairs whose foreground
// is not drawn (zero alpha) are skipped.
func componentPairs(t *Theme, all bool) []ContrastCheck {
	if t.comp == nil {
		return nil
	}
	name := themeLabel(t)
	rv := reflect.ValueOf(t.Components)
	uv := reflect.ValueOf(t.comp.user)
	var out []ContrastCheck
	for k := range numComps {
		tok, user := rv.Field(int(k)), uv.Field(int(k))
		aliased := t.comp.alias[k] != nil
		ctx := t.For(k)
		for _, tg := range compTags[k] {
			if !all && !aliased && isUnset(user.Field(tg.field)) {
				if f := user.FieldByName(tg.on); !f.IsValid() || isUnset(f) {
					continue
				}
			}
			fg := tok.Field(tg.field).Interface().(Color)
			if fg.A == 0 {
				continue
			}
			bg := contrastBg(tg.on, tok, ctx)
			comp := Comp(k).String()
			on := tg.on
			if !strings.HasPrefix(on, "@") {
				on = comp + "." + on
			}
			out = append(out, ContrastCheck{
				Theme: name, Token: comp + "." + tg.name, On: on, Use: comp + " component token",
				FG: fg, BG: bg, Kind: tg.kind, Ratio: ContrastRatio(fg, bg),
			})
		}
	}
	return out
}

// componentChecks classifies componentPairs for CheckContrast: in a
// HighContrast theme every tagged pair is required; otherwise only pairs an
// override touched are reported, and below their minimum they fail (an
// override is the app's choice, never a design-system default).
func componentChecks(t *Theme) []ContrastCheck {
	out := componentPairs(t, t.HighContrast)
	var hc map[[2]string]ContrastCheck
	for i := range out {
		c := &out[i]
		if c.Pass() {
			c.Status = ContrastStatusPass
			continue
		}
		c.Status = ContrastStatusFail
		if hc == nil {
			hc = map[[2]string]ContrastCheck{}
			for _, s := range componentPairs(HighContrast(t), true) {
				hc[[2]string{s.Token, s.On}] = s
			}
		}
		if s, ok := hc[[2]string{c.Token, c.On}]; ok {
			c.SuggestFG, c.SuggestBG, c.SuggestRatio = s.FG, s.BG, s.Ratio
		}
	}
	return out
}

// flipLabel lists the colors towards the other extreme than dir (the
// direction that failed): white, or the page's own near-black and then
// black.
func flipLabel(dir, page Color) []Color {
	black := Hex("#000000")
	if dir == black {
		return []Color{Hex("#ffffff")}
	}
	if RelativeLuminance(page) < 0.05 {
		return []Color{page, black}
	}
	return []Color{black}
}
