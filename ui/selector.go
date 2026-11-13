package ui

// The selector shell shared by MultiSelect, TreeSelect, Cascader and
// InputTag: the frame (border, hover / focus / status colors, size, clear
// button, arrow) and the tag area (closable tags, MaxTagCount folding into
// "+ N ...", inline search editor). Single-value selectors (TreeSelect,
// Cascader without Multiple) use the same shell with the value as text.

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// selectorState is the retained part of a selector: the search text, its
// editor state and where the clear button was drawn.
type selectorState struct {
	edit      editState
	query     string
	clearRect geom.Rect
}

// selector lays out value tags and an inline search editor in a wrapping
// row and draws the frame, arrow and clear button. The owning widget sets
// the fields, calls build from its lazy finish and reads removed /
// chipClicked / clicked() / clearClicked() afterwards.
type selector struct {
	c           *Context
	th          *theme.Theme // Select theme (its Alias applied)
	n, ed       *node
	st          *selectorState
	ia, edIA    Interaction
	size        ControlSize
	status      string
	disabled    bool
	placeholder string
	allowClear  bool
	arrow       bool
	arrowIcon   *icon.Icon
	search      bool   // the editor accepts text
	open        bool   // dropdown shown (focus look, arrow rotation)
	maxCount    int    // collapse chips after this many (0 = all)
	single      string // single-value selectors: the value label
	hasValue    bool

	removed     int  // chip whose close icon was clicked (-1 none)
	chipClicked bool // a chip body was clicked

	// Semantics: the frame is the combobox (the editor is part of it).
	labels   []string // selected value labels (Value of the combobox)
	listID   ID       // the popup's listbox / tree while open
	activeID ID       // highlighted item of the popup (ActiveDescendant)
}

// newSelector adds the frame node of a selector widget of the given kind.
func newSelector(c *Context, kind string) *selector {
	b := &selector{c: c, status: c.itemStatus, arrow: true, removed: -1, size: c.defaultSize()}
	b.th = c.ThemeFor(theme.CompSelect)
	b.n = c.add(kind, "")
	b.st = State[selectorState](c, hashID(b.n.id, "ms-box"), nil)
	b.ia = c.interact(b.n, false, input.CursorPointer)
	b.n.paint = b.paint
	// The frame is the combobox; keyboard focus sits in its editor (a
	// hidden part, so focus is reported here).
	b.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(b)
	b.n.aKbd = true
	return b
}

// popupName names the selector's popup list: the explicit name, else the
// placeholder, else the generic "please select".
func (b *selector) popupName() string { return popupName(b.c, b.n, b.placeholder) }

// popupName names the popup list of trigger: its explicit accessible name,
// else placeholder, else the generic "please select".
func popupName(c *Context, trigger *node, placeholder string) string {
	if trigger.aNameSet && trigger.aName != "" {
		return trigger.aName
	}
	if placeholder != "" {
		return placeholder
	}
	return c.Locale().SelectPlaceholder
}

// semantics completes the combobox node: value (selected labels),
// placeholder fallback name, popup relations. Owners name it with
// b.n.setName (A11yLabel) and set listID / activeID while open.
func (b *selector) semantics(s *a11y.Node) {
	if b.single != "" {
		s.Value = b.single
	} else {
		s.Value = strings.Join(b.labels, ", ")
	}
	s.Placeholder = b.placeholder
	if s.Name == "" {
		s.Name = b.placeholder
	}
	if b.search {
		s.States |= a11y.Editable
	}
	if b.open {
		s.States |= a11y.Expanded
		s.Controls = a11y.NodeID(b.listID)
		s.ActiveDescendant = a11y.NodeID(b.activeID)
	}
	if b.status == "error" {
		s.States |= a11y.Invalid
	}
}

func (b *selector) font() text.Style { return b.c.style(controlFontOf(b.th, b.size), text.Regular) }

