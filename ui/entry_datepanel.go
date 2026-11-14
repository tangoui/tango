package ui

// The shared text-field trigger and footer helpers of the date/time pickers
// (entry_time.go, entry_datetime.go, entry_daterange.go). Date arithmetic
// and the month panel live in date_util.go.

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

// ---------------------------------------------------------------------------
// Text field trigger shared by TimePicker and DateTimePicker.

type dtFieldState struct {
	pop     entPopup
	edit    editState
	buf     string
	editing bool
	clearR  geom.Rect // clear button of the last paint
}

// dtField is an input frame with an editor, a picker icon and a clear button.
type dtField struct {
	c          *Context
	n, ed      *node
	ia, eia    Interaction
	st         *dtFieldState
	ph         string
	size       ControlSize
	status     string
	variant    string // frame look (see InputW.Variant)
	disabled   bool
	allowClear bool
	ic         *icon.Icon
	display    func() string
	// Semantics of the open panel, set by the owner while building it:
	// the popup the combobox controls and its active item.
	ctl, act ID
}

func dtNewField(c *Context, kind string, ic *icon.Icon) *dtField {
	f := &dtField{c: c, ic: ic, status: c.itemStatus, allowClear: true, size: c.defaultSize()}
	f.n = c.add(kind, "")
	f.n.ln.Dir = layout.Row
	f.n.ln.Align = layout.AlignCenter
	f.n.ln.NoStretch = true
	f.ed = &node{id: hashID(f.n.id, "editor"), parent: f.n, disabled: f.n.disabled}
	f.ed.ln.Ref = f.ed
	f.ed.ln.Grow = 1
	f.ed.ln.Measure = func(_, _ float32) geom.Size {
		return geom.Size{W: 60, H: c.Theme.LineHeightFor(c.controlFont(f.size))}
	}
	f.n.kids = append(f.n.kids, f.ed)
	f.ed.ownKeys = true
	// The editor is the focusable editable combobox; the frame around it
	// (a click target that opens the panel) is an unnamed group whose
	// keyboard equivalent is the editor.
	f.ed.setRole(a11y.RoleCombobox).setState(a11y.Editable | a11y.Expandable | a11y.HasPopup).fill(f)
	f.n.setRole(a11y.RoleGroup)
	f.n.aKbd = true
	f.ia = c.interact(f.n, false, input.CursorPointer)
	f.eia = c.interact(f.ed, true, input.CursorText)
	f.n.paint = f.paintFrame
	f.ed.paint = f.paintText
	return f
}

func (f *dtField) semantics(s *a11y.Node) {
	s.Placeholder = f.ph
	if f.st.editing {
		s.Value = f.st.buf
	} else {
		s.Value = f.display()
	}
	if s.Name == "" {
		s.Name = f.ph
	}
	if f.st.pop.open {
		semExpanded(s)
		s.Controls, s.ActiveDescendant = a11y.NodeID(f.ctl), a11y.NodeID(f.act)
	}
	if f.status == "error" {
		s.States |= a11y.Invalid
	}
}

func (f *dtField) setDisabled() {
	f.disabled = true
	f.n.disabled, f.ed.disabled, f.ed.focus = true, true, false
	f.n.cursor, f.ed.cursor = input.CursorNotAllowed, input.CursorNotAllowed
}

func (f *dtField) focused() bool { return f.c.focus == f.ed.id }

func (f *dtField) padding() geom.Insets {
	th := f.c.Theme
	is := float32(14)
	switch th.Flavor {
	case theme.Element:
		return geom.Insets{Left: 11 + is + 8, Right: 11 + is + 4}
	case theme.StyleShadcn:
		// As DatePicker's shadcn field (date.go): px-3, a 16px icon at the
		// end with 6px before it.
		return geom.Insets{Left: th.ControlPaddingH, Right: th.ControlPaddingH + 16 + 6}
	}
	return geom.Insets{Left: 11, Right: 11 + is + 4}
}

