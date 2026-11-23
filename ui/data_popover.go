package ui

import (
	"math"
	"time"

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
	enter     time.Duration
	leave     time.Duration
	autoWidth bool // ignore the theme's default width (AvatarGroup's "+N" list)
	th        *theme.Theme
	tk        *theme.PopoverToken
}

type popoverState struct {
	open      bool
	panel     ID
	hoverWant bool      // the hover trigger's debounced wish last frame
	hovering  bool      // pointer / focus on the trigger or panel
	since     time.Time // when hovering last changed
	hoverOpen bool      // debounced hover state
	escShut   bool      // Escape closed a focus-opened hover popover
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
// "arrow" (bool; default PopoverToken.Arrow: true, false for shadcn whose
// popover has none),
// "mouseEnterDelay", "mouseLeaveDelay" (time.Duration; default the
// theme's PopoverToken delays: antd 0.1 s / 0.1 s, Element 0 / 0.2 s).
func Popover(c *Context) *PopoverW {
	th := c.ThemeFor(theme.CompPopover)
	tk := &th.Components.Popover
	placement := "top"
	if th.UsesElementPlusControls() {
		placement = "bottom" // Element Plus popover.mjs inherits Dropdown's placement.
	}
	return &PopoverW{c: c,
		place:   Prop(c, "Popover", "placement", placement),
		trigger: Prop(c, "Popover", "trigger", "hover"),
		arrow:   Prop(c, "Popover", "arrow", tk.Arrow),
		enter:   Prop(c, "Popover", "mouseEnterDelay", tk.MouseEnterDelay),
		leave:   Prop(c, "Popover", "mouseLeaveDelay", tk.MouseLeaveDelay),
		th:      th, tk: tk,
	}
}

// MouseEnterDelay waits d before a hover popover opens (antd
// mouseEnterDelay, Element show-after).
func (p *PopoverW) MouseEnterDelay(d time.Duration) *PopoverW { p.enter = d; return p }

// MouseLeaveDelay keeps a hover popover open for d after the pointer left
// the trigger and the panel (antd mouseLeaveDelay, Element hide-after).
func (p *PopoverW) MouseLeaveDelay(d time.Duration) *PopoverW { p.leave = d; return p }

// Arrow shows or hides the arrow (antd arrow; NoArrow is Arrow(false)).
func (p *PopoverW) Arrow(on bool) *PopoverW { p.arrow = on; return p }

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
		active := ia.Hovered || (*open && c.hovered(st.panel)) || kbd
		enter := p.enter
		if kbd {
			enter = 0 // keyboard focus opens at once
		}
		want := p.hoverDebounce(st, active, enter)
		if want != st.hoverWant {
			st.hoverWant = want
			*open = want
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
	off := tk.OffsetNoArrow
	if p.arrow {
		off = tk.Offset
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
	if w := p.width; w > 0 {
		ov.n.ln.Width = w
	} else if tk.Width > 0 && !p.noPadding && !p.autoWidth {
		// Element width 150, shadcn w-72: a fixed default width.
		ov.n.ln.Width = tk.Width
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
	if p.arrow && th.UsesElementPlusControls() {
		elementPopupArrow(c, ov.n.id, tk.Bg, tk.BorderColor)
	} else if p.arrow {
		anchor := b.n.id
		ov.n.over = func(l *paint.List, r geom.Rect) {
			ar, ok := c.anchorRect(anchor)
			if !ok {
				return
			}
			drawPopupArrow(l, r, ar, tk.Bg, tk.BorderColor)
		}
	}
	return b
}

// hoverDebounce applies the hover delays. The leave delay is at least
// 100ms, also without animations (where it is exactly 100ms), so the
// pointer can cross the gap to the panel; the enter delay is skipped
// without animations, as in HoverDelay.
func (p *PopoverW) hoverDebounce(st *popoverState, active bool, enter time.Duration) bool {
	c := p.c
	if active != st.hovering {
		st.hovering, st.since = active, c.now
	}
	if active == st.hoverOpen {
		return st.hoverOpen
	}
	const grace = 100 * time.Millisecond
	d := max(p.leave, grace)
	if c.noAnim {
		d = grace
	}
	if active {
		d = enter
		if c.noAnim {
			d = 0
		}
	}
	if d <= 0 || c.now.Sub(st.since) >= d {
		st.hoverOpen = active
		c.invalid = true
	} else {
		c.WakeAt(st.since.Add(d))
	}
	return st.hoverOpen
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
	// HideArrow hides the panel arrow on this step (antd steps[].arrow
	// false).
	HideArrow bool
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

	keyboard   bool
	noInteract bool
	closable   bool
	maskColor  theme.Color
	maskSet    bool
	gapRadius  float32 // < 0: the theme's HoleRadius
	place      string
}

// tourState is the retained state of a Tour.
type tourState struct {
	revealed int // 1 + the step whose target was scrolled into view (0: none)
	mask     struct {
		key tourMaskKey
		ops []paint.Op
		ok  bool
	}
}

// tourMaskKey is what a Tour mask's paint ops depend on.
type tourMaskKey struct {
	r, hole   geom.Rect
	holeR     float32
	hasTarget bool
	col       theme.Color
	scale     float32
}

// Tour starts a tour bound to *open and *current; call Show.
func Tour(c *Context, open *bool, current *int, steps []TourStep) *TourW {
	t := &TourW{c: c, open: open, cur: current, steps: steps, mask: true, arrow: true, gap: 6, reveal: true, id: c.newID("tour", ""),
		keyboard: true, closable: true, gapRadius: -1}
	t.configDefaults() // Defaults("Tour", ...): type, mask, arrow
	t.keyboard = Prop(c, "Tour", "keyboard", t.keyboard)
	t.noInteract = Prop(c, "Tour", "disabledInteraction", false)
	t.place = Prop(c, "Tour", "placement", "")
	return t
}

// Keyboard sets whether Escape closes the tour and ←/→ move between
// steps (default true; antd keyboard, Element close-on-press-escape).
func (t *TourW) Keyboard(on bool) *TourW { t.keyboard = on; return t }

// DisabledInteraction keeps the highlighted target from being clicked
// (antd disabledInteraction; by default the target stays interactive,
// Element target-area-clickable).
func (t *TourW) DisabledInteraction(on bool) *TourW { t.noInteract = on; return t }

// Closable shows or hides the close button; without it Escape does not
// close the tour either (antd closable null; default shown).
func (t *TourW) Closable(on bool) *TourW { t.closable = on; return t }

// MaskColor sets the mask color (antd mask.color; default the theme's).
// A transparent color keeps the mask's pointer blocking without dimming.
func (t *TourW) MaskColor(col theme.Color) *TourW { t.maskColor, t.maskSet = col, true; return t }

// GapRadius sets the corner radius of the hole around the target (antd /
// Element gap.radius, default 2).
func (t *TourW) GapRadius(px float32) *TourW { t.gapRadius = px; return t }

// Placement sets the default panel placement of the steps, as
// Popover.Placement (default "bottom"); TourStep.Placement overrides it.
func (t *TourW) Placement(s string) *TourW { t.place = s; return t }

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

// Gap sets the hole padding around the target (antd / Element
// gap.offset, default 6).
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
	if *t.cur < 0 || *t.cur >= len(t.steps) {
		// Out of range: nothing is shown and *open is kept (rc-tour
		// mergedOpen is false while current is outside the steps).
		t.revealTarget(0, false)
		return t
	}
	th := c.ThemeFor(theme.CompTour)
	tk := &th.Components.Tour
	el := th.Flavor == theme.Element
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
	holeR := tk.HoleRadius
	if t.gapRadius >= 0 {
		holeR = t.gapRadius
	}
	maskCol := tk.MaskColor
	if t.maskSet {
		maskCol = t.maskColor
	}
	// Pointer blocking follows rc-tour's Mask, which is rendered with or
	// without the dimming: with a mask the page around the target is
	// blocked (the hole stays clickable unless DisabledInteraction);
	// without one the page stays interactive when there is a target and
	// interaction is allowed, else a transparent layer blocks it all.
	mask := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		hole := hasTarget && !t.noInteract
		if showMask || !hole {
			tourBlockers(c, tr.Expand(t.gap), hole)
		}
	}, layout.Grow(1))
	mask.n.hit, mask.n.blocks = false, false
	// The mask's ops are kept while its geometry holds, so the GPU
	// renderer finds the corner paths in its atlas (keyed by the path).
	st := State[tourState](c, t.id, nil)
	mask.n.paint = func(l *paint.List, r geom.Rect) {
		if !showMask {
			return
		}
		scale := c.PixelScale()
		k := tourMaskKey{r, hole, holeR, hasTarget, maskCol, scale}
		if mc := &st.mask; !mc.ok || mc.key != k {
			var ml paint.List
			paintTourMask(&ml, r, hole, holeR, hasTarget, maskCol, scale)
			mc.key, mc.ops, mc.ok = k, ml.Ops, true
		}
		l.Ops = append(l.Ops, st.mask.ops...)
	}
	// Panel.
	stepPlace := step.Placement
	if stepPlace == "" {
		stepPlace = t.place
	}
	place, _ := c.popPlacement(stepPlace)
	if stepPlace == "" {
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
			if t.closable && IconButton(c, icon.Close, loc.Tour.Close).Size(tk.CloseBtnSize, tk.CloseIconSize).Color(fg2).Clicked() {
				*t.open, t.closed = false, true
				c.invalid = true
			}
		}, layout.Middle(), layout.Gap(8), layout.Margin(geom.Insets{Bottom: tk.HeaderMarginBottom}))
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
		}, layout.Middle(), layout.Gap(8), layout.Margin(geom.Insets{Top: tk.FooterMarginTop}))
	}, layout.Pad(tk.PanelPadding), tourWidth(tk))
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
	if t.arrow && !step.HideArrow && hasTarget {
		target := step.Target
		ov.n.over = func(l *paint.List, r geom.Rect) {
			ar, ok := c.anchorRect(target)
			if !ok {
				return
			}
			drawPopupArrow(l, r, ar.Expand(t.gap), bg, theme.Color{})
		}
	}
	if c.EscapeAt(ov.ID()) && t.keyboard && t.closable {
		*t.open, t.closed = false, true
		c.invalid = true
	}
	if t.keyboard && *t.open {
		t.arrowKeys(ov.n)
	}
	return t
}

