package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// FormLayout arranges labels and controls.
type FormLayout uint8

const (
	FormHorizontal FormLayout = iota // label left of the control
	FormVertical                     // label above
	FormInline                       // items in a wrapping row
)

// FormW groups FormItems with shared label settings.
type FormW struct {
	c          *Context
	n          *node
	st         *formState
	items      []*FormItemW
	layout     FormLayout
	labelWidth float32
	labelAlign TextAlign
	colon      bool
	scrollErr  bool
	size       ControlSize
	hasSize    bool
	trigger    FormTrigger
	hiddenKeys []string
	disabled   bool // Disabled: every control in the form
	noStar     bool // RequiredMark(false): hide the required asterisk
}

// Form adds a form container; build items with Item(...).Body.
func Form(c *Context) *FormW {
	f := &FormW{c: c, labelWidth: 100, labelAlign: AlignEnd, colon: c.Theme.Flavor == theme.Ant}
	f.n = c.add("form", "")
	f.n.setRole(a11y.RoleForm)
	f.st = State[formState](c, f.n.id, func() formState {
		return formState{errs: map[string]string{}, touched: map[string]bool{}, last: map[string]string{}, deps: map[string]string{}, async: map[string]*formAsync{}}
	})
	f.st.ensure()
	f.configDefaults()
	return f
}

func (f *FormW) Vertical() *FormW              { f.layout = FormVertical; return f }
func (f *FormW) Inline() *FormW                { f.layout = FormInline; return f }
func (f *FormW) LabelWidth(px float32) *FormW  { f.labelWidth = px; return f }
func (f *FormW) LabelAlign(a TextAlign) *FormW { f.labelAlign = a; return f }
func (f *FormW) Colon(on bool) *FormW          { f.colon = on; return f }
func (f *FormW) Layout(o ...layout.Option) *FormW {
	f.n.ln.Style.Apply(o...)
	return f
}

// labelAtEnd reports whether horizontal labels hug the end of their column
// (next to the control): AlignEnd (the default, Ant's labelAlign right) in
// either direction, or the physical side that is the end in this one.
func (f *FormW) labelAtEnd(rtl bool) bool {
	if f.layout != FormHorizontal {
		return false
	}
	switch f.labelAlign {
	case AlignEnd:
		return true
	case AlignPhysicalRight:
		return !rtl
	case AlignPhysicalLeft:
		return rtl
	}
	return false
}

// ScrollToFirstError scrolls the first failing item into view (in its
// nearest scroll container) when Validate or Submit fails.
func (f *FormW) ScrollToFirstError() *FormW { f.scrollErr = true; return f }

// Body builds the form's items.
func (f *FormW) Body(fn func()) *FormW {
	if f.layout == FormInline {
		f.n.ln.Dir = layout.Row
		f.n.ln.Wrap = true
		f.n.ln.Gap = 16
		f.n.ln.Align = layout.AlignStart
	}
	if f.hasSize {
		// The form size is the default size of every control inside.
		inner := fn
		fn = func() { ConfigProvider(f.c).Size(f.size).Body(inner) }
	}
	if f.disabled {
		// Form-level disabled: the subtree flag reaches every control.
		inner := fn
		fn = func() { Disabled(f.c, true, inner) }
	}
	f.c.body(f.n, fn)
	f.finish()
	return f
}

// finish runs after the items are built: drops state of items that are
// gone, completes a pending Submit and scrolls to a failed item.
func (f *FormW) finish() {
	st := f.st
	built := make(map[string]bool, len(f.items))
	for _, it := range f.items {
		built[it.key()] = true
	}
	for _, k := range f.hiddenKeys {
		built[k] = true // keeps its initial value for ResetFields
	}
	for k := range st.errs {
		if !built[k] {
			delete(st.errs, k)
		}
	}
	for k := range st.warns {
		if !built[k] {
			delete(st.warns, k)
		}
	}
	for k := range st.init {
		if !built[k] {
			delete(st.init, k)
			delete(st.restore, k)
		}
	}
	if st.submit {
		ok := f.Validate()
		if !f.Validating() {
			st.submit = false
			if ok {
				st.finished = true
				f.c.invalid = true
			}
		}
	}
	if st.scrollTo != "" {
		for _, it := range f.items {
			if it.key() != st.scrollTo || it.row == nil {
				continue
			}
			p := scrollAncestor(it.row)
			if p == nil || f.c.ScrollIntoView(p.id, it.row.id) {
				st.scrollTo = ""
			}
			break
		}
	}
}

