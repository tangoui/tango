package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// QueryFilterW is a collapsible query form (Ant Design Pro QueryFilter):
// fields flow in 1–4 columns by width with the buttons in the last cell;
// beyond the first row they fold behind an expand link. ProTable uses it
// for its search form.
//
//	q := ui.QueryFilter(c, fields).Values(&query).Show()
//	if q.Searched() { reload(q.Params()) }
type QueryFilterW struct {
	_ [0]func()

	c          *Context
	fields     []ProField
	vals       *ProValues
	labelWidth float32
	expanded   bool
	searchText string
	resetText  string
	loading    bool

	id       ID
	st       *queryState
	searched bool
	reset    bool
}

type queryState struct {
	vals ProValues
	open bool
	init bool
}

// QueryFilter starts a query form over fields; chain options, then Show.
//
// ConfigProvider.Defaults("QueryFilter", ...) keys: "labelWidth"
// (number), "defaultCollapsed" (bool, false = start expanded),
// "searchText", "resetText" (string).
func QueryFilter(c *Context, fields []ProField) *QueryFilterW {
	q := &QueryFilterW{c: c, fields: fields, labelWidth: 80}
	if v, ok := p2bNum(c, "QueryFilter", "labelWidth"); ok {
		q.labelWidth = float32(v)
	}
	q.expanded = !Prop(c, "QueryFilter", "defaultCollapsed", true)
	q.searchText = Prop(c, "QueryFilter", "searchText", "")
	q.resetText = Prop(c, "QueryFilter", "resetText", "")
	return q
}

// Values binds the values (otherwise the filter keeps its own).
func (q *QueryFilterW) Values(v *ProValues) *QueryFilterW { q.vals = v; return q }

// LabelWidth sets the label width (default 80).
func (q *QueryFilterW) LabelWidth(px float32) *QueryFilterW { q.labelWidth = px; return q }

// DefaultExpanded starts with every field shown.
func (q *QueryFilterW) DefaultExpanded() *QueryFilterW { q.expanded = true; return q }

// SearchText sets the search button's text.
func (q *QueryFilterW) SearchText(s string) *QueryFilterW { q.searchText = s; return q }

// ResetText sets the reset button's text.
func (q *QueryFilterW) ResetText(s string) *QueryFilterW { q.resetText = s; return q }

// Loading spins the search button.
func (q *QueryFilterW) Loading(on bool) *QueryFilterW { q.loading = on; return q }

// Searched reports a search (or a reset, which searches again) in the last
// Show.
func (q *QueryFilterW) Searched() bool { return q.searched }

// ResetClicked reports a reset in the last Show.
func (q *QueryFilterW) ResetClicked() bool { return q.reset }

// Params returns the non-empty values as text (dates as 2006-01-02).
func (q *QueryFilterW) Params() map[string]string { return proParams(q.valsRef()) }

// ID returns the widget ID (valid after Show).
func (q *QueryFilterW) ID() ID { return q.id }

func (q *QueryFilterW) valsRef() ProValues {
	if q.vals != nil {
		if *q.vals == nil {
			*q.vals = ProValues{}
		}
		return *q.vals
	}
	return q.st.vals
}

// perRow returns the number of cells per row for width w (Ant Pro
// breakpoints).
func queryPerRow(w float32) int {
	switch {
	case w <= 0:
		return 3
	case w < 560:
		return 1
	case w < 900:
		return 2
	case w >= 1500:
		return 4
	}
	return 3
}

