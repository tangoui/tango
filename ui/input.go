package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// frameLook describes an input-like control frame.
type frameState struct {
	hover, focus, disabled bool
	status                 string // "", "error", "warning"
	borderless             bool
	variant                string       // "" / "outlined", "filled", "borderless", "underlined"
	bg                     *theme.Color // custom background (Input.Bg)
	noBorder               bool         // no border nor focus halo (Input.NoBorder)
}

// frameTok is the token set a frame is painted with: the Input token for
// input-like controls, the Select token for selectors.
type frameTok struct {
	th                        *theme.Theme
	bg, hoverBg, activeBg     theme.Color
	hoverBorder, activeBorder theme.Color
	outline, errOutline       theme.Color
	warnOutline               theme.Color
	outlineW, outlineOff      float32
	shadow                    []paint.Shadow
}

// inputFrameTok reads the frame tokens of the Input component of th.
func inputFrameTok(th *theme.Theme) frameTok {
	k := &th.Components.Input
	return frameTok{th: th, bg: k.Bg, hoverBg: k.HoverBg, activeBg: k.ActiveBg,
		hoverBorder: k.HoverBorderColor, activeBorder: k.ActiveBorderColor,
		outline: k.ActiveShadowColor, errOutline: k.ErrorActiveShadowColor, warnOutline: k.WarningActiveShadowColor,
		outlineW: k.ActiveShadowWidth, outlineOff: k.ActiveShadowOffset, shadow: k.Shadow}
}

// paintFrame draws the shared input frame (Input token of the node's
// theme): background, border and the focus indication of the active
// design system.
func (c *Context) paintFrame(l *paint.List, r geom.Rect, rad geom.Radii, id ID, fs frameState) {
	c.paintFrameWith(l, r, rad, id, fs, inputFrameTok(c.ThemeFor(theme.CompInput)))
}

// paintFrameWith is paintFrame with an explicit token set (selectors pass
// the Select token's).
func (c *Context) paintFrameWith(l *paint.List, r geom.Rect, rad geom.Radii, id ID, fs frameState, ft frameTok) {
	th := ft.th
	bg := ft.bg
	border := th.Border
	hoverC := ft.hoverBorder
	focusC := ft.activeBorder
	outline := ft.outline
	switch fs.status {
	case "error":
		border, hoverC, focusC = th.Error.Base, th.Error.Hover, th.Error.Base
		outline = ft.errOutline
	case "warning":
		border, hoverC, focusC = th.Warning.Base, th.Warning.Hover, th.Warning.Base
		outline = ft.warnOutline
	}
	faded := false
	if th.Flavor == theme.StyleShadcn {
		// new-york: disabled fades the whole field to 50% (without its
		// shadow: opacity multiplies each op, so the shadow would show
		// through).
		if fs.status != "" {
			hoverC = border // no hover border
		}
		if fs.disabled && !fs.borderless {
			faded = true
			l.Opacity(0.5)
			defer l.Pop()
			fs.disabled, fs.hover, fs.focus = false, false, false
		}
	}
	if fs.disabled {
		bg = th.BgContainerDisabled
		if th.Flavor == theme.Element {
			border = th.Split
		}
		l.Fill(r, rad, bg)
		l.Stroke(r, rad, 1, border)
		return
	}
	if fs.borderless {
		return // bare inputs (Command search) draw no frame at all
	}
	if fs.variant != "" && fs.variant != "outlined" || fs.bg != nil || fs.noBorder {
		c.paintFrameVariant(l, r, rad, id, fs, ft, bg, border, hoverC, focusC, outline)
		return
	}
	h := c.Anim(id, "fhover", b2f(fs.hover), th.DurationMid)
	f := c.Anim(id, "ffocus", b2f(fs.focus), th.DurationMid)
	col := theme.Lerp(theme.Lerp(border, hoverC, h), focusC, f)
	bg = theme.Lerp(theme.Lerp(bg, ft.hoverBg, h), ft.activeBg, f)
	if f > 0 && outline.A > 0 && ft.outlineW > 0 {
		paintOutlineWith(l, ft.outlineW, ft.outlineOff, r, rad, theme.Fade(outline, float64(f)))
	}
	l.Fill(r, rad, bg)
	if !fs.borderless {
		if len(ft.shadow) > 0 && !faded {
			l.Shadows(r, rad, ft.shadow)
			l.Fill(r, rad, bg)
		}
		l.Stroke(r, rad, 1, col)
	}
}

// paintOutline draws the focused-input halo: filled behind the control when
// it touches the border (Ant), a separate ring when the theme sets an offset.
func paintOutline(l *paint.List, th *theme.Theme, r geom.Rect, rad geom.Radii, col theme.Color) {
	paintOutlineWith(l, th.ControlOutlineWidth, th.ControlOutlineOffset, r, rad, col)
}

