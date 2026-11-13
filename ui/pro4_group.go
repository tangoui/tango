package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// ProFormGroup (Ant Design Pro ProFormGroup)
//
//	ui.ProForm(c, fields).Children(func(f *ui.FormW) {
//		ui.ProFormGroup(c, "Contact").Columns(2).Collapsible().Body(func() {
//			ui.ProFormItems(c, f, contactFields, vals)
//		})
//	}).Show()
//
// or simply give fields the same ProField.Group: ProForm wraps every run
// of consecutive fields sharing a Group in a ProFormGroup titled with it.
//
// A titled block of form items: the title row (chevron when collapsible,
// Extra at the right), then the items stacked or, with Columns(n), laid
// out n per line. Ant lays group items in a horizontal Space; Tango form
// items fill the line, so the default is stacked and Columns gives the
// grid.

// ProFormGroupW builds a ProFormGroup.
type ProFormGroupW struct {
	c            *Context
	title        string
	desc         string
	extra        func()
	collapsible  bool
	defCollapsed bool
	collapsed    *bool
	cols         int
	gutter       float32
	gap          float32
	toggled      bool
	id           ID
}

type pro4GroupState struct {
	init      bool
	collapsed bool
}

// ProFormGroup starts a group titled title ("" = no title row); chain
// options, then Body.
//
// ConfigProvider.Defaults("ProFormGroup", ...) keys: "collapsible"
// (bool), "columns", "gutter", "size" (gap between rows, number).
func ProFormGroup(c *Context, title string) *ProFormGroupW {
	g := &ProFormGroupW{c: c, title: title, gutter: 24}
	g.collapsible = Prop(c, "ProFormGroup", "collapsible", false)
	if n, ok := proPropInt(c, "ProFormGroup", "columns"); ok {
		g.cols = n
	}
	if v, ok := p2bNum(c, "ProFormGroup", "gutter"); ok {
		g.gutter = float32(v)
	}
	if v, ok := p2bNum(c, "ProFormGroup", "size"); ok {
		g.gap = float32(v)
	}
	return g
}

// Description adds a muted line under the title.
func (g *ProFormGroupW) Description(s string) *ProFormGroupW { g.desc = s; return g }

// Extra builds content at the right of the title row.
func (g *ProFormGroupW) Extra(fn func()) *ProFormGroupW { g.extra = fn; return g }

// Collapsible lets a click on the title fold the items.
func (g *ProFormGroupW) Collapsible() *ProFormGroupW { g.collapsible = true; return g }

// DefaultCollapsed starts an uncontrolled collapsible group folded.
func (g *ProFormGroupW) DefaultCollapsed() *ProFormGroupW {
	g.collapsible, g.defCollapsed = true, true
	return g
}

// Collapsed binds the folded state (implies Collapsible).
func (g *ProFormGroupW) Collapsed(v *bool) *ProFormGroupW {
	g.collapsible, g.collapsed = true, v
	return g
}

// Columns lays the items out n per line (0: stacked).
func (g *ProFormGroupW) Columns(n int) *ProFormGroupW { g.cols = n; return g }

// Gutter sets the horizontal space between columns (default 24).
func (g *ProFormGroupW) Gutter(px float32) *ProFormGroupW { g.gutter = px; return g }

// Size sets the vertical space between item rows (default 0: the items'
// own margins).
func (g *ProFormGroupW) Size(px float32) *ProFormGroupW { g.gap = px; return g }

// Toggled reports a fold / unfold by the user this frame.
func (g *ProFormGroupW) Toggled() bool { return g.toggled }

// ID returns the group's widget ID (after Body).
func (g *ProFormGroupW) ID() ID { return g.id }

