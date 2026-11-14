package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// LightFilterW is a compact filter bar (Ant Design Pro LightFilter): each
// field is a "label: value" pill; clicking it opens a popup with the
// field's editor and clear / confirm buttons. It takes the same field
// definitions as QueryFilter and ProForm.
//
//	lf := ui.LightFilter(c, fields).Values(&filter).Show()
//	if lf.Changed() { reload(lf.Params()) }
type LightFilterW struct {
	_ [0]func()

	c        *Context
	fields   []ProField
	vals     *ProValues
	bordered bool
	size     ControlSize
	sizeSet  bool
	maxText  float32
	maxPills int // fields shown as pills before "more filters" (0: all)

	id      ID
	st      *pro3LightState
	changed bool
}

type pro3LightState struct {
	vals    ProValues
	init    bool
	open    string    // key of the field whose popup is open
	draft   ProValues // popup value
	panel   ID
	focusIn bool // opened from the keyboard: move focus into the popup
}

// LightFilter starts a filter bar over fields; chain options, then Show.
//
// ConfigProvider.Defaults("LightFilter", ...) keys: "size", "bordered",
// "collapse" (number of pills before "more filters").
func LightFilter(c *Context, fields []ProField) *LightFilterW {
	f := &LightFilterW{c: c, fields: fields, maxText: 160}
	if sz, ok := proPropSize(c, "LightFilter"); ok {
		f.size, f.sizeSet = sz, true
	}
	f.bordered = Prop(c, "LightFilter", "bordered", false)
	if n, ok := proPropInt(c, "LightFilter", "collapse"); ok {
		f.maxPills = n
	}
	return f
}

// Values binds the values (otherwise the filter keeps its own).
func (f *LightFilterW) Values(v *ProValues) *LightFilterW { f.vals = v; return f }

// Bordered draws an outline around every pill.
func (f *LightFilterW) Bordered() *LightFilterW { f.bordered = true; return f }

// Size sets the pill size (default: the global size).
func (f *LightFilterW) Size(s ControlSize) *LightFilterW { f.size, f.sizeSet = s, true; return f }

// Collapse shows at most n fields as pills; the others are edited
// together in a "more filters" popup (n <= 0: every field is a pill).
func (f *LightFilterW) Collapse(n int) *LightFilterW { f.maxPills = n; return f }

// Changed reports a value confirmed or cleared in the last Show.
func (f *LightFilterW) Changed() bool { return f.changed }

// Params returns the non-empty values as text (dates as 2006-01-02).
func (f *LightFilterW) Params() map[string]string { return proParams(f.valsRef()) }

// OpenKey returns the key of the field whose popup is open ("" if none).
func (f *LightFilterW) OpenKey() string {
	if f.st == nil {
		return ""
	}
	return f.st.open
}

// ID returns the widget ID (valid after Show).
func (f *LightFilterW) ID() ID { return f.id }

func (f *LightFilterW) valsRef() ProValues {
	if f.vals != nil {
		if *f.vals == nil {
			*f.vals = ProValues{}
		}
		return *f.vals
	}
	return f.st.vals
}

// Show builds the bar.
func (f *LightFilterW) Show() *LightFilterW {
	c := f.c
	if !f.sizeSet {
		f.size = c.defaultSize()
		if _, set := c.ComponentSize(); !set && shad2d(c) {
			f.size = Small // data-table filter buttons are h-8
		}
	}
	root := c.box("light-filter", layout.Row, []layout.Option{layout.Gap(8), layout.Middle(), layout.WrapItems()})
	f.id = root.n.id
	st := State[pro3LightState](c, f.id, func() pro3LightState { return pro3LightState{vals: ProValues{}} })
	f.st = st
	vals := f.valsRef()
	if !st.init {
		st.init = true
		proInitial(f.fields, vals)
	}
	root.Semantics(a11y.RoleToolbar, c.Locale().LightFilter.Name)
	root.Body(func() {
		var rest []*ProField
		shown := 0
		for i := range f.fields {
			fd := &f.fields[i]
			if fd.Hide != nil && fd.Hide(vals) {
				continue
			}
			if f.maxPills > 0 && shown >= f.maxPills {
				rest = append(rest, fd)
				continue
			}
			shown++
			Key(c, fd.Key, func() { f.pill(fd, vals) })
		}
		if len(rest) > 0 {
			f.morePill(rest, vals)
		}
	})
	return f
}

