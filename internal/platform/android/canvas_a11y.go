package android

// Platform-neutral half of the Android canvas accessibility bridge
// (docs/specs/mobile-canvas.md §5.2; the JNI half is
// canvas_a11y_android.go, the Java half CanvasA11y.java). It keeps the
// last semantics tree as the immutable snapshot every query of the
// AccessibilityNodeProvider is answered from, and the virtual view ids of
// its nodes. Free of build tags so it is unit tested on every OS. Main
// thread only, except active.

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

// a11yMaxEvents is the most content-change events one update sends; more
// collapse into one subtree change of the canvas view.
const a11yMaxEvents = 12

type canvasA11y struct {
	// active: an assistive technology (or uiautomator) reads the canvas.
	// Any goroutine.
	active atomic.Bool

	tree  *a11y.Tree
	ids   a11ybridge.VirtualIDs
	focus a11y.NodeID // effective input focus reported by the core

	// selfTest is the name of the node the debug self-test clicks
	// (debug.tango.a11y.click), "" when off; selfDone once it ran.
	selfTest string
	selfDone bool
}

// setTree replaces the snapshot (nil after deactivation).
func (ax *canvasA11y) setTree(t *a11y.Tree, now time.Time) {
	ax.tree = t
	ax.ids.Sync(t, now)
	if t == nil {
		ax.focus = 0
	}
}

// update takes the next snapshot and returns the content-change events to
// send.
func (ax *canvasA11y) update(t *a11y.Tree, changed []a11y.NodeID, now time.Time) []a11ybridge.AndroidEvent {
	prev := ax.tree
	ax.tree = t
	if prev == nil {
		ax.ids.Sync(t, now)
		return []a11ybridge.AndroidEvent{{VID: a11ybridge.AndroidHostID, Changes: a11ybridge.AndroidChangeSubtree}}
	}
	ax.ids.Update(t, changed, now)
	return a11ybridge.AndroidEvents(prev, t, changed, &ax.ids, a11yMaxEvents)
}

// node returns the packed record of virtual view vid (nil: unknown).
func (ax *canvasA11y) node(vid int32) []byte {
	if ax.tree == nil {
		return nil
	}
	id, ok := ax.ids.Node(vid)
	if !ok {
		return nil
	}
	return a11ybridge.PackAndroidNode(ax.tree, id, &ax.ids)
}

// hit returns the virtual view at (x, y) device pixels in the canvas view.
func (ax *canvasA11y) hit(x, y, density float32) int32 {
	if ax.tree == nil {
		return a11ybridge.AndroidNoID
	}
	if !(density > 0) {
		density = 1
	}
	return a11ybridge.AndroidHit(ax.tree, &ax.ids, x/density, y/density)
}

// focusVID is the virtual view with input focus (AndroidNoID: none).
func (ax *canvasA11y) focusVID() int32 {
	if ax.tree == nil || ax.focus == 0 {
		return a11ybridge.AndroidNoID
	}
	if vid, ok := ax.ids.VID(ax.focus); ok {
		return vid
	}
	return a11ybridge.AndroidNoID
}

// vid is the virtual view of node id (AndroidNoID: none).
func (ax *canvasA11y) vid(id a11y.NodeID) int32 {
	if vid, ok := ax.ids.VID(id); ok && ax.tree != nil && ax.tree.Node(id) != nil {
		return vid
	}
	return a11ybridge.AndroidNoID
}

// action resolves an Android action on virtual view vid to the node and
// a11y action to perform.
func (ax *canvasA11y) action(vid int32, act a11ybridge.AndroidAction, text string, progress float64) (a11y.NodeID, a11y.Action, string, bool) {
	if ax.tree == nil {
		return 0, 0, "", false
	}
	id, ok := ax.ids.Node(vid)
	if !ok {
		return 0, 0, "", false
	}
	a, arg, ok := a11ybridge.AndroidPerform(ax.tree.Node(id), act, text, progress)
	return id, a, arg, ok
}

// selfTestTarget returns the virtual view the self-test should click now:
// the first visible node (reading order) whose name is selfTest, or
// AndroidNoID.
func (ax *canvasA11y) selfTestTarget() int32 {
	if ax.selfTest == "" || ax.selfDone || ax.tree == nil {
		return a11ybridge.AndroidNoID
	}
	for _, n := range ax.tree.Nodes() {
		if strings.TrimSpace(n.Name) == ax.selfTest && !n.States.Has(a11y.Offscreen) &&
			a11ybridge.AndroidActions(n).Has(a11ybridge.AndroidClick) {
			return ax.vid(n.ID)
		}
	}
	return a11ybridge.AndroidNoID
}
