// Package a11ybridge holds the platform-neutral parts of the accessibility
// bridges of the desktop backends (docs/specs/a11y-bridge.md): role and
// state translation from ui/a11y to NSAccessibility, UI Automation and
// AT-SPI2, focus and live-region computation, hit testing and coordinate
// conversion. Everything here is pure Go and testable on any OS; the
// backends (darwin/canvas_a11y.go, windows/canvas_a11y_windows.go,
// linux/canvas_a11y_linux.go) and package uihost use it.
package a11ybridge

import (
	"math"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
)

// ---------------------------------------------------------------------------
// Focus and live regions

// EffectiveFocus returns the node assistive technologies should treat as
// focused: tree.Focus, or its ActiveDescendant when that node exists
// (combobox, tree, grid). 0 when nothing has focus.
func EffectiveFocus(t *a11y.Tree) a11y.NodeID {
	if t == nil || t.Focus == 0 {
		return 0
	}
	n := t.Node(t.Focus)
	if n == nil {
		return 0
	}
	if n.ActiveDescendant != 0 && t.Node(n.ActiveDescendant) != nil {
		return n.ActiveDescendant
	}
	return n.ID
}

// LiveChange is the new content of a live region.
type LiveChange struct {
	Root a11y.NodeID // the live region (node with Live set)
	Text string      // its text (tree.Text)
	Live a11y.Live
}

// LiveRoot returns the nearest node at or above id with Live set, or nil.
func LiveRoot(t *a11y.Tree, id a11y.NodeID) *a11y.Node {
	for n := t.Node(id); n != nil; n = t.Node(n.Parent) {
		if n.Live != a11y.LiveOff {
			return n
		}
		if n.Parent == 0 {
			break
		}
	}
	return nil
}

// LiveChanges returns the live regions of next whose content changed
// since prev: a changed node (added or modified, removed ones are looked up
// in prev) inside a live region reports that region once, unless its text
// is the same as in prev. A nil prev (the first tree after activation)
// reports nothing: the user did not see the earlier content change.
func LiveChanges(prev, next *a11y.Tree, changed []a11y.NodeID) []LiveChange {
	if prev == nil || next == nil {
		return nil
	}
	var out []LiveChange
	seen := map[a11y.NodeID]bool{}
	for _, id := range changed {
		var root *a11y.Node
		if next.Node(id) != nil {
			root = LiveRoot(next, id)
		} else if p := prev.Node(id); p != nil {
			// A removed node: its parent in prev may still be in a live
			// region of next.
			if r := LiveRoot(prev, id); r != nil && next.Node(r.ID) != nil {
				root = next.Node(r.ID)
			}
		}
		if root == nil || seen[root.ID] {
			continue
		}
		seen[root.ID] = true
		text := next.Text(root.ID)
		if prev.Node(root.ID) != nil && prev.Text(root.ID) == text {
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, LiveChange{Root: root.ID, Text: text, Live: root.Live})
	}
	return out
}

// ---------------------------------------------------------------------------
// Change classification (which native notification to send)

// Change says what differs between two versions of a node.
type Change uint16

const (
	ChangeAdded    Change = 1 << iota // not in the previous tree
	ChangeRemoved                     // not in the next tree
	ChangeName                        // Name, Description, RoleDescription, Placeholder
	ChangeValue                       // Value or the range values
	ChangeStates                      // States
	ChangeChildren                    // Children (structure below the node)
	ChangeBounds                      // Bounds
	ChangeOther                       // any other field (role, relations, positions)
)

func (c Change) Has(x Change) bool { return c&x != 0 }

// Diff classifies the change of one node between prev and next (either may
// be nil).
func Diff(prev, next *a11y.Node) Change {
	switch {
	case prev == nil && next == nil:
		return 0
	case prev == nil:
		return ChangeAdded
	case next == nil:
		return ChangeRemoved
	}
	var c Change
	if prev.Name != next.Name || prev.Description != next.Description ||
		prev.RoleDescription != next.RoleDescription || prev.Placeholder != next.Placeholder {
		c |= ChangeName
	}
	if prev.Value != next.Value || prev.HasRange != next.HasRange || prev.ValueNow != next.ValueNow ||
		prev.ValueMin != next.ValueMin || prev.ValueMax != next.ValueMax || prev.Step != next.Step {
		c |= ChangeValue
	}
	if prev.States != next.States {
		c |= ChangeStates
	}
	if !sameIDs(prev.Children, next.Children) {
		c |= ChangeChildren
	}
	if prev.Bounds != next.Bounds {
		c |= ChangeBounds
	}
	if c == 0 && !prev.Equal(next) {
		c |= ChangeOther
	}
	return c
}

