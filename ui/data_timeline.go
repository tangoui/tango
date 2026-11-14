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
	Content string // main text (Ant v6 content)
	Body    func() // custom content (replaces Content)
	// Title is the text on the opposite side of the axis (Ant v6 title);
	// any item with a Title centers the axis of a vertical timeline.
	Title string
	// Deprecated: use Title.
	Label   string
	Time    string // Element: timestamp under (or above) the content
	TimeTop bool   // put Time above the content
	// Color is a status ("primary", "success", "warning", "error", "info",
	// "gray") or a hex color; empty uses the theme default.
	Color   string
	DotIcon *icon.Icon // icon instead of the circle
	Dot     func()     // fully custom dot node
	Hollow  bool       // Element: hollow dot
	Large   bool       // Element: larger dot
	// Placement puts this item's content at the "start" or "end" side,
	// overriding the Mode (Ant v6 items[].placement).
	Placement string
	// Loading shows a spinning dot (Ant v6 items[].loading); the axis
	// entering it (leaving it under Reverse) is dotted where the theme's
	// TimelineToken.BusyTailDashed says so.
	Loading bool
	// Center centers the dot vertically on the item (Element center).
	Center bool
}

// TimelineW shows events along a vertical axis.
type TimelineW struct {
	_ [0]func()

	c         *Context
	items     []TimelineItem
	mode      string
	pending   string
	reverse   bool
	horiz     bool
	variant   string  // "filled" | "outlined" | "" (the theme's dot)
	titleSpan float32 // TitleSpan, in 24ths (0: 12)
	box       *Box
	th        *theme.Theme
	tk        *theme.TimelineToken
}

// Timeline starts a timeline; finish with Show.
//
// ConfigProvider.Defaults("Timeline", ...) keys: "mode" ("start" |
// "end" | "alternate" | "alternate-reverse"; "left" / "right" are the
// deprecated names of start / end), "reverse" (bool), "orientation"
// ("vertical" | "horizontal"), "variant" ("outlined" | "filled").
func Timeline(c *Context, items []TimelineItem) *TimelineW {
	th := c.ThemeFor(theme.CompTimeline)
	w := &TimelineW{c: c, items: items, mode: Prop(c, "Timeline", "mode", "start"), reverse: Prop(c, "Timeline", "reverse", false),
		th: th, tk: &th.Components.Timeline}
	w.Orientation(Prop(c, "Timeline", "orientation", "vertical"))
	w.variant = Prop(c, "Timeline", "variant", "")
	return w
}

// Mode places the content: "start" (axis at the inline start, content
// after it; default), "end" (axis at the inline end), "alternate"
// (content alternates around a centered axis, the first item after it)
// or "alternate-reverse" (Element: the first item before it). "left" and
// "right" are the deprecated Ant names of "start" and "end". Items with a
// Title also center the axis. The sides are logical: in RTL "start" puts
// the axis at the right.
func (w *TimelineW) Mode(m string) *TimelineW { w.mode = m; return w }

// Orientation lays the events out "vertical" (default) or "horizontal"
// (Ant v6 orientation; same as Horizontal).
func (w *TimelineW) Orientation(o string) *TimelineW { w.horiz = o == "horizontal"; return w }

// Variant sets the dots: "outlined" (a ring, Ant's default) or "filled"
// (a solid dot, the Element and shadcn default). Empty keeps the theme's
// (TimelineToken.DotOutlined).
func (w *TimelineW) Variant(v string) *TimelineW { w.variant = v; return w }

// TitleSpan sets the share of the width, in 24ths, taken by the titles of
// a vertical timeline whose items have a Title (Ant titleSpan; default 12,
// the axis in the middle). Ignored with Mode("alternate").
func (w *TimelineW) TitleSpan(span float32) *TimelineW { w.titleSpan = max(0, min(span, 24)); return w }

// placement returns "start" or "end" for item idx (its Placement, else
// the Mode).
func (w *TimelineW) placement(e tlEntry, idx int) string {
	switch e.Placement {
	case "start", "end":
		return e.Placement
	}
	switch w.mode {
	case "right", "end":
		return "end"
	case "alternate":
		if idx%2 == 1 {
			return "end"
		}
	case "alternate-reverse":
		if idx%2 == 0 {
			return "end"
		}
	}
	return "start"
}

