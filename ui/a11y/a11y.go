// Package a11y defines the platform-neutral accessibility (semantics) tree
// produced by the Tango UI runner.
//
// Every frame in which a host asks for it (ui.AccessibilityHost), the runner
// derives a Tree from the widget tree: one Node per semantic element (a
// button, a text field, a table cell, a dialog …) with a stable ID, a Role,
// an accessible name, value, states, bounds and the actions a screen reader
// may perform. Nodes that carry no meaning (layout rows, decorations) are
// flattened away. The runner compares the new tree with the previous one and
// reports only the IDs that were added, changed or removed, so a platform
// bridge (NSAccessibility, UI Automation, AT-SPI, UIAccessibility,
// AccessibilityNodeInfo) can update its proxy objects incrementally.
//
// The package has no dependency on the ui package; bridges only need the
// Tree, the Node fields and the role mapping in Mappings (documented in
// docs/accessibility.md).
package a11y

import (
	"fmt"
	"strings"
)

// NodeID identifies a node across frames. It equals the ui.ID of the widget
// the node was derived from, so it stays stable as long as the widget does.
type NodeID uint64

// Role is the semantic type of a node (WAI-ARIA vocabulary).
//
// Append-only: never reorder or insert; the zero value stays RoleNone.
type Role uint8

const (
	RoleNone      Role = iota // not exposed (internal use)
	RoleWindow                // the root of a tree: the window content
	RoleGroup                 // generic grouping (card, form item, fieldset)
	RoleText                  // static text
	RoleHeading               // title; Node.Level is 1-6
	RoleParagraph             // block of text
	RoleLink
	RoleButton // push button; a toggle button carries Toggleable (+ Pressed)
	RoleCheckbox
	RoleRadio
	RoleRadioGroup
	RoleSwitch
	RoleTextbox    // single or multi-line (Multiline) text input; Protected for passwords
	RoleSearchbox  // text input for searching
	RoleSpinButton // numeric input with step buttons
	RoleCombobox   // input or button that opens a listbox / tree / grid
	RoleListbox
	RoleOption
	RoleMenu
	RoleMenuBar
	RoleMenuItem
	RoleTab
	RoleTabList
	RoleTabPanel
	RoleDialog
	RoleAlertDialog
	RoleAlert  // important, usually time-sensitive message (assertive)
	RoleStatus // advisory message (polite)
	RoleTooltip
	RoleSlider
	RoleProgressBar
	RoleTable
	RoleGrid // interactive table (keyboard moves between cells)
	RoleRow
	RoleCell
	RoleColumnHeader
	RoleRowHeader
	RoleTree
	RoleTreeItem
	RoleList
	RoleListItem
	RoleImage
	RoleNavigation // breadcrumb, pagination, anchor
	RoleToolbar
	RoleSeparator // static divider; focusable (with Value range) when it resizes panes
	RoleScrollArea
	RoleForm
	RoleFigure // chart: Name + Description summary
	RoleTimer  // countdown
	RoleMeter  // read-only scalar (Statistic, Gauge)
	roleCount
)

var roleNames = [...]string{
	RoleNone: "none", RoleWindow: "window", RoleGroup: "group", RoleText: "text", RoleHeading: "heading",
	RoleParagraph: "paragraph", RoleLink: "link", RoleButton: "button", RoleCheckbox: "checkbox",
	RoleRadio: "radio", RoleRadioGroup: "radiogroup", RoleSwitch: "switch", RoleTextbox: "textbox",
	RoleSearchbox: "searchbox", RoleSpinButton: "spinbutton", RoleCombobox: "combobox", RoleListbox: "listbox",
	RoleOption: "option", RoleMenu: "menu", RoleMenuBar: "menubar", RoleMenuItem: "menuitem", RoleTab: "tab",
	RoleTabList: "tablist", RoleTabPanel: "tabpanel", RoleDialog: "dialog", RoleAlertDialog: "alertdialog",
	RoleAlert: "alert", RoleStatus: "status", RoleTooltip: "tooltip", RoleSlider: "slider",
	RoleProgressBar: "progressbar", RoleTable: "table", RoleGrid: "grid", RoleRow: "row", RoleCell: "cell",
	RoleColumnHeader: "columnheader", RoleRowHeader: "rowheader", RoleTree: "tree", RoleTreeItem: "treeitem",
	RoleList: "list", RoleListItem: "listitem", RoleImage: "image", RoleNavigation: "navigation",
	RoleToolbar: "toolbar", RoleSeparator: "separator", RoleScrollArea: "scrollarea", RoleForm: "form",
	RoleFigure: "figure", RoleTimer: "timer", RoleMeter: "meter",
}