// selectFrameTok reads the frame tokens of the Select component of th
// (the halo geometry and the status halos come from its Input token).
func selectFrameTok(th *theme.Theme) frameTok {
	ft := inputFrameTok(th)
	k := &th.Components.Select
	ft.bg, ft.hoverBg, ft.activeBg = k.SelectorBg, k.SelectorBg, k.SelectorBg
	ft.hoverBorder, ft.activeBorder, ft.outline = k.HoverBorderColor, k.ActiveBorderColor, k.ActiveOutlineColor
	return ft
}

// build adds chips for labels and the editor. Call it from the owner's lazy
// finish (current parent is the frame).
func (b *selector) build(labels []string) {
	c, th := b.c, b.th
	n := b.n
	n.ln.Dir = layout.Row
	n.ln.Wrap = true
	n.ln.Align = layout.AlignCenter
	gap := float32(4)
	if th.Flavor == theme.Element {
		gap = 6
	}
	n.ln.Gap, n.ln.LineGap = gap, 4
	n.ln.MinH = controlHeightOf(th, b.size)
	ch := selectorTagH(c, b.size)
	pv := max(1, (controlHeightOf(th, b.size)-ch)/2)
	ais, aright, _ := entArrowLook(c)
	padR := float32(11)
	if b.arrow || b.allowClear {
		padR += 18
	}
	if th.Flavor == theme.StyleShadcn {
		padR = aright
		if b.arrow || b.allowClear {
			padR += ais + 6
		}
	}
	n.ln.Padding = geom.Insets{Top: pv, Bottom: pv, Left: 4, Right: padR}
	if b.disabled {
		n.disabled = true
		n.cursor = input.CursorNotAllowed
	}
	b.hasValue = len(labels) > 0 || b.single != ""
	b.labels = labels
	b.removed, b.chipClicked = selectorTags(c, labels, b.maxCount, !b.disabled, b.disabled, b.size)
	ed := c.add("ms-input", b.placeholder)
	b.ed = ed
	ed.aHidden = true // the editor is part of the combobox
	ed.ln.Grow = 1
	ed.disabled = b.disabled
	padL := float32(7)
	if th.Flavor == theme.Element {
		padL = 8
	}
	if th.Flavor == theme.StyleShadcn {
		padL = th.ControlPaddingH - 4
	}
	if len(labels) == 0 {
		ed.ln.Margin.Left = padL
	}
	// Nothing to type (or not focused and empty): the editor takes no room.
	hidden := len(labels) > 0 && (!b.search || (c.focus != ed.id && b.st.query == ""))
	if hidden {
		ed.ln.Margin.Left = -gap
	}
	fsz := b.font()
	ed.ln.Measure = func(maxW, _ float32) geom.Size {
		if hidden {
			return geom.Size{W: 0, H: ch}
		}
		w := text.Width(b.st.query, fsz) + 8
		if len(labels) == 0 && b.st.query == "" {
			ph := b.placeholder
			if b.single != "" {
				ph = b.single
			}
			w = max(w, text.Width(ph, fsz)+4)
		}
		return geom.Size{W: min(max(w, 8), maxW), H: ch}
	}
	ed.ownKeys = true
	cur := input.CursorPointer
	if b.search {
		cur = input.CursorText
	}
	if b.disabled {
		cur = input.CursorNotAllowed
	}
	b.edIA = c.interact(ed, true, cur)
	ed.paint = b.paintEditor
	if !b.disabled && ((b.ia.PressedNow && !b.edIA.PressedNow) || b.chipClicked || b.removed >= 0) {
		c.SetFocus(ed.id)
	}
}

// focused reports whether the editor has keyboard focus.
func (b *selector) focused() bool { return b.ed != nil && b.c.focus == b.ed.id }

// clicked reports a click on the frame or editor (not on the clear icon).
func (b *selector) clicked() bool {
	return (b.ia.Clicked && !b.clearClicked()) || b.edIA.Clicked || b.chipClicked
}

