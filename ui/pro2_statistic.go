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

// Statistic layouts of StatisticCard.StatisticLayout and ProStatistic.
const (
	StatisticVertical   = "vertical"
	StatisticHorizontal = "horizontal"
	StatisticInline     = "inline"
)

// Chart placements of StatisticCard.ChartPlace.
const (
	ChartBottom = "bottom"
	ChartLeft   = "left"
	ChartRight  = "right"
)

// StatisticCardW builds a StatisticCard.
type StatisticCardW struct {
	_ [0]func()

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
	statLayout string
	footer     func()
	card       *ProCardW
}

type pro2StatDesc struct{ title, value, trend string }

// StatisticCard starts a statistic card titled title (the statistic's
// label; use CardTitle for a card header). Chain options, then Show.
func StatisticCard(c *Context, title string) *StatisticCardW {
	w := &StatisticCardW{c: c, title: title, value: "0", chartPlace: ChartBottom, statLayout: StatisticVertical, card: ProCard(c, "")}
	// ConfigProvider.Defaults("StatisticCard", ...) keys: "bordered",
	// "hoverable" (bool), "chartPlacement" ("bottom" | "left" | "right").
	if v, ok := p2bBool(c, "StatisticCard", "bordered"); ok {
		w.card.bordered, w.card.borderSet = v, true
	}
	if Prop(c, "StatisticCard", "hoverable", false) {
		w.card.hoverable = true
	}
	w.chartPlace = Prop(c, "StatisticCard", "chartPlacement", w.chartPlace)
	w.StatisticLayout(Prop(c, "StatisticCard", "layout", StatisticVertical))
	return w
}

// Value sets a number formatted with thousands separators.
func (w *StatisticCardW) Value(v float64, precision int) *StatisticCardW {
	w.value = statisticFormat(v, precision, ",", ".")
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

// StatisticLayout sets the statistic layout: vertical (default), horizontal
// (title and value at opposite ends), or inline (compact, secondary value).
func (w *StatisticCardW) StatisticLayout(mode string) *StatisticCardW {
	switch mode {
	case StatisticVertical, StatisticHorizontal, StatisticInline:
		w.statLayout = mode
	}
	return w
}

// Ghost removes the card background and outer padding.
func (w *StatisticCardW) Ghost() *StatisticCardW { w.card.Ghost(); return w }

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
	n.ln.Width, n.ln.Height = 7, 9
	n.ln.AlignSelf = layout.AlignCenter
	n.setRole(a11y.RoleImage).setName(name)
	n.paint = func(l *paint.List, r geom.Rect) {
		pro2Triangle(l, r, up, col)
	}
}

// Show builds the card.
func (w *StatisticCardW) Show() *StatisticCardW {
	c := w.c
	w.card.Body(func() {
		if w.chart == nil || (w.chartPlace != ChartLeft && w.chartPlace != ChartRight) {
			Column(c, func() {
				w.statistic()
				if w.chart != nil {
					Column(c, w.chart, layout.Margin(geom.Insets{Top: 8, Bottom: 8}))
				}
			})
		} else {
			Row(c, func() {
				if w.chartPlace == ChartLeft {
					Column(c, w.chart, layout.Margin(geom.Insets{Right: 16}))
				}
				Column(c, w.statistic, layout.Grow(1), layout.MinW(0))
				if w.chartPlace == ChartRight {
					Column(c, w.chart, layout.Margin(geom.Insets{Left: 16}))
				}
			}, layout.AlignItems(layout.AlignStart))
		}
		if w.footer != nil {
			th := c.Theme
			Column(c, w.footer, layout.Margin(geom.Insets{Top: th.MarginXS}), layout.Padding(geom.Insets{Top: th.Padding})).
				Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, th.Split) })
		}
	})
	return w
}