// alternate reports an alternating mode.
func (w *TimelineW) alternate() bool { return w.mode == "alternate" || w.mode == "alternate-reverse" }

// busy reports an item drawn with a spinner (Loading or the Pending item).
func (e tlEntry) busy() bool { return e.pending || e.Loading }

// Horizontal lays the events out along a horizontal axis (Ant v6
// orientation="horizontal"): every event takes an equal share of the
// width with its dot on the axis. Mode keeps its meaning with the axis
// turned: "left" / "start" puts the content under the axis, "right" /
// "end" above it, "alternate" alternates (first under); a Label goes to
// the opposite side. In RTL the events run from the right.
func (w *TimelineW) Horizontal() *TimelineW { w.horiz = true; return w }

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
		return th.TextDisabled // antd colorTextDisabled
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
		e := tlEntry{TimelineItem: it}
		if e.Title != "" {
			e.Label = e.Title
		}
		entries = append(entries, e)
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
	if w.horiz {
		w.showHorizontal(entries, labelled)
		return w
	}
	w.box = Column(c, func() {
		for i, e := range entries {
			e := e
			last := i == len(entries)-1
			prevBusy := i > 0 && entries[i-1].busy()
			nextBusy := !last && entries[i+1].busy()
			Key(c, i, func() { w.item(e, i, last, labelled, prevBusy, nextBusy) })
		}
	})
	w.box.n.setRole(a11y.RoleList)
	return w
}

