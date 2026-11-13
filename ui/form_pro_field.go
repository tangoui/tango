package ui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/layout"
)

// Value types used by ProForm fields in addition to the ProTable ones
// (ValueText, ValueMoney, ValueDigit, ValuePercent, ValueDate,
// ValueDateTime, ValueStatus, ValueTag ...): one set describes both a
// column's display and a field's editor.
const (
	ValuePassword    = "password"
	ValueTextArea    = "textarea"
	ValueSelect      = "select"
	ValueMultiSelect = "multiSelect"
	ValueRadio       = "radio"
	ValueRadioButton = "radioButton"
	ValueCheckbox    = "checkbox"
	ValueSwitch      = "switch"
	ValueRate        = "rate"
	ValueSlider      = "slider"
	ValueSegmented   = "segmented"
	ValueDateRange   = "dateRange"
	ValueCascader    = "cascader"   // []string path; options: ValueEnum with Children
	ValueTreeSelect  = "treeSelect" // string; options: ValueEnum with Children
	ValueUpload      = "upload"     // []UploadFile; ProField.Pick adds files
	ValueTime        = "time"       // time.Time (time of day)
	ValueTimeRange   = "timeRange"  // [2]time.Time
	ValueColor       = "color"      // string "#RRGGBB" / "#RRGGBBAA"
	ValueDigitRange  = "digitRange" // [2]float64; an empty end is NaN
)

// ProValues holds form values by field key. The Go type depends on the
// value type: string (text, select, radio ...), float64 (digit, money,
// percent, rate, slider, progress), time.Time (date, dateTime),
// [2]time.Time (dateRange), []string (multiSelect, checkbox), bool
// (switch). A missing key is an empty value.
type ProValues map[string]any

// String returns the value of k as text ("" when missing).
func (v ProValues) String(k string) string {
	switch x := v[k].(type) {
	case nil:
		return ""
	case string:
		return x
	}
	return proValueText(v[k])
}

// Float returns a number value (0 when missing or not a number).
func (v ProValues) Float(k string) float64 { f, _ := proNumber(v[k]); return f }

// Bool returns a switch value.
func (v ProValues) Bool(k string) bool { b, _ := v[k].(bool); return b }

// Time returns a date value (zero when missing).
func (v ProValues) Time(k string) time.Time { t, _ := v[k].(time.Time); return t }

// Range returns a date range value.
func (v ProValues) Range(k string) [2]time.Time { r, _ := v[k].([2]time.Time); return r }

// Strings returns a multi-value (multiSelect, checkbox, cascader path).
func (v ProValues) Strings(k string) []string { s, _ := v[k].([]string); return s }

// Files returns an upload value.
func (v ProValues) Files(k string) []UploadFile { f, _ := v[k].([]UploadFile); return f }

// DigitRange returns a number range; a missing value or end is NaN.
func (v ProValues) DigitRange(k string) [2]float64 {
	if r, ok := v[k].([2]float64); ok {
		return r
	}
	return [2]float64{math.NaN(), math.NaN()}
}

// Clone copies the map (slices are copied too).
func (v ProValues) Clone() ProValues {
	out := make(ProValues, len(v))
	for k, x := range v {
		switch s := x.(type) {
		case []string:
			x = append([]string(nil), s...)
		case []UploadFile:
			x = append([]UploadFile(nil), s...)
		}
		out[k] = x
	}
	return out
}

// proValueText formats a value for validation and signatures: empty
// values give "".
func proValueText(x any) string {
	switch t := x.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return ""
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02 15:04:05")
	case [2]time.Time:
		if t[0].IsZero() && t[1].IsZero() {
			return ""
		}
		return t[0].Format("2006-01-02") + "~" + t[1].Format("2006-01-02")
	case []string:
		return strings.Join(t, ",")
	case []UploadFile:
		names := make([]string, len(t))
		for i, f := range t {
			names[i] = f.Name
		}
		return strings.Join(names, ",")
	case [2]float64:
		a, b := proRangeEnd(t[0]), proRangeEnd(t[1])
		if a == "" && b == "" {
			return ""
		}
		return a + "~" + b
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return fmt.Sprint(x)
}

