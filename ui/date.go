package ui

import (
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

// DatePickerW is an input that opens a date panel (days, or weeks,
// months, quarters or years with Picker).
type DatePickerW struct {
	_ [0]func()

	c           *Context
	n           *node
	val         *time.Time
	st          *dpState
	loc         *Locale // captured at build (paint runs outside ConfigProvider)
	placeholder string
	phSet       bool
	format      string
	formatSet   bool
	picker      PickerType
	disabledFn  func(time.Time) bool
	presets     []DatePreset
	needConfirm bool
	allowClear  bool
	status      string
	changed     bool
	ia          Interaction
	z           *lazy
	disabled    bool
	size        ControlSize
	variant     string
	ids         dtGridIDs // grid and active cell of the open panel (semantics)
}

type dpState struct {
	open   bool
	panel  ID
	fp     fillDPState // panel views (shared with DatePickerPanel)
	clearR geom.Rect   // clear button of the last paint
	ty     dtTyping    // typed text (committed only when it parses)
	draft  time.Time   // NeedConfirm: the picked date until OK
}

// DatePicker adds a date input bound to *val. Picked values are UTC
// midnights (with Picker: the first day of the week, month, quarter or
// year).
//
// The value can also be typed: printable keys edit the text in place and
// open the panel, which follows a valid date; Enter (or leaving the field)
// commits it, invalid or disabled text is dropped.
//
// ConfigProvider.Defaults("DatePicker", ...) keys: "format",
// "placeholder", "picker" (PickerType), "allowClear", "width", "size",
// "variant".
func DatePicker(c *Context, val *time.Time) *DatePickerW {
	w := &DatePickerW{c: c, val: val, loc: c.Locale(), placeholder: c.Locale().SelectDate, format: "2006-01-02", size: c.defaultSize(), allowClear: true, status: c.itemStatus}
	w.n = c.add("datepicker", "")
	w.st = State[dpState](c, w.n.id, nil)
	def := p2aDefaults(c, "DatePicker")
	w.formatSet = def.str("format", &w.format)
	w.phSet = def.str("placeholder", &w.placeholder)
	if v, ok := c.ConfigProp("DatePicker", "picker"); ok {
		if p, ok := v.(PickerType); ok {
			w.picker = p
		}
	}
	def.flag("allowClear", &w.allowClear)
	def.px("width", &w.n.ln.Width)
	def.size(&w.size)
	def.str("variant", &w.variant)
	w.n.ownKeys = true
	w.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(w)
	w.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: 180, H: c.controlHeight(w.size)} }
	w.n.ln.NoStretch = true
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.n.paint = w.paint
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *DatePickerW) A11yLabel(s string) *DatePickerW { w.n.setName(s); return w }

