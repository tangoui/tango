package ui

import (
	"fmt"
	"strings"
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

// ---------------------------------------------------------------------------
// Time columns

// dtTimeCfg configures the hour/minute/second columns.
type dtTimeCfg struct {
	cols  int    // 2 = hh:mm, 3 = hh:mm:ss
	steps [3]int // hour, minute, second steps
	colH  float32
	base  time.Time // date used when the value is zero
	// 12-hour clock: the hour column lists 12, 01 … 11 and an AM / PM
	// column follows (words from the Locale).
	use12  bool
	am, pm string
	// Disabled options (Ant disabledTime, Element disabled-hours /
	// -minutes / -seconds), called with 24-hour values.
	disH func(h int) bool
	disM func(h, m int) bool
	disS func(h, m, s int) bool
	// hide leaves disabled options out (Ant hideDisabledOptions).
	hide bool
}

// n is the number of columns (the AM / PM column included).
func (cfg dtTimeCfg) n() int {
	if cfg.use12 {
		return cfg.cols + 1
	}
	return cfg.cols
}

// part maps a column position to 0 hour, 1 minute, 2 second, 3 AM / PM.
func (cfg dtTimeCfg) part(col int) int {
	if col >= cfg.cols {
		return 3
	}
	return col
}

// disabled reports whether the clock of t is not selectable.
func (cfg dtTimeCfg) disabled(t time.Time) bool {
	if t.IsZero() {
		return false
	}
	h, m, s := t.Hour(), t.Minute(), t.Second()
	return (cfg.disH != nil && cfg.disH(h)) || (cfg.disM != nil && cfg.disM(h, m)) ||
		(cfg.cols > 2 && cfg.disS != nil && cfg.disS(h, m, s))
}

// dtTimeItem is one option of a time column.
type dtTimeItem struct {
	v        int // value given to dtSetPart
	label    string
	disabled bool
}

// items lists the options of column part for the current value cur
// (without the disabled ones when cfg.hide is set).
func (cfg dtTimeCfg) items(part int, cur time.Time) []dtTimeItem {
	out := cfg.allItems(part, cur)
	if !cfg.hide {
		return out
	}
	kept := out[:0]
	for _, it := range out {
		if !it.disabled {
			kept = append(kept, it)
		}
	}
	return kept
}

// allItems lists every option of column part for cur.
func (cfg dtTimeCfg) allItems(part int, cur time.Time) []dtTimeItem {
	h, m := 0, 0
	if !cur.IsZero() {
		h, m = cur.Hour(), cur.Minute()
	}
	var out []dtTimeItem
	switch part {
	case 3:
		for v, word := range []string{cfg.am, cfg.pm} {
			dis := cfg.disH != nil
			for hh := 12 * v; hh < 12*v+12 && dis; hh++ {
				dis = cfg.disH(hh)
			}
			out = append(out, dtTimeItem{v: v, label: word, disabled: dis})
		}
		return out
	case 0:
		limit := 24
		if cfg.use12 {
			limit = 12
		}
		for v := 0; v < limit; v += dtStepOr(cfg.steps[0]) {
			ah, label := v, fmt.Sprintf("%02d", v)
			if cfg.use12 {
				ah = v + 12*(h/12)
				if v == 0 {
					label = "12"
				}
			}
			out = append(out, dtTimeItem{v: v, label: label, disabled: cfg.disH != nil && cfg.disH(ah)})
		}
		return out
	}
	for v := 0; v < 60; v += dtStepOr(cfg.steps[part]) {
		dis := false
		if part == 1 {
			dis = cfg.disM != nil && cfg.disM(h, v)
		} else {
			dis = cfg.disS != nil && cfg.disS(h, m, v)
		}
		out = append(out, dtTimeItem{v: v, label: fmt.Sprintf("%02d", v), disabled: dis})
	}
	return out
}

// sel returns the item value of part for t (-1 for the zero time).
func (cfg dtTimeCfg) sel(part int, t time.Time) int {
	if t.IsZero() {
		return -1
	}
	switch part {
	case 3:
		return t.Hour() / 12
	case 0:
		if cfg.use12 {
			return t.Hour() % 12
		}
	}
	return dtTimePart(t, part)
}

// dtTimeState is the retained column state.
type dtTimeState struct {
	col   int     // keyboard column position
	sync  [4]bool // scroll the selection to the top on the next build
	itemH float32 // item height of the last build (theme switches re-sync)
	// Semantic IDs of the last build: each column's listbox and its
	// selected option (Controls / ActiveDescendant of the owner).
	lists, sels [4]ID
}

// active returns the listbox of the keyboard column and its selected option.
func (s *dtTimeState) active() (list, opt ID) {
	if s.col < 0 || s.col > 3 {
		return 0, 0
	}
	return s.lists[s.col], s.sels[s.col]
}

func (s *dtTimeState) syncAll() { s.sync = [4]bool{true, true, true, true} }

func dtStepOr(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

// dtTimeCols derives the column count from a Go time layout.
func dtTimeCols(format string) int {
	if strings.Contains(format, "05") {
		return 3
	}
	return 2
}

func dtTimeItemH(th *theme.Theme) float32 { return th.Components.DatePicker.TimeCellHeight }

func dtTimeColW(th *theme.Theme) float32 { return th.Components.DatePicker.TimeColumnWidth }

func dtTimePart(t time.Time, part int) int {
	switch part {
	case 0:
		return t.Hour()
	case 1:
		return t.Minute()
	}
	return t.Second()
}

// dtSetPart sets hour/minute/second (or AM / PM, part 3) of *val; a zero
// value starts from midnight of cfg.base. In 12-hour mode the hour value
// is 0-11 and keeps the half of the day.
func dtSetPart(val *time.Time, cfg dtTimeCfg, part, v int) {
	t := *val
	if t.IsZero() {
		b := cfg.base
		if b.IsZero() {
			b = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		t = dtDay(b)
	}
	h, m, s := t.Hour(), t.Minute(), t.Second()
	switch part {
	case 0:
		if cfg.use12 {
			h = v%12 + 12*(h/12)
		} else {
			h = v
		}
	case 1:
		m = v
	case 3:
		h = h%12 + 12*v
	default:
		s = v
	}
	if cfg.cols < 3 {
		s = 0
	}
	*val = time.Date(t.Year(), t.Month(), t.Day(), h, m, s, 0, t.Location())
}

// dtTimeKey applies a keyboard key to the columns: Left / Right switch
// columns, Up / Down step the active one (skipping disabled options).
func dtTimeKey(val *time.Time, cfg dtTimeCfg, st *dtTimeState, k input.Key) bool {
	switch k {
	case input.KeyLeft:
		st.col = max(0, st.col-1)
	case input.KeyRight:
		st.col = min(cfg.n()-1, st.col+1)
	case input.KeyUp, input.KeyDown:
		st.col = min(max(st.col, 0), cfg.n()-1)
		part := cfg.part(st.col)
		items := cfg.items(part, *val)
		if len(items) == 0 {
			return false
		}
		// The option at or before the current value (-1 when empty).
		cur := -1
		if sel := cfg.sel(part, *val); sel >= 0 {
			for i, it := range items {
				if it.v <= sel {
					cur = i
				}
			}
		}
		dir := 1
		if k == input.KeyUp {
			dir = -1
		}
		next := -1
		for n := 0; n < len(items); n++ {
			var i int
			if cur < 0 {
				i = n // empty value: the first enabled option
			} else {
				i = ((cur+dir*(n+1))%len(items) + len(items)) % len(items)
			}
			if !items[i].disabled {
				next = i
				break
			}
		}
		if next < 0 {
			return false
		}
		dtSetPart(val, cfg, part, items[next].v)
		st.sync[st.col] = true
		return true
	}
	return false
}

// dtTimeColumns builds the scrolling columns; it reports a change of *val.
// The columns read hh:mm:ss left to right in every direction (Q5): in an
// RTL scope they are built in a nested LTR scope, and Left / Right keep
// their physical meaning (dtTimeKey).
func dtTimeColumns(c *Context, val *time.Time, cfg dtTimeCfg, st *dtTimeState) bool {
	if !c.IsRTL() {
		return dtTimeColumnsLTR(c, val, cfg, st)
	}
	changed := false
	ConfigProvider(c).Direction(LTR).Body(func() { changed = dtTimeColumnsLTR(c, val, cfg, st) })
	return changed
}

func dtTimeColumnsLTR(c *Context, val *time.Time, cfg dtTimeCfg, st *dtTimeState) bool {
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	ih := dtTimeItemH(th)
	cw := dtTimeColW(th)
	changed := false
	st.lists, st.sels = [4]ID{}, [4]ID{}
	loc := c.Locale()
	if st.itemH != ih {
		st.itemH = ih
		st.syncAll()
	}
	Row(c, func() {
		for col := 0; col < cfg.n(); col++ {
			col := col
			part := cfg.part(col)
			prefix := []string{"h", "m", "s", "ap"}[part]
			sel := cfg.sel(part, *val)
			items := cfg.items(part, *val)
			selIdx := 0
			for i, item := range items {
				if item.v <= sel {
					selIdx = i
				}
			}
			sb := Scroll(c, func() {
				for _, item := range items {
					item := item
					it := c.add("dt-time", fmt.Sprintf("%s%02d", prefix, item.v))
					it.ln.Height = ih
					isSel := item.v == sel
					label := item.label
					// Options of the column listbox; Up / Down on the focused
					// field or panel step the active column.
					it.setRole(a11y.RoleOption).setName(label).setState(a11y.Selectable).stateIf(isSel, a11y.Selected)
					if item.disabled {
						it.disabled = true
					}
					it.aKbd = true
					if isSel {
						st.sels[col] = it.id
					}
					ia := c.interact(it, false, input.CursorPointer)
					if item.disabled {
						it.cursor = input.CursorNotAllowed
					}
					if ia.Clicked && !item.disabled {
						dtSetPart(val, cfg, part, item.v)
						st.col = col
						st.sync[col] = true
						changed = true
						c.invalid = true
					}
					it.paint = func(l *paint.List, r geom.Rect) {
						in := r
						if !el {
							in = r.Inset(geom.Insets{Left: 4, Right: 4})
						}
						rad := geom.Rad(tk.TimeCellRadius)
						col, wt, fs := tk.TimeCellColor, text.Regular, tk.TimeCellFontSize
						switch {
						case item.disabled:
							col = tk.TimeCellDisabledColor
							if isSel && tk.TimeCellDisabledBg.A > 0 {
								l.Fill(in, rad, tk.TimeCellDisabledBg)
							}
						case isSel:
							if th.Flavor == theme.StyleShadcn {
								// Same shape as a selected calendar day.
								rad = geom.Rad(tk.CellRadius)
							}
							if tk.TimeCellActiveBg.A > 0 {
								l.Fill(in, rad, tk.TimeCellActiveBg)
							} else if ia.Hovered {
								l.Fill(in, rad, tk.TimeCellHoverBg)
							}
							col, wt = tk.TimeCellActiveColor, dtWeight(tk.TimeCellActiveFontWeight)
						case ia.Hovered:
							l.Fill(in, rad, tk.TimeCellHoverBg)
						}
						lay := text.Shape(label, c.style(fs, wt), text.Options{NoWrap: true})
						l.Glyphs(geom.Pt(in.X+(in.W-lay.Width)/2, in.Y+(in.H-lay.Height)/2), lay.Glyphs, col)
					}
				}
				pad := c.add("dt-time-pad", "")
				pad.ln.Height = cfg.colH - ih
			}, layout.W(cw), layout.H(cfg.colH), layout.PadVH(0, 0))
			name := loc.A11y.Hour
			switch part {
			case 1:
				name = loc.A11y.Minute
			case 2:
				name = loc.A11y.Second
			case 3:
				name = loc.TimePicker.Meridiem
			}
			sb.n.setRole(a11y.RoleListbox).setName(name)
			st.lists[col] = sb.n.id
			if col > 0 && !el {
				sb.Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, tk.SplitColor)
				})
			}
			if st.sync[col] {
				idx := 0
				if sel >= 0 {
					idx = selIdx
				}
				sb.ScrollTo(0, float64(idx)*float64(ih))
				st.sync[col] = false
			}
		}
	}, layout.Padding(geom.Insets{Top: 4}))
	return changed
}