func sameIDs(a, b []a11y.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// IndexInParent returns the position of id among its parent's children
// (-1 for the root or an unknown node).
func IndexInParent(t *a11y.Tree, id a11y.NodeID) int {
	n := t.Node(id)
	if n == nil {
		return -1
	}
	p := t.Node(n.Parent)
	if p == nil {
		return -1
	}
	for i, k := range p.Children {
		if k == id {
			return i
		}
	}
	return -1
}

// Sibling returns the child of id's parent at offset delta from id (1 next,
// -1 previous), or 0.
func Sibling(t *a11y.Tree, id a11y.NodeID, delta int) a11y.NodeID {
	n := t.Node(id)
	if n == nil {
		return 0
	}
	p := t.Node(n.Parent)
	if p == nil {
		return 0
	}
	i := IndexInParent(t, id) + delta
	if i < 0 || i >= len(p.Children) {
		return 0
	}
	return p.Children[i]
}

// ---------------------------------------------------------------------------
// Hit testing and coordinates

// HitTest returns the deepest node whose bounds contain (x, y), logical
// pixels relative to the canvas. Later children are on top (overlays come
// after page content), so children are tried last to first. Offscreen
// nodes are skipped. The root is returned when no other node is hit and
// the point is inside it; 0 when the point is outside the root.
func HitTest(t *a11y.Tree, x, y float32) a11y.NodeID {
	if t == nil {
		return 0
	}
	root := t.Node(t.Root)
	if root == nil {
		return 0
	}
	if root.Bounds.W > 0 && root.Bounds.H > 0 && !contains(root.Bounds, x, y) {
		return 0
	}
	if id := hit(t, root, x, y); id != 0 {
		return id
	}
	return root.ID
}

func hit(t *a11y.Tree, n *a11y.Node, x, y float32) a11y.NodeID {
	for i := len(n.Children) - 1; i >= 0; i-- {
		k := t.Node(n.Children[i])
		if k == nil || k.States.Has(a11y.Offscreen) {
			continue
		}
		if id := hit(t, k, x, y); id != 0 {
			return id
		}
		if contains(k.Bounds, x, y) {
			return k.ID
		}
	}
	return 0
}

func contains(r a11y.Rect, x, y float32) bool {
	return r.W > 0 && r.H > 0 && x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// DeviceRect is a rectangle in integer device pixels.
type DeviceRect struct{ X, Y, W, H int32 }

// ToDevice converts logical bounds to device pixels of a surface whose
// content origin is at (ox, oy) device pixels (screen coordinates of the
// canvas's top-left on Windows) and whose scale is the device-pixel
// ratio. Edges are rounded independently so adjacent rectangles stay
// adjacent.
func ToDevice(r a11y.Rect, scale float64, ox, oy int32) DeviceRect {
	if scale <= 0 {
		scale = 1
	}
	x0 := round(float64(r.X) * scale)
	y0 := round(float64(r.Y) * scale)
	x1 := round(float64(r.X+r.W) * scale)
	y1 := round(float64(r.Y+r.H) * scale)
	return DeviceRect{X: ox + x0, Y: oy + y0, W: x1 - x0, H: y1 - y0}
}

// FromDevice converts a device-pixel point (same origin as ToDevice) to
// logical canvas coordinates.
func FromDevice(px, py int32, scale float64, ox, oy int32) (x, y float32) {
	if scale <= 0 {
		scale = 1
	}
	return float32(float64(px-ox) / scale), float32(float64(py-oy) / scale)
}

// Offset moves logical bounds by (dx, dy) logical pixels (Linux window and
// screen coordinates are logical: GDK does the scaling).
func Offset(r a11y.Rect, dx, dy float64) (x, y, w, h int32) {
	x0 := round(float64(r.X) + dx)
	y0 := round(float64(r.Y) + dy)
	x1 := round(float64(r.X+r.W) + dx)
	y1 := round(float64(r.Y+r.H) + dy)
	return x0, y0, x1 - x0, y1 - y0
}

func round(v float64) int32 { return int32(math.Round(v)) }
