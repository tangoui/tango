package mobile

import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// List

// ListW is a group of cells with an optional header.
type ListW struct {
	c      *ui.Context
	th     *Theme
	header string
	card   bool
	opts   []layout.Option
	role   a11y.Role // of the cell group (RoleList; Form uses RoleForm)
}

// List starts a cell group titled header ("" for none); add cells in Body.
//
// ConfigProvider defaults ("mobile.List"): "mode" ("default" or "card").
func List(c *ui.Context, header string) *ListW {
	w := &ListW{c: c, th: ThemeOf(c), header: header}
	if v, ok := cfgStr(c, "List", "mode"); ok {
		w.card = v == "card"
	}
	return w
}

// Card insets the group with rounded corners (Ant Mobile mode="card",
// Vant inset).
func (w *ListW) Card() *ListW { w.card = true; return w }

// Layout applies layout options to the outer box.
func (w *ListW) Layout(opts ...layout.Option) *ListW { w.opts = append(w.opts, opts...); return w }

// Body builds the cells.
func (w *ListW) Body(fn func()) {
	c, th := w.c, w.th
	ui.Column(c, func() {
		if w.header != "" {
			fs := th.Font(th.FontMain, text.Regular)
			top, bottom := float32(8), float32(8)
			if th.Style == Vant {
				top, bottom = 16, 8
			} else if th.shad() {
				fs = th.Font(th.FontSmall, text.Medium)
				top, bottom = 16, 8
			}
			x := th.PadH
			if w.card {
				x += th.PadH
			}
			header := w.header
			rtl := c.IsRTL()
			ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: top + fs.LineHeight + bottom} }, func(l *paint.List, r geom.Rect) {
				spanOf(r, rtl).text(l, header, fs, r.X+x, r.Y+top, th.TextWeak)
			}).Semantics(a11y.RoleHeading, header)
		}
		body := ui.Div(c).Bg(th.Box)
		role := w.role
		if role == a11y.RoleNone {
			role = a11y.RoleList
		}
		body.Semantics(role, w.header)
		if w.card {
			body.Radius(th.RadiusLarge).Clip().Style(layout.Margin(geom.Insets{Left: th.PadH, Right: th.PadH}))
		}
		card := w.card
		rad := geom.Rad(th.RadiusLarge)
		body.PaintOver(func(l *paint.List, r geom.Rect) {
			// Hide the first cell's top separator; frame plain lists.
			if th.CardBorder.A > 0 && card {
				l.Fill(geom.Rect{X: r.X + rad.TL, Y: r.Y, W: r.W - 2*rad.TL, H: 1}, geom.Radii{}, th.Box)
			} else {
				l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Box)
			}
			switch {
			case th.CardBorder.A > 0 && card:
				l.Stroke(r, rad, 1, th.CardBorder) // shadcn: outlined card
			case !card && (th.Style == AntMobile || th.CardBorder.A > 0):
				hairlineTop(l, r, r.X, th.Border)
				hairline(l, r, r.X, th.Border)
			}
		})
		body.Body(fn)
	}, w.opts...)
}

// ---------------------------------------------------------------------------
// Cell

// CellW is a list row: optional icon, title with description, value at
// the end, arrow, or switch (the end is the right; the left in RTL, where
// the arrow points left).
type CellW struct {
	c        *ui.Context
	th       *Theme
	b        *ui.Box
	title    string
	desc     string
	extra    string
	ic       *icon.Icon
	arrow    bool
	click    bool
	sw       *bool
	swK      float32
	disabled bool
	required bool
	pr       Press
	changed  bool
	swRect   geom.Rect
	rtl      bool
}

