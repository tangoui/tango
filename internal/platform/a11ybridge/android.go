package a11ybridge

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
)

// ---------------------------------------------------------------------------
// Android (AccessibilityNodeProvider, docs/specs/mobile-canvas.md §5.2)
//
// The Android canvas exposes the tree through an AccessibilityNodeProvider
// on its view: the view itself stands for the root node and every other
// node is a virtual view with an int32 id. The helpers below turn nodes
// into what AccessibilityNodeInfo needs, in Tango's own bit sets; the Java
// side (CanvasA11y.java) maps the bits to the framework's constants, so no
// Android constant is hard-coded here.

// AndroidClass returns the AccessibilityNodeInfo class name of n (TalkBack
// derives the spoken role from it): the Android column of a11y.Mappings,
// with a toggle button as android.widget.ToggleButton.
func AndroidClass(n *a11y.Node) string {
	if n.Role == a11y.RoleButton && n.States.Has(a11y.Toggleable) {
		return "android.widget.ToggleButton"
	}
	p, ok := a11y.Mappings[n.Role]
	if !ok || p.Android == "" {
		return "android.view.View"
	}
	return p.Android
}

// AndroidState is a set of AccessibilityNodeInfo boolean properties.
type AndroidState uint32

const (
	AndroidCheckable   AndroidState = 1 << iota // setCheckable
	AndroidChecked                              // setChecked(true)
	AndroidMixed                                // partially checked (setChecked(CHECKED_STATE_PARTIAL) on API 36)
	AndroidEnabled                              // setEnabled
	AndroidFocusable                            // setFocusable (input focus)
	AndroidFocused                              // setFocused (input focus)
	AndroidSelected                             // setSelected
	AndroidClickable                            // setClickable
	AndroidScrollable                           // setScrollable
	AndroidEditable                             // setEditable
	AndroidPassword                             // setPassword
	AndroidMultiLine                            // setMultiLine
	AndroidHeading                              // setHeading
	AndroidVisible                              // setVisibleToUser
	AndroidInvalid                              // setContentInvalid
	AndroidDismissable                          // setDismissable
	AndroidExpanded                             // expanded (API 36 setExpandedState; else the actions say it)
	AndroidCollapsed                            // expandable and collapsed
	AndroidRequired                             // form field must be filled (API 36 setFieldRequired)
)

func (s AndroidState) Has(x AndroidState) bool { return s&x == x }

// AndroidStates returns the boolean properties of n (docs/accessibility.md
// section 4). Focused is the node's own input-focus state; the backend
// clears it while its view has no focus if it wants to.
func AndroidStates(n *a11y.Node) AndroidState {
	st := n.States
	var s AndroidState
	set := func(cond bool, x AndroidState) {
		if cond {
			s |= x
		}
	}
	toggle := MacToggle(n)
	set(toggle || st.Has(a11y.Checkable), AndroidCheckable)
	set(toggle && CheckValue(n) == 1, AndroidChecked)
	set(st.Has(a11y.Mixed), AndroidMixed)
	set(!st.Has(a11y.Disabled), AndroidEnabled)
	set(st.Has(a11y.Focusable) || n.Actions.Has(a11y.Focus), AndroidFocusable)
	set(st.Has(a11y.Focused), AndroidFocused)
	set(st.Has(a11y.Selected) || st.Has(a11y.Current), AndroidSelected)
	set(n.Actions.Has(a11y.Press) || clickToggles(n), AndroidClickable)
	set(n.Role == a11y.RoleScrollArea && (n.Actions.Has(a11y.Increment) || n.Actions.Has(a11y.Decrement)), AndroidScrollable)
	if isTextInput(n) {
		set(!st.Has(a11y.ReadOnly) && !st.Has(a11y.Disabled), AndroidEditable)
		set(st.Has(a11y.Multiline), AndroidMultiLine)
	}
	set(st.Has(a11y.Protected), AndroidPassword)
	set(n.Role == a11y.RoleHeading || n.Role == a11y.RoleColumnHeader || n.Role == a11y.RoleRowHeader, AndroidHeading)
	set(!st.Has(a11y.Offscreen), AndroidVisible)
	set(st.Has(a11y.Invalid), AndroidInvalid)
	set(n.Actions.Has(a11y.Dismiss), AndroidDismissable)
	set(st.Has(a11y.Expanded), AndroidExpanded)
	set(expandable(n) && !st.Has(a11y.Expanded), AndroidCollapsed)
	set(st.Has(a11y.Required), AndroidRequired)
	return s
}

