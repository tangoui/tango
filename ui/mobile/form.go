package mobile

import (
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// Form

type formStack struct {
	vertical []bool
	items    []formField // enclosing FormItems (innermost last)
}

// formField is the label of a FormItem for the control inside it.
type formField struct {
	label    string
	required bool
	err      string
	used     bool
}

// fieldLabel returns the label of the enclosing FormItem for the first
// control that asks (ok false outside a FormItem or once taken).
func fieldLabel(c *ui.Context) (f formField, ok bool) {
	fs := ui.State[formStack](c, keyForm, nil)
	n := len(fs.items)
	if n == 0 || fs.items[n-1].used {
		return f, false
	}
	fs.items[n-1].used = true
	return fs.items[n-1], true
}

// fieldStates returns Required / Invalid for a labelled control.
func (f formField) states() a11y.State {
	return stateIf(f.required, a11y.Required) | stateIf(f.err != "", a11y.Invalid)
}

var keyForm = ui.ID(0x6d6f62696c650004)

// FormW lays out labelled fields in a white group, labels on the left
// (default) or on top.
type FormW struct {
	c        *ui.Context
	th       *Theme
	vertical bool
	header   string
	card     bool
}

// Form starts a mobile form; add FormItems in Body.
//
// ConfigProvider defaults ("mobile.Form"): "layout" ("horizontal" or
// "vertical"), "mode" ("default" or "card").
func Form(c *ui.Context) *FormW {
	w := &FormW{c: c, th: ThemeOf(c)}
	if v, ok := cfgStr(c, "Form", "layout"); ok {
		w.vertical = v == "vertical"
	}
	if v, ok := cfgStr(c, "Form", "mode"); ok {
		w.card = v == "card"
	}
	return w
}

// Vertical puts labels above their fields.
func (w *FormW) Vertical() *FormW { w.vertical = true; return w }

// Header shows a group caption.
func (w *FormW) Header(s string) *FormW { w.header = s; return w }

// Card insets the group with rounded corners.
func (w *FormW) Card() *FormW { w.card = true; return w }

// Body builds the items.
func (w *FormW) Body(fn func()) {
	fs := ui.State[formStack](w.c, keyForm, nil)
	fs.vertical = append(fs.vertical, w.vertical)
	l := List(w.c, w.header)
	l.role = a11y.RoleForm
	if w.card {
		l.Card()
	}
	l.Body(fn)
	fs.vertical = fs.vertical[:len(fs.vertical)-1]
}

// FormItemW is one labelled field of a Form.
type FormItemW struct {
	c        *ui.Context
	th       *Theme
	label    string
	required bool
	errMsg   string
	help     string
	arrow    bool
	pr       *Press
}

// FormItem starts a labelled field; build the control in Body.
func FormItem(c *ui.Context, label string) *FormItemW {
	return &FormItemW{c: c, th: ThemeOf(c), label: label}
}

// Required marks the label with an asterisk.
func (w *FormItemW) Required() *FormItemW { w.required = true; return w }

// Error shows a validation message under the field ("" for none).
func (w *FormItemW) Error(msg string) *FormItemW { w.errMsg = msg; return w }

// Help shows a hint under the field.
func (w *FormItemW) Help(s string) *FormItemW { w.help = s; return w }

// Arrow shows a navigation arrow (pickers opened from the row).
func (w *FormItemW) Arrow() *FormItemW { w.arrow = true; return w }

// Body builds the field control.
func (w *FormItemW) Body(fn func()) *FormItemW {
	c, th := w.c, w.th
	fs := ui.State[formStack](c, keyForm, nil)
	vertical := len(fs.vertical) > 0 && fs.vertical[len(fs.vertical)-1]
	lf := th.Font(th.FontBody, text.Regular)
	if th.shad() {
		lf = th.Font(th.FontMain, text.Medium) // shadcn Label: text-sm font-medium
	}
	// A label above its field starts at the left in the shadcn style
	// (the other styles keep their centred placement).
	labelAlign := layout.AlignCenter
	if vertical && th.shad() {
		labelAlign = layout.AlignStart
	}
	rtl := c.IsRTL()
	label := func(width float32) {
		req := w.required
		s := w.label
		lb := ui.Custom(c, func(_, _ float32) geom.Size {
			if width > 0 {
				return geom.Size{W: width, H: lf.LineHeight}
			}
			return geom.Size{W: textW(s, lf) + 10, H: lf.LineHeight}
		}, func(l *paint.List, r geom.Rect) {
			// The label starts at the start edge of its box (the right in
			// RTL), after the required asterisk.
			sp := spanOf(r, rtl)
			x := r.X
			if req {
				x += sp.text(l, "*", lf, x, r.Y, th.Danger) + 2
			}
			col := th.Text
			if vertical && !th.shad() {
				col = th.TextSecondary
			}
			sp.text(l, s, lf, x, r.Y, col)
		}, layout.Self(labelAlign))
		if w.arrow {
			lb.Semantics(a11y.RoleText, s) // part of the row button's name
		}
	}
	padV := th.CellPadV
	b := ui.Div(c, layout.Padding(geom.Insets{Top: padV, Bottom: padV, Left: th.PadH, Right: th.PadH}), layout.MinH(th.CellMinHeight), layout.Gap(4)).Bg(th.Box)
	// An Arrow row opens a picker: a button named by its label and value
	// (the texts inside). Otherwise a group named by the label; the
	// control inside takes the label as its name (fieldLabel).
	mode := touchPassive
	if w.arrow {
		mode = touchFocus
		b.Semantics(a11y.RoleButton, "").A11yState(a11y.HasPopup | stateIf(w.required, a11y.Required))
	} else {
		b.Semantics(a11y.RoleGroup, w.label)
		if w.errMsg != "" && c.A11yEnabled() {
			msg := w.errMsg
			b.A11y(func(s *a11y.Node) { s.Description = msg })
		}
	}
	pr := touchAs(c, b, mode)
	w.pr = &pr
	arrow := w.arrow
	fs.items = append(fs.items, formField{label: w.label, required: w.required, err: w.errMsg, used: w.arrow})
	defer func() { fs.items = fs.items[:len(fs.items)-1] }()
	b.Paint(func(l *paint.List, r geom.Rect) {
		if arrow && pr.Pressed {
			l.Fill(r, geom.Radii{}, th.Press)
		}
		spanOf(r, rtl).hairlineTop(l, r, r.X+th.PadH, th.Border)
	})
	b.Body(func() {
		row := func() {
			ui.Div(c, layout.Grow(1), layout.Self(layout.AlignCenter)).Body(fn)
			if arrow {
				ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 18, H: 18} }, func(l *paint.List, r geom.Rect) {
					col := th.TextLight
					if th.Style == Vant || th.shad() {
						col = th.TextWeak
					}
					spanOf(r, rtl).icon(l, icon.Right, r.X, r.Y, 18, col) // points left in RTL
				}, layout.Self(layout.AlignCenter))
			}
		}
		if vertical {
			label(0)
			ui.Row(c, row, layout.Gap(8))
		} else {
			ui.Row(c, func() {
				label(th.FormLabelWidth)
				row()
			}, layout.Gap(8), layout.Middle())
		}
		msg, col := w.errMsg, th.Danger
		if msg == "" {
			msg, col = w.help, th.TextWeak
		}
		if msg != "" {
			left := float32(0)
			if !vertical {
				left = th.FormLabelWidth + 8
			}
			ui.Text(c, msg).Size(th.FontSmall).Color(col).Layout(layout.Margin(geom.Insets{Left: left}))
		}
	})
	return w
}