// Cell adds a list row titled title.
//
// ConfigProvider defaults ("mobile.Cell"): "arrow", "clickable" (bool).
func Cell(c *ui.Context, title string) *CellW {
	th := ThemeOf(c)
	w := &CellW{c: c, th: th, title: title, rtl: c.IsRTL()}
	w.b = ui.Custom(c, w.measure, filled(th.Box, w.paint))
	w.pr = Touchable(c, w.b)
	w.b.Semantics(a11y.RoleListItem, title)
	if c.A11yEnabled() {
		w.b.A11y(w.semantics)
	}
	if v, ok := cfgBool(c, "Cell", "arrow"); ok && v {
		w.Arrow()
	}
	if v, ok := cfgBool(c, "Cell", "clickable"); ok && v {
		w.Clickable()
	}
	return w
}

// semantics completes the row: a button when tappable, a switch when it
// carries one; the description and the value on the right.
func (w *CellW) semantics(s *a11y.Node) {
	switch {
	case w.sw != nil:
		s.States |= a11y.Checkable
		if *w.sw {
			s.States |= a11y.Checked
		}
	case !w.click:
		// A plain row: not an activatable control.
		s.Actions &^= a11y.Press
	}
	if w.disabled {
		s.States |= a11y.Disabled
		s.Actions &^= a11y.Press
	}
	if w.required {
		s.States |= a11y.Required
	}
	s.Description = w.desc
	s.Value = w.extra
}

// Desc shows a description line under the title.
func (w *CellW) Desc(s string) *CellW { w.desc = s; return w }

// Extra shows a value at the end.
func (w *CellW) Extra(s string) *CellW { w.extra = s; return w }

// Icon shows an icon before the title.
func (w *CellW) Icon(ic *icon.Icon) *CellW { w.ic = ic; return w }

// Arrow shows the navigation arrow and pressed feedback.
func (w *CellW) Arrow() *CellW {
	w.arrow, w.click = true, true
	w.role(a11y.RoleButton)
	return w
}

// role sets the row's role (a switch row stays a switch).
func (w *CellW) role(r a11y.Role) {
	if w.sw == nil {
		w.b.Semantics(r, w.title)
	}
}

// Clickable shows pressed feedback without an arrow.
func (w *CellW) Clickable() *CellW { w.click = true; w.role(a11y.RoleButton); return w }

// Required marks the title with a red asterisk (forms).
func (w *CellW) Required() *CellW { w.required = true; return w }

// Disabled greys the row out and ignores taps.
func (w *CellW) Disabled(on bool) *CellW { w.disabled = on; return w }

// Switch shows a switch bound to *on; tapping it toggles.
func (w *CellW) Switch(on *bool) *CellW {
	w.sw = on
	w.b.Semantics(a11y.RoleSwitch, w.title)
	r, _ := w.c.RectOf(w.b.ID())
	sz := switchSize(w.th)
	zone := spanOf(r, w.rtl).R(geom.Rect{X: r.Right() - w.th.PadH - sz.W - 12, Y: r.Y, W: sz.W + 12 + w.th.PadH, H: r.H})
	// Enter / Space or a screen reader's Press toggle the switch (the row
	// is exposed as one switch).
	if w.pr.Tap && !w.disabled && (zone.Contains(w.pr.TapPos) || w.pr.KeyTap) {
		*on = !*on
		w.changed = true
		w.c.Invalidate()
	}
	w.swK = w.c.Anim(w.b.ID(), "switch", b2f(*on), 200*time.Millisecond)
	return w
}

// Clicked reports a tap on the row (not on its switch).
func (w *CellW) Clicked() bool { return w.pr.Tap && !w.disabled && !w.changed }

// LongPressed reports a long press on the row.
func (w *CellW) LongPressed() bool { return w.pr.LongPress && !w.disabled }

// Changed reports a switch toggle this frame.
func (w *CellW) Changed() bool { return w.changed }

// ID returns the row's ID.
func (w *CellW) ID() ui.ID { return w.b.ID() }

func (w *CellW) fonts() (title, desc, extra text.Style) {
	th := w.th
	return th.Font(th.FontBody, text.Regular), th.Font(th.FontSmall, text.Regular), th.Font(th.FontMain, text.Regular)
}

