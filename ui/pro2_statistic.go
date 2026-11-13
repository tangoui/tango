package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// StatisticCard (Ant Design Pro StatisticCard)
//
//	ui.StatisticCard(c, "Sales").Value(126560, 0).Prefix("¥").Trend("up").
//		Desc("Week", "12%", "up").Desc("Day", "11%", "down").
//		Chart(func() { chart.TinyArea(c, vals).Show() }).Show()
//
// The ui package cannot import ui/chart (chart builds on ui), so the chart
// area is a slot: pass a function that builds any chart (chart.TinyArea,
// chart.TinyColumn, chart.TinyProgress …) or other content.

// Chart placements of StatisticCard.ChartPlace.
const (
	ChartBottom = "bottom"
	ChartLeft   = "left"
	ChartRight  = "right"
)

// StatisticCardW builds a StatisticCard.
type StatisticCardW struct {
	c          *Context
	title      string
	value      string
	prefix     string
	suffix     string
	tip        string
	status     string
	trend      string
	ic         *icon.Icon
	valueColor theme.Color
	colorSet   bool
	upColor    theme.Color
	downColor  theme.Color
	trendSet   bool
	descs      []pro2StatDesc
	desc       func()
	chart      func()
	chartPlace string
	footer     func()
	card       *ProCardW
}

type pro2StatDesc struct{ title, value, trend string }

// StatisticCard starts a statistic card titled title (the statistic's
// label; use CardTitle for a card header). Chain options, then Show.
func StatisticCard(c *Context, title string) *StatisticCardW {
	w := &StatisticCardW{c: c, title: title, chartPlace: ChartBottom, card: ProCard(c, "")}
	// ConfigProvider.Defaults("StatisticCard", ...) keys: "bordered",
	// "hoverable" (bool), "chartPlacement" ("bottom" | "left" | "right").
	if v, ok := p2bBool(c, "StatisticCard", "bordered"); ok {
		w.card.bordered, w.card.borderSet = v, true
	}
	if Prop(c, "StatisticCard", "hoverable", false) {
		w.card.hoverable = true
	}
	w.chartPlace = Prop(c, "StatisticCard", "chartPlacement", w.chartPlace)
	return w
}

// Value sets a number formatted with thousands separators.
func (w *StatisticCardW) Value(v float64, precision int) *StatisticCardW {
	w.value = FormatNumber(v, precision)
	return w
}

// ValueText sets the value as preformatted text.
func (w *StatisticCardW) ValueText(s string) *StatisticCardW { w.value = s; return w }

// Prefix sets text before the value (e.g. a currency sign).
func (w *StatisticCardW) Prefix(s string) *StatisticCardW { w.prefix = s; return w }

// Suffix sets text after the value (e.g. a unit).
func (w *StatisticCardW) Suffix(s string) *StatisticCardW { w.suffix = s; return w }

// Tooltip adds a question icon after the title showing tip on hover.
func (w *StatisticCardW) Tooltip(tip string) *StatisticCardW { w.tip = tip; return w }

// Status shows a colored dot before the title ("success", "processing",
// "warning", "error", "default").
func (w *StatisticCardW) Status(s string) *StatisticCardW { w.status = s; return w }

// Trend marks the value as rising ("up") or falling ("down").
func (w *StatisticCardW) Trend(dir string) *StatisticCardW { w.trend = dir; return w }

// TrendColors overrides the rising / falling colors (default: error red
// for up, success green for down, as in Pro).
func (w *StatisticCardW) TrendColors(up, down theme.Color) *StatisticCardW {
	w.upColor, w.downColor, w.trendSet = up, down, true
	return w
}

// Icon shows an icon left of the statistic.
func (w *StatisticCardW) Icon(ic *icon.Icon) *StatisticCardW { w.ic = ic; return w }

// ValueColor colors the value.
func (w *StatisticCardW) ValueColor(col theme.Color) *StatisticCardW {
	w.valueColor, w.colorSet = col, true
	return w
}

