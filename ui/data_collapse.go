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
	// Collapsible overrides the group's Collapsible for this panel:
	// "header" (only the title and the arrow toggle), "icon" (only the
	// arrow toggles) or "disabled"; "" uses the group's mode.
	Collapsible string
}

// CollapseW is an accordion / collapse panel group.
type CollapseW struct {
	_ [0]func()

	c           *Context
	active      *[]string
	items       []CollapseItem
	accordion   bool
	ghost       bool
	borderless  bool
	iconEnd     bool
	iconSet     bool
	size        ControlSize
	collapsible string
	expandIcon  *icon.Icon
	before      func(key string) bool
	changed     bool
	box         *Box
	th          *theme.Theme         // c.ThemeFor(theme.CompCollapse), set by Show
	tk          *theme.CollapseToken // &th.Components.Collapse
}

// Collapse starts a collapse group bound to the app's open panel keys.
// Configure with chained modifiers and finish with Show.
func Collapse(c *Context, active *[]string, items []CollapseItem) *CollapseW {
	if active == nil {
		active = &State[struct{ keys []string }](c, c.newID("collapse-state", ""), nil).keys
	}
	w := &CollapseW{c: c, active: active, items: items, size: c.defaultSize()}
	// ConfigProvider.Defaults("Collapse", ...) keys: "size", "bordered"
	// (bool), "accordion" (bool), "ghost" (bool), "collapsible"
	// ("header" | "icon" | "disabled"), "expandIconPlacement" ("start" |
	// "end"; the deprecated "expandIconPosition" is still read).
	if s, ok := dataPropSize(c, "Collapse"); ok {
		w.size = s
	}
	w.borderless = !Prop(c, "Collapse", "bordered", true)
	w.accordion = Prop(c, "Collapse", "accordion", false)
	w.ghost = Prop(c, "Collapse", "ghost", false)
	w.collapsible = Prop(c, "Collapse", "collapsible", "")
	w.ExpandIconPlacement(Prop(c, "Collapse", "expandIconPlacement", Prop(c, "Collapse", "expandIconPosition", "")))
	return w
}

// Accordion keeps at most one panel open.
func (w *CollapseW) Accordion() *CollapseW { w.accordion = true; return w }

// Ghost makes the panels transparent and borderless.
func (w *CollapseW) Ghost() *CollapseW { w.ghost = true; return w }

// Borderless removes the outer border (Ant bordered={false}).
func (w *CollapseW) Borderless() *CollapseW { w.borderless = true; return w }

// IconEnd puts the expand arrow at the end of the header (Element's default).
//
// Deprecated: use ExpandIconPlacement("end").
func (w *CollapseW) IconEnd() *CollapseW { w.iconEnd, w.iconSet = true, true; return w }

// IconStart puts the expand arrow before the label (Ant's default).
//
// Deprecated: use ExpandIconPlacement("start").
func (w *CollapseW) IconStart() *CollapseW { w.iconEnd, w.iconSet = false, true; return w }

// ExpandIconPlacement places the expand arrow: "start" (before the label,
// Ant's default) or "end" (Element and shadcn's default); antd
// expandIconPlacement, Element expand-icon-position ("left" / "right"
// are accepted too). Other values keep the theme's default.
func (w *CollapseW) ExpandIconPlacement(p string) *CollapseW {
	switch p {
	case "end", "right":
		w.iconEnd, w.iconSet = true, true
	case "start", "left":
		w.iconEnd, w.iconSet = false, true
	}
	return w
}

// Collapsible sets which part of every header toggles its panel (antd
// collapsible): "" the whole header, "header" only the title and the
// arrow, "icon" only the arrow, "disabled" nothing. CollapseItem.Collapsible
// overrides it per panel.
func (w *CollapseW) Collapsible(mode string) *CollapseW { w.collapsible = mode; return w }

// ExpandIcon replaces the expand arrow (antd expandIcon, Element icon). It
// points to the inline end when closed and turns 90° when open.
func (w *CollapseW) ExpandIcon(ic *icon.Icon) *CollapseW { w.expandIcon = ic; return w }