// Submit validates every item (waiting for asynchronous rules) and makes
// Finished report true once all pass. It scrolls to the first error when
// validation fails (ScrollToFirstError).
func (f *FormW) Submit() {
	f.st.submit = true
	f.c.invalid = true
}

// Finished reports (once) that a Submit passed validation.
func (f *FormW) Finished() bool {
	if f.st.finished {
		f.st.finished = false
		return true
	}
	return false
}

// Submitting reports a Submit waiting for asynchronous validation.
func (f *FormW) Submitting() bool { return f.st.submit }

// Validating reports an asynchronous rule still running.
func (f *FormW) Validating() bool {
	f.st.mu.Lock()
	defer f.st.mu.Unlock()
	for _, a := range f.st.async {
		if a.pending {
			return true
		}
	}
	return false
}

// Value returns the last value seen of the item named name (Name or label);
// use it for linkage: f.Item("城市").Hidden(f.Value("国家") == "").
func (f *FormW) Value(name string) string { return f.st.last[name] }

// FormItemW is one labelled form row.
type FormItemW struct {
	f        *FormW
	value    func() string
	rules    []Rule
	label    string
	required bool
	err      string
	help     string
	status   string
	noLabel  bool

	name     string
	deps     []string
	hidden   bool
	disabled bool
	onDeps   func()
	row      *node
	pending  bool
	labelID  ID

	tooltip string
	tipID   ID // tooltip anchor (tests)
	extra   string
	extraFn func()
	vstatus string // ValidateStatus
	warn    string // failing Warning() rule
	trigger FormTrigger
	snap    func() any  // copy of the bound value (ResetFields)
	restore func(v any) // writes a snapshot back
}

// field passes the label, required flag and validation message to the
// first control inside the item (a11y_tree.go).
func (it *FormItemW) field() (string, ID, bool, string) {
	return it.label, it.labelID, it.required, it.err
}

func (it *FormItemW) semantics(*a11y.Node) {}

// Item starts a form item; finish with Body.
func (f *FormW) Item(label string) *FormItemW { return &FormItemW{f: f, label: label} }

func (it *FormItemW) Required() *FormItemW { it.required = true; return it }
func (it *FormItemW) Error(msg string) *FormItemW {
	if msg != "" {
		it.err, it.status = msg, "error"
	}
	return it
}
func (it *FormItemW) Help(msg string) *FormItemW { it.help = msg; return it }

// NoLabel aligns the control with labelled items but shows no label (submit row).
func (it *FormItemW) NoLabel() *FormItemW { it.noLabel = true; return it }

// Name sets the field name that keys the item's validation state (default:
// the label). Items with the same label (Form.List rows) need distinct names.
func (it *FormItemW) Name(name string) *FormItemW { it.name = name; return it }

// Dependencies names fields this item depends on: when one of their values
// changes the item re-validates (if it has a value or was validated) and
// OnDepsChange runs (reset a dependent value, refresh options).
func (it *FormItemW) Dependencies(names ...string) *FormItemW { it.deps = names; return it }

// OnDepsChange runs fn when a dependency's value changed.
func (it *FormItemW) OnDepsChange(fn func()) *FormItemW { it.onDeps = fn; return it }

// Hidden skips the item: nothing is built and it is not validated.
func (it *FormItemW) Hidden(on bool) *FormItemW { it.hidden = on; return it }

