package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// ProDescItem is one entry of ProDescriptions. ValueType and ValueEnum use
// the ProTable / ProForm value types (text, money, digit, percent, date,
// dateTime, select, status, tag, progress, textarea ...): they format the
// value and pick the inline editor.
type ProDescItem struct {
	Key, Label string
	ValueType  string
	ValueEnum  []ProEnum
	Span       int // columns taken (default 1)

	// ReadOnly keeps the item out of inline editing.
	ReadOnly bool
	// Render replaces the value display (all holds every value).
	Render func(c *Context, v any, all ProValues)
	// Field configures the inline editor (Required, Rules, Min / Max,
	// Placeholder, Options ...). Key, Label, ValueType and ValueEnum
	// default to the item's.
	Field *ProField
}

// ProDescriptionsW is a description list driven by item definitions
// (Ant Design Pro ProDescriptions): values formatted by value type, data
// loaded by a request function with a loading state, and inline editing
// of single items with save / cancel.
//
//	ui.ProDescriptions(c, items).Title("Rule").Column(2).Bordered().
//		Request(func() (map[string]any, error) { return api.Rule(id) }).
//		Editable(func(key string, v any, all ui.ProValues, done func(error)) { go save(key, v, done) }).
//		Show()
type ProDescriptionsW struct {
	_ [0]func()

	c        *Context
	items    []ProDescItem
	title    string
	extra    func()
	cols     int
	bordered bool
	size     ControlSize
	sizeSet  bool
	vertical bool
	data     *ProValues
	request  func() (map[string]any, error)
	editable bool
	onSave   func(key string, v any, all ProValues, done func(error))

	id    ID
	st    *pro3DescState
	saved string
}

type pro3DescState struct {
	vals      ProValues
	requested bool
	loading   bool
	err       error
	editing   map[string]ProValues // draft value per item being edited
	errs      map[string]string    // validation / save error per item
	saving    map[string]bool
	reqRes    pro3Async[pro3DescReq]
	saveRes   pro3Async[pro3DescSave]
	seq       int
	checkRes  pro3Async[pro3DescCheck]
	checkGen  map[string]int // AsyncValidate run per item (stale results are dropped)
	checking  map[string]bool
}

// pro3DescCheck is the answer of an item's AsyncValidate.
type pro3DescCheck struct {
	key string
	gen int
	msg string
}

type pro3DescReq struct {
	seq  int
	vals map[string]any
	err  error
}

type pro3DescSave struct {
	key string
	v   any
	err error
}

// ProDescriptions starts a description list over items; chain options,
// then Show.
func ProDescriptions(c *Context, items []ProDescItem) *ProDescriptionsW {
	p := &ProDescriptionsW{c: c, items: items, cols: 3}
	// ConfigProvider.Defaults("ProDescriptions", ...) keys: "column"
	// (number), "bordered" (bool), "size", "layout" ("horizontal" |
	// "vertical").
	if n, ok := proPropInt(c, "ProDescriptions", "column"); ok && n > 0 {
		p.cols = n
	}
	p.bordered = Prop(c, "ProDescriptions", "bordered", false)
	if sz, ok := proPropSize(c, "ProDescriptions"); ok {
		p.size, p.sizeSet = sz, true
	}
	p.vertical = Prop(c, "ProDescriptions", "layout", "horizontal") == "vertical"
	return p
}

// Title sets the heading.
func (p *ProDescriptionsW) Title(s string) *ProDescriptionsW { p.title = s; return p }

// Extra builds actions right of the heading.
func (p *ProDescriptionsW) Extra(fn func()) *ProDescriptionsW { p.extra = fn; return p }

// Column sets the number of columns (default 3).
func (p *ProDescriptionsW) Column(n int) *ProDescriptionsW { p.cols = max(1, n); return p }

// Bordered draws the table-like bordered style.
func (p *ProDescriptionsW) Bordered() *ProDescriptionsW { p.bordered = true; return p }

// Size sets the density (Large default, Middle, Small).
func (p *ProDescriptionsW) Size(s ControlSize) *ProDescriptionsW {
	p.size, p.sizeSet = s, true
	return p
}

// Vertical puts labels above values.
func (p *ProDescriptionsW) Vertical() *ProDescriptionsW { p.vertical = true; return p }

// DataSource binds the values (inline edits are written into it).
func (p *ProDescriptionsW) DataSource(v *ProValues) *ProDescriptionsW { p.data = v; return p }

// Request loads the values on a background goroutine; the list shows a
// loading mask meanwhile and an error banner when it fails.
func (p *ProDescriptionsW) Request(fn func() (map[string]any, error)) *ProDescriptionsW {
	p.request = fn
	return p
}

// Editable turns on inline editing: every item without ReadOnly gets an
// edit icon. onSave (may be nil) receives the new value; call done from
// any goroutine. A nil error commits the value, an error stays shown
// under the editor.
func (p *ProDescriptionsW) Editable(onSave func(key string, v any, all ProValues, done func(error))) *ProDescriptionsW {
	p.editable, p.onSave = true, onSave
	return p
}

