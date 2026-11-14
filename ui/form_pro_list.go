package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"strconv"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// FormListW is a dynamic list of form rows bound to a slice (Ant Design
// Form.List / ProFormList): rows can be added, removed and moved; every
// row's items validate on their own (give them row.Name(field)).
//
//	ui.FormList(f, "联系人", &contacts).Min(1).Max(5).
//		Creator(func() Contact { return Contact{} }).
//		Body(func(row *ui.FormListRow[Contact]) {
//			f.Item("姓名").Name(row.Name("name")).Rules(&row.Value.Name, ui.Required("请输入姓名")).
//				Body(func() { ui.Input(c, &row.Value.Name) })
//		})
type FormListW[T any] struct {
	_ [0]func()

	f         *FormW
	label     string
	items     *[]T
	min, max  int
	creator   func() T
	addText   string
	noActions bool
	rule      func(n int) string
	changed   bool
}

// FormListRow is one row passed to FormListW.Body.
type FormListRow[T any] struct {
	Index int
	Value *T     // the row's element (valid during the callback)
	Key   string // stable key of the row
	list  *FormListW[T]
	op    *formListOp
}

type formListOp struct {
	kind  int8 // 1 remove, 2 up, 3 down
	index int
}

type formListState struct {
	keys []string
	next int
}

// FormList starts a dynamic list inside form f; finish with Body.
func FormList[T any](f *FormW, label string, items *[]T) *FormListW[T] {
	// ConfigProvider.Defaults("ProFormList", ...) keys: "min", "max"
	// (number), "addText" (string).
	l := &FormListW[T]{f: f, label: label, items: items, max: -1}
	if c := f.c; c != nil {
		if n, ok := proPropInt(c, "ProFormList", "min"); ok {
			l.min = n
		}
		if n, ok := proPropInt(c, "ProFormList", "max"); ok {
			l.max = n
		}
		if s, ok := p2bStr(c, "ProFormList", "addText"); ok {
			l.addText = s
		}
	}
	return l
}

// Min keeps at least n rows (remove is disabled at n).
func (l *FormListW[T]) Min(n int) *FormListW[T] { l.min = n; return l }

// Max allows at most n rows (add is disabled at n).
func (l *FormListW[T]) Max(n int) *FormListW[T] { l.max = n; return l }

// Creator returns the element added by the add button (default: zero value).
func (l *FormListW[T]) Creator(fn func() T) *FormListW[T] { l.creator = fn; return l }

// AddText sets the add button's text.
func (l *FormListW[T]) AddText(s string) *FormListW[T] { l.addText = s; return l }

// NoActions hides the per-row move / remove buttons (use row.Remove etc.).
func (l *FormListW[T]) NoActions() *FormListW[T] { l.noActions = true; return l }

// Rule validates the list itself (e.g. at least one row): fn gets the row
// count and returns an error message or "".
func (l *FormListW[T]) Rule(fn func(n int) string) *FormListW[T] { l.rule = fn; return l }

// Changed reports a row added, removed or moved in the last Body.
func (l *FormListW[T]) Changed() bool { return l.changed }

// Name returns a field name unique to this row ("label.key.field").
func (r *FormListRow[T]) Name(field string) string {
	return r.list.label + "." + r.Key + "." + field
}

// Remove deletes the row (after the rows are built).
func (r *FormListRow[T]) Remove() { *r.op = formListOp{1, r.Index} }

// MoveUp swaps the row with the one above.
func (r *FormListRow[T]) MoveUp() { *r.op = formListOp{2, r.Index} }

// MoveDown swaps the row with the one below.
func (r *FormListRow[T]) MoveDown() { *r.op = formListOp{3, r.Index} }

