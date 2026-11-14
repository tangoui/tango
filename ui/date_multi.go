package ui

// MultiDatePicker: a date picker holding several dates (Ant DatePicker
// multiple, Element type="dates" / "months" / "years"). The trigger is the
// selector shell (selector.go) with one closable tag per date; the popup is
// the DatePickerPanel engine with every selected period drawn as selected.

import (
	"slices"
	"time"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

type mdpState struct {
	pop      entPopup
	fp       fillDPState
	draft    []time.Time // NeedConfirm: the selection being edited
	anchor   time.Time   // last toggled period (the panel's reference date)
	lastOpen bool
}

// MultiDatePickerW is a date input with several values shown as tags.
type MultiDatePickerW struct {
	_           [0]func() // not comparable (API rule R4)
	c           *Context
	box         *selector
	val         *[]time.Time
	st          *mdpState
	loc         *Locale
	picker      PickerType
	format      string
	formatSet   bool
	phSet       bool
	disabledFn  func(time.Time) bool
	needConfirm bool
	changed     bool
	forced      bool
	z           *lazy
	ids         dtGridIDs
}

// MultiDatePicker adds a multiple date input bound to *vals (Ant
// DatePicker multiple; with Picker(PickerMonth / PickerYear) Element's
// type="months" / "years"). Each click in the panel toggles a date (with
// Picker: a period start, UTC midnight); the panel stays open, the values
// are kept sorted and shown as closable tags. Backspace removes the last
// tag; arrow keys move the panel cursor and Enter / Space toggle it.
func MultiDatePicker(c *Context, vals *[]time.Time) *MultiDatePickerW {
	w := &MultiDatePickerW{c: c, val: vals, loc: c.Locale()}
	w.box = newSelector(c, "multidatepicker")
	w.box.allowClear = true
	w.box.arrowIcon = icon.Calendar
	w.st = State[mdpState](c, hashID(w.box.n.id, "mdp"), nil)
	w.configDefaults() // Defaults("DatePicker", ...): format, placeholder, picker, allowClear, width, size
	w.z = c.lazy(w.box.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *MultiDatePickerW) A11yLabel(s string) *MultiDatePickerW { w.box.n.setName(s); return w }

func (w *MultiDatePickerW) Placeholder(s string) *MultiDatePickerW {
	w.box.placeholder, w.phSet = s, true
	return w
}

// Picker sets the granularity: PickerDate (default), PickerWeek,
// PickerMonth, PickerQuarter or PickerYear.
func (w *MultiDatePickerW) Picker(p PickerType) *MultiDatePickerW { w.picker = p; return w }

// Format sets the Go layout of the tags (see DatePickerW.Format).
func (w *MultiDatePickerW) Format(f string) *MultiDatePickerW {
	w.format, w.formatSet = f, true
	return w
}

// DisabledDate marks dates (period starts) that cannot be picked.
func (w *MultiDatePickerW) DisabledDate(fn func(time.Time) bool) *MultiDatePickerW {
	w.disabledFn = fn
	return w
}

// NeedConfirm keeps the toggled dates as a draft until the footer's OK
// button commits them; closing the panel otherwise drops the changes.
func (w *MultiDatePickerW) NeedConfirm(on bool) *MultiDatePickerW { w.needConfirm = on; return w }

// MaxTagCount folds the tags after the first n into "+ N ..." (0 = all).
func (w *MultiDatePickerW) MaxTagCount(n int) *MultiDatePickerW { w.box.maxCount = n; return w }

// AllowClear toggles the clear-all button shown on hover (on by default).
func (w *MultiDatePickerW) AllowClear(on bool) *MultiDatePickerW { w.box.allowClear = on; return w }

// Status sets the validation look: "error" or "warning".
func (w *MultiDatePickerW) Status(s string) *MultiDatePickerW    { w.box.status = s; return w }
func (w *MultiDatePickerW) Size(s ControlSize) *MultiDatePickerW { w.box.size = s; return w }

// Width sets the input width in pixels (default 260; tags wrap).
func (w *MultiDatePickerW) Width(px float32) *MultiDatePickerW { w.box.n.ln.Width = px; return w }

// Disabled disables the picker.
func (w *MultiDatePickerW) Disabled(on bool) *MultiDatePickerW {
	if on {
		w.box.disabled = true
	}
	return w
}

// Open forces the panel open (screenshots, tests).
func (w *MultiDatePickerW) Open() *MultiDatePickerW { w.forced = true; return w }

// IsOpen reports whether the panel is shown.
func (w *MultiDatePickerW) IsOpen() bool { w.z.run(); return w.st.pop.open }

// Changed reports a committed change of the dates this frame.
func (w *MultiDatePickerW) Changed() bool { w.z.run(); return w.changed }

// layout is the Go layout of a tag.
func (w *MultiDatePickerW) layout() string {
	if w.formatSet {
		return w.format
	}
	return p2aPickerLayout(w.picker)
}

// shown is the selection displayed: the draft while a NeedConfirm panel is
// open.
func (w *MultiDatePickerW) shown() []time.Time {
	if w.needConfirm && w.st.pop.open {
		return w.st.draft
	}
	return *w.val
}

// setShown replaces the displayed selection (the draft or the value).
func (w *MultiDatePickerW) setShown(v []time.Time) {
	if w.needConfirm && w.st.pop.open {
		w.st.draft = v
	} else {
		w.commit(v)
	}
	w.c.invalid = true
}

func (w *MultiDatePickerW) commit(v []time.Time) {
	if !slices.EqualFunc(v, *w.val, time.Time.Equal) {
		*w.val = v
		w.changed = true
	}
}

// toggle adds or removes the period starting at p, keeping the dates sorted.
func (w *MultiDatePickerW) toggle(p time.Time) {
	if w.disabledFn != nil && w.disabledFn(p) {
		return
	}
	cur := w.shown()
	out := make([]time.Time, 0, len(cur)+1)
	found := false
	for _, t := range cur {
		if fillPeriodStart(t, w.picker, w.loc.WeekStart).Equal(p) {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	w.st.anchor = p
	w.setShown(out)
}

func (w *MultiDatePickerW) finish() {
	c, b, st := w.c, w.box, w.st
	if b.n.ln.Width == 0 {
		b.n.ln.Width = 260
	}
	if !w.phSet {
		b.placeholder = p2aPickerPlaceholder(w.loc, w.picker)
	}
	if w.forced && !b.disabled {
		st.pop.open = true
	}
	if st.pop.open && !st.lastOpen {
		w.reopen()
	}
	shown := w.shown()
	labels := make([]string, len(shown))
	for i, t := range shown {
		labels[i] = p2aFormatPeriod(w.loc, t, w.picker, w.layout(), w.loc.WeekStart)
	}
	b.open = st.pop.open
	b.build(labels)
	if b.disabled {
		st.pop.open, b.open, st.lastOpen = false, false, false
		return
	}
	switch {
	case b.removed >= 0 && b.removed < len(shown):
		w.setShown(slices.Delete(slices.Clone(shown), b.removed, b.removed+1))
	case b.clearClicked():
		w.commit(nil)
		st.pop.open = false
		c.invalid = true
	case b.clicked():
		st.pop.open = !st.pop.open
		if st.pop.open {
			w.reopen()
		}
		c.SetFocus(b.ed.id)
		c.invalid = true
	}
	if !w.forced {
		st.pop.dismiss(c, b.n.id)
	}
	if st.pop.open && c.focus == 0 { // presses inside the panel keep focus
		c.SetFocus(b.ed.id)
	}
	core := w.core()
	keys := b.takeKeys(func(k input.KeyEvent) bool {
		switch k.Key {
		case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight, input.KeyPageUp, input.KeyPageDown,
			input.KeyEnter, input.KeySpace, input.KeyBackspace:
			return true
		}
		return false
	})
	for _, k := range keys {
		switch {
		case k.Key == input.KeyBackspace:
			if s := w.shown(); len(s) > 0 {
				w.setShown(slices.Clone(s[:len(s)-1]))
			}
		case !st.pop.open:
			if k.Key == input.KeyEnter || k.Key == input.KeySpace || k.Key == input.KeyDown {
				st.pop.open = true
				w.reopen()
				c.invalid = true
			}
		default:
			core.key(k)
		}
	}
	b.edit() // nothing to type: consume the remaining input
	st.lastOpen = st.pop.open
	b.open = st.pop.open
	if !st.pop.open {
		return
	}
	w.ids = dtGridIDs{}
	dtStylePanel(c, entPanel(c, b.n.id, &st.pop, false, func() {
		Column(c, func() {
			core.content()
			if w.needConfirm {
				w.footer()
			}
		})
	}))
	b.listID, b.activeID = w.ids.grid, w.ids.active
}

// reopen starts a fresh panel: the draft copies the value and the view
// shows the first date (or today).
func (w *MultiDatePickerW) reopen() {
	st := w.st
	st.draft = slices.Clone(*w.val)
	st.anchor = time.Time{}
	if len(*w.val) > 0 {
		st.anchor = fillPeriodStart((*w.val)[0], w.picker, w.loc.WeekStart)
	}
	st.fp.has = false
}

// core returns the panel engine over the picker state.
func (w *MultiDatePickerW) core() *DatePickerPanelW {
	ws := w.loc.WeekStart
	sel := make([]time.Time, 0, len(w.shown()))
	for _, t := range w.shown() {
		sel = append(sel, fillPeriodStart(t, w.picker, ws))
	}
	return &DatePickerPanelW{c: w.c, val: &w.st.anchor, picker: w.picker, disabled: w.disabledFn,
		st: &w.st.fp, ids: &w.ids, multi: sel, onPick: w.toggle}
}

// footer is the NeedConfirm footer with the OK button.
func (w *MultiDatePickerW) footer() {
	c, st := w.c, w.st
	el := c.Theme.Flavor == theme.Element
	dtFooter(c, func() {
		Spacer(c)
		ok := Button(c, c.Locale().OK).Small()
		if !el {
			ok.Primary()
		}
		if ok.Clicked() {
			w.commit(slices.Clone(st.draft))
			st.pop.open = false
			c.invalid = true
		}
	})
}
