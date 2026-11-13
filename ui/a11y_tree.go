package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Accessibility (semantics tree).
//
// Widgets annotate the nodes they create with a role, an accessible name
// and states (see AUTHORING.md section 17). When a host asks for it
// (AccessibilityHost), the runner derives an a11y.Tree from the annotated
// node tree after layout, compares it with the previous one and reports the
// changed node IDs. With accessibility off nothing beyond a few field
// stores per widget is spent: the tree is never built.

// AccessibilityHost is an optional interface of Window. A platform
// backend implements it to receive the semantics tree and expose it through
// the native accessibility API (NSAccessibility, UI Automation, AT-SPI,
// UIAccessibility, AccessibilityNodeInfo); docs/accessibility.md has the
// role mapping and the expected update protocol.
//
// The runner calls AccessibilityEnabled at the start of every frame (it must
// be cheap) and, while it returns true, AccessibilityUpdate after every
// frame whose tree differs from the previous one. When an assistive
// technology attaches, the host flips its flag and calls Runner.Invalidate
// so the first full tree (every node reported as changed) arrives promptly.
// Both methods run on the UI thread. The tree must not be modified; it
// stays valid after the call (the runner builds a new one each frame).
type AccessibilityHost interface {
	AccessibilityEnabled() bool
	// AccessibilityUpdate delivers the tree of the last frame. changed
	// lists added and modified nodes and then removed ones (tree.Node
	// returns nil for those). tree.Focus is the focused node and
	// tree.Announcements the messages to speak once.
	AccessibilityUpdate(tree *a11y.Tree, changed []a11y.NodeID)
}

// semFiller completes a node's semantics while the tree is built (after
// layout, so chained modifiers have run): values, ranges, relations,
// descriptions. Widget builders implement it and assign themselves to
// node.aFill, which costs nothing when accessibility is off.
type semFiller interface{ semantics(s *a11y.Node) }

// semFunc adapts a function to semFiller. Creating one allocates a
// closure: in per-row code guard it with c.a11yOn.
type semFunc func(s *a11y.Node)

func (f semFunc) semantics(s *a11y.Node) { f(s) }

// setRole marks n as a semantic node of role r. The accessible name
// defaults to n.label; override it with setName when the label is an
// internal key.
func (n *node) setRole(r a11y.Role) *node { n.role = r; return n }

// setName sets the accessible name (texts come from the Locale).
func (n *node) setName(s string) *node { n.aName, n.aNameSet = s, true; return n }

// setState adds states (Checked, Selected, Expandable|Expanded …).
func (n *node) setState(s a11y.State) *node { n.aState |= s; return n }

// stateIf adds s when on.
func (n *node) stateIf(on bool, s a11y.State) *node {
	if on {
		n.aState |= s
	}
	return n
}

// fill sets the late semantics filler (chained after an existing one).
func (n *node) fill(f semFiller) *node {
	if n.aFill != nil {
		prev := n.aFill
		n.aFill = semFunc(func(s *a11y.Node) { prev.semantics(s); f.semantics(s) })
		return n
	}
	n.aFill = f
	return n
}

// A11yEnabled reports whether the semantics tree is built this frame (a
// host or the Driver asked for it). Widgets may skip work that only feeds
// the tree (formatting descriptions, closures) when it is false.
func (c *Context) A11yEnabled() bool { return c.a11yOn }

// Announce asks the screen reader to speak text once (a polite or
// assertive announcement not tied to a node, e.g. "3 results"). It is
// ignored when accessibility is off.
func (c *Context) Announce(text string, live a11y.Live) {
	if !c.a11yOn || text == "" {
		return
	}
	a := a11y.Announcement{Text: text, Live: live}
	for _, x := range c.announce {
		if x == a {
			return // built again in another pass of the same frame
		}
	}
	c.announce = append(c.announce, a)
}

// semOverride is an annotation made with Context.Semantics.
type semOverride struct {
	role   a11y.Role
	name   string
	states a11y.State
}

