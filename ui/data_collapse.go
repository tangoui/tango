package ui

import (
	"math"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Expand/collapse motion shared by Collapse and Tree.

// collMotion builds fn inside a clipping column whose height eases between 0
// and the content height when open changes. Closed and settled content is
// not built at all. It returns the inner column (nil when fully closed).
func collMotion(c *Context, kind string, open bool, fn func(), opts ...layout.Option) *Box {
	outer := c.box(kind, layout.Column, nil)
	id := outer.n.id
	a := c.Anim(id, "open", b2f(open), c.Theme.DurationMid)
	if !open && a <= 0 {
		return nil
	}
	var inner *Box
	outer.Body(func() { inner = Column(c, fn, opts...) })
	if a < 1 {
		outer.Clip()
		h := float32(0)
		if r, ok := c.rects[inner.n.id]; ok {
			h = r.H
		}
		// Height 0 means "auto" for the solver, so keep a hairline minimum.
		outer.n.ln.Height = max(0.001, h*a)
		outer.n.ln.Shrink = 0
	}
	return inner
}

// collNamedBox creates a container node whose label is visible to
// Driver.Find (Box helpers always use an empty label).
func collNamedBox(c *Context, kind, label string, dir layout.Direction, opts ...layout.Option) *Box {
	n := c.add(kind, label)
	n.ln.Dir = dir
	flexbox.Apply(&n.ln.Style, opts...)
	b := &Box{c: c, n: n}
	n.paint = b.paintBox
	return b
}

// collArrow draws the expand arrow (a chevron pointing to the inline end,
// rotated by t*90° towards down; mirrored in RTL).
func collArrow(c *Context, l *paint.List, r geom.Rect, col theme.Color, t float32) {
	c.paintIcon(l, icon.Right, r, col, float64(t)*math.Pi/2)
}

// ---------------------------------------------------------------------------
// Collapse

// CollapseItem is one panel of a Collapse.
type CollapseItem struct {
	Key      string
	Label    string
	Extra    func() // content at the inline end of the header
	Body     func()
	Disabled bool
	NoArrow  bool
}

// CollapseW is an accordion / collapse panel group.
type CollapseW struct {
	_ [0]func()

	c          *Context
	active     *[]string
	items      []CollapseItem
	accordion  bool
	ghost      bool
	borderless bool
	iconEnd    bool
	iconSet    bool
	size       ControlSize
	changed    bool
	box        *Box
	th         *theme.Theme         // c.ThemeFor(theme.CompCollapse), set by Show
	tk         *theme.CollapseToken // &th.Components.Collapse
}

// Collapse starts a collapse group bound to the app's open panel keys.
// Configure with chained modifiers and finish with Show.
func Collapse(c *Context, active *[]string, items []CollapseItem) *CollapseW {
	if active == nil {
		active = &State[struct{ keys []string }](c, c.newID("collapse-state", ""), nil).keys
	}
	w := &CollapseW{c: c, active: active, items: items, size: c.defaultSize()}
	// ConfigProvider.Defaults("Collapse", ...) keys: "size", "bordered"
	// (bool), "accordion" (bool), "ghost" (bool), "expandIconPosition"
	// ("start" | "end").
	if s, ok := dataPropSize(c, "Collapse"); ok {
		w.size = s
	}
	w.borderless = !Prop(c, "Collapse", "bordered", true)
	w.accordion = Prop(c, "Collapse", "accordion", false)
	w.ghost = Prop(c, "Collapse", "ghost", false)
	switch Prop(c, "Collapse", "expandIconPosition", "") {
	case "end", "right":
		w.iconEnd, w.iconSet = true, true
	case "start", "left":
		w.iconEnd, w.iconSet = false, true
	}
	return w
}

// Accordion keeps at most one panel open.
func (w *CollapseW) Accordion() *CollapseW { w.accordion = true; return w }

// Ghost makes the panels transparent and borderless.
func (w *CollapseW) Ghost() *CollapseW { w.ghost = true; return w }

// Borderless removes the outer border (Ant bordered={false}).
func (w *CollapseW) Borderless() *CollapseW { w.borderless = true; return w }

// IconEnd puts the expand arrow at the end of the header (Element's default).
func (w *CollapseW) IconEnd() *CollapseW { w.iconEnd, w.iconSet = true, true; return w }

// IconStart puts the expand arrow before the label (Ant's default).
func (w *CollapseW) IconStart() *CollapseW { w.iconEnd, w.iconSet = false, true; return w }

// Size sets header padding (Small / Middle / Large).
func (w *CollapseW) Size(s ControlSize) *CollapseW { w.size = s; return w }

// Changed reports a panel toggle in the last Show.
func (w *CollapseW) Changed() bool { return w.changed }

// IsOpen reports whether key is open.
func (w *CollapseW) IsOpen(key string) bool { return contains(*w.active, key) }

func (w *CollapseW) toggle(key string) {
	if contains(*w.active, key) {
		*w.active = remove(*w.active, key)
	} else if w.accordion {
		*w.active = append((*w.active)[:0], key)
	} else {
		*w.active = append(*w.active, key)
	}
	w.changed = true
	w.c.invalid = true
}

func (w *CollapseW) headerPad() (v, h float32) {
	tk := w.tk
	switch w.size {
	case Small:
		return tk.HeaderPaddingBlockSM, tk.HeaderPaddingInlineSM
	case Large:
		return tk.HeaderPaddingBlockLG, tk.HeaderPaddingInlineLG
	}
	return tk.HeaderPaddingBlock, tk.HeaderPaddingInline
}

func (w *CollapseW) contentPad() (v, h float32) {
	tk := w.tk
	switch w.size {
	case Small:
		return tk.ContentPaddingBlockSM, tk.ContentPaddingInlineSM
	case Large:
		return tk.ContentPaddingBlockLG, tk.ContentPaddingInlineLG
	}
	return tk.ContentPaddingBlock, tk.ContentPaddingInline
}

// Show builds the panels.
func (w *CollapseW) Show() *CollapseW {
	c := w.c
	w.th = c.ThemeFor(theme.CompCollapse)
	w.tk = &w.th.Components.Collapse
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	iconEnd := w.iconEnd
	if !w.iconSet {
		iconEnd = el || shad
	}
	ant := !el && !shad // shadcn Accordion: no frame, items split by lines
	bordered := ant && !w.borderless && !w.ghost
	w.box = Column(c, func() {
		for i, it := range w.items {
			it := it
			last := i == len(w.items)-1
			Key(c, it.Key, func() { w.panel(it, last, iconEnd, bordered) })
		}
	})
	switch {
	case bordered:
		w.box.Bg(tk.HeaderBg).Border(1, tk.BorderColor).Radius(tk.Radius).Clip()
	case ant && w.borderless && !w.ghost:
		w.box.Bg(tk.HeaderBg)
	case el && !w.ghost:
		w.box.PaintOver(func(l *paint.List, r geom.Rect) {
			dataHLine(l, r.X, r.Y, r.W, tk.BorderColor)
			dataHLine(l, r.X, r.Bottom()-1, r.W, tk.BorderColor)
		})
	}
	return w
}

// Layout applies layout options to the group (call after Show).
func (w *CollapseW) Layout(o ...layout.Option) *CollapseW {
	if w.box != nil {
		w.box.Style(o...)
	}
	return w
}

func (w *CollapseW) panel(it CollapseItem, last, iconEnd, bordered bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	open := contains(*w.active, it.Key)
	pv, ph := w.headerPad()
	var hopts []layout.Option
	if el {
		hopts = []layout.Option{layout.MinH(tk.HeaderHeight), layout.Middle(), layout.Gap(tk.HeaderGap)}
		if w.ghost {
			hopts = append(hopts, layout.Padding(geom.Insets{}))
		}
	} else {
		hopts = []layout.Option{layout.PadVH(pv, ph), layout.Middle(), layout.Gap(tk.HeaderGap)}
	}
	hdr := collNamedBox(c, "collapse-header", it.Label, layout.Row, hopts...)
	disabled := it.Disabled || hdr.n.disabled
	ia := hdr.Focusable(input.CursorPointer)
	if disabled {
		hdr.n.cursor = input.CursorNotAllowed
		hdr.n.focus = false // not a Tab stop
		ia.Clicked = false
	}
	hdr.n.setRole(a11y.RoleButton).setName(it.Label).setState(a11y.Expandable)
	if ia.Clicked && !disabled {
		w.toggle(it.Key)
		open = !open
	}
	hdr.n.stateIf(open, a11y.Expanded)
	t := c.Anim(hdr.n.id, "arrow", b2f(open), th.DurationMid)
	arrowCol, textCol := tk.ArrowColor, tk.HeaderColor
	if disabled {
		arrowCol, textCol = th.TextDisabled, th.TextDisabled
	}
	fs := tk.HeaderFontSize
	if w.size == Large {
		fs = tk.HeaderFontSizeLG
	}
	weight := text.Regular
	if tk.HeaderFontWeight != 0 {
		weight = text.Weight(tk.HeaderFontWeight)
	}
	arrowNode := func() {
		if it.NoArrow {
			return
		}
		n := c.add("collapse-arrow", "")
		sz := tk.ArrowSize
		n.ln.Width, n.ln.Height = sz, sz
		n.ln.AlignSelf = layout.AlignCenter
		if shad {
			// shadcn: muted chevron-down turning 180° when open.
			col := arrowCol
			n.paint = func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.Down, r, col, float64(t)*math.Pi) }
			return
		}
		n.paint = func(l *paint.List, r geom.Rect) { collArrow(c, l, r, arrowCol, t) }
	}
	hdr.Body(func() {
		if !iconEnd {
			arrowNode()
		}
		lab := Text(c, it.Label).Size(fs).Color(textCol).Ellipsis().Layout(layout.Grow(1))
		lab.st.Weight = weight
		if shad && ia.Hovered && !disabled {
			lab.Underline() // hover:underline
		}
		if el {
			lab.st.LineHeight = tk.HeaderHeight
			lab.n.ln.MinH = 0
		}
		if it.Extra != nil {
			it.Extra()
		}
		if iconEnd {
			if el {
				Row(c, arrowNode, layout.Margin(geom.Insets{Right: 8}))
			} else {
				arrowNode()
			}
		}
	})
	sep := tk.BorderColor
	if ia.FocusVisible {
		hdr.PaintOver(func(l *paint.List, r geom.Rect) {
			g := th.FocusRingOffset + th.FocusRingWidth
			drawFocusRing(l, th, r.Inset(geom.All(g)), geom.Rad(th.RadiusSM))
		})
	}
	hdr.Paint(func(l *paint.List, r geom.Rect) {
		if shad {
			if !open && !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
			return
		}
		if el {
			if !w.ghost {
				l.Fill(r, geom.Radii{}, tk.HeaderBg)
			}
			// The item border sits under the header when closed and under
			// the content when open; the group draws the outer lines.
			if !open && !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
			return
		}
		if !w.ghost && !open && !last {
			dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
		}
	})
	if it.Body == nil {
		return
	}
	var bodyOpts []layout.Option
	switch {
	case shad, el:
		bodyOpts = []layout.Option{layout.Padding(geom.Insets{Bottom: tk.ContentPaddingBlock})}
	case w.ghost, w.borderless:
		bodyOpts = []layout.Option{layout.Padding(geom.Insets{Top: tk.BorderlessContentPadding, Right: ph, Bottom: tk.BorderlessContentPaddingBottom, Left: ph})}
	default:
		bodyOpts = []layout.Option{layout.PadVH(w.contentPad())}
	}
	inner := collMotion(c, "collapse-motion", open, func() {
		it.Body()
	}, bodyOpts...)
	if inner == nil {
		return
	}
	inner.n.setRole(a11y.RoleGroup).setName(it.Label)
	if c.a11yOn {
		pid := inner.n.id
		hdr.n.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(pid) }))
	}
	inner.Paint(func(l *paint.List, r geom.Rect) {
		switch {
		case shad:
			if !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
		case el:
			if !w.ghost {
				l.Fill(r, geom.Radii{}, tk.ContentBg)
			}
			if !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
		case bordered:
			l.Fill(r, geom.Radii{}, tk.ContentBg)
			dataHLine(l, r.X, r.Y, r.W, sep)
			if !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
		case w.borderless && !w.ghost:
			if !last {
				dataHLine(l, r.X, r.Bottom()-1, r.W, sep)
			}
		}
	})
}