// pro3Empty reports an empty filter value.
func pro3Empty(v any) bool { return proValueText(v) == "" }

// pill is one field: the trigger and, when open, its popup.
func (f *LightFilterW) pill(fd *ProField, vals ProValues) {
	c, th, st := f.c, f.c.Theme, f.st
	loc := c.Locale()
	v := vals[fd.Key]
	has := !pro3Empty(v)
	open := st.open == fd.Key
	h := c.controlHeight(f.size)
	fs := c.controlFont(f.size)
	label := fd.Label
	if has {
		label = loc.WithColon(fd.Label)
	}
	rad := c.controlRadius(f.size)
	var clear bool
	vt := ""
	if has {
		vt = pro3ValueText(c, fd.ValueType, proOptions(fd, vals), v)
	}
	shad := shad2d(c)
	b := Row(c, func() {
		if shad {
			// data-table faceted filter: dashed outline button, circled
			// plus, the title, then a separator and a secondary badge with
			// the value.
			f.shadPillBody(fd, has, vt, fs, &clear)
			return
		}
		Text(c, label).Size(fs).NoWrap().Color(th.Text)
		if has {
			Text(c, vt).Size(fs).NoWrap().Ellipsis().
				Color(th.Text).Layout(layout.MaxW(f.maxText), layout.MinW(0))
		}
		if has {
			if pro3IconBtn(c, locFmt(loc.LightFilter.ClearFmt, fd.Label), icon.CloseCircle, 16) {
				clear = true
			}
		} else {
			Icon(c, icon.Down).Size(12).Color(th.TextTertiary)
		}
	}, layout.H(h), layout.PadVH(0, 8), layout.Gap(4), layout.Middle())
	b.n.label = fd.Label
	// A Tab stop: Enter / Space open the popup (Interaction.Clicked), Esc
	// closes it and focus returns here (the popup's Anchor).
	ia := b.Focusable(input.CursorPointer)
	name := fd.Label
	if has {
		name = locFmt(loc.LightFilter.PillFmt, fd.Label, vt)
	}
	b.Semantics(a11y.RoleButton, name)
	b.A11yState(a11y.Expandable | a11y.HasPopup)
	if open {
		b.A11yState(a11y.Expanded)
		if c.a11yOn && st.panel != 0 {
			panel := st.panel
			b.n.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(panel) }))
		}
	}
	bg := theme.Color{}
	switch {
	case shad:
		if ia.Hovered || open {
			bg = th.ItemHoverBg
		}
	case has || open:
		bg = th.FillTertiary
	case ia.Hovered:
		bg = th.FillTertiary
	}
	b.Bg(bg).Radius(rad)
	if shad {
		b.DashedBorder(1, th.Border)
	} else if f.bordered {
		bc := th.Border
		if ia.Hovered || open {
			bc = th.Primary.Base
		}
		b.Border(1, bc)
	}
	if ia.FocusVisible {
		b.PaintOver(func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(rad)) })
	}
	if clear {
		delete(vals, fd.Key)
		if open {
			st.open = ""
		}
		f.changed = true
		c.invalid = true
		return
	}
	if ia.Clicked {
		if open {
			st.open = ""
		} else {
			f.openField(fd, vals)
			// Keyboard activation (focus ring showing): move focus into
			// the popup's editor once it has been laid out.
			st.focusIn = ia.FocusVisible
		}
		c.invalid = true
	}
	if st.open == fd.Key {
		f.popup(fd, vals, b.ID())
	}
}

