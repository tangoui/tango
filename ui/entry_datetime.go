package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type dtDateTimeState struct {
	field    dtFieldState
	draft    time.Time
	date     time.Time // day picked in the embedded month panel
	panel    dtPanelState
	ts       dtTimeState
	lastOpen bool
}

// DateTimePickerW is an input whose dropdown combines a month calendar with
// time columns; the value is committed with "确定" or Enter.
type DateTimePickerW struct {
	_ [0]func()

	c       *Context
	f       *dtField
	val     *time.Time
	st      *dtDateTimeState
	format  string
	steps   [3]int
	changed bool
	forced  bool
	z       *lazy
}

// DateTimePicker adds a date-time input bound to *val (zero = empty).
func DateTimePicker(c *Context, val *time.Time) *DateTimePickerW {
	w := &DateTimePickerW{c: c, val: val, format: "2006-01-02 15:04:05"}
	w.f = dtNewField(c, "datetimepicker", icon.Calendar)
	w.st = State[dtDateTimeState](c, w.f.n.id, nil)
	w.f.st = &w.st.field
	w.f.ph = c.Locale().SelectDateTime
	w.f.display = func() string { return dtFormat(*w.val, w.format) }
	// ConfigProvider.Defaults("DateTimePicker", ...): "format",
	// "placeholder", "allowClear", "width", "size", "variant".
	def := p2aDefaults(c, "DateTimePicker")
	def.str("format", &w.format)
	def.str("placeholder", &w.f.ph)
	def.flag("allowClear", &w.f.allowClear)
	def.px("width", &w.f.n.ln.Width)
	def.size(&w.f.size)
	def.str("variant", &w.f.variant)
	w.z = c.lazy(w.f.n, w.finish)
	return w
}

func (w *DateTimePickerW) Placeholder(s string) *DateTimePickerW { w.f.ph = s; return w }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *DateTimePickerW) A11yLabel(s string) *DateTimePickerW { w.f.ed.setName(s); return w }

// Format sets the Go layout (default "2006-01-02 15:04:05"); omitting the
// seconds hides the second column.
func (w *DateTimePickerW) Format(f string) *DateTimePickerW    { w.format = f; return w }
func (w *DateTimePickerW) Steps(h, m, s int) *DateTimePickerW  { w.steps = [3]int{h, m, s}; return w }
func (w *DateTimePickerW) Status(s string) *DateTimePickerW    { w.f.status = s; return w }
func (w *DateTimePickerW) Size(s ControlSize) *DateTimePickerW { w.f.size = s; return w }
func (w *DateTimePickerW) Width(px float32) *DateTimePickerW   { w.f.n.ln.Width = px; return w }
func (w *DateTimePickerW) AllowClear(on bool) *DateTimePickerW { w.f.allowClear = on; return w }

// Variant sets the frame look: "outlined" (default), "filled",
// "borderless" or "underlined".
func (w *DateTimePickerW) Variant(v string) *DateTimePickerW { w.f.variant = v; return w }
func (w *DateTimePickerW) Layout(o ...layout.Option) *DateTimePickerW {
	flexbox.Apply(&w.f.n.ln.Style, o...)
	return w
}

// Disabled disables the picker.
func (w *DateTimePickerW) Disabled(on bool) *DateTimePickerW {
	if on {
		w.f.setDisabled()
	}
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *DateTimePickerW) Open() *DateTimePickerW {
	w.forced = true
	w.st.field.pop.open = true
	return w
}

// IsOpen reports whether the dropdown is shown.
func (w *DateTimePickerW) IsOpen() bool { return w.st.field.pop.open }

// Changed reports a committed value this frame.
func (w *DateTimePickerW) Changed() bool { w.z.run(); return w.changed }

func (w *DateTimePickerW) loc() *time.Location {
	if !w.val.IsZero() {
		return w.val.Location()
	}
	return w.c.now.Location()
}

func (w *DateTimePickerW) commit(t time.Time) {
	if !t.IsZero() && !t.Equal(*w.val) {
		*w.val = t
		w.changed = true
	}
	w.st.field.pop.open = false
	w.st.field.editing = false
	w.c.invalid = true
}

