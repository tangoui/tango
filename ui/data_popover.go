package ui

import (
	"time"

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
// Popover

// PopoverW is a floating card attached to a trigger: title, content and
// an arrow, in twelve placements, opened by hover, click or focus.
type PopoverW struct {
	_ [0]func()

	c         *Context
	title     string
	content   func()
	place     string
	trigger   string
	open      *bool
	width     float32
	arrow     bool
	changed   bool
	isOpen    bool
	noPadding bool
	th        *theme.Theme
	tk        *theme.PopoverToken
}

type popoverState struct {
	open      bool
	panel     ID
	lastHover time.Time
	escShut   bool // Escape closed a focus-opened hover popover
}

// popFocusable returns the first keyboard-focusable node in n's subtree
// (the trigger control of a popover), or nil.
func popFocusable(n *node) *node {
	if n.focus && !n.disabled {
		return n
	}
	for _, k := range n.kids {
		if f := popFocusable(k); f != nil {
			return f
		}
	}
	return nil
}

// popKbdActivated reports Enter / Space delivered to a focused node inside
// n that activates it like a click (keyboard clicks are not recorded in
// c.clicked, which clickedWithin reads).
func (c *Context) popKbdActivated(n *node) bool {
	if c.focus == 0 {
		return false
	}
	f := popFindID(n, c.focus)
	if f == nil || f.ownKeys || f == n {
		return false
	}
	for _, k := range c.keys {
		if k.Down && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
			return true
		}
	}
	return false
}

// popLabel returns the accessible name or first text label in n's subtree.
func popLabel(n *node) string {
	if n.aNameSet {
		return n.aName
	}
	if n.label != "" {
		return n.label
	}
	for _, k := range n.kids {
		if s := popLabel(k); s != "" {
			return s
		}
	}
	return ""
}

func popFindID(n *node, id ID) *node {
	if n.id == id {
		return n
	}
	for _, k := range n.kids {
		if f := popFindID(k, id); f != nil {
			return f
		}
	}
	return nil
}

// Popover starts a popover; set options then call Body with the trigger.
//
// ConfigProvider.Defaults("Popover", ...) keys: "placement", "trigger",
// "arrow" (bool; default true, false for shadcn whose popover has none).
func Popover(c *Context) *PopoverW {
	th := c.ThemeFor(theme.CompPopover)
	return &PopoverW{c: c,
		place:   Prop(c, "Popover", "placement", "top"),
		trigger: Prop(c, "Popover", "trigger", "hover"),
		arrow:   Prop(c, "Popover", "arrow", c.Theme.Flavor != theme.StyleShadcn),
		th:      th, tk: &th.Components.Popover,
	}
}

// Title sets the bold heading.
func (p *PopoverW) Title(s string) *PopoverW { p.title = s; return p }

// Content builds the body.
func (p *PopoverW) Content(fn func()) *PopoverW { p.content = fn; return p }

// Text sets a plain text body.
func (p *PopoverW) Text(s string) *PopoverW {
	c := p.c
	p.content = func() { Text(c, s).Color(p.tk.Color).Layout(layout.MaxW(300)) }
	return p
}

// Placement: top, topLeft, topRight, bottom, bottomLeft, bottomRight,
// left, leftTop, leftBottom, right, rightTop, rightBottom. Left and right
// are logical (start / end): in RTL "left" opens on the right.
func (p *PopoverW) Placement(s string) *PopoverW { p.place = s; return p }

// Trigger: "hover" (default), "click", "focus" or "manual" (Open only).
func (p *PopoverW) Trigger(s string) *PopoverW { p.trigger = s; return p }

// Open binds the open state (controlled mode).
func (p *PopoverW) Open(v *bool) *PopoverW { p.open = v; return p }

// Width fixes the panel width.
func (p *PopoverW) Width(px float32) *PopoverW { p.width = px; return p }

// NoArrow hides the arrow.
func (p *PopoverW) NoArrow() *PopoverW { p.arrow = false; return p }

// NoPadding removes the panel padding (menus, custom cards).
func (p *PopoverW) NoPadding() *PopoverW { p.noPadding = true; return p }

// OpenChanged reports a state change caused by the user this frame.
func (p *PopoverW) OpenChanged() bool { return p.changed }

// IsOpen reports whether the panel is shown.
func (p *PopoverW) IsOpen() bool { return p.isOpen }

