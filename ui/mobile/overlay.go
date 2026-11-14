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
// Popup

// Position places a popup. PosLeft and PosRight are physical sides;
// PosStart and PosEnd are logical: the left and right edge left to right,
// the right and left edge in RTL.
type Position uint8

const (
	PosBottom Position = iota
	PosTop
	PosCenter
	PosLeft
	PosRight
	PosStart
	PosEnd
)

// resolve maps the logical positions to the physical side for the
// direction.
func (p Position) resolve(rtl bool) Position {
	switch {
	case p == PosStart && !rtl, p == PosEnd && rtl:
		return PosLeft
	case p == PosStart, p == PosEnd:
		return PosRight
	}
	return p
}

type popupState struct {
	size geom.Size // panel size of the last frame
	drag float32   // drawer drag offset (shadcn handle), target of the "drag" animation
	held bool      // the handle is being dragged
}

// PopupW is a sliding panel over a dimmed mask.
type PopupW struct {
	c           *ui.Context
	th          *Theme
	open        *bool
	pos         Position
	round       bool
	closeOnMask bool
	mask        bool
	closed      bool
	a           *ui.Box
	panel       *ui.Box
	opts        []layout.Option
	bg          paintColor
	bgSet       bool
	label       string
	role        a11y.Role // of the panel: RoleDialog (Dialog: RoleAlertDialog)
	desc        string
	handle      bool // drawer grab handle (shadcn bottom sheets)
	bare        bool // no panel chrome: the content draws its own cards
	ownBottom   bool // the content ends with bottomInset (ActionSheet, Picker)
}

// bottomInset adds the blank a bottom sheet keeps under its content: the
// safe area, or the soft keyboard when it is taller.
func bottomInset(c *ui.Context) *ui.Box {
	h := max(SafeAreaInsets(c).Bottom, c.KeyboardInset().Bottom)
	return ui.Div(c).Style(layout.H(h))
}

// Popup creates a popup bound to *open (bottom sheet by default); set
// options, then build the content with Body. Tapping the mask or pressing
// Escape closes it. In the shadcn style a bottom popup is a Drawer: an
// outlined sheet with rounded top corners and a grab handle that can be
// dragged down to close it.
//
// ConfigProvider defaults ("mobile.Popup"): "position" ("bottom", "top",
// "center", "left", "right", "start", "end"), "round", "mask", "closeOnMaskClick",
// "handle" (bool).
func Popup(c *ui.Context, open *bool) *PopupW {
	w := newPopup(c, open)
	if v, ok := cfgPosition(c, "Popup"); ok {
		w.pos = v
	}
	if v, ok := cfgBool(c, "Popup", "round"); ok {
		w.round = v
	}
	if v, ok := cfgBool(c, "Popup", "mask"); ok {
		w.mask = v
	}
	if v, ok := cfgBool(c, "Popup", "closeOnMaskClick"); ok {
		w.closeOnMask = v
	}
	if v, ok := cfgBool(c, "Popup", "handle"); ok {
		w.handle = v
	}
	return w
}

// newPopup is Popup without the "mobile.Popup" defaults (the popups of
// ActionSheet, Dialog and Picker read their own component's defaults).
func newPopup(c *ui.Context, open *bool) *PopupW {
	th := ThemeOf(c)
	return &PopupW{c: c, th: th, open: open, closeOnMask: true, mask: true, round: true, handle: th.shad(), a: anchor(c)}
}

// Handle toggles the drawer grab handle of a bottom popup (shadcn default
// on): a bar on top of the sheet; dragging it down closes the popup.
func (w *PopupW) Handle(on bool) *PopupW { w.handle = on; return w }

// Position sets where the panel appears.
func (w *PopupW) Position(p Position) *PopupW { w.pos = p; return w }

// Round toggles rounded corners on the inner edge.
func (w *PopupW) Round(on bool) *PopupW { w.round = on; return w }

// CloseOnMask toggles closing on a mask tap (default on).
func (w *PopupW) CloseOnMask(on bool) *PopupW { w.closeOnMask = on; return w }

// Mask toggles the dimmed backdrop (default on).
func (w *PopupW) Mask(on bool) *PopupW { w.mask = on; return w }

// Bg sets the panel background.
func (w *PopupW) Bg(col paintColor) *PopupW { w.bg, w.bgSet = col, true; return w }