// Clicked reports a tap on the row (open a picker from an Arrow item).
func (w *FormItemW) Clicked() bool { return w.pr != nil && w.pr.Tap }

// ---------------------------------------------------------------------------
// Field (borderless text input)

// FieldW is a borderless single-line text input for forms and cells.
type FieldW struct {
	in *ui.InputW
}

// Field adds a borderless text input bound to *val. It reuses ui.Input
// (editing, IME, clipboard) and hides its frame.
//
// ConfigProvider defaults ("mobile.Field"): "clearable" (bool),
// "maxLength" (int), "placeholder" (string).
func Field(c *ui.Context, val *string) *FieldW {
	th := ThemeOf(c)
	size, h, pad := ui.Large, float32(40), float32(11)
	if th.Style == Vant {
		size, h = ui.Middle, 32
	}
	line := th.Font(th.FontBody, text.Regular).LineHeight
	var in *ui.InputW
	ui.Div(c, layout.H(line), layout.Grow(1)).Clip().Body(func() {
		inset := (h - line) / 2
		in = ui.Input(c, val).Size(size).Layout(layout.Abs(-pad-1, -inset, -pad-1, -inset))
		if th.shad() {
			in.Bg(th.Box) // the field sits on the cell (lifted in dark mode)
		}
	})
	if f, ok := fieldLabel(c); ok {
		in.A11yLabel(f.label)
		c.Semantics(in.ID(), a11y.RoleNone, "", f.states())
	}
	if v, ok := cfgBool(c, "Field", "clearable"); ok && v {
		in.AllowClear()
	}
	if v, ok := cfgNum(c, "Field", "maxLength"); ok && v > 0 {
		in.MaxLength(int(v))
	}
	if v, ok := cfgStr(c, "Field", "placeholder"); ok {
		in.Placeholder(v)
	}
	return &FieldW{in: in}
}