// openField starts editing fd with a copy of its value.
func (f *LightFilterW) openField(fd *ProField, vals ProValues) {
	st := f.st
	st.open = fd.Key
	st.draft = ProValues{}
	if v, ok := vals[fd.Key]; ok {
		if s, ok := v.([]string); ok {
			v = append([]string(nil), s...)
		}
		st.draft[fd.Key] = v
	}
}

// popup is the editor panel of the open field.
func (f *LightFilterW) popup(fd *ProField, vals ProValues, anchor ID) {
	c, th, st := f.c, f.c.Theme, f.st
	loc := c.Locale()
	draft := st.draft
	if draft == nil {
		draft = ProValues{}
		st.draft = draft
	}
	width := float32(240)
	switch fd.ValueType {
	case ValueDateRange, ValueCheckbox, ValueMultiSelect, ValueRadioButton, ValueSegmented:
		width = 320
	}
	if fd.Width > 0 {
		width = fd.Width + 32
	}
	var ok, clr bool
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomStart, Offset: 4, Flip: true}, func() {
		Column(c, func() {
			efd := *fd
			efd.Width = 0
			Row(c, func() { proFieldControl(c, &efd, draft) }, layout.Middle())
		}, layout.PadVH(16, 16), layout.W(width))
		Row(c, func() {
			if Link(c, loc.Clear).Clicked() {
				clr = true
			}
			Spacer(c)
			if Button(c, loc.Confirm).Primary().Small().Clicked() {
				ok = true
			}
		}, layout.PadVH(8, 12), layout.Middle()).Paint(func(l *paint.List, r geom.Rect) {
			dataHLine(l, r.X, r.Y, r.W, th.Split)
		})
	})
	ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.ShadowSecondary)
	if th.PopupBorder.A > 0 {
		ov.Border(1, th.PopupBorder)
	}
	ov.Semantics(a11y.RoleDialog, fd.Label)
	st.panel = ov.ID()
	if st.focusIn {
		// The panel's focus stops are known from the frame after it
		// opened: request that frame, then focus its first stop.
		if l, ok := c.overlayLayer[st.panel]; ok {
			if id := c.firstFocusAt(l); id != 0 {
				c.SetFocus(id)
			}
			st.focusIn = false
		} else {
			c.animating = true // one more frame
		}
	}
	switch {
	case ok:
		if v, has := draft[fd.Key]; has && !pro3Empty(v) {
			vals[fd.Key] = v
		} else {
			delete(vals, fd.Key)
		}
		st.open = ""
		f.changed = true
		c.invalid = true
	case clr:
		delete(vals, fd.Key)
		delete(draft, fd.Key)
		st.open = ""
		f.changed = true
		c.invalid = true
	case c.Dismissed(st.panel, anchor):
		st.open = ""
		c.invalid = true
	}
}

// pro3MoreKey is pro3LightState.open while the "more filters" popup is open.
const pro3MoreKey = "\x00more"

