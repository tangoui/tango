package ui

import (
	"sync"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// ProFormW renders a form from field definitions (Ant Design ProForm): a
// grid of fields whose controls follow their value types (shared with
// ProTable columns), linkage, validation (sync, cross-field, async),
// submit / reset buttons with a loading state and asynchronous initial
// values.
//
//	ui.ProForm(c, fields).Columns(2).
//		Request(func(done func(ui.ProValues, error)) { go load(done) }).
//		OnFinish(func(v ui.ProValues, done func(error)) { go save(v, done) }).
//		Show()
type ProFormW struct {
	c          *Context
	fields     []ProField
	vals       *ProValues
	cols       int
	horizontal bool
	labelWidth float32
	request    func(done func(ProValues, error))
	onFinish   func(vals ProValues, done func(error))
	submitText string
	resetText  string
	noReset    bool
	noSubmit   bool
	submitter  func(s *ProSubmitter)
	children   func(f *FormW)
	gutter     float32
	id         ID

	st       *proFormState
	form     *FormW
	finished bool
}

// ProSubmitter is passed to a custom submitter.
type ProSubmitter struct {
	c *Context
	p *ProFormW
}

// Context returns the build context.
func (s *ProSubmitter) Context() *Context { return s.c }

// Submit validates and submits the form.
func (s *ProSubmitter) Submit() { s.p.Submit() }

// Reset restores the initial values.
func (s *ProSubmitter) Reset() { s.p.Reset() }

// Loading reports a running submit (show it on the submit button).
func (s *ProSubmitter) Loading() bool { return s.p.Submitting() }

type proFormState struct {
	vals       ProValues
	init       ProValues // values after the initial request (reset target)
	requested  bool
	loading    bool
	submitting bool
	reset      bool
	err        error
	reqErr     error

	mu      sync.Mutex
	initRes *proInitResult
	subRes  *proSubResult
	seq     int
}

type proInitResult struct {
	vals ProValues
	err  error
}

type proSubResult struct{ err error }

// ProForm starts a form over fields; chain options, then Show.
func ProForm(c *Context, fields []ProField) *ProFormW {
	p := &ProFormW{c: c, fields: fields, cols: 1, labelWidth: 100, gutter: 24}
	// ConfigProvider.Defaults("ProForm", ...) keys: "layout"
	// ("vertical" | "horizontal"), "labelWidth", "columns", "gutter"
	// (number), "submitter" (false hides the buttons).
	p.horizontal = Prop(c, "ProForm", "layout", "vertical") == "horizontal"
	if v, ok := p2bNum(c, "ProForm", "labelWidth"); ok {
		p.labelWidth = float32(v)
	}
	if n, ok := proPropInt(c, "ProForm", "columns"); ok && n > 0 {
		p.cols = n
	}
	if v, ok := p2bNum(c, "ProForm", "gutter"); ok {
		p.gutter = float32(v)
	}
	p.noSubmit = !Prop(c, "ProForm", "submitter", true)
	return p
}

// Values binds the values (otherwise the form keeps its own).
func (p *ProFormW) Values(v *ProValues) *ProFormW { p.vals = v; return p }

// Columns lays fields out in a grid of n equal columns (ProField.Span
// overrides a field's width out of 24).
func (p *ProFormW) Columns(n int) *ProFormW { p.cols = max(1, n); return p }

// Horizontal puts labels left of the controls (default: above).
func (p *ProFormW) Horizontal() *ProFormW { p.horizontal = true; return p }

// LabelWidth sets the label width of a horizontal form.
func (p *ProFormW) LabelWidth(px float32) *ProFormW { p.labelWidth = px; return p }

// Request loads the initial values asynchronously; call done from any
// goroutine. The form shows a loading mask meanwhile.
func (p *ProFormW) Request(fn func(done func(ProValues, error))) *ProFormW { p.request = fn; return p }

// OnFinish receives the values after a successful validation; call done
// (from any goroutine) when saved. The submit button spins meanwhile and a
// non-nil error is shown above the buttons.
func (p *ProFormW) OnFinish(fn func(vals ProValues, done func(error))) *ProFormW {
	p.onFinish = fn
	return p
}

// SubmitText sets the submit button's text.
func (p *ProFormW) SubmitText(s string) *ProFormW { p.submitText = s; return p }

// ResetText sets the reset button's text.
func (p *ProFormW) ResetText(s string) *ProFormW { p.resetText = s; return p }

// NoReset hides the reset button.
func (p *ProFormW) NoReset() *ProFormW { p.noReset = true; return p }

// NoSubmitter hides the buttons (submit with Submit, e.g. from a modal).
func (p *ProFormW) NoSubmitter() *ProFormW { p.noSubmit = true; return p }

// Submitter replaces the buttons.
func (p *ProFormW) Submitter(fn func(s *ProSubmitter)) *ProFormW { p.submitter = fn; return p }

// Children builds extra items after the fields (FormList, custom items).
func (p *ProFormW) Children(fn func(f *FormW)) *ProFormW { p.children = fn; return p }

// Gutter sets the horizontal gap between grid columns (default 24).
func (p *ProFormW) Gutter(px float32) *ProFormW { p.gutter = px; return p }

// Submit validates (waiting for async rules) and calls OnFinish.
func (p *ProFormW) Submit() {
	if p.form != nil {
		p.form.Submit()
	}
}

// Reset restores the initial values and clears validation.
func (p *ProFormW) Reset() {
	if p.st != nil {
		p.st.reset = true
	}
	p.c.invalid = true
}

// Finished reports a completed submit (OnFinish called done(nil)), once.
func (p *ProFormW) Finished() bool { return p.finished }

// Submitting reports validation or OnFinish in progress.
func (p *ProFormW) Submitting() bool {
	return p.st != nil && (p.st.submitting || (p.form != nil && p.form.Submitting()))
}

// Loading reports the initial request in progress.
func (p *ProFormW) Loading() bool { return p.st != nil && p.st.loading }

// Err returns the last OnFinish error.
func (p *ProFormW) Err() error {
	if p.st == nil {
		return nil
	}
	return p.st.err
}

// Form returns the inner form of the last Show.
func (p *ProFormW) Form() *FormW { return p.form }

// ID returns the widget ID (valid after Show).
func (p *ProFormW) ID() ID { return p.id }

// valsRef returns the bound or own values.
func (p *ProFormW) valsRef() ProValues {
	if p.vals != nil {
		if *p.vals == nil {
			*p.vals = ProValues{}
		}
		return *p.vals
	}
	return p.st.vals
}

// Show builds the form.
func (p *ProFormW) Show() *ProFormW {
	c := p.c
	root := c.box("proform", layout.Column, nil)
	p.id = root.n.id
	st := State[proFormState](c, p.id, func() proFormState { return proFormState{vals: ProValues{}} })
	p.st = st
	vals := p.valsRef()
	p.poll(vals)
	if !st.requested {
		st.requested = true
		proInitial(p.fields, vals)
		st.init = vals.Clone()
		if p.request != nil {
			st.loading = true
			win := c.win
			st.mu.Lock()
			st.seq++
			seq := st.seq
			st.mu.Unlock()
			p.request(func(v ProValues, err error) {
				st.mu.Lock()
				if seq == st.seq {
					st.initRes = &proInitResult{v, err}
				}
				st.mu.Unlock()
				if win != nil {
					win.Invalidate()
				}
			})
			p.poll(vals)
		}
	}
	root.Body(func() {
		Loading(c, st.loading).Body(func() { p.content(vals) })
	})
	return p
}

// poll applies delivered asynchronous results.
func (p *ProFormW) poll(vals ProValues) {
	st := p.st
	st.mu.Lock()
	ir, sr := st.initRes, st.subRes
	st.initRes, st.subRes = nil, nil
	st.mu.Unlock()
	if ir != nil {
		st.loading, st.reqErr = false, ir.err
		for k, v := range ir.vals {
			vals[k] = v
		}
		proInitial(p.fields, vals)
		st.init = vals.Clone()
		p.c.invalid = true
	}
	if sr != nil {
		st.submitting, st.err = false, sr.err
		p.finished = sr.err == nil
		p.c.invalid = true
	}
}

func (p *ProFormW) content(vals ProValues) {
	c, th := p.c, p.c.Theme
	st := p.st
	loc := c.Locale()
	f := Form(c).ScrollToFirstError().LabelWidth(p.labelWidth)
	if !p.horizontal {
		f.Vertical()
	}
	p.form = f
	if st.reset {
		st.reset = false
		for k := range vals {
			delete(vals, k)
		}
		for k, v := range st.init.Clone() {
			vals[k] = v
		}
		f.ResetValidation()
		st.err = nil
	}
	f.Body(func() {
		p.fieldsGrid(f, vals)
		if p.children != nil {
			p.children(f)
		}
		if st.err != nil {
			Row(c, func() {
				Alert(c, "error", st.err.Error()).Show()
			}, layout.Margin(geom.Insets{Bottom: 16}))
		}
		if p.noSubmit {
			return
		}
		it := f.Item("")
		if p.horizontal {
			it.NoLabel()
		}
		it.Body(func() {
			Row(c, func() {
				if p.submitter != nil {
					p.submitter(&ProSubmitter{c: c, p: p})
					return
				}
				sub := p.submitText
				if sub == "" {
					sub = loc.Form.Submit
				}
				rst := p.resetText
				if rst == "" {
					rst = loc.Reset
				}
				if Button(c, sub).Primary().Loading(p.Submitting()).Clicked() {
					p.Submit()
				}
				if !p.noReset && Button(c, rst).Disabled(p.Submitting()).Clicked() {
					p.Reset()
				}
			}, layout.Gap(8), layout.Middle())
		})
	})
	if f.Finished() {
		if p.onFinish == nil {
			p.finished = true
		} else {
			st.submitting, st.err = true, nil
			win := c.win
			p.onFinish(vals.Clone(), func(err error) {
				st.mu.Lock()
				st.subRes = &proSubResult{err}
				st.mu.Unlock()
				if win != nil {
					win.Invalidate()
				}
			})
			p.poll(vals)
		}
		c.invalid = true
	}
	_ = th
}

// fieldsGrid lays the fields out in the grid; consecutive fields sharing
// a Group are wrapped in a ProFormGroup.
func (p *ProFormW) fieldsGrid(f *FormW, vals ProValues) {
	proGroupRuns(p.c, p.fields, func(fields []ProField) { p.fieldsRun(f, fields, vals) })
}

// proGroupRuns calls run for every run of consecutive fields with the same
// Group, inside a titled ProFormGroup when the Group is set.
func proGroupRuns(c *Context, fields []ProField, run func([]ProField)) {
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].Group == fields[i].Group {
			j++
		}
		part := fields[i:j]
		if g := part[0].Group; g != "" {
			Key(c, "group:"+g, func() { ProFormGroup(c, g).Body(func() { run(part) }) })
		} else {
			run(part)
		}
		i = j
	}
}

func (p *ProFormW) fieldsRun(f *FormW, fields []ProField, vals ProValues) {
	c := p.c
	if p.cols <= 1 {
		for i := range fields {
			fd := &fields[i]
			if fd.Span > 0 && fd.Span < 24 {
				Row(c, func() {
					Column(c, func() { proFieldItem(c, f, fd, vals) }, layout.Grow(float32(fd.Span)), layout.Basis(0), layout.MinW(0))
					Div(c, layout.Grow(float32(24-fd.Span)), layout.Basis(0))
				})
				continue
			}
			proFieldItem(c, f, fd, vals)
		}
		return
	}
	span := 24 / p.cols
	GridRow(c).Gutter(p.gutter, 0).Body(func() {
		for i := range fields {
			fd := &fields[i]
			if fd.Hide != nil && fd.Hide(vals) {
				f.Item(fd.Label).Name(fd.Key).Hidden(true).Body(nil)
				continue
			}
			s := span
			if fd.Span > 0 {
				s = fd.Span
			}
			Col(c, s).Body(func() { proFieldItem(c, f, fd, vals) })
		}
	})
}
