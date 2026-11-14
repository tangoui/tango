package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Card

// CardW is a bordered container: cover, header with extra, body, action
// bar, footer, loading skeleton, inner type, hover/always/never shadow.
type CardW struct {
	_ [0]func()

	c         *Context
	title     string
	extra     func()
	cover     func()
	actions   []func()
	footer    func()
	loading   bool
	hoverable bool
	bordered  bool
	small     bool
	inner     bool
	shadow    string // "" (flavor default) | always | hover | never
	pad       *geom.Insets
	opts      []layout.Option
	ia        Interaction
	headBg    theme.Color
	tabs      []TabItem
	tabActive *string
	tabExtra  func()
	tabHit    bool
}

// Card starts a card ("" title = no header); set options then call Body.
//
// ConfigProvider.Defaults("Card", ...) keys: "size" ("small" / Small),
// "variant" ("outlined" | "borderless"; the deprecated "bordered" bool is
// still read), "hoverable" (bool), "shadow" (Element: "always" | "hover"
// | "never"), "type" ("inner").
func Card(c *Context, title string) *CardW {
	w := &CardW{c: c, title: title, bordered: true, small: c.defaultSize() == Small}
	if s, ok := dataPropSize(c, "Card"); ok {
		w.small = s == Small
	}
	w.bordered = Prop(c, "Card", "bordered", w.bordered)
	if Prop(c, "Card", "variant", "") == "borderless" {
		w.bordered = false
	}
	w.hoverable = Prop(c, "Card", "hoverable", false)
	w.shadow = Prop(c, "Card", "shadow", "")
	w.inner = Prop(c, "Card", "type", "") == "inner"
	return w
}

// Variant sets the card variant (antd 6 variant): "outlined" (the
// default, a 1px outline) or "borderless" (no outline; an Ant card then
// keeps a soft shadow, antd .ant-card:not(.ant-card-bordered)).
func (w *CardW) Variant(v string) *CardW {
	switch v {
	case "borderless":
		w.bordered = false
	case "outlined":
		w.bordered = true
	}
	return w
}

func (w *CardW) Extra(fn func()) *CardW { w.extra = fn; return w }

// Cover places content (usually an image) above the header, edge to edge.
func (w *CardW) Cover(fn func()) *CardW { w.cover = fn; return w }

// Actions adds the bottom action bar; each fn builds one equal-width slot.
func (w *CardW) Actions(fns ...func()) *CardW { w.actions = fns; return w }

// Footer adds an Element-style footer below the body.
func (w *CardW) Footer(fn func()) *CardW { w.footer = fn; return w }

// Loading shows an active skeleton instead of the body.
func (w *CardW) Loading(on bool) *CardW { w.loading = on; return w }

// Hoverable lifts the card with a shadow on hover.
func (w *CardW) Hoverable() *CardW { w.hoverable = true; return w }

// Borderless removes the border.
//
// Deprecated: use Variant("borderless") (antd 6 replaced bordered with
// variant).
func (w *CardW) Borderless() *CardW { w.bordered = false; return w }

// Small uses the compact header and padding.
func (w *CardW) Small() *CardW { w.small = true; return w }

// Inner uses Ant's inner-card header (grey header band).
func (w *CardW) Inner() *CardW { w.inner = true; return w }

// Shadow sets the shadow mode: "always", "hover" or "never" (Element's
// el-card shadow prop). The default comes from the theme
// (CardToken.ShadowAlways: never for Ant, except a borderless card, which
// keeps a soft shadow; always for Element and shadcn).
func (w *CardW) Shadow(mode string) *CardW { w.shadow = mode; return w }

// Padding overrides the body padding.
func (w *CardW) Padding(in geom.Insets) *CardW { w.pad = &in; return w }

// Layout applies layout options to the card.
func (w *CardW) Layout(o ...layout.Option) *CardW { w.opts = append(w.opts, o...); return w }

// Tabs adds a tab bar under the title (Ant tabList / activeTabKey): it
// switches *active, and the body is built as the active tab's panel, so
// Body's fn shows the content for *active. Without a title the tab bar is
// the whole header.
func (w *CardW) Tabs(active *string, items []TabItem) *CardW {
	w.tabActive, w.tabs = active, items
	return w
}

