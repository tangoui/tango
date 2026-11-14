//go:build ios

package ios

// Accessibility of canvas views (docs/specs/mobile-canvas.md §5.1): the
// Go half of canvas_a11y.m. The canvas implements
// platform.CanvasAccessible: it keeps the last semantics tree and pushes
// every added or changed node to the view as a tga_node (traits, shape,
// texts and actions from package a11ybridge), so UIKit's queries never
// call Go. UIKit's action methods come back here and go to the sink.
//
// Activation: the bridge turns on when VoiceOver or Switch Control is
// running, or on the first query of the view's accessibilityElements
// (Voice Control, XCUITest and the Accessibility Inspector all ask). It
// turns off once neither is running and no query arrived for 30 seconds;
// the core then drops the tree.

/*
#include <stdlib.h>
#include "canvas_a11y.h"
*/
import "C"

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

var _ platform.CanvasAccessible = (*canvas)(nil)

// textResponder reports whether elements can name a text input responder
// (iOS 18.1+); fixed for the process, so asked once.
var textResponder = sync.OnceValue(func() bool { return C.tga_text_responder() != 0 })

// The header's trait, shape and container values are a11ybridge's:
// indexing a one-element array with a non-zero constant fails to compile.
var (
	_ = [1]int{}[uint32(C.TGA_TRAIT_BUTTON)^uint32(a11ybridge.IOSButton)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_LINK)^uint32(a11ybridge.IOSLink)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_HEADER)^uint32(a11ybridge.IOSHeader)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_SEARCH_FIELD)^uint32(a11ybridge.IOSSearchField)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_IMAGE)^uint32(a11ybridge.IOSImage)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_SELECTED)^uint32(a11ybridge.IOSSelected)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_STATIC_TEXT)^uint32(a11ybridge.IOSStaticText)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_NOT_ENABLED)^uint32(a11ybridge.IOSNotEnabled)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_ADJUSTABLE)^uint32(a11ybridge.IOSAdjustable)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_UPDATES_FREQUENTLY)^uint32(a11ybridge.IOSUpdatesFrequently)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_TAB_BAR)^uint32(a11ybridge.IOSTabBar)]
	_ = [1]int{}[uint32(C.TGA_TRAIT_TOGGLE_BUTTON)^uint32(a11ybridge.IOSToggleButton)]
	_ = [1]int{}[int(C.TGA_SHAPE_ELEMENT)^int(a11ybridge.IOSElement)]
	_ = [1]int{}[int(C.TGA_SHAPE_CONTAINER)^int(a11ybridge.IOSContainer)]
	_ = [1]int{}[int(C.TGA_CONTAINER_DATA_TABLE)^int(a11ybridge.IOSContainerDataTable)]
	_ = [1]int{}[int(C.TGA_CONTAINER_LIST)^int(a11ybridge.IOSContainerList)]
	_ = [1]int{}[int(C.TGA_CONTAINER_LANDMARK)^int(a11ybridge.IOSContainerLandmark)]
	_ = [1]int{}[int(C.TGA_CONTAINER_SEMANTIC_GROUP)^int(a11ybridge.IOSContainerSemanticGroup)]
)

const (
	a11yIdle     = 30 * time.Second        // no query this long (and no VoiceOver): deactivate
	a11yReread   = 1500 * time.Millisecond // an activated element whose value changes within this is read again
	a11yIDPrefix = "tango-"                // accessibilityIdentifier: prefix + node id
)

// canvasA11y is the accessibility state of one canvas. Main thread,
// except active (read by AccessibilityActive from the UI thread, which is
// the main thread on iOS, but kept atomic as the interface requires).
type canvasA11y struct {
	active    atomic.Bool
	tree      *a11y.Tree
	lastQuery time.Time
	// The node an assistive technology last activated, and when: when its
	// value or state changes soon after (a switch toggled), VoiceOver is
	// asked to read it again, as it does for UIKit's own controls.
	activated   a11y.NodeID
	activatedAt time.Time
}

// a11yStart runs when the canvas opens.
func (c *canvas) a11yStart() {
	C.tga_install()
	if C.tga_assistive() != 0 {
		c.a11yActivate("VoiceOver or Switch Control running")
	}
	startA11ySelftest(c)
}

func (c *canvas) a11yActivate(why string) {
	if c.closed || c.ax.active.Swap(true) {
		return
	}
	if a11ySelftest.on {
		log.Printf("canvas a11y: active (%s)", why)
	}
	C.tga_watch(c.handle(), 1)
	c.Invalidate() // the next frame sends the whole tree
}