// popPlacement maps an Ant placement string to a logical Placement (left
// and right mean start and end, spec Q2) and the side of the anchor the
// panel ends up on in the current direction ("top", "bottom", "left",
// "right"), which is where the arrow points from.
func (c *Context) popPlacement(s string) (Placement, string) {
	var p Placement
	switch s {
	case "topLeft":
		p = TopStart
	case "topRight":
		p = TopEnd
	case "bottom":
		p = Bottom
	case "bottomLeft":
		p = BottomStart
	case "bottomRight":
		p = BottomEnd
	case "left":
		p = InlineStart
	case "leftTop":
		p = InlineStartTop
	case "leftBottom":
		p = InlineStartBottom
	case "right":
		p = InlineEnd
	case "rightTop":
		p = InlineEndTop
	case "rightBottom":
		p = InlineEndBottom
	default:
		p = Top
	}
	switch p.Resolve(c.IsRTL()) {
	case Left, LeftStart, LeftEnd:
		return p, "left"
	case Right, RightStart, RightEnd:
		return p, "right"
	case Bottom, BottomStart, BottomEnd:
		return p, "bottom"
	}
	return p, "top"
}

// Body builds the trigger content and, when open, the panel.
func (p *PopoverW) Body(fn func()) *Box {
	c, th, tk := p.c, p.th, p.tk
	el := th.Flavor == theme.Element
	b := c.box("popover-anchor", layout.Column, nil)
	b.n.ln.NoStretch = true
	b.Body(fn)
	st := State[popoverState](c, b.n.id, nil)
	open := &st.open
	if p.open != nil {
		open = p.open
	}
	cursor := input.CursorDefault
	if p.trigger == "click" {
		cursor = input.CursorPointer
	}
	// The trigger control: the first focusable widget inside, or the
	// anchor itself made focusable so keyboard users can open the panel.
	trig := popFocusable(b.n)
	if trig == nil {
		trig = dataFirstSem(b.n, true) // a control that registers its focus later (Button)
	}
	var ia Interaction
	if trig == nil && (p.trigger == "click" || p.trigger == "hover") {
		ia = b.Focusable(cursor)
		b.n.setRole(a11y.RoleButton)
		trig = b.n
		fia := ia
		b.n.over = func(l *paint.List, r geom.Rect) {
			if fia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			}
		}
	} else {
		if trig != nil {
			// The control inside shows the pointer and is the semantic
			// trigger; the anchor is only the hit area around it.
			cursor = input.CursorDefault
		}
		ia = b.Interact(cursor)
	}
	if trig != nil && p.trigger != "manual" {
		trig.setState(a11y.HasPopup)
		if p.trigger == "click" {
			trig.setState(a11y.Expandable)
		}
	}
	was := *open
	switch p.trigger {
	case "hover":
		within := c.focusWithin(b.n)
		if !within {
			st.escShut = false
		}
		kbd := within && c.focusVisible && !st.escShut
		if ia.Hovered || (*open && c.hovered(st.panel)) || kbd {
			st.lastHover = c.now
			*open = true
		} else if *open {
			// Keep the panel for 100ms so the pointer can travel to it.
			if c.now.Sub(st.lastHover) >= 100*time.Millisecond {
				*open = false
			} else {
				c.WakeAt(st.lastHover.Add(100 * time.Millisecond))
			}
		}
	case "click":
		if ia.Clicked || c.clickedWithin(b.n) || (trig != b.n && c.popKbdActivated(b.n)) {
			*open = !*open
		}
	case "focus":
		*open = c.focusWithin(b.n)
	}
	if *open && p.trigger != "hover" && c.Dismissed(st.panel, b.n.id) {
		*open = false
	}
	if *open && p.trigger == "hover" && c.EscapeAt(st.panel) {
		*open, st.escShut = false, true
	}
	if *open != was {
		p.changed = true
		c.invalid = true
	}
	p.isOpen = *open
	if trig != nil {
		trig.stateIf(*open, a11y.Expanded)
		if *open && c.a11yOn {
			trig.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(st.panel) }))
		}
	}
	if !*open {
		return b
	}
	place, _ := c.popPlacement(p.place)
	off := float32(4)
	if p.arrow {
		off = 10
		if el {
			off = 12
		}
	}
	a := c.AnimFrom(b.n.id, "pop", 0, 1, th.DurationMid)
	pad := layout.Pad(tk.InnerPadding)
	if p.noPadding {
		pad = layout.Pad(0)
	}
	ov := Overlay(c, OverlayOptions{Anchor: b.n.id, Place: place, Offset: off, Flip: true}, func() {
		if p.title != "" {
			t := Text(c, p.title).Color(tk.TitleColor)
			if tk.TitleFontSize != c.Theme.FontSize {
				t.Size(tk.TitleFontSize)
			}
			if tk.TitleFontWeight != 0 {
				t.Weight(text.Weight(tk.TitleFontWeight))
			}
			t.Layout(layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
		}
		if p.content != nil {
			Column(c, p.content)
		}
	}, pad)
	minW := float32(0)
	if p.title != "" || el {
		minW = tk.TitleMinWidth // Element keeps its min-width without a title
	}
	ov.n.ln.MinW = minW
	if p.width > 0 {
		ov.n.ln.Width = p.width
	}
	ov.Bg(tk.Bg).Radius(tk.Radius).Opacity(a).Shadow(tk.Shadow)
	if tk.BorderColor.A > 0 {
		ov.Border(1, tk.BorderColor)
	}
	st.panel = ov.n.id
	if p.trigger == "hover" {
		ov.n.setRole(a11y.RoleTooltip)
	} else {
		ov.n.setRole(a11y.RoleDialog)
		if c.a11yOn {
			// Untitled panels are named after their trigger.
			tn := ""
			if trig != nil {
				tn = popLabel(trig)
			}
			ov.n.fill(semFunc(func(s *a11y.Node) {
				dialogSem(s)
				if s.Name == "" {
					s.Name = tn
				}
			}))
		}
	}
	if p.title != "" {
		ov.n.setName(p.title)
	}
	if p.arrow {
		anchor := b.n.id
		ov.n.over = func(l *paint.List, r geom.Rect) {
			ar, ok := c.anchorRect(anchor)
			if !ok {
				return
			}
			popDrawArrow(l, r, ar, tk.Bg, tk.BorderColor)
		}
	}
	return b
}

