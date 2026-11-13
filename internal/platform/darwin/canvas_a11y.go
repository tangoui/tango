//go:build darwin && !ios

package darwin

// Accessibility bridge of canvas views (docs/specs/a11y-bridge.md): the
// TangoCanvasView stands for the root of the semantics tree (an AXGroup),
// and every other node is a TangoA11yElement, an NSAccessibilityElement
// subclass whose NSAccessibility protocol methods read the node from the
// last tree. Roles, states and actions follow a11y.Mappings and
// docs/accessibility.md (via internal/platform/a11ybridge).
//
// Activation: the first time AppKit asks the view for its children, hit
// test or focused element (an assistive technology is reading the
// window), AccessibilityActive turns true and a frame is requested; the
// core then sends the tree and the view posts AXLayoutChanged so the
// client reads it again. There is no reliable "assistive technology gone"
// signal, so the bridge stays active until the window closes.
//
// Everything runs on the main thread: AppKit serves accessibility
// requests there, and the core calls CanvasAccessible from CanvasFrame.
// Frames are computed when asked (accessibilityFrame), so moving the
// window or changing screens needs no new tree.

import (
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

var _ platform.CanvasAccessible = (*canvas)(nil)

// canvasA11y is the accessibility state of one canvas. Main thread, except
// active.
type canvasA11y struct {
	c      *canvas
	active atomic.Bool
	tree   *a11y.Tree
	elems  map[a11y.NodeID]id // TangoA11yElement objects (retained)
	focus  a11y.NodeID        // effective focus from the core
	// axAsked is the node an assistive technology just asked to focus
	// (setAccessibilityFocused:). Its focus change is announced even when
	// the window is not key (display asleep, app in the background): the
	// requester waits for AXFocusedUIElementChanged to confirm.
	axAsked a11y.NodeID
}

type elemRef struct {
	ax *canvasA11y
	id a11y.NodeID
}

var (
	a11yElems = map[id]elemRef{} // element -> node; main thread
	a11yOnce  sync.Once

	axPost         func(element, name id)
	axPostInfo     func(element, name, info id)
	axRoleDesc     func(role, subrole id) id
	msgPointArg    func(obj id, s objc.SEL, p nsPoint) nsPoint
	msgNumberFloat func(cls id, s objc.SEL, v float64) id
)

// Notification names and keys (their values are their AppKit names).
const (
	axFocusedChanged   = "AXFocusedUIElementChanged"
	axValueChanged     = "AXValueChanged"
	axTitleChanged     = "AXTitleChanged"
	axLayoutChanged    = "AXLayoutChanged"
	axDestroyed        = "AXUIElementDestroyed"
	axSelectedChildren = "AXSelectedChildrenChanged"
	axAnnouncement     = "AXAnnouncementRequested"
	axAnnouncementKey  = "AXAnnouncementKey"
	axPriorityKey      = "AXPriorityKey"
	axPriorityMedium   = 50 // NSAccessibilityPriorityMedium
	axPriorityHigh     = 90 // NSAccessibilityPriorityHigh
)

func loadA11y() {
	a11yOnce.Do(func() {
		purego.RegisterLibFunc(&axPost, libAppKit, "NSAccessibilityPostNotification")
		purego.RegisterLibFunc(&axPostInfo, libAppKit, "NSAccessibilityPostNotificationWithUserInfo")
		purego.RegisterLibFunc(&axRoleDesc, libAppKit, "NSAccessibilityRoleDescription")
		purego.RegisterFunc(&msgPointArg, msgSendAddr)
		purego.RegisterFunc(&msgNumberFloat, msgSendAddr)
		registerA11yElement()
	})
}

// ax returns the canvas's accessibility state, creating it on first use.
func (c *canvas) a11yState() *canvasA11y {
	if c.ax == nil {
		c.ax = &canvasA11y{c: c, elems: map[a11y.NodeID]id{}}
	}
	return c.ax
}

// activate marks the canvas as read by an assistive technology.
func (ax *canvasA11y) activate() {
	if !ax.active.Swap(true) && !ax.c.closed {
		ax.c.Invalidate()
	}
}

// ---- platform.CanvasAccessible ----

func (c *canvas) AccessibilityActive() bool {
	ax := c.ax
	return ax != nil && ax.active.Load()
}

func (c *canvas) SetAccessibilityTree(tree *a11y.Tree) {
	if c.closed {
		return
	}
	ax := c.a11yState()
	if tree == nil {
		ax.dropAll()
		ax.tree, ax.focus = nil, 0
		return
	}
	ax.dropAll()
	ax.tree = tree
	ax.post(c.view, axLayoutChanged)
}

func (c *canvas) UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID) {
	if c.closed {
		return
	}
	ax := c.a11yState()
	prev := ax.tree
	ax.tree = tree
	layout := false
	for _, nid := range changed {
		n := tree.Node(nid)
		if n == nil {
			ax.drop(nid)
			layout = true
			continue
		}
		el := ax.elems[nid]
		target := el
		if nid == tree.Root {
			target = c.view
		}
		ch := a11ybridge.Diff(prev.Node(nid), n)
		if ch.Has(a11ybridge.ChangeAdded) || ch.Has(a11ybridge.ChangeChildren) {
			layout = true
		}
		if target == 0 {
			continue // never handed out: nobody listens to it
		}
		if ch.Has(a11ybridge.ChangeValue) || (ch.Has(a11ybridge.ChangeStates) && a11ybridge.MacToggle(n)) {
			ax.post(target, axValueChanged)
		}
		if ch.Has(a11ybridge.ChangeName) {
			ax.post(target, axTitleChanged)
		}
		if ch.Has(a11ybridge.ChangeStates) && (prev.Node(nid).States ^ n.States).Has(a11y.Selected) {
			if p := ax.elemOrView(n.Parent); p != 0 {
				ax.post(p, axSelectedChildren)
			}
		}
		if ch.Has(a11ybridge.ChangeChildren) {
			ax.post(target, axLayoutChanged)
		}
	}
	if layout {
		ax.post(c.view, axLayoutChanged)
	}
}