// clearClicked reports a click on the clear button.
func (b *selector) clearClicked() bool {
	return b.allowClear && b.hasValue && !b.disabled && b.ia.Clicked && b.st.clearRect.Expand(3).Contains(b.c.pointer)
}

// takeKeys removes the key presses accepted by want from the frame's input
// (when the editor is focused) and returns them.
func (b *selector) takeKeys(want func(k input.KeyEvent) bool) []input.KeyEvent {
	if b.ed == nil {
		return nil
	}
	return msTakeKeys(b.c, b.ed.id, want)
}

// edit runs the editor (search text) and consumes remaining input.
func (b *selector) edit() editResult {
	c := b.c
	if b.disabled || b.ed == nil {
		return editResult{}
	}
	if !b.search {
		c.KeysFor(b.ed.id)
		c.TextFor(b.ed.id)
		return editResult{}
	}
	return c.processEdit(b.ed.id, &b.st.edit, &b.st.query, editConfig{}, b.edIA)
}

func (b *selector) paintEditor(l *paint.List, r geom.Rect) {
	c, th := b.c, b.th
	col := th.Text
	if b.disabled {
		col = th.TextDisabled
	}
	ph := ""
	if len(b.n.kids) <= 1 { // no chips
		ph = b.placeholder
	}
	editing := b.search && b.focused() && (b.open || b.st.query != "")
	if !editing {
		s, scol := ph, th.TextQuaternary
		if b.single != "" {
			s, scol = b.single, col
		}
		entTextIn(c, l, s, 0, r, controlFontOf(th, b.size), text.Regular, scol, r.W+1)
		return
	}
	if b.single != "" {
		ph = b.single
	}
	l.Clip(r.Expand(1), geom.Radii{})
	c.entPaintEdit(l, r, b.ed.id, &b.st.edit, b.st.query, b.font(), col, th.TextQuaternary, ph, false)
	l.Pop()
}

func (b *selector) hovered(r geom.Rect) bool {
	c := b.c
	return !b.disabled && c.pointerIn && r.Contains(c.pointer) && c.topLayer == 0
}

func (b *selector) paint(l *paint.List, r geom.Rect) {
	c, th := b.c, b.th
	rad := geom.Rad(controlRadiusOf(th, b.size))
	hover := b.hovered(r)
	focus := b.open || b.focused()
	c.paintFrameWith(l, r, rad, b.n.id, frameState{hover: hover, focus: focus && !b.disabled, disabled: b.disabled, status: b.status}, selectFrameTok(th))
	is, right, _ := entArrowLook(c)
	if th.Flavor == theme.StyleShadcn {
		is = 14
	}
	// Clear button and arrow sit at the inline end.
	cr := c.inline(r).Rect(r.W-right-is, r.Y+(r.H-is)/2, is, is)
	b.st.clearRect = cr
	if b.allowClear && b.hasValue && hover {
		entClearIcon(c, l, cr)
		return
	}
	if b.arrow {
		entArrow(c, l, r, b.n.id, b.open, b.arrowIcon)
	}
}

// ---------------------------------------------------------------------------
// Tags (selected values inside multiple selectors)

// selectorTagH is the chip height for a selector of the given size.
func selectorTagH(c *Context, s ControlSize) float32 {
	tk := &c.ThemeFor(theme.CompSelect).Components.Select
	switch s {
	case Small:
		return tk.MultipleItemHeightSM
	case Large:
		return tk.MultipleItemHeightLG
	}
	return tk.MultipleItemHeight
}

func selectorTagFont(c *Context, s ControlSize) float32 {
	th := c.ThemeFor(theme.CompSelect)
	if th.Flavor != theme.Ant {
		return th.FontSizeSM
	}
	if s == Small {
		return th.FontSizeSM
	}
	return th.FontSize
}