func (w *TimelineW) item(e tlEntry, idx int, last, labelled, prevBusy, nextBusy bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn // inferred: 1px border axis, filled size-2.5 dots, medium titles
	// Which side does the content go to?
	side := "right" // content after the axis (logical: at its end side)
	centered := labelled || w.alternate()
	if w.placement(e, idx) == "end" {
		side = "left"
	}
	// Grow factors of the title and content columns (Ant titleSpan).
	labelGrow, contentGrow := float32(1), float32(1)
	if labelled && !w.alternate() && w.titleSpan > 0 {
		labelGrow, contentGrow = w.titleSpan, 24-w.titleSpan
	}
	axisW := tk.DotSize
	gap := tk.ContentGap       // axis → content
	headTop := tk.DotOffsetTop // dot top inside the first line
	dotD, col := w.dotLook(e)
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
		}, layout.Grow(contentGrow), layout.MinW(0))
	}
	label := func(align TextAlign) {
		Column(c, func() {
			if e.Label != "" {
				Text(c, e.Label).Color(tk.LabelColor).Align(align)
			}
		}, layout.Grow(labelGrow), layout.MinW(0))
	}
	padBottom := tk.ItemPaddingBottom
	if last {
		padBottom = 0
	}
	axis := func() {
		ax := c.box("timeline-axis", layout.Column, []layout.Option{layout.W(axisW), layout.AlignItems(layout.AlignCenter)})
		ax.n.ln.Shrink = 0
		ax.n.aHidden = true // dots and tail are decoration
		// dotY is the dot centre: on the first line, or (Center) in the
		// middle of the item.
		dotY := func(r geom.Rect) float32 {
			if e.Center {
				return r.Y + r.H/2
			}
			return r.Y + headTop + axisW/2
		}
		ax.Paint(func(l *paint.List, r geom.Rect) {
			cx := r.X + r.W/2
			tc, lw := tk.TailColor, tk.TailWidth
			// seg draws the axis from y0 to y1, dotted next to a busy item
			// (Loading, Pending) when the theme says so (antd v6: a rail
			// with the "process" status).
			seg := func(y0, y1 float32, dotted bool) {
				if y1 <= y0 {
					return
				}
				if dotted && tk.BusyTailDashed {
					p := paint.NewPath().MoveTo(cx, y0).LineTo(cx, y1)
					l.StrokePath(p, paint.StrokeStyle{Width: lw, Dash: []float32{4, 3}}, tc)
					return
				}
				l.Fill(geom.Rect{X: cx - lw/2, Y: y0, W: lw, H: y1 - y0}, geom.Radii{}, tc)
			}
			cy := dotY(r)
			// From the previous item down to this dot, then on to the next
			// item: each item draws its share, so centred dots join up.
			// rc-steps colours a rail with the next step's status, or with
			// its own under railFollowPrevStatus (antd Timeline: reverse):
			// the segment entering a busy item is dotted, or the one
			// leaving it when reversed.
			in, out := e.busy(), nextBusy
			if w.reverse {
				in, out = prevBusy, e.busy()
			}
			if idx > 0 {
				seg(r.Y, cy, in)
			}
			if !last {
				seg(cy, r.Bottom()+padBottom, out)
			}
		})
		ax.n.over = func(l *paint.List, r geom.Rect) {
			if e.Dot != nil {
				return
			}
			w.drawDot(l, r.X+r.W/2, dotY(r), e, dotD, col)
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
	it.n.setRole(a11y.RoleListItem).stateIf(e.busy(), a11y.Busy)
}

// dotD is the drawn dot diameter of e; col its color.
func (w *TimelineW) dotLook(e tlEntry) (dotD float32, col theme.Color) {
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	dotD = tk.DotSize
	if (e.Large || e.DotIcon != nil) && el {
		dotD = tk.DotSizeLG
	}
	col = w.dotColor(e)
	if el && e.DotIcon != nil && e.Color == "" {
		col = th.Primary.Base
	}
	return dotD, col
}

// drawDot paints the dot of e centered at (cx, cy): the pending spinner,
// an icon, or the design system's circle.
func (w *TimelineW) drawDot(l *paint.List, cx, cy float32, e tlEntry, dotD float32, col theme.Color) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	outlined := w.variant == "outlined" || w.variant == "" && tk.DotOutlined
	switch {
	case e.busy():
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
	case el || outlined && !shad:
		rad := dotD / 2
		if e.Hollow || outlined {
			bw := tk.DotBorderWidth
			l.Path(paint.NewPath().Circle(cx, cy, rad), tk.DotBg)
			l.StrokePath(paint.NewPath().Circle(cx, cy, rad-bw/2), paint.StrokeStyle{Width: bw}, col)
		} else {
			l.Path(paint.NewPath().Circle(cx, cy, rad), col)
		}
	case !shad: // Variant("filled") under Ant
		l.Path(paint.NewPath().Circle(cx, cy, dotD/2), col)
	default:
		// shadcn: size-2.5 rounded-full with a ring-4 of the background
		// that cuts the axis around the dot.
		l.Path(paint.NewPath().Circle(cx, cy, dotD/2+2), tk.DotBg)
		if outlined {
			bw := tk.DotBorderWidth
			l.StrokePath(paint.NewPath().Circle(cx, cy, dotD/2-bw/2), paint.StrokeStyle{Width: bw}, col)
			return
		}
		l.Path(paint.NewPath().Circle(cx, cy, dotD/2), col)
	}
}