// paintOutlineWith is paintOutline with an explicit halo width and gap.
func paintOutlineWith(l *paint.List, w, off float32, r geom.Rect, rad geom.Radii, col theme.Color) {
	if off <= 0 {
		l.Fill(r.Expand(w), rad.Grow(w), col)
		return
	}
	l.Stroke(r.Expand(off+w), rad.Grow(off+w), w, col)
}

// InputW is a single-line text input.
type InputW struct {
	_ [0]func()

	c           *Context
	th          *theme.Theme      // Input theme (its Alias applied)
	tk          *theme.InputToken // &th.Components.Input
	box, ed     *node
	val         *string
	e           *editState
	st          *inputState
	placeholder string
	prefix      *icon.Icon
	suffix      *icon.Icon
	prefixText  string
	suffixText  string
	allowClear  bool
	password    bool
	disabled    bool
	readOnly    bool
	bare        bool
	status      string
	size        ControlSize
	maxLen      int
	res         editResult
	done        bool
	boxIA, edIA Interaction
	cleared     bool
	partHover   bool // the pointer is on the eye / clear button

	variant   string       // Variant
	bg        *theme.Color // Bg
	noBorder  bool         // NoBorder
	showCount bool         // ShowCount

	kind  input.TextKind // Kind
	enter input.EnterKey // EnterKey
}

type inputState struct {
	edit      editState
	showPwd   bool
	clearRect geom.Rect
	eyeRect   geom.Rect
}

// Input adds a text input bound to *val.
func Input(c *Context, val *string) *InputW {
	w := &InputW{c: c, val: val, status: c.itemStatus, size: c.defaultSize()}
	w.th = c.ThemeFor(theme.CompInput)
	w.tk = &w.th.Components.Input
	w.box = c.add("input", "")
	w.box.ln.Dir = layout.Row
	w.box.ln.Align = layout.AlignCenter
	w.st = State[inputState](c, w.box.id, nil)
	w.e = &w.st.edit
	w.ed = &node{id: hashID(w.box.id, "editor"), parent: w.box, disabled: w.box.disabled}
	w.ed.ln.Ref = w.ed
	w.ed.ln.Grow = 1
	w.ed.ln.Measure = func(maxW, _ float32) geom.Size {
		return geom.Size{W: 120, H: w.th.LineHeightFor(w.fontSize())}
	}
	w.box.kids = append(w.box.kids, w.ed)
	w.box.paint = w.paintBox
	w.ed.paint = w.paintText
	w.ed.setRole(a11y.RoleTextbox).fill(w)
	w.ed.ownKeys = true
	w.edIA = c.interact(w.ed, true, input.CursorText)
	w.boxIA = c.interact(w.box, false, input.CursorText)
	w.configDefaults()
	w.sizeDefaults("Input")
	c.deferred = append(c.deferred, w.finish)
	return w
}

func (w *InputW) Placeholder(s string) *InputW { w.placeholder = s; return w }
func (w *InputW) Prefix(ic *icon.Icon) *InputW { w.prefix = ic; return w }
func (w *InputW) Suffix(ic *icon.Icon) *InputW { w.suffix = ic; return w }

// PrefixText / SuffixText show short static text (units, currency).
func (w *InputW) PrefixText(s string) *InputW { w.prefixText = s; return w }
func (w *InputW) SuffixText(s string) *InputW { w.suffixText = s; return w }

// AllowClear shows a clear button when the input has a value.
func (w *InputW) AllowClear() *InputW { w.allowClear = true; return w }

// Kind tells soft keyboards what the field holds (input.TextEmail,
// input.TextNumber, …): the keyboard layout and whether it suggests words.
// It does not filter what can be typed. Password sets input.TextPassword.
func (w *InputW) Kind(k input.TextKind) *InputW { w.kind = k; return w }

// EnterKey labels the soft keyboard's return key (input.EnterSearch,
// input.EnterNext, …). Pressing it still submits (Submitted).
func (w *InputW) EnterKey(k input.EnterKey) *InputW { w.enter = k; return w }

// Password masks the value and adds a visibility toggle.
func (w *InputW) Password() *InputW { w.password = true; return w }

// Disabled disables editing.
func (w *InputW) Disabled(on bool) *InputW {
	if on {
		w.disabled = true
		w.ed.disabled = true
		w.box.disabled = true
		w.ed.focus = false
		w.ed.cursor = input.CursorNotAllowed
		w.box.cursor = input.CursorNotAllowed
	}
	return w
}

// ReadOnly prevents edits but allows selection.
func (w *InputW) ReadOnly() *InputW { w.readOnly = true; return w }

// Borderless drops the frame (background, border and focus halo), for
// inputs embedded in another surface such as the Command search row.
func (w *InputW) Borderless() *InputW { w.bare = true; return w }

// Status sets the validation status: "error" or "warning".
func (w *InputW) Status(s string) *InputW { w.status = s; return w }