// process runs the shared input logic. It returns the navigation keys meant
// for the panel (only while open), whether the text was edited, whether
// Enter was pressed with the panel closed, and whether clear was clicked.
func (f *dtField) process(hasValue bool) (nav []input.KeyEvent, edited, submitted, cleared bool) {
	c := f.c
	st := f.st
	f.n.ln.Height = c.controlHeight(f.size)
	f.n.ln.Padding = f.padding()
	if f.disabled || f.n.disabled {
		st.pop.open = false
		st.editing = false
		return
	}
	if f.ia.Clicked && f.allowClear && hasValue && f.st.clearR.Expand(3).Contains(c.pointer) {
		cleared = true
		st.pop.open = false
		st.buf = ""
		c.invalid = true
		return
	}
	if f.ia.PressedNow || f.eia.PressedNow {
		if !f.eia.PressedNow {
			c.SetFocus(f.ed.id)
		}
		if !st.pop.open {
			st.pop.open = true
			c.invalid = true
		}
	}
	if act, _, ok := c.A11yAction(f.ed.id); ok && (act == a11y.Expand || act == a11y.Collapse) {
		st.pop.open = act == a11y.Expand
		c.SetFocus(f.ed.id)
		c.invalid = true
	}
	if st.pop.open && c.focus == 0 && c.pressedAny && c.rects[st.pop.panel].Contains(c.pressPos) {
		// Clicks on the (non-focusable) panel options keep the keyboard
		// in the field.
		c.SetFocus(f.ed.id)
	}
	if st.pop.dismiss(c, f.n.id) && c.focus == f.ed.id && c.PressedOutside(f.n.id) {
		c.SetFocus(0)
	}
	if f.focused() {
		var rest []input.KeyEvent
		for _, k := range c.keys {
			isNav := k.Key == input.KeyUp || k.Key == input.KeyDown || k.Key == input.KeyEnter ||
				k.Key == input.KeyPageUp || k.Key == input.KeyPageDown ||
				(st.pop.open && (k.Key == input.KeyLeft || k.Key == input.KeyRight))
			if !isNav {
				rest = append(rest, k)
				continue
			}
			if !k.Down {
				continue
			}
			if !st.pop.open {
				if k.Key == input.KeyEnter {
					submitted = true
				} else if k.Key == input.KeyDown {
					st.pop.open = true
					c.invalid = true
				}
				continue
			}
			nav = append(nav, k)
		}
		c.keys = rest
		if !st.editing {
			st.editing = true
			st.buf = f.display()
		}
		res := c.processEdit(f.ed.id, &st.edit, &st.buf, editConfig{}, f.eia)
		edited = res.changed
	} else if st.editing {
		st.editing = false
	}
	return
}

// setText replaces the edit buffer (after a panel pick) and moves the caret
// to the end.
func (f *dtField) setText(s string) {
	f.st.buf = s
	n := len([]rune(s))
	f.st.edit.cur, f.st.edit.anc = n, n
}

func (f *dtField) paintFrame(l *paint.List, r geom.Rect) {
	c, th := f.c, f.c.Theme
	dis := f.disabled || f.n.disabled
	rad := geom.Rad(c.controlRadius(f.size))
	if f.variant == "underlined" {
		rad = geom.Radii{}
	}
	hover := f.ia.Hovered || f.eia.Hovered
	c.paintFrame(l, r, rad, f.n.id, frameState{hover: hover, focus: f.st.pop.open || f.focused(), disabled: dis, status: f.status, variant: f.variant})
	is := float32(14)
	iy := r.Y + (r.H-is)/2
	col := th.TextQuaternary
	if dis {
		col = th.TextDisabled
	}
	// Icon and clear sit at the logical ends (the padding mirrors with
	// the layout; these are painted, so mirror them here).
	rtl := c.IsRTL()
	f.st.clearR = dtMirrorRect(rtl, geom.Rect{X: r.Right() - 11 - is, Y: iy, W: is, H: is}, r)
	showClear := f.allowClear && !dis && hover && f.display() != ""
	switch th.Flavor {
	case theme.Element:
		drawIcon(l, f.ic, dtMirror(rtl, r, r.X+11, is), iy, is, col)
		if showClear {
			icon.Draw(l, icon.CloseCircle, f.st.clearR, th.TextQuaternary, 0)
		}
		return
	case theme.StyleShadcn:
		// As DatePicker's shadcn field (date.go): a 16px icon in
		// foreground/50 at px-3 from the end.
		f.st.clearR = dtMirrorRect(rtl, geom.Rect{X: r.Right() - th.ControlPaddingH - 16, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, r)
		if showClear {
			entClearIcon(c, l, f.st.clearR)
		} else {
			drawIcon(l, f.ic, f.st.clearR.X, f.st.clearR.Y, 16, theme.Fade(th.Text, .5))
		}
		return
	}
	if showClear {
		entClearIcon(c, l, f.st.clearR)
		return
	}
	drawIcon(l, f.ic, f.st.clearR.X, iy, is, col)
}

func (f *dtField) paintText(l *paint.List, r geom.Rect) {
	c, th := f.c, f.c.Theme
	st := c.style(c.controlFont(f.size), text.Regular)
	col := th.Text
	if f.disabled || f.n.disabled {
		col = th.TextDisabled
	}
	l.Clip(r.Expand(1), geom.Radii{})
	defer l.Pop()
	if f.focused() || f.st.editing {
		c.paintEditor(l, r, f.ed.id, &f.st.edit, f.st.buf, st, col, f.ph, false, false)
		return
	}
	s, tc := f.display(), col
	if s == "" {
		s, tc = f.ph, th.TextQuaternary
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true, Ellipsis: true, MaxWidth: r.W})
	l.Glyphs(geom.Pt(dtTextX(c.IsRTL(), r, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, tc)
}

// ---------------------------------------------------------------------------
// Footer helpers

// dtFooter lays out a panel footer with a top divider.
func dtFooter(c *Context, fn func()) {
	th := c.Theme
	h := float32(40)
	if th.Flavor == theme.Element {
		h = 40
	}
	Row(c, fn, layout.H(h), layout.PadVH(0, 12), layout.Middle(), layout.Gap(8)).Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Components.DatePicker.SplitColor)
	})
}

