package ui

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// DatePreset is a named value shown beside the picker panels: Range for
// RangePicker, Value for DatePicker (Range[0] when Value is zero).
type DatePreset struct {
	Label string
	Range [2]time.Time
	Value time.Time
}

type dtRangeState struct {
	pop      entPopup
	active   int       // 0 = start segment, 1 = end segment
	pick     int       // 0 = waiting for the first date, 1 = for the second
	a        time.Time // first picked date
	hover    time.Time
	cursor   time.Time
	view     time.Time // month of the left panel
	lastOpen bool
	clearR   geom.Rect
	draft    [2]time.Time // ShowTime: ends being edited until OK
	fp       fillDPState  // Picker: the period panels (DatePickerPanel engine)
	ty       dtTyping     // text typed into the active end
}

// RangePickerW selects a date range in two adjacent month panels.
type RangePickerW struct {
	c          *Context
	n          *node
	val        *[2]time.Time
	st         *dtRangeState
	ph         [2]string // explicit placeholders (phSet)
	phSet      bool
	loc        *Locale // captured at build: default placeholders follow picker
	sep        string  // Element separator text, captured at build
	format     string
	formatSet  bool
	showTime   bool
	picker     PickerType
	variant    string
	size       ControlSize
	status     string
	disabled   bool
	allowClear bool
	presets    []DatePreset
	disabledFn func(time.Time) bool
	changed    bool
	forced     bool
	ia         Interaction
	z          *lazy
	ids        dtGridIDs // first grid and cursor day of the open panels (semantics)
}

// RangePicker adds a date range input bound to *val ([start, end], zero =
// empty). The first click picks one end, the second click the other; the
// result is ordered. Picker switches to week, month, quarter or year
// ranges. Each end can also be typed: printable keys edit the active end in
// place; Enter (or leaving the field) applies valid text, invalid text is
// dropped.
func RangePicker(c *Context, val *[2]time.Time) *RangePickerW {
	w := &RangePickerW{c: c, val: val, loc: c.Locale(), sep: c.Locale().RangeSeparator, format: "2006-01-02", status: c.itemStatus, allowClear: true, size: c.defaultSize()}
	w.n = c.add("rangepicker", "")
	// ConfigProvider.Defaults("RangePicker", ...): "format", "separator",
	// "allowClear", "showTime", "width", "size", "variant", "picker"
	// (PickerType), "placeholder" ([2]string).
	def := p2aDefaults(c, "RangePicker")
	if v, ok := c.ConfigProp("RangePicker", "picker"); ok {
		if p, ok := v.(PickerType); ok {
			w.picker = p
		}
	}
	if v, ok := c.ConfigProp("RangePicker", "placeholder"); ok {
		if p, ok := v.([2]string); ok {
			w.ph, w.phSet = p, true
		}
	}
	def.str("variant", &w.variant)
	w.formatSet = def.str("format", &w.format)
	def.str("separator", &w.sep)
	def.flag("allowClear", &w.allowClear)
	def.flag("showTime", &w.showTime)
	def.px("width", &w.n.ln.Width)
	def.size(&w.size)
	w.n.ln.NoStretch = true
	w.n.ownKeys = true
	w.st = State[dtRangeState](c, w.n.id, nil)
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(w)
	w.n.paint = w.paint
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; otherwise the two placeholders).
func (w *RangePickerW) A11yLabel(s string) *RangePickerW { w.n.setName(s); return w }