// showHorizontal builds the horizontal timeline: up to three rows of
// equal cells (content or label above the axis, the axis with the dots,
// content or label under it), so the axis stays straight however tall the
// texts are.
func (w *TimelineW) showHorizontal(entries []tlEntry, labelled bool) {
	c, tk := w.c, w.tk
	n := len(entries)
	below := func(i int) bool { // content under the axis
		return w.placement(entries[i], i) == "start"
	}
	hasTop, hasBottom := false, false
	for i := range entries {
		if below(i) {
			hasBottom = true
			hasTop = hasTop || labelled
		} else {
			hasTop = true
			hasBottom = hasBottom || labelled
		}
	}
	cellOpts := func(top bool) []layout.Option {
		pad := geom.Insets{Left: tk.HorizontalItemPadding, Right: tk.HorizontalItemPadding}
		o := []layout.Option{layout.Grow(1), layout.Basis(0), layout.MinW(0)}
		if top {
			pad.Bottom = tk.HorizontalGap
			o = append(o, layout.JustifyContent(layout.JustifyEnd))
		} else {
			pad.Top = tk.HorizontalGap
		}
		return append(o, layout.Padding(pad))
	}
	// cell builds the content (listitem) or the label of entry i.
	cell := func(i int, top bool) {
		e := entries[i]
		isContent := below(i) != top
		col := Column(c, func() {
			switch {
			case isContent:
				w.hContent(e, labelled)
			case e.Label != "":
				Text(c, e.Label).Color(tk.LabelColor).Align(AlignCenter)
			}
		}, cellOpts(top)...)
		if isContent {
			col.n.setRole(a11y.RoleListItem).stateIf(e.busy(), a11y.Busy)
		}
	}
	row := func(top bool) {
		Row(c, func() {
			for i := range entries {
				Key(c, i, func() { cell(i, top) })
			}
		}, layout.AlignItems(layout.AlignStretch))
	}
	w.box = Column(c, func() {
		if hasTop {
			row(true)
		}
		Row(c, func() {
			for i, e := range entries {
				// The tail from i to i+1 is dotted when it enters a busy
				// event, or (reverse) leaves one; see item.
				busyTail := i < n-1 && entries[i+1].busy()
				if w.reverse {
					busyTail = e.busy()
				}
				Key(c, i, func() { w.hAxis(e, i, i == n-1, busyTail) })
			}
		})
		if hasBottom {
			row(false)
		}
	})
	w.box.n.setRole(a11y.RoleList).setState(a11y.Horizontal)
}

// hContent builds the texts (or Body) of a horizontal timeline event,
// centered under / over its dot.
func (w *TimelineW) hContent(e tlEntry, labelled bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	if el && e.Time != "" && e.TimeTop {
		Text(c, e.Time).Size(tk.TimeFontSize).Color(tk.TimeColor).Align(AlignCenter).Layout(layout.Margin(geom.Insets{Bottom: 4}))
	}
	if e.Body != nil {
		e.Body()
	} else {
		t := Text(c, e.Content).Color(tk.ContentColor).Align(AlignCenter)
		if th.Flavor == theme.StyleShadcn && e.Time != "" && !labelled {
			t.Weight(text.Medium)
		}
	}
	if e.Time != "" && !(el && e.TimeTop) && (el || !labelled) {
		Text(c, e.Time).Size(tk.TimeFontSize).Color(tk.TimeColor).Align(AlignCenter).Layout(layout.Margin(geom.Insets{Top: 4}))
	}
}

// hAxis builds the axis cell of entry i: its dot in the middle and the
// tail towards the next dot (into the next cell, which has the same
// width).
func (w *TimelineW) hAxis(e tlEntry, i int, last, busyTail bool) {
	c, tk := w.c, w.tk
	dotD, col := w.dotLook(e)
	h := max(tk.DotSize, tk.DotSizeLG, tk.IconDotSize, 14)
	ax := c.box("timeline-axis", layout.Column, []layout.Option{layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.H(h),
		layout.AlignItems(layout.AlignCenter), layout.JustifyContent(layout.JustifyCenter)})
	ax.n.aHidden = true // dots and tail are decoration
	dashed := tk.BusyTailDashed && busyTail
	ax.Paint(func(l *paint.List, r geom.Rect) {
		if last {
			return
		}
		s := c.inline(r)
		cy := r.Y + r.H/2
		lw := tk.TailWidth
		if dashed {
			x0, x1 := s.X(r.W/2, 0), s.X(r.W*1.5, 0)
			l.StrokePath(paint.NewPath().MoveTo(x0, cy).LineTo(x1, cy), paint.StrokeStyle{Width: lw, Dash: []float32{4, 3}}, tk.TailColor)
			return
		}
		l.Fill(s.Rect(r.W/2, cy-lw/2, r.W, lw), geom.Radii{}, tk.TailColor)
	})
	if e.Dot != nil {
		ax.Body(e.Dot)
		return
	}
	ax.PaintOver(func(l *paint.List, r geom.Rect) {
		w.drawDot(l, r.X+r.W/2, r.Y+r.H/2, e, dotD, col)
	})
}
