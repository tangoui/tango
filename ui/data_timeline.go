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

// TimelineItem is one event of a Timeline.
type TimelineItem struct {
	Content string // main text
	Body    func() // custom content (replaces Content)
	Label   string // Ant: text on the opposite side of the axis
	Time    string // Element: timestamp under (or above) the content
	TimeTop bool   // put Time above the content
	// Color is a status ("primary", "success", "warning", "error", "info",
	// "gray") or a hex color; empty uses the theme default.
	Color   string
	DotIcon *icon.Icon // icon instead of the circle
	Dot     func()     // fully custom dot node
	Hollow  bool       // Element: hollow dot
	Large   bool       // Element: larger dot
}

// TimelineW shows events along a vertical axis.
type TimelineW struct {
	c       *Context
	items   []TimelineItem
	mode    string
	pending string
	reverse bool
	box     *Box
	th      *theme.Theme
	tk      *theme.TimelineToken
}

// Timeline starts a timeline; finish with Show.
//
// ConfigProvider.Defaults("Timeline", ...) keys: "mode" ("left" |
// "right" | "alternate" | "start" | "end"), "reverse" (bool).
func Timeline(c *Context, items []TimelineItem) *TimelineW {
	th := c.ThemeFor(theme.CompTimeline)
	return &TimelineW{c: c, items: items, mode: Prop(c, "Timeline", "mode", "left"), reverse: Prop(c, "Timeline", "reverse", false),
		th: th, tk: &th.Components.Timeline}
}

// Mode places the content: "left" or "start" (axis at the inline start,
// default), "right" or "end" (axis at the inline end) or "alternate"
// (content alternates around a centered axis). Items with a Label also
// center the axis. The sides are logical: in RTL "left" puts the axis at
// the right.
func (w *TimelineW) Mode(m string) *TimelineW { w.mode = m; return w }

// Pending appends a "still in progress" item with a spinning dot.
func (w *TimelineW) Pending(label string) *TimelineW { w.pending = label; return w }

// Reverse lists the items in reverse order.
func (w *TimelineW) Reverse() *TimelineW { w.reverse = true; return w }

// Layout applies layout options to the timeline (call after Show).
func (w *TimelineW) Layout(o ...layout.Option) *TimelineW {
	if w.box != nil {
		w.box.Style(o...)
	}
	return w
}

type tlEntry struct {
	TimelineItem
	pending bool
}

func (w *TimelineW) dotColor(it tlEntry) theme.Color {
	th := w.th
	switch it.Color {
	case "":
		return w.tk.DotColor
	case "primary", "blue":
		return th.Primary.Base
	case "success", "green":
		return th.Success.Base
	case "warning":
		return th.Warning.Base
	case "error", "danger", "red":
		return th.Error.Base
	case "info":
		return th.Info.Base
	case "gray", "grey":
		return th.TextQuaternary
	}
	if col, err := theme.ParseHex(it.Color); err == nil {
		return col
	}
	return th.Primary.Base
}

// Show builds the timeline.
func (w *TimelineW) Show() *TimelineW {
	c := w.c
	entries := make([]tlEntry, 0, len(w.items)+1)
	for _, it := range w.items {
		entries = append(entries, tlEntry{TimelineItem: it})
	}
	if w.reverse {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}
	if w.pending != "" {
		p := tlEntry{TimelineItem: TimelineItem{Content: w.pending}, pending: true}
		if w.reverse {
			entries = append([]tlEntry{p}, entries...)
		} else {
			entries = append(entries, p)
		}
	}
	labelled := false
	for _, e := range entries {
		if e.Label != "" {
			labelled = true
		}
	}
	w.box = Column(c, func() {
		for i, e := range entries {
			e := e
			last := i == len(entries)-1
			Key(c, i, func() { w.item(e, i, last, labelled) })
		}
	})
	w.box.n.setRole(a11y.RoleList)
	return w
}