// Size sets the control size.
func (w *InputW) Size(s ControlSize) *InputW { w.size = s; return w }

// MaxLength limits the number of runes.
func (w *InputW) MaxLength(n int) *InputW { w.maxLen = n; return w }

// Width fixes the width.
func (w *InputW) Width(px float32) *InputW { w.box.ln.Width = px; return w }

// Layout applies layout options to the outer frame.
func (w *InputW) Layout(opts ...layout.Option) *InputW {
	flexbox.Apply(&w.box.ln.Style, opts...)
	return w
}

// Changed reports whether the value was edited this frame.
func (w *InputW) Changed() bool { w.finish(); return w.res.changed || w.cleared }

// Submitted reports an Enter key press.
func (w *InputW) Submitted() bool { w.finish(); return w.res.submitted }

// Focused reports keyboard focus.
func (w *InputW) Focused() bool { return w.c.focus == w.ed.id }

// Focus requests keyboard focus.
func (w *InputW) Focus() *InputW { w.c.SetFocus(w.ed.id); return w }

// ID returns the editor's widget ID.
func (w *InputW) ID() ID { return w.ed.id }

// A11yLabel sets the accessible name. Inside a FormItem the item label is
// used; otherwise the placeholder is the last resort.
func (w *InputW) A11yLabel(s string) *InputW { w.ed.setName(s); return w }

