package fake

// platform.CanvasAccessible on fake canvases (docs/specs/a11y-bridge.md)
// and platform.MotionPreference / ContrastPreference on the backend. Tests
// play the assistive technology: SetA11yActive switches it on or off, the
// A11y* accessors read what the core sent, and A11yAction performs an
// action through the sink as a backend would.

import (
	"errors"
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
)

var (
	_ platform.CanvasAccessible     = (*Canvas)(nil)
	_ platform.MotionPreference     = (*Backend)(nil)
	_ platform.ContrastPreference   = (*Backend)(nil)
	_ platform.CanvasAccessibleSink = attachedSink{}
)

// A11yCall is one CanvasAccessible call recorded by a fake canvas.
type A11yCall struct {
	Op      string // "set", "update", "focus", "announce"
	Tree    *a11y.Tree
	Changed []a11y.NodeID
	Focus   a11y.NodeID
	Text    string
	Live    a11y.Live
	Source  a11y.NodeID
	Main    bool // called on the main thread
}

type a11yState struct {
	mu     sync.Mutex
	active bool
	tree   *a11y.Tree
	focus  a11y.NodeID
	calls  []A11yCall
}

var a11yStates sync.Map // *Canvas -> *a11yState

func (c *Canvas) a11y() *a11yState {
	if s, ok := a11yStates.Load(c); ok {
		return s.(*a11yState)
	}
	s, _ := a11yStates.LoadOrStore(c, &a11yState{})
	return s.(*a11yState)
}

func (c *Canvas) record(call A11yCall) {
	call.Main = c.b.IsMainThread()
	s := c.a11y()
	s.mu.Lock()
	s.calls = append(s.calls, call)
	s.mu.Unlock()
}

// ---- platform.CanvasAccessible ----

func (c *Canvas) AccessibilityActive() bool {
	s := c.a11y()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (c *Canvas) SetAccessibilityTree(tree *a11y.Tree) {
	s := c.a11y()
	s.mu.Lock()
	s.tree = tree
	if tree == nil {
		s.focus = 0
	}
	s.mu.Unlock()
	c.record(A11yCall{Op: "set", Tree: tree})
}

func (c *Canvas) UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID) {
	s := c.a11y()
	s.mu.Lock()
	s.tree = tree
	s.mu.Unlock()
	c.record(A11yCall{Op: "update", Tree: tree, Changed: slices.Clone(changed)})
}

func (c *Canvas) AccessibilityFocusChanged(id a11y.NodeID) {
	s := c.a11y()
	s.mu.Lock()
	s.focus = id
	s.mu.Unlock()
	c.record(A11yCall{Op: "focus", Focus: id})
}

func (c *Canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	c.record(A11yCall{Op: "announce", Text: text, Live: live, Source: source})
}

// ---- simulation ----

// SetA11yActive simulates an assistive technology attaching (true) or
// leaving (false): AccessibilityActive changes and a frame is requested.
func (c *Canvas) SetA11yActive(on bool) {
	s := c.a11y()
	s.mu.Lock()
	s.active = on
	s.mu.Unlock()
	c.Invalidate()
}

// A11yTree returns the last tree the core sent (nil before activation or
// after SetAccessibilityTree(nil)).
func (c *Canvas) A11yTree() *a11y.Tree {
	s := c.a11y()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tree
}

// A11yFocus returns the node of the last AccessibilityFocusChanged.
func (c *Canvas) A11yFocus() a11y.NodeID {
	s := c.a11y()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focus
}

// A11yCalls returns the recorded CanvasAccessible calls in order.
func (c *Canvas) A11yCalls() []A11yCall {
	s := c.a11y()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// A11yAnnouncements returns the texts of the AccessibilityAnnounce calls.
func (c *Canvas) A11yAnnouncements() []string {
	var out []string
	for _, call := range c.A11yCalls() {
		if call.Op == "announce" {
			out = append(out, call.Text)
		}
	}
	return out
}

// ResetA11yCalls forgets the recorded calls.
func (c *Canvas) ResetA11yCalls() {
	s := c.a11y()
	s.mu.Lock()
	s.calls = nil
	s.mu.Unlock()
}

// A11yAction performs act on node id like a backend whose assistive
// technology asked for it: on the main thread, through the sink's
// CanvasAccessibleSink. It fails when the sink does not implement it.
func (c *Canvas) A11yAction(id a11y.NodeID, act a11y.Action, arg string) error {
	as, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return errors.New("fake: sink does not implement CanvasAccessibleSink")
	}
	var err error
	if !c.b.Do(func() {
		if c.s.Closed() {
			err = errors.New("fake: surface closed")
			return
		}
		err = as.AccessibilityAction(id, act, arg)
	}) {
		return errors.New("fake: loop stopped")
	}
	return err
}

// AccessibilityAction forwards to the wrapped sink until Detach.
func (s attachedSink) AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error {
	if s.c.Detached() {
		return errors.New("fake: canvas detached")
	}
	as, ok := s.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return errors.New("fake: sink does not implement CanvasAccessibleSink")
	}
	return as.AccessibilityAction(id, act, arg)
}

// ---- platform.MotionPreference ----

var motionPrefs sync.Map // *Backend -> bool

// ReduceMotion implements platform.MotionPreference.
func (b *Backend) ReduceMotion() bool {
	v, _ := motionPrefs.Load(b)
	on, _ := v.(bool)
	return on
}

// SetReduceMotion changes the simulated system preference and, once the
// app runs and the value changed, emits AppReduceMotionChanged.
func (b *Backend) SetReduceMotion(on bool) bool {
	old := b.ReduceMotion()
	motionPrefs.Store(b, on)
	if old == on || !b.running() {
		return false
	}
	return b.AppEvent(platform.AppEvent{Kind: platform.AppReduceMotionChanged})
}

// ---- platform.ContrastPreference ----

var contrastPrefs sync.Map // *Backend -> bool

// HighContrast implements platform.ContrastPreference.
func (b *Backend) HighContrast() bool {
	v, _ := contrastPrefs.Load(b)
	on, _ := v.(bool)
	return on
}

// SetHighContrast changes the simulated system preference and, once the
// app runs and the value changed, emits AppContrastChanged.
func (b *Backend) SetHighContrast(on bool) bool {
	old := b.HighContrast()
	contrastPrefs.Store(b, on)
	if old == on || !b.running() {
		return false
	}
	return b.AppEvent(platform.AppEvent{Kind: platform.AppContrastChanged})
}