// Semantics annotates the widget id (built this frame) with a role, an
// accessible name and states, overriding what the widget declared; use it
// for custom widgets built from Div/Custom or to rename a control:
//
//	b := ui.Custom(c, measure, paint)
//	c.Semantics(b.ID(), a11y.RoleImage, loc.Logo)
//
// An empty name keeps the widget's own name. It costs nothing when
// accessibility is off.
func (c *Context) Semantics(id ID, role a11y.Role, name string, states ...a11y.State) {
	if !c.a11yOn {
		return
	}
	o := semOverride{role: role, name: name}
	for _, s := range states {
		o.states |= s
	}
	if c.semOver == nil {
		c.semOver = map[ID]semOverride{}
	}
	c.semOver[id] = o
}

// Semantics sets the box's role and accessible name (an empty name is
// computed from the text inside for roles such as buttons and cells).
func (b *Box) Semantics(role a11y.Role, name string) *Box {
	b.n.role = role
	if name != "" {
		b.n.setName(name)
	}
	return b
}

// A11yState adds accessibility states to the box (Selected, Expanded …).
func (b *Box) A11yState(s a11y.State) *Box { b.n.aState |= s; return b }

// A11y sets a function that completes the box's semantics node
// (description, value, range, relations) while the tree is built. fn runs
// only when accessibility is on.
func (b *Box) A11y(fn func(s *a11y.Node)) *Box { b.n.fill(semFunc(fn)); return b }

// A11yHidden hides the box and its subtree from the semantics tree
// (decoration, or content another node already describes).
func (b *Box) A11yHidden() *Box { b.n.aHidden = true; return b }

// A11yKeyboard declares that a pointer-only box has a keyboard equivalent
// elsewhere (arrow keys of its composite, a focusable twin), so Audit does
// not report it as unreachable.
func (b *Box) A11yKeyboard() *Box { b.n.aKbd = true; return b }

// A11yScroller declares that the box is a scroll container that moves its
// content itself (a touch scroll view with its own scroller) rather than a
// ui scroll container. Semantic nodes inside it get the ScrollIntoView
// action, and the container reads a request for it with
// Context.A11yScrollRequest and scrolls the widget into view. Such a
// container normally clips (Clip), which also marks content scrolled out
// of it Offscreen.
func (b *Box) A11yScroller() *Box { b.n.aScroll = true; return b }

// Focusable makes the box a keyboard focus stop (Tab, focus ring state,
// Enter / Space activate it) and returns its interaction, like Interact.
func (b *Box) Focusable(cursor input.Cursor) Interaction {
	if b.ia == nil || !b.n.focus {
		ia := b.c.interact(b.n, true, cursor)
		b.ia = &ia
	}
	return *b.ia
}

// A11yAction returns an accessibility action requested for id this frame
// (Runner.PerformAction): widgets whose activation does not go through
// Interact (gesture areas, custom hit testing) read it to react to Press,
// Expand, SetValue … Press is also delivered as a click to Interact.
func (c *Context) A11yAction(id ID) (a11y.Action, string, bool) {
	for _, a := range c.actNow {
		if a.id == id {
			return a.act, a.arg, true
		}
	}
	return 0, "", false
}

// A11yScrollRequest returns the widget a screen reader asked to scroll
// into view this frame (the ScrollIntoView action) when container is the
// nearest scroll container around it that is not a ui scroll container
// (Box.A11yScroller); the container scrolls the minimum amount that
// shows the widget's rectangle of the last frame (Context.RectOf) inside
// its visible part. ui scroll containers further out are scrolled by the
// runner itself.
func (c *Context) A11yScrollRequest(container ID) (ID, bool) {
	for _, a := range c.actNow {
		if a.act != a11y.ScrollIntoView {
			continue
		}
		n := c.a11yNodes[a.id]
		if n == nil {
			continue
		}
		for p := a11yScrollAncestor(n); p != nil; p = a11yScrollAncestor(p) {
			if p.aScroll {
				if p.id == container {
					return a.id, true
				}
				break
			}
		}
	}
	return 0, false
}

