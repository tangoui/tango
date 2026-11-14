package ui

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Dock layout model

// DockAxis is the direction of a DockNode split: DockRow lays its children
// side by side (in the reading direction), DockColumn stacks them.
type DockAxis string

const (
	DockRow    DockAxis = "row"
	DockColumn DockAxis = "column"
)

// DockSide is where a tab lands relative to a group: in it (DockCenter)
// or in a new group beside it. DockLeft and DockRight are logical: in a
// right-to-left scope DockLeft is the right (start) side.
//
// Append-only: never reorder or insert; existing values never change.
type DockSide uint8

const (
	DockCenter DockSide = iota
	DockLeft
	DockRight
	DockTop
	DockBottom
)

// DockNode is a node of a dock layout: a split (Axis set, Children with
// their Sizes) or a tab group (a leaf: ID, Tabs and the Active tab). Tabs
// holds application keys; Dock asks the application for each key's title
// and content.
type DockNode struct {
	_ [0]func()

	// Group (leaf).
	ID     string   `json:"id,omitempty"`
	Tabs   []string `json:"tabs,omitempty"`
	Active string   `json:"active,omitempty"`

	// Split.
	Axis     DockAxis    `json:"axis,omitempty"`
	Children []*DockNode `json:"children,omitempty"`
	Sizes    []float64   `json:"sizes,omitempty"` // fractions of the split, one per child, summing to 1
}

// IsGroup reports whether n is a tab group (not a split).
func (n *DockNode) IsGroup() bool { return n.Axis == "" }

// DockLayout is the state of a Dock: a tree of splits whose leaves are
// tab groups, the focused group and the maximized one. The application
// owns it; Dock edits it in place as the user drags, closes, splits or
// resizes. It marshals to JSON with encoding/json (save it when
// DockW.Changed reports) and loads back with ParseDockLayout.
//
// Methods keep the invariants (call Normalize after editing the tree by
// hand): every tab key appears once, splits have at least two children
// and never a child split of the same axis, sizes are positive fractions,
// group IDs are unique and only the root may be an empty group.
type DockLayout struct {
	_ [0]func()

	Root      *DockNode `json:"root"`
	Focus     string    `json:"focus,omitempty"`     // ID of the focused group (new tabs go there)
	Maximized string    `json:"maximized,omitempty"` // ID of the group filling the dock ("" when none)
}

// NewDockLayout returns a layout with one group holding tabs (the first
// one active).
func NewDockLayout(tabs ...string) *DockLayout {
	l := &DockLayout{Root: &DockNode{ID: "g1", Tabs: append([]string(nil), tabs...)}}
	l.Normalize()
	return l
}

// ParseDockLayout decodes a layout saved with json.Marshal and repairs
// whatever breaks the invariants (unknown axes, missing IDs, repeated
// keys, wrong sizes), so a hand-edited or older file still loads.
func ParseDockLayout(data []byte) (*DockLayout, error) {
	l := &DockLayout{}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, err
	}
	l.Normalize()
	return l, nil
}

// Groups returns the tab groups in reading order (depth first).
func (l *DockLayout) Groups() []*DockNode {
	var out []*DockNode
	var walk func(n *DockNode)
	walk = func(n *DockNode) {
		if n == nil {
			return
		}
		if n.IsGroup() {
			out = append(out, n)
			return
		}
		for _, k := range n.Children {
			walk(k)
		}
	}
	walk(l.Root)
	return out
}

// Group returns the group with the given ID (nil when none).
func (l *DockLayout) Group(id string) *DockNode {
	for _, g := range l.Groups() {
		if g.ID == id {
			return g
		}
	}
	return nil
}

// GroupOf returns the group holding the tab key (nil when none).
func (l *DockLayout) GroupOf(key string) *DockNode {
	for _, g := range l.Groups() {
		if slices.Index(g.Tabs, key) >= 0 {
			return g
		}
	}
	return nil
}

// FocusGroup returns the focused group (the first one when Focus names
// none).
func (l *DockLayout) FocusGroup() *DockNode {
	if g := l.Group(l.Focus); g != nil {
		return g
	}
	if gs := l.Groups(); len(gs) > 0 {
		return gs[0]
	}
	return nil
}

// Activate makes key the active tab of its group and focuses the group.
// It reports whether the layout holds key.
func (l *DockLayout) Activate(key string) bool {
	g := l.GroupOf(key)
	if g == nil {
		return false
	}
	g.Active, l.Focus = key, g.ID
	return true
}

