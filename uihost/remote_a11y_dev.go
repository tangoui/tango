//go:build tango_dev

package uihost

// Accessibility under hot reload (devhost/proto_a11y.go,
// docs/specs/a11y-bridge.md). In a worker, remoteWin is the runner's
// ui.AccessibilityHost: it builds the tree while the host asks for it and
// sends a full tree, then deltas, to the host; actions come back and go to
// Runner.PerformAction. In the host, each window's canvas gets the same
// platform bridge as an Open window (a11y.go, without a runner).

import (
	"log"

	"github.com/tangoui/tango/internal/devhost"
	devhostmain "github.com/tangoui/tango/internal/devhost/host"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

// Set before any init runs: remote_dev.go's init runs the host.
var _ = func() bool {
	devhostmain.NewA11yBridge = newDevHostA11y
	return true
}()

// ---- host side ----

// devHostA11y is the platform bridge of a host window: a11yBridge without
// a runner (the tree comes from the worker; force and tap are unused).
type devHostA11y struct{ b *a11yBridge }

func newDevHostA11y(cv platform.Canvas) devhostmain.A11yBridge {
	ca, ok := cv.(platform.CanvasAccessible)
	if !ok {
		return nil
	}
	return devHostA11y{&a11yBridge{ca: ca}}
}

func (d devHostA11y) Active() bool                                  { return d.b.enabled() }
func (d devHostA11y) Update(tree *a11y.Tree, changed []a11y.NodeID) { d.b.update(tree, changed) }
func (d devHostA11y) Replace(tree *a11y.Tree)                       { d.b.replace(tree) }
func (d devHostA11y) Reset()                                        { d.b.reset() }
func (d devHostA11y) Close()                                        { d.b.close() }

// ---- worker side ----

// remoteA11y is a remote window's accessibility state. Main thread.
type remoteA11y struct {
	on    bool // the host wants a tree
	epoch int  // of the host's request
	sent  bool // the host has a full tree of this epoch
}

var _ ui.AccessibilityHost = (*remoteWin)(nil)

func (w *remoteWin) AccessibilityEnabled() bool {
	if w.closed || !w.ax.on {
		w.ax.sent = false // the runner drops its tree: start over with a full one
		return false
	}
	return true
}

func (w *remoteWin) AccessibilityUpdate(tree *a11y.Tree, changed []a11y.NodeID) {
	if w.closed || !w.ax.on || tree == nil {
		return
	}
	full := !w.ax.sent
	w.ax.sent = true
	_ = w.cl.Send(devhost.NewA11yTree(w.id, w.ax.epoch, tree, changed, full))
}

// a11yState follows the host's request. Main thread.
func (w *remoteWin) a11yState(m devhost.A11yState) {
	if w.closed {
		return
	}
	if !m.On {
		w.ax = remoteA11y{}
		w.r.Invalidate() // the runner drops its tree in the next frame
		return
	}
	if w.ax.on && w.ax.epoch == m.Epoch {
		return
	}
	w.ax = remoteA11y{on: true, epoch: m.Epoch}
	// A tree the runner already has (EnableAccessibility) goes out now: the
	// runner only reports changes.
	if t := w.r.AccessibilityTree(); t != nil {
		w.AccessibilityUpdate(t, nil)
	}
	w.r.Invalidate()
}

// a11yAction performs an assistive technology's action. Main thread.
func (w *remoteWin) a11yAction(m devhost.A11yAction) {
	if w.closed {
		return
	}
	if err := w.r.PerformAction(m.ID, m.Act, m.Arg); err != nil && devTraceOn {
		log.Printf("tango: hot reload: accessibility action: %v", err)
	}
}