// morePill is the "more filters" trigger holding the collapsed fields
// and, when open, their popup.
func (f *LightFilterW) morePill(rest []*ProField, vals ProValues) {
	c, th, st := f.c, f.c.Theme, f.st
	loc := c.Locale()
	open := st.open == pro3MoreKey
	set := 0
	for _, fd := range rest {
		if !pro3Empty(vals[fd.Key]) {
			set++
		}
	}
	label := loc.LightFilter.More
	if set > 0 {
		label = locFmt(loc.LightFilter.MoreFmt, set)
	}
	h := c.controlHeight(f.size)
	fs := c.controlFont(f.size)
	rad := c.controlRadius(f.size)
	shad := shad2d(c)
	b := Row(c, func() {
		Icon(c, icon.Filter).Size(fs).Color(th.TextSecondary)
		Text(c, label).Size(fs).NoWrap().Color(th.Text)
		Icon(c, icon.Down).Size(12).Color(th.TextTertiary)
	}, layout.H(h), layout.PadVH(0, 8), layout.Gap(4), layout.Middle())
	b.n.label = loc.LightFilter.More
	ia := b.Focusable(input.CursorPointer)
	b.Semantics(a11y.RoleButton, label)
	b.A11yState(a11y.Expandable | a11y.HasPopup)
	if open {
		b.A11yState(a11y.Expanded)
		if c.a11yOn && st.panel != 0 {
			panel := st.panel
			b.n.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(panel) }))
		}
	}
	switch {
	case shad:
		if open || ia.Hovered {
			b.Bg(th.ItemHoverBg)
		}
	case set > 0 || open || ia.Hovered:
		b.Bg(th.FillTertiary)
	}
	b.Radius(rad)
	if shad {
		b.DashedBorder(1, th.Border)
	} else if f.bordered {
		bc := th.Border
		if ia.Hovered || open {
			bc = th.Primary.Base
		}
		b.Border(1, bc)
	}
	if ia.FocusVisible {
		b.PaintOver(func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(rad)) })
	}
	if ia.Clicked {
		if open {
			st.open = ""
		} else {
			st.open = pro3MoreKey
			st.draft = ProValues{}
			for _, fd := range rest {
				if v, ok := vals[fd.Key]; ok {
					if s, ok := v.([]string); ok {
						v = append([]string(nil), s...)
					}
					st.draft[fd.Key] = v
				}
			}
			st.focusIn = ia.FocusVisible
		}
		c.invalid = true
	}
	if st.open == pro3MoreKey {
		f.morePopup(rest, vals, b.ID(), label)
	}
}

// morePopup edits the collapsed fields together; Confirm applies all.
func (f *LightFilterW) morePopup(rest []*ProField, vals ProValues, anchor ID, title string) {
	c, th, st := f.c, f.c.Theme, f.st
	loc := c.Locale()
	draft := st.draft
	if draft == nil {
		draft = ProValues{}
		st.draft = draft
	}
	var ok, clr bool
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomStart, Offset: 4, Flip: true}, func() {
		Column(c, func() {
			for _, fd := range rest {
				fd := fd
				Key(c, fd.Key, func() {
					Row(c, func() {
						t := Text(c, loc.WithColon(fd.Label)).NoWrap().Color(th.Text).Layout(layout.W(88))
						efd := *fd
						efd.Width = 0
						b := Row(c, func() { proFieldControl(c, &efd, draft) }, layout.Grow(1), layout.MinW(0), layout.Middle())
						b.n.fill(qfField{label: fd.Label, id: t.n.id})
					}, layout.Gap(8), layout.Middle())
				})
			}
		}, layout.PadVH(16, 16), layout.W(360), layout.Gap(12))
		Row(c, func() {
			if Link(c, loc.Clear).Clicked() {
				clr = true
			}
			Spacer(c)
			if Button(c, loc.Confirm).Primary().Small().Clicked() {
				ok = true
			}
		}, layout.PadVH(8, 12), layout.Middle()).Paint(func(l *paint.List, r geom.Rect) {
			dataHLine(l, r.X, r.Y, r.W, th.Split)
		})
	})
	ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.ShadowSecondary)
	if th.PopupBorder.A > 0 {
		ov.Border(1, th.PopupBorder)
	}
	ov.Semantics(a11y.RoleDialog, title)
	st.panel = ov.ID()
	if st.focusIn {
		if l, ok := c.overlayLayer[st.panel]; ok {
			if id := c.firstFocusAt(l); id != 0 {
				c.SetFocus(id)
			}
			st.focusIn = false
		} else {
			c.animating = true
		}
	}
	switch {
	case ok:
		for _, fd := range rest {
			if v, has := draft[fd.Key]; has && !pro3Empty(v) {
				vals[fd.Key] = v
			} else {
				delete(vals, fd.Key)
			}
		}
		st.open = ""
		f.changed = true
		c.invalid = true
	case clr:
		for _, fd := range rest {
			delete(vals, fd.Key)
			delete(draft, fd.Key)
		}
		st.open = ""
		f.changed = true
		c.invalid = true
	case c.Dismissed(st.panel, anchor):
		st.open = ""
		c.invalid = true
	}
}
