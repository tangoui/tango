package ui

// TimeRangePicker: a start / end time input with two sets of time columns
// (Ant TimePicker.RangePicker, Element el-time-picker is-range). It shares
// the columns, 12-hour clock, steps and disabled options of TimePicker and
// the two-segment trigger of RangePicker.

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

type dtTimeRangeState struct {
	pop      entPopup
	active   int // end whose columns the keyboard edits
	draft    [2]time.Time
	ts       [2]dtTimeState
	lastOpen bool
	clearR   geom.Rect
	ty       dtTyping // text typed into the active end
}

// TimeRangePickerW selects a start and an end time.
type TimeRangePickerW struct {
	_ [0]func()

	c          *Context
	n          *node
	val        *[2]time.Time
	st         *dtTimeRangeState
	loc        *Locale
	ph         [2]string
	sep        string
	format     string
	formatSet  bool
	steps      [3]int
	clock      dtClock
	size       ControlSize
	variant    string
	status     string
	disabled   bool
	allowClear bool
	changed    bool
	forced     bool
	ia         Interaction
	z          *lazy
}

// TimeRangePicker adds a time range input bound to *val ([start, end],
// zero = empty). Both ends are edited in side-by-side columns and
// committed with OK (or Enter); the result is ordered by clock. Each end
// can also be typed: printable keys edit the active end, Enter applies
// valid text (and commits once both ends are set), invalid or disabled
// text is dropped.
//
//	ui.TimeRangePicker(c, &span).Format("15:04").MinuteStep(15)
//
// ConfigProvider.Defaults("TimeRangePicker", ...) keys: "format",
// "separator", "use12Hours", "hourStep", "minuteStep", "secondStep",
// "allowClear", "width", "size", "variant", "hideDisabledOptions",
// "placeholder" ([2]string).
func TimeRangePicker(c *Context, val *[2]time.Time) *TimeRangePickerW {
	loc := c.Locale()
	w := &TimeRangePickerW{c: c, val: val, loc: loc, format: "15:04:05", sep: loc.RangeSeparator,
		ph:     [2]string{loc.TimePicker.StartTime, loc.TimePicker.EndTime},
		status: c.itemStatus, allowClear: true, size: c.defaultSize()}
	w.n = c.add("timerangepicker", "")
	w.n.ln.NoStretch = true
	w.n.ownKeys = true
	def := p2aDefaults(c, "TimeRangePicker")
	w.formatSet = def.str("format", &w.format)
	def.str("separator", &w.sep)
	def.flag("use12Hours", &w.clock.use12)
	def.integer("hourStep", &w.steps[0])
	def.integer("minuteStep", &w.steps[1])
	def.integer("secondStep", &w.steps[2])
	def.flag("allowClear", &w.allowClear)
	def.px("width", &w.n.ln.Width)
	def.size(&w.size)
	def.str("variant", &w.variant)
	def.flag("hideDisabledOptions", &w.clock.hide)
	if v, ok := c.ConfigProp("TimeRangePicker", "placeholder"); ok {
		if p, ok := v.([2]string); ok {
			w.ph = p
		}
	}
	w.st = State[dtTimeRangeState](c, w.n.id, nil)
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.n.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup).fill(w)
	w.n.paint = w.paint
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; otherwise the two placeholders).
func (w *TimeRangePickerW) A11yLabel(s string) *TimeRangePickerW { w.n.setName(s); return w }

// semantics: "start – end" as the value; while open the listbox of the
// keyboard column (Controls) and its selected option (ActiveDescendant).
// Left / Right move across the start and end columns, Up / Down step,
// Enter commits, Escape closes.
func (w *TimeRangePickerW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = w.ph[0] + " " + w.sep + " " + w.ph[1]
	}
	if w.hasValue() {
		s.Value = w.text(w.val[0]) + " " + w.sep + " " + w.text(w.val[1])
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.st.pop.open {
		semExpanded(s)
		list, opt := w.st.ts[w.st.active].active()
		s.Controls, s.ActiveDescendant = a11y.NodeID(list), a11y.NodeID(opt)
	}
}

func (w *TimeRangePickerW) Placeholder(start, end string) *TimeRangePickerW {
	w.ph = [2]string{start, end}
	return w
}