// TabBarExtra builds content at the end of the card's tab bar (Ant
// tabBarExtraContent).
func (w *CardW) TabBarExtra(fn func()) *CardW { w.tabExtra = fn; return w }

// TabChanged reports that the user switched tabs this frame (Ant
// onTabChange). Call it after Body.
func (w *CardW) TabChanged() bool { return w.tabHit }

// Clicked reports a click on a hoverable card.
func (w *CardW) Clicked() bool { return w.ia.Clicked }

// Body builds the card.
func (w *CardW) Body(fn func()) *CardW {
	c := w.c
	th := c.ThemeFor(theme.CompCard)
	tk := &th.Components.Card
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	rad := tk.Radius
	b := c.box("card", layout.Column, w.opts)
	b.Bg(tk.Bg).Radius(rad)
	b.n.setRole(a11y.RoleGroup)
	if w.title != "" {
		b.n.setName(w.title)
	}
	b.n.stateIf(w.loading, a11y.Busy)
	if w.cover != nil || len(w.actions) > 0 || w.inner {
		// Edge-to-edge parts follow the rounded corners. Plain cards do not
		// clip so focus rings of controls near the edge stay visible.
		b.Clip()
	}
	border := tk.BorderColor
	mode := w.shadow
	if mode == "" {
		mode = "never"
		if tk.ShadowAlways || !w.bordered {
			mode = "always"
		}
	}
	if w.hoverable {
		mode = "hover"
	}
	if mode == "hover" {
		// A hoverable card is clickable (Clicked): a Tab stop too.
		w.ia = b.Focusable(input.CursorPointer)
	}
	if w.bordered {
		// Drawn above children so the cover and action bar backgrounds
		// do not hide it. antd: a hoverable card's outline turns
		// transparent under the pointer (HoverBorderColor zero).
		outline := border
		if mode == "hover" {
			ha := c.Anim(b.n.id, "hover-border", b2f(w.ia.Hovered), th.DurationMid)
			outline = theme.Lerp(border, tk.HoverBorderColor, ha)
			if tk.HoverBorderColor.A == 0 {
				outline = theme.Fade(border, float64(1-ha))
			}
		}
		b.PaintOver(func(l *paint.List, r geom.Rect) { l.Stroke(r, geom.Rad(rad), 1, outline) })
	}
	switch mode {
	case "always":
		b.Shadow(tk.Shadow)
	case "hover":
		if w.ia.FocusVisible {
			fr, prev := geom.Rad(rad), b.n.over
			b.PaintOver(func(l *paint.List, r geom.Rect) {
				if prev != nil {
					prev(l, r)
				}
				drawFocusRing(l, th, r, fr)
			})
		}
		a := c.Anim(b.n.id, "hover", b2f(w.ia.Hovered), th.DurationMid)
		src := tk.HoverShadow
		// antd: a borderless card keeps boxShadowTertiary at rest and
		// swaps to cardShadow under the pointer.
		var base []paint.Shadow
		if !w.bordered && !tk.ShadowAlways {
			base = tk.Shadow
		}
		if a > 0 || len(base) > 0 {
			sh := make([]paint.Shadow, 0, len(src)+len(base))
			for _, s := range base {
				s.Color = theme.Fade(s.Color, float64(1-a))
				sh = append(sh, s)
			}
			for _, s := range src {
				s.Color = theme.Fade(s.Color, float64(a))
				sh = append(sh, s)
			}
			b.Shadow(sh)
		}
	}
	pad := geom.All(tk.BodyPadding)
	if w.small {
		pad = geom.All(tk.BodyPaddingSM)
	} else if w.inner {
		// antd type inner: body padding "padding bodyPadding" (16 24).
		pad.Top, pad.Bottom = tk.InnerBodyPaddingBlock, tk.InnerBodyPaddingBlock
	}
	hasTabs := w.tabActive != nil && len(w.tabs) > 0
	hasHead := w.title != "" || w.extra != nil || hasTabs
	if shad && hasHead {
		pad.Top = 0 // shadcn CardContent: p-6 pt-0 under the header
	}
	if w.pad != nil {
		pad = *w.pad
	}
	b.Body(func() {
		if w.cover != nil {
			Column(c, w.cover)
		}
		var tb *TabsW
		if hasTabs {
			tb = w.tabBar(tk, border)
		}
		if tb == nil && (w.title != "" || w.extra != nil) {
			h, fs, pv, ph := tk.HeaderHeight, tk.HeaderFontSize, tk.HeaderPaddingBlock, tk.HeaderPaddingInline
			if w.small {
				h, fs, pv, ph = tk.HeaderHeightSM, tk.HeaderFontSizeSM, tk.HeaderPaddingBlockSM, tk.HeaderPaddingInlineSM
			}
			hopts := []layout.Option{layout.PadVH(pv, ph), layout.Middle(), layout.Gap(8)}
			if h > 0 {
				hopts = append(hopts, layout.MinH(h))
			}
			hd := Row(c, func() {
				t := Text(c, w.title).Size(fs).Ellipsis().Layout(layout.Grow(1)).Color(tk.HeaderColor)
				if tk.HeaderFontWeight != 0 {
					t.Weight(text.Weight(tk.HeaderFontWeight))
				}
				if w.inner && !el {
					t.Size(th.FontSize)
				}
				if w.extra != nil {
					w.extra()
				}
			}, hopts...)
			if hb := tk.HeaderBg; w.inner && !el && tk.InnerHeaderBg.A > 0 {
				hd.Bg(tk.InnerHeaderBg)
			} else if hb.A > 0 {
				hd.Bg(hb)
			}
			if !shad || w.inner {
				hd.Paint(func(l *paint.List, r geom.Rect) {
					dataHLine(l, r.X, r.Bottom()-1, r.W, border)
				})
			}
		}
		if w.loading {
			Column(c, func() { Skeleton(c).Active().NoTitle().Rows(4).Show() }, layout.Padding(pad))
		} else if tb != nil && fn != nil {
			Column(c, func() { tb.Panel(fn) }, layout.Padding(pad), layout.Grow(1))
		} else if fn != nil {
			Column(c, fn, layout.Padding(pad), layout.Grow(1))
		}
		if w.footer != nil && shad {
			Row(c, w.footer, layout.Padding(geom.Insets{Left: pad.Left, Right: pad.Right, Bottom: pad.Bottom}), layout.Middle(), layout.Gap(8))
		} else if w.footer != nil {
			Row(c, w.footer, layout.PadVH(18, 20), layout.Middle(), layout.Gap(8)).Paint(func(l *paint.List, r geom.Rect) {
				dataHLine(l, r.X, r.Y, r.W, border)
			})
		}
		if len(w.actions) > 0 {
			n := len(w.actions)
			Row(c, func() {
				for i, a := range w.actions {
					cell := Row(c, a, layout.Grow(1), layout.Center(), layout.Gap(6), layout.H(22))
					if i < n-1 {
						cell.Paint(func(l *paint.List, r geom.Rect) {
							dataVLine(l, c.inline(r).End(), r.Y, r.H, border)
						})
					}
				}
			}, layout.PadVH(12, 0)).Bg(tk.ActionsBg).Paint(func(l *paint.List, r geom.Rect) {
				dataHLine(l, r.X, r.Y, r.W, border)
			})
		}
	})
	return w
}