// AddTab adds key to the group with ID group ("" for the focused group),
// after its active tab, and activates it. A key already in the layout is
// only activated.
func (l *DockLayout) AddTab(key, group string) {
	if l.Activate(key) {
		return
	}
	if l.Root == nil {
		l.Normalize()
	}
	g := l.Group(group)
	if g == nil {
		g = l.FocusGroup()
	}
	at := slices.Index(g.Tabs, g.Active) + 1
	if at <= 0 {
		at = len(g.Tabs)
	}
	g.Tabs = slices.Insert(g.Tabs, at, key)
	g.Active, l.Focus = key, g.ID
}

// RemoveTab removes key. A group left empty is removed (unless it is the
// last one) and its neighbours take its space; the group's next tab (or
// previous) becomes active. It reports whether the layout held key.
func (l *DockLayout) RemoveTab(key string) bool {
	g := l.GroupOf(key)
	if g == nil {
		return false
	}
	i := slices.Index(g.Tabs, key)
	g.Tabs = append(g.Tabs[:i:i], g.Tabs[i+1:]...)
	if g.Active == key {
		g.Active = ""
		if len(g.Tabs) > 0 {
			g.Active = g.Tabs[min(i, len(g.Tabs)-1)]
		}
	}
	l.Normalize()
	return true
}

// MoveTab moves key into the group with ID group at index (clamped; < 0
// appends) and activates it there. Moving within a group reorders it. It
// reports whether anything moved.
func (l *DockLayout) MoveTab(key, group string, index int) bool {
	src, dst := l.GroupOf(key), l.Group(group)
	if src == nil || dst == nil {
		return false
	}
	from := slices.Index(src.Tabs, key)
	if index < 0 || index > len(dst.Tabs) {
		index = len(dst.Tabs)
	}
	if src == dst {
		if index > from {
			index-- // the index counts the tab itself
		}
		if index == from {
			l.Activate(key)
			return false
		}
		src.Tabs = slices.Insert(append(src.Tabs[:from:from], src.Tabs[from+1:]...), index, key)
		l.Activate(key)
		return true
	}
	src.Tabs = append(src.Tabs[:from:from], src.Tabs[from+1:]...)
	if src.Active == key {
		src.Active = ""
		if len(src.Tabs) > 0 {
			src.Active = src.Tabs[min(from, len(src.Tabs)-1)]
		}
	}
	dst.Tabs = slices.Insert(dst.Tabs, index, key)
	dst.Active, l.Focus = key, dst.ID
	l.Normalize()
	return true
}

// SplitTab moves key into a new group on side of the group with ID group
// (DockCenter appends it to that group instead) and returns the new
// group's ID. Splitting a group's only tab off itself does nothing.
func (l *DockLayout) SplitTab(key, group string, side DockSide) (string, bool) {
	if side == DockCenter {
		return group, l.MoveTab(key, group, -1)
	}
	src, dst := l.GroupOf(key), l.Group(group)
	if src == nil || dst == nil || src == dst && len(src.Tabs) == 1 {
		return "", false
	}
	ng := &DockNode{ID: l.newGroupID(), Tabs: []string{key}, Active: key}
	// Take the tab out first: the source may collapse, which can change
	// the target's parent (but never removes the target, which keeps tabs).
	i := slices.Index(src.Tabs, key)
	src.Tabs = append(src.Tabs[:i:i], src.Tabs[i+1:]...)
	if src.Active == key && len(src.Tabs) > 0 {
		src.Active = src.Tabs[min(i, len(src.Tabs)-1)]
	}
	l.Normalize()
	dst = l.Group(group)
	axis := DockRow
	if side == DockTop || side == DockBottom {
		axis = DockColumn
	}
	before := side == DockLeft || side == DockTop
	parent, at := l.parentOf(dst)
	if parent != nil && parent.Axis == axis {
		// Insert next to the target, halving its share.
		half := parent.Sizes[at] / 2
		parent.Sizes[at] = half
		j := at + 1
		if before {
			j = at
		}
		parent.Children = slices.Insert(parent.Children, j, ng)
		parent.Sizes = slices.Insert(parent.Sizes, j, half)
	} else {
		// Replace the target with a split holding it and the new group.
		moved := &DockNode{ID: dst.ID, Tabs: dst.Tabs, Active: dst.Active}
		kids := []*DockNode{moved, ng}
		if before {
			kids = []*DockNode{ng, moved}
		}
		*dst = DockNode{Axis: axis, Children: kids, Sizes: []float64{0.5, 0.5}}
	}
	l.Focus = ng.ID
	l.Normalize()
	return ng.ID, true
}