// Layout applies layout options to the panel.
func (w *PopupW) Layout(opts ...layout.Option) *PopupW { w.opts = append(w.opts, opts...); return w }

// A11yLabel sets the accessible name of the panel (a dialog for screen
// readers; name it after its content).
func (w *PopupW) A11yLabel(s string) *PopupW { w.label = s; return w }

// Closed reports that the user dismissed the popup this frame.
func (w *PopupW) Closed() bool { return w.closed }

// Body builds the panel content (only while open or animating out).
func (w *PopupW) Body(fn func()) *PopupW {
	c, th := w.c, w.th
	id := w.a.ID()
	st := ui.State[popupState](c, id, nil)
	v := motion(c, id, *w.open, 300*time.Millisecond)
	if v <= 0.001 && !*w.open {
		st.drag, st.held = 0, false
		return w
	}
	// The physical side: the screen layer mirrors its row in RTL, so a
	// side panel sits at the row's end when its side is the right in LTR
	// or the left in RTL.
	rtl := c.IsRTL()
	pos := w.pos.resolve(rtl)
	var rootOpts []layout.Option
	switch pos {
	case PosBottom:
		rootOpts = []layout.Option{layout.End()}
	case PosCenter:
		rootOpts = []layout.Option{layout.Center()}
	case PosLeft, PosRight:
		rootOpts = []layout.Option{layout.Dir(layout.Row)}
		if (pos == PosRight) != rtl {
			rootOpts = append(rootOpts, layout.End())
		}
	}
	drawer := w.handle && pos == PosBottom && !w.bare
	var root *ui.Box
	root = screenLayer(c, func() {
		p := ui.Div(c, w.opts...)
		w.panel = p
		bg := th.Box
		if w.bgSet {
			bg = w.bg
		}
		p.Bg(bg).Clip()
		var rr geom.Radii
		if w.round {
			r := th.PopupRad
			switch pos {
			case PosBottom:
				rr = geom.Radii{TL: r, TR: r}
			case PosTop:
				rr = geom.Radii{BL: r, BR: r}
			case PosCenter:
				rr = geom.Rad(r)
			}
			p.Radii(rr)
		}
		if bc := th.CardBorder; bc.A > 0 && !w.bare {
			// shadcn: outlined sheets (the edges at the screen border are
			// pushed off screen) and a shadow-lg dialog.
			p.PaintOver(func(l *paint.List, r geom.Rect) {
				switch pos {
				case PosBottom:
					r = geom.Rect{X: r.X - 1, Y: r.Y, W: r.W + 2, H: r.H + 2}
				case PosTop:
					r = geom.Rect{X: r.X - 1, Y: r.Y - 2, W: r.W + 2, H: r.H + 2}
				case PosLeft:
					r = geom.Rect{X: r.X - 2, Y: r.Y - 1, W: r.W + 2, H: r.H + 2}
				case PosRight:
					r = geom.Rect{X: r.X, Y: r.Y - 1, W: r.W + 2, H: r.H + 2}
				}
				l.Stroke(r, rr, 1, bc)
			})
			if pos == PosCenter {
				p.Shadow(th.Base.ShadowSecondary)
			}
		}
		sink(c, p)
		// A dialog for screen readers; with a mask it is modal (Tab stays
		// inside, the page below is hidden) and Dismiss closes it.
		role := w.role
		if role == a11y.RoleNone {
			role = a11y.RoleDialog
		}
		p.Semantics(role, w.label).A11yState(stateIf(w.mask, a11y.Modal))
		if c.A11yEnabled() {
			desc := w.desc
			p.A11y(func(s *a11y.Node) {
				s.Actions |= a11y.Dismiss
				if s.Description == "" {
					s.Description = desc
				}
			})
		}
		p.Body(func() {
			if drawer {
				w.grabHandle(st)
			}
			fn()
			if pos == PosBottom && !w.ownBottom {
				// Lift the sheet's content above the soft keyboard.
				if kb := c.KeyboardInset().Bottom; kb > 0 {
					ui.Div(c).Style(layout.H(kb))
				}
			}
		})
		// Placement (after the content: a handle drag of this frame moves
		// the sheet right away).
		if pos == PosCenter {
			p.Opacity(v).Scale(0.9 + 0.1*v)
		} else {
			k := 1 - v
			sz := st.size
			if sz.W == 0 {
				sz = geom.Size{W: 400, H: 400}
			}
			var m geom.Affine
			switch pos {
			case PosBottom:
				dur := 250 * time.Millisecond
				if st.held {
					dur = 0
				}
				m = geom.Translate(0, max(sz.H*k, c.Anim(id, "drag", st.drag, dur)))
			case PosTop:
				m = geom.Translate(0, -sz.H*k)
			case PosLeft:
				m = geom.Translate(-sz.W*k, 0)
			case PosRight:
				m = geom.Translate(sz.W*k, 0)
			}
			p.Transform(m)
		}
	}, rootOpts...)
	if w.mask && *w.open {
		c.TrapFocus(root.ID())
	}
	if w.mask {
		root.Bg(fadeCol(th.Mask, float64(v)))
	}
	if r, ok := c.RectOf(w.panel.ID()); ok && *w.open && v >= 1 && !st.held && st.drag == 0 {
		st.size = r.Size()
	}
	var pr Press
	detached(c, func() { pr = touchAs(c, root, touchPassive) })
	if *w.open && ((w.closeOnMask && pr.Tap) || c.EscapeAt(root.ID())) {
		w.dismiss()
	}
	return w
}

