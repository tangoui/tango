package chart

import (
	"sync"

	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// LegendPos places the legend.
type LegendPos uint8

const (
	LegendAuto   LegendPos = iota // style default (Ant: top-left, Element: top-centre)
	LegendTop                     // above the plot, aligned by Style.LegendAlign
	LegendBottom                  // below the plot, centred
	LegendRight                   // inline end of the plot (left in RTL scopes), vertically centred
	LegendLeft                    // inline start of the plot (right in RTL scopes)
	LegendNone                    // hidden
)

// Marker is a legend / tooltip marker shape.
type Marker uint8

const (
	MarkerCircle    Marker = iota // dot (G2)
	MarkerRoundRect               // 25×14 rounded rect (ECharts)
	MarkerLine                    // line with a hollow dot (ECharts line series)
	MarkerSquare
)

// Style is the visual language of charts: palette, axis, legend and tooltip
// tokens. StyleOf derives it from the UI theme; RegisterStyle adds new
// design languages (a shadcn style, a brand palette) without touching the
// charts.
type Style struct {
	Name string

	Colors     []theme.Color // categorical palette, cycled
	Sequential []theme.Color // low → high (heatmap, continuous legend)

	Font      float32     // axis / legend text size
	Label     theme.Color // axis tick labels
	Title     theme.Color // axis titles, statistic captions
	AxisLine  theme.Color // axis base line
	Tick      theme.Color // tick marks
	Grid      theme.Color // grid lines
	GridDash  []float32   // nil = solid
	XAxisLine bool        // draw the bottom axis line (G2 relies on the 0 grid line)
	YAxisLine bool
	TickLen   float32 // 0 hides tick marks
	LabelGap  float32 // tick (or axis) to label

	LegendPos    LegendPos // default placement for multi-series charts
	LegendCenter bool      // centre a top legend (ECharts)
	LegendText   theme.Color
	LegendOff    theme.Color // hidden series
	LegendMarker Marker
	LegendGap    float32 // between items

	TooltipBg     theme.Color
	TooltipTitle  theme.Color
	TooltipName   theme.Color
	TooltipValue  theme.Color
	TooltipFont   float32
	TooltipPad    float32
	TooltipRadius float32
	TooltipShadow []paint.Shadow
	TooltipBold   bool // bold values (ECharts)
	TooltipLine   float32

	Crosshair     theme.Color
	CrosshairDash []float32
	BandHover     theme.Color // background of the hovered band (bars)

	Surface     theme.Color // chart background, used for halo strokes
	LineWidth   float32
	ShowPoints  bool    // draw hollow symbols on lines (ECharts default)
	PointRadius float32 // symbol radius
	AreaOpacity float64 // flat area fill opacity
	AreaGrad    bool    // fade area fills to transparent (G2 look)
	BarPadding  float64 // inner band padding
	BarOuter    float64 // outer band padding
	BarRadius   float32
	PointPad    bool    // lines over categories sit on band centres (ECharts boundaryGap)
	SliceStroke float32 // white separators between pie slices
	Track       theme.Color
	Positive    theme.Color
	Negative    theme.Color
}

// Color returns palette color i (cycled).
func (s *Style) Color(i int) theme.Color {
	if len(s.Colors) == 0 {
		return theme.Hex("#1677ff")
	}
	if i < 0 {
		i = -i
	}
	return s.Colors[i%len(s.Colors)]
}

// StyleFunc derives a chart style from a UI theme.
type StyleFunc func(th *theme.Theme) *Style

type styleReg struct {
	match func(*theme.Theme) bool
	fn    StyleFunc
}

var (
	styleMu   sync.RWMutex
	styleRegs []styleReg
)

// RegisterStyle installs a chart style for themes accepted by match; later
// registrations take precedence. Built-in styles: Ant (G2 classic) for
// theme.Ant and ECharts for theme.Element, each with a dark variant.
//
//	chart.RegisterStyle(func(t *theme.Theme) bool { return t.Name == "shadcn" },
//		func(t *theme.Theme) *chart.Style {
//			s := chart.AntStyle(t)
//			s.Colors = shadcnPalette
//			return s
//		})
func RegisterStyle(match func(*theme.Theme) bool, fn StyleFunc) {
	styleMu.Lock()
	styleRegs = append(styleRegs, styleReg{match, fn})
	styleMu.Unlock()
}

// StyleOf returns the chart style for th.
func StyleOf(th *theme.Theme) *Style {
	styleMu.RLock()
	for i := len(styleRegs) - 1; i >= 0; i-- {
		if r := styleRegs[i]; r.match(th) {
			styleMu.RUnlock()
			return r.fn(th)
		}
	}
	styleMu.RUnlock()
	if th.Flavor == theme.Element {
		return EChartsStyle(th)
	}
	return AntStyle(th)
}

func hexes(s ...string) []theme.Color {
	out := make([]theme.Color, len(s))
	for i, h := range s {
		out[i] = theme.Hex(h)
	}
	return out
}

// G2 v5 "classic" categorical palette (category10).
var g2Palette = []string{"#1783FF", "#00C9C9", "#F0884D", "#D580FF", "#7863FF", "#60C42D", "#BD8F24", "#FF80CA", "#2491B3", "#17C76F"}

// ECharts 5 default and dark-theme palettes.
var (
	echartsPalette     = []string{"#5470c6", "#91cc75", "#fac858", "#ee6666", "#73c0de", "#3ba272", "#fc8452", "#9a60b4", "#ea7ccc"}
	echartsDarkPalette = []string{"#4992ff", "#7cffb2", "#fddd60", "#ff6e76", "#58d9f9", "#05c091", "#ff8a45", "#8d48e3", "#dd79ff"}
)

// opaque drops alpha by compositing over the container background.
func opaque(c, bg theme.Color) theme.Color { return theme.Over(c, bg) }

// darkAdapt lifts dim, saturated hues so they keep contrast on dark
// surfaces (the G2 palette is tuned for white).
func darkAdapt(cs []theme.Color) []theme.Color {
	out := make([]theme.Color, len(cs))
	for i, c := range cs {
		h := theme.ToHSV(c)
		if h.V < 0.9 {
			h.V = min(1, h.V+0.08)
		}
		h.S *= 0.9
		out[i] = theme.FromHSV(h)
	}
	return out
}

// AntStyle is the G2 (Ant Design Charts v2) look derived from an Ant theme.
func AntStyle(th *theme.Theme) *Style {
	ink := th.Text
	ink.A = 255
	bg := th.BgContainer
	s := &Style{
		Name:          "ant",
		Colors:        hexes(g2Palette...),
		Font:          th.FontSizeSM,
		Label:         opaque(theme.Fade(ink, 0.45), bg),
		Title:         opaque(theme.Fade(ink, 0.65), bg),
		AxisLine:      opaque(theme.Fade(ink, 0.25), bg),
		Tick:          opaque(theme.Fade(ink, 0.25), bg),
		Grid:          opaque(theme.Fade(ink, 0.1), bg),
		XAxisLine:     false,
		TickLen:       4,
		LabelGap:      4,
		LegendPos:     LegendTop,
		LegendText:    opaque(theme.Fade(ink, 0.65), bg),
		LegendOff:     opaque(theme.Fade(ink, 0.25), bg),
		LegendMarker:  MarkerCircle,
		LegendGap:     16,
		TooltipBg:     th.BgElevated,
		TooltipTitle:  opaque(theme.Fade(ink, 0.65), th.BgElevated),
		TooltipName:   opaque(theme.Fade(ink, 0.65), th.BgElevated),
		TooltipValue:  opaque(theme.Fade(ink, 0.85), th.BgElevated),
		TooltipFont:   th.FontSizeSM,
		TooltipPad:    12,
		TooltipRadius: th.RadiusSM,
		TooltipShadow: []paint.Shadow{{Y: 6, Blur: 12, Color: theme.RGBA(0, 0, 0, 0.12)}},
		TooltipLine:   20,
		Crosshair:     opaque(theme.Fade(ink, 0.25), bg),
		BandHover:     theme.Fade(ink, 0.05),
		Surface:       bg,
		LineWidth:     2,
		PointRadius:   3,
		AreaOpacity:   0.25,
		AreaGrad:      true,
		BarPadding:    0.1,
		BarOuter:      0.05,
		SliceStroke:   1,
		Track:         opaque(theme.Fade(ink, 0.06), bg),
		Positive:      th.Error.Base, // Chinese convention: red up
		Negative:      th.Success.Base,
	}
	s.Sequential = []theme.Color{th.Primary.Shades[0], th.Primary.Shades[2], th.Primary.Shades[4], th.Primary.Shades[6], th.Primary.Shades[8]}
	if th.Dark {
		// The theme has no chart palette token; G2's palette is tuned for
		// white, so dark themes lift it (see darkAdapt).
		s.Colors = darkAdapt(s.Colors)
		s.TooltipShadow = []paint.Shadow{{Y: 6, Blur: 12, Color: theme.RGBA(0, 0, 0, 0.45)}}
		// Dark palettes already run dark → bright, so the same steps put
		// high values on the brightest cells.
	}
	return s
}

// EChartsStyle is the ECharts 5 default look, used for Element Plus.
func EChartsStyle(th *theme.Theme) *Style {
	bg := th.BgContainer
	s := &Style{
		Name:          "echarts",
		Colors:        hexes(echartsPalette...),
		Sequential:    hexes("#f6efa6", "#e9b88d", "#d88273", "#c9625e", "#bf444c"),
		Font:          th.FontSizeSM,
		Label:         theme.Hex("#6E7079"),
		Title:         theme.Hex("#464646"),
		AxisLine:      theme.Hex("#6E7079"),
		Tick:          theme.Hex("#6E7079"),
		Grid:          theme.Hex("#E0E6F1"),
		XAxisLine:     true,
		TickLen:       5,
		LabelGap:      3,
		LegendPos:     LegendTop,
		LegendCenter:  true,
		LegendText:    theme.Hex("#333333"),
		LegendOff:     theme.Hex("#cccccc"),
		LegendMarker:  MarkerRoundRect,
		LegendGap:     10,
		TooltipBg:     th.BgElevated,
		TooltipTitle:  theme.Hex("#666666"),
		TooltipName:   theme.Hex("#666666"),
		TooltipValue:  theme.Hex("#666666"),
		TooltipFont:   th.FontSize,
		TooltipPad:    10,
		TooltipRadius: 4,
		TooltipShadow: []paint.Shadow{{X: 1, Y: 2, Blur: 10, Color: theme.RGBA(0, 0, 0, 0.2)}},
		TooltipBold:   true,
		TooltipLine:   24,
		Crosshair:     theme.Hex("#B9BEC9"),
		CrosshairDash: []float32{4, 4},
		BandHover:     theme.RGBA(150, 150, 150, 0.2),
		Surface:       bg,
		LineWidth:     2,
		ShowPoints:    true,
		PointRadius:   2.5,
		AreaOpacity:   0.7,
		BarPadding:    0.2,
		BarOuter:      0.1,
		PointPad:      true,
		Track:         theme.Hex("#E6EBF8"),
		Positive:      th.Error.Base,
		Negative:      th.Success.Base,
	}
	if th.Dark {
		// ECharts ships a separate dark theme; the UI theme has no chart
		// tokens, so its values are selected here.
		s.Colors = hexes(echartsDarkPalette...)
		s.Label = theme.Hex("#B9B8CE")
		s.Title = theme.Hex("#EEF1FA")
		s.AxisLine = theme.Hex("#B9B8CE")
		s.Tick = theme.Hex("#B9B8CE")
		s.Grid = theme.Hex("#484753")
		s.LegendText = theme.Hex("#EEF1FA")
		s.LegendOff = theme.Hex("#5a5a6a")
		s.TooltipTitle = th.TextSecondary
		s.TooltipName = th.TextSecondary
		s.TooltipValue = th.Text
		s.TooltipShadow = []paint.Shadow{{X: 1, Y: 2, Blur: 10, Color: theme.RGBA(0, 0, 0, 0.5)}}
		s.Crosshair = theme.Hex("#817f91")
		s.Track = theme.Hex("#2b2b38")
		s.Sequential = hexes("#3c3a5a", "#5b4f8e", "#8d5aa8", "#c96aa0", "#f0a08a")
	}
	return s
}