func (w *InputW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = *w.val, w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	if w.password {
		s.States |= a11y.Protected
	}
	if w.readOnly {
		s.States |= a11y.ReadOnly
		s.Actions &^= a11y.SetValue
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.showCount {
		s.Description = w.countText()
	}
}

func (w *InputW) suffixWidth() float32 {
	is := w.fontSize()
	sw := w.countWidth()
	if w.allowClear {
		sw += is + 4
	}
	if w.password {
		sw += is + 4
	}
	if w.suffix != nil {
		sw += is + 4
	}
	if w.suffixText != "" {
		sw += text.Width(w.suffixText, w.c.style(w.fontSize(), text.Regular)) + 4
	}
	return sw
}

func (w *InputW) prefixWidth() float32 {
	pw := float32(0)
	if w.prefix != nil {
		pw += w.fontSize() + 4
	}
	if w.prefixText != "" {
		pw += text.Width(w.prefixText, w.c.style(w.fontSize(), text.Regular)) + 4
	}
	return pw
}

func (w *InputW) padX() float32 {
	switch w.size {
	case Small:
		return w.tk.PaddingInlineSM
	case Large:
		return w.tk.PaddingInlineLG
	}
	return w.tk.PaddingInline
}

// fontSize is the text (and affix icon) size of the field.
func (w *InputW) fontSize() float32 { return inputFontSize(w.tk, w.size) }

// inputFontSize is the text size of an input-like field of size s.
func inputFontSize(tk *theme.InputToken, s ControlSize) float32 {
	switch s {
	case Small:
		return tk.InputFontSizeSM
	case Large:
		return tk.InputFontSizeLG
	}
	return tk.InputFontSize
}

func (w *InputW) finish() {
	if w.done {
		return
	}
	w.done = true
	c := w.c
	w.box.ln.Height = controlHeightOf(w.th, w.size)
	w.box.ln.Padding = geom.Insets{Left: w.padX() + w.prefixWidth(), Right: w.padX() + w.suffixWidth()}
	if w.showCount {
		w.countNode()
	}
	if w.box.disabled {
		return
	}
	if w.password || w.allowClear && *w.val != "" {
		w.parts()
	}
	if w.boxIA.PressedNow && !w.edIA.PressedNow {
		c.SetFocus(w.ed.id)
	}
	w.e.hints = input.TextHints{Kind: w.kind, Enter: w.enter}
	if w.password {
		w.e.hints.Kind = input.TextPassword
	}
	w.res = c.processEdit(w.ed.id, w.e, w.val, editConfig{maxLen: w.maxLen, readOnly: w.readOnly}, w.edIA)
}

// parts adds the password eye and the clear button as nodes over the
// icons the frame paints (same hit area as before: the icon grown by 3px),
// so they have a name and the eye is reachable with Tab.
func (w *InputW) parts() {
	c := w.c
	loc := c.Locale()
	is := w.fontSize()
	right := w.padX() + w.countWidth()
	if w.suffixText != "" {
		right += text.Width(w.suffixText, w.font()) + 4
	}
	if w.suffix != nil {
		right += is + 4
	}
	top := (w.box.ln.Height - is) / 2
	part := func(key, name string, focusable bool) (Interaction, *node) {
		n := &node{id: hashID(w.box.id, key), parent: w.box, label: key}
		n.ln.Ref = n
		flexbox.Apply(&n.ln.Style, layout.Abs(layout.Unset, top-3, right-3, layout.Unset))
		n.ln.Width, n.ln.Height = is+6, is+6
		n.setRole(a11y.RoleButton).setName(name)
		w.box.kids = append(w.box.kids, n)
		ia := c.interact(n, focusable, input.CursorPointer)
		w.partHover = w.partHover || ia.Hovered
		if focusable {
			n.over = func(l *paint.List, r geom.Rect) {
				if ia.FocusVisible {
					entFocusRing(c, l, r, 4)
				}
			}
		}
		return ia, n
	}
	if w.password {
		name := loc.A11y.ShowPassword
		if w.st.showPwd {
			name = loc.A11y.HidePassword
		}
		ia, _ := part("eye", name, true)
		if ia.PressedNow {
			c.SetFocus(w.ed.id) // a pointer press keeps the caret in the field
		}
		if ia.Clicked {
			w.st.showPwd = !w.st.showPwd
			c.invalid = true
		}
		right += is + 4
	}
	if w.allowClear && *w.val != "" {
		// Not a Tab stop (as in Ant / Element): select-all + Delete is
		// the keyboard equivalent.
		ia, n := part("clear", loc.Clear, false)
		n.aKbd = true
		if ia.Clicked {
			*w.val = ""
			w.e.cur, w.e.anc = 0, 0
			w.cleared = true
			c.SetFocus(w.ed.id)
			c.invalid = true
		}
	}
}

func (w *InputW) font() text.Style { return w.c.style(w.fontSize(), text.Regular) }

func (w *InputW) paintBox(l *paint.List, r geom.Rect) {
	c, th := w.c, w.th
	rad := geom.Rad(controlRadiusOf(th, w.size))
	hover := w.boxIA.Hovered || w.edIA.Hovered || w.partHover
	if w.variant == "underlined" {
		rad = geom.Radii{}
	}
	c.paintFrameWith(l, r, rad, w.box.id, frameState{hover: hover, focus: w.Focused(), disabled: w.box.disabled && !w.bare, status: w.status, borderless: w.bare, variant: w.variant, bg: w.bg, noBorder: w.noBorder}, inputFrameTok(th))
	// Prefixes run from the inline start, suffixes (and the eye / clear
	// buttons) from the inline end; off is a logical offset from the start.
	s := c.inline(r)
	is := w.fontSize()
	iy := r.Y + (r.H-is)/2
	off := w.padX()
	iconCol := th.Text
	if w.box.disabled {
		iconCol = th.TextDisabled
	}
	st := w.font()
	if w.prefix != nil {
		c.drawIconAt(l, w.prefix, s.X(off, is), iy, is, iconCol)
		off += is + 4
	}
	if w.prefixText != "" {
		lay := text.Shape(w.prefixText, st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, iconCol)
	}
	off = r.W - w.padX() - w.countWidth()
	if w.suffixText != "" {
		lay := text.Shape(w.suffixText, st, text.Options{NoWrap: true})
		off -= lay.Width
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.TextTertiary)
		off -= 4
	}
	if w.suffix != nil {
		off -= is
		c.drawIconAt(l, w.suffix, s.X(off, is), iy, is, th.TextTertiary)
		off -= 4
	}
	if w.password {
		off -= is
		w.st.eyeRect = s.Rect(off, iy, is, is)
		ic := icon.EyeInvisible
		if w.st.showPwd {
			ic = icon.Eye
		}
		drawIcon(l, ic, w.st.eyeRect.X, iy, is, th.TextTertiary)
		off -= 4
	}
	if w.allowClear {
		off -= is
		w.st.clearRect = s.Rect(off, iy, is, is)
		if *w.val != "" && (hover || w.Focused()) && !w.box.disabled && th.Flavor == theme.StyleShadcn {
			entClearIcon(c, l, w.st.clearRect)
		} else if *w.val != "" && (hover || w.Focused()) && !w.box.disabled {
			cr := w.st.clearRect.Expand(-1)
			l.Path(paint.NewPath().Circle(cr.X+cr.W/2, cr.Y+cr.H/2, cr.W/2), th.TextQuaternary)
			s := cr.W / 24
			cx, cy := cr.X+cr.W/2, cr.Y+cr.H/2
			l.StrokePath(paint.NewPath().MoveTo(cx-4*s, cy-4*s).LineTo(cx+4*s, cy+4*s).MoveTo(cx+4*s, cy-4*s).LineTo(cx-4*s, cy+4*s), paint.StrokeStyle{Width: 2.2 * s, Cap: paint.CapRound}, th.BgContainer)
		}
	}
}

func (w *InputW) paintText(l *paint.List, r geom.Rect) {
	th := w.th
	col := th.Text
	if w.box.disabled {
		col = th.TextDisabled
	}
	l.Clip(r.Expand(1), geom.Radii{})
	w.c.entPaintEdit(l, r, w.ed.id, w.e, *w.val, w.font(), col, w.tk.PlaceholderColor, w.placeholder, w.password && !w.st.showPwd)
	l.Pop()
}

// ---------------------------------------------------------------------------

