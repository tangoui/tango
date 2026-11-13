package ui

// RangePicker.ShowTime: start / end time columns under the two month
// panels and an OK footer (Ant RangePicker showTime, Element
// type="datetimerange"). Days and times are drafts in dtRangeState.draft
// until OK commits them, ordered.

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// draftDay sets the date of the end being picked, keeping its clock
// (00:00:00 for a new end); after the start the end is picked next.
func (w *RangePickerW) draftDay(d time.Time) {
	st := w.st
	i := st.pick
	old := st.draft[i]
	loc := d.Location()
	if !old.IsZero() {
		loc = old.Location()
	}
	st.draft[i] = time.Date(d.Year(), d.Month(), d.Day(), old.Hour(), old.Minute(), old.Second(), 0, loc)
	if i == 0 {
		st.a, st.pick, st.active = dtDay(d), 1, 1
	} else {
		st.pick, st.active = 0, 1
	}
	w.c.invalid = true
}

// activate switches the end being edited without dropping the drafts.
func (w *RangePickerW) activate(seg int) {
	st := w.st
	st.active, st.pick = seg, seg
	st.a = time.Time{}
	if seg == 1 && !st.draft[0].IsZero() {
		st.a = dtDay(st.draft[0])
	}
	if !st.draft[seg].IsZero() {
		st.cursor = dtDay(st.draft[seg])
	}
	w.c.invalid = true
}

func (w *RangePickerW) draftReady() bool {
	return !w.st.draft[0].IsZero() && !w.st.draft[1].IsZero()
}

// commitDraft commits the drafts (ordered) and closes.
func (w *RangePickerW) commitDraft() {
	if !w.draftReady() {
		return
	}
	a, b := w.st.draft[0], w.st.draft[1]
	if b.Before(a) {
		a, b = b, a
	}
	w.set([2]time.Time{a, b})
}

// timeLayout is the clock part of the trigger layout.
func (w *RangePickerW) timeLayout() string {
	if strings.Contains(w.layout(), "05") {
		return "15:04:05"
	}
	return "15:04"
}

// timeArea builds the start / end time columns and the OK footer.
func (w *RangePickerW) timeArea() {
	c, st := w.c, w.st
	th := c.Theme
	el := th.Flavor == theme.Element
	loc := c.Locale()
	tf := w.timeLayout()
	colH := 6 * dtTimeItemH(th)
	if el {
		colH = 5 * dtTimeItemH(th)
	}
	labels := [2]string{loc.TimePicker.StartTime, loc.TimePicker.EndTime}
	Row(c, func() {
		for i := 0; i < 2; i++ {
			i := i
			Key(c, i, func() {
				col := Column(c, func() {
					head := labels[i]
					if s := dtFormat(st.draft[i], tf); s != "" {
						head += "  " + s
					}
					t := Text(c, head).NoWrap().Size(th.FontSizeSM)
					if st.active == i {
						t.Color(th.Primary.Base)
					} else {
						t.Color(th.TextSecondary)
					}
					TimePanel(c, &st.draft[i]).Format(tf).Height(colH).A11yLabel(labels[i])
				}, layout.W(dtPanelWidth(th)), layout.PadVH(8, 12), layout.Gap(4), layout.Middle())
				if i == 0 {
					col.Paint(func(l *paint.List, r geom.Rect) { dtEndDivider(c, l, r, th) })
				}
			})
		}
	}).Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Components.DatePicker.SplitColor)
	})
	dtFooter(c, func() {
		if el {
			if dtTextBtn(c, loc.Clear, false) {
				st.draft = [2]time.Time{}
				st.pick, st.active, st.a = 0, 0, time.Time{}
				c.invalid = true
			}
			Spacer(c)
			if dtPlainBtn(c, loc.OK) {
				w.commitDraft()
			}
			return
		}
		Spacer(c)
		if Button(c, loc.OK).Primary().Small().Disabled(!w.draftReady()).Clicked() {
			w.commitDraft()
		}
	})
}