// Reload runs the request again (valid after Show).
func (p *ProDescriptionsW) Reload() {
	if p.st != nil {
		p.st.requested = false
		p.c.invalid = true
	}
}

// Loading reports a pending request.
func (p *ProDescriptionsW) Loading() bool { return p.st != nil && p.st.loading }

// Err returns the last request error.
func (p *ProDescriptionsW) Err() error {
	if p.st == nil {
		return nil
	}
	return p.st.err
}

// Values returns the current values.
func (p *ProDescriptionsW) Values() ProValues {
	if p.st == nil {
		return nil
	}
	return p.valsRef()
}

// Editing reports whether the item key is being edited.
func (p *ProDescriptionsW) Editing(key string) bool {
	if p.st == nil {
		return false
	}
	_, ok := p.st.editing[key]
	return ok
}

// Saved returns the key of the item committed this frame ("" if none).
func (p *ProDescriptionsW) Saved() string { return p.saved }

// ID returns the widget ID (valid after Show).
func (p *ProDescriptionsW) ID() ID { return p.id }

func (p *ProDescriptionsW) valsRef() ProValues {
	if p.data != nil {
		if *p.data == nil {
			*p.data = ProValues{}
		}
		return *p.data
	}
	return p.st.vals
}

// Show builds the list.
func (p *ProDescriptionsW) Show() *ProDescriptionsW {
	c := p.c
	root := c.box("pro-descriptions", layout.Column, nil)
	p.id = root.n.id
	st := State[pro3DescState](c, p.id, func() pro3DescState {
		return pro3DescState{vals: ProValues{}, editing: map[string]ProValues{}, errs: map[string]string{}, saving: map[string]bool{}}
	})
	p.st = st
	vals := p.valsRef()
	p.poll(vals)
	if !st.requested && p.request != nil {
		st.requested = true
		st.loading = true
		st.seq++
		seq, fn, ch, inv := st.seq, p.request, st.reqRes.chInit(), c.Invalidator()
		go func() {
			m, err := fn()
			ch <- pro3DescReq{seq: seq, vals: m, err: err}
			inv()
		}()
	}
	root.Body(func() {
		if st.err != nil {
			Row(c, func() { Alert(c, "error", st.err.Error()).Show() }, layout.Margin(geom.Insets{Bottom: 16}))
		}
		Loading(c, st.loading).Body(func() { p.list(vals) })
	})
	return p
}

// chInit returns the channel, creating it.
func (a *pro3Async[T]) chInit() chan T { a.init(); return a.ch }

func (p *ProDescriptionsW) poll(vals ProValues) {
	st := p.st
	for {
		r, ok := st.reqRes.poll()
		if !ok {
			break
		}
		if r.seq != st.seq {
			continue
		}
		st.loading, st.err = false, r.err
		if r.err == nil {
			for k, v := range r.vals {
				vals[k] = v
			}
		}
		p.c.invalid = true
	}
	for {
		r, ok := st.checkRes.poll()
		if !ok {
			break
		}
		if r.gen != st.checkGen[r.key] || !st.checking[r.key] {
			continue
		}
		delete(st.checking, r.key)
		p.c.invalid = true
		if r.msg != "" {
			st.errs[r.key] = r.msg
			continue
		}
		if draft, editing := st.editing[r.key]; editing {
			for i := range p.items {
				if p.items[i].Key == r.key {
					fd := p.field(&p.items[i])
					fd.Key = r.key
					p.commit(&fd, draft, vals)
					break
				}
			}
		}
	}
	for {
		r, ok := st.saveRes.poll()
		if !ok {
			break
		}
		delete(st.saving, r.key)
		if r.err != nil {
			st.errs[r.key] = r.err.Error()
		} else {
			vals[r.key] = r.v
			delete(st.editing, r.key)
			delete(st.errs, r.key)
			p.saved = r.key
		}
		p.c.invalid = true
	}
}

// field returns the editor definition of an item.
func (p *ProDescriptionsW) field(it *ProDescItem) ProField {
	var fd ProField
	if it.Field != nil {
		fd = *it.Field
	}
	if fd.Key == "" {
		fd.Key = it.Key
	}
	if fd.Label == "" {
		fd.Label = it.Label
	}
	if fd.ValueType == "" {
		fd.ValueType = it.ValueType
		switch it.ValueType {
		case ValueStatus, ValueTag:
			fd.ValueType = ValueSelect
		case ValueLink, "":
			fd.ValueType = ValueText
		}
	}
	if fd.ValueEnum == nil {
		fd.ValueEnum = it.ValueEnum
	}
	return fd
}

func (p *ProDescriptionsW) list(vals ProValues) {
	c := p.c
	items := make([]DescItem, len(p.items))
	for i := range p.items {
		it := &p.items[i]
		items[i] = DescItem{Label: it.Label, Span: it.Span, Render: func() {
			Key(c, it.Key, func() { p.cell(it, vals) })
		}}
	}
	d := Descriptions(c, items).Column(p.cols)
	if p.title != "" {
		d.Title(p.title)
	}
	if p.extra != nil {
		d.Extra(p.extra)
	}
	if p.bordered {
		d.Bordered()
	}
	if p.sizeSet {
		d.Size(p.size)
	}
	if p.vertical {
		d.Vertical()
	}
}