// InputNumberW is a numeric input with step controls.
type InputNumberW struct {
	_ [0]func()

	c          *Context
	th         *theme.Theme // Input theme (its Alias applied)
	nth        *theme.Theme // InputNumber theme (its Alias applied)
	tk         *theme.InputNumberToken
	box, ed    *node
	up, down   *node
	handles    *node
	val        *float64
	st         *numState
	min, max   float64
	step       float64
	precision  int
	disabled   bool
	size       ControlSize
	changed    bool
	done       bool
	ia, edIA   Interaction
	upIA, dnIA Interaction
	prefixTxt  string
	prefix     *icon.Icon

	empty       *bool // Empty: the field may hold no value
	placeholder string
	status      string

	formatter func(string) string // Formatter
	parser    func(string) string // Parser
	ctrlPos   string              // ControlsPosition: "sides", or "right" / "" (stacked at the right)
	wheel     bool                // Wheel
	sval      *string             // StringMode
	variant   string              // Variant
}

type numState struct {
	edit    editState
	buf     string
	editing bool
}

// InputNumber adds a numeric input bound to *val (val may be nil with
// StringMode).
func InputNumber(c *Context, val *float64) *InputNumberW {
	if val == nil {
		val = new(float64)
	}
	w := &InputNumberW{c: c, val: val, min: math.Inf(-1), max: math.Inf(1), step: 1, precision: -1, size: c.defaultSize(), status: c.itemStatus}
	w.nth = c.ThemeFor(theme.CompInputNumber)
	w.tk = &w.nth.Components.InputNumber
	tk := w.tk
	w.box = c.add("inputnumber", "")
	w.box.ln.Dir = layout.Row
	w.box.ln.Width = tk.ControlWidth
	w.st = State[numState](c, w.box.id, nil)
	w.ed = &node{id: hashID(w.box.id, "editor"), parent: w.box}
	w.ed.ln.Ref = w.ed
	w.ed.ln.Grow = 1
	w.ed.ln.Margin = geom.Insets{Left: 11, Right: 4}
	w.ed.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: 40, H: 22} }
	w.up = &node{id: hashID(w.box.id, "up"), parent: w.box}
	w.down = &node{id: hashID(w.box.id, "down"), parent: w.box}
	handles := &node{id: hashID(w.box.id, "handles"), parent: w.box}
	w.handles = handles
	handles.ln.Ref = handles
	handles.ln.Width = tk.HandleWidth
	w.up.ln.Grow, w.down.ln.Grow = 1, 1
	w.up.ln.Ref, w.down.ln.Ref = w.up, w.down
	handles.kids = []*node{w.up, w.down}
	w.box.kids = append(w.box.kids, w.ed, handles)
	w.ed.setRole(a11y.RoleSpinButton).fill(w)
	handles.aHidden = true // Increment / Decrement actions of the spin button
	w.ia = c.interact(w.box, false, input.CursorText)
	w.ed.ownKeys = true
	w.edIA = c.interact(w.ed, true, input.CursorText)
	w.upIA = c.interact(w.up, false, input.CursorPointer)
	w.dnIA = c.interact(w.down, false, input.CursorPointer)
	th := c.ThemeFor(theme.CompInput)
	w.th = th
	w.box.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(controlRadiusOf(th, w.size))
		if w.variant == "underlined" {
			rad = geom.Radii{}
		}
		c.paintFrameWith(l, r, rad, w.box.id, frameState{hover: w.ia.Hovered, focus: c.focus == w.ed.id, disabled: w.box.disabled, status: w.status, variant: w.variant}, inputFrameTok(th))
		w.paintPrefix(l, r)
	}
	w.ed.paint = func(l *paint.List, r geom.Rect) {
		col := th.Text
		if w.box.disabled {
			col = th.TextDisabled
		}
		st := c.style(inputFontSize(&th.Components.Input, w.size), text.Regular)
		s := w.display()
		er := r
		if w.ctrlPos == "sides" {
			// Element centres the value between the side buttons.
			show := s
			if show == "" {
				show = w.placeholder
			}
			if tw := text.Width(show, st) + 2; tw < r.W {
				er = geom.Rect{X: r.X + (r.W-tw)/2, Y: r.Y, W: tw, H: r.H}
			}
		}
		l.Clip(r, geom.Radii{})
		// Digits stay left to right (DirAuto finds no strong character);
		// in RTL the value is right-aligned like other inputs.
		c.entPaintEdit(l, er, w.ed.id, &w.st.edit, s, st, col, th.Components.Input.PlaceholderColor, w.placeholder, false)
		l.Pop()
	}
	handles.paint = func(l *paint.List, r geom.Rect) {
		if th.Flavor == theme.StyleShadcn {
			w.paintHandlesShadcn(l, r)
			return
		}
		a := c.Anim(w.box.id, "handles", b2f((w.ia.Hovered || c.focus == w.ed.id) && !w.box.disabled), th.DurationMid)
		if th.Flavor == theme.Element {
			// el-input-number controls-position="right": the buttons are
			// always shown (Ant fades them in on hover or focus).
			a = 1
		}
		if a <= 0.01 {
			return
		}
		l.Opacity(a)
		// The handles sit at the inline end; their divider faces the value.
		s := c.inline(r)
		if tk.HandleBg.A > 0 {
			// Inside the frame's 1px border, rounded at the outer corners.
			rr := max(controlRadiusOf(th, w.size)-1, 0)
			rad := geom.Radii{TR: rr, BR: rr}
			if c.IsRTL() {
				rad = geom.Radii{TL: rr, BL: rr}
			}
			l.Fill(s.Rect(1, r.Y+1, r.W-2, r.H-2), rad, tk.HandleBg)
		}
		l.Fill(s.Rect(0, r.Y+1, 1, r.H-2), geom.Radii{}, tk.HandleBorderColor)
		l.Fill(s.Rect(0, r.Y+r.H/2, r.W-1, 1), geom.Radii{}, tk.HandleBorderColor)
		is := tk.HandleFontSize
		for i := range 2 {
			// The step nodes are painted after this parent, so their abs
			// rects are not set yet this frame: split the handles' own rect
			// (the steps grow equally, up above down).
			hr := geom.Rect{X: r.X, Y: r.Y + float32(i)*r.H/2, W: r.W, H: r.H / 2}
			hov := (i == 0 && w.upIA.Hovered) || (i == 1 && w.dnIA.Hovered)
			col := tk.HandleColor
			if hov {
				if tk.HandleHoverBg.A > 0 {
					l.Fill(hr, geom.Radii{}, tk.HandleHoverBg)
				}
				col = tk.HandleHoverColor
			}
			if w.atLimit(i == 0) {
				col = tk.HandleDisabledColor
			}
			ic := icon.Up
			if i == 1 {
				ic = icon.Down
			}
			drawIcon(l, ic, hr.X+(hr.W-is)/2, hr.Y+(hr.H-is)/2, is, col)
		}
		l.Pop()
	}
	w.configDefaults()
	w.sizeDefaults()
	c.deferred = append(c.deferred, w.finish)
	return w
}