// Disabled disables every control of the item.
func (it *FormItemW) Disabled(on bool) *FormItemW { it.disabled = on; return it }

func (it *FormItemW) key() string {
	if it.name != "" {
		return it.name
	}
	return it.label
}

// Rules attaches validation rules to a string value. The item validates
// live once the value changed (see ValidateTrigger), and on
// FormW.Validate. The value is restored by FormW.ResetFields.
func (it *FormItemW) Rules(val *string, rules ...Rule) *FormItemW {
	it.RulesFunc(func() string { return *val }, rules...)
	it.snap = func() any { return *val }
	it.restore = func(v any) { *val = v.(string) }
	return it
}

// RulesFunc is Rules for values that are not strings (format them).
func (it *FormItemW) RulesFunc(val func() string, rules ...Rule) *FormItemW {
	it.value, it.rules = val, rules
	for _, r := range rules {
		if r.required {
			it.required = true
		}
	}
	return it
}

// check runs the rules: synchronous ones first, then asynchronous ones
// (started once per value; pending reports a result still awaited).
func (it *FormItemW) check() string {
	v := it.value()
	it.pending = false
	it.warn = ""
	var async []Rule
	for _, r := range it.rules {
		if r.async != nil {
			async = append(async, r)
			continue
		}
		if r.warning {
			continue
		}
		if msg := r.check(v); msg != "" {
			return msg
		}
	}
	for _, r := range it.rules {
		if r.warning && r.check != nil {
			if msg := r.check(v); msg != "" {
				it.warn = msg
				break
			}
		}
	}
	if len(async) == 0 {
		return ""
	}
	st := it.f.st
	st.mu.Lock()
	defer st.mu.Unlock()
	a := st.async[it.key()]
	if a == nil {
		a = &formAsync{}
		st.async[it.key()] = a
	}
	if a.started && a.val == v && !a.pending {
		return a.msg
	}
	if a.started && a.val == v {
		it.pending = true
		return ""
	}
	a.started, a.val, a.pending, a.msg = true, v, true, ""
	a.seq++
	seq := a.seq
	win := it.f.c.win
	rules := async
	var run func(i int)
	run = func(i int) {
		if i == len(rules) {
			st.mu.Lock()
			if a.seq == seq {
				a.pending = false
			}
			st.mu.Unlock()
			if win != nil {
				win.Invalidate()
			}
			return
		}
		rules[i].async(v, func(msg string) {
			if msg != "" {
				st.mu.Lock()
				if a.seq == seq {
					a.msg, a.pending = msg, false
				}
				st.mu.Unlock()
				if win != nil {
					win.Invalidate()
				}
				return
			}
			run(i + 1)
		})
	}
	st.mu.Unlock()
	run(0) // synchronous validators may complete right away
	st.mu.Lock()
	if a.seq == seq && !a.pending {
		return a.msg
	}
	it.pending = true
	return ""
}