// Format sets the Go time layout ("15:04:05" default, "15:04", or with
// Use12Hours "03:04 PM").
func (w *TimeRangePickerW) Format(f string) *TimeRangePickerW {
	w.format, w.formatSet = f, true
	return w
}
func (w *TimeRangePickerW) HourStep(n int) *TimeRangePickerW     { w.steps[0] = n; return w }
func (w *TimeRangePickerW) MinuteStep(n int) *TimeRangePickerW   { w.steps[1] = n; return w }
func (w *TimeRangePickerW) SecondStep(n int) *TimeRangePickerW   { w.steps[2] = n; return w }
func (w *TimeRangePickerW) Status(s string) *TimeRangePickerW    { w.status = s; return w }
func (w *TimeRangePickerW) Size(s ControlSize) *TimeRangePickerW { w.size = s; return w }
func (w *TimeRangePickerW) Width(px float32) *TimeRangePickerW   { w.n.ln.Width = px; return w }
func (w *TimeRangePickerW) AllowClear(on bool) *TimeRangePickerW {
	w.allowClear = on
	return w
}

// Use12Hours shows 12-hour columns with AM / PM.
func (w *TimeRangePickerW) Use12Hours() *TimeRangePickerW { w.clock.use12 = true; return w }

// DisabledHours marks hours (0-23) that cannot be picked.
func (w *TimeRangePickerW) DisabledHours(fn func(h int) bool) *TimeRangePickerW {
	w.clock.disH = fn
	return w
}

// DisabledMinutes marks minutes of hour h that cannot be picked.
func (w *TimeRangePickerW) DisabledMinutes(fn func(h, m int) bool) *TimeRangePickerW {
	w.clock.disM = fn
	return w
}

// DisabledSeconds marks seconds of h:m that cannot be picked.
func (w *TimeRangePickerW) DisabledSeconds(fn func(h, m, s int) bool) *TimeRangePickerW {
	w.clock.disS = fn
	return w
}

// Disabled disables the picker.
func (w *TimeRangePickerW) Disabled(on bool) *TimeRangePickerW {
	if on {
		w.disabled, w.n.disabled, w.n.focus, w.n.cursor = true, true, false, input.CursorNotAllowed
	}
	return w
}

func (w *TimeRangePickerW) Layout(o ...layout.Option) *TimeRangePickerW {
	flexbox.Apply(&w.n.ln.Style, o...)
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *TimeRangePickerW) Open() *TimeRangePickerW { w.forced = true; w.st.pop.open = true; return w }

// IsOpen reports whether the dropdown is shown.
func (w *TimeRangePickerW) IsOpen() bool { return w.st.pop.open }

// Changed reports a committed range this frame.
func (w *TimeRangePickerW) Changed() bool { w.z.run(); return w.changed }

// Variant sets the frame look: "outlined" (default), "filled",
// "borderless" or "underlined".
func (w *TimeRangePickerW) Variant(v string) *TimeRangePickerW { w.variant = v; return w }

// HideDisabledOptions leaves disabled hours, minutes and seconds out of
// the columns instead of greying them (Ant hideDisabledOptions).
func (w *TimeRangePickerW) HideDisabledOptions(on bool) *TimeRangePickerW {
	w.clock.hide = on
	return w
}

// shown returns the ends the trigger displays (the drafts while open).
func (w *TimeRangePickerW) shown() [2]time.Time {
	if w.st.pop.open {
		return w.st.draft
	}
	return *w.val
}

// parse reads typed text of end i; disabled clocks are rejected.
func (w *TimeRangePickerW) parse(i int) (time.Time, bool) {
	cfg := w.cfg(i)
	t, ok := p2aParseClock(w.loc, w.st.ty.buf, w.layout(), cfg.base.Location())
	if !ok {
		return time.Time{}, false
	}
	t = dtWithClock(cfg.base, t)
	if cfg.disabled(t) {
		return time.Time{}, false
	}
	return t, true
}

// typed applies a valid typed end: committed with the other end set,
// otherwise the other end is edited next.
func (w *TimeRangePickerW) typed(t time.Time) {
	st := w.st
	st.ty.on = false
	st.draft[st.active] = t
	st.ts[st.active].syncAll()
	if !st.draft[1-st.active].IsZero() {
		w.commit()
		return
	}
	st.active = 1 - st.active
	w.c.invalid = true
}