// semantics: "start – end" as the value; while open the first month grid
// (Controls) and the cursor day (ActiveDescendant). Arrow keys move the
// cursor, PageUp / PageDown change the month, Enter picks an end, Escape
// closes.
func (w *RangePickerW) semantics(s *a11y.Node) {
	if s.Name == "" {
		ph := w.placeholders()
		s.Name = ph[0] + " " + w.sep + " " + ph[1]
	}
	if w.hasValue() {
		s.Value = w.show(w.val[0]) + " " + w.sep + " " + w.show(w.val[1])
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.st.pop.open {
		semExpanded(s)
		s.Controls, s.ActiveDescendant = a11y.NodeID(w.ids.grid), a11y.NodeID(w.ids.active)
	}
}

func (w *RangePickerW) Placeholder(start, end string) *RangePickerW {
	w.ph, w.phSet = [2]string{start, end}, true
	return w
}
func (w *RangePickerW) Format(f string) *RangePickerW         { w.format, w.formatSet = f, true; return w }
func (w *RangePickerW) Status(s string) *RangePickerW         { w.status = s; return w }
func (w *RangePickerW) Size(s ControlSize) *RangePickerW      { w.size = s; return w }
func (w *RangePickerW) Width(px float32) *RangePickerW        { w.n.ln.Width = px; return w }
func (w *RangePickerW) AllowClear(on bool) *RangePickerW      { w.allowClear = on; return w }
func (w *RangePickerW) Presets(p ...DatePreset) *RangePickerW { w.presets = p; return w }

// Variant sets the frame look: "outlined" (default), "filled",
// "borderless" or "underlined".
func (w *RangePickerW) Variant(v string) *RangePickerW { w.variant = v; return w }

// Picker sets the granularity of both ends: PickerDate (default),
// PickerWeek, PickerMonth, PickerQuarter or PickerYear (Ant picker, Element
// type="monthrange" / "yearrange"). Ends are the first days of their
// periods (UTC midnights, like DatePicker with Picker); ShowTime is ignored.
func (w *RangePickerW) Picker(p PickerType) *RangePickerW { w.picker = p; return w }

// ShowTime adds start / end time columns under the month panels and an OK
// button (Ant RangePicker showTime, Element type="datetimerange"): days
// and times are drafts until OK (or Enter on the picked end day) commits.
// The default format becomes "2006-01-02 15:04:05"; a format without
// seconds hides the second columns.
func (w *RangePickerW) ShowTime() *RangePickerW { w.showTime = true; return w }

// layout returns the Go layout of the trigger texts.
func (w *RangePickerW) layout() string {
	if w.periods() {
		if w.formatSet {
			return w.format
		}
		return p2aPickerLayout(w.picker)
	}
	if w.showTime && !w.formatSet {
		return "2006-01-02 15:04:05"
	}
	return w.format
}

// periods reports a coarser granularity than days.
func (w *RangePickerW) periods() bool { return w.picker != PickerDate }

// placeholders are the explicit ones (Placeholder, Defaults "placeholder"),
// else the locale's start / end words for the granularity (Start week …,
// StartDate / EndDate for dates and date-times).
func (w *RangePickerW) placeholders() [2]string {
	if w.phSet {
		return w.ph
	}
	return p2aRangePlaceholders(w.loc, w.picker)
}

// show formats one end for the trigger.
func (w *RangePickerW) show(t time.Time) string {
	if w.periods() {
		loc := w.c.Locale()
		return p2aFormatPeriod(loc, t, w.picker, w.layout(), loc.WeekStart)
	}
	return dtFormat(t, w.layout())
}

// parse reads typed text of one end (period start, day, or with ShowTime
// date and clock); disabled dates are rejected.
func (w *RangePickerW) parse(s string) (time.Time, bool) {
	var d time.Time
	ok := false
	if w.periods() {
		loc := w.c.Locale()
		d, ok = p2aParsePeriod(loc, s, w.picker, w.layout(), loc.WeekStart)
	} else {
		tz := w.c.now.Location()
		if !w.val[0].IsZero() {
			tz = w.val[0].Location()
		}
		d, ok = dtParse(s, w.layout(), tz)
		if ok && !w.showTime {
			d = dtDay(d)
		}
	}
	if !ok || (w.disabledFn != nil && w.disabledFn(dtDay(d))) {
		return time.Time{}, false
	}
	return d, true
}

// shown returns the ends the trigger displays.
func (w *RangePickerW) shown() [2]time.Time {
	st := w.st
	vals := *w.val
	switch {
	case !st.pop.open:
	case w.periods():
		if !st.fp.rangeStart.IsZero() {
			vals = [2]time.Time{st.fp.rangeStart, {}}
		}
	case w.showTime:
		vals = st.draft
	case st.pick == 1:
		vals = [2]time.Time{st.a, {}}
	}
	return vals
}

// typed applies valid text d to the active end: with the other end known
// the range is set (ShowTime: drafted, committed when both are ready),
// otherwise d becomes the first end and the other end is edited next.
func (w *RangePickerW) typed(d time.Time) {
	st := w.st
	st.ty.on = false
	i := st.active
	if w.showTime {
		st.draft[i] = d
		st.cursor, st.view = dtDay(d), dtMonthStart(d)
		if w.draftReady() {
			w.commitDraft()
		} else {
			w.activate(1 - i)
		}
		return
	}
	other := w.shown()[1-i]
	if !other.IsZero() {
		a, b := d, other
		if b.Before(a) {
			a, b = b, a
		}
		w.set([2]time.Time{a, b})
		return
	}
	st.a, st.pick, st.active = d, 1, 1-i
	st.fp.rangeStart = d
	st.cursor = d
	w.c.invalid = true
}

// follow moves the panels to a valid date being typed.
func (w *RangePickerW) follow(d time.Time) {
	st := w.st
	if w.periods() {
		lvl := fillViewLevel(w.picker)
		st.fp.mode, st.fp.view, st.fp.cursor = lvl, fillAnchor(d, lvl), d
	} else {
		st.cursor = dtDay(d)
		if st.cursor.Before(st.view) || !st.cursor.Before(dtAddMonths(st.view, 2)) {
			st.view = dtMonthStart(d)
		}
	}
	w.c.invalid = true
}

// core returns the period panel engine (Picker) over the picker state.
func (w *RangePickerW) core() *DatePickerPanelW {
	return &DatePickerPanelW{c: w.c, val: &w.val[0], end: &w.val[1], ranged: true, picker: w.picker,
		disabled: w.disabledFn, st: &w.st.fp, ids: &w.ids, onRange: func(a, b time.Time) { w.set([2]time.Time{a, b}) }}
}

// DisabledDate marks days that cannot be picked.
func (w *RangePickerW) DisabledDate(fn func(time.Time) bool) *RangePickerW {
	w.disabledFn = fn
	return w
}

func (w *RangePickerW) Layout(o ...layout.Option) *RangePickerW {
	w.n.ln.Style.Apply(o...)
	return w
}

// Disabled disables the picker.
func (w *RangePickerW) Disabled(on bool) *RangePickerW {
	if on {
		w.disabled, w.n.disabled, w.n.focus, w.n.cursor = true, true, false, input.CursorNotAllowed
	}
	return w
}

// Open forces the panels open (screenshots, tests).
func (w *RangePickerW) Open() *RangePickerW { w.forced = true; w.st.pop.open = true; return w }

// IsOpen reports whether the panels are shown.
func (w *RangePickerW) IsOpen() bool { return w.st.pop.open }

// Changed reports a committed range this frame.
func (w *RangePickerW) Changed() bool { w.z.run(); return w.changed }

func (w *RangePickerW) isDisabled() bool { return w.disabled || w.n.disabled }

func (w *RangePickerW) hasValue() bool { return !w.val[0].IsZero() || !w.val[1].IsZero() }

// segment returns which half of the trigger x falls into (the start input
// is at the inline start: the right half in RTL).
func (w *RangePickerW) segment(x float32) int {
	r := w.c.rects[w.n.id]
	if (x > r.X+r.W/2) != w.c.IsRTL() {
		return 1
	}
	return 0
}

func (w *RangePickerW) openAt(seg int) {
	st := w.st
	st.pop.open = true
	st.active = seg
	st.pick, st.a = 0, time.Time{}
	if seg == 1 && !w.val[0].IsZero() {
		st.pick, st.a = 1, w.val[0]
	}
	st.draft = *w.val
	base := w.val[0]
	if base.IsZero() {
		base = w.c.now
	}
	st.view = dtMonthStart(base)
	st.cursor = dtDay(base)
	if seg == 1 && !w.val[1].IsZero() {
		st.cursor = dtDay(w.val[1])
	}
	if w.periods() {
		lvl := fillViewLevel(w.picker)
		st.fp = fillDPState{has: true, picker: w.picker, mode: lvl, view: fillAnchor(base, lvl)}
		if seg == 1 && !w.val[0].IsZero() {
			st.fp.rangeStart = w.val[0]
		}
	}
	w.c.invalid = true
}

func (w *RangePickerW) pickDay(d time.Time) {
	st := w.st
	if w.disabledFn != nil && w.disabledFn(d) {
		return
	}
	st.cursor = d
	if w.showTime {
		w.draftDay(d)
		return
	}
	if st.pick == 0 {
		st.a, st.pick, st.active = d, 1, 1
		w.c.invalid = true
		return
	}
	a, b := st.a, d
	if b.Before(a) {
		a, b = b, a
	}
	w.set([2]time.Time{a, b})
}

func (w *RangePickerW) set(v [2]time.Time) {
	if !v[0].Equal(w.val[0]) || !v[1].Equal(w.val[1]) {
		*w.val = v
		w.changed = true
	}
	w.st.pop.open = false
	w.st.pick, w.st.active = 0, 0
	w.st.ty.on = false
	w.c.invalid = true
}

func (w *RangePickerW) finish() {
	c, st := w.c, w.st
	th := c.Theme
	w.n.ln.Height = c.controlHeight(w.size)
	if w.n.ln.Width == 0 {
		w.n.ln.Width = 260
		if th.Flavor == theme.Element {
			w.n.ln.Width = 350
		}
		if w.showTime {
			w.n.ln.Width = 380
			if th.Flavor == theme.Element {
				w.n.ln.Width = 400
			}
		}
	}
	if w.isDisabled() {
		st.pop.open = false
		return
	}
	if w.ia.Clicked && w.allowClear && w.hasValue() && w.st.clearR.Expand(3).Contains(c.pointer) {
		*w.val = [2]time.Time{}
		w.changed = true
		st.pop.open = false
		c.invalid = true
		return
	}
	if w.ia.PressedNow {
		seg := w.segment(c.pointer.X)
		switch {
		case !st.pop.open:
			w.openAt(seg)
		case seg != st.active && w.showTime:
			w.activate(seg)
		case seg != st.active:
			w.openAt(seg)
		}
	}
	if w.forced {
		st.pop.open = true
	}
	if st.pop.open && !st.lastOpen && !w.ia.PressedNow {
		w.openAt(st.active)
	}
	st.pop.dismiss(c, w.n.id)
	ev := dtTypeInput(c, w.n.id, &st.ty, func() string { return w.show(w.shown()[st.active]) })
	if ev.blurred {
		if d, ok := w.parse(st.ty.buf); ok {
			w.typed(d)
		}
		if !dtFocusIn(c, st.pop.panel) {
			st.pop.open = false
		}
		c.invalid = true
	}
	if ev.started && !st.pop.open {
		w.openAt(st.active)
	}
	if ev.edited {
		if d, ok := w.parse(st.ty.buf); ok {
			w.follow(d)
		}
	}
	var core *DatePickerPanelW
	if w.periods() {
		core = w.core()
	}
	for _, k := range ev.nav {
		if !k.Down {
			continue
		}
		if !st.pop.open {
			if k.Key == input.KeyEnter || k.Key == input.KeyDown || k.Key == input.KeySpace {
				w.openAt(0)
			}
			continue
		}
		if st.ty.on && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
			if d, ok := w.parse(st.ty.buf); ok && k.Key == input.KeyEnter {
				w.typed(d)
			}
			continue
		}
		if core != nil {
			core.key(k)
			continue
		}
		if k.Key == input.KeyEnter || k.Key == input.KeySpace {
			if w.showTime && w.draftReady() && dtSameDay(st.cursor, st.draft[st.active]) {
				w.commitDraft() // Enter again on the picked day confirms
				continue
			}
			w.pickDay(st.cursor)
			continue
		}
		if cur, ok := dtMoveCursor(st.cursor, k.Key, c.IsRTL()); ok {
			st.cursor = cur
			if cur.Before(st.view) {
				st.view = dtMonthStart(cur)
			} else if !cur.Before(dtAddMonths(st.view, 2)) {
				st.view = dtAddMonths(dtMonthStart(cur), -1)
			}
		}
	}
	st.lastOpen = st.pop.open
	if !st.pop.open {
		st.ty.on = false
		return
	}
	if core != nil {
		w.periodPanel(core)
		if !st.fp.rangeStart.IsZero() && st.pop.open {
			st.active = 1
		}
		return
	}
	w.panel()
}