// String returns the WAI-ARIA style role name ("button", "treeitem").
func (r Role) String() string {
	if int(r) < len(roleNames) && roleNames[r] != "" {
		return roleNames[r]
	}
	return fmt.Sprintf("role(%d)", uint8(r))
}

// Roles returns every exposed role (RoleNone excluded) in declaration order.
func Roles() []Role {
	out := make([]Role, 0, roleCount-1)
	for r := RoleWindow; r < roleCount; r++ {
		out = append(out, r)
	}
	return out
}

// ParseRole returns the role named s (as printed by Role.String).
func ParseRole(s string) (Role, bool) {
	for r := Role(0); r < roleCount; r++ {
		if roleNames[r] == s {
			return r, true
		}
	}
	return RoleNone, false
}

// Interactive reports whether the role is a control a user operates
// (and therefore needs an accessible name and keyboard access).
func (r Role) Interactive() bool {
	switch r {
	case RoleLink, RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleTextbox, RoleSearchbox,
		RoleSpinButton, RoleCombobox, RoleListbox, RoleOption, RoleMenuItem, RoleTab, RoleSlider,
		RoleTreeItem:
		return true
	}
	return false
}

// NameFromContent reports whether a node of this role without an explicit
// name is named by the text it contains (WAI-ARIA "name from content").
func (r Role) NameFromContent() bool {
	switch r {
	case RoleLink, RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleOption, RoleMenuItem, RoleTab,
		RoleTreeItem, RoleCell, RoleColumnHeader, RoleRowHeader, RoleHeading, RoleTooltip, RoleListItem,
		RoleRow, RoleText, RoleParagraph, RoleStatus, RoleAlert:
		return true
	}
	return false
}

// Atomic reports whether the role's descendants are presentational: their
// text becomes the node's name and plain text children are not exposed
// separately (a button's label, an option's text).
func (r Role) Atomic() bool {
	switch r {
	case RoleButton, RoleLink, RoleCheckbox, RoleRadio, RoleSwitch, RoleOption, RoleMenuItem, RoleTab,
		RoleSlider, RoleProgressBar, RoleImage, RoleTextbox, RoleSearchbox, RoleSpinButton, RoleHeading,
		RoleSeparator, RoleMeter, RoleTimer, RoleText, RoleParagraph:
		return true
	}
	return false
}

// State is a set of boolean node states.
//
// Append-only: a new flag takes the next free bit; existing bits never
// change.
type State uint32

const (
	Disabled        State = 1 << iota
	Focusable             // can take keyboard focus
	Focused               // has keyboard focus
	Checkable             // has a checked state (checkbox, radio, switch, menu item)
	Checked               // checked / on
	Mixed                 // indeterminate (checkbox, tri-state tree item)
	Selectable            // can be selected (option, tab, row, cell, tree item)
	Selected              // selected / current tab
	Expandable            // has an expanded state
	Expanded              // expanded / open
	Toggleable            // toggle button: has a pressed state
	Pressed               // toggle button pressed
	Required              // form field must be filled
	Invalid               // failed validation; Description holds the message
	ReadOnly              // value can be read and focused but not edited
	Multiline             // multi-line text input
	Protected             // password input: Value is not exposed
	Modal                 // dialog that makes the rest of the window inert
	Multiselectable       // listbox / tree / table allows several selected items
	Busy                  // loading; content may change
	Offscreen             // scrolled out of the visible area
	Current               // current item of a set (page, step, breadcrumb)
	HasPopup              // opens a menu, listbox or dialog
	Visited               // visited link
	Vertical              // orientation (sliders, separators, tab lists, menus)
	Horizontal
	Editable // text can be typed (combobox with search)
)