func expandable(n *a11y.Node) bool {
	return n.States.Has(a11y.Expandable) || n.Actions.Has(a11y.Expand) || n.Actions.Has(a11y.Collapse)
}

// clickToggles reports whether a click on n expands or collapses it (an
// expandable trigger without a press action).
func clickToggles(n *a11y.Node) bool {
	return !n.Actions.Has(a11y.Press) && (n.Actions.Has(a11y.Expand) || n.Actions.Has(a11y.Collapse))
}

// AndroidAction is a set of AccessibilityNodeInfo actions. Accessibility
// focus (ACTION_ACCESSIBILITY_FOCUS / CLEAR) is not listed: every node has
// it and the Java side handles it alone.
type AndroidAction uint16

const (
	AndroidClick          AndroidAction = 1 << iota // ACTION_CLICK
	AndroidFocus                                    // ACTION_FOCUS (input focus)
	AndroidScrollForward                            // ACTION_SCROLL_FORWARD
	AndroidScrollBackward                           // ACTION_SCROLL_BACKWARD
	AndroidExpand                                   // ACTION_EXPAND
	AndroidCollapse                                 // ACTION_COLLAPSE
	AndroidSetText                                  // ACTION_SET_TEXT
	AndroidSetProgress                              // ACTION_SET_PROGRESS
	AndroidShowOnScreen                             // ACTION_SHOW_ON_SCREEN
	AndroidDismiss                                  // ACTION_DISMISS
)

func (a AndroidAction) Has(x AndroidAction) bool { return a&x == x }

// AndroidActions returns the actions n offers (docs/accessibility.md
// section 5). Only the applicable one of expand and collapse is offered
// when the node says whether it is expanded.
func AndroidActions(n *a11y.Node) AndroidAction {
	var a AndroidAction
	add := func(cond bool, x AndroidAction) {
		if cond {
			a |= x
		}
	}
	acts := n.Actions
	add(acts.Has(a11y.Press) || clickToggles(n), AndroidClick)
	add(acts.Has(a11y.Focus), AndroidFocus)
	add(acts.Has(a11y.Increment), AndroidScrollForward)
	add(acts.Has(a11y.Decrement), AndroidScrollBackward)
	known := n.States.Has(a11y.Expandable)
	exp := n.States.Has(a11y.Expanded)
	add(acts.Has(a11y.Expand) && (!known || !exp), AndroidExpand)
	add(acts.Has(a11y.Collapse) && (!known || exp), AndroidCollapse)
	add(acts.Has(a11y.SetValue) && !n.HasRange && !n.States.Has(a11y.ReadOnly), AndroidSetText)
	add(n.HasRange && (acts.Has(a11y.SetValue) || acts.Has(a11y.Increment) || acts.Has(a11y.Decrement)), AndroidSetProgress)
	add(acts.Has(a11y.ScrollIntoView), AndroidShowOnScreen)
	add(acts.Has(a11y.Dismiss), AndroidDismiss)
	return a
}

// AndroidPerform turns an Android action on n into the a11y action and
// argument to hand to CanvasAccessibleSink.AccessibilityAction. text is
// ACTION_SET_TEXT's text, progress ACTION_SET_PROGRESS's value. ok is
// false when n does not offer the action (or the requested progress is
// already its value).
func AndroidPerform(n *a11y.Node, act AndroidAction, text string, progress float64) (a11y.Action, string, bool) {
	if n == nil || !AndroidActions(n).Has(act) {
		return 0, "", false
	}
	switch act {
	case AndroidClick:
		if n.Actions.Has(a11y.Press) {
			return a11y.Press, "", true
		}
		if n.States.Has(a11y.Expanded) && n.Actions.Has(a11y.Collapse) || !n.Actions.Has(a11y.Expand) {
			return a11y.Collapse, "", true
		}
		return a11y.Expand, "", true
	case AndroidFocus:
		return a11y.Focus, "", true
	case AndroidScrollForward:
		return a11y.Increment, "", true
	case AndroidScrollBackward:
		return a11y.Decrement, "", true
	case AndroidExpand:
		return a11y.Expand, "", true
	case AndroidCollapse:
		return a11y.Collapse, "", true
	case AndroidSetText:
		return a11y.SetValue, text, true
	case AndroidSetProgress:
		if math.IsNaN(progress) || math.IsInf(progress, 0) {
			return 0, "", false
		}
		if n.ValueMax > n.ValueMin {
			progress = min(max(progress, n.ValueMin), n.ValueMax)
		}
		switch a := RangeAction(n, progress); a {
		case a11y.SetValue:
			return a, strconv.FormatFloat(progress, 'g', -1, 64), true
		case 0:
			return 0, "", false
		default:
			return a, "", true
		}
	case AndroidShowOnScreen:
		return a11y.ScrollIntoView, "", true
	case AndroidDismiss:
		return a11y.Dismiss, "", true
	}
	return 0, "", false
}