func (w *InputNumberW) Min(v float64) *InputNumberW      { w.min = v; return w }
func (w *InputNumberW) Max(v float64) *InputNumberW      { w.max = v; return w }
func (w *InputNumberW) Step(v float64) *InputNumberW     { w.step = v; return w }
func (w *InputNumberW) Precision(n int) *InputNumberW    { w.precision = n; return w }
func (w *InputNumberW) Width(px float32) *InputNumberW   { w.box.ln.Width = px; return w }
func (w *InputNumberW) Size(s ControlSize) *InputNumberW { w.size = s; return w }
func (w *InputNumberW) Changed() bool                    { w.finish(); return w.changed }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used).
func (w *InputNumberW) A11yLabel(s string) *InputNumberW { w.ed.setName(s); return w }

// Formatter formats the value for display ("$ 1,000"); the text being
// edited keeps the formatted form. Pair it with Parser.
func (w *InputNumberW) Formatter(fn func(value string) string) *InputNumberW {
	w.formatter = fn
	return w
}

// Parser turns the displayed text back into a plain number ("1000").
// Without one, a formatted field keeps only digits, '.', '-' and 'e'.
func (w *InputNumberW) Parser(fn func(display string) string) *InputNumberW {
	w.parser = fn
	return w
}

// Prefix shows an icon before the value.
func (w *InputNumberW) Prefix(ic *icon.Icon) *InputNumberW { w.prefix = ic; return w }

// PrefixText shows short static text before the value ("¥").
func (w *InputNumberW) PrefixText(s string) *InputNumberW { w.prefixTxt = s; return w }

// ControlsPosition places the step buttons: "right" (stacked at the right,
// Ant's look and Element's controls-position="right") or "sides" (− before
// and + after the value). The default follows the design system: "sides"
// under Element (el-input-number's default), "right" otherwise.
func (w *InputNumberW) ControlsPosition(pos string) *InputNumberW { w.ctrlPos = pos; return w }

// Wheel lets the mouse wheel step the value while the field has focus
// (Ant changeOnWheel).
func (w *InputNumberW) Wheel() *InputNumberW { w.wheel = true; return w }

// Variant sets the frame look (see InputW.Variant).
func (w *InputNumberW) Variant(v string) *InputNumberW { w.variant = v; return w }

// StringMode binds the value as a decimal string instead of a float64, so
// it keeps every digit ("0.1" + "0.2" = "0.3", 20-digit amounts; Ant
// stringMode). "" is an empty field. *val (if any) receives the nearest
// float64.
func (w *InputNumberW) StringMode(s *string) *InputNumberW { w.sval = s; return w }