// CardActionW is the action slot CardAction returns.
type CardActionW struct {
	_ [0]func()

	clicked bool
}

// Clicked reports whether the action was activated this frame.
func (a *CardActionW) Clicked() bool { return a.clicked }

// tabBar builds the header of a card with tabs: the title row (when there
// is a title or extra) and the tab bar below it, with the header line at
// the bottom. It returns the shown bar.
func (w *CardW) tabBar(tk *theme.CardToken, border theme.Color) *TabsW {
	c := w.c
	th := c.ThemeFor(theme.CompCard)
	el, shad := th.Flavor == theme.Element, th.Flavor == theme.StyleShadcn
	h, fs, pv, ph := tk.HeaderHeight, tk.HeaderFontSize, tk.HeaderPaddingBlock, tk.HeaderPaddingInline
	if w.small {
		h, fs, pv, ph = tk.HeaderHeightSM, tk.HeaderFontSizeSM, tk.HeaderPaddingBlockSM, tk.HeaderPaddingInlineSM
	}
	var tb *TabsW
	hd := Column(c, func() {
		if w.title != "" || w.extra != nil {
			ropts := []layout.Option{layout.Padding(geom.Insets{Top: pv, Left: ph, Right: ph}), layout.Middle(), layout.Gap(8)}
			if h > 0 {
				ropts = append(ropts, layout.MinH(h-pv))
			}
			Row(c, func() {
				t := Text(c, w.title).Size(fs).Ellipsis().Layout(layout.Grow(1)).Color(tk.HeaderColor)
				if tk.HeaderFontWeight != 0 {
					t.Weight(text.Weight(tk.HeaderFontWeight))
				}
				if w.inner && !el {
					t.Size(th.FontSize)
				}
				if w.extra != nil {
					w.extra()
				}
			}, ropts...)
		}
		bopts := []layout.Option{layout.Padding(geom.Insets{Left: ph, Right: ph})}
		if shad {
			bopts = []layout.Option{layout.Padding(geom.Insets{Top: pv / 2, Left: ph, Right: ph, Bottom: pv / 2})}
		}
		Column(c, func() {
			tb = Tabs(c, w.tabActive, w.tabs).NoMargin()
			if w.small {
				tb.Size(Small)
			}
			if w.tabExtra != nil {
				tb.Extra(w.tabExtra)
			}
			tb.Show()
			w.tabHit = tb.Changed()
		}, bopts...)
	})
	if hb := tk.HeaderBg; w.inner && !el && tk.InnerHeaderBg.A > 0 {
		hd.Bg(tk.InnerHeaderBg)
	} else if hb.A > 0 {
		hd.Bg(hb)
	}
	if !shad || w.inner {
		hd.Paint(func(l *paint.List, r geom.Rect) {
			dataHLine(l, r.X, r.Bottom()-1, r.W, border)
		})
	}
	return tb
}

