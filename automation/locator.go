package automation

import (
	"slices"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/ui/a11y"
)

// Locator finds a node of a self-drawn window's semantics tree. It is a
// description, not a node: every method looks the node up again in the
// current tree, waiting up to the timeout for it to appear (Exists and
// Count do not wait), so it survives re-renders and works before the node
// exists. Methods returning *Locator return a refined copy.
type Locator struct {
	w       *Window
	q       Query
	timeout time.Duration // 0: the app's
}

func (l *Locator) with(fn func(q *Query)) *Locator {
	c := *l
	c.q.States = slices.Clone(l.q.States)
	fn(&c.q)
	return &c
}

// Nth picks the i-th match in reading order (0 the first; negative counts
// from the end, -1 the last).
func (l *Locator) Nth(i int) *Locator { return l.with(func(q *Query) { q.Nth = i }) }

// First is Nth(0); Last is Nth(-1).
func (l *Locator) First() *Locator { return l.Nth(0) }

// Last picks the last match.
func (l *Locator) Last() *Locator { return l.Nth(-1) }

// WithState keeps nodes that have every state of s (a11y.Checked|a11y.Focused).
func (l *Locator) WithState(s a11y.State) *Locator {
	return l.with(func(q *Query) { q.States = append(q.States, stateNames(s, "")...) })
}

// WithoutState keeps nodes that have none of the states of s.
func (l *Locator) WithoutState(s a11y.State) *Locator {
	return l.with(func(q *Query) { q.States = append(q.States, stateNames(s, "!")...) })
}

// WithValue keeps nodes whose Value is v.
func (l *Locator) WithValue(v string) *Locator { return l.with(func(q *Query) { q.Value = &v }) }

// Timeout sets how long this locator waits.
func (l *Locator) Timeout(d time.Duration) *Locator {
	c := *l
	c.timeout = d
	return &c
}

// Query returns the locator's query.
func (l *Locator) Query() Query { return l.q }

func (l *Locator) String() string { return l.w.String() + ": " + l.q.String() }

func (l *Locator) wait() time.Duration {
	if l.timeout > 0 {
		return l.timeout
	}
	return l.w.app.timeout
}

func (l *Locator) req(op string, timeout time.Duration) *wire.Request {
	q := l.q
	r := l.w.req(op)
	r.Query = &q
	r.Timeout = timeout.Milliseconds()
	return r
}

// Node waits for the node and returns a copy of it.
func (l *Locator) Node() a11y.Node {
	l.w.app.t.Helper()
	return *l.w.app.call(l.req(wire.OpFind, l.wait()), l.String()).Node
}

// Wait is Node: it waits until the node exists.
func (l *Locator) Wait() a11y.Node { l.w.app.t.Helper(); return l.Node() }

// WaitGone waits until no node matches.
func (l *Locator) WaitGone() {
	l.w.app.t.Helper()
	r := l.req(wire.OpWait, l.wait())
	r.Gone = true
	l.w.app.call(r, l.String())
}

// WaitState waits until the node exists and has every state of s.
func (l *Locator) WaitState(s a11y.State) a11y.Node {
	l.w.app.t.Helper()
	return l.WithState(s).Node()
}

// WaitValue waits until the node exists and its Value is v.
func (l *Locator) WaitValue(v string) a11y.Node {
	l.w.app.t.Helper()
	return l.WithValue(v).Node()
}

// Exists reports whether the node exists now (no waiting beyond the first
// tree of the window).
func (l *Locator) Exists() bool {
	l.w.app.t.Helper()
	_, err := l.w.app.try(l.req(wire.OpFind, 0))
	if err == nil {
		return true
	}
	if e, ok := err.(*Error); ok && e.Code == wire.CodeTimeout {
		return false
	}
	l.w.app.t.Fatalf("automation: %s: %v", l, err)
	return false
}

// Count returns how many nodes match now (Nth ignored).
func (l *Locator) Count() int {
	l.w.app.t.Helper()
	return l.w.app.call(l.req(wire.OpCount, 0), l.String()).Count
}