// ---------------------------------------------------------------------------
// TimePanel (embeddable)

type dtTimePanelState struct{ ts dtTimeState }

// TimePanelW is a standalone set of hour/minute/second columns.
type TimePanelW struct {
	c       *Context
	n       *node
	val     *time.Time
	st      *dtTimePanelState
	loc     *Locale
	format  string
	steps   [3]int
	height  float32
	clock   dtClock
	changed bool
	ia      Interaction
	z       *lazy
}

// dtClock holds the 12-hour and disabled-option settings shared by the
// time widgets.
type dtClock struct {
	use12 bool
	disH  func(h int) bool
	disM  func(h, m int) bool
	disS  func(h, m, s int) bool
	hide  bool // hideDisabledOptions
}

// apply copies the settings into a column configuration.
func (k dtClock) apply(cfg *dtTimeCfg, loc *Locale) {
	cfg.use12, cfg.disH, cfg.disM, cfg.disS, cfg.hide = k.use12, k.disH, k.disM, k.disS, k.hide
	cfg.am, cfg.pm = p2aMeridiem(loc)
}

// TimePanel adds hour/minute(/second) scroll columns bound to *val. It is
// focusable: Up/Down change the active column, Left/Right switch columns.
func TimePanel(c *Context, val *time.Time) *TimePanelW {
	w := &TimePanelW{c: c, val: val, format: "15:04:05", loc: c.Locale()}
	w.n = c.add("timepanel", "")
	w.n.ln.NoStretch = true
	w.n.ownKeys = true
	w.st = State(c, w.n.id, func() dtTimePanelState { s := dtTimePanelState{}; s.ts.syncAll(); return s })
	w.ia = c.interact(w.n, true, input.CursorDefault)
	w.n.setRole(a11y.RoleGroup).setName(c.Locale().SelectTime).fill(w)
	w.n.over = func(l *paint.List, r geom.Rect) {
		if w.ia.FocusVisible {
			entFocusRing(c, l, r, c.Theme.RadiusSM)
		}
	}
	w.z = c.lazy(w.n, w.build)
	return w
}