// Body builds the rows with fn (items inside a row are laid out inline),
// the row actions and the add button.
func (l *FormListW[T]) Body(fn func(row *FormListRow[T])) *FormListW[T] {
	f := l.f
	c := f.c
	th := c.Theme
	loc := c.Locale()
	st := State[formListState](c, hashID(f.n.id, "form-list", l.label), nil)
	items := *l.items
	// Keep one key per element (the app may have changed the slice).
	for len(st.keys) < len(items) {
		st.next++
		st.keys = append(st.keys, strconv.Itoa(st.next))
	}
	st.keys = st.keys[:len(items)]
	for len(*l.items) < l.min {
		l.add(st)
	}
	items = *l.items
	op := &formListOp{}
	add := false
	it := f.Item(l.label)
	if l.rule != nil {
		n := len(items)
		rule := l.rule
		it.Name(l.label+".__list").RulesFunc(func() string { return strconv.Itoa(len(*l.items)) }, Check(func(string) string { return rule(n) }))
	}
	it.Body(func() {
		Column(c, func() {
			for i := range items {
				i := i
				row := &FormListRow[T]{Index: i, Value: &items[i], Key: st.keys[i], list: l, op: op}
				Key(c, row.Key, func() {
					Row(c, func() {
						prev := f.layout
						f.layout = FormInline
						Row(c, func() { fn(row) }, layout.Grow(1), layout.MinW(0), layout.WrapItems(), layout.Gap(16))
						f.layout = prev
						if l.noActions {
							return
						}
						Row(c, func() {
							act := func(ic *icon.Icon, label string, enabled bool, kind int8) {
								b := IconButton(c, ic, label).Size(28, 16)
								if !enabled {
									b.Color(th.TextDisabled).NoHoverBg()
									b.n.setState(a11y.Disabled) // looks disabled; stays focusable
								}
								if enabled && b.Clicked() {
									*op = formListOp{kind, i}
								}
							}
							act(icon.ArrowUp, loc.Form.MoveUp, i > 0, 2)
							act(icon.ArrowDown, loc.Form.MoveDown, i < len(items)-1, 3)
							act(icon.Delete, loc.Delete, len(items) > l.min, 1)
						}, layout.Gap(2), layout.H(th.ControlHeight), layout.Middle())
					}, layout.Gap(8), layout.AlignItems(layout.AlignStart))
				})
			}
			txt := l.addText
			if txt == "" {
				txt = loc.Form.AddRow
			}
			full := l.max >= 0 && len(items) >= l.max
			if Button(c, txt).Dashed().Icon(icon.Plus).Block().Disabled(full).Clicked() {
				add = true
			}
		}, layout.Grow(1), layout.MinW(0))
	})
	switch {
	case add && (l.max < 0 || len(*l.items) < l.max):
		l.add(st)
		l.changed = true
	case op.kind == 1 && len(*l.items) > l.min:
		i := op.index
		s := *l.items
		*l.items = append(s[:i:i], s[i+1:]...)
		st.keys = append(st.keys[:i:i], st.keys[i+1:]...)
		l.changed = true
	case op.kind == 2 && op.index > 0:
		l.swap(st, op.index, op.index-1)
	case op.kind == 3 && op.index < len(*l.items)-1:
		l.swap(st, op.index, op.index+1)
	}
	if l.changed {
		c.invalid = true
	}
	return l
}

func (l *FormListW[T]) add(st *formListState) {
	var v T
	if l.creator != nil {
		v = l.creator()
	}
	*l.items = append(*l.items, v)
	st.next++
	st.keys = append(st.keys, strconv.Itoa(st.next))
}

func (l *FormListW[T]) swap(st *formListState, i, j int) {
	s := *l.items
	s[i], s[j] = s[j], s[i]
	st.keys[i], st.keys[j] = st.keys[j], st.keys[i]
	l.changed = true
}

var _ = geom.Pt

// proFormListItem builds a formList field (ProField.List): a Form.List
// whose rows are ProValues, each row's items being the List fields bound
// to that row. Fields without a Width get 180px (row items are inline).
func proFormListItem(c *Context, f *FormW, fd *ProField, vals ProValues) {
	cfg := fd.List
	fields := make([]ProField, len(cfg.Fields))
	copy(fields, cfg.Fields)
	for i := range fields {
		if fields[i].Width == 0 {
			fields[i].Width = 180
		}
	}
	rows, _ := vals[fd.Key].([]ProValues)
	// The list's label prefixes the row item names ("label.row.field").
	l := FormList(f, fd.Label, &rows).Creator(func() ProValues {
		v := ProValues{}
		proInitial(fields, v)
		return v
	})
	if cfg.Min > 0 {
		l.Min(cfg.Min)
	}
	if cfg.Max > 0 {
		l.Max(cfg.Max)
	}
	if cfg.AddText != "" {
		l.AddText(cfg.AddText)
	}
	if fd.Required {
		msg := locFmt(c.Locale().Form.RequiredInputFmt, fd.Label)
		l.Rule(func(n int) string {
			if n == 0 {
				return msg
			}
			return ""
		})
	}
	l.Body(func(row *FormListRow[ProValues]) {
		if *row.Value == nil {
			*row.Value = ProValues{}
		}
		rv := *row.Value
		for i := range fields {
			sub := &fields[i]
			Key(c, sub.Key, func() { proFieldItemAs(c, f, sub, rv, row.Name(sub.Key), false) })
		}
	})
	if len(rows) == 0 {
		delete(vals, fd.Key)
	} else {
		vals[fd.Key] = rows
	}
}