// Desc adds an inline statistic under the value ("Week 12% ▲"); trend is
// "up", "down" or "".
func (w *StatisticCardW) Desc(title, value, trend string) *StatisticCardW {
	w.descs = append(w.descs, pro2StatDesc{title, value, trend})
	return w
}

// Description builds custom content under the value.
func (w *StatisticCardW) Description(fn func()) *StatisticCardW { w.desc = fn; return w }

// Chart builds the chart area (a chart.Tiny*, any chart, or content).
func (w *StatisticCardW) Chart(fn func()) *StatisticCardW { w.chart = fn; return w }

// ChartPlace puts the chart below (ChartBottom, default), left or right
// of the statistic.
func (w *StatisticCardW) ChartPlace(p string) *StatisticCardW { w.chartPlace = p; return w }

// Footer builds a footer below a divider.
func (w *StatisticCardW) Footer(fn func()) *StatisticCardW { w.footer = fn; return w }

// CardTitle shows a card header with title (the statistic keeps its own).
func (w *StatisticCardW) CardTitle(s string) *StatisticCardW { w.card.title = s; return w }

// Extra builds the header actions.
func (w *StatisticCardW) Extra(fn func()) *StatisticCardW { w.card.Extra(fn); return w }

// Loading shows the card's loading placeholder.
func (w *StatisticCardW) Loading(on bool) *StatisticCardW { w.card.Loading(on); return w }

// Bordered draws a border.
func (w *StatisticCardW) Bordered() *StatisticCardW { w.card.Bordered(); return w }

// Hoverable lifts the card on hover.
func (w *StatisticCardW) Hoverable() *StatisticCardW { w.card.Hoverable(); return w }

// ColSpan sizes the card in a parent ProCard / StatisticCardGroup row.
func (w *StatisticCardW) ColSpan(span int) *StatisticCardW { w.card.ColSpan(span); return w }

// Layout applies layout options to the card.
func (w *StatisticCardW) Layout(o ...layout.Option) *StatisticCardW { w.card.Layout(o...); return w }

// Card returns the underlying ProCard (valid after Show for queries).
func (w *StatisticCardW) Card() *ProCardW { return w.card }

// Clicked reports a click on a hoverable card.
func (w *StatisticCardW) Clicked() bool { return w.card.Clicked() }

func (w *StatisticCardW) trendColor(dir string) theme.Color {
	th := w.c.Theme
	up, down := th.Error.Base, th.Success.Base
	if w.trendSet {
		up, down = w.upColor, w.downColor
	}
	if dir == "down" {
		return down
	}
	return up
}

// trendMark adds the small trend triangle.
func (w *StatisticCardW) trendMark(dir string, size float32) {
	c := w.c
	if dir != "up" && dir != "down" {
		return
	}
	col := w.trendColor(dir)
	up := dir == "up"
	name := c.Locale().StatisticCard.Down
	if up {
		name = c.Locale().StatisticCard.Up
	}
	n := c.add("pro2-trend", dir)
	n.ln.Width, n.ln.Height = size, size
	n.ln.AlignSelf = layout.AlignCenter
	n.setRole(a11y.RoleImage).setName(name)
	n.paint = func(l *paint.List, r geom.Rect) {
		tw, tht := r.W*0.75, r.H*0.5
		pro2Triangle(l, geom.Rect{X: r.X + (r.W-tw)/2, Y: r.Y + (r.H-tht)/2, W: tw, H: tht}, up, col)
	}
}

// Show builds the card.
func (w *StatisticCardW) Show() *StatisticCardW {
	c := w.c
	w.card.Body(func() {
		if w.chart == nil || w.chartPlace == ChartBottom {
			Column(c, func() {
				w.statistic()
				if w.chart != nil {
					Column(c, w.chart, layout.Margin(geom.Insets{Top: 8}))
				}
			})
		} else {
			Row(c, func() {
				if w.chartPlace == ChartLeft {
					Column(c, w.chart, layout.Margin(geom.Insets{Right: 24}))
				}
				Column(c, w.statistic, layout.Grow(1), layout.MinW(0))
				if w.chartPlace == ChartRight {
					Column(c, w.chart, layout.Margin(geom.Insets{Left: 24}))
				}
			}, layout.Middle())
		}
		if w.footer != nil && pro2Shad(c) {
			// CardFooter: muted text-sm, no divider.
			Column(c, w.footer, layout.Gap(6), layout.Margin(geom.Insets{Top: 16}))
		} else if w.footer != nil {
			th := c.Theme
			Column(c, w.footer, layout.Margin(geom.Insets{Top: 16}), layout.Padding(geom.Insets{Top: 16})).
				Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, th.Split) })
		}
	})
	return w
}