// A11yLabel sets the accessible name (default: the Locale's SelectTime).
func (w *TimePanelW) A11yLabel(s string) *TimePanelW { w.n.setName(s); return w }

// semantics: the time as value and the selected option of the keyboard
// column as the active item.
func (w *TimePanelW) semantics(s *a11y.Node) {
	s.Value = p2aFormatClock(w.loc, *w.val, w.layout())
	_, opt := w.st.ts.active()
	s.ActiveDescendant = a11y.NodeID(opt)
}

// Format sets the Go layout that decides the columns ("15:04" or "15:04:05").
func (w *TimePanelW) Format(f string) *TimePanelW { w.format = f; return w }

// Steps sets the hour, minute and second steps.
func (w *TimePanelW) Steps(h, m, s int) *TimePanelW { w.steps = [3]int{h, m, s}; return w }

// Use12Hours shows a 12-hour clock with an AM / PM column.
func (w *TimePanelW) Use12Hours() *TimePanelW { w.clock.use12 = true; return w }

// DisabledHours marks hours (0-23) that cannot be picked.
func (w *TimePanelW) DisabledHours(fn func(h int) bool) *TimePanelW { w.clock.disH = fn; return w }

// DisabledMinutes marks minutes of hour h that cannot be picked.
func (w *TimePanelW) DisabledMinutes(fn func(h, m int) bool) *TimePanelW { w.clock.disM = fn; return w }