// parentOf returns the split holding n and n's index in it (nil, -1 for
// the root).
func (l *DockLayout) parentOf(n *DockNode) (*DockNode, int) {
	var found *DockNode
	at := -1
	var walk func(p *DockNode)
	walk = func(p *DockNode) {
		if p == nil || p.IsGroup() || found != nil {
			return
		}
		for i, k := range p.Children {
			if k == n {
				found, at = p, i
				return
			}
			walk(k)
		}
	}
	walk(l.Root)
	return found, at
}

// newGroupID returns an unused group ID ("g" and a number).
func (l *DockLayout) newGroupID() string {
	next := 1
	for _, g := range l.Groups() {
		if n, err := strconv.Atoi(strings.TrimPrefix(g.ID, "g")); err == nil && strings.HasPrefix(g.ID, "g") && n >= next {
			next = n + 1
		}
	}
	return "g" + strconv.Itoa(next)
}

// Normalize restores the invariants after the tree was edited by hand:
// it drops nil children, repeated tab keys and empty groups (keeping one
// group), collapses splits with a single child, merges a split into a
// parent of the same axis, fixes sizes, unknown axes, missing or repeated
// group IDs and active tabs, and clears Focus / Maximized when they name
// no group.
func (l *DockLayout) Normalize() {
	seenKey := map[string]bool{}
	seenID := map[string]bool{}
	var fixIDs []*DockNode
	var norm func(n *DockNode) *DockNode
	norm = func(n *DockNode) *DockNode {
		if n == nil {
			return nil
		}
		if n.Axis == "" && len(n.Children) > 0 && len(n.Tabs) == 0 {
			n.Axis = DockRow
		}
		if n.Axis != "" && n.Axis != DockRow && n.Axis != DockColumn {
			n.Axis = DockRow
		}
		if n.IsGroup() {
			n.Children, n.Sizes = nil, nil
			tabs := n.Tabs[:0:0]
			for _, k := range n.Tabs {
				if !seenKey[k] {
					seenKey[k] = true
					tabs = append(tabs, k)
				}
			}
			n.Tabs = tabs
			if len(tabs) == 0 {
				n.Active = ""
				return nil
			}
			if slices.Index(tabs, n.Active) < 0 {
				n.Active = tabs[0]
			}
			if n.ID == "" || seenID[n.ID] {
				fixIDs = append(fixIDs, n)
			} else {
				seenID[n.ID] = true
			}
			return n
		}
		n.ID, n.Tabs, n.Active = "", nil, ""
		sizes := dockSizes(n.Sizes, len(n.Children))
		var kids []*DockNode
		var ks []float64
		for i, k := range n.Children {
			k = norm(k)
			if k == nil {
				continue
			}
			if !k.IsGroup() && k.Axis == n.Axis {
				// Same axis: splice the grandchildren in, scaled.
				for j, g := range k.Children {
					kids = append(kids, g)
					ks = append(ks, sizes[i]*k.Sizes[j])
				}
				continue
			}
			kids = append(kids, k)
			ks = append(ks, sizes[i])
		}
		switch len(kids) {
		case 0:
			return nil
		case 1:
			return kids[0]
		}
		n.Children, n.Sizes = kids, dockSizes(ks, len(kids))
		return n
	}
	root := norm(l.Root)
	if root == nil {
		// Keep one (empty) group; reuse the old root's ID when it was one.
		id := ""
		if l.Root != nil && l.Root.IsGroup() {
			id = l.Root.ID
		}
		root = &DockNode{ID: id}
		if id == "" {
			fixIDs = append(fixIDs, root)
		}
	}
	l.Root = root
	for _, g := range fixIDs {
		g.ID = ""
		for n := 1; ; n++ {
			id := "g" + strconv.Itoa(n)
			if !seenID[id] {
				g.ID, seenID[id] = id, true
				break
			}
		}
	}
	if l.Group(l.Focus) == nil {
		l.Focus = l.FocusGroup().ID
	}
	if l.Maximized != "" && l.Group(l.Maximized) == nil {
		l.Maximized = ""
	}
}

// dockSizes returns n positive fractions summing to 1: s normalized, or
// equal shares when s does not have n positive finite entries.
func dockSizes(s []float64, n int) []float64 {
	out := make([]float64, n)
	sum := 0.0
	ok := len(s) == n
	for i := 0; ok && i < n; i++ {
		if !(s[i] > 0) || s[i] > 1e9 {
			ok = false
			break
		}
		sum += s[i]
	}
	for i := range out {
		if ok {
			out[i] = s[i] / sum
		} else {
			out[i] = 1 / float64(n)
		}
	}
	return out
}