func (w *CellW) measure(maxW, _ float32) geom.Size {
	th := w.th
	tf, df, _ := w.fonts()
	h := tf.LineHeight
	if th.Style == Vant {
		h = 24
	}
	if w.desc != "" {
		h += df.LineHeight + 2
	}
	h += 2 * th.CellPadV
	h = max(h, th.TouchMin)
	if w.desc == "" {
		h = max(h, th.CellMinHeight)
	}
	return geom.Size{W: maxW, H: h}
}

func (w *CellW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	tf, df, ef := w.fonts()
	// Laid out left to right; sp mirrors the parts in RTL.
	sp := spanOf(r, w.rtl)
	if w.click && w.pr.Pressed && !w.disabled && !(w.sw != nil && sp.P(w.pr.PressPos.X) > r.Right()-th.PadH-80) {
		l.Fill(r, geom.Radii{}, th.Press)
	}
	x := r.X + th.PadH
	// Separator at the top, inset like the content (the List hides it on
	// the first row).
	sp.hairlineTop(l, r, x, th.Border)
	if w.disabled {
		l.Opacity(0.4)
	}
	if w.ic != nil {
		is := float32(20)
		col := th.Text
		if th.shad() {
			col = th.TextWeak // muted-foreground icons
		}
		sp.icon(l, w.ic, x, r.Y+(r.H-is)/2, is, col)
		x += is + 10
	}
	right := r.Right() - th.PadH
	if w.arrow {
		as := float32(18)
		if th.Style == Vant || th.shad() {
			as = 16
		}
		col := th.TextLight
		if th.Style == Vant || th.shad() {
			col = th.TextWeak
		}
		sp.icon(l, icon.Right, right-as+2, r.Y+(r.H-as)/2, as, col)
		right -= as + 4
	}
	if w.sw != nil {
		sz := switchSize(th)
		sr := geom.Rect{X: right - sz.W, Y: r.Y + (r.H-sz.H)/2, W: sz.W, H: sz.H}
		w.swRect = sp.R(sr)
		drawSwitchDir(l, th, w.swRect, *w.sw, w.swK, w.rtl)
		right = sr.X - 8
	}
	if w.extra != "" {
		right -= sp.content().textRight(l, w.extra, ef, right, r, th.TextWeak) + 8
	}
	// Title (+ description).
	th1 := tf.LineHeight
	if th.Style == Vant {
		th1 = 24
	}
	block := th1
	if w.desc != "" {
		block += df.LineHeight + 2
	}
	y := r.Y + (r.H-block)/2
	if w.required {
		x += sp.text(l, "*", tf, x-1, y+(th1-tf.LineHeight)/2, th.Danger) + 1
	}
	sp.ellipsis(l, w.title, tf, x, y+(th1-tf.LineHeight)/2, max(0, right-x), th.Text)
	if w.desc != "" {
		sp.ellipsis(l, w.desc, df, x, y+th1+2, max(0, right-x), th.TextWeak)
	}
	if w.disabled {
		l.Pop()
	}
}

// ---------------------------------------------------------------------------
// SwipeCell

// SwipeAction is a button revealed by swiping a SwipeCell.
type SwipeAction struct {
	Key   string
	Text  string
	Color string // "primary" | "success" | "warning" | "danger" | "light" | "" (light)
}

type swipeState struct {
	sc      Scroller
	gen     uint64
	pending int // Open/Close requested before the range was known
	hasPend bool
}

// SwipeCellW wraps content that can be swiped sideways to reveal actions.
//
// Sides are logical: the Right actions sit at the end of the row and are
// revealed by swiping towards the start (to the left; in RTL the actions
// sit on the left and a swipe to the right reveals them), the Left ones at
// the start.
type SwipeCellW struct {
	c            *ui.Context
	th           *Theme
	left, right  []SwipeAction
	st           *swipeState
	outer        *ui.Box
	actionSide   int
	actionIndex  int
	acted        bool
	closeOnClick bool
	rtl          bool
}

