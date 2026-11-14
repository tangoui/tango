package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// OKLCH converts an OKLCH color to sRGB: l is lightness in [0,1], c the
// chroma and h the hue in degrees. Channels outside the sRGB gamut are
// clamped. alpha is in [0,1].
func OKLCH(l, c, h, alpha float64) Color {
	hr := h * math.Pi / 180
	a, b := c*math.Cos(hr), c*math.Sin(hr)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	L, M, S := l_*l_*l_, m_*m_*m_, s_*s_*s_
	r := 4.0767416621*L - 3.3077115913*M + 0.2309699292*S
	g := -1.2684380046*L + 2.6097574011*M - 0.3413193965*S
	bl := -0.0041960863*L - 0.7034186147*M + 1.7076147010*S
	return Color{R: srgb8(r), G: srgb8(g), B: srgb8(bl), A: uint8(math.Round(clamp01(alpha) * 255))}
}

func srgb8(x float64) uint8 {
	x = clamp01(x)
	if x <= 0.0031308 {
		x *= 12.92
	} else {
		x = 1.055*math.Pow(x, 1/2.4) - 0.055
	}
	return uint8(math.Round(clamp01(x) * 255))
}

// ParseOKLCH parses CSS oklch(): "oklch(0.205 0 0)", "oklch(98.5% 0 none)",
// "oklch(1 0 0 / 10%)" or "oklch(1 0 0 / 0.15)". Lightness may be a
// number in [0,1] or a percentage; the hue may be "none".
func ParseOKLCH(s string) (Color, error) {
	bad := fmt.Errorf("theme: bad oklch color %q", s)
	t := strings.TrimSpace(s)
	if len(t) < 6 || !strings.EqualFold(t[:6], "oklch(") || !strings.HasSuffix(t, ")") {
		return Color{}, bad
	}
	body := t[6 : len(t)-1]
	alpha := 1.0
	if i := strings.IndexByte(body, '/'); i >= 0 {
		a, ok := cssNumber(strings.TrimSpace(body[i+1:]), 1)
		if !ok {
			return Color{}, bad
		}
		alpha, body = a, body[:i]
	}
	f := strings.Fields(body)
	if len(f) != 3 {
		return Color{}, bad
	}
	l, ok1 := cssNumber(f[0], 1)
	c, ok2 := cssNumber(f[1], 0.4) // CSS: 100% chroma = 0.4
	h, ok3 := 0.0, true
	if !strings.EqualFold(f[2], "none") {
		h, ok3 = cssNumber(strings.TrimSuffix(f[2], "deg"), 1)
	}
	if !ok1 || !ok2 || !ok3 {
		return Color{}, bad
	}
	return OKLCH(l, c, h, alpha), nil
}

// cssNumber parses a number or a percentage; pct100 is the value 100% maps to.
func cssNumber(s string, pct100 float64) (float64, bool) {
	if strings.EqualFold(s, "none") {
		return 0, true
	}
	scale := 1.0
	if strings.HasSuffix(s, "%") {
		s, scale = s[:len(s)-1], pct100/100
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v * scale, true
}