// DisabledSeconds marks seconds of h:m that cannot be picked.
func (w *TimePanelW) DisabledSeconds(fn func(h, m, s int) bool) *TimePanelW {
	w.clock.disS = fn
	return w
}

// Height sets the column height.
func (w *TimePanelW) Height(px float32) *TimePanelW { w.height = px; return w }

// HideDisabledOptions leaves disabled options out of the columns.
func (w *TimePanelW) HideDisabledOptions(on bool) *TimePanelW { w.clock.hide = on; return w }

// Changed reports a new value this frame.
func (w *TimePanelW) Changed() bool { w.z.run(); return w.changed }

func (w *TimePanelW) layout() string {
	if w.clock.use12 && w.format == "15:04:05" {
		return "03:04:05 PM"
	}
	return w.format
}

func (w *TimePanelW) cfg() dtTimeCfg {
	h := w.height
	if h == 0 {
		_, tk := dtTheme(w.c)
		h = tk.TimeColumnHeight
	}
	cfg := dtTimeCfg{cols: dtTimeCols(w.format), steps: w.steps, colH: h, base: w.c.now}
	w.clock.apply(&cfg, w.loc)
	return cfg
}

func (w *TimePanelW) build() {
	cfg := w.cfg()
	for _, k := range w.c.KeysFor(w.n.id) {
		if k.Down && dtTimeKey(w.val, cfg, &w.st.ts, k.Key) {
			w.changed = true
		}
	}
	if dtTimeColumns(w.c, w.val, cfg, &w.st.ts) {
		w.changed = true
	}
}