func (w *PopupW) dismiss() {
	*w.open = false
	w.closed = true
	w.c.Invalidate()
}

// grabHandle builds the drawer handle: a muted bar on top of the sheet.
// Dragging it moves the sheet down; releasing past a quarter of its
// height (or flicking down) closes the popup, else it springs back.
func (w *PopupW) grabHandle(st *popupState) {
	c, th := w.c, w.th
	hb := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 28} }, func(l *paint.List, r geom.Rect) {
		hw, hh := float32(48), float32(5)
		l.Fill(geom.Rect{X: r.X + (r.W-hw)/2, Y: r.Y + 12, W: hw, H: hh}, geom.Rad(hh/2), th.DrawerHandle)
	})
	// Decorative for screen readers: the dialog offers Dismiss.
	hb.A11yHidden()
	g := Area(c, hb, AxisVertical, nil)
	switch {
	case g.Pan.Active:
		st.held = true
		st.drag = max(0, g.Pan.Delta.Y)
		c.Invalidate()
	case g.Pan.Ended:
		st.held = false
		if *w.open && (st.drag > st.size.H/4 || g.Pan.Velocity.Y > 800) {
			w.dismiss() // the sheet slides out from where it was let go
		} else {
			st.drag = 0
		}
		c.Invalidate()
	}
}

// ---------------------------------------------------------------------------
// ActionSheet

// Action is one entry of an ActionSheet.
type Action struct {
	Key         string
	Text        string
	Description string
	Danger      bool
	Bold        bool
	Disabled    bool
}

// ActionSheetW is a bottom menu of actions.
type ActionSheetW struct {
	c        *ui.Context
	th       *Theme
	open     *bool
	actions  []Action
	extra    string
	cancel   string
	selected int
	done     bool
	opened   bool
	shown    bool
	maskOff  bool // closeOnMaskClick false
}

// ActionSheet opens a bottom action menu bound to *open. It renders when
// Show is called (after options) and closes after a selection. In the
// shadcn style the actions sit in an outlined rounded card floating above
// the bottom edge, the cancel button in a second card below.
//
// ConfigProvider defaults ("mobile.ActionSheet"): "cancelText" (string,
// shows the cancel button), "closeOnMaskClick" (bool).
func ActionSheet(c *ui.Context, open *bool, actions []Action) *ActionSheetW {
	w := &ActionSheetW{c: c, th: ThemeOf(c), open: open, actions: actions, selected: -1}
	if v, ok := cfgStr(c, "ActionSheet", "cancelText"); ok {
		w.cancel = v
	}
	if v, ok := cfgBool(c, "ActionSheet", "closeOnMaskClick"); ok {
		w.maskOff = !v
	}
	return w
}

// Extra shows a caption above the actions.
func (w *ActionSheetW) Extra(s string) *ActionSheetW { w.extra = s; return w }

// Cancel adds a separated cancel button ("" uses the localized "取消").
func (w *ActionSheetW) Cancel(s string) *ActionSheetW {
	w.cancel = or(s, w.c.Locale().Cancel)
	return w
}