func (c *canvas) a11yDeactivate() {
	if !c.ax.active.Swap(false) {
		return
	}
	if a11ySelftest.on {
		log.Printf("canvas a11y: inactive (no assistive technology, no query for %v)", a11yIdle)
	}
	C.tga_watch(c.handle(), 0)
	c.Invalidate() // the next frame drops the tree (SetAccessibilityTree(nil))
}

// ---- platform.CanvasAccessible ----

func (c *canvas) AccessibilityActive() bool { return c.ax.active.Load() }

func (c *canvas) SetAccessibilityTree(tree *a11y.Tree) {
	if c.closed {
		return
	}
	c.ax.tree = tree
	if tree == nil {
		C.tga_clear(c.handle())
		return
	}
	C.tga_begin(c.handle(), C.uint64_t(tree.Root), 1)
	for _, n := range tree.Nodes() {
		c.a11yPush(tree, n)
	}
	var screen a11y.NodeID
	for _, n := range tree.Nodes() {
		if a11ybridge.IOSModal(n) {
			screen = n.ID
			break
		}
	}
	C.tga_end(c.handle(), 1, C.uint64_t(screen), 0)
}

func (c *canvas) UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID) {
	if c.closed {
		return
	}
	prev := c.ax.tree
	c.ax.tree = tree
	C.tga_begin(c.handle(), C.uint64_t(tree.Root), 0)
	var reread a11y.NodeID
	for _, id := range changed {
		n := tree.Node(id)
		if n == nil {
			C.tga_remove(c.handle(), C.uint64_t(id))
			continue
		}
		c.a11yPush(tree, n)
		if id == c.ax.activated && time.Since(c.ax.activatedAt) < a11yReread {
			ch := a11ybridge.Diff(prev.Node(id), n)
			if ch.Has(a11ybridge.ChangeValue) || ch.Has(a11ybridge.ChangeStates) || ch.Has(a11ybridge.ChangeName) {
				reread = id
				c.ax.activated = 0
			}
		}
	}
	note := a11ybridge.IOSNotify(prev, tree, changed)
	C.tga_end(c.handle(), cbool(note.Layout), C.uint64_t(note.Screen), C.uint64_t(reread))
}

func (c *canvas) AccessibilityFocusChanged(id a11y.NodeID) {
	if c.closed {
		return
	}
	C.tga_focus(c.handle(), C.uint64_t(id))
}

func (c *canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	if c.closed || strings.TrimSpace(text) == "" {
		return
	}
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.tga_announce(c.handle(), cs, cbool(live == a11y.LiveAssertive))
}