// Body builds the control(s) of the item.
func (it *FormItemW) Body(fn func()) {
	f, c := it.f, it.f.c
	th := c.ThemeFor(theme.CompForm)
	tk := &th.Components.Form
	key := it.key()
	if len(it.deps) > 0 {
		sig := ""
		for _, d := range it.deps {
			sig += f.st.last[d] + "\x00"
		}
		if old, seen := f.st.deps[key]; seen && old != sig {
			if it.onDeps != nil {
				it.onDeps()
				c.invalid = true
			}
			if it.value != nil && (f.st.touched[key] || it.value() != "") {
				f.st.touched[key] = true
			}
		}
		f.st.deps[key] = sig
	}
	if len(it.deps) > 0 && f.st.touched[key] && it.value != nil {
		if old, seen := f.st.depsSeen[key]; seen && old != f.st.deps[key] {
			f.st.force[key] = true // a dependency changed: re-check now
		}
		f.st.depsSeen[key] = f.st.deps[key]
	}
	if it.hidden {
		if it.value != nil {
			f.st.last[key] = it.value()
		}
		delete(f.st.errs, key)
		delete(f.st.warns, key)
		f.hiddenKeys = append(f.hiddenKeys, key)
		return
	}
	trig := it.validateTrigger()
	if it.value != nil {
		f.items = append(f.items, it)
		st := f.st
		if it.snap != nil {
			if _, ok := st.init[key]; !ok {
				st.init[key] = it.snap()
			}
			st.restore[key] = it.restore
		}
		v := it.value()
		changed := false
		if old, seen := st.last[key]; seen && old != v {
			changed = true
		}
		st.last[key] = v
		run := false
		switch {
		case st.force[key]:
			delete(st.force, key)
			st.touched[key], run = true, true
		case trig == ValidateOnChange:
			if changed {
				st.touched[key] = true
			}
			run = st.touched[key]
		default:
			// onBlur / onSubmit: only the trigger validates a new value;
			// the shown result is refreshed while the value is unchanged
			// (asynchronous rules finishing).
			run = st.touched[key] && st.checked[key] == v
		}
		if run {
			st.errs[key] = it.check()
			st.warns[key] = it.warn
			st.checked[key] = v
		}
		if it.err == "" && st.errs[key] != "" {
			it.Error(st.errs[key])
		}
		if it.pending && it.help == "" {
			it.help = c.Locale().Form.Validating
		}
		it.warn = st.warns[key]
		if st.errs[key] != "" {
			it.warn = ""
		}
	}
	if it.status == "" {
		switch {
		case it.vstatus != "":
			it.status = it.vstatus
		case it.warn != "":
			it.status = "warning"
		}
	}
	if it.vstatus == "validating" && it.help == "" {
		it.help = c.Locale().Form.Validating
	}
	if it.disabled {
		inner := fn
		fn = func() { Disabled(c, true, inner) }
	}
	prevStatus := c.itemStatus
	c.itemStatus = it.status
	defer func() { c.itemStatus = prevStatus }()
	dir := layout.Row
	if f.layout == FormVertical {
		dir = layout.Column
	}
	mb := tk.ItemMarginBottom
	shadForm := th.Flavor == theme.StyleShadcn
	if (it.err != "" || it.help != "" || it.warn != "") && !shadForm {
		mb = 0 // Ant / Element: the message fills the item's bottom margin
	}
	row := c.box("form-item", dir, nil)
	row.n.fill(it)
	it.row = row.n
	row.n.ln.Margin.Bottom = mb
	if f.layout == FormInline {
		row.n.ln.Margin = geom.Insets{Bottom: tk.InlineItemMarginBottom}
	}
	row.Body(func() {
		if f.layout != FormVertical || it.label != "" {
			lw := f.labelWidth
			if f.layout == FormInline {
				lw = 0
			}
			label := c.add("form-label", it.label)
			it.labelID = label.id
			label.setRole(a11y.RoleText)
			label.aHidden = it.noLabel || it.label == ""
			label.ln.Height = tk.LabelHeight
			if f.layout == FormVertical {
				// shadcn FormItem: grid gap-2 under a leading-none label.
				label.ln.Height = tk.VerticalLabelHeight + tk.VerticalLabelPadding
				label.ln.Padding.Bottom = tk.VerticalLabelPadding
			}
			if lw > 0 && f.layout == FormHorizontal {
				label.ln.Width = lw
			}
			st := c.style(tk.LabelFontSize, text.Regular)
			if tk.LabelFontWeight > 0 {
				st.Weight = text.Weight(tk.LabelFontWeight)
			}
			s := it.label
			colon := ""
			if f.colon && s != "" && f.layout != FormVertical {
				colon = strings.TrimPrefix(c.Locale().WithColon(s), s)
			}
			tipW := float32(0)
			if it.tooltip != "" && !it.noLabel && s != "" {
				tipW = formTipGap + formTipSize
			}
			label.ln.Measure = func(_, _ float32) geom.Size {
				w := text.Width(s+colon, st) + tipW
				if it.star() {
					w += 11
				}
				return geom.Size{W: w + 8, H: 22}
			}
			if tipW > 0 {
				it.labelTip(s, colon, st, lw)
			}
			label.paint = func(l *paint.List, r geom.Rect) {
				if it.noLabel {
					return
				}
				lay := text.Shape(s, st, c.textOpts(text.Options{NoWrap: true}))
				cl := text.Shape(colon, st, c.textOpts(text.Options{NoWrap: true}))
				w := lay.Width + tipW + cl.Width
				if it.star() {
					w += 11
				}
				// The label runs [*][text][tip][colon] from the start edge;
				// off is measured from the start, so in RTL the group is
				// mirrored (star at the right, colon toward the control).
				sp := c.inline(r)
				off := float32(0)
				pr := tk.LabelColonMarginInlineEnd
				if f.labelAtEnd(c.IsRTL()) {
					off = r.W - pr - w
					if f.colon {
						off += 6
					}
				}
				y := r.Y + (tk.LabelHeight-lay.Height)/2
				if f.layout == FormVertical {
					y = r.Y
					if shadForm {
						y -= (lay.Height - shadFormLabelH) / 2 // leading-none
					}
				}
				if it.star() {
					star := text.Shape("*", c.style(tk.LabelFontSize, text.Regular), text.Options{NoWrap: true})
					l.Glyphs(geom.Pt(sp.X(off, star.Width), y+2), star.Glyphs, tk.LabelRequiredMarkColor)
					off += 11
				}
				col := tk.LabelColor
				if it.err != "" && tk.LabelErrorColor.A > 0 {
					col = tk.LabelErrorColor // shadcn FormLabel turns destructive on error
				}
				l.Glyphs(geom.Pt(sp.X(off, lay.Width), y), lay.Glyphs, col)
				if colon != "" {
					l.Glyphs(geom.Pt(sp.X(off+lay.Width+tipW, cl.Width), y), cl.Glyphs, col)
				}
			}
		}
		Column(c, func() {
			fn()
			msg, col := it.err, tk.ErrorColor
			if msg == "" && it.warn != "" {
				msg, col = it.warn, tk.WarningColor
			}
			if msg == "" {
				msg, col = it.help, tk.HelpColor
				switch it.status {
				case "error", "warning", "success":
					col = th.ColorOf(it.status).Base
				}
			}
			if msg != "" {
				t := Text(c, msg).Color(col)
				if it.err != "" {
					// Validation errors are announced when they appear.
					t.n.setRole(a11y.RoleAlert)
				} else if msg == it.warn {
					t.n.setRole(a11y.RoleStatus)
				}
				formMessageText(t, tk)
				if th.Flavor == theme.StyleShadcn {
					// FormMessage / FormDescription: text-sm, gap-2 under the control.
					t.Layout(layout.Margin(geom.Insets{Top: shadFormGap}))
				} else {
					Gap(c, 2)
				}
			}
			it.buildExtra()
		}, layout.Grow(1), layout.MinW(0))
	})
	it.trackBlur(trig)
}