// dtTextBtn is the small text button of Element picker footers.
func dtTextBtn(c *Context, label string, primary bool) bool {
	th := c.Theme
	n := c.add("dt-btn", label)
	n.ln.NoStretch = true
	fs := th.FontSizeSM
	wt := text.Regular
	if primary {
		wt = text.Medium
	}
	n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: entTextW(c, label, fs, wt) + 10, H: 28} }
	n.setRole(a11y.RoleButton)
	ia := c.interact(n, true, input.CursorPointer)
	n.paint = func(l *paint.List, r geom.Rect) {
		col := th.Text
		if primary {
			col = th.Primary.Base
		}
		if ia.Hovered {
			col = th.Primary.Hover
		}
		entText(c, l, label, r.X+5, r, fs, wt, col, 0)
		if ia.FocusVisible {
			entFocusRing(c, l, r, th.RadiusSM)
		}
	}
	return ia.Clicked
}

// ---------------------------------------------------------------------------
// Typing into a picker trigger that keeps the keyboard on its own node
// (DatePicker, RangePicker, TimeRangePicker).

// dtTyping is the retained inline editor of such a trigger. While on, the
// trigger shows buf with a caret instead of the value; nothing is
// committed until buf parses (Enter, or leaving the field).
type dtTyping struct {
	edit editState
	buf  string
	on   bool
}

// dtTypeEvt is what dtTypeInput saw this frame.
type dtTypeEvt struct {
	nav     []input.KeyEvent // keys left for the picker (navigation, Enter, Space while not typing)
	started bool             // typing began this frame (the picker should open)
	edited  bool             // buf changed
	blurred bool             // focus left while typing (commit buf if valid)
}

// dtTypeEditKey reports keys that belong to the text editor: deletion and
// clipboard always, caret movement only while typing.
func dtTypeEditKey(k input.KeyEvent, typing bool) bool {
	switch k.Key {
	case input.KeyBackspace, input.KeyDelete:
		return true
	case input.KeyA, input.KeyC, input.KeyX, input.KeyV:
		return k.Mods.Shortcut()
	case input.KeyLeft, input.KeyRight, input.KeyHome, input.KeyEnd:
		return typing
	}
	return false
}

// dtTypeInput splits this frame's keyboard input of the focused trigger id
// between the picker (returned in nav) and the inline editor t. Printable
// text, Backspace / Delete and paste start typing: text and paste replace
// the shown value, deletion edits it from the end.
func dtTypeInput(c *Context, id ID, t *dtTyping, display func() string) (ev dtTypeEvt) {
	if c.focus != id {
		if t.on {
			t.on, ev.blurred = false, true
		}
		return
	}
	var edit []input.KeyEvent
	start, replace := false, false
	for _, k := range c.keys {
		if !dtTypeEditKey(k, t.on) {
			ev.nav = append(ev.nav, k)
			continue
		}
		if !t.on && k.Down && k.Key != input.KeyA && k.Key != input.KeyC && k.Key != input.KeyX {
			start = true
			replace = k.Key == input.KeyV
		}
		if t.on || start {
			edit = append(edit, k)
		}
	}
	var typed []string
	for _, s := range c.text {
		if !t.on && !start && strings.TrimSpace(s) == "" {
			continue // the Space key opens the panel
		}
		typed = append(typed, s)
		if !t.on && !start {
			start, replace = true, true
		}
	}
	c.keys, c.text = edit, typed
	if start && !t.on {
		t.on, ev.started = true, true
		t.buf = display()
		n := len([]rune(t.buf))
		t.edit.cur, t.edit.anc, t.edit.wasFocused = n, n, true
		if replace {
			t.edit.anc = 0
		}
	}
	if t.on {
		ev.edited = c.processEdit(id, &t.edit, &t.buf, editConfig{}, Interaction{}).changed
	}
	c.keys, c.text = nil, nil
	return
}

// dtFocusIn reports whether the focused widget lies in the panel (a footer
// button clicked while typing must not close it).
func dtFocusIn(c *Context, panel ID) bool {
	if c.focus == 0 || panel == 0 {
		return false
	}
	pr, fr := c.rects[panel], c.rects[c.focus]
	return !fr.Empty() && pr.Contains(fr.Center())
}

// dtPaintTyping draws the inline editor of a typing trigger in r (the text
// area), or requests text input for a focused trigger that is not typing
// yet so that the host delivers keystrokes (soft keyboards, IME).
func dtPaintTyping(c *Context, l *paint.List, r geom.Rect, id ID, t *dtTyping, st text.Style, col theme.Color, ph string) bool {
	if c.focus != id {
		return false
	}
	if !t.on {
		c.WantTextInput(geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H})
		return false
	}
	l.Clip(r.Expand(1), geom.Radii{})
	c.paintEditor(l, r, id, &t.edit, t.buf, st, col, ph, false, false)
	l.Pop()
	return true
}