// CardAction is a convenience action slot: a clickable icon (e.g.
// icon.Setting, icon.Edit, icon.More, icon.Like) in the secondary text
// color that turns primary on hover. Clicked on the result reports an
// activation.
func CardAction(c *Context, ic *icon.Icon, label string) *CardActionW {
	th := c.Theme
	n := c.add("card-action", label)
	n.setRole(a11y.RoleButton)
	n.ln.Width, n.ln.Height = 16, 16
	ia := c.interact(n, true, input.CursorPointer)
	h := c.Anim(n.id, "hover", b2f(ia.Hovered), th.DurationMid)
	n.paint = func(l *paint.List, r geom.Rect) {
		c.paintIcon(l, ic, r, theme.Lerp(th.TextTertiary, th.Primary.Base, h), 0)
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
	}
	return &CardActionW{clicked: ia.Clicked}
}

// CardMetaW is the Card.Meta block CardMeta returns; it embeds the row's
// Box.
type CardMetaW struct {
	_ [0]func()

	*Box
}

// CardMeta lays out Card.Meta: avatar on the left, title and description.
func CardMeta(c *Context, avatar func(), title, desc string) *CardMetaW {
	th := c.Theme
	b := Row(c, func() {
		if avatar != nil {
			Column(c, avatar, layout.Margin(geom.Insets{Right: 16}))
		}
		Column(c, func() {
			if title != "" {
				t := Text(c, title).Size(th.FontSizeLG).Ellipsis()
				t.Strong()
				t.Layout(layout.Margin(geom.Insets{Bottom: 8}))
			}
			if desc != "" {
				Text(c, desc).Tertiary()
			}
		}, layout.Grow(1))
	}, layout.AlignItems(layout.AlignStart))
	return &CardMetaW{Box: b}
}

// ---------------------------------------------------------------------------
// Descriptions

// DescItem is one label/value pair; Render replaces Value with custom
// content, Span takes several columns.
type DescItem struct {
	Label, Value string
	Span         int
	Render       func()
	// Spans sets the span from a breakpoint upwards (antd span={{ xs: 1,
	// md: 2 }}); the largest set breakpoint not above the window width
	// wins, Span is the fallback.
	Spans map[Breakpoint]int
	// Filled makes the item take the rest of its row and ends the row
	// (antd span: 'filled').
	Filled bool
}

// DescriptionsW is a description list: title, extra, columns, bordered,
// sizes, vertical layout, label width, per-item span and custom render.
type DescriptionsW struct {
	_ [0]func()

	c        *Context
	b        *Box
	title    string
	extra    func()
	items    []DescItem
	cols     int
	bordered bool
	size     ControlSize
	vertical bool
	labelW   float32
	colon    bool
	labelBg  theme.Color
	colsAt   dataRespCols // explicit ColumnAt / Columns
	colsDef  dataRespCols // ConfigProvider default map
	colsSet  bool         // explicit Column
	th       *theme.Theme
	tk       *theme.DescriptionsToken
}