// a11yScrollAncestor returns the nearest ancestor of n that scrolls
// vertically: a ui scroll container or a box that scrolls its content
// itself (A11yScroller).
func a11yScrollAncestor(n *node) *node {
	for p := n.parent; p != nil; p = p.parent {
		if p.aScroll || p.scroll != nil && p.ln.ScrollY {
			return p
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Field labels: FormItem passes its label, required flag and validation
// message to the controls inside it.

// fieldInfo is implemented by fillers of nodes that label the controls
// inside them (FormItem, ProForm fields).
type fieldInfo interface {
	field() (label string, labelID ID, required bool, err string)
}

type fieldCtx struct {
	label    string
	labelID  ID
	required bool
	err      string
	used     bool
}

// fieldRole reports roles that take their name from an enclosing label.
func fieldRole(r a11y.Role) bool {
	switch r {
	case a11y.RoleTextbox, a11y.RoleSearchbox, a11y.RoleSpinButton, a11y.RoleCombobox, a11y.RoleSlider,
		a11y.RoleSwitch, a11y.RoleListbox, a11y.RoleRadioGroup, a11y.RoleCheckbox, a11y.RoleTree, a11y.RoleGroup,
		a11y.RoleGrid:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Tree building

type a11yBuilder struct {
	c      *Context
	t      *a11y.Tree
	win    geom.Rect
	nodes  map[ID]*node // semantic nodes of this frame (actions)
	parent map[ID]*node
	// focusAlias is the node reporting focus held by a hidden part.
	focusAlias *a11y.Node
}

// subtreeHas reports whether id is n or one of its descendants.
func subtreeHas(n *node, id ID) bool {
	if n.id == id {
		return true
	}
	for _, k := range n.kids {
		if subtreeHas(k, id) {
			return true
		}
	}
	return false
}

func toA11yRect(r geom.Rect) a11y.Rect { return a11y.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }

// layerRoots returns the root of every layer painted this frame (page
// first) with its layer number.
func (c *Context) layerRoots() []*node {
	out := make([]*node, 0, len(c.overlays)+1)
	out = append(out, c.root)
	for _, o := range c.overlays {
		out = append(out, o.root)
	}
	return out
}

// modalLayer returns the highest layer holding an open focus-trapping
// panel (modal dialog, drawer) this frame; 0 when none.
func (c *Context) modalLayer() int {
	top := 0
	for _, tr := range c.traps {
		if l := c.overlayIndex(tr); l > top {
			top = l
		}
	}
	return top
}

// overlayIndex returns the layer of the overlay root id in the current
// build (1-based), or 0.
func (c *Context) overlayIndex(id ID) int {
	for i, o := range c.overlays {
		if o.root.id == id {
			return i + 1
		}
	}
	return 0
}

// nodeRTL is the direction of the scope n was built in (the window's root
// scope for nodes without a recorded one).
func (c *Context) nodeRTL(n *node) bool {
	if n.env != nil {
		return n.env.rtl
	}
	return c.envRoot.rtl
}

// buildA11yTree derives the semantics tree of the frame just painted.
func (c *Context) buildA11yTree() *a11y.Tree {
	b := &a11yBuilder{c: c, win: geom.Rect{W: c.size.W, H: c.size.H}, nodes: map[ID]*node{}, parent: map[ID]*node{}}
	rootID := a11y.NodeID(c.root.id)
	b.t = a11y.NewTree(rootID)
	b.t.RTL = c.envRoot.rtl
	root := &a11y.Node{ID: rootID, Role: a11y.RoleWindow, Bounds: toA11yRect(b.win), RTL: c.envRoot.rtl}
	b.t.Add(root)
	b.nodes[c.root.id] = c.root
	modal := c.modalLayer()
	for i, ln := range c.layerRoots() {
		if i < modal {
			continue // under a modal dialog: inert, not exposed
		}
		if i > 0 {
			// Overlay roots are exposed only when they carry a role (a
			// tooltip or menu panel); otherwise their content joins the root.
			b.visit0(ln, root, nil, b.win)
			continue
		}
		for _, k := range ln.kids {
			b.visit0(k, root, nil, b.win)
		}
	}
	if _, ok := b.nodes[c.focus]; ok && c.focus != 0 {
		b.t.Focus = a11y.NodeID(c.focus)
	} else if b.focusAlias != nil && b.focusAlias.ID != rootID {
		b.t.Focus = b.focusAlias.ID
		b.focusAlias.States |= a11y.Focused
	}
	b.t.Announcements = append([]a11y.Announcement(nil), c.announce...)
	c.a11yNodes = b.nodes
	c.a11yParent = b.parent
	return b.t
}

// interactiveUnannotated reports a pointer target without a role: it is
// exposed as a button so it is at least reachable (Audit reports it).
func interactiveUnannotated(n *node) bool {
	return n.role == a11y.RoleNone && n.hit && (n.focus || n.cursor == input.CursorPointer) && !n.blocks
}

// visit0 adds n (when it has a role) and its subtree under parent. clip
// is the visible part of the window left by the clipping and scrolling
// ancestors: nodes entirely outside it are Offscreen.
func (b *a11yBuilder) visit0(n *node, parent *a11y.Node, f *fieldCtx, clip geom.Rect) {
	c := b.c
	r, painted := c.rects[n.id]
	if !painted {
		return
	}
	offscreen := r.Intersect(clip).Empty()
	if n.clip || n.scroll != nil {
		clip = clip.Intersect(r) // what the subtree can show
	}
	if n.aHidden {
		// Part of the parent's semantics (a combobox's search editor): focus
		// inside it is reported on the parent.
		if c.focus != 0 && subtreeHas(n, c.focus) {
			b.focusAlias = parent
		}
		return
	}
	role := n.role
	if role == a11y.RoleNone && interactiveUnannotated(n) {
		role = a11y.RoleButton
	}
	var over *semOverride
	if o, ok := c.semOver[n.id]; ok {
		over = &o
		if o.role != a11y.RoleNone {
			role = o.role
		}
	}
	if fi, ok := n.aFill.(fieldInfo); ok {
		label, lid, req, err := fi.field()
		f = &fieldCtx{label: label, labelID: lid, required: req, err: err}
	}
	if role == a11y.RoleNone {
		for _, k := range n.kids {
			b.visit0(k, parent, f, clip)
		}
		return
	}
	an := &a11y.Node{ID: a11y.NodeID(n.id), Parent: parent.ID, Role: role, Bounds: toA11yRect(r), RTL: c.nodeRTL(n)}
	if n.aNameSet {
		an.Name = n.aName
	} else {
		an.Name = n.label
	}
	if over != nil && over.name != "" {
		an.Name = over.name
	}
	st := n.aState
	if over != nil {
		st |= over.states
	}
	disabled := n.disabled || n.cursor == input.CursorNotAllowed
	if disabled {
		st |= a11y.Disabled
	}
	if n.focus && !disabled {
		st |= a11y.Focusable
		if c.focus == n.id {
			st |= a11y.Focused
		}
	}
	if offscreen {
		st |= a11y.Offscreen
	}
	an.States = st
	// Default actions; fillers may add or remove.
	var act a11y.Action
	if st.Has(a11y.Focusable) {
		act |= a11y.Focus
	}
	if !disabled {
		if n.hit && (n.cursor == input.CursorPointer || n.focus) || n.aTarget != 0 {
			switch role {
			case a11y.RoleButton, a11y.RoleLink, a11y.RoleCheckbox, a11y.RoleRadio, a11y.RoleSwitch, a11y.RoleOption,
				a11y.RoleMenuItem, a11y.RoleTab, a11y.RoleTreeItem, a11y.RoleCombobox, a11y.RoleCell, a11y.RoleRow,
				a11y.RoleColumnHeader, a11y.RoleListItem, a11y.RoleImage, a11y.RoleGroup:
				act |= a11y.Press
			}
		}
		if st.Has(a11y.Expandable) {
			if st.Has(a11y.Expanded) {
				act |= a11y.Collapse
			} else {
				act |= a11y.Expand
			}
		}
		if st.Has(a11y.Focusable) {
			switch role {
			case a11y.RoleSlider, a11y.RoleSpinButton:
				act |= a11y.Increment | a11y.Decrement
			}
			switch role {
			case a11y.RoleTextbox, a11y.RoleSearchbox, a11y.RoleSpinButton:
				if !st.Has(a11y.ReadOnly) {
					act |= a11y.SetValue
				}
			}
		}
	}
	if a11yScrollAncestor(n) != nil {
		act |= a11y.ScrollIntoView
	}
	an.Actions = act
	b.t.Add(an)
	parent.Children = append(parent.Children, an.ID)
	b.nodes[n.id] = n
	if pn, ok := b.nodes[ID(parent.ID)]; ok {
		b.parent[n.id] = pn
	}
	// Field label for the first control inside a form item.
	if f != nil && fieldRole(role) && role != a11y.RoleGroup && !f.used {
		f.used = true
		if an.Name == "" {
			an.Name = f.label
			an.LabelledBy = a11y.NodeID(f.labelID)
		}
		if f.required {
			an.States |= a11y.Required
		}
		if f.err != "" {
			an.States |= a11y.Invalid
			if an.Description == "" {
				an.Description = f.err
			}
		}
	}
	for _, k := range n.kids {
		b.visit0(k, an, f, clip)
	}
	if n.aFill != nil {
		n.aFill.semantics(an)
		// The filler may have set Expanded: offer the matching action.
		if an.States.Has(a11y.Expandable) && an.Actions&(a11y.Expand|a11y.Collapse) != 0 {
			an.Actions &^= a11y.Expand | a11y.Collapse
			if an.States.Has(a11y.Expanded) {
				an.Actions |= a11y.Collapse
			} else {
				an.Actions |= a11y.Expand
			}
		}
	}
	if an.Live == a11y.LiveOff {
		switch role {
		case a11y.RoleAlert:
			an.Live = a11y.LiveAssertive
		case a11y.RoleStatus:
			an.Live = a11y.LivePolite
		}
	}
	if role.Atomic() {
		// Plain text inside a button / option is its name, not a child.
		kept := an.Children[:0]
		var text []string
		for _, id := range an.Children {
			k := b.t.Node(id)
			if k != nil && (k.Role == a11y.RoleText || k.Role == a11y.RoleParagraph) && len(k.Children) == 0 {
				if k.Name != "" {
					text = append(text, k.Name)
				}
				b.t.Remove(id)
				delete(b.nodes, ID(id))
				continue
			}
			kept = append(kept, id)
		}
		an.Children = kept
		if an.Name == "" && len(text) > 0 {
			an.Name = strings.Join(text, " ")
		}
	}
	if an.Name == "" && role.NameFromContent() {
		an.Name = b.t.Text(an.ID)
	}
	if item := setItemRole(role); item != a11y.RoleNone {
		numberSet(b.t, an, item)
	}
	if an.States.Has(a11y.Protected) {
		an.Value = ""
	}
}

// ---------------------------------------------------------------------------
// Actions

type a11yAction struct {
	id  ID
	act a11y.Action
	arg string
}

// PerformAction asks the widget behind node id to perform act (from a
// screen reader, voice control or a test). arg is the new value for
// SetValue (text, or a number for sliders and spin buttons). The node and
// action must be in the last reported tree; the action runs at the start
// of the next frame, which this call requests. It must be called on the UI
// thread.
func (r *Runner) PerformAction(id a11y.NodeID, act a11y.Action, arg string) error {
	t := r.a11yTree
	if t == nil {
		return fmt.Errorf("ui: accessibility is not enabled")
	}
	n := t.Node(id)
	if n == nil {
		return fmt.Errorf("ui: no accessibility node %d", id)
	}
	if !n.Actions.Has(act) {
		return fmt.Errorf("ui: %s does not support %s", n, act)
	}
	r.actions = append(r.actions, a11yAction{id: ID(id), act: act, arg: arg})
	r.Invalidate()
	return nil
}

// applyAction turns a queued accessibility action into the input the
// widget already understands (a click, focus, keys, a value).
func (c *Context) applyAction(a a11yAction) {
	n := c.a11yNodes[a.id]
	if n == nil {
		return
	}
	c.actNow = append(c.actNow, a)
	target := a.id
	if n.aTarget != 0 {
		target = n.aTarget
	}
	focus := func() {
		if n.focus {
			c.focus = a.id
			c.focusVisible = true
		}
	}
	key := func(k input.Key) {
		if !n.focus {
			return // not focusable: the widget reads A11yAction instead
		}
		focus()
		c.keys = append(c.keys, input.KeyEvent{Key: k, Code: k, Down: true}, input.KeyEvent{Key: k, Code: k})
	}
	switch a.act {
	case a11y.Focus:
		focus()
	case a11y.Press, a11y.Expand, a11y.Collapse:
		if n.focus {
			c.focus = a.id
		}
		c.clicked[target] = 1
		c.lastClick, c.lastClickID = c.now, target
		if r, ok := c.rects[target]; ok {
			c.pressPos = r.Center() // dialogs zoom from the activated control
		}
	case a11y.Increment, a11y.Decrement:
		inc := a.act == a11y.Increment
		switch {
		case n.role == a11y.RoleSlider && inc:
			key(input.KeyRight)
		case n.role == a11y.RoleSlider:
			key(input.KeyLeft)
		case inc:
			key(input.KeyUp)
		default:
			key(input.KeyDown)
		}
	case a11y.SetValue:
		focus()
		if c.setValues == nil {
			c.setValues = map[ID]string{}
		}
		c.setValues[a.id] = a.arg
	case a11y.ScrollIntoView:
		// Scroll containers that move their content themselves read the
		// request during the build (A11yScrollRequest).
		for p := a11yScrollAncestor(n); p != nil; p = a11yScrollAncestor(p) {
			if p.scroll != nil {
				c.ScrollIntoView(p.id, a.id)
			}
		}
	case a11y.Dismiss:
		c.escape = true
	}
}

// takeSetValue returns (and consumes) a SetValue request for id.
func (c *Context) takeSetValue(id ID) (string, bool) {
	if c.setValues == nil {
		return "", false
	}
	s, ok := c.setValues[id]
	if ok {
		delete(c.setValues, id)
	}
	return s, ok
}

// takeSetNumber is takeSetValue for numeric widgets.
func (c *Context) takeSetNumber(id ID) (float64, bool) {
	s, ok := c.takeSetValue(id)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil
}

// ---------------------------------------------------------------------------
// Helpers for widgets

// rangeSem fills a range node (slider, progress, spin button).
func rangeSem(s *a11y.Node, now, lo, hi, step float64) {
	s.HasRange = true
	s.ValueNow, s.ValueMin, s.ValueMax, s.Step = now, lo, hi, step
	if s.Value == "" {
		s.Value = strconv.FormatFloat(now, 'f', -1, 64)
	}
}

// setItemRole returns the item role numbered inside a set container.
func setItemRole(r a11y.Role) a11y.Role {
	switch r {
	case a11y.RoleListbox:
		return a11y.RoleOption
	case a11y.RoleMenu, a11y.RoleMenuBar:
		return a11y.RoleMenuItem
	case a11y.RoleTabList:
		return a11y.RoleTab
	case a11y.RoleRadioGroup:
		return a11y.RoleRadio
	case a11y.RoleList:
		return a11y.RoleListItem
	case a11y.RoleTree, a11y.RoleTreeItem:
		return a11y.RoleTreeItem
	}
	return a11y.RoleNone
}

// numberSet fills PosInSet / SetSize of the children of s with role item
// that the widget did not number itself.
func numberSet(t *a11y.Tree, s *a11y.Node, item a11y.Role) {
	var ks []*a11y.Node
	for _, id := range s.Children {
		if k := t.Node(id); k != nil && k.Role == item {
			ks = append(ks, k)
		}
	}
	for i, k := range ks {
		if k.SetSize == 0 {
			k.PosInSet, k.SetSize = i+1, len(ks)
		}
	}
}