// a11yPush sends node n to the view (or removes its element when it is
// not exposed).
func (c *canvas) a11yPush(tree *a11y.Tree, n *a11y.Node) {
	shape, self := a11ybridge.IOSLayout(n)
	if n.ID == tree.Root {
		shape, self = a11ybridge.IOSContainer, false // the view itself
	}
	if shape == a11ybridge.IOSSkip {
		C.tga_remove(c.handle(), C.uint64_t(n.ID))
		return
	}
	txt := a11ybridge.IOSStrings(n)
	acts := a11ybridge.IOSActions(n)
	var cn C.tga_node
	cn.id, cn.parent = C.uint64_t(n.ID), C.uint64_t(n.Parent)
	cn.shape = C.int(shape)
	cn.self = cbool(self)
	traits := a11ybridge.IOSTraits(n)
	if shape == a11ybridge.IOSContainer {
		// A container is not spoken as a control (its self element is);
		// only a tab bar is marked on the container itself.
		traits &= a11ybridge.IOSTabBar
	}
	cn.traits = C.uint32_t(traits)
	cn.container = C.int(a11ybridge.IOSContainerKind(n))
	cn.modal = cbool(a11ybridge.IOSModal(n))
	cn.x, cn.y, cn.w, cn.h = C.double(n.Bounds.X), C.double(n.Bounds.Y), C.double(n.Bounds.W), C.double(n.Bounds.H)
	var frees []unsafe.Pointer
	cstr := func(s string) *C.char {
		if s == "" {
			return nil
		}
		p := C.CString(s)
		frees = append(frees, unsafe.Pointer(p))
		return p
	}
	cn.label, cn.value = cstr(txt.Label), cstr(txt.Value)
	// Editable fields: the element names the canvas as its text responder
	// (iOS 18.1+), else its hint says how to edit (a11ybridge.IOSEditHint).
	editable := shape == a11ybridge.IOSElement && a11ybridge.IOSEditable(n)
	cn.editable = cbool(editable)
	cn.hint = cstr(a11ybridge.IOSEditHint(txt.Hint, n, textResponder()))
	cn.ident = cstr(a11yIDPrefix + strconv.FormatUint(uint64(n.ID), 10))
	if acts.Activate != 0 {
		cn.actions |= C.TGA_ACT_ACTIVATE
	}
	if acts.Increment {
		cn.actions |= C.TGA_ACT_INCREMENT
	}
	if acts.Decrement {
		cn.actions |= C.TGA_ACT_DECREMENT
	}
	if len(acts.Custom) > 0 {
		names := make([]string, len(acts.Custom))
		for i, a := range acts.Custom {
			names[i] = strings.ReplaceAll(a.Name, "\n", " ")
		}
		cn.custom = cstr(strings.Join(names, "\n"))
	}
	cn.row, cn.col = C.int(n.RowIndex), C.int(n.ColIndex)
	if n.Role == a11y.RoleRow || n.Role == a11y.RoleTable || n.Role == a11y.RoleGrid {
		cn.row = 0
		cn.col = 0
	}
	cn.rows, cn.cols = C.int(n.RowCount), C.int(n.ColCount)
	if k := len(n.Children); k > 0 {
		// C memory: cn itself is Go memory, which may not hold Go pointers.
		p := C.malloc(C.size_t(k) * C.size_t(unsafe.Sizeof(C.uint64_t(0))))
		frees = append(frees, p)
		copy(unsafe.Slice((*a11y.NodeID)(p), k), n.Children)
		cn.kids = (*C.uint64_t)(p)
		cn.nkids = C.int(k)
	}
	C.tga_set(c.handle(), &cn)
	for _, p := range frees {
		C.free(p)
	}
}

// a11yPerform hands act on node id to the sink. Actions arrive from UIKit
// event handling, never inside a frame; one that did would wait for the
// next.
func (c *canvas) a11yPerform(id a11y.NodeID, act a11y.Action) bool {
	if c.closed || !c.ready || act == 0 {
		return false
	}
	as, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return false
	}
	if c.inFrame {
		return false // the sink is never called inside CanvasFrame
	}
	err := as.AccessibilityAction(id, act, "")
	if a11ySelftest.on {
		log.Printf("canvas a11y: perform %s on %d: %v", act, id, err)
	}
	return err == nil
}

//export tangoIOSCanvasA11yAction
func tangoIOSCanvasA11yAction(h C.uintptr_t, nid C.uint64_t, op C.int, arg C.int) C.int {
	c := lookup(h)
	if c == nil || c.ax.tree == nil {
		return 0
	}
	t := c.ax.tree
	id := a11y.NodeID(nid)
	n := t.Node(id)
	if n == nil {
		return 0
	}
	var act a11y.Action
	target := id
	switch op {
	case C.TGA_OP_ACTIVATE:
		act = a11ybridge.IOSActions(n).Activate
	case C.TGA_OP_INCREMENT:
		act = n.Actions & a11y.Increment
	case C.TGA_OP_DECREMENT:
		act = n.Actions & a11y.Decrement
	case C.TGA_OP_ESCAPE:
		target, act = a11ybridge.IOSEscapeTarget(t, id), a11y.Dismiss
	case C.TGA_OP_SCROLL_FORWARD, C.TGA_OP_SCROLL_BACK:
		target, act = a11ybridge.IOSScrollTarget(t, id, op == C.TGA_OP_SCROLL_FORWARD), a11y.ScrollIntoView
	case C.TGA_OP_CUSTOM:
		if cs := a11ybridge.IOSActions(n).Custom; int(arg) >= 0 && int(arg) < len(cs) {
			act = cs[arg].Act
		}
	case C.TGA_OP_FOCUSED:
		// VoiceOver moved to a node scrolled out of view: bring it in.
		if n.States.Has(a11y.Offscreen) && n.Actions.Has(a11y.ScrollIntoView) {
			act = a11y.ScrollIntoView
		}
	}
	if target == 0 || act == 0 {
		return 0
	}
	if !c.a11yPerform(target, act) {
		return 0
	}
	if op == C.TGA_OP_ACTIVATE || op == C.TGA_OP_INCREMENT || op == C.TGA_OP_DECREMENT || op == C.TGA_OP_CUSTOM {
		c.ax.activated, c.ax.activatedAt = target, time.Now()
	}
	return 1
}