// AndroidStrings are the text properties of an AccessibilityNodeInfo.
type AndroidStrings struct {
	Text               string // setText
	ContentDescription string // setContentDescription
	StateDescription   string // setStateDescription (API 30)
	Hint               string // setHintText (API 26)
	Error              string // setError (with setContentInvalid)
	PaneTitle          string // setPaneTitle (API 28)
	RoleDescription    string // extras "AccessibilityNodeInfo.roleDescription"
}

// AndroidText returns the text properties of n. Text inputs carry their
// content as text and their label and placeholder as hint (TalkBack reads
// the hint of an empty field and after the content otherwise); images
// carry their name as content description; every other node its name as
// text. A value of a non-text node (a cell's detail, a slider's spoken
// value) is its state description. The description is the hint, or the
// error message of an invalid node.
func AndroidText(n *a11y.Node) AndroidStrings {
	var s AndroidStrings
	desc := n.Description
	if n.States.Has(a11y.Invalid) {
		s.Error, desc = n.Description, ""
	}
	switch {
	case isTextInput(n):
		if !n.States.Has(a11y.Protected) {
			s.Text = n.Value
		}
		s.Hint = joinDistinct(n.Name, n.Placeholder, desc)
	case n.Role == a11y.RoleImage || n.Role == a11y.RoleFigure:
		s.ContentDescription = n.Name
		s.Hint = desc
	default:
		s.Text = n.Name
		if n.Value != n.Name {
			s.StateDescription = n.Value
		}
		s.Hint = desc
	}
	if n.Role == a11y.RoleDialog || n.Role == a11y.RoleAlertDialog {
		s.PaneTitle = n.Name
	}
	s.RoleDescription = n.RoleDescription
	return s
}

