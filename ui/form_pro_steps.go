package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// StepsFormStep is one step of a StepsForm.
type StepsFormStep struct {
	Title, Description string
	Fields             []ProField
	Columns            int
	// Children builds extra items after the fields (FormList ...).
	Children func(f *FormW)
}

// StepsFormW splits a form into steps (Ant Design Pro StepsForm): the
// current step's fields validate before moving on; the last step submits
// the values of all steps.
type StepsFormW struct {
	_ [0]func()

	c        *Context
	steps    []StepsFormStep
	vals     *ProValues
	onFinish func(vals ProValues, done func(error))
	width    float32
	finished bool
	st       *stepsFormState
}

type stepsFormState struct {
	cur  int
	vals ProValues
}

// StepsForm starts a step form; chain options, then Show.
func StepsForm(c *Context, steps []StepsFormStep) *StepsFormW {
	// ConfigProvider.Defaults("StepsForm", ...) keys: "width" (number).
	s := &StepsFormW{c: c, steps: steps, width: 600}
	if v, ok := p2bNum(c, "StepsForm", "width"); ok && v > 0 {
		s.width = float32(v)
	}
	return s
}

// Values binds the values of all steps.
func (s *StepsFormW) Values(v *ProValues) *StepsFormW { s.vals = v; return s }

// OnFinish receives the values after the last step validated; call done
// (from any goroutine) when saved.
func (s *StepsFormW) OnFinish(fn func(vals ProValues, done func(error))) *StepsFormW {
	s.onFinish = fn
	return s
}

// Width sets the form width under the steps (default 600).
func (s *StepsFormW) Width(px float32) *StepsFormW { s.width = px; return s }

// Current returns the 0-based current step (valid after Show).
func (s *StepsFormW) Current() int {
	if s.st == nil {
		return 0
	}
	return s.st.cur
}

// SetCurrent jumps to step i (no validation).
func (s *StepsFormW) SetCurrent(i int) *StepsFormW {
	if s.st != nil {
		s.st.cur = max(0, min(i, len(s.steps)-1))
		s.c.invalid = true
	}
	return s
}

// Finished reports a completed submit of the last step, once.
func (s *StepsFormW) Finished() bool { return s.finished }

// Show builds the steps and the current step's form.
func (s *StepsFormW) Show() *StepsFormW {
	c := s.c
	loc := c.Locale()
	root := c.box("steps-form", layout.Column, nil)
	st := State[stepsFormState](c, root.n.id, func() stepsFormState { return stepsFormState{vals: ProValues{}} })
	s.st = st
	if s.vals == nil {
		s.vals = &st.vals
	}
	if len(s.steps) == 0 {
		return s
	}
	st.cur = max(0, min(st.cur, len(s.steps)-1))
	root.Body(func() {
		items := make([]StepItem, len(s.steps))
		for i, sp := range s.steps {
			items[i] = StepItem{Title: sp.Title, Description: sp.Description}
		}
		Row(c, func() {
			Steps(c, items).Current(st.cur).Show()
		}, layout.MaxW(max(s.width, 160*float32(len(s.steps)))), layout.Self(layout.AlignCenter), layout.Margin(geom.Insets{Bottom: 32}))
		cur := st.cur
		last := cur == len(s.steps)-1
		sp := s.steps[cur]
		Column(c, func() {
			Key(c, cur, func() {
				pf := ProForm(c, sp.Fields).Values(s.vals).Columns(max(1, sp.Columns))
				if sp.Children != nil {
					pf.Children(sp.Children)
				}
				pf.OnFinish(func(v ProValues, done func(error)) {
					if last && s.onFinish != nil {
						s.onFinish(v, done)
						return
					}
					done(nil)
				})
				pf.Submitter(func(sb *ProSubmitter) {
					if cur > 0 && Button(c, loc.Form.PrevStep).Disabled(sb.Loading()).Clicked() {
						st.cur--
						c.invalid = true
					}
					next := loc.Form.NextStep
					if last {
						next = loc.Form.Submit
					}
					if Button(c, next).Primary().Loading(sb.Loading()).Clicked() {
						sb.Submit()
					}
				})
				pf.Show()
				if pf.Finished() {
					if last {
						s.finished = true
					} else {
						st.cur++
					}
					c.invalid = true
				}
			})
		}, layout.MaxW(s.width), layout.Self(layout.AlignCenter))
	})
	return s
}
