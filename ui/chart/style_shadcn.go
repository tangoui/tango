package chart

import (
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func init() {
	RegisterStyle(func(t *theme.Theme) bool { return t.Flavor == theme.StyleShadcn }, ShadcnStyle)
}

// ShadcnStyle is the shadcn/ui chart look (Recharts with shadcn's chart
// conventions): the --chart-1 … --chart-5 palette, horizontal grid lines
// only, no axis or tick lines, 12px muted labels, a bordered rounded
// tooltip with square markers, rounded bars and gradient areas. It is
// registered for theme.StyleShadcn themes.
func ShadcnStyle(th *theme.Theme) *Style {
	s := AntStyle(th)
	s.Name = "shadcn"
	bg := th.BgContainer
	if v := th.Shadcn; v != nil {
		s.Colors = append([]theme.Color(nil), v.Chart[:]...)
		// More series than five: continue with lighter steps of the same
		// hues so the sequence stays on palette.
		for _, c := range v.Chart {
			s.Colors = append(s.Colors, theme.Mix(c, bg, 0.4))
		}
	}
	muted := th.TextTertiary
	s.Font = th.FontSizeSM
	s.Label = muted
	s.Title = muted
	s.AxisLine = th.Split
	s.Tick = th.Split
	s.Grid = th.Split
	s.GridDash = nil
	s.XAxisLine = false
	s.YAxisLine = false
	s.TickLen = 0
	s.LabelGap = 8
	s.LegendPos = LegendBottom
	s.LegendCenter = true
	s.LegendText = th.Text
	s.LegendOff = th.TextDisabled
	s.LegendMarker = MarkerSquare
	s.LegendGap = 16
	s.TooltipBg = th.BgElevated
	s.TooltipTitle = th.Text
	s.TooltipName = muted
	s.TooltipValue = th.Text
	s.TooltipFont = th.FontSizeSM
	s.TooltipPad = 10
	s.TooltipRadius = th.RadiusLG
	s.TooltipShadow = []paint.Shadow{
		{Y: 20, Blur: 25, Spread: -5, Color: theme.RGBA(0, 0, 0, .1)},
		{Y: 8, Blur: 10, Spread: -6, Color: theme.RGBA(0, 0, 0, .1)},
	} // shadow-xl
	s.TooltipBold = true
	s.TooltipLine = 20
	s.Crosshair = th.Fill
	s.CrosshairDash = nil
	s.BandHover = th.FillSecondary
	s.Surface = bg
	s.LineWidth = 2
	s.ShowPoints = false
	s.PointRadius = 3
	s.AreaOpacity = 0.4
	s.AreaGrad = true
	s.BarPadding = 0.2
	s.BarOuter = 0.1
	s.BarRadius = 4
	s.SliceStroke = 2
	s.Track = th.FillSecondary
	s.Positive = th.Error.Base
	s.Negative = th.Success.Base
	if len(s.Colors) > 0 {
		c := s.Colors[0]
		s.Sequential = []theme.Color{theme.Mix(bg, c, .15), theme.Mix(bg, c, .35), theme.Mix(bg, c, .55), theme.Mix(bg, c, .8), c}
	}
	return s
}
