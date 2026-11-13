package ui

import (
	"math"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Editors and read-only views of the value types added to ProField /
// ProColumn in the second Pro round: cascader, treeSelect, upload, time,
// timeRange, color and digitRange. proFieldControl and pro3ValueText /
// pro3ValueView dispatch here.

// p2eCascader converts enums (with Children) to cascader options.
func p2eCascader(es []ProEnum) []CascaderOption {
	out := make([]CascaderOption, len(es))
	for i, e := range es {
		out[i] = CascaderOption{Label: e.Label, Value: e.Value, Children: p2eCascader(e.Children)}
	}
	return out
}

// p2eTree converts enums (with Children) to tree-select nodes.
func p2eTree(es []ProEnum) []TreeSelectNode {
	out := make([]TreeSelectNode, len(es))
	for i, e := range es {
		out[i] = TreeSelectNode{Label: e.Label, Value: e.Value, Children: p2eTree(e.Children)}
	}
	return out
}

// p2ePathLabels maps a cascader value path to its labels.
func p2ePathLabels(es []ProEnum, path []string) []string {
	out := make([]string, 0, len(path))
	level := es
	for _, v := range path {
		label := v
		var next []ProEnum
		for _, e := range level {
			if e.Value == v {
				label, next = e.Label, e.Children
				break
			}
		}
		out = append(out, label)
		level = next
	}
	return out
}

// p2eTreeLabel finds the label of v anywhere in the tree.
func p2eTreeLabel(es []ProEnum, v string) (string, bool) {
	for _, e := range es {
		if e.Value == v {
			return e.Label, true
		}
		if s, ok := p2eTreeLabel(e.Children, v); ok {
			return s, true
		}
	}
	return "", false
}

// p2eUploadState delivers files picked by ProField.Pick.
type p2eUploadState struct {
	res pro3Async[[]UploadFile]
}

// p2eFieldControl builds the editor of the new value types.
func p2eFieldControl(c *Context, fd *ProField, vals ProValues, ph string, grow []layout.Option) {
	loc := c.Locale()
	k := fd.Key
	opts := proOptions(fd, vals)
	switch fd.ValueType {
	case ValueCascader:
		s := append([]string(nil), vals.Strings(k)...)
		w := Cascader(c, &s, p2eCascader(opts)).Placeholder(ph).AllowClear().Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			if len(s) == 0 {
				delete(vals, k)
			} else {
				vals[k] = s
			}
		}
	case ValueTreeSelect:
		s := vals.String(k)
		w := TreeSelect(c, &s, p2eTree(opts)).Placeholder(ph).AllowClear().Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			if s == "" {
				delete(vals, k)
			} else {
				vals[k] = s
			}
		}
	case ValueTime:
		t := vals.Time(k)
		w := TimePicker(c, &t).Placeholder(ph).Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			if t.IsZero() {
				delete(vals, k)
			} else {
				vals[k] = t
			}
		}
	case ValueTimeRange:
		r := vals.Range(k)
		changed := false
		Row(c, func() {
			a := TimePicker(c, &r[0]).Placeholder(loc.TimePicker.StartTime).Layout(layout.Grow(1), layout.MinW(0))
			a.A11yLabel(locFmt(loc.ProField.StartFmt, fd.Label))
			if a.Changed() {
				changed = true
			}
			Text(c, loc.RangeSeparator).Color(c.Theme.TextTertiary).NoWrap()
			b := TimePicker(c, &r[1]).Placeholder(loc.TimePicker.EndTime).Layout(layout.Grow(1), layout.MinW(0))
			b.A11yLabel(locFmt(loc.ProField.EndFmt, fd.Label))
			if b.Changed() {
				changed = true
			}
		}, append([]layout.Option{layout.Gap(8), layout.Middle()}, grow...)...)
		if changed {
			if r[0].IsZero() && r[1].IsZero() {
				delete(vals, k)
			} else {
				vals[k] = r
			}
		}
	case ValueDigitRange:
		r := vals.DigitRange(k)
		changed := false
		end := func(i int, name string) {
			v := r[i]
			empty := math.IsNaN(v)
			if empty {
				v = 0
			}
			w := InputNumber(c, &v).Empty(&empty).Placeholder(name)
			w.A11yLabel(locFmt(loc.ProField.RangeEndFmt, fd.Label, name))
			if fd.Precision > 0 {
				w.Precision(fd.Precision)
			}
			if fd.Min != 0 || fd.Max != 0 {
				w.Min(fd.Min).Max(fd.Max)
			}
			w.box.ln.Grow, w.box.ln.MinW, w.box.ln.Width = 1, 0, 0
			if w.Changed() {
				changed = true
				if empty {
					r[i] = math.NaN()
				} else {
					r[i] = v
				}
			}
		}
		Row(c, func() {
			end(0, loc.ProField.Min)
			Text(c, loc.RangeSeparator).Color(c.Theme.TextTertiary).NoWrap()
			end(1, loc.ProField.Max)
		}, append([]layout.Option{layout.Gap(8), layout.Middle()}, grow...)...)
		if changed {
			if math.IsNaN(r[0]) && math.IsNaN(r[1]) {
				delete(vals, k)
			} else {
				vals[k] = r
			}
		}
	case ValueColor:
		col, err := theme.ParseHex(vals.String(k))
		if err != nil {
			col = theme.Color{}
		}
		Row(c, func() {
			w := ColorPicker(c, &col).ShowText().AllowClear()
			if fd.Label != "" {
				w.A11yLabel(fd.Label)
			}
			if w.Changed() || w.Cleared() {
				if col == (theme.Color{}) {
					delete(vals, k)
				} else {
					vals[k] = strings.ToUpper(theme.HexString(col))
				}
			}
		}, layout.MinH(c.Theme.ControlHeight), layout.Middle())
	case ValueUpload:
		files := append([]UploadFile(nil), vals.Files(k)...)
		set := func() {
			if len(files) == 0 {
				delete(vals, k)
			} else {
				vals[k] = files
			}
		}
		before := len(files)
		Column(c, func() {
			// The dragger accepts dropped files and highlights while files
			// are dragged over it (Upload wraps it in a DropZone).
			up := Upload(c, &files)
			if fd.Dragger {
				up.Dragger()
			}
			st := State[p2eUploadState](c, hashID(up.n.id, "p2e-upload"), nil) // Upload keeps its own state on up.n.id
			for {
				got, ok := st.res.poll()
				if !ok {
					break
				}
				files = append(files, got...)
				c.invalid = true
			}
			if fd.Pick != nil {
				pick, ch, inv := fd.Pick, st.res.chInit(), c.Invalidator()
				up.OnPick(func() { pick(func(fs []UploadFile) { ch <- fs; inv() }) })
			}
			if up.Changed() || len(files) != before {
				set()
			}
		}, grow...)
	}
}