// ---------------------------------------------------------------------------
// TimePicker

type dtTimePickerState struct {
	field    dtFieldState
	draft    time.Time
	ts       dtTimeState
	lastOpen bool
}

// TimePickerW is an input with a dropdown of time columns.
type TimePickerW struct {
	c         *Context
	f         *dtField
	val       *time.Time
	st        *dtTimePickerState
	loc       *Locale
	format    string
	formatSet bool
	steps     [3]int
	clock     dtClock
	changed   bool
	forced    bool
	z         *lazy
}

// TimePicker adds a time input bound to *val (zero = empty). The date part
// of a new value is today's date.
//
// ConfigProvider.Defaults("TimePicker", ...) keys: "format",
// "placeholder", "use12Hours", "hourStep", "minuteStep", "secondStep",
// "allowClear", "width", "size", "variant", "hideDisabledOptions".
func TimePicker(c *Context, val *time.Time) *TimePickerW {
	w := &TimePickerW{c: c, val: val, format: "15:04:05", loc: c.Locale()}
	w.f = dtNewField(c, "timepicker", icon.Clock)
	w.st = State[dtTimePickerState](c, w.f.n.id, nil)
	w.f.st = &w.st.field
	w.f.ph = c.Locale().SelectTime
	w.f.display = func() string { return p2aFormatClock(w.loc, *w.val, w.layout()) }
	def := p2aDefaults(c, "TimePicker")
	w.formatSet = def.str("format", &w.format)
	def.str("placeholder", &w.f.ph)
	def.flag("use12Hours", &w.clock.use12)
	def.integer("hourStep", &w.steps[0])
	def.integer("minuteStep", &w.steps[1])
	def.integer("secondStep", &w.steps[2])
	def.flag("allowClear", &w.f.allowClear)
	def.px("width", &w.f.n.ln.Width)
	def.size(&w.f.size)
	def.str("variant", &w.f.variant)
	def.flag("hideDisabledOptions", &w.clock.hide)
	w.z = c.lazy(w.f.n, w.finish)
	return w
}