// arrowKeys moves between steps with ←/→ (rc-tour keyboard) while focus
// is in the panel (where a masked tour traps it) or nowhere: a focused
// page widget (the clickable target, Tabs, Slider) keeps its arrow keys,
// and so does a widget in the panel that handles keys itself.
func (t *TourW) arrowKeys(panel *node) {
	c := t.c
	if c.focus != 0 {
		f := findFocused(panel, c.focus)
		if f == nil || f.ownKeys {
			return
		}
	}
	for _, k := range c.keys {
		if !k.Down {
			continue
		}
		switch k.Key {
		case input.KeyLeft:
			if *t.cur > 0 {
				t.goTo(*t.cur - 1)
			}
		case input.KeyRight:
			if *t.cur < len(t.steps)-1 {
				t.goTo(*t.cur + 1)
			}
		}
	}
}

// tourWidth is the panel width option: a maximum when the panel fits its
// content (antd width 520 with max-width fit-content), else fixed.
func tourWidth(tk *theme.TourToken) layout.Option {
	if tk.FitContent {
		return layout.MaxW(tk.Width)
	}
	return layout.W(tk.Width)
}

// paintTourMask paints the Tour's dimming mask over r with a hole of
// radius holeR around hole (no hole when hasTarget is false), for a
// window at the given device scale.
//
// It paints the pixels of one path "r minus the rounded hole" without a
// window-sized path, which the GPU renderer cannot atlas and would
// rasterize on the CPU every frame: solid rects around the hole, thin
// rects along its straight edges, and at each corner a small path of the
// corner box minus the hole. Pieces tile the area without overlap and
// their seams are snapped to whole device pixels, so no pixel is blended
// twice (a seam at a fractional device position would show as a line)
// and only the hole's own edges are anti-aliased, as before.
//
// Renderers round a FillRect's blend slightly differently from a
// FillPath's, so the solid area may differ from the single path by one
// color level; the shape and the anti-aliasing are the same.
func paintTourMask(l *paint.List, r, hole geom.Rect, holeR float32, hasTarget bool, col theme.Color, scale float32) {
	if !hasTarget {
		l.Fill(r, geom.Radii{}, col)
		return
	}
	if scale <= 0 {
		scale = 1
	}
	s := float64(scale)
	fl := func(v float32) float32 { return float32(math.Floor(float64(v)*s) / s) }
	ce := func(v float32) float32 { return float32(math.Ceil(float64(v)*s) / s) }
	hole.W, hole.H = max(hole.W, 0), max(hole.H, 0)
	rad := geom.Rad(holeR).Clamp(hole.W, hole.H).TL
	x0, y0, x1, y1 := hole.X, hole.Y, hole.Right(), hole.Bottom()
	X0, Y0, X1, Y1 := fl(x0), fl(y0), ce(x1), ce(y1)
	// Inner grid lines: between xa and xb (ya and yb) the hole's edges are
	// straight. When the corners leave no room, both meet at the middle.
	split := func(lo, hi, a0, a1 float32) (float32, float32) {
		a, b := ce(lo+rad), fl(hi-rad)
		if a > b {
			m := min(max(fl((lo+hi)/2), a0), a1)
			return m, m
		}
		return a, b
	}
	xa, xb := split(x0, x1, X0, X1)
	ya, yb := split(y0, y1, Y0, Y1)
	rect := func(ax, ay, bx, by float32) {
		l.Fill(geom.Rect{X: ax, Y: ay, W: bx - ax, H: by - ay}.Intersect(r), geom.Radii{}, col)
	}
	// Around the hole's grid-snapped box.
	rect(r.X, r.Y, r.Right(), Y0)
	rect(r.X, Y0, X0, Y1)
	rect(X1, Y0, r.Right(), Y1)
	rect(r.X, Y1, r.Right(), r.Bottom())
	// Along the straight edges, up to the hole.
	rect(xa, Y0, xb, y0)
	rect(X0, ya, x0, yb)
	rect(x1, ya, X1, yb)
	rect(xa, y1, xb, Y1)
	// Corners: the box minus the hole (non-zero rule), from the same
	// flattened outline the single path used.
	var outline []geom.Point
	corner := func(bx0, by0, bx1, by1 float32) {
		box := geom.Rect{X: bx0, Y: by0, W: bx1 - bx0, H: by1 - by0}
		if box.Empty() || box.Intersect(r).Empty() {
			return
		}
		if outline == nil {
			if pl := paint.NewPath().RRect(hole, geom.Rad(holeR)).Flatten(0.1); len(pl) > 0 {
				outline = pl[0].Pts
			}
		}
		in := clipToRect(outline, box)
		if len(in) < 3 {
			l.Fill(box, geom.Radii{}, col)
			return
		}
		p := paint.NewPath().Rect(box)
		p.MoveTo(in[len(in)-1].X, in[len(in)-1].Y)
		for i := len(in) - 2; i >= 0; i-- {
			p.LineTo(in[i].X, in[i].Y)
		}
		l.Path(p.Close(), col)
	}
	corner(X0, Y0, xa, ya)
	corner(xb, Y0, X1, ya)
	corner(xb, yb, X1, Y1)
	corner(X0, yb, xa, Y1)
}