// SwipeCell starts a swipeable row; set actions, then build the content
// with Body (typically one Cell).
//
// ConfigProvider defaults ("mobile.SwipeCell"): "closeOnAction" (bool).
func SwipeCell(c *ui.Context) *SwipeCellW {
	w := &SwipeCellW{c: c, th: ThemeOf(c), actionIndex: -1, closeOnClick: true, rtl: c.IsRTL()}
	if v, ok := cfgBool(c, "SwipeCell", "closeOnAction"); ok {
		w.closeOnClick = v
	}
	w.outer = ui.Div(c).Clip()
	w.st = ui.State[swipeState](c, w.outer.ID(), nil)
	return w
}

// Right sets the actions at the end, revealed by swiping left (right in
// RTL).
func (w *SwipeCellW) Right(a ...SwipeAction) *SwipeCellW { w.right = a; return w }

// Left sets the actions at the start, revealed by swiping right (left in
// RTL).
func (w *SwipeCellW) Left(a ...SwipeAction) *SwipeCellW { w.left = a; return w }

// Open opens the Right (end, side > 0) or Left (start, side < 0) actions.
func (w *SwipeCellW) Open(side int) *SwipeCellW {
	w.st.pending, w.st.hasPend = side, true
	w.c.Invalidate()
	return w
}

// Close closes the cell.
func (w *SwipeCellW) Close() *SwipeCellW { return w.Open(0) }

// IsOpen reports whether actions are (being) shown.
func (w *SwipeCellW) IsOpen() bool { return w.st.sc.Pos != 0 || w.st.sc.Mode == Dragging }

func (w *SwipeCellW) target(side int, now time.Time) {
	sc := &w.st.sc
	t := 0.0
	if side > 0 {
		t = sc.Max
	} else if side < 0 {
		t = sc.Min
	}
	sc.SnapTo(t, now)
	w.st.gen++
	w.c.Invalidate()
}

func (w *SwipeCellW) actionWidth(a SwipeAction) float32 {
	th := w.th
	if th.Style == Vant {
		return max(65, textW(a.Text, th.Font(14, text.Regular))+24)
	}
	return textW(a.Text, th.Font(th.FontMain, text.Regular)) + 24
}

func (w *SwipeCellW) sideWidth(as []SwipeAction) float32 {
	var s float32
	for _, a := range as {
		s += w.actionWidth(a)
	}
	return s
}