func (w *TimePickerW) Placeholder(s string) *TimePickerW { w.f.ph = s; return w }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *TimePickerW) A11yLabel(s string) *TimePickerW { w.f.ed.setName(s); return w }

// Format sets the Go time layout, e.g. "15:04:05" (default), "15:04" or
// with Use12Hours "03:04 PM" (AM / PM are shown with the Locale words).
func (w *TimePickerW) Format(f string) *TimePickerW    { w.format, w.formatSet = f, true; return w }
func (w *TimePickerW) HourStep(n int) *TimePickerW     { w.steps[0] = n; return w }
func (w *TimePickerW) MinuteStep(n int) *TimePickerW   { w.steps[1] = n; return w }
func (w *TimePickerW) SecondStep(n int) *TimePickerW   { w.steps[2] = n; return w }
func (w *TimePickerW) Status(s string) *TimePickerW    { w.f.status = s; return w }
func (w *TimePickerW) Size(s ControlSize) *TimePickerW { w.f.size = s; return w }
func (w *TimePickerW) Width(px float32) *TimePickerW   { w.f.n.ln.Width = px; return w }

// Steps sets the hour, minute and second steps at once.
func (w *TimePickerW) Steps(h, m, s int) *TimePickerW { w.steps = [3]int{h, m, s}; return w }

// Use12Hours shows a 12-hour clock: hours 12, 01 … 11 and an AM / PM
// column; the default format becomes "03:04:05 PM".
func (w *TimePickerW) Use12Hours() *TimePickerW { w.clock.use12 = true; return w }

// DisabledHours marks hours (0-23) that cannot be picked or typed.
func (w *TimePickerW) DisabledHours(fn func(h int) bool) *TimePickerW { w.clock.disH = fn; return w }

// DisabledMinutes marks minutes of hour h that cannot be picked or typed.
func (w *TimePickerW) DisabledMinutes(fn func(h, m int) bool) *TimePickerW {
	w.clock.disM = fn
	return w
}

// DisabledSeconds marks seconds of h:m that cannot be picked or typed.
func (w *TimePickerW) DisabledSeconds(fn func(h, m, s int) bool) *TimePickerW {
	w.clock.disS = fn
	return w
}

// AllowClear toggles the clear button (on by default).
func (w *TimePickerW) AllowClear(on bool) *TimePickerW { w.f.allowClear = on; return w }

// HideDisabledOptions leaves disabled hours, minutes and seconds out of
// the columns instead of greying them (Ant hideDisabledOptions).
func (w *TimePickerW) HideDisabledOptions(on bool) *TimePickerW { w.clock.hide = on; return w }

// Variant sets the frame look: "outlined" (default), "filled",
// "borderless" or "underlined".
func (w *TimePickerW) Variant(v string) *TimePickerW { w.f.variant = v; return w }

// Disabled disables the picker.
func (w *TimePickerW) Disabled(on bool) *TimePickerW {
	if on {
		w.f.setDisabled()
	}
	return w
}

// Layout applies layout options to the input frame.
func (w *TimePickerW) Layout(o ...layout.Option) *TimePickerW {
	w.f.n.ln.Style.Apply(o...)
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *TimePickerW) Open() *TimePickerW { w.forced = true; w.st.field.pop.open = true; return w }

// IsOpen reports whether the dropdown is shown.
func (w *TimePickerW) IsOpen() bool { return w.st.field.pop.open }

// Changed reports a committed value this frame.
func (w *TimePickerW) Changed() bool { w.z.run(); return w.changed }

// layout is the Go layout of the text (12-hour default with Use12Hours).
func (w *TimePickerW) layout() string {
	if w.clock.use12 && !w.formatSet {
		return "03:04:05 PM"
	}
	return w.format
}

func (w *TimePickerW) cfg() dtTimeCfg {
	_, tk := dtTheme(w.c)
	h := tk.TimeColumnHeight
	base := *w.val
	if base.IsZero() {
		base = w.c.now
	}
	cfg := dtTimeCfg{cols: dtTimeCols(w.layout()), steps: w.steps, colH: h, base: base}
	w.clock.apply(&cfg, w.loc)
	return cfg
}