func (c *canvas) AccessibilityFocusChanged(nid a11y.NodeID) {
	if c.closed {
		return
	}
	ax := c.a11yState()
	ax.focus = nid
	asked := nid != 0 && ax.axAsked == nid
	ax.axAsked = 0
	if !c.focused && !asked {
		return // another view (the WebView) or window has keyboard focus
	}
	if el := ax.elemOrView(nid); el != 0 {
		ax.post(el, axFocusedChanged)
	}
}

func (c *canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	if c.closed || text == "" {
		return
	}
	prio := axPriorityMedium
	if live == a11y.LiveAssertive {
		prio = axPriorityHigh
	}
	withPool(func() {
		keys := []id{axConst("NSAccessibilityAnnouncementKey", axAnnouncementKey), axConst("NSAccessibilityPriorityKey", axPriorityKey)}
		vals := []id{nsString(text), send(class("NSNumber"), "numberWithInteger:", uintptr(prio))}
		info := send(class("NSDictionary"), "dictionaryWithObjects:forKeys:count:",
			uintptr(unsafe.Pointer(&vals[0])), uintptr(unsafe.Pointer(&keys[0])), 2)
		app := send(class("NSApplication"), "sharedApplication")
		axPostInfo(app, axConst("NSAccessibilityAnnouncementRequestedNotification", axAnnouncement), info)
	})
}

// a11yDestroy releases the elements; called from destroy.
func (c *canvas) a11yDestroy() {
	if c.ax == nil {
		return
	}
	c.ax.dropAll()
	c.ax.tree = nil
	c.ax.active.Store(false)
}

// ---- elements ----

func (ax *canvasA11y) post(el id, name string) {
	if el == 0 {
		return
	}
	withPool(func() { axPost(el, nsString(name)) })
}

// elem returns the element of node nid, creating it; 0 for the root
// (the view) or a node not in the tree.
func (ax *canvasA11y) elem(nid a11y.NodeID) id {
	if ax.tree == nil || nid == ax.tree.Root || ax.tree.Node(nid) == nil {
		return 0
	}
	if el := ax.elems[nid]; el != 0 {
		return el
	}
	el := send(class("TangoA11yElement"), "new")
	ax.elems[nid] = el
	a11yElems[el] = elemRef{ax, nid}
	return el
}

// elemOrView is elem, with the view for the root and for 0.
func (ax *canvasA11y) elemOrView(nid a11y.NodeID) id {
	if ax.tree == nil || nid == 0 || nid == ax.tree.Root {
		return ax.c.view
	}
	return ax.elem(nid)
}