// popDrawArrow draws a triangular arrow on the side of panel r that faces
// anchor ar, pointing at the anchor's centre (clamped inside the panel).
// A non-zero border colour outlines the two outer edges (Element).
func popDrawArrow(l *paint.List, r, ar geom.Rect, bg, border theme.Color) {
	const half, h = 7, 7
	p := paint.NewPath()
	edge := paint.NewPath()
	switch {
	case r.Bottom() <= ar.Y+1: // panel above: arrow at bottom
		cx := max(r.X+12+half, min(ar.X+ar.W/2, r.Right()-12-half))
		y := r.Bottom() - 1
		p.MoveTo(cx-half-1, y).LineTo(cx, y+h).LineTo(cx+half+1, y).Close()
		edge.MoveTo(cx-half, y+0.5).LineTo(cx, y+h).LineTo(cx+half, y+0.5)
	case r.Y >= ar.Bottom()-1: // below: arrow at top
		cx := max(r.X+12+half, min(ar.X+ar.W/2, r.Right()-12-half))
		y := r.Y + 1
		p.MoveTo(cx-half-1, y).LineTo(cx, y-h).LineTo(cx+half+1, y).Close()
		edge.MoveTo(cx-half, y-0.5).LineTo(cx, y-h).LineTo(cx+half, y-0.5)
	case r.Right() <= ar.X+1: // left of anchor: arrow on the right
		cy := max(r.Y+12+half, min(ar.Y+ar.H/2, r.Bottom()-12-half))
		x := r.Right() - 1
		p.MoveTo(x, cy-half-1).LineTo(x+h, cy).LineTo(x, cy+half+1).Close()
		edge.MoveTo(x+0.5, cy-half).LineTo(x+h, cy).LineTo(x+0.5, cy+half)
	default: // right of anchor: arrow on the left
		cy := max(r.Y+12+half, min(ar.Y+ar.H/2, r.Bottom()-12-half))
		x := r.X + 1
		p.MoveTo(x, cy-half-1).LineTo(x-h, cy).LineTo(x, cy+half+1).Close()
		edge.MoveTo(x-0.5, cy-half).LineTo(x-h, cy).LineTo(x-0.5, cy+half)
	}
	l.Path(p, bg)
	if border.A > 0 {
		l.StrokePath(edge, paint.StrokeStyle{Width: 1}, border)
	}
}