func (w *TimeRangePickerW) isDisabled() bool { return w.disabled || w.n.disabled }
func (w *TimeRangePickerW) hasValue() bool   { return !w.val[0].IsZero() || !w.val[1].IsZero() }

func (w *TimeRangePickerW) layout() string {
	if w.clock.use12 && !w.formatSet {
		return "03:04:05 PM"
	}
	return w.format
}

func (w *TimeRangePickerW) text(t time.Time) string { return p2aFormatClock(w.loc, t, w.layout()) }

func (w *TimeRangePickerW) cfg(end int) dtTimeCfg {
	_, tk := dtTheme(w.c)
	h := tk.TimeColumnHeight
	base := w.st.draft[end]
	if base.IsZero() {
		base = w.st.draft[1-end]
	}
	if base.IsZero() {
		base = w.c.now
	}
	cfg := dtTimeCfg{cols: dtTimeCols(w.layout()), steps: w.steps, colH: h, base: base}
	w.clock.apply(&cfg, w.loc)
	return cfg
}

// open starts editing at end seg with the drafts copied from the value.
func (w *TimeRangePickerW) open(seg int) {
	st := w.st
	st.pop.open = true
	st.active = seg
	st.draft = *w.val
	for i := range st.ts {
		st.ts[i].syncAll()
		st.ts[i].col = 0
	}
	w.c.invalid = true
}

// commit stores the drafts (ordered by clock) unless an end is missing or
// disabled.
func (w *TimeRangePickerW) commit() {
	st := w.st
	a, b := st.draft[0], st.draft[1]
	if a.IsZero() || b.IsZero() || w.cfg(0).disabled(a) || w.cfg(1).disabled(b) {
		return
	}
	if dtClockOf(b) < dtClockOf(a) {
		a, b = b, a
	}
	if !a.Equal(w.val[0]) || !b.Equal(w.val[1]) {
		*w.val = [2]time.Time{a, b}
		w.changed = true
	}
	st.pop.open = false
	w.c.invalid = true
}

// dtClockOf is the time of day of t in seconds.
func dtClockOf(t time.Time) int { return t.Hour()*3600 + t.Minute()*60 + t.Second() }

// key moves across both column sets: Left / Right cross from the last
// start column to the first end column, Up / Down step. The columns of a
// set always read left to right (dtTimeColumns); in RTL the start set is
// at the right, so Left crosses from the first start column to the last
// end column and Right back.
func (w *TimeRangePickerW) key(k input.Key) {
	st := w.st
	cfg := w.cfg(st.active)
	ts := &st.ts[st.active]
	rtl := w.c.IsRTL()
	switch {
	case !rtl && k == input.KeyRight && ts.col >= cfg.n()-1 && st.active == 0:
		st.active, st.ts[1].col = 1, 0
	case !rtl && k == input.KeyLeft && ts.col <= 0 && st.active == 1:
		st.active, st.ts[0].col = 0, w.cfg(0).n()-1
	case rtl && k == input.KeyLeft && ts.col <= 0 && st.active == 0:
		st.active, st.ts[1].col = 1, w.cfg(1).n()-1
	case rtl && k == input.KeyRight && ts.col >= cfg.n()-1 && st.active == 1:
		st.active, st.ts[0].col = 0, 0
	default:
		dtTimeKey(&st.draft[st.active], cfg, ts, k)
	}
	w.c.invalid = true
}

