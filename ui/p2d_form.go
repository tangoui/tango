package ui

import (
	"cmp"
	"fmt"
	"reflect"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// FormTrigger says when a form item validates its value.
//
// Append-only: never reorder or insert; the zero value stays ValidateInherit.
type FormTrigger uint8

const (
	// ValidateInherit uses the form's trigger (items) or onChange (forms).
	ValidateInherit FormTrigger = iota
	// ValidateOnChange validates live once the value changed (default).
	ValidateOnChange
	// ValidateOnBlur validates when focus leaves the item's control.
	ValidateOnBlur
	// ValidateOnSubmit validates only in Validate / Submit.
	ValidateOnSubmit
)

// parseTrigger maps "onChange" | "onBlur" | "onSubmit" (ConfigProvider
// defaults) to a trigger.
func parseTrigger(s string) FormTrigger {
	switch s {
	case "onChange", "change":
		return ValidateOnChange
	case "onBlur", "blur":
		return ValidateOnBlur
	case "onSubmit", "submit":
		return ValidateOnSubmit
	}
	return ValidateInherit
}

const (
	formTipSize = 14 // label tooltip icon
	formTipGap  = 4
)

func (st *formState) ensure() {
	if st.warns == nil {
		st.warns = map[string]string{}
	}
	if st.checked == nil {
		st.checked = map[string]string{}
	}
	if st.force == nil {
		st.force = map[string]bool{}
	}
	if st.focusIn == nil {
		st.focusIn = map[string]bool{}
	}
	if st.depsSeen == nil {
		st.depsSeen = map[string]string{}
	}
	if st.init == nil {
		st.init = map[string]any{}
	}
	if st.restore == nil {
		st.restore = map[string]func(any){}
	}
}

// configDefaults applies ConfigProvider Defaults("Form", ...): "layout"
// ("horizontal" | "vertical" | "inline"), "labelWidth", "labelAlign"
// ("left" | "right"), "colon", "requiredMark" (bool), "disabled" (bool),
// "validateTrigger" ("onChange" | "onBlur" | "onSubmit") and "size"
// (ControlSize or "small" | "middle" | "large").
func (f *FormW) configDefaults() {
	c := f.c
	switch propString(c, "Form", "layout", "") {
	case "vertical":
		f.layout = FormVertical
	case "inline":
		f.layout = FormInline
	}
	f.labelWidth = propFloat(c, "Form", "labelWidth", f.labelWidth)
	switch propString(c, "Form", "labelAlign", "") {
	case "left": // Ant's left / right are logical (text-align start / end)
		f.labelAlign = AlignStart
	case "right":
		f.labelAlign = AlignEnd
	}
	f.colon = propBool(c, "Form", "colon", f.colon)
	f.noStar = !propBool(c, "Form", "requiredMark", true)
	f.disabled = propBool(c, "Form", "disabled", false)
	f.trigger = parseTrigger(propString(c, "Form", "validateTrigger", ""))
	if s, ok := formPropSize(c, "Form"); ok {
		f.size, f.hasSize = s, true
	}
}

// formPropSize reads a "size" default given as a ControlSize or as Ant's
// "small" | "middle" | "large".
func formPropSize(c *Context, comp string) (ControlSize, bool) {
	v, ok := c.ConfigProp(comp, "size")
	if !ok {
		return Middle, false
	}
	switch x := v.(type) {
	case ControlSize:
		return x, true
	case string:
		switch x {
		case "small":
			return Small, true
		case "middle", "default":
			return Middle, true
		case "large":
			return Large, true
		}
	}
	return Middle, false
}

// Disabled disables every control in the form (Ant Form disabled, Element
// el-form disabled); it passes down like ui.Disabled.
func (f *FormW) Disabled(on bool) *FormW { f.disabled = on; return f }

// RequiredMark shows (default) or hides the asterisk of required items
// (Ant requiredMark, Element hide-required-asterisk). Required rules
// still apply.
func (f *FormW) RequiredMark(on bool) *FormW { f.noStar = !on; return f }

// star reports whether the item draws the required asterisk.
func (it *FormItemW) star() bool { return it.required && !it.f.noStar }

// ValidateFields validates only the named items (Name, or the label) and
// reports whether they all passed (Ant validateFields(names), Element
// validateField). Without names it is Validate.
func (f *FormW) ValidateFields(names ...string) bool {
	if len(names) == 0 {
		return f.Validate()
	}
	return f.validate(formNameSet(names))
}

// ClearValidate removes the validation result of the named items (all
// items without names) without touching their values (Ant setFields /
// Element clearValidate). They validate again live after the next edit,
// or on Validate.
func (f *FormW) ClearValidate(names ...string) {
	if len(names) == 0 {
		f.ResetValidation()
		return
	}
	st := f.st
	st.mu.Lock()
	for _, k := range names {
		delete(st.async, k)
	}
	st.mu.Unlock()
	for _, k := range names {
		delete(st.errs, k)
		delete(st.warns, k)
		delete(st.checked, k)
		delete(st.force, k)
		delete(st.touched, k)
		if st.scrollTo == k {
			st.scrollTo = ""
		}
	}
	f.c.invalid = true
}

func formNameSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// OnReset makes ResetFields call fn for this item: items bound with
// RulesFunc have no pointer the form could restore, so the application
// resets its own value (Rules and RulesOf restore automatically).
//
//	f.Item("生日").RulesFunc(func() string { return d.Format(ui.DateLayout) }, ...).OnReset(func() { d = time.Time{} })
func (it *FormItemW) OnReset(fn func()) *FormItemW {
	if fn == nil {
		return it
	}
	it.snap = func() any { return nil }
	it.restore = func(any) { fn() }
	return it
}

// Size sets the size of every control in the form (Ant Form size,
// Element el-form size); a control's own Size still wins.
func (f *FormW) Size(s ControlSize) *FormW { f.size, f.hasSize = s, true; return f }

// ValidateTrigger sets when items validate (default ValidateOnChange);
// FormItemW.ValidateTrigger overrides it per item.
func (f *FormW) ValidateTrigger(t FormTrigger) *FormW { f.trigger = t; return f }

// ResetFields restores every item bound with Rules(*string) or RulesOf to
// the value it had when the item was first built, calls the OnReset
// callback of RulesFunc items, and clears validation (Ant resetFields,
// Element resetFields). With names only those items are reset.
func (f *FormW) ResetFields(names ...string) {
	st := f.st
	var want map[string]bool
	if len(names) > 0 {
		want = formNameSet(names)
	}
	for k, fn := range st.restore {
		if want != nil && !want[k] {
			continue
		}
		if v, ok := st.init[k]; ok && fn != nil {
			fn(v)
		}
		delete(st.last, k) // the restored value is not an edit
		delete(st.depsSeen, k)
		delete(st.focusIn, k)
	}
	if want != nil {
		f.ClearValidate(names...)
		return
	}
	clear(st.focusIn)
	f.ResetValidation()
}

// Tooltip shows a question-mark icon after the label with a tooltip (Ant
// Form.Item tooltip).
func (it *FormItemW) Tooltip(tip string) *FormItemW { it.tooltip = tip; return it }

// Extra shows a description line under the control and its message (Ant
// Form.Item extra).
func (it *FormItemW) Extra(s string) *FormItemW { it.extra = s; return it }

// ExtraBody builds custom content under the control (Extra as content).
func (it *FormItemW) ExtraBody(fn func()) *FormItemW { it.extraFn = fn; return it }

// ValidateStatus sets the status by hand: "success" | "warning" | "error"
// | "validating" (Ant validateStatus). The Help text takes the status
// color; a failing rule still shows its error.
func (it *FormItemW) ValidateStatus(s string) *FormItemW { it.vstatus = s; return it }

// ValidateTrigger sets when this item validates (see FormTrigger).
func (it *FormItemW) ValidateTrigger(t FormTrigger) *FormItemW { it.trigger = t; return it }

func (it *FormItemW) validateTrigger() FormTrigger {
	if it.trigger != ValidateInherit {
		return it.trigger
	}
	if it.f.trigger != ValidateInherit {
		return it.f.trigger
	}
	return ValidateOnChange
}

// trackBlur validates an onBlur item when focus left its subtree.
func (it *FormItemW) trackBlur(trig FormTrigger) {
	if trig != ValidateOnBlur || it.value == nil || it.row == nil {
		return
	}
	c, st, key := it.f.c, it.f.st, it.key()
	in := c.focusWithin(it.row)
	if st.focusIn[key] && !in {
		st.force[key] = true
		c.invalid = true
	}
	st.focusIn[key] = in
}

// labelTip adds the label's tooltip icon. It is a sibling of the label
// (a leaf), positioned over the gap the label leaves for it.
func (it *FormItemW) labelTip(s, colon string, st text.Style, lw float32) {
	f, c := it.f, it.f.c
	tk := &c.ThemeFor(theme.CompForm).Components.Form
	req := float32(0)
	if it.star() {
		req = 11
	}
	sw, cw := text.Width(s, st), text.Width(colon, st)
	total := req + sw + formTipGap + formTipSize + cw
	left := req + sw + formTipGap
	top := (tk.LabelHeight - formTipSize) / 2
	if f.layout == FormVertical {
		top = (22 - formTipSize) / 2
	}
	if f.labelAtEnd(c.IsRTL()) { // left is a start offset (Abs mirrors in RTL)
		pr := tk.LabelColonMarginInlineEnd
		boxW := lw
		if boxW <= 0 {
			boxW = total + 8
		}
		x := boxW - pr - total
		if f.colon {
			x += 6
		}
		left = x + req + sw + formTipGap
	}
	tip := it.tooltip
	b := Tooltip(c, tip, func() {
		n := c.add("form-tip", "")
		n.ln.Width, n.ln.Height = formTipSize, formTipSize
		n.aHidden = true // the tooltip anchor describes the item
		n.paint = func(l *paint.List, r geom.Rect) {
			drawIcon(l, icon.Question, r.X, r.Y, r.W, tk.LabelTooltipColor)
		}
	})
	flexbox.Apply(&b.n.ln.Style, layout.Abs(left, top, layout.Unset, layout.Unset))
	it.tipID = b.n.id
}

// buildExtra adds the Extra line under the control.
func (it *FormItemW) buildExtra() {
	if it.extra == "" && it.extraFn == nil {
		return
	}
	c := it.f.c
	tk := &c.ThemeFor(theme.CompForm).Components.Form
	Column(c, func() {
		if it.extra != "" {
			formMessageText(Text(c, it.extra).Color(tk.HelpColor), tk)
		}
		if it.extraFn != nil {
			it.extraFn()
		}
	}, layout.Margin(geom.Insets{Top: tk.ExtraMarginTop}))
}

// Warning turns the rule into a warning: a failing value shows the
// message in the warning color and status, but Validate still passes (Ant
// rule warningOnly).
func (r Rule) Warning() Rule { r.warning = true; return r }

// ---------------------------------------------------------------------------
// Rules for values of any type

// RuleOf validates a form value of type T; build them with RequiredValue,
// MinValue, MaxValue, RangeValue, RequiredItems, MinItems, MaxItems,
// MustBeTrue or CheckValue, and attach them with RulesOf.
type RuleOf[T any] struct {
	required bool
	warning  bool
	check    func(v T) string
}

// Warning turns the rule into a warning (see Rule.Warning).
func (r RuleOf[T]) Warning() RuleOf[T] { r.warning = true; return r }

// RulesOf attaches typed rules to *val (numbers, booleans, slices …) and
// binds the value for ResetFields; it is Rules for any type. The item's
// Value (FormW.Value, Dependencies) is the value formatted with fmt.
//
//	ui.RulesOf(form.Item("年龄"), &age, ui.RangeValue(18.0, 60.0, "18–60"))
func RulesOf[T any](it *FormItemW, val *T, rules ...RuleOf[T]) *FormItemW {
	rs := make([]Rule, len(rules))
	for i, r := range rules {
		chk := r.check
		rs[i] = Rule{required: r.required, warning: r.warning, check: func(string) string {
			if chk == nil {
				return ""
			}
			return chk(*val)
		}}
	}
	it.RulesFunc(func() string { return fmt.Sprint(*val) }, rs...)
	it.snap = func() any { return cloneValue(*val) }
	it.restore = func(v any) { *val = cloneValue(v.(T)) }
	return it
}

// cloneValue copies slices and maps so a snapshot does not share storage
// with the value the application keeps editing.
func cloneValue[T any](v T) T {
	rv := reflect.ValueOf(&v).Elem()
	switch rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return v
		}
		cp := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		reflect.Copy(cp, rv)
		rv.Set(cp)
	case reflect.Map:
		if rv.IsNil() {
			return v
		}
		cp := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		it := rv.MapRange()
		for it.Next() {
			cp.SetMapIndex(it.Key(), it.Value())
		}
		rv.Set(cp)
	}
	return v
}