// ---------------------------------------------------------------------------
// Tour

// TourStep is one step of a Tour.
type TourStep struct {
	Target      ID // widget to highlight (0: centred panel, no hole)
	Title       string
	Description string
	Cover       func() // optional picture above the title
	Placement   string // as Popover.Placement; default "bottom"

	// Per-step overrides of the tour's options.
	Mask     TourMask // TourMaskOn / TourMaskOff; zero: the tour's setting
	Type     TourType // TourTypeDefault / TourTypePrimary; "": the tour's setting
	PrevText string   // label of the previous button ("": Locale)
	NextText string   // label of the next / finish button ("": Locale)
	HidePrev bool     // no previous button on this step
	HideNext bool     // no next / finish button on this step
}

// TourMask overrides the tour's mask for one step.
type TourMask uint8

const (
	TourMaskInherit TourMask = iota // the tour's setting (NoMask)
	TourMaskOn                      // dim the page around the target
	TourMaskOff                     // no mask on this step
)

// TourType is the panel style of a tour step.
type TourType string

const (
	TourTypeDefault TourType = "default" // the container-colored panel
	TourTypePrimary TourType = "primary" // the brand-colored panel
)

// TourActions is what an ActionsRender function gets: the step position
// and the operations of the default buttons.
type TourActions struct {
	_       [0]func()
	Current int // 0-based index of the shown step
	Total   int
	t       *TourW
	def     func()
}

// Prev moves to the previous step.
func (a *TourActions) Prev() { a.GoTo(a.Current - 1) }

// Next moves to the next step, or finishes the tour on the last one.
func (a *TourActions) Next() {
	if a.Current >= a.Total-1 {
		a.t.finish()
		return
	}
	a.GoTo(a.Current + 1)
}

// GoTo moves to step i (clamped).
func (a *TourActions) GoTo(i int) { a.t.goTo(i) }

// Close dismisses the tour (Closed reports it).
func (a *TourActions) Close() { a.t.close() }

// Default builds the default buttons (previous, next / finish) in place.
func (a *TourActions) Default() { a.def() }

// TourW is a guided tour: a mask with a hole around the current target
// and a panel with step indicators and prev/next buttons.
type TourW struct {
	_ [0]func()

	c        *Context
	open     *bool
	cur      *int
	steps    []TourStep
	primary  bool
	mask     bool
	arrow    bool
	gap      float32
	finished bool
	closed   bool
	changed  bool
	id       ID // retained state (scroll into view)
	reveal   bool
	dots     func(current, total int)
	actions  func(a *TourActions)
}

// tourState is the retained state of a Tour.
type tourState struct {
	revealed int // 1 + the step whose target was scrolled into view (0: none)
}

// Tour starts a tour bound to *open and *current; call Show.
func Tour(c *Context, open *bool, current *int, steps []TourStep) *TourW {
	return &TourW{c: c, open: open, cur: current, steps: steps, mask: true, arrow: true, gap: 6, reveal: true, id: c.newID("tour", "")}
}

// ScrollIntoView sets whether each step scrolls its target into the view
// of the scroll container holding it when the step is shown (default on,
// Ant scrollIntoViewOptions).
func (t *TourW) ScrollIntoView(on bool) *TourW { t.reveal = on; return t }

// IndicatorsRender replaces the step indicators (dots; Element: "1 / 3")
// with the content fn builds (Ant indicatorsRender). current is 0-based.
func (t *TourW) IndicatorsRender(fn func(current, total int)) *TourW { t.dots = fn; return t }

// ActionsRender replaces the previous / next buttons with the content fn
// builds (Ant actionsRender); a.Default() builds the default buttons and
// a.Prev / Next / GoTo / Close drive the tour.
func (t *TourW) ActionsRender(fn func(a *TourActions)) *TourW { t.actions = fn; return t }

// goTo shows step i (clamped).
func (t *TourW) goTo(i int) {
	i = max(0, min(i, len(t.steps)-1))
	if i != *t.cur {
		*t.cur = i
		t.changed = true
		t.c.invalid = true
	}
}

// finish closes the tour from its last step.
func (t *TourW) finish() {
	*t.open, t.finished = false, true
	t.c.invalid = true
}

// close dismisses the tour.
func (t *TourW) close() {
	*t.open, t.closed = false, true
	t.c.invalid = true
}