// Show renders the sheet.
func (w *ActionSheetW) Show() *ActionSheetW {
	if w.shown {
		return w
	}
	w.shown = true
	c, th := w.c, w.th
	vant := th.Style == Vant
	p := newPopup(c, w.open).A11yLabel(or(w.extra, c.Locale().A11y.MoreActions))
	p.ownBottom = true
	if w.maskOff {
		p.CloseOnMask(false)
	}
	if th.shad() {
		w.showCards(p)
		return w
	}
	p.Body(func() {
		if w.extra != "" {
			fs := th.Font(th.FontMain, text.Regular)
			pad := geom.Insets{Top: 18, Bottom: 18, Left: 12, Right: 12}
			if vant {
				fs = th.Font(14, text.Regular)
				pad = geom.Insets{Top: 20, Bottom: 20, Left: 16, Right: 16}
			}
			b := ui.Custom(c, func(maxW, _ float32) geom.Size {
				lay := text.Shape(w.extra, fs, text.Options{MaxWidth: maxW - pad.Left - pad.Right})
				return geom.Size{W: maxW, H: lay.Height + pad.Top + pad.Bottom}
			}, nop)
			b.Semantics(a11y.RoleText, w.extra)
			b.Paint(func(l *paint.List, r geom.Rect) {
				lay := text.Shape(w.extra, fs, text.Options{MaxWidth: r.W - pad.Left - pad.Right})
				l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+pad.Top), lay.Glyphs, th.TextWeak)
				hairline(l, r, r.X, th.Border)
			})
		}
		for i, a := range w.actions {
			i, a := i, a
			ui.Key(c, i, func() {
				if w.item(a.Text, a.Description, a.Danger, a.Bold, a.Disabled, i > 0 || w.extra != "") {
					w.selected, w.done = i, true
				}
			})
		}
		if w.cancel != "" {
			ui.Div(c, layout.H(8)).Bg(th.PageBg)
			if w.item(w.cancel, "", false, false, false, false) {
				w.done = true
			}
		}
		bottomInset(c)
	})
	if w.done {
		*w.open = false
		c.Invalidate()
	}
	return w
}

// showCards renders the shadcn sheet: grouped rounded cards on a bare
// panel.
func (w *ActionSheetW) showCards(p *PopupW) {
	c, th := w.c, w.th
	p.bare = true
	p.Round(false).Bg(paintColor{}).Layout(layout.Padding(geom.Insets{Left: 8, Right: 8, Bottom: 8}), layout.Gap(8))
	card := func(fn func()) {
		ui.Column(c, fn).Bg(th.Box).Radius(th.RadiusLarge).Clip().Border(1, th.CardBorder)
	}
	p.Body(func() {
		card(func() {
			if w.extra != "" {
				fs := th.Font(th.FontSmall, text.Medium)
				pad := geom.Insets{Top: 14, Bottom: 14, Left: 16, Right: 16}
				b := ui.Custom(c, func(maxW, _ float32) geom.Size {
					lay := text.Shape(w.extra, fs, text.Options{MaxWidth: maxW - pad.Left - pad.Right})
					return geom.Size{W: maxW, H: lay.Height + pad.Top + pad.Bottom}
				}, nop)
				b.Semantics(a11y.RoleText, w.extra)
				b.Paint(func(l *paint.List, r geom.Rect) {
					lay := text.Shape(w.extra, fs, text.Options{MaxWidth: r.W - pad.Left - pad.Right})
					l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+pad.Top), lay.Glyphs, th.TextWeak)
				})
			}
			for i, a := range w.actions {
				i, a := i, a
				ui.Key(c, i, func() {
					if w.item(a.Text, a.Description, a.Danger, a.Bold, a.Disabled, i > 0 || w.extra != "") {
						w.selected, w.done = i, true
					}
				})
			}
		})
		if w.cancel != "" {
			card(func() {
				if w.item(w.cancel, "", false, true, false, false) {
					w.done = true
				}
			})
		}
		bottomInset(c)
	})
	if w.done {
		*w.open = false
		c.Invalidate()
	}
}