// ProField describes a form field; ProForm, QueryFilter, StepsForm,
// ModalForm and DrawerForm render fields from it.
type ProField struct {
	Key, Label  string
	ValueType   string    // see the Value* constants (default text)
	ValueEnum   []ProEnum // options of select / radio / checkbox / status / tag
	Placeholder string
	Help        string // hint under the control
	Span        int    // grid columns out of 24 (default: 24 / form columns)
	// Group puts the field in a titled ProFormGroup with the consecutive
	// fields of the same Group (ProForm, ModalForm, StepsForm).
	Group   string
	Width   float32 // fixed control width (default: fill the column)
	Initial any     // value when the form starts or resets

	// Validation: Required adds a required rule with the locale message;
	// Rules check the value's text form; Validate checks the typed value
	// against all values (cross-field); AsyncValidate runs after them.
	Required      bool
	Rules         []Rule
	Validate      func(v any, all ProValues) string
	AsyncValidate func(v any, done func(msg string))

	// Linkage: Dependencies lists keys whose change re-validates this
	// field, refreshes Options and drops a value Options no longer offers.
	Dependencies []string
	Hide         func(all ProValues) bool
	Disable      func(all ProValues) bool
	Options      func(all ProValues) []ProEnum

	// Number fields.
	Min, Max  float64
	Precision int

	// Pick handles the upload field's trigger (open a file dialog); call
	// done from any goroutine with the files to add.
	Pick func(done func(files []UploadFile))
	// Dragger shows the upload field as a drag area (Upload.Dragger):
	// it highlights while files are dragged over it and takes dropped
	// files (DropZone inside Upload).
	Dragger bool

	// Render replaces the built-in control (bind it to vals[Key]).
	Render func(c *Context, vals ProValues)
}