// Placeholder sets the hint text.
func (w *FieldW) Placeholder(s string) *FieldW { w.in.Placeholder(s); return w }

// Password masks the text.
func (w *FieldW) Password() *FieldW { w.in.Password(); return w }

// Type tells the soft keyboard what the field holds (input.TextNumber,
// input.TextEmail, input.TextPhone, …): its layout and suggestions.
// input.TextPassword also masks the text.
func (w *FieldW) Type(k input.TextKind) *FieldW {
	if k == input.TextPassword {
		w.in.Password()
	}
	w.in.Kind(k)
	return w
}

// EnterKey labels the soft keyboard's return key (input.EnterNext,
// input.EnterDone, …); pressing it still reports Submitted.
func (w *FieldW) EnterKey(k input.EnterKey) *FieldW { w.in.EnterKey(k); return w }

// MaxLength limits the length.
func (w *FieldW) MaxLength(n int) *FieldW { w.in.MaxLength(n); return w }

// Changed reports an edit this frame.
func (w *FieldW) Changed() bool { return w.in.Changed() }

// Submitted reports Enter.
func (w *FieldW) Submitted() bool { return w.in.Submitted() }

// Input returns the underlying ui.InputW.
func (w *FieldW) Input() *ui.InputW { return w.in }

// ---------------------------------------------------------------------------
// SearchBar

// SearchBarW is a rounded search field with an optional cancel action.
type SearchBarW struct {
	c         *ui.Context
	th        *Theme
	in        *ui.InputW
	cancelled bool
}