func (w *ActionSheetW) item(label, desc string, danger, bold, disabled, sep bool) bool {
	c, th := w.c, w.th
	vant := th.Style == Vant
	fs := th.Font(th.FontTitle, text.Regular)
	if vant {
		fs = th.Font(16, text.Regular)
	} else if th.shad() {
		fs = th.Font(th.FontBody, text.Regular)
	}
	if bold {
		fs.Weight = text.Semibold
		if th.shad() {
			fs.Weight = text.Medium
		}
	}
	ds := th.Font(th.FontSmall, text.Regular)
	h := th.ActionHeight
	if desc != "" {
		h += ds.LineHeight
	}
	b := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: h} }, nop)
	b.Semantics(a11y.RoleButton, label).A11yState(stateIf(disabled, a11y.Disabled))
	if c.A11yEnabled() && (desc != "" || disabled) {
		b.A11y(func(s *a11y.Node) {
			s.Description = desc
			if disabled {
				s.Actions &^= a11y.Press
			}
		})
	}
	pr := Touchable(c, b)
	b.Paint(func(l *paint.List, r geom.Rect) {
		if sep {
			hairlineTop(l, r, r.X, th.Border)
		}
		if pr.Pressed && !disabled {
			l.Fill(r, geom.Radii{}, th.Press)
		}
		col := th.Text
		if danger {
			col = th.Danger
		}
		if disabled {
			col = th.TextLight
		}
		if desc == "" {
			drawTextCenter(l, label, fs, r, col)
			return
		}
		top := r.Y + (r.H-fs.LineHeight-ds.LineHeight)/2
		drawTextCenter(l, label, fs, geom.Rect{X: r.X, Y: top, W: r.W, H: fs.LineHeight}, col)
		drawTextCenter(l, desc, ds, geom.Rect{X: r.X, Y: top + fs.LineHeight, W: r.W, H: ds.LineHeight}, th.TextWeak)
	})
	return pr.Tap && !disabled
}

// Selected returns the index of the action tapped this frame.
func (w *ActionSheetW) Selected() (int, bool) {
	w.Show()
	return w.selected, w.selected >= 0
}

// Cancelled reports a tap on the cancel button or the mask this frame.
func (w *ActionSheetW) Cancelled() bool {
	w.Show()
	return w.done && w.selected < 0
}

// ---------------------------------------------------------------------------
// Dialog

// DialogW is a centred modal with a title, message and buttons.
type DialogW struct {
	c          *ui.Context
	th         *Theme
	open       *bool
	title      string
	content    string
	ok, cancel string
	confirmed  bool
	cancelled  bool
	body       func()
	closeMask  bool
	shown      bool
	panel      ui.ID
	okDef      string // configured defaults of the button texts
	cancelDef  string
}

// Dialog creates a modal dialog bound to *open. By default it is an alert
// with one acknowledge button; Confirm adds a cancel button. Call Show
// (or a query) after the options.
//
// In the shadcn style the dialog follows AlertDialog on small screens:
// an outlined card with a centred title, a muted message and full-width
// stacked buttons (solid confirm above an outline cancel).
//
// ConfigProvider defaults ("mobile.Dialog"): "closeOnMaskClick" (bool),
// "confirmText" (string, the acknowledge / OK text), "cancelText"
// (string, used by Confirm when it gets "").
func Dialog(c *ui.Context, open *bool, title string) *DialogW {
	w := &DialogW{c: c, th: ThemeOf(c), open: open, title: title}
	if v, ok := cfgBool(c, "Dialog", "closeOnMaskClick"); ok {
		w.closeMask = v
	}
	if v, ok := cfgStr(c, "Dialog", "confirmText"); ok {
		w.ok, w.okDef = v, v
	}
	if v, ok := cfgStr(c, "Dialog", "cancelText"); ok {
		w.cancelDef = v
	}
	return w
}

// Content sets the message.
func (w *DialogW) Content(s string) *DialogW { w.content = s; return w }

// Custom builds custom content under the message.
func (w *DialogW) Custom(fn func()) *DialogW { w.body = fn; return w }

// Confirm turns the dialog into a confirm box with OK / Cancel buttons
// ("" keeps the localized texts).
func (w *DialogW) Confirm(ok, cancel string) *DialogW {
	loc := w.c.Locale()
	w.ok, w.cancel = or(ok, or(w.okDef, loc.Confirm)), or(cancel, or(w.cancelDef, loc.Cancel))
	return w
}