//export tangoIOSCanvasA11yQuery
func tangoIOSCanvasA11yQuery(h C.uintptr_t) {
	c := lookup(h)
	if c == nil || !c.ready {
		return
	}
	c.ax.lastQuery = time.Now()
	c.a11yActivate("accessibilityElements queried")
}

//export tangoIOSCanvasA11yStatus
func tangoIOSCanvasA11yStatus(running C.int) {
	if running == 0 {
		return // the idle check deactivates later
	}
	for _, c := range canvases {
		if !c.closed && c.ready {
			c.a11yActivate("VoiceOver or Switch Control started")
		}
	}
}

//export tangoIOSCanvasA11yTick
func tangoIOSCanvasA11yTick(h C.uintptr_t) {
	c := lookup(h)
	if c == nil || C.tga_assistive() != 0 || time.Since(c.ax.lastQuery) < a11yIdle {
		return
	}
	c.a11yDeactivate()
}

// ---- self-test ----

// a11ySelftest drives the bridge as an assistive technology would on the
// simulator, where VoiceOver cannot run (docs/specs/mobile-canvas.md
// §7.3). With TANGO_CANVAS_A11Y_SELFTEST set (simctl launch:
// SIMCTL_CHILD_TANGO_CANVAS_A11Y_SELFTEST=1) the first canvas, a few
// seconds after it opens, logs the element tree UIKit sees, then performs
// accessibility actions on elements found by label and logs the results.
// The value may list the labels to activate, "a;b;c"; "1" uses the
// examples/mobile-ui defaults.
var a11ySelftest struct {
	on      bool
	started bool
	labels  []string
	step    int
}

func init() {
	v := os.Getenv("TANGO_CANVAS_A11Y_SELFTEST")
	if v == "" {
		return
	}
	a11ySelftest.on = true
	if v == "1" {
		a11ySelftest.labels = []string{"点我计数", "蓝牙", "表单"}
	} else {
		a11ySelftest.labels = strings.Split(v, ";")
	}
}

func startA11ySelftest(c *canvas) {
	if !a11ySelftest.on || a11ySelftest.started {
		return
	}
	a11ySelftest.started = true
	log.Printf("canvas a11y selftest: start, active=%v, labels %q", c.AccessibilityActive(), a11ySelftest.labels)
	C.tga_after(3, c.handle())
}

func (c *canvas) a11yDump(what string) {
	p := C.tga_dump(c.handle())
	s := C.GoString(p)
	C.free(unsafe.Pointer(p))
	log.Printf("canvas a11y selftest: %s (active=%v):\n%s", what, c.AccessibilityActive(), s)
}

func (c *canvas) a11yOp(label string, op C.int) C.int {
	cs := C.CString(label)
	defer C.free(unsafe.Pointer(cs))
	return C.tga_selftest_op(c.handle(), cs, op)
}

//export tangoIOSCanvasA11ySelftest
func tangoIOSCanvasA11ySelftest(h C.uintptr_t) {
	c := lookup(h)
	if c == nil || !a11ySelftest.on {
		return
	}
	step := a11ySelftest.step
	a11ySelftest.step++
	next := func(sec float64) { C.tga_after(C.double(sec), h) }
	// Steps: the first query (activates the bridge; the tree follows a
	// frame later), the tree, then each label activated and the tree
	// after it, then an increment, then the final tree.
	labels := a11ySelftest.labels
	switch {
	case step == 0:
		c.a11yDump("first query")
		next(2)
	case step == 1:
		c.a11yDump("tree")
		next(0.5)
	case step-2 < 2*len(labels):
		l := labels[(step-2)/2]
		if (step-2)%2 == 0 {
			log.Printf("canvas a11y selftest: activate %q -> %d", l, c.a11yOp(l, C.TGA_OP_ACTIVATE))
			next(1.5)
		} else {
			c.a11yDump("after activate " + strconv.Quote(l))
			next(0.5)
		}
	case step-2 == 2*len(labels):
		log.Printf("canvas a11y selftest: increment first adjustable -> %d", c.a11yOp("", -1))
		next(1)
	default:
		c.a11yDump("done")
	}
}