func (w *DateTimePickerW) finish() {
	c, st, f := w.c, w.st, w.f
	loc := c.Locale()
	th := c.Theme
	if f.n.ln.Width == 0 {
		f.n.ln.Width = 200
		if th.Flavor == theme.Element {
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
	if st.field.pop.open && !st.lastOpen {
		st.draft = *w.val
		st.date = time.Time{}
		if !w.val.IsZero() {
			st.date = dtDayUTC(*w.val)
		}
		st.ts.syncAll()
		st.ts.col = 0
		st.panel.sync(st.date, c.now)
		f.setText(dtFormat(st.draft, w.format))
	}
	st.lastOpen = st.field.pop.open
	if edited {
		if t, ok := dtParse(st.field.buf, w.format, w.loc()); ok {
			st.draft = t
			st.date = dtDayUTC(t)
			st.ts.syncAll()
			st.panel.sync(st.date, c.now)
		}
	}
	if submitted {
		if t, ok := dtParse(st.field.buf, w.format, w.loc()); ok {
			w.commit(t)
		}
	}
	base := st.draft
	if base.IsZero() {
		base = c.now
		if !st.date.IsZero() {
			base = time.Date(st.date.Year(), st.date.Month(), st.date.Day(), 0, 0, 0, 0, w.loc())
		}
	}
	hh := th.Components.DatePicker.HeaderHeight
	colH := dtPanelHeight(th) - (hh + 4) // the month panel height minus the time header and 4px pad
	cfg := dtTimeCfg{cols: dtTimeCols(w.format), steps: w.steps, colH: colH, base: base}
	for _, k := range nav {
		if k.Key == input.KeyEnter {
			t := st.draft
			if p, ok := dtParse(st.field.buf, w.format, w.loc()); ok {
				t = p
			}
			w.commit(t)
			continue
		}
		if dtTimeKey(&st.draft, cfg, &st.ts, k.Key) {
			f.setText(dtFormat(st.draft, w.format))
		}
	}
	if !st.field.pop.open {
		st.lastOpen = false
		return
	}
	// Up / Down step the active time column (Left / Right switch columns);
	// the date is picked with the pointer or typed into the field.
	defer func() { f.ctl, f.act = st.ts.active() }()
	dtStylePanel(c, entPanel(c, f.n.id, &st.field.pop, false, func() {
		Column(c, func() {
			Row(c, func() {
				if d, ok := dtDatePanel(c, &st.panel, st.date, nil, false, nil); ok {
					st.date = d
					clock := st.draft
					st.draft = time.Date(d.Year(), d.Month(), d.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, w.loc())
					f.setText(dtFormat(st.draft, w.format))
					c.invalid = true
				}
				Column(c, func() {
					tf := "15:04:05"
					if cfg.cols < 3 {
						tf = "15:04"
					}
					s := dtFormat(st.draft, tf)
					hn := c.add("dt-time-head", s)
					hn.ln.Height = hh
					hn.paint = func(l *paint.List, r geom.Rect) {
						l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Components.DatePicker.SplitColor)
						if s == "" {
							return
						}
						wt := text.Weight(th.FontWeightStrong)
						tw := entTextW(c, s, th.FontSize, wt)
						entText(c, l, s, r.X+(r.W-tw)/2, r, th.FontSize, wt, th.Text, 0)
					}
					if dtTimeColumns(c, &st.draft, cfg, &st.ts) {
						f.setText(dtFormat(st.draft, w.format))
						if st.date.IsZero() {
							st.date = dtDayUTC(st.draft)
						}
					}
				}).Paint(func(l *paint.List, r geom.Rect) {
					x := r.X // divider on the inline-start edge, next to the days
					if c.IsRTL() {
						x = r.Right() - 1
					}
					l.Fill(geom.Rect{X: x, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, th.Components.DatePicker.SplitColor)
				})
			})
			dtFooter(c, func() {
				now := func() time.Time {
					n := c.now.Truncate(time.Second)
					if cfg.cols < 3 {
						n = n.Truncate(time.Minute)
					}
					return n
				}
				if th.Flavor == theme.Element {
					if dtTextBtn(c, loc.Now, true) {
						w.commit(now())
					}
					Spacer(c)
					if dtPlainBtn(c, loc.OK) {
						w.commit(st.draft)
					}
					return
				}
				if dtLink(c, loc.Now) {
					w.commit(now())
				}
				Spacer(c)
				if Button(c, loc.OK).Primary().Small().Disabled(st.draft.IsZero()).Clicked() {
					w.commit(st.draft)
				}
			})
		})
	}))
}
