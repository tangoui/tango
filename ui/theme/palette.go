package theme

import "math"

// Palette is a semantic color with its derived interaction shades. Field
// names follow Ant Design's map tokens (colorXxxBg, colorXxxHover, ...).
type Palette struct {
	Base        Color // colorPrimary / --el-color-primary
	Hover       Color // pointer over a solid control
	Active      Color // pressed
	Bg          Color // lightest tint: selected rows, alert background
	BgHover     Color // tint under the pointer
	Border      Color // tinted border (alert, plain button)
	BorderHover Color
	TextHover   Color
	Text        Color
	TextActive  Color
	// Shades is the full 10-step scale (Ant: color-1..color-10; Element:
	// light-9,8,7,5,3, base, dark-2 padded with darker steps).
	Shades [10]Color
}

// Ant Design palette generator constants (@ant-design/colors, MIT).
const (
	antHueStep         = 2.0
	antSaturationStep  = 0.16
	antSaturationStep2 = 0.05
	antBrightnessStep1 = 0.05
	antBrightnessStep2 = 0.15
	antLightCount      = 5
	antDarkCount       = 4
)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func antHue(h HSV, i int, light bool) float64 {
	var hue float64
	if math.Round(h.H) >= 60 && math.Round(h.H) <= 240 {
		if light {
			hue = math.Round(h.H) - antHueStep*float64(i)
		} else {
			hue = math.Round(h.H) + antHueStep*float64(i)
		}
	} else {
		if light {
			hue = math.Round(h.H) + antHueStep*float64(i)
		} else {
			hue = math.Round(h.H) - antHueStep*float64(i)
		}
	}
	if hue < 0 {
		hue += 360
	} else if hue >= 360 {
		hue -= 360
	}
	return hue
}

func antSat(h HSV, i int, light bool) float64 {
	if h.H == 0 && h.S == 0 {
		return h.S
	}
	var s float64
	switch {
	case light:
		s = h.S - antSaturationStep*float64(i)
	case i == antDarkCount:
		s = h.S + antSaturationStep
	default:
		s = h.S + antSaturationStep2*float64(i)
	}
	if s > 1 {
		s = 1
	}
	if light && i == antLightCount && s > 0.1 {
		s = 0.1
	}
	if s < 0.06 {
		s = 0.06
	}
	return round2(s)
}

func antVal(h HSV, i int, light bool) float64 {
	var v float64
	if light {
		v = h.V + antBrightnessStep1*float64(i)
	} else {
		v = h.V - antBrightnessStep2*float64(i)
	}
	if v > 1 {
		v = 1
	}
	return round2(v)
}

// AntShades implements the Ant Design color palette algorithm
// (@ant-design/colors generate(), MIT licensed): ten shades where index 5 is
// the input color. For dark themes the light shades are blended onto bg the
// way Ant's dark algorithm does.
func AntShades(c Color, dark bool, bg Color) [10]Color {
	var out [10]Color
	h := ToHSV(c)
	n := 0
	for i := antLightCount; i > 0; i-- {
		out[n] = FromHSV(HSV{antHue(h, i, true), antSat(h, i, true), antVal(h, i, true)})
		n++
	}
	out[n] = Color{R: c.R, G: c.G, B: c.B, A: 255}
	n++
	for i := 1; i <= antDarkCount; i++ {
		out[n] = FromHSV(HSV{antHue(h, i, false), antSat(h, i, false), antVal(h, i, false)})
		n++
	}
	if !dark {
		return out
	}
	// darkColorMap from @ant-design/colors.
	m := [10]struct {
		idx int
		op  float64
	}{{7, .15}, {6, .25}, {5, .3}, {5, .45}, {5, .65}, {5, .85}, {4, .9}, {3, .95}, {2, .97}, {1, .98}}
	var d [10]Color
	for i, e := range m {
		d[i] = Mix(bg, out[e.idx], e.op)
	}
	return d
}

// AntPalette maps the 10-step scale onto semantic tokens exactly like
// antd v5 genColorMapToken.
func AntPalette(c Color, dark bool, bg Color) Palette {
	s := AntShades(c, dark, bg)
	return Palette{
		Base: s[5], Hover: s[4], Active: s[6],
		Bg: s[0], BgHover: s[1], Border: s[2], BorderHover: s[3],
		TextHover: s[4], Text: s[5], TextActive: s[6],
		Shades: s,
	}
}

// ElementPalette follows Element Plus: light-N = mix(white, c, N*10%),
// dark-2 = mix(black, c, 20%). In dark mode the "white" is the page bg.
func ElementPalette(c Color, dark bool, bg Color) Palette {
	white := Hex("#ffffff")
	black := Hex("#000000")
	if dark {
		white = bg
		black = Hex("#ffffff")
	}
	light := func(n int) Color { return Mix(c, white, float64(n)/10) }
	darkC := func(n int) Color { return Mix(c, black, float64(n)/10) }
	s := [10]Color{light(9), light(8), light(7), light(5), light(3), c, darkC(2), darkC(4), darkC(6), darkC(8)}
	return Palette{
		Base: c, Hover: light(3), Active: darkC(2),
		Bg: light(9), BgHover: light(8), Border: light(5), BorderHover: light(3),
		TextHover: light(3), Text: c, TextActive: darkC(2),
		Shades: s,
	}
}