// selectorTag adds a selected-value chip into the current (row) parent. It
// returns 2 when the close icon was clicked, 1 for a click elsewhere on the
// chip and 0 otherwise.
// The label starts at the inline start of the chip, the close icon sits at
// its end.
func selectorTag(c *Context, label string, closable, disabled bool, size ControlSize) int {
	th := c.ThemeFor(theme.CompSelect)
	tk := &th.Components.Select
	n := c.add("selector-tag", label)
	n.ln.NoStretch = true
	h := selectorTagH(c, size)
	fs := selectorTagFont(c, size)
	pad := float32(8)
	if size == Small {
		pad = 4
	}
	if th.Flavor == theme.Element {
		pad = 9
	}
	n.ln.Measure = func(maxW, _ float32) geom.Size {
		w := entTextW(c, label, fs, text.Regular) + 2*pad
		if closable {
			w += 14
		}
		return geom.Size{W: min(w, maxW), H: h}
	}
	if disabled {
		n.disabled = true
	}
	ia := c.interact(n, false, input.CursorDefault)
	if closable && !disabled {
		// Exposed as the tag's remove button (Backspace in the editor
		// removes the last tag from the keyboard).
		n.setRole(a11y.RoleButton)
		n.aTarget = n.id
		if c.a11yOn {
			n.setName(c.Locale().RemoveItem(label))
		}
	}
	res := 0
	if ia.Clicked && !disabled {
		res = 1
		if r := c.rects[n.id]; closable && c.inline(r).Off(c.pointer.X) > r.W-pad-14 {
			res = 2
			c.invalid = true
		}
		if act, _, ok := c.A11yAction(n.id); ok && act == a11y.Press && closable {
			res = 2
			c.invalid = true
		}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		bg, fg, cl := tk.MultipleItemBg, tk.MultipleItemColor, th.TextTertiary
		rad := geom.Rad(th.RadiusSM)
		if th.Flavor == theme.Element {
			cl = th.Info.Base
			rad = geom.Rad(th.Radius)
		}
		if disabled {
			fg = th.TextDisabled
			if th.Flavor == theme.Ant {
				bg = th.Fill
				bg.A /= 2
			}
		}
		l.Fill(r, rad, bg)
		maxW := r.W - 2*pad
		if closable {
			maxW -= 14
		}
		entTextIn(c, l, label, pad, r, fs, text.Regular, fg, maxW)
		if closable && !disabled {
			s := c.inline(r)
			hov := ia.Hovered && s.Off(c.pointer.X) > r.W-pad-14
			cr := s.Rect(r.W-pad-10, r.Y+(r.H-10)/2, 10, 10)
			if th.Flavor == theme.Element {
				cr = s.Rect(r.W-pad-12, r.Y+(r.H-12)/2, 12, 12)
				if hov {
					l.Path(paint.NewPath().Circle(cr.X+6, cr.Y+6, 7), th.Info.Base)
					cl = th.BgContainer
				}
				drawIcon(l, icon.Close, cr.X+2, cr.Y+2, 8, cl)
				return
			}
			if hov {
				cl = th.Text
			}
			drawIcon(l, icon.Close, cr.X, cr.Y, cr.W, cl)
		}
	}
	return res
}

// selectorTags adds chips for labels, collapsing everything after maxCount
// (<= 0: show all) into a "+ N ..." chip. It returns the index of the label
// whose close icon was clicked (-1 none) and whether any chip body was clicked.
func selectorTags(c *Context, labels []string, maxCount int, closable, disabled bool, size ControlSize) (removed int, clicked bool) {
	removed = -1
	shown := len(labels)
	if maxCount > 0 && shown > maxCount {
		shown = maxCount
	}
	for i := 0; i < shown; i++ {
		i := i
		Key(c, labels[i], func() {
			switch selectorTag(c, labels[i], closable, disabled, size) {
			case 2:
				removed = i
			case 1:
				clicked = true
			}
		})
	}
	if rest := len(labels) - shown; rest > 0 {
		if selectorTag(c, "+ "+itoaEnt(rest)+" ...", false, disabled, size) == 1 {
			clicked = true
		}
	}
	return removed, clicked
}