// revealTarget scrolls the step's target into view once per shown step.
func (t *TourW) revealTarget(target ID, hasTarget bool) {
	c := t.c
	st := State[tourState](c, t.id, nil)
	if !*t.open {
		st.revealed = 0
		return
	}
	want := *t.cur + 1
	if !t.reveal || st.revealed == want || target == 0 {
		return
	}
	if !hasTarget {
		return // not laid out yet: try next frame
	}
	// After the build, when the target's node exists wherever it was
	// built relative to the tour.
	c.deferred = append(c.deferred, func() {
		n := tourFind(c.root, target)
		if n == nil {
			return
		}
		if p := scrollAncestor(n); p != nil && !c.ScrollIntoView(p.id, target) {
			c.animating = true // container not painted yet
			return
		}
		st.revealed = want
	})
}

// tourFind returns the node with id in the tree under n, or nil.
func tourFind(n *node, id ID) *node {
	if n == nil {
		return nil
	}
	if n.id == id {
		return n
	}
	for _, k := range n.kids {
		if f := tourFind(k, id); f != nil {
			return f
		}
	}
	return nil
}

// Primary uses the brand-coloured panel.
func (t *TourW) Primary() *TourW { t.primary = true; return t }

// NoMask hides the dimming mask.
func (t *TourW) NoMask() *TourW { t.mask = false; return t }

// NoArrow hides the panel arrow.
func (t *TourW) NoArrow() *TourW { t.arrow = false; return t }

// Gap sets the hole padding around the target.
func (t *TourW) Gap(px float32) *TourW { t.gap = px; return t }

// Finished reports a click on the last step's finish button.
func (t *TourW) Finished() bool { return t.finished }

// Closed reports the tour was dismissed (close button or Escape).
func (t *TourW) Closed() bool { return t.closed }

// Changed reports a step change.
func (t *TourW) Changed() bool { return t.changed }