// periodPanel builds the popup of a Picker range: presets and the two
// DatePickerPanel grids.
func (w *RangePickerW) periodPanel(core *DatePickerPanelW) {
	c, st := w.c, w.st
	w.ids = dtGridIDs{}
	dtStylePanel(c, entPanel(c, w.n.id, &st.pop, false, func() {
		Row(c, func() {
			if len(w.presets) > 0 {
				w.presetList()
			}
			core.content()
		})
	}))
}

func (w *RangePickerW) panel() {
	c, st := w.c, w.st
	th := c.Theme
	el := th.Flavor == theme.Element
	today := dtDay(c.now)
	hover := time.Time{}
	w.ids = dtGridIDs{}
	dtStylePanel(c, entPanel(c, w.n.id, &st.pop, false, func() {
		Row(c, func() {
			if len(w.presets) > 0 {
				w.presetList()
			}
			nav := func(m int) { st.view = dtAddMonths(st.view, m); c.invalid = true }
			selA, selB := w.val[0], w.val[1]
			if w.showTime {
				selA, selB = st.draft[0], st.draft[1]
			}
			var prevA, prevB time.Time
			if st.pick == 1 {
				selA, selB = st.a, time.Time{}
				prevA = st.a
				prevB = st.hover
				if prevB.IsZero() {
					prevB = st.cursor
				}
			}
			Column(c, func() {
				Row(c, func() {
					for i := 0; i < 2; i++ {
						i := i
						Key(c, i, func() {
							m := dtMonth{
								view: dtAddMonths(st.view, i), today: today,
								selA: selA, selB: selB, prevA: prevA, prevB: prevB,
								cursor: st.cursor, disabled: w.disabledFn, hideOut: true,
								navPrev: i == 0, navNext: i == 1, onNav: nav, hover: &hover, ids: &w.ids,
							}
							b := Column(c, func() {
								if d, ok := dtMonthPanel(c, m); ok {
									w.pickDay(d)
								}
							})
							if i == 0 && el {
								b.Paint(func(l *paint.List, r geom.Rect) { dtEndDivider(c, l, r, th) })
							}
						})
					}
				})
				if w.showTime {
					w.timeArea()
				}
			})
		})
	}))
	if !hover.Equal(st.hover) {
		st.hover = hover
		c.invalid = true
	}
}