// SearchBar adds a search field bound to *q. placeholder "" uses the
// localized "搜索". Pass a non-empty cancel text to show a cancel action
// while the field has text. In the shadcn style the field is a muted,
// borderless rounded-lg input on the bar background with a ring on focus.
//
// ConfigProvider defaults ("mobile.SearchBar"): "placeholder" (string,
// used when placeholder is ""), "cancelText" (string, used when cancel is
// ""), "shape" ("round" or "default").
func SearchBar(c *ui.Context, q *string, placeholder, cancel string) *SearchBarW {
	th := ThemeOf(c)
	w := &SearchBarW{c: c, th: th}
	vant := th.Style == Vant
	shad := th.shad()
	h := th.SearchHeight
	pad := geom.Insets{Top: 8, Bottom: 8, Left: th.PadH, Right: th.PadH}
	if vant {
		pad = geom.Insets{Top: 10, Bottom: 10, Left: 12, Right: 12}
	}
	if placeholder == "" {
		placeholder, _ = cfgStr(c, "SearchBar", "placeholder")
	}
	if cancel == "" {
		cancel, _ = cfgStr(c, "SearchBar", "cancelText")
	}
	round := vant
	if v, ok := cfgRound(c, "SearchBar"); ok {
		round = v
	}
	ph := or(placeholder, c.Locale().Search)
	rtl := c.IsRTL()
	bar := ui.Row(c, func() {
		rad := th.SearchRadius
		if round {
			rad = h / 2
		}
		field := ui.Div(c, layout.H(h), layout.Grow(1)).Clip().Radius(rad)
		field.Body(func() {
			if shad {
				// A muted field without a frame; the ring below marks focus.
				w.in = ui.Input(c, q).Prefix(icon.Search).Placeholder(ph).AllowClear().Bg(th.Fill).NoBorder().Layout(layout.Abs(0, 0, 0, 0))
			} else {
				// ui.Input draws its own white field; clip away its border.
				w.in = ui.Input(c, q).Prefix(icon.Search).Placeholder(ph).AllowClear().Layout(layout.Abs(-1, -1, -1, -1))
			}
			c.Semantics(w.in.ID(), a11y.RoleSearchbox, "")
		})
		if shad && c.Focused() == w.in.ID() {
			ring := th.Ring
			field.PaintOver(func(l *paint.List, r geom.Rect) {
				l.Stroke(r.Inset(geom.All(1)), geom.Rad(max(0, rad-1)), 2, ring)
			})
		}
		if cancel != "" && *q != "" {
			fs := th.Font(th.FontMain, text.Regular)
			if shad {
				fs.Weight = text.Medium
			}
			b := ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: textW(cancel, fs) + 12, H: h} }, nop)
			b.Semantics(a11y.RoleButton, cancel)
			pr := Touchable(c, b)
			b.Paint(func(l *paint.List, r geom.Rect) {
				col := th.Primary
				if vant || shad {
					col = th.Text
				}
				if pr.Pressed {
					col = fadeCol(col, 0.6)
				}
				spanOf(r, rtl).textRight(l, cancel, fs, r.Right(), r, col)
			})
			if pr.Tap {
				*q = ""
				w.cancelled = true
				c.Invalidate()
			}
		}
	}, layout.Padding(pad), layout.Gap(8), layout.Middle())
	if shad {
		bar.Bg(th.Box).PaintOver(func(l *paint.List, r geom.Rect) { hairline(l, r, r.X, th.Border) })
	} else {
		bar.Bg(th.PageBg)
	}
	return w
}

// Changed reports an edit this frame.
func (w *SearchBarW) Changed() bool { return w.in.Changed() || w.cancelled }

// Submitted reports Enter (search).
func (w *SearchBarW) Submitted() bool { return w.in.Submitted() }

// Cancelled reports a tap on the cancel action.
func (w *SearchBarW) Cancelled() bool { return w.cancelled }

// ---------------------------------------------------------------------------
// Stepper

// StepperW is a − value + control.
type StepperW struct {
	c        *ui.Context
	th       *Theme
	b        *ui.Box
	v        *float64
	min, max float64
	step     float64
	digits   int
	disabled bool
	pr       Press
	changed  bool
	done     bool
	st       *stepperState
	rtl      bool // − at the right, + at the left
}

type stepperState struct {
	pending bool // a tap waiting for the bounds of the final build pass
	pos     geom.Point
	steps   int // keyboard / screen reader steps waiting for the bounds
	set     bool
	setTo   float64 // a value set by a screen reader (SetValue)
}