// Show builds the tour layers when open.
func (t *TourW) Show() *TourW {
	c := t.c
	if !*t.open || len(t.steps) == 0 {
		t.revealTarget(0, false)
		return t
	}
	th := c.ThemeFor(theme.CompTour)
	tk := &th.Components.Tour
	el := th.Flavor == theme.Element
	*t.cur = max(0, min(*t.cur, len(t.steps)-1))
	step := t.steps[*t.cur]
	tr, hasTarget := geom.Rect{}, false
	if step.Target != 0 {
		tr, hasTarget = c.anchorRect(step.Target)
		if !hasTarget {
			c.animating = true // target geometry is known next frame
		}
	}
	t.revealTarget(step.Target, hasTarget)
	showMask, primary := t.mask, t.primary
	switch step.Mask {
	case TourMaskOn:
		showMask = true
	case TourMaskOff:
		showMask = false
	}
	switch step.Type {
	case TourTypePrimary:
		primary = true
	case TourTypeDefault:
		primary = false
	}
	hole := tr.Expand(t.gap)
	// Mask layer: swallows pointer input, punches a hole over the target.
	mask := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {}, layout.Grow(1))
	mask.n.paint = func(l *paint.List, r geom.Rect) {
		if !showMask {
			return
		}
		p := paint.NewPath().Rect(r)
		if hasTarget {
			p.Append(paint.NewPath().RRect(hole, geom.Rad(tk.HoleRadius)).Reversed())
		}
		l.Path(p, tk.MaskColor)
	}
	// Panel.
	place, _ := c.popPlacement(step.Placement)
	if step.Placement == "" {
		place = Bottom
	}
	opts := OverlayOptions{Anchor: step.Target, Place: place, Offset: t.gap + 10, Flip: true}
	if !hasTarget {
		opts = OverlayOptions{Place: ScreenCenter}
	}
	fg, fg2 := tk.TitleColor, tk.DescriptionColor
	if primary {
		fg, fg2 = tk.PrimaryColor, tk.PrimaryDescriptionColor
	}
	n := len(t.steps)
	loc := c.Locale()
	ov := Overlay(c, opts, func() {
		if step.Cover != nil {
			Column(c, step.Cover, layout.Margin(geom.Insets{Bottom: 12}), layout.Center())
		}
		Row(c, func() {
			tt := Text(c, step.Title).Color(fg).Layout(layout.Grow(1)).Size(tk.TitleFontSize)
			if tk.TitleFontWeight != 0 {
				tt.Weight(text.Weight(tk.TitleFontWeight))
			}
			if IconButton(c, icon.Close, loc.Tour.Close).Size(tk.CloseBtnSize, tk.CloseIconSize).Color(fg2).Clicked() {
				*t.open, t.closed = false, true
				c.invalid = true
			}
		}, layout.Middle(), layout.Gap(8), layout.Margin(geom.Insets{Bottom: 8}))
		if step.Description != "" {
			Text(c, step.Description).Color(fg2)
		}
		Row(c, func() {
			switch {
			case t.dots != nil:
				t.dots(*t.cur, n)
			case el:
				Text(c, itoa64(int64(*t.cur+1))+" / "+itoa64(int64(n))).Color(fg2).Size(tk.CounterFontSize)
			default:
				tourDots(c, tk, n, *t.cur, primary)
			}
			Spacer(c)
			buttons := func() {
				if *t.cur > 0 && !step.HidePrev {
					label := step.PrevText
					if label == "" {
						label = loc.Tour.Prev
					}
					b := Button(c, label).Small()
					if primary {
						b.Ghost()
					}
					if b.Clicked() {
						t.goTo(*t.cur - 1)
					}
				}
				if step.HideNext {
					return
				}
				label := loc.Tour.Next
				if *t.cur == n-1 {
					label = loc.Tour.Finish
				}
				if step.NextText != "" {
					label = step.NextText
				}
				nb := Button(c, label).Small().Primary()
				if primary {
					nb = nb.Variant(VariantDefault)
				}
				if nb.Clicked() {
					if *t.cur == n-1 {
						t.finish()
					} else {
						t.goTo(*t.cur + 1)
					}
				}
			}
			if t.actions != nil {
				t.actions(&TourActions{Current: *t.cur, Total: n, t: t, def: buttons})
			} else {
				buttons()
			}
		}, layout.Middle(), layout.Gap(8), layout.Margin(geom.Insets{Top: 16}))
	}, layout.Pad(tk.PanelPadding), layout.W(tk.Width))
	bg := tk.Bg
	if primary {
		bg = tk.PrimaryBg
	}
	a := c.AnimFrom(mask.n.id, "tour", 0, 1, th.DurationMid)
	ov.Bg(bg).Radius(tk.Radius).Shadow(tk.Shadow).Opacity(a)
	ov.n.setRole(a11y.RoleDialog).setName(step.Title).stateIf(showMask, a11y.Modal)
	if c.a11yOn {
		desc := step.Description
		ov.n.fill(semFunc(func(s *a11y.Node) {
			dialogSem(s)
			if s.Description == "" {
				s.Description = desc
			}
		}))
	}
	if showMask {
		c.TrapFocus(ov.ID())
	}
	if t.arrow && hasTarget {
		target := step.Target
		ov.n.over = func(l *paint.List, r geom.Rect) {
			ar, ok := c.anchorRect(target)
			if !ok {
				return
			}
			popDrawArrow(l, r, ar.Expand(t.gap), bg, theme.Color{})
		}
	}
	if c.EscapeAt(ov.ID()) {
		*t.open, t.closed = false, true
		c.invalid = true
	}
	return t
}

func tourDots(c *Context, tk *theme.TourToken, n, cur int, primary bool) {
	nd := c.add("tour-dots", "")
	nd.setRole(a11y.RoleText).setName(itoa64(int64(cur+1)) + " / " + itoa64(int64(n)))
	sz, step := tk.IndicatorSize, tk.IndicatorSize+tk.IndicatorGap
	nd.ln.Width, nd.ln.Height = float32(n)*step-tk.IndicatorGap, sz
	nd.ln.AlignSelf = layout.AlignCenter
	nd.paint = func(l *paint.List, r geom.Rect) {
		sp := c.inline(r) // the first step's dot at the inline start
		for i := 0; i < n; i++ {
			col := tk.IndicatorColor
			if i == cur {
				col = tk.IndicatorActiveColor
			}
			if primary {
				col = tk.PrimaryIndicatorColor
				if i == cur {
					col = tk.PrimaryIndicatorActiveColor
				}
			}
			l.Fill(sp.Rect(float32(i)*step, r.Y, sz, sz), geom.Rad(sz/2), col)
		}
	}
}

var _ = text.Regular
