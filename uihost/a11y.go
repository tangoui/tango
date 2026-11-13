package uihost

// Accessibility bridge (docs/specs/a11y-bridge.md): the window of a canvas
// is a ui.AccessibilityHost; the semantics tree it receives fans out to
// the platform bridge (platform.CanvasAccessible, while an assistive
// technology is active) and to internal subscribers (tap: automation).
// force keeps the tree built without an assistive technology. The host is
// the platform.CanvasAccessibleSink that hands actions to the runner.
//
// Everything runs on the main thread, which is the UI thread.

import (
	"errors"
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

// a11yTreeFunc receives the tree of every frame that changed it (changed
// as for ui.AccessibilityHost), and (nil, nil) when the tree is dropped
// because nobody needs it any more. It must not modify the tree.
type a11yTreeFunc func(tree *a11y.Tree, changed []a11y.NodeID)

type a11yTapEntry struct {
	fn   a11yTreeFunc
	dead bool
}

// a11yBridge is the accessibility state of one canvas window. Main thread.
type a11yBridge struct {
	app *tango.App
	r   *ui.Runner
	ca  platform.CanvasAccessible // nil: the backend cannot expose a tree

	forced int             // force() references
	taps   []*a11yTapEntry // subscribers, in subscription order
	on     bool            // the last AccessibilityEnabled answer
	tree   *a11y.Tree      // last tree received (nil while off)

	// Platform side.
	platOn bool        // the platform reported an assistive technology
	sent   bool        // the platform has a tree
	focus  a11y.NodeID // effective focus last reported to the platform

	stopMotion func()
	closed     bool
}

var a11yBridges struct {
	mu sync.Mutex
	m  map[*ui.Runner]*a11yBridge
}

// newA11y creates the bridge of a canvas window and starts following the
// system "reduce motion" preference (motion.go). Main thread.
func newA11y(app *tango.App, cv platform.Canvas, r *ui.Runner, ropts ui.RunnerOptions) *a11yBridge {
	b := &a11yBridge{app: app, r: r}
	b.ca, _ = cv.(platform.CanvasAccessible)
	a11yBridges.mu.Lock()
	if a11yBridges.m == nil {
		a11yBridges.m = map[*ui.Runner]*a11yBridge{}
	}
	a11yBridges.m[r] = b
	a11yBridges.mu.Unlock()
	b.stopMotion = followMotion(app, r, ropts)
	return b
}

// a11yFor returns the bridge of a runner created by Open or AttachWith
// (nil for others, e.g. under tan dev).
func a11yFor(r *ui.Runner) *a11yBridge {
	a11yBridges.mu.Lock()
	defer a11yBridges.mu.Unlock()
	return a11yBridges.m[r]
}

// tap subscribes fn to tree updates and returns a function that cancels
// the subscription (idempotent, callable from fn). When a tree exists, fn
// is first called synchronously with it and every node as changed, so a
// late subscriber has a complete baseline. Subscribing does not turn the
// tree on: use force for that.
func (b *a11yBridge) tap(fn a11yTreeFunc) (cancel func()) {
	if b == nil || fn == nil {
		return func() {}
	}
	e := &a11yTapEntry{fn: fn}
	b.taps = append(b.taps, e)
	if b.tree != nil {
		b.call(e, b.tree, allIDs(b.tree))
	}
	return func() { e.dead = true }
}

// force keeps the semantics tree built while no assistive technology is
// active (automation). It requests a frame; the returned function drops
// the reference (idempotent).
func (b *a11yBridge) force() (release func()) {
	if b == nil {
		return func() {}
	}
	b.forced++
	b.r.Invalidate()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.forced--
			b.r.Invalidate()
		})
	}
}

// enabled answers ui.AccessibilityHost.AccessibilityEnabled at the start
// of a frame (inside CanvasFrame).
func (b *a11yBridge) enabled() bool {
	if b.closed {
		return false
	}
	plat := b.ca != nil && b.ca.AccessibilityActive()
	switch {
	case plat && !b.platOn && b.tree != nil:
		// The platform attached while the tree was already built (force):
		// the runner reports only changes, so hand over the current tree.
		b.platOn = true
		b.sendFull(b.tree)
	case !plat && b.platOn:
		b.platOn = false
		if b.sent {
			b.ca.SetAccessibilityTree(nil)
		}
		b.sent, b.focus = false, 0
	}
	b.platOn = plat
	on := plat || b.forced > 0
	if !on && b.on && b.tree != nil {
		b.tree = nil
		b.fanOut(nil, nil)
	}
	b.on = on
	return on
}