// All returns every node matching now (Nth ignored).
func (l *Locator) All() []a11y.Node {
	l.w.app.t.Helper()
	return l.w.app.call(l.req(wire.OpCount, 0), l.String()).Nodes
}

// Bounds waits for the node and returns its rectangle (logical pixels from
// the top-left of the window's content).
func (l *Locator) Bounds() a11y.Rect { l.w.app.t.Helper(); return l.Node().Bounds }

// Name waits for the node and returns its accessible name.
func (l *Locator) Name() string { l.w.app.t.Helper(); return l.Node().Name }

// Value waits for the node and returns its value.
func (l *Locator) Value() string { l.w.app.t.Helper(); return l.Node().Value }

// Has waits for the node and reports whether it has every state of s.
func (l *Locator) Has(s a11y.State) bool { l.w.app.t.Helper(); return l.Node().States.Has(s) }

// Perform waits for the node and asks it to perform act (with arg for
// a11y.SetValue), as an assistive technology would; it returns once the
// frame that handled it has run.
func (l *Locator) Perform(act a11y.Action, arg string) a11y.Node {
	l.w.app.t.Helper()
	r := l.req(wire.OpAction, l.wait())
	r.Action = act.String()
	r.Arg = arg
	return *l.w.app.call(r, l.String()+": "+act.String()).Node
}

// Click activates the node: the a11y Press action when it supports it,
// otherwise a pointer click at the center of its bounds.
func (l *Locator) Click() {
	l.w.app.t.Helper()
	n := l.Node()
	if n.Actions.Has(a11y.Press) {
		l.Perform(a11y.Press, "")
		return
	}
	l.clickAt(n)
}

// ClickPointer clicks the center of the node's bounds with the pointer
// (real hit testing, hover and press states) instead of the a11y action.
func (l *Locator) ClickPointer() {
	l.w.app.t.Helper()
	l.clickAt(l.Node())
}

func (l *Locator) clickAt(n a11y.Node) {
	l.w.app.t.Helper()
	b := n.Bounds
	l.w.Click(b.X+b.W/2, b.Y+b.H/2)
}

// Focus moves the keyboard focus to the node.
func (l *Locator) Focus() { l.w.app.t.Helper(); l.Perform(a11y.Focus, "") }

// SetValue replaces the node's value (text fields, number inputs,
// sliders) with the a11y SetValue action.
func (l *Locator) SetValue(v string) { l.w.app.t.Helper(); l.Perform(a11y.SetValue, v) }

// Type focuses the node and sends text as typed (committed) input.
func (l *Locator) Type(text string) {
	l.w.app.t.Helper()
	n := l.Node()
	if n.Actions.Has(a11y.Focus) {
		l.Perform(a11y.Focus, "")
	} else {
		l.clickAt(n)
	}
	l.w.Type(text)
}

// Expand opens the node (combobox, tree item, collapse panel).
func (l *Locator) Expand() { l.w.app.t.Helper(); l.Perform(a11y.Expand, "") }

// Collapse closes it.
func (l *Locator) Collapse() { l.w.app.t.Helper(); l.Perform(a11y.Collapse, "") }

// Increment steps a slider or spin button up.
func (l *Locator) Increment() { l.w.app.t.Helper(); l.Perform(a11y.Increment, "") }

// Decrement steps it down.
func (l *Locator) Decrement() { l.w.app.t.Helper(); l.Perform(a11y.Decrement, "") }

// Dismiss closes a dialog, popup or message.
func (l *Locator) Dismiss() { l.w.app.t.Helper(); l.Perform(a11y.Dismiss, "") }

// ScrollIntoView scrolls the node's ancestors so it is visible.
func (l *Locator) ScrollIntoView() { l.w.app.t.Helper(); l.Perform(a11y.ScrollIntoView, "") }

func stateNames(s a11y.State, prefix string) []string {
	var out []string
	for i := 0; i < 32; i++ {
		if st := a11y.State(1) << i; s&st != 0 {
			out = append(out, prefix+st.String())
		}
	}
	return out
}
