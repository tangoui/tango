package host

// Accessibility of host windows (devhost/proto_a11y.go): the host window's
// canvas is the platform side of the bridge, the tree comes from the
// worker that draws the window. Package uihost provides the platform
// bridge (NewA11yBridge, its a11y.go); the host keeps one tree per worker
// of a window and shows the owner's. When the owner changes (hot reload)
// the platform's tree is replaced, or dropped until the new worker sends
// one, so focus never points into the old process's tree.
//
// Main thread unless noted.

import (
	"errors"
	"fmt"

	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
)

// A11yBridge is the platform accessibility bridge of one host window's
// canvas. All methods run on the main thread.
type A11yBridge interface {
	// Active reports whether an assistive technology uses the canvas. It
	// is polled at the start of every CanvasFrame and may itself update
	// the platform (drop its tree when the technology left).
	Active() bool
	// Update hands the platform a new tree; changed as for
	// ui.AccessibilityHost.
	Update(tree *a11y.Tree, changed []a11y.NodeID)
	// Replace gives the platform tree instead of the one it has (another
	// process drew it), dropping the old one with its focus first.
	Replace(tree *a11y.Tree)
	// Reset drops the platform's tree (no tree to show).
	Reset()
	// Close releases the bridge (the window closed).
	Close()
}

// NewA11yBridge creates the bridge of a host window's canvas; nil (or a nil
// result: the canvas cannot expose a tree) leaves the windows without one.
// Package uihost sets it.
var NewA11yBridge func(cv platform.Canvas) A11yBridge

// maxCarriedAnnouncements bounds announcements kept from trees that were
// replaced before the platform saw them.
const maxCarriedAnnouncements = 16

// winA11y is the accessibility state of a host window.
type winA11y struct {
	br    A11yBridge
	on    bool                   // the platform wants a tree (last Active)
	epoch int                    // of the last A11yState{On: true}
	trees map[*worker]*a11y.Tree // newest tree of each worker drawing the window
	from  *worker                // whose tree the platform has (nil: none)
	shown *a11y.Tree             // the tree the platform has
}

func (x *win) initA11y() {
	if NewA11yBridge != nil && x.cv != nil {
		x.ax.br = NewA11yBridge(x.cv)
	}
	x.ax.trees = map[*worker]*a11y.Tree{}
}

// pollA11y follows the platform's activation at the start of a frame and
// tells the window's workers when it changes.
func (h *Host) pollA11y(x *win) {
	if x.ax.br == nil {
		return
	}
	on := x.ax.br.Active()
	if on == x.ax.on {
		return
	}
	x.ax.on = on
	if on {
		x.ax.epoch++
	} else {
		// The bridge dropped the platform's tree itself.
		clear(x.ax.trees)
		x.ax.from, x.ax.shown = nil, nil
	}
	for _, w := range []*worker{x.owner, x.next} {
		h.sendA11yState(x, w)
	}
}

// a11yOpened tells a worker that just opened (or took over) window x that
// a tree is wanted, so it builds one with its first frame.
func (h *Host) a11yOpened(x *win, w *worker) {
	if x.ax.on {
		h.sendA11yState(x, w)
	}
}

// sendA11yState tells w whether window x wants a tree.
func (h *Host) sendA11yState(x *win, w *worker) {
	if x.ax.br == nil || w == nil || w.gone {
		return
	}
	w.send(devhost.A11yState{Win: x.id, On: x.ax.on, Epoch: x.ax.epoch})
}

// a11yTree stores a worker's tree for its window.
func (h *Host) a11yTree(w *worker, m devhost.A11yTree) {
	x := h.wins[m.Win]
	if x == nil || x.closed || !x.ax.on || m.Epoch != x.ax.epoch || (x.owner != w && x.next != w) {
		return // the window, the request or the worker is gone
	}
	prev := x.ax.trees[w]
	t, err := m.Apply(prev)
	if err != nil {
		// Out of step (cannot happen on an ordered stream): ask again.
		trace("window %d: %v; asking for a full tree", x.id, err)
		x.ax.epoch++
		delete(x.ax.trees, w)
		for _, ww := range []*worker{x.owner, x.next} {
			h.sendA11yState(x, ww)
		}
		return
	}
	if prev != nil && prev != x.ax.shown && len(prev.Announcements) > 0 {
		// The platform never saw prev: keep its announcements.
		ann := append(append([]a11y.Announcement(nil), prev.Announcements...), t.Announcements...)
		if len(ann) > maxCarriedAnnouncements {
			ann = ann[len(ann)-maxCarriedAnnouncements:]
		}
		t.Announcements = ann
	}
	x.ax.trees[w] = t
	if x.owner == w {
		x.cv.Invalidate() // flushA11y in CanvasFrame
	}
}

// flushA11y brings the platform's tree up to date with the window's owner
// (in CanvasFrame and before presenting a frame).
func (h *Host) flushA11y(x *win) {
	br := x.ax.br
	if br == nil || !x.ax.on || x.closed {
		return
	}
	w := x.owner
	var t *a11y.Tree
	if w != nil && !w.gone {
		t = x.ax.trees[w]
	}
	switch {
	case t == nil:
		if x.ax.from != nil {
			br.Reset()
			x.ax.from, x.ax.shown = nil, nil
		}
	case x.ax.from != w:
		br.Replace(t)
		x.ax.from, x.ax.shown = w, t
	case t != x.ax.shown:
		br.Update(t, a11y.Diff(x.ax.shown, t))
		x.ax.shown = t
	}
}

// a11yWorkerGone forgets the trees of a worker that disconnected.
func (h *Host) a11yWorkerGone(w *worker) {
	for _, x := range h.wins {
		delete(x.ax.trees, w)
		if x.ax.from == w {
			h.flushA11y(x) // owner gone: drop its tree now
		}
	}
}

func (x *win) closeA11y() {
	if x.ax.br != nil {
		x.ax.br.Close()
	}
	x.ax = winA11y{}
}

// ---- platform.CanvasAccessibleSink ----

var _ platform.CanvasAccessibleSink = (*win)(nil)

var errA11yNoWorker = errors.New("devhost: no app draws this window")

// AccessibilityAction forwards an assistive technology's action to the
// worker whose tree the platform shows; the worker's runner performs it
// in its next frame.
func (x *win) AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error {
	w := x.owner
	if x.closed || w == nil || w.gone || x.ax.from != w || x.ax.shown == nil {
		return errA11yNoWorker
	}
	n := x.ax.shown.Node(id)
	if n == nil {
		return fmt.Errorf("devhost: no accessibility node %d", id)
	}
	if !n.Actions.Has(act) {
		return fmt.Errorf("devhost: %s does not support %s", n, act)
	}
	x.forward(devhost.A11yAction{Win: x.id, ID: id, Act: act, Arg: arg})
	return nil
}