// statistic renders the shared Pro Statistic. Theme-specific visual values
// come from the existing Statistic token, never from flavor switches.
func (w *StatisticCardW) statistic() {
	c, th := w.c, w.c.Theme
	tk := &c.ThemeFor(theme.CompStatistic).Components.Statistic
	mode := w.statLayout
	titleSize, valueSize, valueLH := tk.TitleFontSize, tk.ContentFontSize, tk.ContentLineHeight
	valueWeight := text.Weight(tk.ContentFontWeight)
	titleCol, valueCol := th.Text, tk.ContentColor
	if mode == StatisticHorizontal {
		titleSize, valueSize, valueLH, valueWeight = th.FontSize, th.FontSize, th.LineHeightFor(th.FontSize), text.Medium
	} else if mode == StatisticInline {
		titleSize, valueSize, valueLH = th.FontSizeSM, th.FontSizeSM, th.LineHeightFor(th.FontSizeSM)
		valueCol = th.TextSecondary
		valueWeight = text.Regular
	} else if w.trend == "up" || w.trend == "down" {
		valueCol = w.trendColor(w.trend)
	}
	if mode == StatisticHorizontal && (w.trend == "up" || w.trend == "down") {
		valueCol = w.trendColor(w.trend)
	}
	if w.colorSet {
		valueCol = w.valueColor
	}
	title := func() {
		Row(c, func() {
			Text(c, w.title).Size(titleSize).Color(titleCol).NoWrap().Ellipsis().Layout(layout.MinW(0))
			if w.tip != "" {
				Column(c, func() {
					Tooltip(c, w.tip, func() { Icon(c, icon.Question).Size(titleSize).Color(th.TextTertiary) }).Semantics(a11y.RoleImage, c.Locale().ProCard.MoreInfo)
				}).Label("pro2-stat-tip")
			}
		}, layout.Gap(4), layout.Middle(), layout.MinW(0))
	}
	value := func() {
		Row(c, func() {
			w.trendMark(w.trend, 9)
			if w.prefix != "" {
				Text(c, w.prefix).Size(valueSize).LineHeight(valueLH).Color(valueCol).NoWrap()
			}
			v := Text(c, w.value).Size(valueSize).LineHeight(valueLH).Color(valueCol).NoWrap()
			if valueWeight != 0 {
				v.Weight(valueWeight)
			}
			if w.suffix != "" {
				Text(c, w.suffix).Size(valueSize).LineHeight(valueLH).Color(valueCol).NoWrap()
			}
		}, layout.Gap(4), layout.Middle())
	}
	root := Row(c, func() {
		if w.ic != nil {
			Column(c, func() { Icon(c, w.ic).Size(valueLH).Color(th.Primary.Base) }, layout.Margin(geom.Insets{Right: 16}))
		}
		if w.status != "" {
			dot := th.ColorOf(w.status).Base
			if w.status == "default" {
				dot = th.TextQuaternary
			}
			Column(c, func() {
				Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 6, H: 6} }, func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Rad(3), dot) }, layout.NoStretch())
			}, layout.W(14), layout.Padding(geom.Insets{Top: (th.LineHeightFor(titleSize) - 6) / 2})).Label("pro2-stat-status")
		}
		Column(c, func() {
			if mode == StatisticHorizontal || mode == StatisticInline {
				opts := []layout.Option{layout.Middle(), layout.Gap(6)}
				if mode == StatisticHorizontal {
					opts = append(opts, layout.Between())
				}
				Row(c, func() { title(); value() }, opts...)
			} else {
				title()
				Column(c, value, layout.Margin(geom.Insets{Top: 4}))
			}
			if len(w.descs) > 0 {
				Column(c, func() {
					for _, d := range w.descs {
						sw := &StatisticCardW{c: c, title: d.title, value: d.value, trend: d.trend, statLayout: StatisticInline, upColor: w.upColor, downColor: w.downColor, trendSet: w.trendSet}
						sw.statistic()
					}
				}, layout.Gap(4))
			}
			if w.desc != nil {
				Column(c, w.desc)
			}
		}, layout.Grow(1), layout.MinW(0))
	}, layout.AlignItems(layout.AlignStart))
	root.Label("pro2-stat-"+w.title).Semantics(a11y.RoleGroup, w.title)
}

// ProStatisticW is the standalone Pro statistic (StatisticCard.Statistic).
// It shares StatisticCard's statistic renderer without adding a card surface.
type ProStatisticW struct{ stat *StatisticCardW }

// ProStatistic starts a compact Pro statistic. ui.Statistic is the antd
// statistic; this entry adds Pro layout, descriptions, status, tip and trend.
func ProStatistic(c *Context, title string) *ProStatisticW {
	w := StatisticCard(c, title)
	w.StatisticLayout(Prop(c, "ProStatistic", "layout", StatisticInline))
	return &ProStatisticW{w}
}

// Value formats a numeric value.
func (w *ProStatisticW) Value(v float64, precision int) *ProStatisticW {
	w.stat.Value(v, precision)
	return w
}

// ValueText sets a preformatted value.
func (w *ProStatisticW) ValueText(s string) *ProStatisticW { w.stat.ValueText(s); return w }

// Prefix adds text before the value.
func (w *ProStatisticW) Prefix(s string) *ProStatisticW { w.stat.Prefix(s); return w }