// RequiredValue fails on the zero value (0, false, "", nil).
func RequiredValue[T comparable](msg string) RuleOf[T] {
	var zero T
	return RuleOf[T]{required: true, check: func(v T) string {
		if v == zero {
			return msg
		}
		return ""
	}}
}

// MinValue fails when the value is below lo.
func MinValue[T cmp.Ordered](lo T, msg string) RuleOf[T] {
	return RuleOf[T]{check: func(v T) string {
		if v < lo {
			return msg
		}
		return ""
	}}
}

// MaxValue fails when the value is above hi.
func MaxValue[T cmp.Ordered](hi T, msg string) RuleOf[T] {
	return RuleOf[T]{check: func(v T) string {
		if v > hi {
			return msg
		}
		return ""
	}}
}

// RangeValue fails outside [lo, hi].
func RangeValue[T cmp.Ordered](lo, hi T, msg string) RuleOf[T] {
	return RuleOf[T]{check: func(v T) string {
		if v < lo || v > hi {
			return msg
		}
		return ""
	}}
}

// RequiredItems fails on an empty slice (checkbox group, multi-select,
// upload list).
func RequiredItems[E any](msg string) RuleOf[[]E] {
	return RuleOf[[]E]{required: true, check: func(v []E) string {
		if len(v) == 0 {
			return msg
		}
		return ""
	}}
}

