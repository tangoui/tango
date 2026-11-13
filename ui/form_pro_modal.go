package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// ModalFormW is a ProForm inside a Modal (Ant Design Pro ModalForm): the
// dialog's OK button submits (spinning while OnFinish runs) and the dialog
// closes once the values are saved.
type ModalFormW struct {
	c        *Context
	open     *bool
	title    string
	fields   []ProField
	vals     *ProValues
	onFinish func(vals ProValues, done func(error))
	width    float32
	cols     int
	drawer   bool
	finished bool
	keep     bool
}

type modalFormState struct {
	busy    bool
	submit  bool
	wasOpen bool
}

// ModalForm starts a dialog form bound to open; chain options, then Show.
//
// ConfigProvider.Defaults("ModalForm" / "DrawerForm", ...) keys: "width",
// "columns" (number).
func ModalForm(c *Context, open *bool, title string, fields []ProField) *ModalFormW {
	return (&ModalFormW{c: c, open: open, title: title, fields: fields, cols: 1}).defaults("ModalForm")
}

func (m *ModalFormW) defaults(comp string) *ModalFormW {
	if v, ok := p2bNum(m.c, comp, "width"); ok && v > 0 {
		m.width = float32(v)
	}
	if n, ok := proPropInt(m.c, comp, "columns"); ok && n > 0 {
		m.cols = n
	}
	return m
}

// DrawerForm is ModalForm in a right-side drawer (Ant Design Pro
// DrawerForm).
func DrawerForm(c *Context, open *bool, title string, fields []ProField) *ModalFormW {
	return (&ModalFormW{c: c, open: open, title: title, fields: fields, cols: 1, drawer: true}).defaults("DrawerForm")
}

// Values binds the values.
func (m *ModalFormW) Values(v *ProValues) *ModalFormW { m.vals = v; return m }

// OnFinish receives the validated values; call done (from any goroutine)
// when saved. A nil error closes the dialog.
func (m *ModalFormW) OnFinish(fn func(vals ProValues, done func(error))) *ModalFormW {
	m.onFinish = fn
	return m
}

// Width sets the dialog width (drawer: its size).
func (m *ModalFormW) Width(px float32) *ModalFormW { m.width = px; return m }

// Columns lays the fields out in n grid columns.
func (m *ModalFormW) Columns(n int) *ModalFormW { m.cols = max(1, n); return m }

// KeepValues keeps the values after closing (default: reset on open).
func (m *ModalFormW) KeepValues() *ModalFormW { m.keep = true; return m }

// Finished reports a completed submit (the dialog just closed), once.
func (m *ModalFormW) Finished() bool { return m.finished }

// Show builds the dialog (nothing while closed).
func (m *ModalFormW) Show() *ModalFormW {
	c := m.c
	loc := c.Locale()
	st := State[modalFormState](c, hashID(c.scope, "modal-form", m.title), nil)
	if m.vals == nil {
		m.vals = State[ProValues](c, hashID(c.scope, "modal-form-vals", m.title), func() ProValues { return ProValues{} })
	}
	if *m.open && !st.wasOpen && !m.keep {
		*m.vals = ProValues{}
	}
	st.wasOpen = *m.open
	var pf *ProFormW
	form := func() {
		pf = ProForm(c, m.fields).Values(m.vals).Columns(m.cols).NoSubmitter()
		if m.onFinish != nil {
			pf.OnFinish(m.onFinish)
		}
		pf.Show()
		if st.submit {
			st.submit = false
			pf.Submit()
		}
		st.busy = pf.Submitting()
		if pf.Finished() {
			m.finished = true
			*m.open = false
			c.invalid = true
		}
	}
	if m.drawer {
		d := Drawer(c, m.open, m.title)
		if m.width > 0 {
			d.Size(m.width)
		}
		d.Footer(func() {
			if Button(c, loc.Cancel).Clicked() {
				*m.open = false
				c.invalid = true
			}
			if Button(c, loc.OK).Primary().Loading(st.busy).Clicked() {
				st.submit = true
				c.invalid = true
			}
		})
		d.Body(form)
		return m
	}
	md := Modal(c, m.open, m.title).ConfirmLoading(st.busy)
	if m.width > 0 {
		md.Width(m.width)
	}
	md.Body(func() { Column(c, form, layout.Padding(geom.Insets{Top: 12})) })
	if md.Ok() {
		st.submit = true
		c.invalid = true
	}
	return m
}