// Suffix adds text after the value.
func (w *ProStatisticW) Suffix(s string) *ProStatisticW { w.stat.Suffix(s); return w }

// Trend sets the trend direction.
func (w *ProStatisticW) Trend(s string) *ProStatisticW { w.stat.Trend(s); return w }

// Tooltip adds a title tooltip.
func (w *ProStatisticW) Tooltip(s string) *ProStatisticW { w.stat.Tooltip(s); return w }

// Status adds a status dot.
func (w *ProStatisticW) Status(s string) *ProStatisticW { w.stat.Status(s); return w }

// Icon adds a leading icon.
func (w *ProStatisticW) Icon(ic *icon.Icon) *ProStatisticW { w.stat.Icon(ic); return w }

// ValueColor overrides the value color.
func (w *ProStatisticW) ValueColor(col theme.Color) *ProStatisticW {
	w.stat.ValueColor(col)
	return w
}

// TrendColors overrides the trend colors.
func (w *ProStatisticW) TrendColors(up, down theme.Color) *ProStatisticW {
	w.stat.TrendColors(up, down)
	return w
}

// Description builds description content.
func (w *ProStatisticW) Description(fn func()) *ProStatisticW {
	w.stat.Description(fn)
	return w
}

// StatisticLayout sets vertical, horizontal or inline layout.
func (w *ProStatisticW) StatisticLayout(s string) *ProStatisticW {
	w.stat.StatisticLayout(s)
	return w
}

// Show builds a standalone statistic. Consecutive standalone statistics
// have a 4px block gap, as in Pro's `& + &` rule.
func (w *ProStatisticW) Show() *ProStatisticW {
	c := w.stat.c
	previous := pro2LastKid(c)
	w.stat.statistic()
	n := pro2LastKid(c)
	if previous != nil && previous.label == "pro2-standalone-stat" {
		n.ln.Margin.Top = 4
	}
	n.label = "pro2-standalone-stat"
	return w
}

// StatisticCardDividerW is an explicit Pro statistic divider.
type StatisticCardDividerW struct{ box *Box }

// StatisticCardDivider builds StatisticCard.Divider. Empty orientation
// reads the Defaults key "orientation" and otherwise defaults to vertical.
func StatisticCardDivider(c *Context, orientation string) *StatisticCardDividerW {
	if orientation == "" {
		orientation = Prop(c, "StatisticCardDivider", "orientation", SplitVertical)
	}
	th := c.Theme
	opts := []layout.Option{layout.W(th.LineWidth), layout.Self(layout.AlignStretch), layout.Margin(geom.Insets{Left: th.MarginXS, Right: th.MarginXS, Top: th.MarginLG, Bottom: th.MarginLG})}
	if orientation == SplitHorizontal {
		opts = []layout.Option{layout.H(th.LineWidth), layout.Margin(geom.Insets{Left: th.MarginLG, Right: th.MarginLG, Top: th.MarginXS, Bottom: th.MarginXS})}
	}
	b := Column(c, nil, opts...).Label("pro2-stat-divider").Bg(th.Split).Semantics(a11y.RoleSeparator, "")
	return &StatisticCardDividerW{box: b}
}

// Layout applies layout options to the divider.
func (w *StatisticCardDividerW) Layout(opts ...layout.Option) *StatisticCardDividerW {
	w.box.Style(opts...)
	return w
}

// StatisticCardOperationW is an operator between statistics.
type StatisticCardOperationW struct{ box *Box }

// StatisticCardOperation builds StatisticCard.Operation using heading-4
// typography and Pro's marginLG block spacing.
func StatisticCardOperation(c *Context, value string) *StatisticCardOperationW {
	th := c.Theme
	b := Column(c, func() {
		Text(c, value).Size(th.Heading[3]).Weight(text.Weight(th.FontWeightStrong)).Color(th.Text).NoWrap()
	},
		layout.End(), layout.Self(layout.AlignStretch), layout.Margin(geom.Insets{Top: th.MarginLG, Bottom: th.MarginLG})).Label("pro2-stat-op")
	return &StatisticCardOperationW{box: b}
}

// Layout applies layout options to the operation.
func (w *StatisticCardOperationW) Layout(opts ...layout.Option) *StatisticCardOperationW {
	w.box.Style(opts...)
	return w
}

// StatisticCardGroup builds StatisticCard.Group: a ProCard with zero body
// padding. Use Divider for explicit separators, or Split for automatic ones.
func StatisticCardGroup(c *Context) *ProCardW {
	return ProCard(c, "").BodyPadding(geom.Insets{})
}