// p2eValueText formats the new value types (ok false: not one of them).
func p2eValueText(c *Context, valueType string, enum []ProEnum, v any) (string, bool) {
	loc := c.Locale()
	switch valueType {
	case ValueCascader:
		path, _ := v.([]string)
		if len(path) == 0 {
			return "-", true
		}
		return strings.Join(p2ePathLabels(enum, path), " / "), true
	case ValueTreeSelect:
		s, _ := v.(string)
		if s == "" {
			return "-", true
		}
		if l, ok := p2eTreeLabel(enum, s); ok {
			return l, true
		}
		return s, true
	case ValueUpload:
		if s := proValueText(v); s != "" {
			return strings.ReplaceAll(s, ",", ", "), true
		}
		return "-", true
	case ValueTime:
		t, _ := v.(time.Time)
		if t.IsZero() {
			return "-", true
		}
		return t.Format("15:04:05"), true
	case ValueTimeRange:
		r, _ := v.([2]time.Time)
		if r[0].IsZero() && r[1].IsZero() {
			return "-", true
		}
		f := func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("15:04:05")
		}
		return f(r[0]) + " " + loc.RangeSeparator + " " + f(r[1]), true
	case ValueDigitRange:
		r, ok := v.([2]float64)
		if !ok || (math.IsNaN(r[0]) && math.IsNaN(r[1])) {
			return "-", true
		}
		f := func(x float64) string {
			if math.IsNaN(x) {
				return "-"
			}
			return ProFormat(ValueDigit, nil, x)
		}
		return f(r[0]) + " " + loc.RangeSeparator + " " + f(r[1]), true
	case ValueColor:
		s, _ := v.(string)
		if s == "" {
			return "-", true
		}
		return strings.ToUpper(s), true
	}
	return "", false
}

// p2eColorView is the read-only color: a swatch and its hex code.
func p2eColorView(c *Context, v any) {
	th := c.Theme
	s, _ := v.(string)
	col, err := theme.ParseHex(s)
	if s == "" || err != nil {
		Text(c, "-")
		return
	}
	Row(c, func() {
		n := c.add("p2e-swatch", "")
		n.ln.Width, n.ln.Height = 16, 16
		n.aHidden = true
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(th.RadiusXS)
			if col.A < 255 {
				l.Checker(r, rad, 4, theme.Color{}, theme.Color{})
			}
			l.Fill(r, rad, col)
			l.Stroke(r, rad, 1, th.BorderSecondary)
		}
		Text(c, strings.ToUpper(s))
	}, layout.Gap(6), layout.Middle()).Semantics(a11y.RoleImage, strings.ToUpper(s))
}