// semantics: the value, and while open the panel grid (Controls) and the
// cell of the keyboard cursor (ActiveDescendant). Arrow keys move the
// cursor, PageUp / PageDown change the month (the year / decade in coarse
// views), Ctrl/Cmd+Up zooms out to months / years, Enter picks or drills
// down, Escape closes.
func (w *DatePickerW) semantics(s *a11y.Node) {
	ph := w.ph()
	s.Placeholder = ph
	if !w.shown().IsZero() {
		s.Value = w.display()
	}
	if s.Name == "" {
		s.Name = ph
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.st.open {
		semExpanded(s)
		s.Controls = a11y.NodeID(w.ids.grid)
		s.ActiveDescendant = a11y.NodeID(w.ids.active)
	}
}

func (w *DatePickerW) Placeholder(s string) *DatePickerW { w.placeholder, w.phSet = s, true; return w }

// Format sets the Go layout of the value (date, month and year pickers;
// week and quarter values use the Locale labels "2026-46th" / "2026-Q4").
func (w *DatePickerW) Format(f string) *DatePickerW    { w.format, w.formatSet = f, true; return w }
func (w *DatePickerW) Size(s ControlSize) *DatePickerW { w.size = s; return w }

// Picker sets the granularity: PickerDate (default), PickerWeek,
// PickerMonth, PickerQuarter or PickerYear (Ant picker, Element type).
// Clicking the panel titles switches between the year, month and day views.
func (w *DatePickerW) Picker(p PickerType) *DatePickerW { w.picker = p; return w }

// Presets adds a side list of named dates (DatePreset.Value) that pick
// immediately.
func (w *DatePickerW) Presets(p ...DatePreset) *DatePickerW { w.presets = p; return w }

// NeedConfirm keeps a picked date as a draft until the footer's OK button
// (or Enter on the picked cell, or Enter after typing) commits it (Ant
// needConfirm); closing the panel otherwise drops the draft. Presets still
// commit immediately.
func (w *DatePickerW) NeedConfirm(on bool) *DatePickerW { w.needConfirm = on; return w }

// AllowClear toggles the clear button shown on hover (on by default).
func (w *DatePickerW) AllowClear(on bool) *DatePickerW { w.allowClear = on; return w }

// Status sets the validation look: "error" or "warning".
func (w *DatePickerW) Status(s string) *DatePickerW { w.status = s; return w }

// Variant sets the frame look: "outlined" (default), "filled",
// "borderless" or "underlined" (Ant variant).
func (w *DatePickerW) Variant(v string) *DatePickerW { w.variant = v; return w }

// Width sets the input width in pixels (default 180).
func (w *DatePickerW) Width(px float32) *DatePickerW { w.n.ln.Width = px; return w }

// Layout applies layout options to the input.
func (w *DatePickerW) Layout(o ...layout.Option) *DatePickerW {
	flexbox.Apply(&w.n.ln.Style, o...)
	return w
}

func (w *DatePickerW) Changed() bool { w.z.run(); return w.changed }

// Open forces the panel open (screenshots, tests).
func (w *DatePickerW) Open() *DatePickerW {
	if !w.st.open {
		w.st.open = true
		w.st.fp.has = false
		w.st.draft = *w.val
	}
	return w
}

// IsOpen reports whether the panel is shown.
func (w *DatePickerW) IsOpen() bool { w.z.run(); return w.st.open }

// DisabledDate marks dates that cannot be picked (with Picker: called with
// the first day of the period).
func (w *DatePickerW) DisabledDate(fn func(time.Time) bool) *DatePickerW {
	w.disabledFn = fn
	return w
}
func (w *DatePickerW) Disabled(on bool) *DatePickerW {
	if on {
		w.disabled, w.n.disabled, w.n.focus = true, true, false
	}
	return w
}

// ph is the placeholder shown: explicit, else per granularity.
func (w *DatePickerW) ph() string {
	if w.phSet {
		return w.placeholder
	}
	return p2aPickerPlaceholder(w.loc, w.picker)
}

// layout is the Go layout of the value.
func (w *DatePickerW) layout() string {
	if w.formatSet {
		return w.format
	}
	return p2aPickerLayout(w.picker)
}

// shown is the value the input shows: the draft while a NeedConfirm
// panel is open.
func (w *DatePickerW) shown() time.Time {
	if w.needConfirm && w.st.open {
		return w.st.draft
	}
	return *w.val
}

// display formats the value for the input.
func (w *DatePickerW) display() string {
	return p2aFormatPeriod(w.loc, w.shown(), w.picker, w.layout(), w.loc.WeekStart)
}

// core returns the panel engine of DatePickerPanel over the picker state.
func (w *DatePickerW) core(pick func(time.Time)) *DatePickerPanelW {
	val := w.val
	if w.needConfirm {
		val = &w.st.draft
	}
	return &DatePickerPanelW{c: w.c, val: val, picker: w.picker, disabled: w.disabledFn, st: &w.st.fp, ids: &w.ids, onPick: pick}
}

func (w *DatePickerW) finish() {
	c, st := w.c, w.st
	if w.disabled || w.n.disabled {
		st.open = false
		return
	}
	setOpen := func(on bool) {
		if on && !st.open {
			st.fp.has = false
			st.draft = *w.val
		}
		st.open = on
		c.invalid = true
	}
	if w.ia.Clicked && w.allowClear && !w.val.IsZero() && !(w.needConfirm && st.open) && st.clearR.Expand(3).Contains(c.pointer) {
		*w.val = time.Time{}
		w.changed = true
		st.open = false
		c.invalid = true
		return
	}
	if w.ia.Clicked {
		setOpen(!st.open)
	}
	if st.open && c.focus == 0 && c.pressedAny && c.rects[st.panel].Contains(c.pressPos) {
		// Clicks inside the panel keep the keyboard on the picker.
		c.SetFocus(w.n.id)
	}
	commit := func(d time.Time) {
		if d.IsZero() || (w.disabledFn != nil && w.disabledFn(d)) {
			return
		}
		if !d.Equal(*w.val) {
			*w.val = d
			w.changed = true
		}
		st.ty.on = false
		setOpen(false)
	}
	pick := commit
	if w.needConfirm {
		// A pick only drafts; OK (or Enter again) commits.
		pick = func(d time.Time) {
			if w.disabledFn != nil && w.disabledFn(d) {
				return
			}
			st.draft = d
			st.ty.on = false
			c.invalid = true
		}
	}
	core := w.core(pick)
	parse := func() (time.Time, bool) {
		d, ok := p2aParsePeriod(w.loc, st.ty.buf, w.picker, w.layout(), w.loc.WeekStart)
		if !ok || (w.disabledFn != nil && w.disabledFn(d)) {
			return time.Time{}, false
		}
		return d, true
	}
	ev := dtTypeInput(c, w.n.id, &st.ty, w.display)
	if ev.blurred {
		if d, ok := parse(); ok && !w.needConfirm && !d.Equal(*w.val) {
			*w.val = d
			w.changed = true
		}
		if !dtFocusIn(c, st.panel) {
			st.open = false // Tab out of a typed field closes the panel
		}
		c.invalid = true
	}
	if ev.started && !st.open {
		setOpen(true)
	}
	if ev.edited {
		if d, ok := parse(); ok {
			// The panel follows the typed date.
			core.sync()
			lvl := fillViewLevel(w.picker)
			st.fp.mode, st.fp.view, st.fp.cursor = lvl, fillAnchor(d, lvl), d
		}
	}
	for _, k := range ev.nav {
		if !k.Down {
			continue
		}
		if st.ty.on {
			if k.Key == input.KeyEnter {
				if d, ok := parse(); ok {
					commit(d) // Enter confirms typed text, also with NeedConfirm
				}
				continue
			}
			if k.Key == input.KeySpace {
				continue
			}
		}
		switch {
		case !st.open:
			if k.Key == input.KeyEnter || k.Key == input.KeySpace || k.Key == input.KeyDown {
				setOpen(true)
			}
		case w.needConfirm && (k.Key == input.KeyEnter || k.Key == input.KeySpace) && st.fp.mode == fillViewLevel(w.picker) &&
			(st.fp.cursor.IsZero() || st.fp.cursor.Equal(st.draft)):
			// Enter on the drafted cell (or without moving) confirms.
			if st.draft.IsZero() {
				setOpen(false)
			} else {
				commit(st.draft)
			}
		case (k.Key == input.KeyEnter || k.Key == input.KeySpace) && st.fp.cursor.IsZero() && st.fp.mode == fillViewLevel(w.picker):
			// Enter without moving the cursor keeps the value.
			setOpen(false)
		default:
			core.key(k)
		}
	}
	if st.open && c.Dismissed(st.panel, w.n.id) {
		st.open = false
	}
	if !st.open {
		st.ty.on = false
		return
	}
	th, tk := dtTheme(c)
	loc := c.Locale()
	a := c.AnimFrom(w.n.id, "open", 0, 1, th.DurationMid)
	ov := Overlay(c, OverlayOptions{Anchor: w.n.id, Place: BottomStart, Offset: 4, Flip: true}, func() {
		Row(c, func() {
			if len(w.presets) > 0 {
				w.presetList(commit)
			}
			Column(c, func() {
				core.content()
				switch {
				case w.needConfirm:
					w.confirmFooter(pick, commit)
				case w.picker == PickerDate && dtFooterLink(c, loc.Today):
					pick(dtDayUTC(c.now))
				}
			})
		})
	})
	ov.Bg(tk.PanelBg).Radius(tk.PanelRadius).Shadow(tk.PanelShadow).Opacity(a)
	entPopupBorder(c, ov)
	st.panel = ov.n.id
}

// confirmFooter is the NeedConfirm footer: Today (date picker) drafts
// today, OK commits the draft.
func (w *DatePickerW) confirmFooter(pick, commit func(time.Time)) {
	c, st := w.c, w.st
	loc := c.Locale()
	el := c.Theme.Flavor == theme.Element
	dtFooter(c, func() {
		if w.picker == PickerDate {
			today := false
			if el {
				today = dtTextBtn(c, loc.Today, true)
			} else {
				today = dtLink(c, loc.Today)
			}
			if today {
				pick(dtDayUTC(c.now))
			}
		}
		Spacer(c)
		ok := Button(c, loc.OK).Small().Disabled(st.draft.IsZero())
		if !el {
			ok.Primary()
		}
		if ok.Clicked() {
			commit(st.draft)
		}
	})
}

// presetList builds the side list of preset dates.
func (w *DatePickerW) presetList(pick func(time.Time)) { dtPresetList(w.c, w.presets, pick) }

// dtPresetList builds the side list of preset values (DatePicker,
// DateTimePicker): Value, else Range[0].
func dtPresetList(c *Context, presets []DatePreset, pick func(time.Time)) {
	th, tk := dtTheme(c)
	b := Column(c, func() {
		for _, p := range presets {
			v := p.Value
			if v.IsZero() {
				v = p.Range[0]
			}
			if dtPresetItem(c, p.Label) {
				pick(v)
			}
		}
	}, layout.W(tk.PresetsWidth), layout.PadVH(8, 8), layout.Gap(4))
	if th.Flavor == theme.Element {
		b.Style(layout.PadVH(8, 4), layout.Gap(0))
	}
	b.Paint(func(l *paint.List, r geom.Rect) { dtEndDivider(c, l, r, th) })
}

func (w *DatePickerW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.Theme
	dis := w.disabled || w.n.disabled
	rad := geom.Rad(c.controlRadius(w.size))
	if w.variant == "underlined" {
		rad = geom.Radii{}
	}
	c.paintFrame(l, r, rad, w.n.id, frameState{hover: w.ia.Hovered, focus: w.st.open || w.ia.Focused, disabled: dis, status: w.status, variant: w.variant})
	st := c.style(c.controlFont(w.size), text.Regular)
	s, col := w.ph(), th.TextQuaternary
	if !w.shown().IsZero() {
		s, col = w.display(), th.Text
	}
	if dis {
		col = th.TextDisabled
	}
	is := float32(14)
	iy := r.Y + (r.H-is)/2
	iconCol := th.TextQuaternary
	if dis {
		iconCol = th.TextDisabled
	}
	showClear := w.allowClear && !dis && w.ia.Hovered && !w.val.IsZero() && !(w.needConfirm && w.st.open)
	// Positions are computed in LTR terms and mirrored in RTL: the
	// calendar icon / clear button sit at the logical ends of the field.
	rtl := c.IsRTL()
	x := r.X + 11
	right := r.Right() - 11 - is - 4
	switch th.Flavor {
	case theme.StyleShadcn:
		x = r.X + th.ControlPaddingH
		w.st.clearR = dtMirrorRect(rtl, geom.Rect{X: r.Right() - th.ControlPaddingH - 16, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, r)
		if showClear {
			entClearIcon(c, l, w.st.clearR)
		} else {
			drawIcon(l, icon.Calendar, w.st.clearR.X, w.st.clearR.Y, 16, theme.Fade(th.Text, .5))
		}
	case theme.Element:
		drawIcon(l, icon.Calendar, dtMirror(rtl, r, x, is), iy, is, iconCol)
		x += is + 8
		w.st.clearR = dtMirrorRect(rtl, geom.Rect{X: r.Right() - 11 - is, Y: iy, W: is, H: is}, r)
		if showClear {
			icon.Draw(l, icon.CloseCircle, w.st.clearR, th.TextQuaternary, 0)
		}
	default:
		w.st.clearR = dtMirrorRect(rtl, geom.Rect{X: r.Right() - 11 - is, Y: iy, W: is, H: is}, r)
		if showClear {
			entClearIcon(c, l, w.st.clearR)
		} else {
			drawIcon(l, icon.Calendar, w.st.clearR.X, iy, is, iconCol)
		}
	}
	lh := th.LineHeightFor(c.controlFont(w.size))
	tr := geom.Rect{X: x, Y: r.Y + (r.H-lh)/2, W: max(right-x, 0), H: lh}
	if th.Flavor == theme.StyleShadcn {
		tr.W = max(dtMirrorRect(rtl, w.st.clearR, r).X-6-x, 0)
	}
	tr = dtMirrorRect(rtl, tr, r)
	tc := th.Text
	if dis {
		tc = th.TextDisabled
	}
	if dtPaintTyping(c, l, tr, w.n.id, &w.st.ty, st, tc, w.ph()) {
		return
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true, Ellipsis: true, MaxWidth: tr.W})
	l.Glyphs(geom.Pt(dtTextX(rtl, tr, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
}