var stateNames = []string{"disabled", "focusable", "focused", "checkable", "checked", "mixed", "selectable",
	"selected", "expandable", "expanded", "toggleable", "pressed", "required", "invalid", "readonly",
	"multiline", "protected", "modal", "multiselectable", "busy", "offscreen", "current", "haspopup",
	"visited", "vertical", "horizontal", "editable"}

// Has reports whether every state of x is set.
func (s State) Has(x State) bool { return s&x == x }

// String lists the set states ("focusable|checked").
func (s State) String() string {
	var b []string
	for i, n := range stateNames {
		if s&(1<<i) != 0 {
			b = append(b, n)
		}
	}
	return strings.Join(b, "|")
}

// Action is a set of operations a host may ask a node to perform
// (ui.Runner.PerformAction).
//
// Append-only: a new flag takes the next free bit; existing bits never
// change.
type Action uint16

const (
	Press          Action = 1 << iota // activate (click, toggle, select an option)
	Increment                         // step a slider / spin button up
	Decrement                         // step it down
	Expand                            // open a combobox, expand a tree item / collapse panel
	Collapse                          // close it
	SetValue                          // replace the value (text, or a number for ranges)
	Focus                             // move keyboard focus here
	ScrollIntoView                    // scroll ancestors so the node is visible
	Dismiss                           // close a dialog, popup or message (Escape)
)

var actionNames = []string{"press", "increment", "decrement", "expand", "collapse", "setValue", "focus",
	"scrollIntoView", "dismiss"}

// Has reports whether every action of x is supported.
func (a Action) Has(x Action) bool { return a&x == x }

// String lists the actions ("press|focus").
func (a Action) String() string {
	var b []string
	for i, n := range actionNames {
		if a&(1<<i) != 0 {
			b = append(b, n)
		}
	}
	return strings.Join(b, "|")
}

// Live is the politeness of a live region: how a screen reader announces
// changes of its content.
//
// Append-only: never reorder or insert; the zero value stays LiveOff.
type Live uint8

const (
	LiveOff       Live = iota
	LivePolite         // announce when the user is idle (status, toast)
	LiveAssertive      // interrupt (errors, alerts)
)

func (l Live) String() string {
	switch l {
	case LivePolite:
		return "polite"
	case LiveAssertive:
		return "assertive"
	}
	return "off"
}

// Sort is the sort direction of a column header.
//
// Append-only: never reorder or insert; the zero value stays SortNone.
type Sort uint8

const (
	SortNone Sort = iota
	SortAscending
	SortDescending
)

func (s Sort) String() string {
	switch s {
	case SortAscending:
		return "ascending"
	case SortDescending:
		return "descending"
	}
	return "none"
}

// Rect is a rectangle in window logical pixels (the coordinates of
// ui.Window.Size; multiply by Window.Scale for device pixels).
type Rect struct{ X, Y, W, H float32 }

// Node is one element of the semantics tree. Zero fields mean "not
// applicable"; hosts should not expose them.
type Node struct {
	ID       NodeID
	Parent   NodeID   // 0 for the root
	Children []NodeID // in reading order
	Role     Role

	Name        string // accessible name (label)
	Description string // longer help / validation message / chart summary
	Value       string // text value (text field content, selected option, "3 of 5")
	Placeholder string // hint shown in an empty text field
	// RoleDescription refines the role for speech ("chart", "color picker").
	RoleDescription string

	// Range values (slider, progress bar, spin button, meter); valid when
	// HasRange.
	HasRange                           bool
	ValueNow, ValueMin, ValueMax, Step float64

	States  State
	Actions Action
	Bounds  Rect
	Live    Live

	Level              int // heading level 1-6, tree item depth (1 = top level)
	PosInSet, SetSize  int // 1-based position among siblings of a set (options, tabs, tree items)
	RowIndex, ColIndex int // 1-based position in a table / grid (rows, cells, headers)
	RowCount, ColCount int // table / grid size (including rows not built by virtualization)
	Sort               Sort

	// Relations (IDs of other nodes in the same tree, 0 = none).
	LabelledBy       NodeID // node whose text is the name (form label)
	DescribedBy      NodeID // node whose text is the description (error message)
	Controls         NodeID // popup / panel this node opens or drives
	ActiveDescendant NodeID // focused item of a composite that keeps focus itself

	Shortcut string // keyboard shortcut, e.g. "Ctrl+K"

	// RTL reports that the node's content lays out and reads right to
	// left (its ConfigProvider scope; nested scopes may differ from the
	// window, see Tree.RTL). It is informational for bridges that expose a
	// text direction: Bounds are already the mirrored screen rectangles and
	// Children stay in reading order, so hosts must not flip either.
	RTL bool
}