// MinItems fails when a non-empty slice has fewer than n elements
// (combine with RequiredItems).
func MinItems[E any](n int, msg string) RuleOf[[]E] {
	return RuleOf[[]E]{check: func(v []E) string {
		if len(v) > 0 && len(v) < n {
			return msg
		}
		return ""
	}}
}

// MaxItems fails when the slice has more than n elements.
func MaxItems[E any](n int, msg string) RuleOf[[]E] {
	return RuleOf[[]E]{check: func(v []E) string {
		if len(v) > n {
			return msg
		}
		return ""
	}}
}

// MustBeTrue fails unless the value is true ("I agree to the terms"); the
// item shows the required mark.
func MustBeTrue(msg string) RuleOf[bool] {
	return RuleOf[bool]{required: true, check: func(v bool) string {
		if !v {
			return msg
		}
		return ""
	}}
}

// CheckValue wraps a custom validator returning an error message or "".
func CheckValue[T any](fn func(v T) string) RuleOf[T] { return RuleOf[T]{check: fn} }

// ---------------------------------------------------------------------------
// ConfigProvider default helpers (numbers of any type, strings, bools).

func propString(c *Context, comp, key, def string) string {
	return Prop(c, comp, key, def)
}

func propBool(c *Context, comp, key string, def bool) bool {
	return Prop(c, comp, key, def)
}

func propFloat(c *Context, comp, key string, def float32) float32 {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float32:
		return x
	case float64:
		return float32(x)
	case int:
		return float32(x)
	case int64:
		return float32(x)
	}
	return def
}

func propInt64(c *Context, comp, key string, def int64) int64 {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return def
	}
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	}
	return def
}