// configDefaults applies ConfigProvider Defaults("InputNumber", ...):
// "controlsPosition", "variant" (string), "wheel" (bool).
func (w *InputNumberW) configDefaults() {
	c := w.c
	if w.ctrlPos == "" && w.nth.Flavor == theme.Element {
		w.ctrlPos = "sides" // el-input-number puts − and + at the sides by default
	}
	w.ctrlPos = propString(c, "InputNumber", "controlsPosition", w.ctrlPos)
	w.variant = propString(c, "InputNumber", "variant", w.variant)
	w.wheel = propBool(c, "InputNumber", "wheel", w.wheel)
}

func (w *InputNumberW) semantics(s *a11y.Node) {
	s.Placeholder = w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.isEmpty() {
		return
	}
	now := *w.val
	if w.sval != nil {
		if d, ok := parseDec(*w.sval); ok {
			now = d.float()
		}
	}
	s.HasRange, s.ValueNow, s.Step = true, now, w.step
	if !math.IsInf(w.min, 0) {
		s.ValueMin = w.min
	}
	if !math.IsInf(w.max, 0) {
		s.ValueMax = w.max
	}
	s.Value = w.display()
}

// Placeholder is shown while the field is empty (see Empty).
func (w *InputNumberW) Placeholder(s string) *InputNumberW { w.placeholder = s; return w }

// Status overrides the validation status ("error" | "warning"); inside a
// form item it defaults to the item's status.
func (w *InputNumberW) Status(s string) *InputNumberW { w.status = s; return w }

// Empty lets the field hold no value, like a nullable number: *empty
// reports whether it is empty. An empty field shows the placeholder and
// ignores *val; typing a number clears *empty, clearing the text sets it.
// The step buttons start an empty field from 0 (clamped to Min/Max).
// Without Empty, clearing the text and leaving keeps the previous value.
func (w *InputNumberW) Empty(empty *bool) *InputNumberW { w.empty = empty; return w }

func (w *InputNumberW) isEmpty() bool {
	if w.sval != nil && w.empty == nil {
		return *w.sval == ""
	}
	return w.empty != nil && *w.empty
}

// setEmpty marks the field empty (Empty mode, or StringMode's "").
func (w *InputNumberW) setEmpty() {
	if w.sval != nil && *w.sval != "" {
		*w.sval = ""
		w.changed = true
		w.c.invalid = true
	}
	if w.empty != nil && !*w.empty {
		*w.empty = true
		w.changed = true
		w.c.invalid = true
	}
}

// base is the value the step controls start from.
func (w *InputNumberW) base() float64 {
	if w.isEmpty() {
		return math.Max(w.min, math.Min(w.max, 0))
	}
	return *w.val
}

// text formats the current value for editing ("" when empty).
func (w *InputNumberW) text() string {
	if w.isEmpty() {
		return ""
	}
	if w.sval != nil {
		return w.formatDec(*w.sval)
	}
	return w.format(*w.val)
}

// shown is the value as displayed (formatted).
func (w *InputNumberW) shown() string {
	t := w.text()
	if w.formatter != nil && t != "" {
		return w.formatter(t)
	}
	return t
}

func (w *InputNumberW) Disabled(on bool) *InputNumberW {
	if on {
		w.box.disabled, w.ed.disabled, w.ed.focus = true, true, false
		w.up.disabled, w.down.disabled = true, true
	}
	return w
}