// statistic builds title, value and description.
func (w *StatisticCardW) statistic() {
	c, th := w.c, w.c.Theme
	tk := &c.ThemeFor(theme.CompStatistic).Components.Statistic
	titleSize, valueSize, valueLH := tk.TitleFontSize, tk.ContentFontSize, tk.ContentLineHeight
	titleCol := th.TextSecondary
	shad := pro2Shad(c)
	if shad {
		// CardDescription + text-2xl font-semibold tabular-nums.
		_, titleCol = shad2dMuted(th)
	}
	Row(c, func() {
		if w.ic != nil {
			Icon(c, w.ic).Size(valueLH).Color(th.Primary.Base)
		}
		col := Column(c, func() {
			Row(c, func() {
				if w.status != "" {
					p := th.ColorOf(w.status)
					if w.status == "default" {
						p.Base = th.TextQuaternary
					}
					dot := p.Base
					Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 6, H: 6} }, func(l *paint.List, r geom.Rect) {
						l.Fill(r, geom.Rad(3), dot)
					}, layout.Self(layout.AlignCenter))
				}
				Text(c, w.title).Size(titleSize).Color(titleCol).NoWrap().Ellipsis().Layout(layout.MinW(0))
				if w.tip != "" {
					Tooltip(c, w.tip, func() {
						Icon(c, icon.Question).Size(titleSize).Color(th.TextTertiary)
					}).Semantics(a11y.RoleImage, c.Locale().ProCard.MoreInfo)
				}
			}, layout.Gap(6), layout.Middle(), layout.Margin(geom.Insets{Bottom: 4}))
			Row(c, func() {
				w.trendMark(w.trend, 16)
				vcol := tk.ContentColor
				if w.colorSet {
					vcol = w.valueColor
				}
				if w.prefix != "" {
					Text(c, w.prefix).Size(valueSize).LineHeight(valueLH).Color(vcol).NoWrap()
				}
				v := Text(c, w.value).Size(valueSize).LineHeight(valueLH).Color(vcol).NoWrap()
				if shad {
					v.Weight(text.Semibold)
				}
				if w.suffix != "" {
					Text(c, w.suffix).Size(th.FontSize).Color(th.TextSecondary).NoWrap().
						Layout(layout.Self(layout.AlignEnd), layout.Margin(geom.Insets{Bottom: 4}))
				}
			}, layout.Gap(4), layout.Middle())
			if len(w.descs) > 0 {
				Row(c, func() {
					for _, d := range w.descs {
						Row(c, func() {
							Text(c, d.title).Color(titleCol).NoWrap()
							Text(c, d.value).NoWrap()
							w.trendMark(d.trend, 12)
						}, layout.Gap(6), layout.Middle())
					}
				}, layout.GapXY(16, 4), layout.WrapItems(), layout.Margin(geom.Insets{Top: 8}))
			}
			if w.desc != nil {
				Column(c, w.desc, layout.Margin(geom.Insets{Top: 8}))
			}
		}, layout.Grow(1), layout.MinW(0))
		col.Semantics(a11y.RoleGroup, w.title)
	}, layout.Gap(16), layout.Middle())
}

// StatisticCardGroup is a ProCard holding StatisticCards side by side on
// one surface with vertical dividers (StatisticCard.Group); set
// Direction("column") / Split(SplitHorizontal) to stack them.
func StatisticCardGroup(c *Context) *ProCardW {
	return ProCard(c, "").Split(SplitVertical)
}