// OKText sets the acknowledge button text of an alert.
func (w *DialogW) OKText(s string) *DialogW { w.ok = s; return w }

// CloseOnMask lets a mask tap dismiss the dialog (cancel).
func (w *DialogW) CloseOnMask() *DialogW { w.closeMask = true; return w }

// Show renders the dialog.
func (w *DialogW) Show() *DialogW {
	if w.shown {
		return w
	}
	w.shown = true
	c, th := w.c, w.th
	vant := th.Style == Vant
	if w.ok == "" {
		w.ok = c.Locale().GotIt
	}
	width := min(th.DialogWidth, c.Size().W*0.85)
	p := newPopup(c, w.open).Position(PosCenter).CloseOnMask(w.closeMask).Round(true)
	p.Bg(th.Box).Layout(layout.W(width))
	p.role = a11y.RoleAlertDialog
	p.label = or(w.title, w.content)
	if w.title != "" {
		p.desc = w.content
	}
	if th.shad() {
		w.shadBody(p)
	} else {
		p.Body(func() {
			w.classicBody(vant)
		})
	}
	if p.panel != nil {
		w.panel = p.panel.ID()
	}
	if p.Closed() {
		w.cancelled = true
	}
	if w.confirmed || w.cancelled {
		*w.open = false
		c.Invalidate()
	}
	return w
}

// shadBody builds the shadcn AlertDialog content (mobile layout).
func (w *DialogW) shadBody(p *PopupW) {
	c, th := w.c, w.th
	p.Body(func() {
		ui.Column(c, func() {
			if w.title != "" {
				centerText(c, w.title, th.Font(th.FontTitle, text.Semibold), th.Text)
			}
			if w.content != "" {
				centerText(c, w.content, th.Font(th.FontMain, text.Regular), th.TextWeak)
			}
			if w.body != nil {
				w.body()
			}
			ui.Column(c, func() {
				if Button(c, w.ok).Primary().Block().Clicked() {
					w.confirmed = true
				}
				if w.cancel != "" && Button(c, w.cancel).Block().Clicked() {
					w.cancelled = true
				}
			}, layout.Gap(8), layout.Margin(geom.Insets{Top: 12}))
		}, layout.Padding(geom.All(24)), layout.Gap(8))
	})
}

func (w *DialogW) classicBody(vant bool) {
	c, th := w.c, w.th
	{
		ui.Column(c, func() {
			titleFS := th.Font(th.FontTitle, text.Semibold)
			msgFS := th.Font(th.FontMain, text.Regular)
			msgCol := th.Text
			pad := geom.Insets{Top: 20, Left: 12, Right: 12, Bottom: 20}
			if vant {
				titleFS = th.Font(16, text.Semibold)
				msgFS = th.Font(14, text.Regular)
				msgFS.LineHeight = 20
				pad = geom.Insets{Top: 26, Left: 24, Right: 24, Bottom: 26}
				if w.title != "" {
					msgCol = th.TextSecondary
				}
			}
			ui.Column(c, func() {
				if w.title != "" {
					centerText(c, w.title, titleFS, th.Text)
				}
				if w.content != "" {
					centerText(c, w.content, msgFS, msgCol)
				}
				if w.body != nil {
					w.body()
				}
			}, layout.Padding(pad), layout.Gap(8))
			if w.cancel == "" {
				if w.button(w.ok, true, true) {
					w.confirmed = true
				}
				return
			}
			ui.Row(c, func() {
				if w.button(w.cancel, false, true) {
					w.cancelled = true
				}
				if w.button(w.ok, true, false) {
					w.confirmed = true
				}
			})
		})
	}
}

func centerText(c *ui.Context, s string, fs text.Style, col theme.Color) {
	ui.Text(c, s).Size(fs.Size).LineHeight(fs.LineHeight).Weight(fs.Weight).Color(col).Align(ui.AlignCenter).Layout(layout.Self(layout.AlignStretch))
}