// Body builds the row content.
func (w *SwipeCellW) Body(fn func()) *SwipeCellW {
	c, th, st := w.c, w.th, w.st
	sc := &st.sc
	now := c.Now()
	lw, rw := w.sideWidth(w.left), w.sideWidth(w.right)
	sc.Min, sc.Max = -float64(lw), float64(rw)
	sc.NoBounce = false
	if r, ok := c.RectOf(w.outer.ID()); ok {
		sc.Dim = float64(r.W)
	}
	// The scroller works in logical units: Pos > 0 shows the end actions.
	// In RTL a physical swipe to the right is a logical swipe to the left.
	sgn := float32(1)
	if w.rtl {
		sgn = -1
	}
	accept := func(d Direction) bool {
		if w.rtl {
			switch d {
			case DirLeft:
				d = DirRight
			case DirRight:
				d = DirLeft
			}
		}
		switch d {
		case DirLeft:
			return rw > 0 || sc.Pos < 0
		case DirRight:
			return lw > 0 || sc.Pos > 0
		}
		return false
	}
	if st.hasPend {
		st.hasPend = false
		w.target(st.pending, now)
	}
	g := Area(c, w.outer, AxisHorizontal, accept)
	if g.Pan.Began {
		sc.BeginDrag()
	}
	if g.Pan.Active {
		sc.DragBy(-float64(sgn * g.Pan.Delta.X))
	}
	if g.Pan.Ended {
		v := -float64(sgn * g.Pan.Velocity.X)
		p := sc.Pos
		side := 0
		switch {
		case v > SwipeMinVelocity && rw > 0 && p > 0:
			side = 1
		case v < -SwipeMinVelocity && lw > 0 && p < 0:
			side = -1
		case p > float64(rw)/2 && rw > 0:
			side = 1
		case p < -float64(lw)/2 && lw > 0:
			side = -1
		}
		if (v < -SwipeMinVelocity && p > 0) || (v > SwipeMinVelocity && p < 0) {
			side = 0
		}
		w.target(side, now)
	}
	open := sc.Pos != 0 && sc.Mode != Dragging
	if open && g.Outside {
		w.target(0, now)
	}
	if sc.Active() {
		if pump(c, w.outer.ID(), st.gen) {
			sc.Step(now)
		} else {
			sc.Settle()
		}
	}
	pos := float32(sc.Pos)
	Within(c, w.outer.ID(), func() {
		w.outer.Body(func() {
			// The action rows are placed at the logical end / start (Abs
			// right / left), which the layout mirrors in RTL.
			ui.Div(c).Transform(geom.Translate(-sgn*pos, 0)).Body(func() {
				fn()
				if rw > 0 {
					w.actions(w.right, 1, layout.Abs(layout.Unset, 0, -rw, 0), rw)
				}
				if lw > 0 {
					w.actions(w.left, -1, layout.Abs(-lw, 0, layout.Unset, 0), lw)
				}
			})
		})
	})
	// A tap on the content of an open cell closes it.
	if open && g.Tap && !w.acted {
		w.target(0, now)
	}
	_ = th
	return w
}

func (w *SwipeCellW) actions(as []SwipeAction, side int, pos layout.Option, width float32) {
	c, th := w.c, w.th
	ui.Row(c, func() {
		for i, a := range as {
			i, a := i, a
			ui.Key(c, a.Key+a.Text, func() {
				aw := w.actionWidth(a)
				b := ui.Div(c, layout.W(aw))
				// Revealed by a swipe (pointer only); screen readers press
				// the buttons directly.
				b.Semantics(a11y.RoleButton, a.Text)
				pr := Touchable(c, b)
				bg := th.ColorOf(a.Color)
				fg := th.onColor(a.Color)
				if a.Color == "" || a.Color == "light" {
					bg, fg = th.Fill, th.TextSecondary
					if th.Style == AntMobile && !th.Dark {
						bg = theme.Hex("#cccccc")
						fg = th.OnPrimary
					}
				}
				st := th.Font(th.FontMain, text.Regular)
				if th.Style == Vant {
					st = th.Font(14, text.Regular)
				}
				b.Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(r, geom.Radii{}, bg)
					if pr.Pressed {
						l.Fill(r, geom.Radii{}, theme.RGBA(0, 0, 0, .1))
					}
					drawTextCenter(l, a.Text, st, r, fg)
				})
				if pr.Tap {
					w.acted, w.actionSide, w.actionIndex = true, side, i
					if w.closeOnClick {
						w.target(0, c.Now())
					}
				}
			})
		}
	}, pos, layout.W(width))
}

// Action reports a tapped action this frame: side (1 Right / end, -1
// Left / start),
// index into that side's actions.
func (w *SwipeCellW) Action() (side, index int, ok bool) {
	return w.actionSide, w.actionIndex, w.acted
}

// ActionKey returns the Key (or Text) of the tapped action, "" if none.
func (w *SwipeCellW) ActionKey() string {
	if !w.acted {
		return ""
	}
	as := w.right
	if w.actionSide < 0 {
		as = w.left
	}
	return or(as[w.actionIndex].Key, as[w.actionIndex].Text)
}

// KeepOpen keeps the cell open after an action tap.
func (w *SwipeCellW) KeepOpen() *SwipeCellW { w.closeOnClick = false; return w }