// Descriptions adds a description list (default 3 columns, no border).
// Modifiers apply until the end of the build; the content is built then.
//
// ConfigProvider.Defaults("Descriptions", ...) keys: "column" (int or
// map[Breakpoint]int), "bordered", "size", "layout" ("vertical"),
// "colon" (bool).
func Descriptions(c *Context, items []DescItem) *DescriptionsW {
	th := c.ThemeFor(theme.CompDescriptions)
	tk := &th.Components.Descriptions
	def := Large // antd size "default" has the largest padding
	if tk.DefaultMiddle {
		def = Middle // Element's default size
	}
	d := &DescriptionsW{c: c, items: items, cols: 3, colon: true, size: c.sizeOr(def), th: th, tk: tk}
	dataPropCols(c, "Descriptions", "column", &d.cols, &d.colsDef)
	d.bordered = Prop(c, "Descriptions", "bordered", false)
	if s, ok := dataPropSize(c, "Descriptions"); ok {
		d.size = s
		if s == Middle && Prop(c, "Descriptions", "size", "") == "default" {
			d.size = def
		}
	}
	d.vertical = Prop(c, "Descriptions", "layout", "horizontal") == "vertical"
	d.colon = Prop(c, "Descriptions", "colon", true)
	d.b = c.box("descriptions", layout.Column, nil)
	dataLazy(c, d.b.n, d.build)
	return d
}

func (d *DescriptionsW) Title(s string) *DescriptionsW  { d.title = s; return d }
func (d *DescriptionsW) Extra(fn func()) *DescriptionsW { d.extra = fn; return d }
func (d *DescriptionsW) Column(n int) *DescriptionsW {
	d.cols, d.colsSet = max(1, n), true
	return d
}

// ColumnAt sets the column count from breakpoint bp upwards (Ant
// column={{ xs: 1, sm: 2, md: 3 }}); the largest set breakpoint not above
// the window width wins, Column (default 3) is the fallback.
func (d *DescriptionsW) ColumnAt(bp Breakpoint, n int) *DescriptionsW { d.colsAt.set(bp, n); return d }

// Columns sets several breakpoint column counts at once.
func (d *DescriptionsW) Columns(m map[Breakpoint]int) *DescriptionsW { d.colsAt.setMap(m); return d }
func (d *DescriptionsW) Bordered() *DescriptionsW                    { d.bordered = true; return d }
func (d *DescriptionsW) Size(s ControlSize) *DescriptionsW           { d.size = s; return d }
func (d *DescriptionsW) Vertical() *DescriptionsW                    { d.vertical = true; return d }
func (d *DescriptionsW) LabelWidth(px float32) *DescriptionsW {
	d.labelW = px
	return d
}
func (d *DescriptionsW) NoColon() *DescriptionsW { d.colon = false; return d }

// Layout applies layout options to the list.
func (d *DescriptionsW) Layout(o ...layout.Option) *DescriptionsW { d.b.Style(o...); return d }

// LabelBg overrides the bordered label background.
func (d *DescriptionsW) LabelBg(col theme.Color) *DescriptionsW { d.labelBg = col; return d }