func (w *DialogW) button(label string, primary, divider bool) bool {
	c, th := w.c, w.th
	vant := th.Style == Vant
	fs := th.Font(th.FontTitle, text.Regular)
	h := float32(50)
	if vant {
		fs = th.Font(16, text.Regular)
		h = 48
	}
	if primary && !vant {
		fs.Weight = text.Semibold
	}
	b := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: h} }, nop, layout.Grow(1))
	b.Semantics(a11y.RoleButton, label)
	pr := Touchable(c, b)
	rtl := c.IsRTL()
	b.Paint(func(l *paint.List, r geom.Rect) {
		hairlineTop(l, r, r.X, th.Border)
		if divider && w.cancel != "" {
			// Between the buttons: at the end edge of the first one.
			spanOf(r, rtl).fill(l, geom.Rect{X: r.Right() - 0.5, Y: r.Y, W: 0.5, H: r.H}, geom.Radii{}, th.Border)
		}
		if pr.Pressed {
			l.Fill(r, geom.Radii{}, th.Press)
		}
		col := th.Text
		if primary {
			col = th.Primary
		} else if !vant {
			col = th.TextWeak
		}
		drawTextCenter(l, label, fs, r, col)
	})
	return pr.Tap
}

// PanelID returns the ID of the dialog panel (0 while closed).
func (w *DialogW) PanelID() ui.ID { w.Show(); return w.panel }

// Confirmed reports a tap on the OK / acknowledge button this frame.
func (w *DialogW) Confirmed() bool { w.Show(); return w.confirmed }

// Cancelled reports a cancel this frame (button or mask).
func (w *DialogW) Cancelled() bool { w.Show(); return w.cancelled }

// ---------------------------------------------------------------------------
// Toast

// ToastW is a short centred message.
type ToastW struct {
	c     *ui.Context
	th    *Theme
	show  *bool
	msg   string
	kind  string
	dur   time.Duration
	pos   Position
	shown bool
	a     *ui.Box
}

type toastState struct {
	until time.Time
	was   bool
}

// Toast shows msg while *show is true and clears it after the duration
// (2 s; 0 keeps it). Call Show after the options. In the shadcn style it
// is a Sonner toast: an outlined popover card with the icon inline,
// shown at the bottom by default.
//
// ConfigProvider defaults ("mobile.Toast"): "duration" (time.Duration or
// ms), "position" ("top", "center", "bottom").
func Toast(c *ui.Context, show *bool, msg string) *ToastW {
	th := ThemeOf(c)
	w := &ToastW{c: c, th: th, show: show, msg: msg, dur: 2 * time.Second, pos: PosCenter, a: anchor(c)}
	if th.shad() {
		w.pos = PosBottom
	}
	if v, ok := cfgDur(c, "Toast", "duration"); ok {
		w.dur = v
	}
	if v, ok := cfgPosition(c, "Toast"); ok {
		w.pos = v
	}
	return w
}

// Icon adds an icon: "success", "fail", "loading".
func (w *ToastW) Icon(kind string) *ToastW { w.kind = kind; return w }

// Duration sets the auto-hide delay (0 = stay).
func (w *ToastW) Duration(d time.Duration) *ToastW { w.dur = d; return w }

// Position sets the vertical placement (PosTop, PosCenter, PosBottom).
func (w *ToastW) Position(p Position) *ToastW { w.pos = p; return w }

// Show renders the toast.
func (w *ToastW) Show() *ToastW {
	if w.shown {
		return w
	}
	w.shown = true
	c := w.c
	id := w.a.ID()
	st := ui.State[toastState](c, id, nil)
	now := c.Now()
	if *w.show && !st.was {
		st.until = now.Add(w.dur)
	}
	st.was = *w.show
	if *w.show && w.dur > 0 {
		if !now.Before(st.until) {
			*w.show = false
			st.was = false
			c.Invalidate()
		} else {
			c.WakeAt(st.until)
		}
	}
	v := motion(c, id, *w.show, 200*time.Millisecond)
	if v <= 0.001 && !*w.show {
		return w
	}
	drawToast(c, w.th, id, w.msg, w.kind, w.pos, v)
	return w
}