func (w *TimelineW) item(e tlEntry, idx int, last, labelled bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn // inferred: 1px border axis, filled size-2.5 dots, medium titles
	// Which side does the content go to?
	side := "right" // content after the axis (logical: at its end side)
	centered := labelled || w.mode == "alternate"
	switch w.mode {
	case "right", "end":
		side = "left"
	case "alternate":
		if idx%2 == 1 {
			side = "left"
		}
	}
	axisW := tk.DotSize
	gap := tk.ContentGap       // axis → content
	headTop := tk.DotOffsetTop // dot top inside the first line
	dotD := axisW              // drawn dot diameter; the axis column keeps its width
	if (e.Large || e.DotIcon != nil) && el {
		dotD = tk.DotSizeLG
	}
	pendingDot := e.pending
	col := w.dotColor(e)
	if el && e.DotIcon != nil && e.Color == "" {
		col = th.Primary.Base
	}
	textCol := tk.ContentColor

	content := func(align TextAlign) {
		Column(c, func() {
			if el && e.Time != "" && e.TimeTop {
				Text(c, e.Time).Size(tk.TimeFontSize).Color(tk.TimeColor).Align(align).LineHeight(13).
					Layout(layout.Margin(geom.Insets{Bottom: 8, Top: 4}))
			}
			if e.Body != nil {
				e.Body()
			} else {
				t := Text(c, e.Content).Color(textCol).Align(align)
				if shad && e.Time != "" && !labelled {
					t.Weight(text.Medium) // title over a muted timestamp
				}
			}
			if el && e.Time != "" && !e.TimeTop {
				Text(c, e.Time).Size(tk.TimeFontSize).Color(tk.TimeColor).Align(align).LineHeight(13).
					Layout(layout.Margin(geom.Insets{Top: 8}))
			}
			if !el && e.Time != "" && !labelled {
				Text(c, e.Time).Size(tk.TimeFontSize).Color(tk.TimeColor).Align(align)
			}
		}, layout.Grow(1), layout.MinW(0))
	}
	label := func(align TextAlign) {
		Column(c, func() {
			if e.Label != "" {
				Text(c, e.Label).Color(tk.LabelColor).Align(align)
			}
		}, layout.Grow(1), layout.MinW(0))
	}
	padBottom := tk.ItemPaddingBottom
	if last {
		padBottom = 0
	}
	axis := func() {
		ax := c.box("timeline-axis", layout.Column, []layout.Option{layout.W(axisW), layout.AlignItems(layout.AlignCenter)})
		ax.n.ln.Shrink = 0
		ax.n.aHidden = true // dots and tail are decoration
		ax.Paint(func(l *paint.List, r geom.Rect) {
			cx := r.X + r.W/2
			// Tail: from this head to the next item's head.
			if !last {
				tc, lw := tk.TailColor, tk.TailWidth
				y0, y1 := r.Y+headTop+axisW/2, r.Bottom()+padBottom+headTop
				if pendingDot || (w.pending != "" && idx == len(w.items)-1 && !w.reverse) {
					// Dashed tail towards the pending item (Ant style).
					if !el {
						p := paint.NewPath().MoveTo(cx, y0).LineTo(cx, y1)
						l.StrokePath(p, paint.StrokeStyle{Width: lw, Dash: []float32{4, 3}}, tc)
						return
					}
				}
				l.Fill(geom.Rect{X: cx - lw/2, Y: y0, W: lw, H: y1 - y0}, geom.Radii{}, tc)
			}
		})
		ax.n.over = func(l *paint.List, r geom.Rect) {
			cx := r.X + r.W/2
			cy := r.Y + headTop + axisW/2
			if e.Dot != nil {
				return
			}
			switch {
			case pendingDot:
				s := float32(14)
				icon.Draw(l, icon.Loading, geom.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s}, tk.PendingColor, spinAngle(c))
			case e.DotIcon != nil:
				s := tk.IconDotSize
				if el {
					l.Path(paint.NewPath().Circle(cx, cy, dotD/2), col)
					c.paintIcon(l, e.DotIcon, geom.Rect{X: cx - 5, Y: cy - 5, W: 10, H: 10}, theme.Hex("#ffffff"), 0)
					return
				}
				l.Fill(geom.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s}, geom.Radii{}, tk.DotBg)
				c.paintIcon(l, e.DotIcon, geom.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s}, col, 0)
			case el:
				rad := dotD / 2
				if e.Hollow {
					bw := tk.DotBorderWidth
					l.Path(paint.NewPath().Circle(cx, cy, rad), tk.DotBg)
					l.StrokePath(paint.NewPath().Circle(cx, cy, rad-bw/2), paint.StrokeStyle{Width: bw}, col)
				} else {
					l.Path(paint.NewPath().Circle(cx, cy, rad), col)
				}
			case shad:
				// size-2.5 rounded-full with a ring-4 of the background
				// that cuts the axis around the dot.
				l.Path(paint.NewPath().Circle(cx, cy, dotD/2+2), tk.DotBg)
				l.Path(paint.NewPath().Circle(cx, cy, dotD/2), col)
			default:
				bw := tk.DotBorderWidth
				l.Path(paint.NewPath().Circle(cx, cy, dotD/2), tk.DotBg)
				l.StrokePath(paint.NewPath().Circle(cx, cy, dotD/2-bw/2), paint.StrokeStyle{Width: bw}, col)
			}
		}
		if e.Dot != nil {
			ax.Body(func() {
				Column(c, e.Dot, layout.Margin(geom.Insets{Top: headTop - 4}))
			})
			ax.n.ln.Align = layout.AlignCenter
			ax.Paint(func(l *paint.List, r geom.Rect) {})
		}
	}
	minH := float32(0)
	if last && !el {
		minH = 48 // Ant keeps room under the last event
	}
	it := Row(c, func() {
		switch {
		// The row mirrors in RTL; texts align towards the axis.
		case centered && side == "right":
			label(AlignEnd)
			Gap(c, gap)
			axis()
			Gap(c, gap)
			content(AlignStart)
		case centered:
			content(AlignEnd)
			Gap(c, gap)
			axis()
			Gap(c, gap)
			label(AlignStart)
		case side == "left":
			content(AlignEnd)
			Gap(c, gap)
			axis()
		default:
			axis()
			Gap(c, gap)
			content(AlignStart)
		}
	}, layout.Padding(geom.Insets{Bottom: padBottom}), layout.MinH(minH))
	it.n.setRole(a11y.RoleListItem).stateIf(e.pending, a11y.Busy)
}