// Equal reports whether a and b describe the same node (all fields,
// including the order of children).
func (a *Node) Equal(b *Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if a.Children[i] != b.Children[i] {
			return false
		}
	}
	return a.ID == b.ID && a.Parent == b.Parent && a.Role == b.Role && a.Name == b.Name &&
		a.Description == b.Description && a.Value == b.Value && a.Placeholder == b.Placeholder &&
		a.RoleDescription == b.RoleDescription && a.HasRange == b.HasRange && a.ValueNow == b.ValueNow &&
		a.ValueMin == b.ValueMin && a.ValueMax == b.ValueMax && a.Step == b.Step && a.States == b.States &&
		a.Actions == b.Actions && a.Bounds == b.Bounds && a.Live == b.Live && a.Level == b.Level &&
		a.PosInSet == b.PosInSet && a.SetSize == b.SetSize && a.RowIndex == b.RowIndex &&
		a.ColIndex == b.ColIndex && a.RowCount == b.RowCount && a.ColCount == b.ColCount && a.Sort == b.Sort &&
		a.LabelledBy == b.LabelledBy && a.DescribedBy == b.DescribedBy && a.Controls == b.Controls &&
		a.ActiveDescendant == b.ActiveDescendant && a.Shortcut == b.Shortcut && a.RTL == b.RTL
}

// String formats the node for logs and test failures.
func (n *Node) String() string {
	var b strings.Builder
	b.WriteString(n.Role.String())
	if n.Name != "" {
		fmt.Fprintf(&b, " %q", n.Name)
	}
	if n.Value != "" {
		fmt.Fprintf(&b, " value=%q", n.Value)
	}
	if n.HasRange {
		fmt.Fprintf(&b, " range=%g[%g,%g]", n.ValueNow, n.ValueMin, n.ValueMax)
	}
	if n.Description != "" {
		fmt.Fprintf(&b, " desc=%q", n.Description)
	}
	if n.States != 0 {
		fmt.Fprintf(&b, " [%s]", n.States)
	}
	if n.Live != LiveOff {
		fmt.Fprintf(&b, " live=%s", n.Live)
	}
	if n.Level > 0 {
		fmt.Fprintf(&b, " level=%d", n.Level)
	}
	if n.SetSize > 0 {
		fmt.Fprintf(&b, " %d/%d", n.PosInSet, n.SetSize)
	}
	if n.RowIndex > 0 || n.ColIndex > 0 {
		fmt.Fprintf(&b, " r%dc%d", n.RowIndex, n.ColIndex)
	}
	if n.Sort != SortNone {
		fmt.Fprintf(&b, " sort=%s", n.Sort)
	}
	return b.String()
}

// Announcement is a message for the screen reader that is not tied to a
// persistent node (ui.Context.Announce). Hosts speak it once.
type Announcement struct {
	Text string
	Live Live
}

// Tree is a snapshot of the semantics of one window. It is immutable once
// handed to a host: the runner builds a new Tree every frame it reports.
type Tree struct {
	Root  NodeID
	Focus NodeID // node with keyboard focus (0: none)
	// Announcements made during the frame that produced this tree.
	Announcements []Announcement
	// RTL is the window's default direction (its root scope). Nodes carry
	// their own scope's direction in Node.RTL.
	RTL bool

	nodes map[NodeID]*Node
	order []NodeID // depth-first order, root first
}

// NewTree returns an empty tree; Add nodes parent-first.
func NewTree(root NodeID) *Tree {
	return &Tree{Root: root, nodes: map[NodeID]*Node{}}
}