func drawToast(c *ui.Context, th *Theme, id ui.ID, msg, kind string, pos Position, v float32) {
	if th.shad() {
		drawSonner(c, th, id, msg, kind, pos, v)
		return
	}
	vant := th.Style == Vant
	fs := th.Font(th.FontMain, text.Regular)
	if vant {
		fs = th.Font(14, text.Regular)
		fs.LineHeight = 20
	}
	maxW := min(c.Size().W*0.7, 280)
	ang := 0.0
	if kind == "loading" {
		ang = spinAngle(c, id)
	}
	place := ui.ScreenCenter
	margin := float32(0)
	if pos == PosTop {
		place, margin = ui.ScreenTop, c.Size().H*0.2
	}
	shift := float32(0)
	if pos == PosBottom {
		shift = c.Size().H * 0.3
	}
	ov := ui.Overlay(c, ui.OverlayOptions{Place: place, Margin: margin, ShiftY: shift}, func() {
		detached(c, func() {
			icon := kind != ""
			b := ui.Custom(c, func(_, _ float32) geom.Size {
				lay := text.Shape(msg, fs, text.Options{MaxWidth: maxW - 24})
				if icon {
					side := float32(120)
					if vant {
						side = 88
					}
					return geom.Size{W: max(side, lay.Width+24), H: max(side, 36+8+lay.Height+40)}
				}
				return geom.Size{W: lay.Width + 2*th.ToastPad, H: lay.Height + 2*th.ToastPad - 2}
			}, nop)
			// A live region: failures interrupt, the rest is polite.
			role := a11y.RoleStatus
			if kind == "fail" {
				role = a11y.RoleAlert
			}
			b.Semantics(role, msg).A11yState(stateIf(kind == "loading", a11y.Busy))
			b.Paint(func(l *paint.List, r geom.Rect) {
				l.Fill(r, geom.Rad(8), th.ToastBg)
				white := theme.Hex("#ffffff")
				lay := text.Shape(msg, fs, text.Options{MaxWidth: maxW - 24})
				if !icon {
					l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, white)
					return
				}
				is := float32(36)
				top := r.Y + (r.H-is-8-lay.Height)/2
				ir := geom.Rect{X: r.X + (r.W-is)/2, Y: top, W: is, H: is}
				switch kind {
				case "success":
					drawToastIcon(l, ir, toastCheck, white)
				case "fail":
					drawToastIcon(l, ir, toastCross, white)
				case "loading":
					drawSpinner(l, ir.X+is/2, ir.Y+is/2, is/2-3, ang, white)
				}
				l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, top+is+8), lay.Glyphs, white)
			})
		})
	})
	ov.Opacity(v)
}

// The toast status glyphs (a local "icon" flag shadows the package where
// they are drawn).
var toastCheck, toastCross = icon.Check, icon.Close

func drawToastIcon(l *paint.List, r geom.Rect, ic *icon.Icon, col theme.Color) {
	icon.Draw(l, ic, r, col, 0)
}

// ---------------------------------------------------------------------------
// Imperative toasts

// ToastOptions configure ShowToast.
type ToastOptions struct {
	Icon     string        // "success", "fail", "loading" or ""
	Duration time.Duration // default 2 s
	Position Position      // default PosCenter
}

type toastQueue struct {
	msg   string
	o     ToastOptions
	until time.Time
	seq   uint64
}

var keyToasts = ui.ID(0x6d6f62696c650003)

// ShowToast shows msg as a toast from event-handling code (replacing the
// current one). It is drawn by the enclosing Provider.
func ShowToast(c *ui.Context, msg string, o ...ToastOptions) {
	q := ui.State[toastQueue](c, keyToasts, nil)
	var opt ToastOptions
	if len(o) > 0 {
		opt = o[0]
	}
	if opt.Duration == 0 {
		opt.Duration = 2 * time.Second
		if d, ok := cfgDur(c, "Toast", "duration"); ok && d > 0 {
			opt.Duration = d
		}
	}
	q.msg, q.o, q.until = msg, opt, c.Now().Add(opt.Duration)
	q.seq++
	c.Invalidate()
}

// ClearToast hides the imperative toast.
func ClearToast(c *ui.Context) {
	q := ui.State[toastQueue](c, keyToasts, nil)
	q.until = time.Time{}
}

func renderToasts(c *ui.Context) {
	q := ui.State[toastQueue](c, keyToasts, nil)
	now := c.Now()
	on := q.msg != "" && now.Before(q.until)
	if on {
		c.WakeAt(q.until)
	}
	id := keyToasts
	v := motion(c, id, on, 200*time.Millisecond)
	if v <= 0.001 && !on {
		return
	}
	drawToast(c, ThemeOf(c), id, q.msg, q.o.Icon, q.o.Position, v)
}