// Body builds the group with fn as its items.
func (g *ProFormGroupW) Body(fn func()) *ProFormGroupW {
	c, th := g.c, g.c.Theme
	root := c.box("pro-form-group", layout.Column, []layout.Option{layout.MinW(0), layout.Margin(geom.Insets{Bottom: 8})})
	g.id = root.n.id
	root.Semantics(a11y.RoleGroup, g.title)
	st := State[pro4GroupState](c, g.id, nil)
	if !st.init {
		st.init, st.collapsed = true, g.defCollapsed
	}
	folded := g.collapsible && st.collapsed
	if g.collapsed != nil {
		folded = *g.collapsed
	}
	root.Body(func() {
		if g.title != "" || g.extra != nil {
			folded = g.header(folded, st)
		}
		if folded {
			return
		}
		body := c.box("pro-form-group-body", layout.Column, []layout.Option{layout.MinW(0), layout.Gap(g.gap)})
		body.Body(fn)
		if g.cols > 1 {
			w := float32(0)
			if r, ok := c.rects[body.n.id]; ok {
				w = r.W
			}
			body.n.ln.Dir = layout.Row
			body.n.ln.Wrap = true
			body.n.ln.Gap, body.n.ln.LineGap = g.gutter, g.gap
			body.n.ln.Align = layout.AlignStart
			colW := float32(0)
			if w > 0 {
				colW = (w - g.gutter*float32(g.cols-1)) / float32(g.cols)
			}
			for _, k := range body.n.kids {
				k.ln.MinW = 0
				if colW > 0 {
					k.ln.Width, k.ln.Grow = colW-0.5, 0
				} else {
					k.ln.Grow, k.ln.Basis, k.ln.HasBasis = 1, 0, true
				}
			}
		}
	})
	_ = th
	return g
}

// header is the title row; it returns the folded state after a click.
func (g *ProFormGroupW) header(folded bool, st *pro4GroupState) bool {
	c, th := g.c, g.c.Theme
	_, muted := shad2dMuted(th)
	if !shad2d(c) {
		muted = th.TextTertiary
	}
	hd := c.box("pro-form-group-head", layout.Row, []layout.Option{layout.Middle(), layout.Gap(8), layout.Margin(geom.Insets{Bottom: 16})})
	if g.collapsible {
		ia := c.interact(hd.n, true, input.CursorPointer)
		name := g.title
		if name == "" {
			name = c.Locale().Collapse
			if folded {
				name = c.Locale().Expand
			}
		}
		hd.n.setRole(a11y.RoleButton).setName(name).setState(a11y.Expandable).stateIf(!folded, a11y.Expanded)
		if ia.Clicked {
			folded = !folded
			if g.collapsed != nil {
				*g.collapsed = folded
			} else {
				st.collapsed = folded
			}
			g.toggled = true
			c.invalid = true
		}
		hd.PaintOver(func(l *paint.List, r geom.Rect) {
			if ia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			}
		})
	}
	hd.Body(func() {
		if g.collapsible {
			rot := c.Anim(hd.n.id, "rot", map[bool]float32{true: 0, false: 90}[folded], th.DurationMid)
			Icon(c, icon.Right).Size(12).Color(th.Text).Rotate(float64(rot))
		}
		Column(c, func() {
			if g.title != "" {
				t := Text(c, g.title).Weight(text.Medium).Color(th.TextHeading).NoWrap().Ellipsis()
				if !shad2d(c) {
					t.Size(th.FontSizeLG)
				}
			}
			if g.desc != "" {
				Text(c, g.desc).Color(muted)
			}
		}, layout.Gap(4), layout.Grow(1), layout.MinW(0))
		if g.extra != nil {
			Row(c, g.extra, layout.Gap(8), layout.Middle())
		}
	})
	return folded
}

// ProFormItems builds the form items of fields inside f (for use in
// ProForm.Children, ProFormGroup bodies and custom form layouts).
func ProFormItems(c *Context, f *FormW, fields []ProField, vals ProValues) {
	for i := range fields {
		fd := &fields[i]
		Key(c, fd.Key, func() { proFieldItem(c, f, fd, vals) })
	}
}