func (ax *canvasA11y) drop(nid a11y.NodeID) {
	el := ax.elems[nid]
	if el == 0 {
		return
	}
	delete(ax.elems, nid)
	delete(a11yElems, el)
	ax.post(el, axDestroyed)
	release(el)
}

func (ax *canvasA11y) dropAll() {
	for nid := range ax.elems {
		ax.drop(nid)
	}
}

// children returns the elements of n's children.
func (ax *canvasA11y) children(n *a11y.Node) id {
	objs := make([]id, 0, len(n.Children))
	for _, k := range n.Children {
		if el := ax.elem(k); el != 0 {
			objs = append(objs, el)
		}
	}
	return nsArray(objs...)
}

// screenRect converts logical canvas bounds to AppKit screen coordinates
// (points, bottom-left origin).
func (ax *canvasA11y) screenRect(b a11y.Rect) nsRect {
	c := ax.c
	r := nsRect{Origin: nsPoint{float64(b.X), float64(b.Y)}, Size: nsSize{float64(b.W), float64(b.H)}}
	inWin := msgConvertRect(c.view, sel("convertRect:toView:"), r, 0)
	return msgContentRectForRect(c.s.win, sel("convertRectToScreen:"), inWin)
}

// hitTest returns the element (or the view) at screen point p.
func (ax *canvasA11y) hitTest(p nsPoint) id {
	c := ax.c
	if ax.tree == nil {
		return c.view
	}
	inWin := msgPointArg(c.s.win, sel("convertPointFromScreen:"), p)
	v := msgConvertPoint(c.view, sel("convertPoint:fromView:"), inWin, 0)
	nid := a11ybridge.HitTest(ax.tree, float32(v.X), float32(v.Y))
	if nid == 0 {
		return c.view
	}
	return ax.elemOrView(nid)
}

// perform sends act to the core.
func (ax *canvasA11y) perform(nid a11y.NodeID, act a11y.Action, arg string) bool {
	c := ax.c
	if c.closed || !c.ready || act == 0 {
		return false
	}
	as, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return false
	}
	if act == a11y.Focus && send(c.s.win, "firstResponder") != c.view {
		send(c.s.win, "makeFirstResponder:", uintptr(c.view))
	}
	return as.AccessibilityAction(nid, act, arg) == nil
}

// elemNode returns the canvas state and node behind an element.
func elemNode(self id) (*canvasA11y, *a11y.Node) {
	ref, ok := a11yElems[self]
	if !ok || ref.ax.c.closed || ref.ax.tree == nil {
		return nil, nil
	}
	n := ref.ax.tree.Node(ref.id)
	if n == nil {
		return nil, nil
	}
	return ref.ax, n
}

// textRole reports roles whose text is their AXValue (static text).
func textRole(n *a11y.Node) bool {
	return n.Role == a11y.RoleText || n.Role == a11y.RoleParagraph
}