// Stepper adds a stepper bound to *v (step 1, no bounds). Set options,
// then read Changed (applying the tap happens on the first query or at
// paint, whichever comes first, so bounds always apply). In the shadcn
// style it is an outlined button group (h-10) with ghost − / + buttons.
//
// ConfigProvider defaults ("mobile.Stepper"): "step", "min", "max",
// "digits" (numbers), "disabled" (bool).
func Stepper(c *ui.Context, v *float64) *StepperW {
	th := ThemeOf(c)
	w := &StepperW{c: c, th: th, v: v, min: math.Inf(-1), max: math.Inf(1), step: 1, rtl: c.IsRTL()}
	if x, ok := cfgNum(c, "Stepper", "step"); ok && x > 0 {
		w.step = x
	}
	if x, ok := cfgNum(c, "Stepper", "min"); ok {
		w.min = x
	}
	if x, ok := cfgNum(c, "Stepper", "max"); ok {
		w.max = x
	}
	if x, ok := cfgNum(c, "Stepper", "digits"); ok {
		w.digits = int(x)
	}
	if x, ok := cfgBool(c, "Stepper", "disabled"); ok {
		w.disabled = x
	}
	m := newMirror(c, false)
	w.b = ui.Custom(c, w.measure, w.paint)
	// A spin button: Up / Down step, and so do the arrow key towards +
	// (Right; Left in RTL, where + is on the left) and the one towards −;
	// Home / End jump to the bounds; the − and + zones are exposed as
	// buttons.
	name := ""
	f, labelled := fieldLabel(c)
	if labelled {
		name = f.label
	}
	w.b.Semantics(a11y.RoleSpinButton, name).A11yState(f.states())
	if c.A11yEnabled() {
		w.b.A11y(w.semantics)
	}
	w.pr = Touchable(c, w.b)
	w.st = ui.State[stepperState](c, w.b.ID(), nil)
	if w.pr.Tap && !w.pr.KeyTap {
		w.st.pending, w.st.pos = true, w.pr.TapPos
	}
	id := w.b.ID()
	if act, arg, ok := c.A11yAction(id); ok {
		switch act {
		case a11y.Increment:
			w.st.steps++
		case a11y.Decrement:
			w.st.steps--
		case a11y.SetValue:
			if x, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil {
				w.st.set, w.st.setTo = true, x
			}
		}
		keyPresses(c, id) // the action's synthesized keys
	} else if c.Focused() == id {
		for _, k := range keyPresses(c, id) {
			if dec, inc := inlineKey(w.rtl, k); inc {
				w.st.steps++
			} else if dec {
				w.st.steps--
			}
			switch k {
			case input.KeyUp:
				w.st.steps++
			case input.KeyDown:
				w.st.steps--
			case input.KeyHome:
				w.st.steps = math.MinInt32
			case input.KeyEnd:
				w.st.steps = math.MaxInt32
			}
		}
	}
	if m != nil {
		r, _ := c.RectOf(id)
		minus, _, plus := w.geom(geom.Rect{W: r.W, H: r.H})
		m.beside(w.b, a11y.RoleNone, "", 0)
		loc := c.Locale()
		if m.part(minus, a11y.RoleButton, loc.A11y.Decrease, 0, false).tap {
			w.st.steps--
		}
		if m.part(plus, a11y.RoleButton, loc.A11y.Increase, 0, false).tap {
			w.st.steps++
		}
	}
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the label is used).
func (w *StepperW) A11yLabel(s string) *StepperW { w.b.Semantics(a11y.RoleSpinButton, s); return w }

func (w *StepperW) semantics(s *a11y.Node) {
	s.HasRange, s.ValueNow, s.Step = true, *w.v, w.step
	if !math.IsInf(w.min, 0) {
		s.ValueMin = w.min
	}
	if !math.IsInf(w.max, 0) {
		s.ValueMax = w.max
	}
	s.Value = w.format()
	if w.disabled {
		s.States |= a11y.Disabled
		s.Actions &^= a11y.Increment | a11y.Decrement | a11y.SetValue
	} else {
		stepSem(s)
		s.Actions |= a11y.SetValue
	}
}

