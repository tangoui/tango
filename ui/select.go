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

type selectState struct {
	open   bool
	active int
	query  string
	edit   editState
	panel  ID // dropdown overlay; its rect is looked up when needed
	// List scrolling: the highlight last shown, whether the list was shown
	// last frame, and the pointer then (hover only moves the highlight
	// after the pointer moves, not when the list scrolls under it).
	shown     int
	listShown bool
	ptr       geom.Point
}

// SelectW is a dropdown select.
type SelectW struct {
	c           *Context
	th          *theme.Theme // Select theme (its Alias applied)
	n           *node
	val         *string
	opts        []Option
	st          *selectState
	placeholder string
	search      bool
	allowClear  bool
	disabled    bool
	status      string
	size        ControlSize
	changed     bool
	ia          Interaction
	z           *lazy
	edID        ID
	optIDs      []ID // option nodes of the open list (ActiveDescendant)
	listID      ID
	a11yName    string // name computed for the combobox (names the listbox)

	rx      p2bRich         // groups, remote, create, render, virtual (p2b_select.go)
	eng     *VirtualSelectW // dropdown engine while rx is in use
	created string
	isNew   bool
}

// Select adds a single-value select bound to *val.
func Select(c *Context, val *string, opts []Option) *SelectW {
	w := &SelectW{c: c, val: val, opts: opts, status: c.itemStatus, size: c.defaultSize()}
	w.th = c.ThemeFor(theme.CompSelect)
	w.n = c.add("select", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.Align = layout.AlignCenter
	w.st = State[selectState](c, w.n.id, nil)
	w.edID = hashID(w.n.id, "search")
	w.n.ln.Measure = nil
	w.n.paint = w.paint
	w.n.ownKeys = true
	w.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(w)
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.rx.defaults(c)
	sb := selector{}
	sb.size = w.size
	p2bSelectDefaults(c, &sb, nil, nil, nil)
	w.allowClear, w.search, w.placeholder, w.size = sb.allowClear, sb.search, sb.placeholder, sb.size
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *SelectW) A11yLabel(s string) *SelectW { w.n.setName(s); return w }

func (w *SelectW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = w.label(), w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	w.a11yName = s.Name
	if w.search {
		s.States |= a11y.Editable
	}
	if w.st.open {
		s.States |= a11y.Expanded
		s.Controls = a11y.NodeID(w.listID)
		if w.eng != nil {
			s.ActiveDescendant = a11y.NodeID(w.eng.st.activeID)
		} else if a := w.st.active; a >= 0 && a < len(w.optIDs) {
			s.ActiveDescendant = a11y.NodeID(w.optIDs[a])
		}
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
}

func (w *SelectW) Placeholder(s string) *SelectW      { w.placeholder = s; return w }
func (w *SelectW) Searchable() *SelectW               { w.search = true; return w }
func (w *SelectW) AllowClear() *SelectW               { w.allowClear = true; return w }
func (w *SelectW) Status(s string) *SelectW           { w.status = s; return w }
func (w *SelectW) Size(s ControlSize) *SelectW        { w.size = s; return w }
func (w *SelectW) Width(px float32) *SelectW          { w.n.ln.Width = px; return w }
func (w *SelectW) Layout(o ...layout.Option) *SelectW { w.n.ln.Style.Apply(o...); return w }
func (w *SelectW) Disabled(on bool) *SelectW {
	if on {
		w.disabled, w.n.disabled, w.n.focus, w.n.cursor = true, true, false, input.CursorNotAllowed
	}
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *SelectW) Open() *SelectW {
	if !w.st.open {
		w.st.open = true
		w.st.active = max(0, w.index(*w.val))
	}
	return w
}

// IsOpen reports whether the dropdown is shown.
func (w *SelectW) IsOpen() bool { return w.st.open }

// Changed reports a new selection this frame.
func (w *SelectW) Changed() bool { w.z.run(); return w.changed }

func (w *SelectW) label() string {
	if w.eng != nil {
		if *w.val == "" {
			return ""
		}
		return w.eng.labelOf(*w.val)
	}
	for _, o := range w.opts {
		if o.Value == *w.val {
			return o.Label
		}
	}
	return ""
}

func (w *SelectW) filtered() []Option {
	if !w.search {
		return w.opts
	}
	return filterOptions(w.opts, w.st.query, w.rx.filter)
}

func (w *SelectW) finish() {
	c := w.c
	st := w.st
	w.n.ln.Height = controlHeightOf(w.th, w.size)
	w.n.ln.Padding = geom.Insets{Left: 11, Right: 11 + 18}
	// Text node for selected label / placeholder / search editor.
	ed := &node{id: w.edID, parent: w.n}
	ed.ln.Ref = ed
	ed.ln.Grow = 1
	ed.ln.Measure = func(_, _ float32) geom.Size {
		return geom.Size{W: max(80, text.Width(w.label(), c.style(controlFontOf(w.th, w.size), text.Regular))), H: 22}
	}
	w.n.kids = append(w.n.kids, ed)
	ed.paint = w.paintValue
	if w.disabled || w.n.disabled {
		st.open = false
		return
	}
	if w.rx.on(w.opts) {
		w.finishRich(ed)
		return
	}
	if w.ia.Clicked {
		if r := c.rects[w.n.id]; w.allowClear && *w.val != "" && c.inline(r).Off(c.pointer.X) > r.W-30 {
			*w.val = ""
			w.changed = true
			st.open = false
		} else if !st.open || !w.search {
			st.open = !st.open
		}
		if st.open {
			st.query = ""
			st.active = max(0, w.index(*w.val))
		}
		c.invalid = true
	}
	if st.open && c.PressedOutside(w.n.id, st.panel) {
		st.open = false
		c.invalid = true
	}
	if st.open && c.EscapeAt(st.panel) {
		st.open = false
	}
	opts := w.filtered()
	if c.focus == w.n.id || c.focus == w.edID {
		for _, k := range c.keys {
			if !k.Down {
				continue
			}
			switch k.Key {
			case input.KeyDown:
				if !st.open {
					st.open = true
					st.active = max(0, w.index(*w.val))
				} else if len(opts) > 0 {
					st.active = (st.active + 1) % len(opts)
				}
			case input.KeyUp:
				if len(opts) > 0 {
					st.active = (st.active - 1 + len(opts)) % len(opts)
				}
			case input.KeyEnter, input.KeySpace:
				if !st.open {
					st.open = true
					st.active = max(0, w.index(*w.val))
				} else if k.Key == input.KeyEnter && st.active < len(opts) && !opts[st.active].Disabled {
					w.pick(opts[st.active])
				}
			}
		}
	}
	ed.aHidden = true // the search editor is part of the combobox
	if st.open && w.search {
		c.SetFocus(w.edID)
		ed.hit, ed.focus = true, true
		if c.processEdit(w.edID, &st.edit, &st.query, editConfig{}, Interaction{}).changed {
			st.active = 0
		}
	}
	if !st.open {
		st.listShown = false
		if c.focus == w.edID {
			c.SetFocus(w.n.id)
		}
		return
	}
	w.dropdown(opts)
	st.ptr = c.pointer
}

func (w *SelectW) index(v string) int {
	for i, o := range w.filtered() {
		if o.Value == v {
			return i
		}
	}
	return -1
}

func (w *SelectW) pick(o Option) {
	if *w.val != o.Value {
		*w.val = o.Value
		w.changed = true
	}
	w.st.open = false
	w.st.query = ""
	w.c.SetFocus(w.n.id)
	w.c.invalid = true
}

func (w *SelectW) dropdown(opts []Option) {
	c, th := w.c, w.th
	tk := &th.Components.Select
	el := th.Flavor == theme.Element
	a := c.AnimFrom(w.n.id, "open", 0, 1, th.DurationMid)
	st := w.st
	follow := !st.listShown || st.active != st.shown
	moved := c.pointer != st.ptr
	optH := tk.OptionHeight
	ov := Overlay(c, OverlayOptions{Anchor: w.n.id, Place: BottomStart, Offset: 4, MatchWidth: true, Flip: true}, func() {
		if len(opts) == 0 {
			Empty(c).Simple().Description(c.Locale().NoData)
			return
		}
		sb := Scroll(c, func() { w.options(opts, moved, optH) }, layout.MaxH(256))
		sb.n.setRole(a11y.RoleListbox)
		if c.a11yOn {
			// The page (and the combobox) is visited before this overlay.
			sb.n.fill(semFunc(func(s *a11y.Node) { s.Name = w.a11yName }))
		}
		w.listID = sb.n.id
		if follow && st.active >= 0 {
			// Keep the highlighted option (the value on open) in view.
			s := sb.s
			view := float64(s.view.H)
			if view <= 0 {
				view = 256
			}
			top, h := float64(st.active)*float64(optH), float64(optH)
			if top < s.off.Y {
				s.off.Y = top
			} else if top+h > s.off.Y+view {
				s.off.Y = top + h - view
			}
		}
	}, layout.PadVH(4, 4))
	st.shown, st.listShown = st.active, true
	if el {
		ov.n.ln.Padding = geom.VH(6, 0)
	}
	ov.Bg(tk.PopupBg).Radius(tk.PopupRadius).Shadow(tk.PopupShadow)
	if tk.PopupBorderColor.A > 0 {
		ov.Border(1, tk.PopupBorderColor)
	}
	ov.Opacity(a)
	ov.n.clip = true
	w.st.panel = ov.n.id
}

// options builds the option rows of the dropdown list.
func (w *SelectW) options(opts []Option, moved bool, optH float32) {
	c, th := w.c, w.th
	tk := &th.Components.Select
	el := th.Flavor == theme.Element
	{
		w.optIDs = w.optIDs[:0]
		for i, o := range opts {
			i, o := i, o
			it := c.add("option", o.Label)
			w.optIDs = append(w.optIDs, it.id)
			it.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(o.Value == *w.val, a11y.Selected)
			it.ln.Height = optH
			it.ln.Measure = func(_, _ float32) geom.Size {
				return geom.Size{W: text.Width(o.Label, c.style(th.FontSize, text.Semibold)) + 24, H: 32}
			}
			ia := c.interact(it, false, input.CursorPointer)
			if o.Disabled {
				it.disabled = true
				it.cursor = input.CursorNotAllowed
			}
			if ia.Hovered && moved && !o.Disabled {
				w.st.active = i
			}
			if ia.Clicked && !o.Disabled {
				w.pick(o)
			}
			sel := o.Value == *w.val
			it.paint = func(l *paint.List, r geom.Rect) {
				if th.Flavor == theme.StyleShadcn {
					entPaintOption(c, l, r, o.Label, entOptState{selected: sel, active: w.st.active == i, disabled: o.Disabled})
					return
				}
				col := th.Text
				wt := text.Regular
				if el {
					col = th.TextSecondary
				}
				if !el {
					r = r.Inset(geom.Insets{})
				}
				rad := geom.Rad(th.RadiusSM)
				if el {
					rad = geom.Radii{}
				}
				switch {
				case o.Disabled:
					col = th.TextDisabled
				case sel:
					col, wt = tk.OptionSelectedColor, themeWeight(tk.OptionSelectedFontWeight)
					if !el || w.st.active == i {
						bg := tk.OptionSelectedBg
						if el {
							bg = tk.OptionActiveBg
						}
						l.Fill(r, rad, bg)
					}
				case w.st.active == i:
					l.Fill(r, rad, tk.OptionActiveBg)
				}
				padL := tk.OptionPadding
				lay := text.Shape(o.Label, c.style(tk.OptionFontSize, wt), text.Options{NoWrap: true, Ellipsis: true, MaxWidth: r.W - padL - 12})
				l.Glyphs(geom.Pt(c.inline(r).X(padL, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			}
		}
	}
}

func (w *SelectW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.th
	rad := geom.Rad(controlRadiusOf(th, w.size))
	dis := w.disabled || w.n.disabled
	focus := w.st.open || c.focus == w.n.id || c.focus == w.edID
	c.paintFrameWith(l, r, rad, w.n.id, frameState{hover: w.ia.Hovered, focus: focus, disabled: dis, status: w.status}, selectFrameTok(th))
	is, right, col := entArrowLook(c)
	if th.Flavor == theme.Element {
		is, col = 12, th.TextTertiary
	}
	x := c.inline(r).X(r.W-right-is, is) // arrow / clear at the inline end
	y := r.Y + (r.H-is)/2
	if w.allowClear && *w.val != "" && w.ia.Hovered && !dis {
		entClearIcon(c, l, geom.Rect{X: x, Y: y, W: is, H: is})
		return
	}
	ic := icon.Down
	ang := 0.0
	if w.search && w.st.open {
		ic = icon.Search
	} else if th.Flavor == theme.Element {
		ang = float64(c.Anim(w.n.id, "arrow", b2f(w.st.open), th.DurationMid)) * 3.14159
	}
	c.paintIcon(l, ic, geom.Rect{X: x, Y: y, W: is, H: is}, col, ang)
}

func (w *SelectW) paintValue(l *paint.List, r geom.Rect) {
	c, th := w.c, w.th
	st := c.style(controlFontOf(th, w.size), text.Regular)
	dis := w.disabled || w.n.disabled
	col := th.Text
	if dis {
		col = th.TextDisabled
	}
	l.Clip(r, geom.Radii{})
	defer l.Pop()
	if w.search && w.st.open {
		ph := w.label()
		if ph == "" {
			ph = w.placeholder
		}
		c.entPaintEdit(l, r, w.edID, &w.st.edit, w.st.query, st, col, th.TextQuaternary, ph, false)
		return
	}
	s, ccol := w.label(), col
	if s == "" {
		s, ccol = w.placeholder, th.TextQuaternary
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true, Ellipsis: true, MaxWidth: r.W})
	l.Glyphs(geom.Pt(c.inline(r).X(0, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, ccol)
}