// BeforeCollapse is asked before a panel opens or closes (Element
// before-collapse); returning false keeps the panel as it is.
func (w *CollapseW) BeforeCollapse(fn func(key string) bool) *CollapseW { w.before = fn; return w }

// Size sets header padding (Small / Middle / Large).
func (w *CollapseW) Size(s ControlSize) *CollapseW { w.size = s; return w }

// Changed reports a panel toggle in the last Show.
func (w *CollapseW) Changed() bool { return w.changed }

// IsOpen reports whether key is open.
// In accordion mode only the first active key counts (rc-collapse
// useItems: activeKey[0]).
func (w *CollapseW) IsOpen(key string) bool {
	if w.accordion {
		return len(*w.active) > 0 && (*w.active)[0] == key
	}
	return contains(*w.active, key)
}

func (w *CollapseW) toggle(key string) bool {
	if w.before != nil && !w.before(key) {
		return false
	}
	if w.accordion {
		if w.IsOpen(key) {
			*w.active = (*w.active)[:0]
		} else {
			*w.active = append((*w.active)[:0], key)
		}
	} else if contains(*w.active, key) {
		*w.active = remove(*w.active, key)
	} else {
		*w.active = append(*w.active, key)
	}
	w.changed = true
	w.c.invalid = true
	return true
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

// headerStart is the start padding of a header without an arrow before
// the label (antd's small and large headers pad the start less; the
// arrow's offset margin makes up the difference when it is there).
func (w *CollapseW) headerStart(h float32) float32 {
	switch w.size {
	case Small:
		return w.tk.HeaderPaddingInlineStartSM
	case Large:
		return w.tk.HeaderPaddingInlineStartLG
	}
	return h
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
		iconEnd = tk.ExpandIconEnd
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
	if w.accordion {
		w.box.n.setRole(a11y.RoleTabList) // rc-collapse: role tablist in accordion mode
	}
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

// collTrigger marks n as the panel's toggle: a button (a tab in accordion
// mode, as rc-collapse) that is expandable and, when it has no text of
// its own, named by the panel label.
func (w *CollapseW) collTrigger(n *node, label string, disabled bool) {
	role := a11y.RoleButton
	if w.accordion {
		role = a11y.RoleTab
	}
	n.setRole(role).setName(label).setState(a11y.Expandable)
	if disabled {
		n.cursor = input.CursorNotAllowed
		n.focus = false // not a Tab stop (rc-collapse tabIndex -1)
	}
}

func (w *CollapseW) panel(it CollapseItem, last, iconEnd, bordered bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	open := w.IsOpen(it.Key)
	mode := it.Collapsible
	if mode == "" {
		mode = w.collapsible
	}
	if it.Disabled {
		mode = "disabled"
	}
	partial := mode == "header" || mode == "icon" // only parts of the header toggle
	pv, ph := w.headerPad()
	var hopts []layout.Option
	if el {
		hopts = []layout.Option{layout.MinH(tk.HeaderHeight), layout.Middle(), layout.Gap(tk.HeaderGap)}
		if w.ghost {
			hopts = append(hopts, layout.Padding(geom.Insets{}))
		}
	} else {
		start := ph
		if iconEnd || it.NoArrow {
			start = w.headerStart(ph)
		}
		hopts = []layout.Option{layout.Padding(geom.Insets{Top: pv, Bottom: pv, Left: start, Right: ph}), layout.Middle(), layout.Gap(tk.HeaderGap)}
	}
	hdr := collNamedBox(c, "collapse-header", it.Label, layout.Row, hopts...)
	disabled := mode == "disabled" || hdr.n.disabled
	var ia, arrowIA, titleIA Interaction
	var trigger *node // the node carrying the button semantics
	if partial {
		hdr.n.cursor = input.CursorDefault
	} else {
		ia = hdr.Focusable(input.CursorPointer)
		trigger = hdr.n
		w.collTrigger(hdr.n, it.Label, disabled)
	}
	var t float32 // arrow turn, set after the toggle below
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
	var arrowN *node
	arrowNode := func() {
		if it.NoArrow {
			return
		}
		n := c.add("collapse-arrow", "")
		arrowN = n
		sz := tk.ArrowSize
		n.ln.Width, n.ln.Height = sz, sz
		n.ln.AlignSelf = layout.AlignCenter
		if partial {
			// antd: the icon is the button when collapsible is header / icon.
			arrowIA = c.interact(n, !disabled, input.CursorPointer)
			trigger = n
			w.collTrigger(n, it.Label, disabled)
		} else {
			n.aHidden = true
		}
		if ic := w.expandIcon; ic != nil {
			n.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, ic, r, arrowCol, float64(t)*math.Pi/2) }
		} else if shad {
			// shadcn: muted chevron-down turning 180° when open.
			n.paint = func(l *paint.List, r geom.Rect) { icon.Draw(l, icon.Down, r, arrowCol, float64(t)*math.Pi) }
		} else {
			n.paint = func(l *paint.List, r geom.Rect) { collArrow(c, l, r, arrowCol, t) }
		}
		if partial && arrowIA.FocusVisible {
			n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(th.RadiusSM)) }
		}
	}
	hdr.Body(func() {
		if !iconEnd {
			arrowNode()
		}
		lab := Text(c, it.Label).Size(fs).Color(textCol).Ellipsis()
		if mode == "header" {
			// antd .collapsible-header: the title does not grow and is the
			// only clickable text.
			if it.NoArrow {
				// No arrow button: the title is the trigger (rc-collapse
				// gives the title span the full collapsibleProps).
				titleIA = c.interact(lab.n, !disabled, input.CursorPointer)
				trigger = lab.n
				w.collTrigger(lab.n, it.Label, disabled)
				if titleIA.FocusVisible {
					lab.n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(th.RadiusSM)) }
				}
			} else {
				titleIA = c.interact(lab.n, false, input.CursorPointer)
				lab.n.aKbd = true // the arrow button is the keyboard equivalent
			}
			if disabled {
				titleIA = Interaction{}
			}
			lab.n.ln.Shrink = 1
			Spacer(c)
		} else {
			lab.Layout(layout.Grow(1))
		}
		lab.st.Weight = weight
		if shad && (ia.Hovered || titleIA.Hovered) && !disabled {
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
	if disabled {
		ia.Clicked, arrowIA.Clicked, titleIA.Clicked = false, false, false
	}
	if (ia.Clicked || arrowIA.Clicked || titleIA.Clicked) && w.toggle(it.Key) {
		open = !open
	}
	if trigger != nil {
		trigger.stateIf(open, a11y.Expanded)
	}
	if arrowN != nil {
		t = c.Anim(arrowN.id, "arrow", b2f(open), th.DurationMid)
	}
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
	cv, ch := w.contentPad()
	switch {
	case shad, el:
		bodyOpts = []layout.Option{layout.Padding(geom.Insets{Bottom: tk.ContentPaddingBlock})}
	case w.ghost:
		if w.borderless {
			ch = tk.BorderlessContentPaddingInline
		}
		bodyOpts = []layout.Option{layout.PadVH(tk.GhostContentPaddingBlock, ch)}
	case w.borderless:
		bi := tk.BorderlessContentPaddingInline
		bodyOpts = []layout.Option{layout.Padding(geom.Insets{Top: tk.BorderlessContentPadding, Right: bi, Bottom: tk.BorderlessContentPaddingBottom, Left: bi})}
	default:
		bodyOpts = []layout.Option{layout.PadVH(cv, ch)}
	}
	inner := collMotion(c, "collapse-motion", open, func() {
		it.Body()
	}, bodyOpts...)
	if inner == nil {
		return
	}
	prole := a11y.RoleGroup
	if w.accordion {
		prole = a11y.RoleTabPanel
	}
	inner.n.setRole(prole).setName(it.Label)
	if c.a11yOn && trigger != nil {
		pid, tn := inner.n.id, trigger
		tn.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(pid) }))
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