func (w *InputNumberW) format(v float64) string {
	if w.precision >= 0 {
		return strconv.FormatFloat(v, 'f', w.precision, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// formatDec formats a decimal string with the precision (StringMode).
func (w *InputNumberW) formatDec(s string) string {
	d, ok := parseDec(s)
	if !ok {
		return s
	}
	if w.precision >= 0 {
		return d.round(w.precision).fixed(w.precision)
	}
	return s
}

func (w *InputNumberW) display() string {
	if w.st.editing {
		return w.st.buf
	}
	return w.shown()
}

// parse turns typed text into a plain number string ("" for no number).
func (w *InputNumberW) parse(t string) string {
	switch {
	case w.parser != nil:
		t = w.parser(t)
	case w.formatter != nil:
		t = strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == 'e' || r == 'E' {
				return r
			}
			return -1
		}, t)
	}
	return strings.TrimSpace(t)
}

// atLimit reports a step button that cannot move further (up: at Max).
func (w *InputNumberW) atLimit(up bool) bool {
	if w.isEmpty() {
		return false
	}
	v := *w.val
	if w.sval != nil {
		if d, ok := parseDec(*w.sval); ok {
			v = d.float()
		}
	}
	if up {
		return !math.IsInf(w.max, 0) && v >= w.max
	}
	return !math.IsInf(w.min, 0) && v <= w.min
}

func (w *InputNumberW) set(v float64) {
	v = math.Max(w.min, math.Min(w.max, v))
	if w.precision >= 0 {
		p := math.Pow(10, float64(w.precision))
		v = math.Round(v*p) / p
	}
	if w.sval != nil {
		w.setDec(decFromFloat(v))
		return
	}
	if v != *w.val || w.isEmpty() {
		*w.val = v
		if w.empty != nil {
			*w.empty = false
		}
		w.changed = true
		w.c.invalid = true
	}
}

// setDec stores an exact decimal (StringMode), clamped and rounded.
func (w *InputNumberW) setDec(d dec) {
	if !math.IsInf(w.min, 0) {
		if lo := decFromFloat(w.min); d.cmp(lo) < 0 {
			d = lo
		}
	}
	if !math.IsInf(w.max, 0) {
		if hi := decFromFloat(w.max); d.cmp(hi) > 0 {
			d = hi
		}
	}
	s := d.String()
	if w.precision >= 0 {
		s = d.round(w.precision).fixed(w.precision)
	}
	if s != *w.sval || w.isEmpty() {
		*w.sval = s
		if w.empty != nil {
			*w.empty = false
		}
		w.changed = true
		w.c.invalid = true
	}
	*w.val = d.float()
}

// stepBy moves the value by n steps.
func (w *InputNumberW) stepBy(n float64) {
	if w.sval == nil {
		w.set(w.base() + n*w.step)
		return
	}
	cur, ok := parseDec(*w.sval)
	if !ok || w.isEmpty() {
		cur = decFromFloat(w.base())
	}
	st := decFromFloat(w.step)
	if n < 0 {
		st = st.neg()
	}
	w.setDec(cur.add(st))
}

func (w *InputNumberW) finish() {
	if w.done {
		return
	}
	w.done = true
	c := w.c
	w.box.ln.Height = controlHeightOf(w.th, w.size)
	w.layoutParts()
	if w.box.disabled {
		return
	}
	if w.upIA.Clicked {
		w.st.editing = false
		w.stepBy(1)
	}
	if w.dnIA.Clicked {
		w.st.editing = false
		w.stepBy(-1)
	}
	if w.ia.PressedNow && !w.edIA.PressedNow && !w.upIA.PressedNow && !w.dnIA.PressedNow {
		c.SetFocus(w.ed.id)
	}
	focused := c.focus == w.ed.id
	if focused && !w.st.editing {
		w.st.editing = true
		w.st.buf = w.shown()
	}
	if focused && w.wheel {
		if _, dy, ok := c.WheelFor(w.box.id); ok && dy != 0 {
			if dy < 0 {
				w.stepBy(1)
			} else {
				w.stepBy(-1)
			}
			w.st.buf = w.shown()
		}
	}
	if focused {
		for _, k := range c.keys {
			if k.Down && (k.Key == input.KeyUp || k.Key == input.KeyDown) {
				if k.Key == input.KeyDown {
					w.stepBy(-1)
				} else {
					w.stepBy(1)
				}
				w.st.buf = w.shown()
			}
		}
		var filter func(r rune) bool
		if w.formatter == nil && w.parser == nil {
			filter = func(r rune) bool { return (r >= '0' && r <= '9') || r == '.' || r == '-' }
		}
		res := c.processEdit(w.ed.id, &w.st.edit, &w.st.buf, editConfig{filter: filter}, w.edIA)
		if res.changed {
			w.typed(w.parse(w.st.buf))
		}
		if res.submitted {
			w.commit()
		}
	} else if w.st.editing {
		w.commit()
	}
}

// typed applies text being typed: a valid number inside Min/Max becomes
// the value at once (out-of-range text waits for commit).
func (w *InputNumberW) typed(t string) {
	if t == "" {
		w.setEmpty()
		return
	}
	if w.sval != nil {
		d, ok := parseDec(t)
		if !ok {
			return
		}
		if !math.IsInf(w.min, 0) && d.cmp(decFromFloat(w.min)) < 0 || !math.IsInf(w.max, 0) && d.cmp(decFromFloat(w.max)) > 0 {
			return
		}
		if t != *w.sval {
			*w.sval = t
			*w.val = d.float()
			if w.empty != nil {
				*w.empty = false
			}
			w.changed = true
		}
		return
	}
	if v, err := strconv.ParseFloat(t, 64); err == nil && v >= w.min && v <= w.max {
		if v != *w.val || w.isEmpty() {
			*w.val = v
			if w.empty != nil {
				*w.empty = false
			}
			w.changed = true
		}
	}
}

func (w *InputNumberW) commit() {
	t := w.parse(w.st.buf)
	switch {
	case t == "" && (w.empty != nil || w.sval != nil):
		w.setEmpty()
	case w.sval != nil:
		if d, ok := parseDec(t); ok {
			w.setDec(d)
		}
	default:
		if v, err := strconv.ParseFloat(t, 64); err == nil {
			w.set(v)
		}
	}
	w.st.editing = w.c.focus == w.ed.id
	w.st.buf = w.shown()
}