func (w *StepperW) format() string {
	s := strconv.FormatFloat(*w.v, 'f', w.digits, 64)
	if w.digits == 0 && *w.v != math.Trunc(*w.v) {
		s = strconv.FormatFloat(*w.v, 'f', -1, 64)
	}
	return s
}

// Min sets the lower bound.
func (w *StepperW) Min(v float64) *StepperW { w.min = v; return w }

// Max sets the upper bound.
func (w *StepperW) Max(v float64) *StepperW { w.max = v; return w }

// Step sets the increment.
func (w *StepperW) Step(s float64) *StepperW { w.step = s; return w }

// Digits fixes the decimals shown.
func (w *StepperW) Digits(n int) *StepperW { w.digits = n; return w }

// Disabled disables the control.
func (w *StepperW) Disabled(on bool) *StepperW { w.disabled = on; return w }

// Changed reports a step this frame.
func (w *StepperW) Changed() bool { w.apply(); return w.changed }

// geom returns the physical parts in r: − value + from the start edge.
func (w *StepperW) geom(r geom.Rect) (minus, val, plus geom.Rect) {
	minus, val, plus = w.geomLTR(r)
	sp := spanOf(r, w.rtl)
	return sp.R(minus), sp.R(val), sp.R(plus)
}

// stepperBox spans the − and + buttons (in either order).
func stepperBox(minus, plus geom.Rect) geom.Rect {
	x0, x1 := min(minus.X, plus.X), max(minus.Right(), plus.Right())
	return geom.Rect{X: x0, Y: minus.Y, W: x1 - x0, H: minus.H}
}

// geomLTR lays the parts out left to right.
func (w *StepperW) geomLTR(r geom.Rect) (minus, val, plus geom.Rect) {
	th := w.th
	bh := th.StepperHeight
	y := r.Y + (r.H-bh)/2
	gap := float32(0)
	if th.Style == Vant {
		gap = 2
	}
	minus = geom.Rect{X: r.X, Y: y, W: bh, H: bh}
	val = geom.Rect{X: minus.Right() + gap, Y: y, W: th.StepperInputW, H: bh}
	plus = geom.Rect{X: val.Right() + gap, Y: y, W: bh, H: bh}
	return
}

func (w *StepperW) apply() {
	if w.done {
		return
	}
	w.done = true
	steps, set := w.st.steps, w.st.set
	w.st.steps, w.st.set = 0, false
	if !w.st.pending && steps == 0 && !set {
		return
	}
	pending := w.st.pending
	w.st.pending = false
	if w.disabled {
		return
	}
	nv := *w.v
	switch {
	case set:
		nv = w.st.setTo
	case steps == math.MinInt32 && !math.IsInf(w.min, 0):
		nv = w.min
	case steps == math.MaxInt32 && !math.IsInf(w.max, 0):
		nv = w.max
	case steps == math.MinInt32 || steps == math.MaxInt32:
		return
	case steps != 0:
		nv += float64(steps) * w.step
	case pending:
		r, ok := w.c.RectOf(w.b.ID())
		if !ok {
			return
		}
		minus, _, plus := w.geomLTR(r)
		p := spanOf(r, w.rtl).Pt(w.st.pos) // logical
		switch {
		case p.X < minus.Right()+4:
			nv -= w.step
		case p.X > plus.X-4:
			nv += w.step
		default:
			return
		}
	}
	nv = max(w.min, min(nv, w.max))
	if w.digits >= 0 {
		k := math.Pow(10, float64(w.digits))
		nv = math.Round(nv*k) / k
	}
	if nv != *w.v {
		*w.v = nv
		w.changed = true
		w.c.Invalidate()
	}
}

