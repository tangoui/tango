package devhost

import (
	"encoding/gob"
	"errors"

	"github.com/tangoui/tango/ui/a11y"
)

// Accessibility under hot reload (docs/specs/a11y-bridge.md, "tan dev"):
// the host window's canvas talks to the assistive technology, the worker's
// runner builds the semantics tree. The host tells the worker when a tree
// is wanted (A11yState), the worker sends it as a full snapshot followed by
// deltas (A11yTree), and actions of the assistive technology go back to
// the worker (A11yAction), which hands them to Runner.PerformAction.
//
// The worker only sends A11yTree after an A11yState{On: true}, so a peer
// that never asks never sees one.

// A11yState switches the semantics tree of window Win on or off (host →
// worker). Every On carries a new Epoch: the worker answers with a full
// A11yTree of that epoch and tags the deltas that follow with it, so the
// host can drop trees sent for an earlier request.
type A11yState struct {
	Win   int
	On    bool
	Epoch int
}

// A11yTree is the semantics tree of window Win after a frame that changed
// it (worker → host). With Full set, Nodes holds the whole tree in
// depth-first order; otherwise Nodes holds the nodes added or changed
// since the previous A11yTree of the same epoch and Removed the IDs it no
// longer has. Root, Focus, RTL and Announcements always describe the new
// tree.
type A11yTree struct {
	Win           int
	Epoch         int
	Full          bool
	Root          a11y.NodeID
	Focus         a11y.NodeID
	RTL           bool
	Announcements []a11y.Announcement
	Nodes         []a11y.Node
	Removed       []a11y.NodeID
}

// A11yAction asks the worker to perform an assistive technology's action
// on node ID of window Win (host → worker).
type A11yAction struct {
	Win int
	ID  a11y.NodeID
	Act a11y.Action
	Arg string
}

func init() {
	for _, m := range []any{A11yState{}, A11yTree{}, A11yAction{}} {
		gob.Register(m)
	}
}

// NewA11yTree encodes tree for window win: every node when full, else the
// nodes listed in changed (ui.AccessibilityHost semantics: IDs tree no
// longer has are removals).
func NewA11yTree(win, epoch int, tree *a11y.Tree, changed []a11y.NodeID, full bool) A11yTree {
	m := A11yTree{Win: win, Epoch: epoch, Full: full, Root: tree.Root, Focus: tree.Focus, RTL: tree.RTL,
		Announcements: tree.Announcements}
	if full {
		nodes := tree.Nodes()
		m.Nodes = make([]a11y.Node, len(nodes))
		for i, n := range nodes {
			m.Nodes[i] = *n
		}
		return m
	}
	for _, id := range changed {
		if n := tree.Node(id); n != nil {
			m.Nodes = append(m.Nodes, *n)
		} else {
			m.Removed = append(m.Removed, id)
		}
	}
	return m
}

// ErrA11yDelta is returned by Apply for a delta without a base tree.
var ErrA11yDelta = errors.New("devhost: accessibility delta without a base tree")

// Apply returns the tree m describes: m itself when Full, else prev with
// m's changes. prev is not modified (trees are immutable snapshots);
// unchanged nodes are shared with it. Nodes are ordered depth first from
// the root, followed by any not reachable from it.
func (m A11yTree) Apply(prev *a11y.Tree) (*a11y.Tree, error) {
	t := a11y.NewTree(m.Root)
	t.Focus, t.RTL = m.Focus, m.RTL
	t.Announcements = m.Announcements
	if m.Full {
		for i := range m.Nodes {
			n := m.Nodes[i]
			t.Add(&n)
		}
		return t, nil
	}
	if prev == nil {
		return nil, ErrA11yDelta
	}
	nodes := make(map[a11y.NodeID]*a11y.Node, prev.Len()+len(m.Nodes))
	var order []a11y.NodeID // prev's order, then new nodes
	for _, n := range prev.Nodes() {
		nodes[n.ID] = n
		order = append(order, n.ID)
	}
	for i := range m.Nodes {
		n := m.Nodes[i]
		if _, ok := nodes[n.ID]; !ok {
			order = append(order, n.ID)
		}
		nodes[n.ID] = &n
	}
	for _, id := range m.Removed {
		delete(nodes, id)
	}
	var walk func(id a11y.NodeID)
	walk = func(id a11y.NodeID) {
		n := nodes[id]
		if n == nil || t.Node(id) != nil {
			return
		}
		t.Add(n)
		for _, k := range n.Children {
			walk(k)
		}
	}
	walk(m.Root)
	for _, id := range order {
		if n := nodes[id]; n != nil && t.Node(id) == nil {
			t.Add(n)
		}
	}
	return t, nil
}