// clipToRect clips the convex polygon pts to b (Sutherland–Hodgman).
// Points on b's edges get b's exact coordinates.
func clipToRect(pts []geom.Point, b geom.Rect) []geom.Point {
	edge := func(in []geom.Point, inside func(geom.Point) bool, cross func(p, q geom.Point) geom.Point) []geom.Point {
		var out []geom.Point
		for i, q := range in {
			p := in[(i+len(in)-1)%len(in)]
			switch qi, pi := inside(q), inside(p); {
			case qi && pi:
				out = append(out, q)
			case qi:
				out = append(out, cross(p, q), q)
			case pi:
				out = append(out, cross(p, q))
			}
		}
		return out
	}
	atX := func(x float32) func(p, q geom.Point) geom.Point {
		return func(p, q geom.Point) geom.Point { return geom.Pt(x, p.Y+(x-p.X)/(q.X-p.X)*(q.Y-p.Y)) }
	}
	atY := func(y float32) func(p, q geom.Point) geom.Point {
		return func(p, q geom.Point) geom.Point { return geom.Pt(p.X+(y-p.Y)/(q.Y-p.Y)*(q.X-p.X), y) }
	}
	x0, y0, x1, y1 := b.X, b.Y, b.Right(), b.Bottom()
	pts = edge(pts, func(p geom.Point) bool { return p.X >= x0 }, atX(x0))
	pts = edge(pts, func(p geom.Point) bool { return p.X <= x1 }, atX(x1))
	pts = edge(pts, func(p geom.Point) bool { return p.Y >= y0 }, atY(y0))
	return edge(pts, func(p geom.Point) bool { return p.Y <= y1 }, atY(y1))
}

// tourBlockers builds the four pointer-blocking strips of a Tour mask
// around hole (one full-screen strip when open is false).
func tourBlockers(c *Context, hole geom.Rect, open bool) {
	block := func(r geom.Rect) {
		if r.W <= 0 || r.H <= 0 {
			return
		}
		n := c.add("tour-mask", "")
		n.aHidden = true
		n.hit, n.blocks = true, true
		n.ln.Absolute, n.ln.NoMirror = true, true
		n.ln.Width, n.ln.Height = r.W, r.H
		n.ln.Pos = flexbox.Pos{Left: r.X, Top: r.Y, Right: layout.Unset, Bottom: layout.Unset}
	}
	w, h := c.size.W, c.size.H
	if !open {
		block(geom.Rect{W: w, H: h})
		return
	}
	block(geom.Rect{W: w, H: hole.Y})                                            // above
	block(geom.Rect{Y: hole.Bottom(), W: w, H: h - hole.Bottom()})               // below
	block(geom.Rect{Y: hole.Y, W: hole.X, H: hole.H})                            // start side
	block(geom.Rect{X: hole.Right(), Y: hole.Y, W: w - hole.Right(), H: hole.H}) // end side
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