func (w *TimeRangePickerW) finish() {
	c, st := w.c, w.st
	th := c.Theme
	w.n.ln.Height = c.controlHeight(w.size)
	if w.n.ln.Width == 0 {
		w.n.ln.Width = 260
		if w.clock.use12 {
			w.n.ln.Width = 300
		}
		if th.Flavor == theme.Element {
			w.n.ln.Width = 350
		}
	}
	if w.isDisabled() {
		st.pop.open = false
		return
	}
	if w.ia.Clicked && w.allowClear && w.hasValue() && st.clearR.Expand(3).Contains(c.pointer) {
		*w.val = [2]time.Time{}
		w.changed = true
		st.pop.open = false
		c.invalid = true
		return
	}
	if w.ia.PressedNow {
		r := c.rects[w.n.id]
		seg := 0
		if (c.pointer.X > r.X+r.W/2) != c.IsRTL() { // start end at the inline start
			seg = 1
		}
		if !st.pop.open {
			w.open(seg)
		} else {
			st.active = seg
			c.invalid = true
		}
	}
	if w.forced {
		st.pop.open = true
	}
	if st.pop.open && !st.lastOpen && !w.ia.PressedNow {
		w.open(st.active)
	}
	if st.pop.open && c.focus == 0 && c.pressedAny && c.rects[st.pop.panel].Contains(c.pressPos) {
		c.SetFocus(w.n.id) // clicks inside the panel keep the keyboard here
	}
	st.pop.dismiss(c, w.n.id)
	ev := dtTypeInput(c, w.n.id, &st.ty, func() string { return w.text(w.shown()[st.active]) })
	if ev.blurred && st.pop.open {
		if t, ok := w.parse(st.active); ok {
			w.typed(t)
		}
		if !dtFocusIn(c, st.pop.panel) {
			st.pop.open = false
		}
		c.invalid = true
	}
	if ev.started && !st.pop.open {
		w.open(st.active)
	}
	if ev.edited {
		if t, ok := w.parse(st.active); ok {
			st.draft[st.active] = t
			st.ts[st.active].syncAll()
		}
	}
	for _, k := range ev.nav {
		if !k.Down {
			continue
		}
		if !st.pop.open {
			if k.Key == input.KeyEnter || k.Key == input.KeyDown || k.Key == input.KeySpace {
				w.open(0)
			}
			continue
		}
		if st.ty.on && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
			if t, ok := w.parse(st.active); ok && k.Key == input.KeyEnter {
				w.typed(t)
			}
			continue
		}
		switch k.Key {
		case input.KeyEnter, input.KeySpace:
			w.commit()
		case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight:
			w.key(k.Key)
		}
	}
	st.lastOpen = st.pop.open
	if !st.pop.open {
		st.ty.on = false
		return
	}
	w.panel()
}

func (w *TimeRangePickerW) panel() {
	c, st := w.c, w.st
	th := c.Theme
	el := th.Flavor == theme.Element
	loc := c.Locale()
	dtStylePanel(c, entPanel(c, w.n.id, &st.pop, false, func() {
		Column(c, func() {
			Row(c, func() {
				for i := 0; i < 2; i++ {
					i := i
					Key(c, i, func() {
						col := Column(c, func() {
							t := Text(c, w.ph[i]).NoWrap().Size(th.FontSizeSM)
							if st.active == i {
								t.Color(th.Primary.Base)
							} else {
								t.Color(th.TextSecondary)
							}
							if dtTimeColumns(c, &st.draft[i], w.cfg(i), &st.ts[i]) {
								st.active = i
							}
						}, layout.PadVH(8, 8), layout.Gap(4), layout.Middle())
						if i == 0 {
							col.Paint(func(l *paint.List, r geom.Rect) { dtEndDivider(c, l, r, th) })
						}
					})
				}
			})
			ready := !st.draft[0].IsZero() && !st.draft[1].IsZero() && !w.cfg(0).disabled(st.draft[0]) && !w.cfg(1).disabled(st.draft[1])
			if el {
				Row(c, func() {
					Spacer(c)
					if dtTextBtn(c, loc.Cancel, false) {
						st.pop.open = false
						c.invalid = true
					}
					if dtTextBtn(c, loc.OK, true) {
						w.commit()
					}
				}, layout.H(36), layout.PadVH(0, 5), layout.Middle(), layout.Gap(4)).Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Components.DatePicker.SplitColor)
				})
				return
			}
			dtFooter(c, func() {
				Spacer(c)
				if Button(c, loc.OK).Primary().Small().Disabled(!ready).Clicked() {
					w.commit()
				}
			})
		})
	}))
}

func (w *TimeRangePickerW) paint(l *paint.List, r geom.Rect) {
	open := w.st.pop.open
	vals := w.shown()
	dis := w.isDisabled()
	p2aPaintRangeField(w.c, l, r, p2aRangeField{
		id: w.n.id, size: w.size, status: w.status, variant: w.variant, ty: &w.st.ty, ph: w.ph, sep: w.sep, ic: icon.Clock,
		text:   [2]string{w.text(vals[0]), w.text(vals[1])},
		active: w.st.active, open: open, hover: w.ia.Hovered, focus: w.ia.Focused, dis: dis,
		focusVisible: w.ia.FocusVisible, showClear: w.allowClear && !dis && w.ia.Hovered && w.hasValue(),
		clearR: &w.st.clearR,
	})
}
