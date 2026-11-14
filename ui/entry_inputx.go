package ui

import (
	"strconv"
	"unicode/utf8"

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
// Search

// SearchW is an input with a search button (Ant Input.Search, Element input
// with an append search button).
type SearchW struct {
	_ [0]func()

	c           *Context
	n           *node
	val         *string
	placeholder string
	enter       string
	loading     bool
	allowClear  bool
	disabled    bool
	status      string
	size        ControlSize
	searched    bool
	changed     bool
	z           *lazy
	inputID     ID
	name        string
}

// Search adds a search box bound to *val.
func Search(c *Context, val *string) *SearchW {
	w := &SearchW{c: c, val: val, status: c.itemStatus, size: c.defaultSize()}
	w.n = c.add("search", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.Align = layout.AlignStretch
	w.n.ln.Width = 260
	w.configDefaults()
	w.z = c.lazy(w.n, w.build)
	return w
}

func (w *SearchW) Placeholder(s string) *SearchW { w.placeholder = s; return w }

// A11yLabel sets the accessible name of the search box (inside a FormItem
// the item label is used; the placeholder is the last resort).
func (w *SearchW) A11yLabel(s string) *SearchW { w.name = s; return w }

// EnterButton replaces the icon button with a primary button labelled s.
func (w *SearchW) EnterButton(s string) *SearchW { w.enter = s; return w }

// Loading shows a spinner in the button.
func (w *SearchW) Loading(on bool) *SearchW    { w.loading = on; return w }
func (w *SearchW) AllowClear() *SearchW        { w.allowClear = true; return w }
func (w *SearchW) Status(s string) *SearchW    { w.status = s; return w }
func (w *SearchW) Size(s ControlSize) *SearchW { w.size = s; return w }
func (w *SearchW) Width(px float32) *SearchW   { w.n.ln.Width = px; return w }
func (w *SearchW) Layout(o ...layout.Option) *SearchW {
	flexbox.Apply(&w.n.ln.Style, o...)
	return w
}
func (w *SearchW) Disabled(on bool) *SearchW {
	if on {
		w.disabled = true
	}
	return w
}

// Searched reports a search request (button click or Enter).
func (w *SearchW) Searched() bool { w.z.run(); return w.searched }

// Changed reports an edit of the text.
func (w *SearchW) Changed() bool { w.z.run(); return w.changed }

// InputID returns the ID of the inner editor (focus it with c.SetFocus).
func (w *SearchW) InputID() ID { w.z.run(); return w.inputID }

func (w *SearchW) build() {
	c := w.c
	dis := w.disabled || w.n.disabled
	in := Input(c, w.val).Placeholder(w.placeholder).Size(w.size).Status(w.status).Disabled(dis).Layout(layout.Grow(1), layout.MinW(0))
	if w.allowClear {
		in.AllowClear()
	}
	w.inputID = in.ID()
	in.ed.setRole(a11y.RoleSearchbox)
	if w.name != "" {
		in.A11yLabel(w.name)
	}
	if c.a11yOn {
		search := c.Locale().Search
		in.ed.fill(semFunc(func(s *a11y.Node) {
			if s.Name == "" {
				s.Name = search
			}
		}))
	}
	if in.Changed() {
		w.changed = true
	}
	if in.Submitted() && !w.loading {
		w.searched = true
	}
	switch {
	case w.enter != "":
		b := Button(c, w.enter).Primary().Size(w.size).Loading(w.loading).Disabled(dis)
		if b.Clicked() {
			w.searched = true
		}
	case c.Theme.Flavor == theme.Element:
		a := Addon(c, "").Icon(icon.Search).Size(w.size).Disabled(dis).A11yLabel(c.Locale().Search)
		if w.loading {
			a.Icon(icon.Loading)
			a.n.paint = ixSpinAddon(c, a)
		}
		if a.Clicked() && !w.loading {
			w.searched = true
		}
	default:
		if ixSearchButton(c, w.size, w.loading, dis) {
			w.searched = true
		}
	}
	cmpJoin(c, w.n)
}

// ixSpinAddon paints an addon whose icon spins (loading).
func ixSpinAddon(c *Context, a *AddonW) PaintFunc {
	base := a.paint
	return func(l *paint.List, r geom.Rect) {
		th := c.Theme
		saved := a.ic
		a.ic = nil
		base(l, r)
		a.ic = saved
		fs := c.controlFont(a.size)
		icon.Draw(l, icon.Loading, geom.Rect{X: r.X + (r.W-fs)/2, Y: r.Y + (r.H-fs)/2, W: fs, H: fs}, th.TextTertiary, spinAngle(c))
	}
}

// ixSearchButton is Ant's square search button: a default button whose
// icon uses the description color and turns primary on hover.
func ixSearchButton(c *Context, size ControlSize, loading, disabled bool) bool {
	n := c.add("search-button", c.Locale().Search)
	n.setRole(a11y.RoleButton).stateIf(loading, a11y.Busy)
	h := c.controlHeight(size)
	n.ln.Width, n.ln.Height = h, h
	if disabled {
		n.disabled = true
	}
	cur := input.CursorPointer
	if disabled {
		cur = input.CursorNotAllowed
	}
	ia := c.interact(n, true, cur)
	n.paint = func(l *paint.List, r geom.Rect) {
		th := c.Theme
		rad := geom.Rad(c.controlRadius(size))
		hv := c.Anim(n.id, "hover", b2f(ia.Hovered && !disabled), th.DurationMid)
		border := theme.Lerp(th.Border, th.Primary.Hover, hv)
		fg := theme.Lerp(th.TextTertiary, th.Primary.Hover, hv)
		bg := th.BgContainer
		if ia.Pressed {
			border, fg = th.Primary.Active, th.Primary.Active
		}
		if disabled {
			bg, border, fg = th.BgContainerDisabled, th.Border, th.TextDisabled
		}
		btk := c.ThemeFor(theme.CompButton).Components.Button
		if th.Flavor == theme.StyleShadcn {
			// shadcn's outline icon button, as Button's shadcn look: the
			// border stays, hover is bg-accent, disabled is opacity-50
			// (its primary is near-black, so Ant's primary-tinted hover
			// would read as a pressed dark border).
			if disabled {
				hv = 0
				l.Opacity(.5)
				defer l.Pop()
			}
			border = btk.DefaultBorderColor
			fg = theme.Lerp(btk.DefaultColor, btk.DefaultHoverColor, hv)
			bg = theme.Lerp(btk.DefaultBg, btk.DefaultHoverBg, hv)
		}
		l.Shadows(r, rad, btk.DefaultShadow)
		l.Fill(r, rad, th.BgContainer)
		l.Fill(r, rad, bg)
		l.Stroke(r, rad, 1, border)
		is := c.controlFont(size)
		ir := geom.Rect{X: r.X + (r.W-is)/2, Y: r.Y + (r.H-is)/2, W: is, H: is}
		if loading {
			icon.Draw(l, icon.Loading, ir, fg, spinAngle(c))
		} else {
			icon.Draw(l, icon.Search, ir, fg, 0)
		}
		if ia.FocusVisible {
			entFocusRing(c, l, r, rad.TL)
		}
	}
	return ia.Clicked && !disabled && !loading
}

// ---------------------------------------------------------------------------
// TextArea

// TextAreaW is a multi-line input with character count, auto-size, clear
// button and wheel scrolling (Ant Input.TextArea, Element type=textarea).
type TextAreaW struct {
	_ [0]func()

	c           *Context
	n, box, ed  *node
	sb          *ScrollBox
	val         *string
	st          *ixTAState
	placeholder string
	rows        int
	minR, maxR  int
	auto        bool
	status      string
	maxLen      int
	showCount   bool
	exceed      bool
	allowClear  bool
	disabled    bool
	bare        bool // no frame, fills its parent (MarkdownEditor)
	res         editResult
	cleared     bool
	done        bool
	boxIA, edIA Interaction
}

type ixTAState struct {
	edit      editState
	lastCur   int
	lastLen   int
	clearRect geom.Rect
}

const ixLineH = 22

// TextArea adds a multi-line input bound to *val.
func TextArea(c *Context, val *string) *TextAreaW {
	w := &TextAreaW{c: c, val: val, rows: 3, status: c.itemStatus}
	w.n = c.add("textarea-x", "")
	w.n.ln.Dir = layout.Column
	w.st = State[ixTAState](c, w.n.id, func() ixTAState { return ixTAState{lastCur: -1} })
	c.body(w.n, func() {
		w.box = c.add("textarea-box", "")
		w.box.ln.Dir = layout.Column
		c.body(w.box, func() {
			w.sb = Scroll(c, func() {
				w.ed = c.add("textarea-editor", "")
				w.ed.ln.Measure = w.measureText
			})
		})
	})
	w.box.paint = w.paintBox
	w.box.over = w.paintOver
	w.ed.paint = w.paintText
	w.ed.ownKeys = true
	w.ed.setRole(a11y.RoleTextbox).setState(a11y.Multiline).fill(w)
	w.edIA = c.interact(w.ed, true, input.CursorText)
	w.boxIA = c.interact(w.box, false, input.CursorText)
	w.configDefaults()
	c.deferred = append(c.deferred, w.finish)
	return w
}

func (w *TextAreaW) Placeholder(s string) *TextAreaW { w.placeholder = s; return w }

// A11yLabel sets the accessible name. Inside a FormItem the item label is
// used; otherwise the placeholder is the last resort.
func (w *TextAreaW) A11yLabel(s string) *TextAreaW { w.ed.setName(s); return w }

func (w *TextAreaW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = *w.val, w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	if w.showCount {
		s.Description = w.countText()
	}
	if w.status == "error" || w.over() {
		s.States |= a11y.Invalid
	}
}

// Rows sets the fixed number of visible rows (default 3).
func (w *TextAreaW) Rows(n int) *TextAreaW { w.rows = max(1, n); return w }

// AutoSize grows the box with its content between minRows and maxRows
// (maxRows <= 0: unbounded); beyond maxRows the content scrolls.
func (w *TextAreaW) AutoSize(minRows, maxRows int) *TextAreaW {
	w.auto, w.minR, w.maxR = true, max(1, minRows), maxRows
	return w
}
func (w *TextAreaW) Status(s string) *TextAreaW { w.status = s; return w }

// MaxLength limits the number of characters (see Exceed).
func (w *TextAreaW) MaxLength(n int) *TextAreaW { w.maxLen = n; return w }

// ShowCount shows "count / max".
func (w *TextAreaW) ShowCount() *TextAreaW { w.showCount = true; return w }

// Exceed lets the text grow beyond MaxLength and marks it as an error
// instead of truncating.
func (w *TextAreaW) Exceed() *TextAreaW     { w.exceed = true; return w }
func (w *TextAreaW) AllowClear() *TextAreaW { w.allowClear = true; return w }
func (w *TextAreaW) Width(px float32) *TextAreaW {
	w.n.ln.Width = px
	return w
}
func (w *TextAreaW) Layout(o ...layout.Option) *TextAreaW {
	flexbox.Apply(&w.n.ln.Style, o...)
	return w
}
func (w *TextAreaW) Disabled(on bool) *TextAreaW {
	if on {
		w.disabled = true
		w.box.disabled, w.ed.disabled, w.ed.focus = true, true, false
		w.ed.cursor, w.box.cursor = input.CursorNotAllowed, input.CursorNotAllowed
	}
	return w
}

// Changed reports an edit this frame.
func (w *TextAreaW) Changed() bool { w.finish(); return w.res.changed || w.cleared }

// Focused reports keyboard focus.
func (w *TextAreaW) Focused() bool { return w.c.focus == w.ed.id }

// ID returns the editor ID.
func (w *TextAreaW) ID() ID { return w.ed.id }

func (w *TextAreaW) font() text.Style { return w.c.style(w.c.Theme.FontSize, text.Regular) }

func (w *TextAreaW) measureText(maxW, _ float32) geom.Size {
	if geom.IsInf(maxW) {
		maxW = 200
	}
	h := text.Shape(*w.val, w.font(), text.Options{MaxWidth: maxW}).Height
	return geom.Size{W: maxW, H: max(ixLineH, h)}
}

func (w *TextAreaW) count() int { return utf8.RuneCountInString(*w.val) }

func (w *TextAreaW) over() bool { return w.maxLen > 0 && w.count() > w.maxLen }

func (w *TextAreaW) padding() geom.Insets {
	in := geom.VH(4, 11)
	if w.c.Theme.Flavor == theme.Element {
		in = geom.VH(4, 11)
	}
	if w.allowClear {
		in.Right += 20
	}
	if w.showCount && w.c.Theme.Flavor == theme.Element {
		in.Bottom += 14
	}
	return in
}

func (w *TextAreaW) visibleRows() int {
	if !w.auto {
		return w.rows
	}
	wid := float32(200)
	if r, ok := w.c.rects[w.ed.id]; ok && r.W > 0 {
		wid = r.W
	}
	h := text.Shape(*w.val, w.font(), text.Options{MaxWidth: wid}).Height
	n := int((max(h, ixLineH) + ixLineH/2) / ixLineH)
	n = max(n, w.minR)
	if w.maxR > 0 {
		n = min(n, w.maxR)
	}
	return n
}

func (w *TextAreaW) finish() {
	if w.done {
		return
	}
	w.done = true
	c := w.c
	in := w.padding()
	w.box.ln.Padding = in
	view := float32(w.visibleRows() * ixLineH)
	if w.bare {
		// Fill the parent: the scroll view takes whatever height is left.
		w.box.ln.Padding = geom.VH(5, 12)
		w.n.ln.Grow, w.box.ln.Grow, w.sb.n.ln.Grow = 1, 1, 1
		w.n.ln.MinH, w.box.ln.MinH, w.sb.n.ln.MinH = 0, 0, 0
		if v := w.sb.s.view.H; v > 0 {
			view = v
		}
		w.ed.ln.MinH = view
	} else {
		w.ed.ln.MinH = view
		w.sb.n.ln.Height = view
		w.box.ln.Height = view + in.Vertical()
	}
	if w.showCount && c.Theme.Flavor != theme.Element {
		cnt := &node{id: hashID(w.n.id, "count"), parent: w.n}
		cnt.ln.Ref = cnt
		cnt.ln.AlignSelf = layout.AlignEnd
		cnt.ln.NoStretch = true
		cnt.ln.Measure = func(_, _ float32) geom.Size {
			return geom.Size{W: entTextW(c, w.countText(), c.Theme.FontSize, text.Regular), H: ixLineH}
		}
		cnt.paint = func(l *paint.List, r geom.Rect) {
			col := c.Theme.TextTertiary
			if w.over() {
				col = c.Theme.Error.Base
			}
			entText(c, l, w.countText(), r.X, r, c.Theme.FontSize, text.Regular, col, 0)
		}
		w.n.kids = append(w.n.kids, cnt)
	}
	if w.box.disabled {
		return
	}
	if w.allowClear && w.boxIA.Clicked && *w.val != "" && w.st.clearRect.Expand(3).Contains(c.pointer) {
		*w.val = ""
		w.st.edit.cur, w.st.edit.anc = 0, 0
		w.cleared = true
		c.SetFocus(w.ed.id)
		c.invalid = true
	}
	// A press on the padding or on the empty scroll area below the text
	// (not on a scrollbar) focuses the editor.
	sbPress := c.pressed[w.sb.n.id^scrollTag] && !w.sb.s.dragX && !w.sb.s.dragY
	if (w.boxIA.PressedNow || sbPress) && !w.edIA.PressedNow {
		// A press on the padding: caret at the end, not select-all.
		n := utf8.RuneCountInString(*w.val)
		w.st.edit.cur, w.st.edit.anc, w.st.edit.wasFocused = n, n, true
		c.SetFocus(w.ed.id)
	}
	ml := w.maxLen
	if w.exceed {
		ml = 0
	}
	e := &w.st.edit
	w.res = c.processEdit(w.ed.id, e, w.val, editConfig{multiline: true, maxLen: ml}, w.edIA)
	// Keep the caret visible inside the scroll view when it moved.
	n := len(*w.val)
	if c.focus == w.ed.id && (e.cur != w.st.lastCur || n != w.st.lastLen) {
		wid := float32(200)
		if r, ok := c.rects[w.ed.id]; ok && r.W > 0 {
			wid = r.W
		}
		lay := text.Shape(*w.val, w.font(), text.Options{MaxWidth: wid})
		_, top, h := lay.Caret(clampIdx(e.cur, utf8.RuneCountInString(*w.val)))
		s := w.sb.s
		y, old := s.off.Y, s.off.Y
		if float64(top) < y {
			y = float64(top)
		}
		if float64(top+h) > y+float64(view) {
			y = float64(top + h - view)
		}
		if y != old {
			s.off.Y = max(0, y)
			c.invalid = true
		}
	}
	w.st.lastCur, w.st.lastLen = e.cur, n
}

func (w *TextAreaW) countText() string {
	s := strconv.Itoa(w.count())
	if w.maxLen > 0 {
		s += " / " + strconv.Itoa(w.maxLen)
	}
	return s
}

func (w *TextAreaW) paintBox(l *paint.List, r geom.Rect) {
	c := w.c
	if w.bare {
		l.Fill(r, geom.Radii{}, c.Theme.BgContainer)
		return
	}
	status := w.status
	if w.over() && status == "" {
		status = "error"
	}
	hover := w.boxIA.Hovered || w.edIA.Hovered
	c.paintFrame(l, r, geom.Rad(c.Theme.Radius), w.box.id, frameState{hover: hover, focus: c.focus == w.ed.id, disabled: w.box.disabled, status: status})
}

func (w *TextAreaW) paintOver(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.Theme
	if w.allowClear {
		is := float32(12)
		w.st.clearRect = c.inline(r).Rect(r.W-11-is, r.Y+4+(ixLineH-is)/2, is, is) // at the inline end
		hover := w.boxIA.Hovered || w.edIA.Hovered
		if *w.val != "" && (hover || c.focus == w.ed.id) && !w.box.disabled {
			entClearIcon(c, l, w.st.clearRect)
		}
	}
	if w.showCount && th.Flavor == theme.Element {
		s := w.countText()
		fs := th.FontSizeSM
		tw := entTextW(c, s, fs, text.Regular)
		cr := c.inline(r).Rect(r.W-10-tw, r.Bottom()-5-14, tw, 14) // bottom inline end
		bg := th.BgContainer
		if w.box.disabled {
			bg = th.BgContainerDisabled
		}
		l.Fill(cr, geom.Radii{}, bg)
		col := th.TextTertiary
		if w.over() {
			col = th.Error.Base
		}
		entText(c, l, s, cr.X, cr, fs, text.Regular, col, 0)
	}
}

func (w *TextAreaW) paintText(l *paint.List, r geom.Rect) {
	th := w.c.Theme
	col := th.Text
	if w.box.disabled {
		col = th.TextDisabled
	}
	w.c.paintEditor(l, r, w.ed.id, &w.st.edit, *w.val, w.font(), col, w.placeholder, false, true)
}
