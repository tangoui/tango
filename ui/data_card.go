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
}

// Card starts a card ("" title = no header); set options then call Body.
//
// ConfigProvider.Defaults("Card", ...) keys: "size" ("small" / Small),
// "bordered" (bool), "variant" ("outlined" | "borderless"), "hoverable"
// (bool), "shadow" (Element: "always" | "hover" | "never").
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
func (w *CardW) Borderless() *CardW { w.bordered = false; return w }

// Small uses the compact header and padding.
func (w *CardW) Small() *CardW { w.small = true; return w }

// Inner uses Ant's inner-card header (grey header band).
func (w *CardW) Inner() *CardW { w.inner = true; return w }

// Shadow sets the shadow mode: "always", "hover" or "never" (default:
// never for Ant, always for Element; Element's el-card shadow prop).
func (w *CardW) Shadow(mode string) *CardW { w.shadow = mode; return w }

// Padding overrides the body padding.
func (w *CardW) Padding(in geom.Insets) *CardW { w.pad = &in; return w }

// Layout applies layout options to the card.
func (w *CardW) Layout(o ...layout.Option) *CardW { w.opts = append(w.opts, o...); return w }

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
	if w.bordered {
		// Drawn above children so the cover and action bar backgrounds
		// do not hide it.
		b.PaintOver(func(l *paint.List, r geom.Rect) { l.Stroke(r, geom.Rad(rad), 1, border) })
	}
	mode := w.shadow
	if mode == "" {
		mode = "never"
		if el || shad {
			mode = "always"
		}
	}
	if w.hoverable {
		mode = "hover"
	}
	switch mode {
	case "always":
		b.Shadow(tk.Shadow)
	case "hover":
		// A hoverable card is clickable (Clicked): a Tab stop too.
		w.ia = b.Focusable(input.CursorPointer)
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
		if a > 0 {
			sh := make([]paint.Shadow, len(src))
			for i, s := range src {
				s.Color = theme.Fade(s.Color, float64(a))
				sh[i] = s
			}
			b.Shadow(sh)
		}
	}
	pad := geom.All(tk.BodyPadding)
	if w.small {
		pad = geom.All(tk.BodyPaddingSM)
	}
	hasHead := w.title != "" || w.extra != nil
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
		if w.title != "" || w.extra != nil {
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

// CardAction is a convenience action slot: a clickable icon (e.g.
// icon.Setting, icon.Edit, icon.More, icon.Like) in the secondary text
// color that turns primary on hover.
func CardAction(c *Context, ic *icon.Icon, label string) bool {
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
	return ia.Clicked
}

// CardMeta lays out Card.Meta: avatar on the left, title and description.
func CardMeta(c *Context, avatar func(), title, desc string) {
	th := c.Theme
	Row(c, func() {
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
}

// ---------------------------------------------------------------------------
// Descriptions

// DescItem is one label/value pair; Render replaces Value with custom
// content, Span takes several columns.
type DescItem struct {
	Label, Value string
	Span         int
	Render       func()
}

// DescriptionsW is a description list: title, extra, columns, bordered,
// sizes, vertical layout, label width, per-item span and custom render.
type DescriptionsW struct {
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
	d := &DescriptionsW{c: c, items: items, cols: 3, colon: true, size: c.sizeOr(Large), th: c.ThemeFor(theme.CompDescriptions)}
	d.tk = &d.th.Components.Descriptions
	dataPropCols(c, "Descriptions", "column", &d.cols, &d.colsDef)
	d.bordered = Prop(c, "Descriptions", "bordered", false)
	if s, ok := dataPropSize(c, "Descriptions"); ok {
		d.size = s
		if s == Middle && Prop(c, "Descriptions", "size", "") == "default" {
			d.size = Large // Ant "default" is the largest padding
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

func (d *DescriptionsW) rows() [][]DescItem {
	var rows [][]DescItem
	var cur []DescItem
	used := 0
	for i, it := range d.items {
		span := max(1, it.Span)
		if used+span > d.cols && len(cur) > 0 {
			// Stretch the previous row's last item to fill.
			rows = append(rows, cur)
			cur, used = nil, 0
		}
		span = min(span, d.cols)
		it.Span = span
		if i == len(d.items)-1 && used+span < d.cols {
			it.Span = d.cols - used
		}
		cur = append(cur, it)
		used += span
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	return rows
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
		Text(c, it.Value).Color(tk.ContentColor)
	}
	labelText := func(it DescItem, suffix bool) {
		s := it.Label
		if suffix && d.colon && !d.bordered {
			s = c.Locale().WithColon(s)
			if el {
				s = it.Label
			}
		}
		t := Text(c, s).NoWrap().Color(tk.LabelColor)
		if el && d.bordered {
			t.Strong()
		}
	}
	if d.title != "" || d.extra != nil {
		Row(c, func() {
			t := Text(c, d.title).Size(tk.TitleFontSize).Color(tk.TitleColor).Layout(layout.Grow(1))
			t.Strong()
			if d.extra != nil {
				d.extra()
			}
		}, layout.Middle(), layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
	}
	rows := d.rows()
	grid := Column(c, func() {
		for ri, row := range rows {
			last := ri == len(rows)-1
			line := func(r *Box) {
				if d.bordered && !last {
					r.Paint(func(l *paint.List, rr geom.Rect) { dataHLine(l, rr.X, rr.Bottom()-1, rr.W, border) })
				}
			}
			if d.vertical {
				// Label row then content row.
				lr := Row(c, func() {
					for ci, it := range row {
						cell := Row(c, func() { labelText(it, false) }, layout.Grow(float32(it.Span)), layout.Basis(0), layout.PadVH(pv, ph))
						cell.n.setRole(a11y.RoleColumnHeader)
						if d.bordered {
							cell.Bg(labelBg)
							if ci < len(row)-1 {
								cell.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
							}
						} else {
							cell.Style(layout.PadVH(0, 0), layout.Margin(geom.Insets{Bottom: 4}))
						}
					}
				})
				lr.n.setRole(a11y.RoleRow)
				if d.bordered {
					lr.Paint(func(l *paint.List, rr geom.Rect) { dataHLine(l, rr.X, rr.Bottom()-1, rr.W, border) })
				}
				cr := Row(c, func() {
					for ci, it := range row {
						cell := Row(c, func() { content(it) }, layout.Grow(float32(it.Span)), layout.Basis(0), layout.PadVH(pv, ph))
						cell.n.setRole(a11y.RoleCell)
						if d.bordered {
							if ci < len(row)-1 {
								cell.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
							}
						} else {
							cell.Style(layout.PadVH(0, 0), layout.Margin(geom.Insets{Bottom: 16}))
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
						cell := Row(c, func() {
							lopts := []layout.Option{layout.PadVH(pv, ph)}
							if d.labelW > 0 {
								lopts = append(lopts, layout.W(d.labelW))
							}
							Row(c, func() { labelText(it, false) }, lopts...).Bg(labelBg).Paint(func(l *paint.List, r geom.Rect) {
								dataVLine(l, dataEndX(c, r), r.Y, r.H, border)
							}).n.setRole(a11y.RoleRowHeader)
							Row(c, func() { content(it) }, layout.Grow(1), layout.PadVH(pv, ph), layout.Middle()).n.setRole(a11y.RoleCell)
						}, layout.Grow(span), layout.Basis(0))
						if ci < len(row)-1 {
							cell.Paint(func(l *paint.List, r geom.Rect) { dataVLine(l, dataEndX(c, r), r.Y, r.H, border) })
						}
					} else {
						Row(c, func() {
							lb := Row(c, func() { labelText(it, true) })
							lb.n.setRole(a11y.RoleRowHeader)
							if d.labelW > 0 {
								lb.Style(layout.W(d.labelW))
							}
							if tk.LabelMarginInlineEnd > 0 {
								lb.Style(layout.Margin(geom.Insets{Right: tk.LabelMarginInlineEnd}))
							}
							Column(c, func() { content(it) }, layout.Grow(1)).n.setRole(a11y.RoleCell)
						}, layout.Grow(span), layout.Basis(0), layout.Padding(geom.Insets{Bottom: tk.ItemPaddingBottom, Right: ph / 2}))
					}
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