func (w *StepperW) measure(_, _ float32) geom.Size {
	th := w.th
	gap := float32(0)
	if th.Style == Vant {
		gap = 4
	}
	return geom.Size{W: 2*th.StepperHeight + th.StepperInputW + gap, H: th.TouchMin}
}

func (w *StepperW) paint(l *paint.List, r geom.Rect) {
	w.apply()
	th := w.th
	if th.shad() {
		w.paintShad(l, r)
		return
	}
	vant := th.Style == Vant
	minus, val, plus := w.geom(r)
	rad := geom.Rad(2)
	if vant {
		rad = geom.Rad(4)
	}
	canDec, canInc := *w.v-w.step >= w.min-1e-9, *w.v+w.step <= w.max+1e-9
	btn := func(br geom.Rect, ic *icon.Icon, enabled bool, rr geom.Radii) {
		l.Fill(br, rr, th.Fill)
		col := th.Primary
		if vant {
			col = th.Text
		}
		if !enabled || w.disabled {
			col = th.TextLight
		} else if w.pr.Pressed && br.Expand(4).Contains(w.pr.PressPos) {
			l.Fill(br, rr, th.Press)
		}
		drawIcon(l, ic, br.X+(br.W-14)/2, br.Y+(br.H-14)/2, 14, col)
	}
	// The outer corners of − and + face the start and end edges.
	sp := spanOf(r, w.rtl)
	if vant {
		btn(minus, icon.Minus, canDec, sp.Radii(geom.Radii{TL: rad.TL, BL: rad.BL}))
		btn(plus, icon.Plus, canInc, sp.Radii(geom.Radii{TR: rad.TR, BR: rad.BR}))
		l.Fill(val, geom.Radii{}, th.Fill)
	} else {
		l.Fill(stepperBox(minus, plus), rad, th.Fill)
		btn(minus, icon.Minus, canDec, rad)
		btn(plus, icon.Plus, canInc, rad)
	}
	s := w.format()
	fs := th.Font(th.FontMain, text.Regular)
	col := th.Text
	if w.disabled {
		col = th.TextLight
	}
	drawTextCenter(l, s, fs, val, col)
}

// paintShad draws the shadcn stepper: one outlined rounded-md group, the
// buttons separated by vertical borders, pressed buttons get the accent.
func (w *StepperW) paintShad(l *paint.List, r geom.Rect) {
	th := w.th
	minus, val, plus := w.geom(r)
	box := stepperBox(minus, plus)
	sp := spanOf(r, w.rtl)
	rad := th.Radius
	l.Shadows(box, geom.Rad(rad), th.Base.InputShadow)
	l.Fill(box, geom.Rad(rad), th.Box)
	canDec, canInc := *w.v-w.step >= w.min-1e-9, *w.v+w.step <= w.max+1e-9
	btn := func(br geom.Rect, ic *icon.Icon, enabled bool, rr geom.Radii) {
		col := th.Text
		if !enabled || w.disabled {
			col = th.TextLight
		} else if w.pr.Pressed && br.Expand(4).Contains(w.pr.PressPos) {
			l.Fill(br, rr, th.Fill)
		}
		drawIcon(l, ic, br.X+(br.W-16)/2, br.Y+(br.H-16)/2, 16, col)
	}
	btn(minus, icon.Minus, canDec, sp.Radii(geom.Radii{TL: rad, BL: rad}))
	btn(plus, icon.Plus, canInc, sp.Radii(geom.Radii{TR: rad, BR: rad}))
	l.Fill(geom.Rect{X: val.X, Y: val.Y, W: 1, H: val.H}, geom.Radii{}, th.Border)
	l.Fill(geom.Rect{X: val.Right() - 1, Y: val.Y, W: 1, H: val.H}, geom.Radii{}, th.Border)
	l.Stroke(box, geom.Rad(rad), 1, th.CardBorder)
	col := th.Text
	if w.disabled {
		col = th.TextLight
	}
	drawTextCenter(l, w.format(), th.Font(th.FontMain, text.Medium), val, col)
}