// Add inserts n (its ID must be unique in the tree). Builders add nodes in
// depth-first order so that Walk and Nodes follow reading order.
func (t *Tree) Add(n *Node) {
	if _, dup := t.nodes[n.ID]; !dup {
		t.order = append(t.order, n.ID)
	}
	t.nodes[n.ID] = n
}

// Remove deletes node id (not its children) from the tree. Builders use it
// to fold presentational children into their parent; the caller fixes the
// parent's Children.
func (t *Tree) Remove(id NodeID) {
	if _, ok := t.nodes[id]; !ok {
		return
	}
	delete(t.nodes, id)
	for i, x := range t.order {
		if x == id {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
}

// Node returns the node with id, or nil.
func (t *Tree) Node(id NodeID) *Node {
	if t == nil {
		return nil
	}
	return t.nodes[id]
}

// Len returns the number of nodes.
func (t *Tree) Len() int {
	if t == nil {
		return 0
	}
	return len(t.nodes)
}

// Nodes returns every node in depth-first (reading) order.
func (t *Tree) Nodes() []*Node {
	if t == nil {
		return nil
	}
	out := make([]*Node, 0, len(t.order))
	for _, id := range t.order {
		out = append(out, t.nodes[id])
	}
	return out
}

// Walk calls fn for every node depth first starting at the root; depth is
// 0 for the root. Returning false skips the node's children.
func (t *Tree) Walk(fn func(n *Node, depth int) bool) {
	if t == nil {
		return
	}
	var walk func(id NodeID, d int)
	walk = func(id NodeID, d int) {
		n := t.nodes[id]
		if n == nil || !fn(n, d) {
			return
		}
		for _, k := range n.Children {
			walk(k, d+1)
		}
	}
	walk(t.Root, 0)
}

// Find returns the first node (reading order) with role and name; role
// RoleNone matches any role and name "" any name.
func (t *Tree) Find(role Role, name string) *Node {
	for _, n := range t.Nodes() {
		if (role == RoleNone || n.Role == role) && (name == "" || n.Name == name) {
			return n
		}
	}
	return nil
}

// FindAll returns every node with role and name (see Find).
func (t *Tree) FindAll(role Role, name string) []*Node {
	var out []*Node
	for _, n := range t.Nodes() {
		if (role == RoleNone || n.Role == role) && (name == "" || n.Name == name) {
			out = append(out, n)
		}
	}
	return out
}

// Ancestor returns the nearest ancestor of id with role, or nil.
func (t *Tree) Ancestor(id NodeID, role Role) *Node {
	n := t.Node(id)
	for n != nil && n.Parent != 0 {
		n = t.Node(n.Parent)
		if n != nil && n.Role == role {
			return n
		}
	}
	return nil
}

// Text returns the names of n and its descendants joined by spaces
// (useful to read the content of a dialog or a live region).
func (t *Tree) Text(id NodeID) string {
	var parts []string
	var walk func(id NodeID)
	walk = func(id NodeID) {
		n := t.Node(id)
		if n == nil {
			return
		}
		if n.Name != "" {
			parts = append(parts, n.Name)
		}
		for _, k := range n.Children {
			walk(k)
		}
	}
	walk(id)
	return strings.Join(parts, " ")
}

// Dump formats the tree as an indented outline (debugging, golden tests).
func (t *Tree) Dump() string {
	var b strings.Builder
	t.Walk(func(n *Node, d int) bool {
		b.WriteString(strings.Repeat("  ", d))
		b.WriteString(n.String())
		b.WriteByte('\n')
		return true
	})
	return b.String()
}

// Diff returns the IDs of nodes added or changed in next, followed by the
// IDs of nodes of prev that next no longer has (look them up in next: a
// nil Node means "removed"). A nil prev reports every node of next.
func Diff(prev, next *Tree) []NodeID {
	var out []NodeID
	for _, id := range next.order {
		n := next.nodes[id]
		if prev == nil || !n.Equal(prev.nodes[id]) {
			out = append(out, id)
		}
	}
	if prev != nil {
		for _, id := range prev.order {
			if _, ok := next.nodes[id]; !ok {
				out = append(out, id)
			}
		}
	}
	return out
}