// update receives ui.AccessibilityHost.AccessibilityUpdate.
func (b *a11yBridge) update(tree *a11y.Tree, changed []a11y.NodeID) {
	if b.closed {
		return
	}
	prev := b.tree
	b.tree = tree
	if b.platOn && b.ca != nil {
		if !b.sent {
			b.sendFull(tree)
		} else {
			b.ca.UpdateAccessibilityNodes(tree, changed)
			b.notify(tree)
			for _, lc := range a11ybridge.LiveChanges(prev, tree, changed) {
				b.ca.AccessibilityAnnounce(lc.Text, lc.Live, lc.Root)
			}
		}
	}
	b.fanOut(tree, changed)
}

// sendFull gives the platform the whole tree, then focus and this frame's
// announcements (live regions are not announced for a first tree).
func (b *a11yBridge) sendFull(tree *a11y.Tree) {
	b.ca.SetAccessibilityTree(tree)
	b.sent, b.focus = true, 0
	b.notify(tree)
}

func (b *a11yBridge) notify(tree *a11y.Tree) {
	if f := a11ybridge.EffectiveFocus(tree); f != b.focus {
		b.focus = f
		b.ca.AccessibilityFocusChanged(f)
	}
	for _, a := range tree.Announcements {
		b.ca.AccessibilityAnnounce(a.Text, a.Live, 0)
	}
}

func (b *a11yBridge) fanOut(tree *a11y.Tree, changed []a11y.NodeID) {
	// Subscribers may tap or cancel from inside their callback: iterate a
	// copy, then drop cancelled entries from the current list.
	for _, e := range slices.Clone(b.taps) {
		if !e.dead {
			b.call(e, tree, changed)
		}
	}
	b.taps = slices.DeleteFunc(b.taps, func(e *a11yTapEntry) bool { return e.dead })
}

// call runs one subscriber; a panic goes to the app's panic handling and
// does not stop the others.
func (b *a11yBridge) call(e *a11yTapEntry, tree *a11y.Tree, changed []a11y.NodeID) {
	run := func() { e.fn(tree, changed) }
	if b.app != nil && corehook.Guard != nil {
		corehook.Guard(b.app, "ui accessibility subscriber", run)
		return
	}
	run()
}

// close runs when the window closes or the canvas is detached.
func (b *a11yBridge) close() {
	if b == nil || b.closed {
		return
	}
	b.closed = true
	a11yBridges.mu.Lock()
	delete(a11yBridges.m, b.r)
	a11yBridges.mu.Unlock()
	if b.stopMotion != nil {
		b.stopMotion()
	}
	if b.tree != nil {
		b.tree = nil
		b.fanOut(nil, nil)
	}
	b.taps = nil
}

// reset drops the tree, the platform's included (the dev host's window
// lost the process that drew it, remote_a11y_dev.go).
func (b *a11yBridge) reset() {
	if b.closed {
		return
	}
	if b.sent && b.ca != nil {
		b.ca.SetAccessibilityTree(nil)
	}
	b.sent, b.focus = false, 0
	if b.tree != nil {
		b.tree = nil
		b.fanOut(nil, nil)
	}
}

// replace swaps in a tree that does not continue the current one (another
// process drew it): the old tree and its focus go first, so nothing refers
// to its nodes, then tree arrives as a first tree.
func (b *a11yBridge) replace(tree *a11y.Tree) {
	b.reset()
	if !b.closed && tree != nil {
		b.update(tree, allIDs(tree))
	}
}

func allIDs(t *a11y.Tree) []a11y.NodeID {
	nodes := t.Nodes()
	ids := make([]a11y.NodeID, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// ---- ui.AccessibilityHost on window ----

var _ ui.AccessibilityHost = (*window)(nil)

func (w *window) AccessibilityEnabled() bool { return w.ax != nil && w.ax.enabled() }

func (w *window) AccessibilityUpdate(tree *a11y.Tree, changed []a11y.NodeID) {
	if w.ax != nil {
		w.ax.update(tree, changed)
	}
}

// ---- platform.CanvasAccessibleSink on host ----

var _ platform.CanvasAccessibleSink = (*host)(nil)

var errA11yClosed = errors.New("uihost: window closed")

// AccessibilityAction hands an assistive technology's request to the
// runner; it runs at the start of the next frame.
func (h *host) AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error {
	if h.closed || h.r == nil {
		return errA11yClosed
	}
	return h.r.PerformAction(id, act, arg)
}