func (w *RangePickerW) presetList() {
	c := w.c
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	b := Column(c, func() {
		for _, p := range w.presets {
			if dtPresetItem(c, p.Label) {
				w.set(p.Range)
			}
		}
	}, layout.W(tk.PresetsWidth), layout.PadVH(8, 8), layout.Gap(4))
	if el {
		b.Style(layout.PadVH(8, 4), layout.Gap(0))
	}
	b.Paint(func(l *paint.List, r geom.Rect) { dtEndDivider(c, l, r, th) })
}

func (w *RangePickerW) paint(l *paint.List, r geom.Rect) {
	open := w.st.pop.open
	dis := w.isDisabled()
	vals := w.shown()
	p2aPaintRangeField(w.c, l, r, p2aRangeField{
		id: w.n.id, size: w.size, status: w.status, variant: w.variant, ty: &w.st.ty, ph: w.placeholders(), sep: w.sep, ic: icon.Calendar,
		text:   [2]string{w.show(vals[0]), w.show(vals[1])},
		active: w.st.active, open: open, hover: w.ia.Hovered, focus: w.ia.Focused, dis: dis,
		focusVisible: w.ia.FocusVisible, showClear: w.allowClear && !dis && w.ia.Hovered && w.hasValue(),
		clearR: &w.st.clearR,
	})
}

// dtPresetItem is a clickable preset label in the side list.
func dtPresetItem(c *Context, label string) bool {
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	n := c.add("dt-preset", label)
	n.ln.Height = 28
	n.setRole(a11y.RoleButton)
	ia := c.interact(n, true, input.CursorPointer)
	n.over = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			entFocusRing(c, l, r, th.RadiusSM)
		}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		col := tk.PresetColor
		if ia.Hovered && tk.PresetHoverColor.A > 0 {
			col = tk.PresetHoverColor
		}
		if el {
			dtStartText(c, l, label, 8, r, th.FontSize, text.Regular, col, r.W-8)
			return
		}
		if ia.Hovered && tk.PresetHoverBg.A > 0 {
			l.Fill(r, geom.Rad(th.RadiusSM), tk.PresetHoverBg)
		}
		dtStartText(c, l, label, 8, r, th.FontSize, text.Regular, col, r.W-16)
	}
	return ia.Clicked
}