// descRows splits items into rows of cols columns like antd's useRow: an
// item that does not fit is cut to the rest of the row (it does not wrap),
// a Filled item takes the rest of its row and ends it, and the last item
// of a short row stretches to fill it. span(i) is item i's span.
func descRows(items []DescItem, cols int, span func(i int) int) [][]DescItem {
	var rows [][]DescItem
	var cur []DescItem
	count := 0
	for i, it := range items {
		if it.Filled {
			it.Span = 1
			cur = append(cur, it)
			rows, cur, count = append(rows, cur), nil, 0
			continue
		}
		it.Span = max(1, span(i))
		rest := cols - count
		count += it.Span
		if count >= cols {
			if count > cols {
				it.Span = rest
			}
			cur = append(cur, it)
			rows, cur, count = append(rows, cur), nil, 0
			continue
		}
		cur = append(cur, it)
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	for _, row := range rows {
		n := 0
		for _, it := range row {
			n += it.Span
		}
		if n < cols {
			row[len(row)-1].Span += cols - n
		}
	}
	return rows
}

func (d *DescriptionsW) rows() [][]DescItem {
	return descRows(d.items, d.cols, func(i int) int {
		it := &d.items[i]
		if len(it.Spans) == 0 {
			return it.Span
		}
		var r dataRespCols
		r.setMap(it.Spans)
		return r.resolve(d.c, it.Span)
	})
}

func (d *DescriptionsW) pads() (v, h float32) {
	tk := d.tk
	switch d.size {
	case Small:
		return tk.CellPaddingBlockSM, tk.CellPaddingInlineSM
	case Middle:
		return tk.CellPaddingBlockMD, tk.CellPaddingInlineMD
	}
	return tk.CellPaddingBlock, tk.CellPaddingInline
}

// itemPad returns the bottom padding of borderless rows and of the label
// row of a borderless vertical list.
func (d *DescriptionsW) itemPad() (row, vlabel float32) {
	tk := d.tk
	switch d.size {
	case Small:
		return tk.ItemPaddingBottomSM, tk.VerticalLabelPaddingBottomSM
	case Middle:
		return tk.ItemPaddingBottomMD, tk.VerticalLabelPaddingBottomMD
	}
	return tk.ItemPaddingBottom, tk.VerticalLabelPaddingBottom
}

func (d *DescriptionsW) build() {
	c, th, tk := d.c, d.th, d.tk
	fallback := d.cols
	if !d.colsSet {
		fallback = d.colsDef.resolve(c, d.cols)
	}
	d.cols = d.colsAt.resolve(c, fallback)
	el := th.Flavor == theme.Element
	border := tk.BorderColor
	labelBg := tk.LabelBg
	if d.labelBg.A > 0 {
		labelBg = d.labelBg
	}
	pv, ph := d.pads()
	rowPad, vlabelPad := d.itemPad()
	labelEnd := tk.LabelMarginInlineEnd
	if d.size == Small {
		labelEnd = tk.LabelMarginInlineEndSM
	}
	if tk.ShowColon && !d.colon {
		// antd -item-no-colon::after keeps marginInline 2 8.
		labelEnd += tk.ColonMarginInlineStart
	}
	colonText := ""
	if d.colon && tk.ShowColon {
		colonText = c.Locale().LabelColon
	}
	if d.bordered && !d.vertical && d.labelW == 0 {
		// Align the label column across rows like a table would.
		lst := c.style(th.FontSize, text.Regular)
		for _, it := range d.items {
			d.labelW = max(d.labelW, text.Width(it.Label, lst)+2*ph+1)
		}
	}
	content := func(it DescItem) {
		if it.Render != nil {
			it.Render()
			return
		}
		col := tk.ContentColor
		if d.bordered {
			col = tk.BorderedContentColor
		}
		Text(c, it.Value).Color(col)
	}
	labelText := func(it DescItem) {
		col := tk.LabelColor
		if d.bordered {
			col = tk.BorderedLabelColor
		}
		t := Text(c, it.Label).NoWrap().Color(col)
		if el && d.bordered {
			t.Strong()
		}
	}
	// plainLabel builds a borderless label: the text, then the colon with
	// antd's margins (2px before, the label gap after; without a colon
	// the gap stays, like antd's empty ::after).
	plainLabel := func(it DescItem) *Box {
		lb := Row(c, func() {
			labelText(it)
			if colonText != "" {
				ct := Text(c, colonText).NoWrap().Color(tk.LabelColor).Layout(layout.Margin(geom.Insets{Left: tk.ColonMarginInlineStart}))
				ct.n.aHidden = true // decoration, like antd's ::after
			}
		})
		lb.n.ln.Shrink = 0
		return lb
	}
	if d.title != "" || d.extra != nil {
		tfs, tmb := tk.TitleFontSize, tk.TitleMarginBottom
		switch d.size {
		case Small:
			tfs, tmb = tk.TitleFontSizeSM, tk.TitleMarginBottomSM
		case Middle:
			tmb = tk.TitleMarginBottomMD
		}
		Row(c, func() {
			t := Text(c, d.title).Size(tfs).Color(tk.TitleColor).Ellipsis().Layout(layout.Grow(1))
			t.Strong()
			if d.extra != nil {
				d.extra()
			}
		}, layout.Middle(), layout.Margin(geom.Insets{Bottom: tmb}))
	}
	rows := d.rows()
	// cell builds one borderless grid cell: an unpadded outer box that
	// takes span columns (so columns line up whatever the padding) and a
	// padded inner box.
	cell := func(span int, bottom, end float32, role a11y.Role, fn func()) {
		Row(c, func() {
			in := Row(c, fn, layout.Grow(1), layout.MinW(0), layout.Padding(geom.Insets{Bottom: bottom, Right: end}), layout.AlignItems(layout.AlignStart))
			if role != a11y.RoleNone {
				in.n.setRole(role)
			}
		}, layout.Grow(float32(span)), layout.Basis(0), layout.MinW(0))
	}
	grid := Column(c, func() {
		for ri, row := range rows {
			last := ri == len(rows)-1
			line := func(r *Box) {
				if d.bordered && !last {
					r.Paint(func(l *paint.List, rr geom.Rect) { dataHLine(l, rr.X, rr.Bottom()-1, rr.W, border) })
				}
			}
			bottom := rowPad
			if last {
				bottom = 0 // antd: the last row has no bottom padding
			}
			endPad := func(ci int) float32 {
				if ci == len(row)-1 {
					return 0
				}
				return tk.ItemPaddingEnd
			}
			if d.vertical {
				// Label row then content row.
				lr := Row(c, func() {
					for ci, it := range row {
						if !d.bordered {
							cell(it.Span, vlabelPad, endPad(ci), a11y.RoleColumnHeader, func() { plainLabel(it) })
							continue
						}
						cl := Row(c, func() { labelText(it) }, layout.Grow(float32(it.Span)), layout.Basis(0), layout.PadVH(pv, ph))
						cl.n.setRole(a11y.RoleColumnHeader)
						cl.Bg(labelBg)
						if ci < len(row)-1 {
							cl.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
						}
					}
				})
				lr.n.setRole(a11y.RoleRow)
				if d.bordered {
					lr.Paint(func(l *paint.List, rr geom.Rect) { dataHLine(l, rr.X, rr.Bottom()-1, rr.W, border) })
				}
				cr := Row(c, func() {
					for ci, it := range row {
						if !d.bordered {
							cell(it.Span, bottom, endPad(ci), a11y.RoleCell, func() { content(it) })
							continue
						}
						cc := Row(c, func() { content(it) }, layout.Grow(float32(it.Span)), layout.Basis(0), layout.PadVH(pv, ph))
						cc.n.setRole(a11y.RoleCell)
						if ci < len(row)-1 {
							cc.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
						}
					}
				})
				cr.n.setRole(a11y.RoleRow)
				line(cr)
				continue
			}
			r := Row(c, func() {
				for ci, it := range row {
					span := float32(it.Span)
					if d.bordered {
						bc := Row(c, func() {
							lopts := []layout.Option{layout.PadVH(pv, ph)}
							if d.labelW > 0 {
								lopts = append(lopts, layout.W(d.labelW))
							}
							Row(c, func() { labelText(it) }, lopts...).Bg(labelBg).Paint(func(l *paint.List, r geom.Rect) {
								dataVLine(l, dataEndX(c, r), r.Y, r.H, border)
							}).n.setRole(a11y.RoleRowHeader)
							Column(c, func() { content(it) }, layout.Grow(1), layout.MinW(0), layout.PadVH(pv, ph)).n.setRole(a11y.RoleCell)
						}, layout.Grow(span), layout.Basis(0), layout.MinW(0))
						if ci < len(row)-1 {
							bc.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
						}
						continue
					}
					cell(it.Span, bottom, endPad(ci), a11y.RoleNone, func() {
						lb := plainLabel(it)
						lb.n.setRole(a11y.RoleRowHeader)
						if d.labelW > 0 {
							lb.Style(layout.W(d.labelW))
						}
						lb.Style(layout.Margin(geom.Insets{Right: labelEnd}))
						Column(c, func() { content(it) }, layout.Grow(1), layout.MinW(0)).n.setRole(a11y.RoleCell)
					})
				}
			}, layout.AlignItems(layout.AlignStretch))
			r.n.setRole(a11y.RoleRow)
			line(r)
		}
	})
	grid.n.setRole(a11y.RoleTable)
	if d.title != "" {
		grid.n.setName(d.title)
	}
	if d.bordered {
		grid.Border(1, border).Clip()
		if tk.Radius > 0 {
			grid.Radius(tk.Radius)
		}
	}
}