// commit stores t unless its clock is disabled.
func (w *TimePickerW) commit(t time.Time) {
	if w.cfg().disabled(t) {
		return
	}
	if !t.IsZero() && !t.Equal(*w.val) {
		*w.val = t
		w.changed = true
	}
	w.st.field.pop.open = false
	w.st.field.editing = false
	w.c.invalid = true
}

func (w *TimePickerW) parse(s string) (time.Time, bool) {
	cfg := w.cfg()
	t, ok := p2aParseClock(w.loc, s, w.layout(), cfg.base.Location())
	if !ok {
		return t, false
	}
	return dtWithClock(cfg.base, t), true
}

func (w *TimePickerW) finish() {
	c, st, f := w.c, w.st, w.f
	loc := c.Locale()
	if f.n.ln.Width == 0 {
		f.n.ln.Width = 150
		if dtTimeCols(w.layout()) == 2 {
			f.n.ln.Width = 128
		}
		if w.clock.use12 {
			f.n.ln.Width += 32
		}
		if c.Theme.Flavor == theme.Element {
			f.n.ln.Width = 220
		}
	}
	nav, edited, submitted, cleared := f.process(!w.val.IsZero())
	if w.forced && !f.disabled && !f.n.disabled {
		st.field.pop.open = true
	}
	if cleared {
		if !w.val.IsZero() {
			*w.val = time.Time{}
			w.changed = true
		}
		return
	}
	cfg := w.cfg()
	show := func(t time.Time) string { return p2aFormatClock(w.loc, t, w.layout()) }
	if st.field.pop.open && !st.lastOpen {
		st.draft = *w.val
		st.ts.syncAll()
		st.ts.col = 0
		f.setText(show(st.draft))
	}
	st.lastOpen = st.field.pop.open
	if edited {
		if t, ok := w.parse(st.field.buf); ok && !cfg.disabled(t) {
			st.draft = t
			st.ts.syncAll()
		}
	}
	if submitted {
		if t, ok := w.parse(st.field.buf); ok {
			w.commit(t)
		}
	}
	for _, k := range nav {
		switch k.Key {
		case input.KeyEnter:
			t := st.draft
			if p, ok := w.parse(st.field.buf); ok {
				t = p
			}
			w.commit(t)
		default:
			if dtTimeKey(&st.draft, cfg, &st.ts, k.Key) {
				f.setText(show(st.draft))
			}
		}
	}
	if !st.field.pop.open {
		st.lastOpen = false
		return
	}
	th := c.Theme
	defer func() { f.ctl, f.act = st.ts.active() }()
	dtStylePanel(c, entPanel(c, f.n.id, &st.field.pop, false, func() {
		Column(c, func() {
			if dtTimeColumns(c, &st.draft, cfg, &st.ts) {
				f.setText(show(st.draft))
			}
			bad := st.draft.IsZero() || cfg.disabled(st.draft)
			if th.Flavor == theme.Element {
				Row(c, func() {
					Spacer(c)
					if dtTextBtn(c, loc.Cancel, false) {
						st.field.pop.open = false
						c.invalid = true
					}
					if dtTextBtn(c, loc.OK, true) {
						w.commit(st.draft)
					}
				}, layout.H(36), layout.PadVH(0, 5), layout.Middle(), layout.Gap(4)).Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Components.DatePicker.SplitColor)
				})
				return
			}
			dtFooter(c, func() {
				now := c.now.Truncate(time.Second)
				if cfg.cols < 3 {
					now = now.Truncate(time.Minute)
				}
				if !cfg.disabled(now) && dtLink(c, loc.Now) {
					w.commit(now)
				}
				Spacer(c)
				if Button(c, loc.OK).Primary().Small().Disabled(bad).Clicked() {
					w.commit(st.draft)
				}
			})
		})
	}))
}