// joinDistinct joins the non-empty parts that differ from the ones before
// them with ", ".
func joinDistinct(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dup := false
		for _, q := range out {
			if q == p {
				dup = true
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

// AndroidLive is the live-region mode of n: 0 none, 1 polite, 2
// assertive (setLiveRegion).
func AndroidLive(n *a11y.Node) int {
	switch n.Live {
	case a11y.LivePolite:
		return 1
	case a11y.LiveAssertive:
		return 2
	}
	return 0
}

// AndroidRangeType says how a range is spoken: AndroidRangeInt or
// AndroidRangeFloat (RangeInfo.RANGE_TYPE_*).
const (
	AndroidRangeInt   = 0
	AndroidRangeFloat = 1
)

// AndroidRange returns the RangeInfo of a range node: integer when the
// bounds, the value and the step are whole numbers.
func AndroidRange(n *a11y.Node) (typ int, minV, maxV, cur float64) {
	whole := func(v float64) bool { return v == math.Trunc(v) }
	typ = AndroidRangeFloat
	if whole(n.ValueMin) && whole(n.ValueMax) && whole(n.ValueNow) && whole(n.Step) {
		typ = AndroidRangeInt
	}
	return typ, n.ValueMin, n.ValueMax, n.ValueNow
}

// AndroidCollection is the CollectionInfo of a list, table, tree, tab list
// or radio group.
type AndroidCollection struct {
	Rows, Cols   int
	Hierarchical bool
	Selection    int // 0 none, 1 single, 2 multiple (SELECTION_MODE_*)
}

// AndroidItem is the CollectionItemInfo of an item of a collection
// (0-based indices).
type AndroidItem struct {
	Row, Col int
	Heading  bool
	Selected bool
}

func listLike(r a11y.Role) bool {
	switch r {
	case a11y.RoleList, a11y.RoleListbox, a11y.RoleMenu, a11y.RoleRadioGroup:
		return true
	}
	return false
}

// AndroidCollectionInfo returns the CollectionInfo of n, if n is a
// collection. Lists count their children (or the set size of the first
// one, which includes items virtualization did not build); tables use
// RowCount and ColCount; trees count their tree items.
func AndroidCollectionInfo(t *a11y.Tree, n *a11y.Node) (AndroidCollection, bool) {
	var c AndroidCollection
	selMode := func(items []a11y.NodeID) int {
		if n.States.Has(a11y.Multiselectable) {
			return 2
		}
		for _, id := range items {
			if k := t.Node(id); k != nil && k.States.Has(a11y.Selectable) {
				return 1
			}
		}
		return 0
	}
	count := func() int {
		if len(n.Children) > 0 {
			if k := t.Node(n.Children[0]); k != nil && k.SetSize > len(n.Children) {
				return k.SetSize
			}
		}
		return len(n.Children)
	}
	switch {
	case listLike(n.Role):
		c.Rows, c.Cols = count(), 1
		c.Selection = selMode(n.Children)
	case n.Role == a11y.RoleTabList || n.Role == a11y.RoleMenuBar:
		c.Rows, c.Cols = 1, count()
		c.Selection = selMode(n.Children)
	case n.Role == a11y.RoleTree:
		var items []a11y.NodeID
		var walk func(id a11y.NodeID)
		walk = func(id a11y.NodeID) {
			k := t.Node(id)
			if k == nil {
				return
			}
			if k.Role == a11y.RoleTreeItem {
				items = append(items, id)
			}
			for _, g := range k.Children {
				walk(g)
			}
		}
		for _, k := range n.Children {
			walk(k)
		}
		c.Rows, c.Cols, c.Hierarchical = len(items), 1, true
		c.Selection = selMode(items)
	case n.Role == a11y.RoleTable || n.Role == a11y.RoleGrid:
		rows, cols := n.RowCount, n.ColCount
		if rows <= 0 || cols <= 0 {
			r, cmax := 0, 0
			t.Walk(func(k *a11y.Node, _ int) bool {
				if k.ID != n.ID && (k.Role == a11y.RoleTable || k.Role == a11y.RoleGrid) {
					return false // a nested table counts its own
				}
				if k.Role == a11y.RoleRow && t.Ancestor(k.ID, n.Role) == n {
					r++
					cmax = max(cmax, len(k.Children))
				}
				return true
			})
			if rows <= 0 {
				rows = r
			}
			if cols <= 0 {
				cols = cmax
			}
		}
		c.Rows, c.Cols = rows, cols
		if n.States.Has(a11y.Multiselectable) {
			c.Selection = 2
		}
	default:
		return c, false
	}
	return c, true
}

// AndroidCollectionItem returns the CollectionItemInfo of n, if n is an
// item of a collection: a child of a list, tab list or radio group, a tree
// item, or a table cell or header with row and column indices.
func AndroidCollectionItem(t *a11y.Tree, n *a11y.Node) (AndroidItem, bool) {
	it := AndroidItem{Selected: n.States.Has(a11y.Selected)}
	switch n.Role {
	case a11y.RoleCell, a11y.RoleColumnHeader, a11y.RoleRowHeader:
		if n.RowIndex <= 0 || n.ColIndex <= 0 {
			return it, false
		}
		it.Row, it.Col = n.RowIndex-1, n.ColIndex-1
		it.Heading = n.Role != a11y.RoleCell
		return it, true
	case a11y.RoleTreeItem:
		it.Row = posOrIndex(t, n)
		return it, true
	}
	p := t.Node(n.Parent)
	if p == nil {
		return it, false
	}
	switch {
	case listLike(p.Role):
		it.Row = posOrIndex(t, n)
	case p.Role == a11y.RoleTabList || p.Role == a11y.RoleMenuBar:
		it.Col = posOrIndex(t, n)
	default:
		return it, false
	}
	return it, true
}

func posOrIndex(t *a11y.Tree, n *a11y.Node) int {
	if n.PosInSet > 0 {
		return n.PosInSet - 1
	}
	return max(IndexInParent(t, n.ID), 0)
}

// ---------------------------------------------------------------------------
// Virtual view ids

// AndroidHostID is the virtual id of the root node: the canvas view itself
// (AccessibilityNodeProvider.HOST_VIEW_ID).
const AndroidHostID int32 = -1

// AndroidNoID is the id of no node (a hit test that missed, no focus).
const AndroidNoID int32 = math.MinInt32

// VirtualIDs maps node IDs (uint64) to Android virtual view ids (int32)
// and back. A node keeps its id while it is in the tree; the root is
// always AndroidHostID. The id of a removed node is not handed out again
// until it has been retired for MinUpdates updates and MinAge, so an
// accessibility service still holding it (TalkBack caches node infos)
// never reaches another node with it. The zero value is ready. Not safe
// for concurrent use.
type VirtualIDs struct {
	// MinUpdates and MinAge delay the reuse of a removed node's id
	// (defaults when zero: 4 updates and 30 seconds).
	MinUpdates int
	MinAge     time.Duration

	byNode  map[a11y.NodeID]int32
	byVID   map[int32]a11y.NodeID
	root    a11y.NodeID
	next    int32
	gen     int
	retired []retiredID // oldest first
}

type retiredID struct {
	vid int32
	gen int
	at  time.Time
}

func (v *VirtualIDs) init() {
	if v.byNode == nil {
		v.byNode = map[a11y.NodeID]int32{}
		v.byVID = map[int32]a11y.NodeID{}
	}
}

// Sync makes the mapping match tree t: nodes new to it get an id, nodes
// no longer in it are retired. A nil tree retires every node.
func (v *VirtualIDs) Sync(t *a11y.Tree, now time.Time) {
	v.init()
	v.gen++
	for id := range v.byNode {
		if t.Node(id) == nil {
			v.retire(id, now)
		}
	}
	v.setRoot(t)
	for _, n := range t.Nodes() {
		v.assign(n.ID, now)
	}
}

// Update applies the change list of one update (changed as
// CanvasAccessible.UpdateAccessibilityNodes gets it) to the mapping.
func (v *VirtualIDs) Update(t *a11y.Tree, changed []a11y.NodeID, now time.Time) {
	v.init()
	v.gen++
	v.setRoot(t)
	for _, id := range changed {
		if t.Node(id) == nil {
			v.retire(id, now)
		} else {
			v.assign(id, now)
		}
	}
}

func (v *VirtualIDs) setRoot(t *a11y.Tree) {
	r := a11y.NodeID(0)
	if t != nil && t.Node(t.Root) != nil {
		r = t.Root
	}
	if r == v.root {
		return
	}
	if v.root != 0 {
		delete(v.byNode, v.root)
	}
	v.root = r
	if r != 0 {
		if old, ok := v.byNode[r]; ok {
			// The new root was an ordinary node: free its id.
			delete(v.byVID, old)
			v.retired = append(v.retired, retiredID{old, v.gen, time.Time{}})
		}
		v.byNode[r] = AndroidHostID
	}
}

func (v *VirtualIDs) assign(id a11y.NodeID, now time.Time) {
	if _, ok := v.byNode[id]; ok || id == v.root {
		return
	}
	vid := v.alloc(now)
	v.byNode[id] = vid
	v.byVID[vid] = id
}

func (v *VirtualIDs) retire(id a11y.NodeID, now time.Time) {
	vid, ok := v.byNode[id]
	if !ok {
		return
	}
	delete(v.byNode, id)
	if id == v.root {
		v.root = 0
		return
	}
	delete(v.byVID, vid)
	v.retired = append(v.retired, retiredID{vid, v.gen, now})
}

func (v *VirtualIDs) alloc(now time.Time) int32 {
	minUpd, minAge := v.MinUpdates, v.MinAge
	if minUpd <= 0 {
		minUpd = 4
	}
	if minAge <= 0 {
		minAge = 30 * time.Second
	}
	if len(v.retired) > 0 {
		r := v.retired[0]
		if v.gen-r.gen >= minUpd && now.Sub(r.at) >= minAge || v.next == math.MaxInt32 {
			v.retired = v.retired[1:]
			return r.vid
		}
	}
	v.next++
	return v.next
}

// VID returns the virtual id of node id (AndroidHostID for the root).
func (v *VirtualIDs) VID(id a11y.NodeID) (int32, bool) {
	vid, ok := v.byNode[id]
	return vid, ok
}

// Node returns the node of virtual id vid (the root for AndroidHostID).
func (v *VirtualIDs) Node(vid int32) (a11y.NodeID, bool) {
	if vid == AndroidHostID {
		return v.root, v.root != 0
	}
	id, ok := v.byVID[vid]
	return id, ok
}

// Len returns the number of mapped nodes, the root included.
func (v *VirtualIDs) Len() int { return len(v.byNode) }

// ---------------------------------------------------------------------------
// Change events

// AndroidChange is a set of AccessibilityEvent content change types of
// TYPE_WINDOW_CONTENT_CHANGED; zero is CONTENT_CHANGE_TYPE_UNDEFINED.
type AndroidChange uint8

const (
	AndroidChangeSubtree            AndroidChange = 1 << iota // children added, removed or reordered
	AndroidChangeText                                         // text or hint
	AndroidChangeContentDescription                           // content description
	AndroidChangeStateDescription                             // state description, checked, selected, range value
	AndroidChangePaneTitle                                    // pane title (dialogs)
)

// AndroidEvent is one TYPE_WINDOW_CONTENT_CHANGED event to send.
type AndroidEvent struct {
	VID     int32
	Changes AndroidChange
}

// AndroidEvents returns the content-change events of one update, in tree
// order, one per virtual view (types merged). ids must already map next
// (VirtualIDs.Update). Added and removed nodes change their parent's
// subtree. A node that only moved (scrolling) changes nothing itself; one
// event with no type on the host reports that bounds moved (or nodes
// scrolled in or out of view). More than max
// events (max > 0) collapse into a subtree change of the host.
func AndroidEvents(prev, next *a11y.Tree, changed []a11y.NodeID, ids *VirtualIDs, max int) []AndroidEvent {
	var out []AndroidEvent
	at := map[int32]int{}
	add := func(id a11y.NodeID, c AndroidChange) {
		vid, ok := ids.VID(id)
		if !ok {
			return
		}
		if i, ok := at[vid]; ok {
			out[i].Changes |= c
			return
		}
		at[vid] = len(out)
		out = append(out, AndroidEvent{vid, c})
	}
	moved := false
	for _, id := range changed {
		n := next.Node(id)
		var p *a11y.Node
		if prev != nil {
			p = prev.Node(id)
		}
		ch := Diff(p, n)
		switch {
		case ch == 0:
			continue
		case ch.Has(ChangeRemoved):
			if p != nil && next.Node(p.Parent) != nil {
				add(p.Parent, AndroidChangeSubtree)
			}
			continue
		case ch.Has(ChangeAdded):
			if next.Node(n.Parent) != nil {
				add(n.Parent, AndroidChangeSubtree)
			}
			continue
		}
		var c AndroidChange
		movedHere := ch.Has(ChangeBounds)
		if ch.Has(ChangeChildren) {
			c |= AndroidChangeSubtree
		}
		if ch.Has(ChangeName) {
			s0, s1 := AndroidText(p), AndroidText(n)
			if s0.Text != s1.Text || s0.Hint != s1.Hint || s0.Error != s1.Error {
				c |= AndroidChangeText
			}
			if s0.ContentDescription != s1.ContentDescription {
				c |= AndroidChangeContentDescription
			}
			if s0.PaneTitle != s1.PaneTitle {
				c |= AndroidChangePaneTitle
			}
		}
		if ch.Has(ChangeValue) {
			if isTextInput(n) {
				c |= AndroidChangeText
			} else {
				c |= AndroidChangeStateDescription
			}
		}
		if ch.Has(ChangeStates) {
			if (p.States^n.States)&^a11y.Offscreen != 0 {
				c |= AndroidChangeStateDescription
			} else {
				movedHere = true // scrolled in or out of view: visibility only
			}
		}
		moved = moved || movedHere
		if c == 0 && !movedHere {
			// Role, relations, positions, actions: nothing more precise.
			add(id, 0)
			continue
		}
		if c != 0 {
			add(id, c)
		}
	}
	if max > 0 && len(out) > max {
		return []AndroidEvent{{AndroidHostID, AndroidChangeSubtree}}
	}
	if moved {
		if _, ok := at[AndroidHostID]; !ok {
			out = append(out, AndroidEvent{AndroidHostID, 0})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Packed node records (Go to Java, CanvasA11y.java)

// AndroidRecordVersion is the first byte of a packed node record.
const AndroidRecordVersion = 1

// Record flags.
const (
	androidRecRange      = 1 << 0
	androidRecCollection = 1 << 1
	androidRecItem       = 1 << 2
)

// PackAndroidNode encodes node id of t for CanvasA11y.java, which builds
// the AccessibilityNodeInfo from it; nil when the node is not in t or ids.
// The layout, all little-endian:
//
//	u8  version (AndroidRecordVersion)
//	i32 virtual id, i32 parent's virtual id (AndroidNoID for the root)
//	u32 AndroidState, u16 AndroidAction, u8 live region, u8 flags
//	f32 x, y, w, h: Node.Bounds, logical pixels relative to the canvas
//	flags&1: u8 range type, f32 min, max, current
//	flags&2: i32 rows, cols, u8 hierarchical, u8 selection mode
//	flags&4: i32 row, col, u8 heading, u8 selected
//	i32 n, n × i32 children's virtual ids, in reading order
//	8 strings (i32 byte length + UTF-8): class, text, content description,
//	  state description, hint, error, pane title, role description
func PackAndroidNode(t *a11y.Tree, id a11y.NodeID, ids *VirtualIDs) []byte {
	n := t.Node(id)
	if n == nil {
		return nil
	}
	vid, ok := ids.VID(id)
	if !ok {
		return nil
	}
	parent := AndroidNoID
	if id != t.Root {
		if p, ok := ids.VID(n.Parent); ok {
			parent = p
		}
	}
	coll, isColl := AndroidCollectionInfo(t, n)
	item, isItem := AndroidCollectionItem(t, n)
	var flags uint8
	if n.HasRange {
		flags |= androidRecRange
	}
	if isColl {
		flags |= androidRecCollection
	}
	if isItem {
		flags |= androidRecItem
	}
	b := make([]byte, 0, 96+8*len(n.Children)+2*len(n.Name))
	le := binary.LittleEndian
	i32 := func(v int32) { b = le.AppendUint32(b, uint32(v)) }
	f32 := func(v float64) { b = le.AppendUint32(b, math.Float32bits(float32(v))) }
	u8 := func(v bool) {
		if v {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	str := func(s string) {
		i32(int32(len(s)))
		b = append(b, s...)
	}
	b = append(b, AndroidRecordVersion)
	i32(vid)
	i32(parent)
	b = le.AppendUint32(b, uint32(AndroidStates(n)))
	b = le.AppendUint16(b, uint16(AndroidActions(n)))
	b = append(b, uint8(AndroidLive(n)), flags)
	f32(float64(n.Bounds.X))
	f32(float64(n.Bounds.Y))
	f32(float64(n.Bounds.W))
	f32(float64(n.Bounds.H))
	if n.HasRange {
		typ, lo, hi, cur := AndroidRange(n)
		b = append(b, uint8(typ))
		f32(lo)
		f32(hi)
		f32(cur)
	}
	if isColl {
		i32(int32(coll.Rows))
		i32(int32(coll.Cols))
		u8(coll.Hierarchical)
		b = append(b, uint8(coll.Selection))
	}
	if isItem {
		i32(int32(item.Row))
		i32(int32(item.Col))
		u8(item.Heading)
		u8(item.Selected)
	}
	kids := make([]int32, 0, len(n.Children))
	for _, k := range n.Children {
		if t.Node(k) == nil {
			continue
		}
		if kv, ok := ids.VID(k); ok {
			kids = append(kids, kv)
		}
	}
	i32(int32(len(kids)))
	for _, k := range kids {
		i32(k)
	}
	s := AndroidText(n)
	for _, v := range []string{AndroidClass(n), s.Text, s.ContentDescription, s.StateDescription, s.Hint, s.Error, s.PaneTitle, s.RoleDescription} {
		str(v)
	}
	return b
}

// AndroidHit returns the virtual id of the node at (x, y) logical pixels
// relative to the canvas (HitTest), or AndroidNoID.
func AndroidHit(t *a11y.Tree, ids *VirtualIDs, x, y float32) int32 {
	id := HitTest(t, x, y)
	if id == 0 {
		return AndroidNoID
	}
	if vid, ok := ids.VID(id); ok {
		return vid
	}
	return AndroidNoID
}
