package ui

import (
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
)

// ProFieldSet combines several controls into one form value (Ant Design
// Pro ProFormFieldSet), such as a numeric range or a phone area code plus
// number. The value of Key is a []any with one entry per part (nil for an
// empty part); read it with ProValues.Parts.
type ProFieldSet struct {
	Key, Label string
	Parts      []ProField // ValueType, ValueEnum, Placeholder, Width, Min / Max of each part
	Separator  string     // text between the parts (e.g. "~"); empty: a gap
	Required   bool       // every part must be filled
	// Validate checks the combined value (e.g. min <= max).
	Validate func(parts []any, all ProValues) string
	Help     string
	Span     int
	Initial  []any
}

// Parts returns the value of a field set (nil when missing).
func (v ProValues) Parts(k string) []any { p, _ := v[k].([]any); return p }

// ProFormFieldSet turns a field set into a ProField for ProForm,
// QueryFilter, ModalForm, StepsForm …:
//
//	fields = append(fields, ui.ProFormFieldSet(c, ui.ProFieldSet{
//		Key: "budget", Label: "Budget", Separator: "~", Required: true,
//		Parts: []ui.ProField{{ValueType: ui.ValueDigit}, {ValueType: ui.ValueDigit}},
//	}))
func ProFormFieldSet(c *Context, fs ProFieldSet) ProField {
	fd := ProField{Key: fs.Key, Label: fs.Label, ValueType: "fieldSet", Help: fs.Help, Span: fs.Span}
	if fs.Initial != nil {
		fd.Initial = append([]any(nil), fs.Initial...)
	}
	set := fs
	// ConfigProvider.Defaults("ProFormFieldSet", ...) keys: "separator"
	// (string between the parts when the set gives none).
	if set.Separator == "" {
		set.Separator = Prop(c, "ProFormFieldSet", "separator", "")
	}
	msg := pro3FieldSetMsg(c, &set)
	fd.Validate = func(v any, all ProValues) string { return pro3FieldSetCheck(&set, msg, all) }
	if fs.Required {
		// Marks the item required (asterisk); the check itself is done by
		// Validate, which sees the parts.
		fd.Rules = []Rule{{required: true, check: func(string) string { return "" }}}
	}
	fd.Render = func(c *Context, vals ProValues) { pro3FieldSetControl(c, &set, vals) }
	return fd
}

// FieldSet builds a field set as a labelled item of form f (outside a
// ProForm); vals holds the values.
func FieldSet(c *Context, f *FormW, fs ProFieldSet, vals ProValues) {
	fd := ProFormFieldSet(c, fs)
	proFieldItem(c, f, &fd, vals)
}

func pro3FieldSetMsg(c *Context, fs *ProFieldSet) string {
	if fs.Label == "" {
		return c.Locale().Required
	}
	return locFmt(c.Locale().Form.RequiredInputFmt, fs.Label)
}

// pro3FieldSetCheck validates the combined value.
func pro3FieldSetCheck(fs *ProFieldSet, msg string, all ProValues) string {
	parts := all.Parts(fs.Key)
	if fs.Required {
		for i := range fs.Parts {
			if i >= len(parts) || pro3Empty(parts[i]) {
				return msg
			}
		}
	}
	if fs.Validate != nil {
		return fs.Validate(parts, all)
	}
	return ""
}

func pro3PartKey(fd *ProField, i int) string {
	if fd.Key != "" {
		return fd.Key
	}
	return "#" + strconv.Itoa(i)
}

// pro3FieldSetControl lays the parts' editors in a row bound to
// vals[fs.Key].
func pro3FieldSetControl(c *Context, fs *ProFieldSet, vals ProValues) {
	th := c.Theme
	b := Row(c, func() {
		parts := vals.Parts(fs.Key)
		for i := range fs.Parts {
			part := fs.Parts[i]
			k := pro3PartKey(&part, i)
			part.Key = k
			if part.Label == "" {
				part.Label = fs.Label
			}
			sub := ProValues{}
			if i < len(parts) && parts[i] != nil {
				sub[k] = parts[i]
			}
			if i > 0 && fs.Separator != "" {
				Text(c, fs.Separator).Color(th.TextTertiary).NoWrap()
			}
			before := proValueText(sub[k])
			Key(c, k, func() {
				opts := []layout.Option{layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Middle()}
				if part.Width > 0 {
					opts = []layout.Option{layout.Middle()}
				}
				Row(c, func() { proFieldControl(c, &part, sub) }, opts...)
			})
			if proValueText(sub[k]) != before || (sub[k] == nil) != (i >= len(parts) || parts[i] == nil) {
				// Copy on write: the form's reset target may share the slice.
				np := make([]any, len(fs.Parts))
				copy(np, parts)
				np[i] = sub[k]
				if pro3Empty(np[i]) {
					np[i] = nil
				}
				vals[fs.Key] = np
				parts = np
			}
		}
	}, layout.Gap(8), layout.Middle(), layout.Grow(1), layout.MinW(0))
	b.Semantics(a11y.RoleGroup, fs.Label)
}
