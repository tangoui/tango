package theme

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// Color is the non-premultiplied 8-bit color used throughout the UI.
type Color = color.NRGBA

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa". It panics on malformed input
// because it is meant for literal design tokens.
func Hex(s string) Color {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseHex is the error-returning form of Hex. It accepts 3, 4, 6 or 8
// hex digits (#rgb, #rgba, #rrggbb, #rrggbbaa), with or without the '#'
// and surrounding spaces, in either case.
func ParseHex(s string) (Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 || len(h) == 4 {
		b := make([]byte, 0, 8)
		for i := 0; i < len(h); i++ {
			b = append(b, h[i], h[i])
		}
		h = string(b)
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return Color{}, fmt.Errorf("theme: bad hex color %q", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("theme: bad hex color %q", s)
	}
	return Color{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}

// HexString formats c as #rrggbb (or #rrggbbaa when not opaque), lower
// case; use strings.ToUpper for the #RRGGBB display form.
func HexString(c Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// RGBString formats c as CSS "rgb(r, g, b)", or "rgba(r, g, b, a)" with
// alpha rounded to two decimals when it is not opaque.
func RGBString(c Color) string {
	if c.A == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
	}
	a := strconv.FormatFloat(math.Round(float64(c.A)/255*100)/100, 'f', -1, 64)
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", c.R, c.G, c.B, a)
}

// ParseRGB parses CSS rgb()/rgba() with comma or space separators and an
// optional "/ alpha"; channels are 0-255 integers, alpha 0-1 or a percent.
func ParseRGB(s string) (Color, error) {
	bad := func() (Color, error) { return Color{}, fmt.Errorf("theme: bad rgb color %q", s) }
	t := strings.ToLower(strings.TrimSpace(s))
	var body string
	switch {
	case strings.HasPrefix(t, "rgba(") && strings.HasSuffix(t, ")"):
		body = t[5 : len(t)-1]
	case strings.HasPrefix(t, "rgb(") && strings.HasSuffix(t, ")"):
		body = t[4 : len(t)-1]
	default:
		return bad()
	}
	parts := strings.FieldsFunc(body, func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
	if len(parts) != 3 && len(parts) != 4 {
		return bad()
	}
	var ch [3]uint8
	for i := 0; i < 3; i++ {
		v, err := strconv.Atoi(parts[i])
		if err != nil || v < 0 || v > 255 {
			return bad()
		}
		ch[i] = uint8(v)
	}
	c := Color{R: ch[0], G: ch[1], B: ch[2], A: 255}
	if len(parts) == 4 {
		p := parts[3]
		pct := strings.HasSuffix(p, "%")
		a, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
		if err != nil {
			return bad()
		}
		if pct {
			a /= 100
		}
		c.A = uint8(math.Round(clamp01(a) * 255))
	}
	return c, nil
}

// HSBString formats c as "hsb(h, s%, b%)" (hue in whole degrees,
// saturation and brightness in whole percent), or "hsba(h, s%, b%, a)"
// with alpha rounded to two decimals when it is not opaque. HSB is the
// same color space as HSV; HSVString spells it "hsv(...)".
func HSBString(c Color) string { return hsvText("hsb", ToHSVA(c)) }

// HSVString is HSBString with the "hsv(...)" / "hsva(...)" spelling.
func HSVString(c Color) string { return hsvText("hsv", ToHSVA(c)) }

// HSBAString formats h like HSBString; name is "hsb" or "hsv". Use it to
// keep the hue of greys, which a Color cannot carry.
func HSBAString(name string, h HSVA) string { return hsvText(name, h) }

func hsvText(name string, h HSVA) string {
	hue := int(math.Round(h.H)) % 360
	s, v := int(math.Round(clamp01(h.S)*100)), int(math.Round(clamp01(h.V)*100))
	a := math.Round(clamp01(h.A)*100) / 100
	if a >= 1 {
		return fmt.Sprintf("%s(%d, %d%%, %d%%)", name, hue, s, v)
	}
	return fmt.Sprintf("%sa(%d, %d%%, %d%%, %s)", name, hue, s, v, strconv.FormatFloat(a, 'f', -1, 64))
}

// ParseHSB parses hsb()/hsba()/hsv()/hsva() text: hue in degrees (an
// optional "deg" suffix), saturation and brightness in percent (the "%" is
// optional; 0-100), alpha 0-1 or a percent; comma or space separators and
// an optional "/ alpha".
func ParseHSB(s string) (HSVA, error) {
	bad := func() (HSVA, error) { return HSVA{}, fmt.Errorf("theme: bad hsb color %q", s) }
	t := strings.ToLower(strings.TrimSpace(s))
	var body string
	for _, p := range []string{"hsba(", "hsva(", "hsb(", "hsv("} {
		if strings.HasPrefix(t, p) && strings.HasSuffix(t, ")") {
			body = t[len(p) : len(t)-1]
			break
		}
	}
	if body == "" {
		return bad()
	}
	parts := strings.FieldsFunc(body, func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
	if len(parts) != 3 && len(parts) != 4 {
		return bad()
	}
	num := func(p, suffix string) (float64, bool) {
		v, err := strconv.ParseFloat(strings.TrimSuffix(p, suffix), 64)
		return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	hue, ok := num(parts[0], "deg")
	if !ok {
		return bad()
	}
	sat, ok1 := num(parts[1], "%")
	val, ok2 := num(parts[2], "%")
	if !ok1 || !ok2 || sat < 0 || sat > 100 || val < 0 || val > 100 {
		return bad()
	}
	h := HSVA{H: math.Mod(math.Mod(hue, 360)+360, 360), S: sat / 100, V: val / 100, A: 1}
	if len(parts) == 4 {
		a, ok := num(parts[3], "%")
		if !ok {
			return bad()
		}
		if strings.HasSuffix(parts[3], "%") {
			a /= 100
		}
		h.A = clamp01(a)
	}
	return h, nil
}

// ParseColor accepts any form a user may type into a color field: hex
// (see ParseHex), CSS rgb()/rgba(), hsb()/hsv() (see ParseHSB), plus
// "transparent".
func ParseColor(s string) (Color, error) {
	t := strings.TrimSpace(s)
	if strings.EqualFold(t, "transparent") {
		return Color{}, nil
	}
	if strings.HasPrefix(strings.ToLower(t), "rgb") {
		return ParseRGB(t)
	}
	if lt := strings.ToLower(t); strings.HasPrefix(lt, "hsb") || strings.HasPrefix(lt, "hsv") {
		h, err := ParseHSB(t)
		if err != nil {
			return Color{}, err
		}
		return FromHSVA(h), nil
	}
	c, err := ParseHex(t)
	if err != nil {
		return Color{}, fmt.Errorf("theme: bad color %q", s)
	}
	return c, nil
}

// Luma is the perceived brightness of c in [0,1] (Rec. 601 weights, alpha
// ignored); above ~0.75 a dark mark reads better on it than a white one.
func Luma(c Color) float64 {
	return (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// RGBA builds a color from CSS rgba() components (0-255 channels, 0-1 alpha).
func RGBA(r, g, b uint8, a float64) Color {
	return Color{R: r, G: g, B: b, A: uint8(math.Round(a * 255))}
}

// Fade returns c with its alpha multiplied by a.
func Fade(c Color, a float64) Color {
	c.A = uint8(math.Round(float64(c.A) * a))
	return c
}

// Mix returns the Sass/Less style mix: weight of b blended into a
// (weight 0 → a, 1 → b). Element Plus derives its light/dark scale this way.
func Mix(a, b Color, weight float64) Color {
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-weight) + float64(y)*weight)) }
	return Color{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

// Over composites fg over an opaque bg and returns the opaque result.
func Over(fg, bg Color) Color {
	a := float64(fg.A) / 255
	l := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	return Color{R: l(fg.R, bg.R), G: l(fg.G, bg.G), B: l(fg.B, bg.B), A: 255}
}

// Lerp interpolates every channel including alpha, t in [0,1].
func Lerp(a, b Color, t float32) Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return Color{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

// HSV is hue [0,360), saturation and value [0,1].
type HSV struct{ H, S, V float64 }

// ToHSV converts an RGB color (alpha ignored), matching tinycolor2.
func ToHSV(c Color) HSV {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	d := mx - mn
	var h float64
	s := 0.0
	if mx != 0 {
		s = d / mx
	}
	if d != 0 {
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
	return HSV{h, s, mx}
}

// FromHSV converts back to an opaque RGB color.
func FromHSV(h HSV) Color {
	hh := math.Mod(h.H, 360) / 60
	if hh < 0 {
		hh += 6
	}
	i := math.Floor(hh)
	f := hh - i
	p := h.V * (1 - h.S)
	q := h.V * (1 - f*h.S)
	t := h.V * (1 - (1-f)*h.S)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = h.V, t, p
	case 1:
		r, g, b = q, h.V, p
	case 2:
		r, g, b = p, h.V, t
	case 3:
		r, g, b = p, q, h.V
	case 4:
		r, g, b = t, p, h.V
	default:
		r, g, b = h.V, p, q
	}
	cv := func(x float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, x)) * 255)) }
	return Color{R: cv(r), G: cv(g), B: cv(b), A: 255}
}

// HSVA is HSV plus alpha in [0,1], the model color pickers edit.
type HSVA struct{ H, S, V, A float64 }

// ToHSVA converts c, keeping its alpha.
func ToHSVA(c Color) HSVA {
	h := ToHSV(c)
	return HSVA{H: h.H, S: h.S, V: h.V, A: float64(c.A) / 255}
}

// FromHSVA converts back to RGB with alpha (clamped to [0,1]).
func FromHSVA(h HSVA) Color {
	c := FromHSV(HSV{H: h.H, S: h.S, V: h.V})
	c.A = uint8(math.Round(clamp01(h.A) * 255))
	return c
}
