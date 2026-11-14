package ui

import (
	"sync/atomic"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Confirm / MessageBox / Prompt (Ant Modal.confirm/info/success/warning/error,
// Element ElMessageBox.confirm/alert/prompt)
//
// These are imperative: call one of them once (typically inside a
// Button(...).Clicked() branch) and the dialog stays on screen until a
// button, Escape or (optionally) the mask closes it. The application does not
// keep an open flag; dialogs live in a global layer like Message.

// ConfirmOptions configure Confirm, MessageBox and Prompt.
type ConfirmOptions struct {
	Title   string
	Content string
	// Status picks the leading icon: "info", "success", "warning", "error"
	// or "none". Confirm/Prompt default to "warning"/"none", MessageBox to
	// "info".
	Status     string
	OkText     string // default "确定" ("知道了" for Ant MessageBox)
	CancelText string // default "取消"
	Danger     bool   // red OK button (destructive actions)
	Width      float32
	// MaskClosable closes the dialog (as a cancel) on a mask click.
	MaskClosable bool
	// OnOk runs on the UI thread when OK is activated. The dialog closes
	// after it returns unless it called d.Loading(true); then it stays open
	// with a spinning OK button until d.Close().
	OnOk func(d *ConfirmRef)
	// OnCancel runs on Cancel, the close button, Escape or a mask click.
	OnCancel func()
	// DistinguishCancelAndClose tells the close button, Escape and a
	// mask click apart from Cancel: they run OnClose (not OnCancel) and
	// ConfirmRef.Action reports ConfirmActionClose.
	DistinguishCancelAndClose bool
	// OnClose runs when the dialog is closed without Cancel or OK
	// (close button, Escape, mask) and DistinguishCancelAndClose is set.
	OnClose func()
	// Center centers the icon, title, content and buttons.
	Center bool
	// Draggable lets the user move the dialog by dragging its title.
	Draggable bool
	// Icon replaces the status icon; it is drawn in the status color
	// (the primary color for Status "none").
	Icon *icon.Icon

	// Prompt only.
	InputValue       string
	InputPlaceholder string
	// Validate returns an error message ("" = valid); OK is refused while
	// it fails.
	Validate func(v string) string
}

// ConfirmRef is a handle to an open dialog.
type ConfirmRef struct {
	id      ID
	kind    int
	o       ConfirmOptions
	value   string
	errMsg  string
	tried   bool
	focused bool
	ret     ID   // focus before the box opened (restored when it closes)
	retSet  bool // ret captured
	loading atomic.Bool
	closed  atomic.Bool
	win     Window  // to wake the UI after Close/Loading from another goroutine
	loc     *Locale // locale active where the box was opened (rendered in a global layer)
	env     *env    // scope the box was opened in (theme, direction)
	action  ConfirmAction
	// Draggable: the drag handle built this frame and the drag offset.
	handle         *node
	off, from, at0 geom.Point
}

// ConfirmAction is how a Confirm / MessageBox / Prompt dialog was closed.
type ConfirmAction string

const (
	// ConfirmActionConfirm: the OK button (or Enter in a Prompt).
	ConfirmActionConfirm ConfirmAction = "confirm"
	// ConfirmActionCancel: the Cancel button, or with
	// DistinguishCancelAndClose unset also the close button, Escape and
	// a mask click.
	ConfirmActionCancel ConfirmAction = "cancel"
	// ConfirmActionClose: the close button, Escape or a mask click when
	// DistinguishCancelAndClose is set.
	ConfirmActionClose ConfirmAction = "close"
)

const (
	moreBoxConfirm = iota
	moreBoxAlert
	moreBoxPrompt
)

type moreBoxes struct {
	list []*ConfirmRef
	seq  int
}

var moreBoxesKey = hashID("more-confirm-boxes")

func init() { globalLayers = append(globalLayers, moreRenderBoxes) }

// Confirm opens a confirmation dialog with Cancel and OK buttons.
func Confirm(c *Context, o ConfirmOptions) *ConfirmRef { return moreOpenBox(c, moreBoxConfirm, o) }

// MessageBox opens an information dialog with a single OK button (Ant
// Modal.info/success/warning/error, Element ElMessageBox.alert). Set
// o.Status to pick the icon.
func MessageBox(c *Context, o ConfirmOptions) *ConfirmRef { return moreOpenBox(c, moreBoxAlert, o) }

// Prompt opens a dialog with a text input; read the value in OnOk with
// d.Value().
func Prompt(c *Context, o ConfirmOptions) *ConfirmRef { return moreOpenBox(c, moreBoxPrompt, o) }

func moreOpenBox(c *Context, kind int, o ConfirmOptions) *ConfirmRef {
	o.defaults(c) // ConfigProvider "Confirm" / "Modal" defaults
	bs := State[moreBoxes](c, moreBoxesKey, nil)
	bs.seq++
	d := &ConfirmRef{id: hashID("more-confirm", bs.seq), kind: kind, o: o, value: o.InputValue, win: c.win, loc: c.Locale(), env: c.env}
	bs.list = append(bs.list, d)
	c.invalid = true
	return d
}

// Close closes the dialog without calling any callback. It is safe to call
// from any goroutine (e.g. when async work started in OnOk finishes): the
// state is atomic and a frame is requested through Window.Invalidate,
// which the Window contract makes thread-safe.
func (d *ConfirmRef) Close() {
	d.closed.Store(true)
	d.wake()
}

// Loading shows (or clears) the spinner on the OK button. While loading,
// OK is inert and the dialog stays open after OnOk returns. Safe to call
// from another goroutine.
func (d *ConfirmRef) Loading(on bool) {
	d.loading.Store(on)
	d.wake()
}

// wake requests a frame; Window.Invalidate may be called from any thread.
func (d *ConfirmRef) wake() {
	if d.win != nil {
		d.win.Invalidate()
	}
}

// IsOpen reports whether the dialog is still shown.
func (d *ConfirmRef) IsOpen() bool { return !d.closed.Load() }

// Value returns the Prompt input.
func (d *ConfirmRef) Value() string { return d.value }

// Action reports how the dialog was closed by the user ("" while open or
// after Close).
func (d *ConfirmRef) Action() ConfirmAction { return d.action }

// OpenDialogs returns how many Confirm/MessageBox/Prompt dialogs are open.
func OpenDialogs(c *Context) int {
	bs := State[moreBoxes](c, moreBoxesKey, nil)
	n := 0
	for _, d := range bs.list {
		if d.IsOpen() {
			n++
		}
	}
	return n
}

func moreRenderBoxes(c *Context) {
	bs := State[moreBoxes](c, moreBoxesKey, nil)
	keep := bs.list[:0]
	for _, d := range bs.list {
		if motionLive(c, d.id, !d.closed.Load()) { // closed boxes stay while leaving
			keep = append(keep, d)
		}
	}
	clear(bs.list[len(keep):])
	bs.list = keep
	if len(bs.list) == 0 {
		return
	}
	list := append([]*ConfirmRef(nil), bs.list...)
	for i, d := range list {
		top := i == len(list)-1
		Key(c, d.id, func() { moreRenderBox(c, d, top) })
	}
}

func (d *ConfirmRef) status() string {
	s := d.o.Status
	if s == "" {
		switch d.kind {
		case moreBoxAlert:
			s = "info"
		case moreBoxPrompt:
			s = "none"
		default:
			s = "warning"
		}
	}
	return s
}

func (d *ConfirmRef) ok(c *Context) {
	if d.loading.Load() {
		return
	}
	if d.kind == moreBoxPrompt && d.o.Validate != nil {
		d.tried = true
		if d.errMsg = d.o.Validate(d.value); d.errMsg != "" {
			d.focused = false // put the caret back into the input
			c.invalid = true
			return
		}
	}
	d.action = ConfirmActionConfirm
	if d.o.OnOk != nil {
		d.o.OnOk(d)
	}
	if !d.loading.Load() {
		d.closed.Store(true)
	}
	c.invalid = true
}

// cancel closes the dialog through Cancel (viaClose false) or the close
// button, Escape or the mask (viaClose true).
func (d *ConfirmRef) cancel(c *Context, viaClose bool) {
	if !d.closed.CompareAndSwap(false, true) {
		return // already closed (possibly by Close from another goroutine)
	}
	if viaClose && d.o.DistinguishCancelAndClose {
		d.action = ConfirmActionClose
		if d.o.OnClose != nil {
			d.o.OnClose()
		}
	} else {
		d.action = ConfirmActionCancel
		if d.o.OnCancel != nil {
			d.o.OnCancel()
		}
	}
	c.invalid = true
}

func moreRenderBox(c *Context, d *ConfirmRef, top bool) {
	// Boxes render in a global layer, outside the ConfigProvider of the
	// caller: restore the scope (theme, direction) and locale captured by
	// moreOpenBox.
	c.withEnv(d.env, func() {
		ConfigProvider(c).Locale(d.loc).Body(func() { moreRenderBoxIn(c, d, top) })
	})
}

func moreRenderBoxIn(c *Context, d *ConfirmRef, top bool) {
	th := c.Theme
	tk := &c.ThemeFor(theme.CompModal).Components.Modal
	el := th.Flavor == theme.Element
	mo, _ := modalMotion(c, d.id, !d.closed.Load())
	mask := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {})
	mask.Bg(theme.Fade(tk.MaskBg, mo.mask()))
	if !mo.leaving && d.o.MaskClosable && c.clicked[mask.n.id] > 0 {
		d.cancel(c, true)
		return
	}
	width := d.o.Width
	if width == 0 {
		width = 416
		if th.Flavor == theme.StyleShadcn {
			width = 512 // max-w-lg
		}
		if el {
			width = 420
		}
	}
	okText := d.o.OkText
	if okText == "" {
		okText = c.Locale().OK
		if d.kind == moreBoxAlert && !el {
			okText = c.Locale().GotIt
		}
	}
	cancelText := d.o.CancelText
	if cancelText == "" {
		cancelText = c.Locale().Cancel
	}
	status := d.status()
	var focusID ID
	var okClicked, cancelClicked, closeClicked bool
	buttons := func() {
		if d.kind != moreBoxAlert {
			if Button(c, cancelText).Clicked() {
				cancelClicked = true
			}
		}
		ok := Button(c, okText).Primary().Loading(d.loading.Load())
		if d.o.Danger {
			ok.Danger()
		}
		if focusID == 0 {
			focusID = ok.ID()
		}
		if ok.Clicked() {
			okClicked = true
		}
	}
	input := func() {
		in := Input(c, &d.value).Placeholder(d.o.InputPlaceholder)
		if d.errMsg != "" {
			in.Status("error")
		}
		focusID = in.ID()
		if in.Changed() && d.tried && d.o.Validate != nil {
			d.errMsg = d.o.Validate(d.value)
		}
		if in.Submitted() {
			okClicked = true
		}
		if d.errMsg != "" {
			t := Text(c, d.errMsg).Size(12).Color(th.Error.Base)
			if el {
				t.LineHeight(18).Layout(layout.Margin(geom.Insets{Top: 2}))
			}
		}
	}
	var place Placement
	var margin float32
	if el || th.Flavor == theme.StyleShadcn {
		place, margin = ScreenCenter, 16 // shadcn AlertDialog is centered
	} else {
		place, margin = ScreenTopCenter, 100
	}
	d.handle = nil
	dlg := Overlay(c, mo.place(OverlayOptions{Place: place, Margin: margin, ShiftX: d.off.X, ShiftY: d.off.Y}), func() {
		switch {
		case el:
			moreElementBox(c, d, status, buttons, input, &closeClicked)
		case th.Flavor == theme.StyleShadcn:
			shad2cConfirmBox(c, d, status, buttons, input)
		default:
			moreAntBox(c, d, status, buttons, input)
		}
	}, layout.W(width))
	dlg.Bg(tk.ContentBg).Radius(tk.Radius).Shadow(tk.Shadow)
	if tk.BorderColor.A > 0 {
		dlg.Border(1, tk.BorderColor)
	}
	mo.apply(dlg)
	role := a11y.RoleAlertDialog
	if d.kind == moreBoxPrompt {
		role = a11y.RoleDialog
	}
	name, desc := d.o.Title, d.o.Content
	if name == "" {
		name, desc = desc, ""
	}
	dlg.n.setRole(role).setName(name).setState(a11y.Modal)
	if c.a11yOn {
		dlg.n.fill(semFunc(func(s *a11y.Node) {
			s.Description = desc
			dialogSem(s)
		}))
	}
	if mo.leaving {
		d.restoreFocus(c)
		return // fading out: buttons and Escape no longer act
	}
	if top {
		c.TrapFocus(dlg.n.id)
	}
	d.drag(c, dlg.n.id)
	if !d.focused && focusID != 0 {
		d.focused = true
		if !d.retSet {
			d.ret, d.retSet = c.focus, true
		}
		c.SetFocus(focusID)
	}
	if top && c.EscapeAt(dlg.n.id) {
		closeClicked = true
	}
	switch {
	case okClicked:
		d.ok(c)
	case cancelClicked:
		d.cancel(c, false)
	case closeClicked:
		d.cancel(c, true)
	}
	if d.closed.Load() {
		d.restoreFocus(c)
	}
}

// drag moves a Draggable dialog (panel dlg) by its title: the handle the
// box builder recorded this frame follows the pointer, kept inside the
// window so the title stays reachable.
func (d *ConfirmRef) drag(c *Context, dlg ID) {
	h := d.handle
	d.handle = nil
	if !d.o.Draggable || h == nil {
		return
	}
	ia := c.interact(h, false, input.CursorMove)
	if ia.PressedNow {
		d.from, d.at0 = d.off, c.pointer
	}
	if !c.Dragging(h.id) {
		return
	}
	nx := d.from.X + c.pointer.X - d.at0.X
	ny := d.from.Y + c.pointer.Y - d.at0.Y
	if r, ok := c.rects[dlg]; ok {
		baseX, baseY := r.X-d.off.X, r.Y-d.off.Y
		nx = max(-baseX, min(nx, c.size.W-baseX-r.W))
		ny = max(-baseY, min(ny, c.size.H-baseY-40))
	}
	if nx != d.off.X || ny != d.off.Y {
		d.off = geom.Point{X: nx, Y: ny}
		c.invalid = true
	}
}

// leadIcon adds the leading icon of size px: the custom Icon in the status
// color, else the status icon; nil for status "none" without Icon.
func (d *ConfirmRef) leadIcon(c *Context, status string, size float32) *node {
	if d.o.Icon != nil {
		col := c.Theme.Primary.Base
		if status != "none" {
			col = c.Theme.ColorOf(status).Base
		}
		return Icon(c, d.o.Icon).Size(size).Color(col).n
	}
	if status == "none" {
		return nil
	}
	return statusIcon(c, status, size)
}

// justify is the main-axis option of the title, content and button rows:
// centered with Center, else start (end for buttons).
func (d *ConfirmRef) justify(buttons bool) layout.Option {
	switch {
	case d.o.Center:
		return layout.JustifyContent(layout.JustifyCenter)
	case buttons:
		return layout.End()
	}
	return layout.JustifyContent(layout.JustifyStart)
}

// textAlign is the alignment of the title and content.
func (d *ConfirmRef) textAlign() TextAlign {
	if d.o.Center {
		return AlignCenter
	}
	return AlignStart
}

// grow is the text column's main-axis option: it fills its row unless the
// box is centered.
func (d *ConfirmRef) grow() layout.Option {
	if d.o.Center {
		return layout.Grow(0)
	}
	return layout.Grow(1)
}

// restoreFocus gives keyboard focus back to the widget focused when the
// box opened (the button that opened it), once.
func (d *ConfirmRef) restoreFocus(c *Context) {
	if d.ret != 0 && c.focusLayer(d.ret) >= 0 {
		c.SetFocus(d.ret)
	}
	d.ret = 0
}

func moreAntBox(c *Context, d *ConfirmRef, status string, buttons, input func()) {
	th := c.Theme
	Column(c, func() {
		texts := func() {
			if d.o.Title != "" {
				Text(c, d.o.Title).Size(th.FontSizeLG).LineHeight(24).Strong().Color(th.TextHeading).Align(d.textAlign())
			}
			if d.o.Content != "" {
				Text(c, d.o.Content).Align(d.textAlign())
			}
			if d.kind == moreBoxPrompt {
				Column(c, input, layout.Gap(4), layout.Margin(geom.Insets{Top: 4}), layout.Self(layout.AlignStretch))
			}
		}
		var head *Box
		if d.o.Center {
			// Centered: the icon sits above the centered title.
			head = Column(c, func() {
				d.leadIcon(c, status, 32)
				texts()
			}, layout.AlignItems(layout.AlignCenter), layout.Gap(8))
		} else {
			head = Row(c, func() {
				if ic := d.leadIcon(c, status, 22); ic != nil {
					ic.ln.Margin = geom.Insets{Top: 1, Right: 12}
				}
				Column(c, texts, layout.Grow(1), layout.Gap(8))
			}, layout.AlignItems(layout.AlignStart))
		}
		d.handle = head.n
		Row(c, buttons, d.justify(true), layout.Gap(8), layout.Margin(geom.Insets{Top: 24}))
	}, layout.PadVH(20, 24))
}

// moreElementBox is the Element ElMessageBox body. close reports the
// close button.
func moreElementBox(c *Context, d *ConfirmRef, status string, buttons, input func(), close *bool) {
	th := c.Theme
	center := d.o.Center
	Column(c, func() {
		head := Row(c, func() {
			if center {
				// Balance the close button so the title stays centered.
				Spacer(c)
				Div(c, layout.W(24))
				if ic := d.leadIcon(c, status, 24); ic != nil {
					ic.ln.Margin.Right = 8
				}
			}
			t := Text(c, d.o.Title).Size(18).LineHeight(24).Color(th.TextHeading)
			if center {
				Spacer(c)
			} else {
				t.Layout(layout.Grow(1))
			}
			if IconButton(c, icon.Close, c.Locale().Close).Size(24, 16).NoHoverBg().Color(th.TextTertiary).Clicked() {
				*close = true
			}
		}, layout.Middle(), layout.Margin(geom.Insets{Bottom: 12}))
		d.handle = head.n
		Row(c, func() {
			if !center {
				if ic := d.leadIcon(c, status, 24); ic != nil {
					ic.ln.Margin.Right = 12
				}
			}
			if d.o.Content != "" {
				Text(c, d.o.Content).Color(th.TextSecondary).LineHeight(24).Align(d.textAlign()).Layout(d.grow())
			}
		}, layout.Middle(), d.justify(false))
		if d.kind == moreBoxPrompt {
			Column(c, input, layout.Margin(geom.Insets{Top: 12}))
		}
		Row(c, buttons, d.justify(true), layout.Gap(12), layout.Margin(geom.Insets{Top: 12}))
	}, layout.Pad(12))
}