// macValue is the AXValue of n (0 for none).
func macValue(n *a11y.Node) id {
	switch {
	case n.States.Has(a11y.Protected):
		return 0
	case a11ybridge.MacToggle(n):
		return send(class("NSNumber"), "numberWithInteger:", uintptr(a11ybridge.CheckValue(n)))
	case n.Role == a11y.RoleTab:
		return send(class("NSNumber"), "numberWithInteger:", uintptr(boolInt(n.States.Has(a11y.Selected))))
	case n.HasRange:
		return msgNumberFloat(class("NSNumber"), sel("numberWithDouble:"), n.ValueNow)
	case n.Value != "":
		return nsString(n.Value)
	case textRole(n) && n.Name != "":
		return nsString(n.Name)
	}
	return 0
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nsStr(s string) id {
	if s == "" {
		return 0
	}
	return nsString(s)
}

// actionFor maps an AX perform selector to the node's action (0: none).
func actionFor(n *a11y.Node, selector string) a11y.Action {
	for _, a := range a11ybridge.MacActions(n) {
		switch {
		case selector == "accessibilityPerformPress" && a.Name == "AXPress",
			selector == "accessibilityPerformIncrement" && a.Name == "AXIncrement",
			selector == "accessibilityPerformDecrement" && a.Name == "AXDecrement",
			selector == "accessibilityPerformCancel" && a.Name == "AXCancel",
			selector == "accessibilityPerformShowMenu" && a.Name == "AXShowMenu":
			return a.Act
		}
	}
	return 0
}

func performer(selector string) objc.MethodDef {
	return method(selector, func(self id, _ objc.SEL) bool {
		ax, n := elemNode(self)
		if n == nil {
			return false
		}
		return ax.perform(n.ID, actionFor(n, selector), "")
	})
}

func registerA11yElement() {
	str := func(get func(n *a11y.Node) id) func(self id, _ objc.SEL) id {
		return func(self id, _ objc.SEL) id {
			if _, n := elemNode(self); n != nil {
				return get(n)
			}
			return 0
		}
	}
	flag := func(get func(ax *canvasA11y, n *a11y.Node) bool) func(self id, _ objc.SEL) bool {
		return func(self id, _ objc.SEL) bool {
			if ax, n := elemNode(self); n != nil {
				return get(ax, n)
			}
			return false
		}
	}
	rangeNum := func(get func(n *a11y.Node) float64) func(self id, _ objc.SEL) id {
		return func(self id, _ objc.SEL) id {
			if _, n := elemNode(self); n != nil && n.HasRange {
				return msgNumberFloat(class("NSNumber"), sel("numberWithDouble:"), get(n))
			}
			return 0
		}
	}
	methods := []objc.MethodDef{
		method("isAccessibilityElement", flag(func(*canvasA11y, *a11y.Node) bool { return true })),
		method("accessibilityRole", str(func(n *a11y.Node) id { r, _ := a11ybridge.MacRole(n); return nsString(r) })),
		method("accessibilitySubrole", str(func(n *a11y.Node) id { _, s := a11ybridge.MacRole(n); return nsStr(s) })),
		method("accessibilityRoleDescription", str(func(n *a11y.Node) id {
			if n.RoleDescription != "" {
				return nsString(n.RoleDescription)
			}
			r, s := a11ybridge.MacRole(n)
			return axRoleDesc(nsString(r), nsStr(s))
		})),
		method("accessibilityLabel", str(func(n *a11y.Node) id {
			if textRole(n) && n.Value == "" {
				return 0 // the text is the value
			}
			return nsStr(n.Name)
		})),
		method("accessibilityHelp", str(func(n *a11y.Node) id { return nsStr(n.Description) })),
		method("accessibilityValue", str(macValue)),
		method("accessibilityPlaceholderValue", str(func(n *a11y.Node) id { return nsStr(n.Placeholder) })),
		method("accessibilityIdentifier", str(func(n *a11y.Node) id { return nsString("tango-" + strconv.FormatUint(uint64(n.ID), 10)) })),
		method("accessibilityMinValue", rangeNum(func(n *a11y.Node) float64 { return n.ValueMin })),
		method("accessibilityMaxValue", rangeNum(func(n *a11y.Node) float64 { return n.ValueMax })),
		method("isAccessibilityEnabled", flag(func(_ *canvasA11y, n *a11y.Node) bool { return !n.States.Has(a11y.Disabled) })),
		method("isAccessibilityFocused", flag(func(ax *canvasA11y, n *a11y.Node) bool { return ax.c.focused && ax.focus == n.ID })),
		method("isAccessibilitySelected", flag(func(_ *canvasA11y, n *a11y.Node) bool { return n.States.Has(a11y.Selected) })),
		method("isAccessibilityExpanded", flag(func(_ *canvasA11y, n *a11y.Node) bool { return n.States.Has(a11y.Expanded) })),
		method("isAccessibilityRequired", flag(func(_ *canvasA11y, n *a11y.Node) bool { return n.States.Has(a11y.Required) })),
		method("isAccessibilityModal", flag(func(_ *canvasA11y, n *a11y.Node) bool { return n.States.Has(a11y.Modal) })),
		method("accessibilityParent", func(self id, _ objc.SEL) id {
			if ax, n := elemNode(self); n != nil {
				return ax.elemOrView(n.Parent)
			}
			return 0
		}),
		method("accessibilityChildren", func(self id, _ objc.SEL) id {
			if ax, n := elemNode(self); n != nil {
				return ax.children(n)
			}
			return nsArray()
		}),
		method("accessibilityWindow", func(self id, _ objc.SEL) id {
			if ax, n := elemNode(self); n != nil {
				return ax.c.s.win
			}
			return 0
		}),
		method("accessibilityTopLevelUIElement", func(self id, _ objc.SEL) id {
			if ax, n := elemNode(self); n != nil {
				return ax.c.s.win
			}
			return 0
		}),
		method("accessibilityFrame", func(self id, _ objc.SEL) nsRect {
			if ax, n := elemNode(self); n != nil {
				return ax.screenRect(n.Bounds)
			}
			return nsRect{}
		}),
		method("accessibilityHitTest:", func(self id, _ objc.SEL, p nsPoint) id { return self }),
		performer("accessibilityPerformPress"),
		performer("accessibilityPerformIncrement"),
		performer("accessibilityPerformDecrement"),
		performer("accessibilityPerformCancel"),
		performer("accessibilityPerformShowMenu"),
		method("setAccessibilityValue:", func(self id, _ objc.SEL, v id) {
			ax, n := elemNode(self)
			if n == nil || !n.Actions.Has(a11y.SetValue) || v == 0 {
				return
			}
			var arg string
			if sendBool(v, "isKindOfClass:", uintptr(class("NSNumber"))) {
				arg = goString(send(v, "stringValue"))
			} else {
				arg = nsStringArg(v)
			}
			ax.perform(n.ID, a11y.SetValue, arg)
		}),
		method("setAccessibilityFocused:", func(self id, _ objc.SEL, on bool) {
			if ax, n := elemNode(self); n != nil && on && n.Actions.Has(a11y.Focus) {
				if ax.focus == n.ID {
					// Already focused: the core will not report a change,
					// but the requester waits for the notification.
					if el := ax.elemOrView(n.ID); el != 0 {
						ax.post(el, axFocusedChanged)
					}
					return
				}
				ax.axAsked = n.ID
				ax.perform(n.ID, a11y.Focus, "")
			}
		}),
		method("isAccessibilitySelectorAllowed:", func(self id, _ objc.SEL, s objc.SEL) bool {
			_, n := elemNode(self)
			if n == nil {
				return false
			}
			name := selGetName(s)
			switch name {
			case "accessibilityPerformPress", "accessibilityPerformIncrement", "accessibilityPerformDecrement",
				"accessibilityPerformCancel", "accessibilityPerformShowMenu":
				return actionFor(n, name) != 0
			case "setAccessibilityValue:":
				return n.Actions.Has(a11y.SetValue)
			case "setAccessibilityFocused:":
				return n.Actions.Has(a11y.Focus) || n.States.Has(a11y.Focusable)
			}
			return true
		}),
	}
	classDef("TangoA11yElement", "NSAccessibilityElement", nil, methods)
}

// canvasA11yMethods are the NSAccessibility methods of TangoCanvasView
// (and so of TangoCanvasOverlay): the view is the tree's root.
func canvasA11yMethods() []objc.MethodDef {
	loadA11y()
	state := func(self id) *canvasA11y {
		c := canvasOf(self)
		if c == nil {
			return nil
		}
		ax := c.a11yState()
		ax.activate()
		return ax
	}
	return []objc.MethodDef{
		method("isAccessibilityElement", func(self id, _ objc.SEL) bool { return canvasOf(self) != nil }),
		method("accessibilityRole", func(self id, _ objc.SEL) id { return nsString("AXGroup") }),
		method("accessibilityLabel", func(self id, _ objc.SEL) id {
			if ax := state(self); ax != nil && ax.tree != nil {
				if n := ax.tree.Node(ax.tree.Root); n != nil {
					return nsStr(n.Name)
				}
			}
			return 0
		}),
		method("accessibilityChildren", func(self id, _ objc.SEL) id {
			ax := state(self)
			if ax == nil || ax.tree == nil {
				return nsArray()
			}
			root := ax.tree.Node(ax.tree.Root)
			if root == nil {
				return nsArray()
			}
			return ax.children(root)
		}),
		method("accessibilityHitTest:", func(self id, _ objc.SEL, p nsPoint) id {
			if ax := state(self); ax != nil {
				return ax.hitTest(p)
			}
			return self
		}),
		method("accessibilityFocusedUIElement", func(self id, _ objc.SEL) id {
			ax := state(self)
			if ax == nil || ax.focus == 0 {
				return self
			}
			if el := ax.elemOrView(ax.focus); el != 0 {
				return el
			}
			return self
		}),
	}
}

// axConst returns the AppKit NSString constant sym, or value when the
// symbol is missing.
func axConst(sym, value string) id {
	if v := constPtr(libAppKit, sym); v != 0 {
		return id(v)
	}
	return nsString(value)
}