// Show builds the query form.
func (q *QueryFilterW) Show() *QueryFilterW {
	c := q.c
	th := c.Theme
	loc := c.Locale()
	root := c.box("query-filter", layout.Column, nil)
	q.id = root.n.id
	st := State[queryState](c, q.id, func() queryState { return queryState{vals: ProValues{}, open: q.expanded} })
	q.st = st
	vals := q.valsRef()
	if !st.init {
		st.init = true
		proInitial(q.fields, vals)
	}
	w := float32(0)
	if r, ok := c.rects[q.id]; ok {
		w = r.W
	}
	per := queryPerRow(w)
	var items []*ProField
	for i := range q.fields {
		if fd := &q.fields[i]; fd.Hide == nil || !fd.Hide(vals) {
			items = append(items, fd)
		}
	}
	shown := items
	collapsible := len(items) > per-1
	if !st.open && collapsible {
		shown = items[:max(0, per-1)]
	}
	rows := (len(shown) + per) / per
	gap := float32(24)
	if shad2d(c) {
		gap = 16 // data-table toolbar spacing
	}
	root.Body(func() {
		Column(c, func() {
			for r := 0; r < rows; r++ {
				Row(c, func() {
					for k := 0; k < per; k++ {
						i := r*per + k
						switch {
						case i < len(shown):
							fd := shown[i]
							Row(c, func() { q.field(fd, vals) }, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Middle())
						case i == rows*per-1:
							Row(c, func() { q.buttons(collapsible) }, layout.Grow(1), layout.Basis(0), layout.Gap(8), layout.Middle())
						default:
							Div(c, layout.Grow(1), layout.Basis(0))
						}
					}
				}, layout.Gap(gap))
			}
		}, layout.Gap(gap))
	})
	if q.reset {
		proResetValues(q.fields, vals)
		q.searched = true
	}
	if q.searched {
		c.invalid = true
	}
	_ = th
	_ = loc
	return q
}

func (q *QueryFilterW) buttons(collapsible bool) {
	c, th, st := q.c, q.c.Theme, q.st
	loc := c.Locale()
	Spacer(c)
	rst := q.resetText
	if rst == "" {
		rst = loc.Reset
	}
	sch := q.searchText
	if sch == "" {
		sch = loc.Form.Query
	}
	if Button(c, rst).Clicked() {
		q.reset = true
	}
	if Button(c, sch).Primary().Loading(q.loading).Clicked() {
		q.searched = true
	}
	if collapsible {
		lbl, ic := loc.Expand, icon.Down
		if st.open {
			lbl, ic = loc.Collapse, icon.Up
		}
		if shad2d(c) {
			// A ghost button with the chevron (data-table "View" style).
			b := Button(c, lbl).Text().Icon(ic)
			b.n.setState(a11y.Expandable).stateIf(st.open, a11y.Expanded)
			if b.Clicked() {
				st.open = !st.open
				c.invalid = true
			}
			return
		}
		Row(c, func() {
			lk := Link(c, lbl)
			lk.n.setRole(a11y.RoleButton).setState(a11y.Expandable).stateIf(st.open, a11y.Expanded)
			if lk.Clicked() {
				st.open = !st.open
				c.invalid = true
			}
			Icon(c, ic).Size(12).Color(th.Primary.Base)
		}, layout.Gap(4), layout.Middle())
	}
}

// field is one labelled query control.
func (q *QueryFilterW) field(fd *ProField, vals ProValues) {
	c, th := q.c, q.c.Theme
	Key(c, fd.Key, func() {
		lbl := fd.Label
		if th.Flavor == theme.Ant {
			lbl = c.Locale().WithColon(lbl)
		}
		t := Text(c, lbl).NoWrap().Align(AlignEnd).Layout(layout.W(q.labelWidth), layout.Margin(geom.Insets{Right: 8}))
		if th.Flavor == theme.StyleShadcn {
			t.Weight(text.Medium) // Label: text-sm font-medium, no colon
		}
		dis := fd.Disable != nil && fd.Disable(vals)
		Disabled(c, dis, func() {
			b := Row(c, func() { proFieldControl(c, fd, vals) }, layout.Grow(1), layout.MinW(0), layout.Middle())
			b.n.fill(qfField{label: fd.Label, id: t.n.id}) // the label names the control
		})
	})
}

// qfField passes a QueryFilter label to the control next to it (like a
// FormItem label).
type qfField struct {
	label string
	id    ID
}

func (f qfField) field() (string, ID, bool, string) { return f.label, f.id, false, "" }
func (f qfField) semantics(*a11y.Node)              {}