// formMessageText applies the token message size to a help / error /
// Extra line.
func formMessageText(t *Label, tk *theme.FormToken) {
	if tk.MessageFontSize > 0 {
		t.Size(tk.MessageFontSize)
	}
	if tk.MessageLineHeight > 0 {
		t.LineHeight(tk.MessageLineHeight)
	}
}

// FormStatus returns the status string to pass to a control for an error.
func FormStatus(err string) string {
	if err != "" {
		return "error"
	}
	return ""
}

type formState struct {
	errs    map[string]string
	touched map[string]bool
	last    map[string]string
	deps    map[string]string // dependency values seen per item

	warns    map[string]string // failing Warning() rules per item
	checked  map[string]string // value the shown result was computed for
	force    map[string]bool   // validate on the next build (blur, deps)
	focusIn  map[string]bool   // focus was inside the item (onBlur)
	depsSeen map[string]string
	init     map[string]any // first value seen per item (ResetFields)
	restore  map[string]func(any)

	mu       sync.Mutex // guards async (validators finish on any goroutine)
	async    map[string]*formAsync
	submit   bool
	finished bool
	scrollTo string
}

// formAsync is the state of an item's asynchronous rules.
type formAsync struct {
	val     string // value being / last validated
	msg     string
	seq     int
	pending bool
	started bool
}