// proRangeEnd formats one end of a digit range ("" when empty).
func proRangeEnd(f float64) string {
	if math.IsNaN(f) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// proEnumOptions converts enums to options.
func proEnumOptions(es []ProEnum) []Option {
	out := make([]Option, len(es))
	for i, e := range es {
		out[i] = Option{Label: e.Label, Value: e.Value}
	}
	return out
}

// proOptions returns the field's current options.
func proOptions(fd *ProField, vals ProValues) []ProEnum {
	if fd.Options != nil {
		return fd.Options(vals)
	}
	return fd.ValueEnum
}

// proFieldFromColumn derives the search field of a ProTable column.
func proFieldFromColumn[T any](col ProColumn[T]) ProField {
	fd := ProField{Key: col.Key, Label: col.Title, ValueType: ValueText, ValueEnum: col.ValueEnum}
	switch col.ValueType {
	case ValueStatus, ValueTag, ValueSelect, ValueRadio, ValueRadioButton:
		fd.ValueType = ValueSelect
	case ValueDate, ValueDateTime:
		fd.ValueType = ValueDate
	case ValueDateRange, ValueCascader, ValueTreeSelect, ValueTime, ValueTimeRange, ValueDigitRange, ValueColor:
		fd.ValueType = col.ValueType
	}
	return fd
}

// proFieldRules builds the FormItem rules of a field.
func proFieldRules(c *Context, fd *ProField, vals ProValues) []Rule {
	var rules []Rule
	if fd.Required {
		msg := c.Locale().Required
		if fd.Label != "" {
			msg = locFmt(c.Locale().Form.RequiredInputFmt, fd.Label)
			switch fd.ValueType {
			case ValueSelect, ValueMultiSelect, ValueRadio, ValueRadioButton, ValueCheckbox, ValueStatus, ValueTag,
				ValueDate, ValueDateTime, ValueDateRange, ValueSegmented,
				ValueCascader, ValueTreeSelect, ValueUpload, ValueTime, ValueTimeRange, ValueColor:
				msg = locFmt(c.Locale().Form.RequiredSelectFmt, fd.Label)
			}
		}
		rules = append(rules, Required(msg))
	}
	rules = append(rules, fd.Rules...)
	if fd.Validate != nil {
		v := fd.Validate
		rules = append(rules, Check(func(string) string { return v(vals[fd.Key], vals) }))
	}
	if fd.AsyncValidate != nil {
		av := fd.AsyncValidate
		rules = append(rules, AsyncCheck(func(_ string, done func(string)) { av(vals[fd.Key], done) }))
	}
	return rules
}

// proFieldItem builds a labelled field inside form f. It handles hide,
// disable, dependencies and option refresh.
func proFieldItem(c *Context, f *FormW, fd *ProField, vals ProValues) {
	if fd.Hide != nil && fd.Hide(vals) {
		f.Item(fd.Label).Name(fd.Key).Hidden(true).Body(nil)
		return
	}
	it := f.Item(fd.Label).Name(fd.Key).RulesFunc(func() string { return proValueText(vals[fd.Key]) }, proFieldRules(c, fd, vals)...)
	if fd.Help != "" {
		it.Help(fd.Help)
	}
	if len(fd.Dependencies) > 0 {
		it.Dependencies(fd.Dependencies...).OnDepsChange(func() { proDropStale(fd, vals) })
	}
	if fd.Disable != nil {
		it.Disabled(fd.Disable(vals))
	}
	it.Body(func() { proFieldControl(c, fd, vals) })
}

// proDropStale clears a choice value the refreshed options no longer offer.
func proDropStale(fd *ProField, vals ProValues) {
	if fd.Options == nil {
		return
	}
	opts := fd.Options(vals)
	has := func(v string) bool {
		for _, o := range opts {
			if o.Value == v {
				return true
			}
		}
		return false
	}
	switch x := vals[fd.Key].(type) {
	case string:
		if x != "" && !has(x) {
			delete(vals, fd.Key)
		}
	case []string:
		var keep []string
		for _, v := range x {
			if has(v) {
				keep = append(keep, v)
			}
		}
		vals[fd.Key] = keep
	}
}

// proFieldControl builds the editor of a field bound to vals[fd.Key].
func proFieldControl(c *Context, fd *ProField, vals ProValues) {
	if fd.Render != nil {
		fd.Render(c, vals)
		return
	}
	loc := c.Locale()
	k := fd.Key
	ph := fd.Placeholder
	grow := []layout.Option{layout.Grow(1), layout.MinW(0)}
	if fd.Width > 0 {
		grow = []layout.Option{layout.W(fd.Width)}
	}
	sel := func() string {
		if ph != "" {
			return ph
		}
		return loc.SelectPlaceholder
	}
	inp := func() string {
		if ph != "" {
			return ph
		}
		return loc.Placeholder
	}
	switch fd.ValueType {
	case ValuePassword:
		s := vals.String(k)
		if Input(c, &s).Placeholder(inp()).Password().Layout(grow...).Changed() {
			vals[k] = s
		}
	case ValueTextArea:
		s := vals.String(k)
		if TextArea(c, &s).Placeholder(inp()).Rows(3).Layout(grow...).Changed() {
			vals[k] = s
		}
	case ValueMoney, ValueDigit, ValuePercent, ValueProgress:
		v, ok := proNumber(vals[k])
		empty := !ok
		w := InputNumber(c, &v).Placeholder(inp()).Empty(&empty)
		if fd.Min != 0 || fd.Max != 0 {
			w.Min(fd.Min).Max(fd.Max)
		}
		switch {
		case fd.Precision > 0:
			w.Precision(fd.Precision)
		case fd.ValueType == ValueMoney:
			w.Precision(2)
		}
		if fd.Width > 0 {
			w.Width(fd.Width)
		} else {
			w.box.ln.Grow, w.box.ln.MinW, w.box.ln.Width = 1, 0, 0
		}
		if w.Changed() {
			if empty {
				delete(vals, k)
			} else {
				vals[k] = v
			}
		}
	case ValueDate:
		t := vals.Time(k)
		dp := DatePicker(c, &t).Placeholder(sel())
		dp.n.ln.Grow, dp.n.ln.MinW = 1, 0
		if fd.Width > 0 {
			dp.n.ln.Grow, dp.n.ln.Width = 0, fd.Width
		}
		if dp.Changed() {
			vals[k] = t
		}
	case ValueDateTime:
		t := vals.Time(k)
		if DateTimePicker(c, &t).Placeholder(sel()).Layout(grow...).Changed() {
			vals[k] = t
		}
	case ValueDateRange:
		r := vals.Range(k)
		if RangePicker(c, &r).Layout(grow...).Changed() {
			vals[k] = r
		}
	case ValueSelect, ValueStatus, ValueTag:
		s := vals.String(k)
		if Select(c, &s, proEnumOptions(proOptions(fd, vals))).Placeholder(sel()).AllowClear().Layout(grow...).Changed() {
			vals[k] = s
		}
	case ValueMultiSelect:
		s := append([]string(nil), vals.Strings(k)...)
		if MultiSelect(c, &s, proEnumOptions(proOptions(fd, vals))).Placeholder(sel()).AllowClear().Layout(grow...).Changed() {
			vals[k] = s
		}
	case ValueCheckbox:
		s := append([]string(nil), vals.Strings(k)...)
		Row(c, func() {
			if CheckboxGroup(c, &s, proEnumOptions(proOptions(fd, vals))) {
				vals[k] = s
			}
		}, layout.MinH(c.Theme.ControlHeight), layout.Middle())
	case ValueRadio, ValueRadioButton:
		s := vals.String(k)
		Row(c, func() {
			g := RadioGroup(c, &s, proEnumOptions(proOptions(fd, vals)))
			if fd.ValueType == ValueRadioButton {
				g.Buttons()
			}
			if g.Changed() {
				vals[k] = s
			}
		}, layout.MinH(c.Theme.ControlHeight), layout.Middle())
	case ValueSegmented:
		s := vals.String(k)
		if Segmented(c, &s, proEnumOptions(proOptions(fd, vals))).Changed() {
			vals[k] = s
		}
	case ValueSwitch:
		b := vals.Bool(k)
		Row(c, func() {
			if Switch(c, &b).Changed() {
				vals[k] = b
			}
		}, layout.MinH(c.Theme.ControlHeight), layout.Middle())
	case ValueRate:
		v := vals.Float(k)
		Row(c, func() {
			if Rate(c, &v).Changed() {
				vals[k] = v
			}
		}, layout.MinH(c.Theme.ControlHeight), layout.Middle())
	case ValueSlider:
		v := vals.Float(k)
		lo, hi := fd.Min, fd.Max
		if hi <= lo {
			lo, hi = 0, 100
		}
		if Slider(c, &v, lo, hi).Layout(grow...).Changed() {
			vals[k] = v
		}
	case ValueCascader, ValueTreeSelect, ValueUpload, ValueTime, ValueTimeRange, ValueColor, ValueDigitRange:
		p2eFieldControl(c, fd, vals, sel(), grow)
	default:
		s := vals.String(k)
		if Input(c, &s).Placeholder(inp()).AllowClear().Layout(grow...).Changed() {
			vals[k] = s
		}
	}
}

// proInitial fills missing values from the fields' Initial values.
func proInitial(fields []ProField, vals ProValues) {
	for _, fd := range fields {
		if _, ok := vals[fd.Key]; !ok && fd.Initial != nil {
			vals[fd.Key] = fd.Initial
		}
	}
}

// proResetValues clears vals back to the fields' Initial values.
func proResetValues(fields []ProField, vals ProValues) {
	for k := range vals {
		delete(vals, k)
	}
	proInitial(fields, vals)
}

// proParams converts non-empty values to query parameters (text form;
// dates as 2006-01-02).
func proParams(vals ProValues) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var s string
		switch t := vals[k].(type) {
		case time.Time:
			if !t.IsZero() {
				s = t.Format("2006-01-02")
			}
		default:
			s = proValueText(t)
		}
		if strings.TrimSpace(s) != "" {
			out[k] = s
		}
	}
	return out
}
