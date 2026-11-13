package ui

import (
	"sort"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shad2dSelectionBar is the data-table selection line: the muted
// "n selected" text and a ghost clear button, without the tinted alert
// box of Ant. left / opt replace the text and the clear button
// (ProTable TableAlertRender / TableAlertOptionRender). It reports a
// click on the default clear button.
func shad2dSelectionBar(c *Context, n int, left, opt func()) bool {
	th := c.Theme
	_, muted := shad2dMuted(th)
	cleared := false
	Row(c, func() {
		if left != nil {
			left()
		} else {
			Text(c, c.Locale().Selected(n)).Color(muted)
		}
		Spacer(c)
		if opt != nil {
			opt()
		} else if Button(c, c.Locale().ProTable.ClearSelection).Text().Small().Clicked() {
			cleared = true
		}
	}, layout.Gap(8), layout.Middle(), layout.MinH(32), layout.Margin(geom.Insets{Bottom: 8}))
	return cleared
}

// shadContent is the data-table layout of a ProTable: the query form and
// the toolbar sit on the page surface (no cards), the table is wrapped in
// a rounded-md border, and the footer holds the muted selection count (or
// total) with Previous / Next outline buttons.
func (p *ProTableW[T]) shadContent() {
	c, th, st := p.c, p.c.Theme, p.st
	if p.search && p.hasSearch() {
		Column(c, func() { p.queryFilter() }, layout.Margin(geom.Insets{Bottom: 16}))
	}
	p.toolbarRow()
	if st.err != nil {
		Row(c, func() {
			Alert(c, "error", locFmt(c.Locale().ProTable.LoadErrorFmt, st.err.Error())).Show()
		}, layout.Margin(geom.Insets{Bottom: 16}))
		Row(c, func() {
			if Button(c, c.Locale().Retry).Small().Icon(icon.Reload).Clicked() {
				p.Reload()
			}
		}, layout.Margin(geom.Insets{Bottom: 16}))
	}
	// rounded-md border drawn over the table (its header and rows paint
	// edge to edge).
	rad, border := th.Radius, th.BorderSecondary
	Column(c, func() { p.tableView() }, layout.MinW(0)).Radius(rad).Clip().
		PaintOver(func(l *paint.List, r geom.Rect) { l.Stroke(r, geom.Rad(rad), 1, border) })
	p.shadFooter()
}

// shadFooter is the data-table pagination row.
func (p *ProTableW[T]) shadFooter() {
	c, st := p.c, p.st
	loc := c.Locale()
	_, muted := shad2dMuted(c.Theme)
	sel := p.selection != nil && len(*p.selection) > 0 && !p.noAlert
	if p.noPager && !sel {
		return
	}
	Row(c, func() {
		Row(c, func() {
			switch {
			case sel:
				keys := append([]string(nil), *p.selection...)
				if p.alertFn != nil {
					p.alertFn(keys, p.clearSelection)
				} else {
					Text(c, loc.Selected(len(keys))).Color(muted)
				}
				if p.alertOpt != nil {
					p.alertOpt(keys, p.clearSelection)
				} else if Button(c, loc.ProTable.ClearSelection).Text().Small().Clicked() {
					p.clearSelection()
				}
			case st.total > 0:
				Text(c, loc.Total(st.total)).Color(muted)
			}
		}, layout.Grow(1), layout.MinW(0), layout.Gap(8), layout.Middle())
		if p.noPager {
			return
		}
		size := max(1, st.pageSize)
		pages := max(1, (st.total+size-1)/size)
		Row(c, func() {
			cur := strconv.Itoa(size)
			var opts []Option
			for _, n := range proPageSizes(size) {
				opts = append(opts, Option{Label: loc.PageSizeLabel(n), Value: strconv.Itoa(n)})
			}
			sw := Select(c, &cur, opts).Size(Small).Width(112).A11yLabel(loc.PageSizeLabel(size))
			if sw.Changed() {
				if n, err := strconv.Atoi(cur); err == nil && n > 0 {
					st.pageSize, st.page = n, 1
					c.invalid = true
				}
			}
			Text(c, locFmt(loc.A11y.PageFmt, st.page)+" / "+strconv.Itoa(pages)).Weight(text.Medium).NoWrap()
			if Button(c, loc.PrevPage).Small().Disabled(st.page <= 1).Clicked() && st.page > 1 {
				st.page--
				c.invalid = true
			}
			if Button(c, loc.NextPage).Small().Disabled(st.page >= pages).Clicked() && st.page < pages {
				st.page++
				c.invalid = true
			}
		}, layout.Gap(8), layout.Middle()).Semantics(a11y.RoleNavigation, loc.A11y.Pagination)
	}, layout.Middle(), layout.Gap(16), layout.Margin(geom.Insets{Top: 12}))
}

// proPageSizes lists the page sizes offered (the current one included).
func proPageSizes(cur int) []int {
	out := []int{10, 20, 50, 100}
	for _, n := range out {
		if n == cur {
			return out
		}
	}
	out = append(out, cur)
	sort.Ints(out)
	return out
}

// shadTool is the shadcn toolbar option button: ghost 32px icon button,
// or (label != "") an outline small button with the icon and the label
// (the data-table "View" column toggle). It reports a click and its ID.
func (p *ProTableW[T]) shadTool(tip, label string, draw func(*paint.List, geom.Rect, theme.Color), ic *icon.Icon, on, popup bool) (bool, ID) {
	c, th := p.c, p.c.Theme
	n := c.add("pro-tool", tip)
	n.setRole(a11y.RoleButton)
	if popup {
		n.setState(a11y.Expandable|a11y.HasPopup).stateIf(on, a11y.Expanded)
	}
	n.ln.Height = 32
	n.ln.NoStretch = true
	var lay *text.Layout
	if label != "" {
		lay = text.Shape(label, c.style(th.FontSize, text.Medium), c.textOpts(text.Options{NoWrap: true}))
		n.ln.Width = 12 + 16 + 8 + lay.Width + 12
	} else {
		n.ln.Width = 32
	}
	ia := c.interact(n, true, input.CursorPointer)
	accent := th.ItemHoverBg
	if v := th.Shadcn; v != nil {
		accent = v.Accent
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(th.Radius)
		if lay != nil {
			l.Shadows(r, rad, th.ButtonShadow)
			l.Fill(r, rad, th.BgContainer)
		}
		if ia.Hovered || on {
			l.Fill(r, rad, accent)
		}
		if lay != nil {
			l.Stroke(r, rad, 1, th.Border)
		}
		// Icon at the start, the label after it (mirrored in RTL).
		s := c.inline(r)
		ir := geom.Rect{X: r.X + 8, Y: r.Y + 8, W: 16, H: 16}
		if lay != nil {
			ir.X = s.X(12, 16)
		}
		if draw != nil {
			draw(l, ir, th.Text)
		} else {
			icon.Draw(l, ic, ir, th.Text, 0)
		}
		if lay != nil {
			l.Glyphs(geom.Pt(s.X(12+16+8, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.Text)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, rad)
		}
	}
	if ia.Hovered && !on && lay == nil {
		tf := &tblFrame{c: c, th: th}
		tf.tip(n.id, tip, false)
		tf.flushTips()
	}
	return ia.Clicked, n.id
}

// shadPillBody is the content of a shadcn LightFilter pill.
func (f *LightFilterW) shadPillBody(fd *ProField, has bool, vt string, fs float32, clear *bool) {
	c, th := f.c, f.c.Theme
	loc := c.Locale()
	muted, mutedFg := shad2dMuted(th)
	if v := th.Shadcn; v != nil {
		muted = v.Secondary
	}
	shad2dIconNode(c, 16, shad2dIconPlusCircle, mutedFg)
	Text(c, fd.Label).Size(fs).Weight(text.Medium).NoWrap().Color(th.Text)
	if !has {
		return
	}
	Div(c, layout.W(1), layout.H(16), layout.Margin(geom.Insets{Left: 4, Right: 4})).Bg(th.BorderSecondary)
	Row(c, func() {
		Text(c, vt).Size(th.FontSizeSM).NoWrap().Ellipsis().Color(th.Text).Layout(layout.MaxW(f.maxText), layout.MinW(0))
	}, layout.PadVH(2, 4), layout.MinW(0)).Bg(muted).Radius(th.RadiusXS + 2)
	if pro3IconBtn(c, locFmt(loc.LightFilter.ClearFmt, fd.Label), icon.Close, 14) {
		*clear = true
	}
}