// cell is the value of one item: display + edit icon, or the editor.
func (p *ProDescriptionsW) cell(it *ProDescItem, vals ProValues) {
	c, st, th := p.c, p.st, p.c.Theme
	loc := c.Locale()
	draft, editing := st.editing[it.Key]
	if !editing {
		Row(c, func() {
			Row(c, func() {
				if it.Render != nil {
					it.Render(c, vals[it.Key], vals)
				} else {
					pro3ValueView(c, it.ValueType, it.ValueEnum, vals[it.Key])
				}
			}, layout.MinW(0), layout.Shrink(1), layout.Middle())
			if p.editable && !it.ReadOnly {
				if pro3IconBtn(c, locFmt(loc.ProDescriptions.EditFmt, it.Label), icon.Edit, 22) {
					v := vals[it.Key]
					if s, ok := v.([]string); ok {
						v = append([]string(nil), s...)
					}
					d := ProValues{}
					if v != nil {
						d[it.Key] = v
					}
					st.editing[it.Key] = d
					delete(st.errs, it.Key)
					c.invalid = true
				}
			}
		}, layout.Gap(4), layout.Middle(), layout.MinW(0))
		return
	}
	fd := p.field(it)
	fd.Key = it.Key
	saving := st.saving[it.Key] || st.checking[it.Key]
	Column(c, func() {
		Row(c, func() {
			b := Row(c, func() {
				Disabled(c, saving, func() { proFieldControl(c, &fd, draft) })
			}, layout.Grow(1), layout.MinW(120), layout.Middle())
			b.Semantics(a11y.RoleGroup, fd.Label)
			if saving {
				Icon(c, icon.Loading).Spin().Size(14).Color(th.Primary.Base)
			} else if pro3IconBtn(c, loc.Save, icon.Check, 24) {
				p.save(&fd, draft, vals)
			}
			if pro3IconBtn(c, loc.Cancel, icon.Close, 24) && (!saving || st.checking[it.Key]) {
				delete(st.editing, it.Key)
				delete(st.errs, it.Key)
				delete(st.checking, it.Key) // a late answer is dropped
				c.invalid = true
			}
		}, layout.Gap(4), layout.Middle())
		if msg := st.errs[it.Key]; msg != "" {
			Text(c, msg).Color(th.Error.Base).Size(th.FontSizeSM).Layout(layout.Margin(geom.Insets{Top: 2}))
		} else if st.checking[it.Key] {
			Text(c, loc.Form.Validating).Color(th.TextTertiary).Size(th.FontSizeSM).Layout(layout.Margin(geom.Insets{Top: 2}))
		}
	}, layout.Grow(1), layout.MinW(0))
}

// save validates the draft of fd and commits it (through onSave).
func (p *ProDescriptionsW) save(fd *ProField, draft, vals ProValues) {
	c, st := p.c, p.st
	key := fd.Key
	all := vals.Clone()
	for k, v := range draft {
		all[k] = v
	}
	rules := proFieldRules(c, fd, all)
	if msg := pro3Check(rules, proValueText(draft[key])); msg != "" {
		st.errs[key] = msg
		c.invalid = true
		return
	}
	if fd.AsyncValidate != nil {
		// The server check runs after the synchronous rules; the item
		// shows the spinner meanwhile and saves once it passes.
		if st.checkGen == nil {
			st.checkGen, st.checking = map[string]int{}, map[string]bool{}
		}
		st.checkGen[key]++
		st.checking[key] = true
		delete(st.errs, key)
		gen, ch, inv := st.checkGen[key], st.checkRes.chInit(), c.Invalidator()
		fd.AsyncValidate(draft[key], func(msg string) {
			ch <- pro3DescCheck{key: key, gen: gen, msg: msg}
			inv()
		})
		c.invalid = true
		p.poll(vals) // a synchronous validator has already answered
		return
	}
	p.commit(fd, draft, vals)
}

// commit writes a validated draft (through onSave).
func (p *ProDescriptionsW) commit(fd *ProField, draft, vals ProValues) {
	c, st := p.c, p.st
	key := fd.Key
	all := vals.Clone()
	for k, v := range draft {
		all[k] = v
	}
	v := draft[key]
	if p.onSave == nil {
		vals[key] = v
		delete(st.editing, key)
		delete(st.errs, key)
		p.saved = key
		c.invalid = true
		return
	}
	st.saving[key] = true
	ch, inv := st.saveRes.chInit(), c.Invalidator()
	p.onSave(key, v, all, func(err error) {
		ch <- pro3DescSave{key: key, v: v, err: err}
		inv()
	})
	p.poll(vals) // a synchronous save has already delivered
	c.invalid = true
}