// Validate checks every item with rules built so far in this frame, shows
// their errors and reports whether all passed. Items whose asynchronous
// rules are still running count as not passed (see Validating, Submit).
func (f *FormW) Validate() bool { return f.validate(nil) }

// validate checks the items whose key passes want (nil: all).
func (f *FormW) validate(want map[string]bool) bool {
	ok := true
	first := ""
	for _, it := range f.items {
		k := it.key()
		if want != nil && !want[k] {
			continue
		}
		f.st.touched[k] = true
		msg := it.check()
		f.st.errs[k] = msg
		f.st.warns[k] = it.warn
		f.st.checked[k] = it.value()
		if msg != "" || it.pending {
			ok = false
		}
		if msg != "" && first == "" {
			first = k
		}
	}
	if first != "" && f.scrollErr && !f.Validating() {
		f.st.scrollTo = first
	}
	f.c.invalid = true
	return ok
}

// ResetValidation clears errors and touched flags.
func (f *FormW) ResetValidation() {
	clear(f.st.errs)
	clear(f.st.warns)
	clear(f.st.checked)
	clear(f.st.force)
	clear(f.st.touched)
	f.st.mu.Lock()
	clear(f.st.async)
	f.st.mu.Unlock()
	f.st.submit, f.st.scrollTo = false, ""
	f.c.invalid = true
}

// Errors returns the current error message per item label.
func (f *FormW) Errors() map[string]string { return f.st.errs }

// Rule validates a form value; build rules with Required, MinLen, MaxLen,
// Pattern, Email, Match, Check or AsyncCheck.
type Rule struct {
	required bool
	warning  bool // Warning(): shown as a warning, does not fail Validate
	check    func(v string) string
	async    func(v string, done func(msg string))
}

// AsyncCheck validates asynchronously (a server lookup): fn starts the
// check and calls done (from any goroutine) with an error message or "".
// It runs after the synchronous rules pass, once per value; the item shows
// a "validating" hint meanwhile.
func AsyncCheck(fn func(v string, done func(msg string))) Rule { return Rule{async: fn} }

// Match fails when the value differs from *other (confirm password). Give
// the item Dependencies(otherName) to re-check when the other field changes.
func Match(other *string, msg string) Rule {
	return Rule{check: func(v string) string {
		if v != *other {
			return msg
		}
		return ""
	}}
}

// Required fails on an empty value.
func Required(msg string) Rule {
	return Rule{required: true, check: func(v string) string {
		if v == "" {
			return msg
		}
		return ""
	}}
}

// MinLen fails when the value has fewer than n characters (empty passes;
// combine with Required).
func MinLen(n int, msg string) Rule {
	return Rule{check: func(v string) string {
		if v != "" && utf8.RuneCountInString(v) < n {
			return msg
		}
		return ""
	}}
}

// MaxLen fails when the value has more than n characters.
func MaxLen(n int, msg string) Rule {
	return Rule{check: func(v string) string {
		if utf8.RuneCountInString(v) > n {
			return msg
		}
		return ""
	}}
}

// Pattern fails when a non-empty value does not match re.
func Pattern(re string, msg string) Rule {
	rx := regexp.MustCompile(re)
	return Rule{check: func(v string) string {
		if v != "" && !rx.MatchString(v) {
			return msg
		}
		return ""
	}}
}

// Email checks a simple e-mail shape.
func Email(msg string) Rule { return Pattern(`^[^@\s]+@[^@\s]+\.[^@\s]+$`, msg) }

// Check wraps a custom validator returning an error message or "".
func Check(fn func(v string) string) Rule { return Rule{check: fn} }
