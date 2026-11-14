package a11ybridge

// iOS (UIAccessibility) translation for the canvas bridge of
// internal/platform/ios (docs/specs/mobile-canvas.md §5.1): traits,
// element / container shape, label-value-hint text and the actions of a
// node. Traits are a Tango bit set; the Objective-C side maps each bit to
// its UIAccessibilityTrait* constant, so no UIKit value is written here.

import (
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
)

// IOSTraitSet is a set of UIAccessibilityTraits named by Tango bits.
type IOSTraitSet uint32

const (
	IOSButton            IOSTraitSet = 1 << iota // UIAccessibilityTraitButton
	IOSLink                                      // UIAccessibilityTraitLink
	IOSHeader                                    // UIAccessibilityTraitHeader
	IOSSearchField                               // UIAccessibilityTraitSearchField
	IOSImage                                     // UIAccessibilityTraitImage
	IOSSelected                                  // UIAccessibilityTraitSelected
	IOSStaticText                                // UIAccessibilityTraitStaticText
	IOSNotEnabled                                // UIAccessibilityTraitNotEnabled
	IOSAdjustable                                // UIAccessibilityTraitAdjustable
	IOSUpdatesFrequently                         // UIAccessibilityTraitUpdatesFrequently
	IOSTabBar                                    // UIAccessibilityTraitTabBar
	IOSToggleButton                              // UIAccessibilityTraitToggleButton (iOS 17)
)

// iosTraitNames are the trait names used in the IOS column of
// a11y.Mappings, in bit order.
var iosTraitNames = []string{"button", "link", "header", "searchField", "image", "selected", "staticText",
	"notEnabled", "adjustable", "updatesFrequently", "tabBar", "toggleButton"}

// Has reports whether every trait of x is set.
func (t IOSTraitSet) Has(x IOSTraitSet) bool { return t&x == x }

// String lists the traits ("button|selected").
func (t IOSTraitSet) String() string {
	var b []string
	for i, n := range iosTraitNames {
		if t&(1<<i) != 0 {
			b = append(b, n)
		}
	}
	return strings.Join(b, "|")
}

// iosMapping parses the IOS column of a11y.Mappings for role: the traits
// it names and whether the role is a pure container. A parenthesized note
// ("container (dataTable)", "adjustable (splitter only)") is dropped.
func iosMapping(role a11y.Role) (traits IOSTraitSet, container bool) {
	p, ok := a11y.Mappings[role]
	if !ok {
		return 0, false
	}
	s, _, _ := strings.Cut(p.IOS, "(")
	s = strings.TrimSpace(s)
	if s == "container" {
		return 0, true
	}
	for _, tok := range strings.Split(s, "+") {
		for i, n := range iosTraitNames {
			if tok == n {
				traits |= 1 << i
			}
		}
	}
	return traits, role == a11y.RoleTabList // a tab bar is a container with a trait
}

// iosToggle reports nodes whose on/off state is their value under the
// toggleButton trait (checkbox, switch, toggle button, checkable menu
// item). Radio buttons use the selected trait instead.
func iosToggle(n *a11y.Node) bool {
	return MacToggle(n) && n.Role != a11y.RoleRadio
}

// IOSTraits returns the traits of n: the IOS column of a11y.Mappings
// refined by the states and actions of docs/accessibility.md sections 3-5.
func IOSTraits(n *a11y.Node) IOSTraitSet {
	t, _ := iosMapping(n.Role)
	st := n.States
	ranged := n.Actions&(a11y.Increment|a11y.Decrement) != 0
	if n.Role == a11y.RoleSeparator && !ranged {
		t &^= IOSAdjustable // a static divider; only a pane splitter adjusts
	}
	if ranged {
		t |= IOSAdjustable
	}
	if iosToggle(n) {
		// The value says on, off or mixed; the selected trait would read
		// the state twice.
		t |= IOSButton | IOSToggleButton
	} else if st.Has(a11y.Selected) || st.Has(a11y.Current) ||
		(n.Role == a11y.RoleRadio && st.Has(a11y.Checked)) {
		t |= IOSSelected
	}
	if st.Has(a11y.Disabled) {
		t |= IOSNotEnabled
	}
	if st.Has(a11y.Busy) {
		t |= IOSUpdatesFrequently
	}
	if t&(IOSHeader|IOSButton|IOSLink) == IOSHeader {
		// Headings are text, as a UILabel marked as a header reads.
		t |= IOSStaticText
	}
	if n.Actions.Has(a11y.Press) && t&(IOSButton|IOSLink|IOSAdjustable|IOSSearchField|IOSTabBar) == 0 {
		// A clickable list item or cell reads as a button.
		t = t&^IOSStaticText | IOSButton
	}
	return t
}

// IOSShape says how a node appears in the UIAccessibility hierarchy.
type IOSShape uint8

const (
	IOSSkip      IOSShape = iota // not exposed
	IOSElement                   // an accessibility element (isAccessibilityElement = YES)
	IOSContainer                 // a container of the children's elements
)

// IOSLayout returns the shape of n. Nodes with children are containers
// unless their role is atomic (a button whose label came from its
// children). self reports a container whose own role and name are not a
// container's (a list item holding a switch): it also exposes an element
// for itself, first among its children, so the name and actions stay
// reachable. Elements without area (nothing to point at) and unnamed
// empty containers are skipped.
func IOSLayout(n *a11y.Node) (shape IOSShape, self bool) {
	_, container := iosMapping(n.Role)
	if len(n.Children) > 0 && (container || !n.Role.Atomic()) {
		self = !container && (n.Name != "" || n.Actions&^(a11y.Focus|a11y.ScrollIntoView) != 0)
		return IOSContainer, self
	}
	if container && n.Name == "" {
		return IOSSkip, false
	}
	if n.Bounds.W <= 0 || n.Bounds.H <= 0 {
		return IOSSkip, false
	}
	return IOSElement, false
}

// IOSContainerType names a UIAccessibilityContainerType.
type IOSContainerType uint8

const (
	IOSContainerNone IOSContainerType = iota
	IOSContainerDataTable
	IOSContainerList
	IOSContainerLandmark
	IOSContainerSemanticGroup
)

// IOSContainerKind returns the container type of n (meaningful when its
// shape is IOSContainer).
func IOSContainerKind(n *a11y.Node) IOSContainerType {
	switch n.Role {
	case a11y.RoleTable, a11y.RoleGrid:
		return IOSContainerDataTable
	case a11y.RoleList, a11y.RoleListbox, a11y.RoleMenu, a11y.RoleTree:
		return IOSContainerList
	case a11y.RoleNavigation, a11y.RoleForm:
		return IOSContainerLandmark
	case a11y.RoleWindow, a11y.RoleScrollArea, a11y.RoleTabList:
		return IOSContainerNone
	}
	return IOSContainerSemanticGroup
}

// IOSModal reports a container whose siblings VoiceOver must ignore
// (accessibilityViewIsModal).
func IOSModal(n *a11y.Node) bool { return n.States.Has(a11y.Modal) }

// IOSWords are the words the bridge adds to labels, values and hints
// where UIKit has no trait for a state, and the names of custom actions.
// The defaults are English; an application may replace them (main thread,
// before the first tree).
type IOSWords struct {
	Expand, Collapse, Focus string // custom action names
	Expanded, Collapsed     string // value suffix of expandable nodes
	Mixed                   string // value of a mixed checkbox
	Required                string // hint suffix
	Invalid                 string // value suffix (the hint holds the message)
	// EditHint is the hint suffix of an editable text input where UIKit
	// cannot be given the canvas as the field's text responder (before
	// iOS 18.1), so VoiceOver does not announce it as a text field.
	EditHint string
}

// IOSPhrases is the vocabulary IOSStrings and IOSActions use.
var IOSPhrases = IOSWords{
	Expand: "Expand", Collapse: "Collapse", Focus: "Focus",
	Expanded: "expanded", Collapsed: "collapsed", Mixed: "mixed",
	Required: "required", Invalid: "invalid",
	EditHint: "Text field. Double tap to edit.",
}

// IOSEditable reports an enabled, editable text input: a text box, search
// box or editable combobox that takes typing (spin buttons adjust
// instead). UIAccessibility has no public text-field trait, so the bridge
// marks these for UIKit another way: from iOS 18.1 the element names the
// canvas view (a UITextInput) as its accessibilityTextInputResponder,
// which makes VoiceOver treat it as editable text; before that its hint
// says how to edit it (IOSEditHint).
func IOSEditable(n *a11y.Node) bool {
	if n.Role == a11y.RoleSpinButton || !isTextInput(n) {
		return false
	}
	return !n.States.Has(a11y.Disabled) && !n.States.Has(a11y.ReadOnly)
}

// IOSEditHint returns hint (IOSStrings' hint of n) with IOSPhrases.EditHint
// appended for an editable text input when responder is false (no text
// responder: before iOS 18.1); otherwise hint unchanged.
func IOSEditHint(hint string, n *a11y.Node, responder bool) string {
	if responder || !IOSEditable(n) {
		return hint
	}
	if hint == "" {
		return IOSPhrases.EditHint
	}
	return hint + ", " + IOSPhrases.EditHint
}

// IOSText is the accessibilityLabel, accessibilityValue and
// accessibilityHint of a node.
type IOSText struct{ Label, Value, Hint string }

// IOSStrings returns the label (Name), value (Value, the range value or
// the toggle state, plus state words) and hint (Description, plus
// "required") of n. Toggles use "1" / "0", which VoiceOver reads as on /
// off under the toggleButton trait, as for a UISwitch.
func IOSStrings(n *a11y.Node) IOSText {
	var t IOSText
	t.Label = n.Name
	text := isTextInput(n)
	if t.Label == "" && text {
		t.Label = n.Placeholder
	}
	var vals []string
	switch {
	case n.States.Has(a11y.Protected):
		// A password field never exposes its text.
	case iosToggle(n):
		switch CheckValue(n) {
		case 2:
			vals = append(vals, IOSPhrases.Mixed)
		case 1:
			vals = append(vals, "1")
		default:
			vals = append(vals, "0")
		}
	case n.Value != "":
		vals = append(vals, n.Value)
	case n.HasRange:
		vals = append(vals, iosRangeText(n))
	case text && n.Placeholder != "" && t.Label != n.Placeholder:
		vals = append(vals, n.Placeholder)
	}
	if n.States.Has(a11y.Expandable) || n.States.Has(a11y.Expanded) {
		if n.States.Has(a11y.Expanded) {
			vals = append(vals, IOSPhrases.Expanded)
		} else {
			vals = append(vals, IOSPhrases.Collapsed)
		}
	}
	if n.States.Has(a11y.Invalid) {
		vals = append(vals, IOSPhrases.Invalid)
	}
	t.Value = strings.Join(vals, ", ")
	hints := []string{}
	if n.Description != "" {
		hints = append(hints, n.Description)
	}
	if n.States.Has(a11y.Required) {
		hints = append(hints, IOSPhrases.Required)
	}
	t.Hint = strings.Join(hints, ", ")
	return t
}

// iosRangeText formats a range value without a text value: a percentage
// for progress bars and meters, the number otherwise.
func iosRangeText(n *a11y.Node) string {
	if (n.Role == a11y.RoleProgressBar || n.Role == a11y.RoleMeter) && n.ValueMax > n.ValueMin {
		p := (n.ValueNow - n.ValueMin) / (n.ValueMax - n.ValueMin) * 100
		return strconv.Itoa(int(math.Round(p))) + "%"
	}
	return strconv.FormatFloat(n.ValueNow, 'f', -1, 64)
}

// IOSCustomAction is a UIAccessibilityCustomAction: its name and the
// action it performs.
type IOSCustomAction struct {
	Name string
	Act  a11y.Action
}

// IOSActionSet is what a node answers to UIKit's accessibility actions.
type IOSActionSet struct {
	// Activate is performed by accessibilityActivate (a double tap): Press,
	// else Expand or Collapse, else Focus for a text input (to edit it).
	// 0 lets UIKit tap the element's activation point instead.
	Activate  a11y.Action
	Increment bool // accessibilityIncrement (adjustable)
	Decrement bool // accessibilityDecrement
	Custom    []IOSCustomAction
}

// IOSActions returns the actions of n. Escape and scrolling depend on the
// ancestors: see IOSEscapeTarget and IOSScrollTarget.
func IOSActions(n *a11y.Node) IOSActionSet {
	var s IOSActionSet
	acts := n.Actions
	expanded := n.States.Has(a11y.Expanded)
	switch {
	case acts.Has(a11y.Press):
		s.Activate = a11y.Press
	case expanded && acts.Has(a11y.Collapse):
		s.Activate = a11y.Collapse
	case !expanded && acts.Has(a11y.Expand):
		s.Activate = a11y.Expand
	case acts.Has(a11y.Collapse):
		s.Activate = a11y.Collapse
	case acts.Has(a11y.Focus) && (isTextInput(n) || n.States.Has(a11y.Focusable)):
		s.Activate = a11y.Focus
	}
	s.Increment = acts.Has(a11y.Increment)
	s.Decrement = acts.Has(a11y.Decrement)
	add := func(name string, a a11y.Action) {
		if acts.Has(a) && s.Activate != a {
			s.Custom = append(s.Custom, IOSCustomAction{name, a})
		}
	}
	if !expanded || !n.States.Has(a11y.Expandable) {
		add(IOSPhrases.Expand, a11y.Expand)
	}
	if expanded || !n.States.Has(a11y.Expandable) {
		add(IOSPhrases.Collapse, a11y.Collapse)
	}
	if !n.States.Has(a11y.Selectable) {
		// Items of a selection (tabs, options, rows) take focus when
		// pressed; a separate Focus action would only add noise.
		add(IOSPhrases.Focus, a11y.Focus)
	}
	return s
}

// IOSEscapeTarget returns the node that handles accessibilityPerformEscape
// (the two-finger scrub) on id: id or its nearest ancestor with Dismiss,
// so the gesture closes the dialog or popup the element is in. 0: none.
func IOSEscapeTarget(t *a11y.Tree, id a11y.NodeID) a11y.NodeID {
	for n := t.Node(id); n != nil; n = t.Node(n.Parent) {
		if n.Actions.Has(a11y.Dismiss) {
			return n.ID
		}
		if n.Parent == 0 {
			break
		}
	}
	return 0
}

// IOSScrollTarget answers accessibilityScroll: (a three-finger swipe) on
// id. The tree has no page-scroll action, so the bridge scrolls the next
// page into view instead: among the descendants of the nearest scrolling
// ancestor (scroll area, list, table, tree, grid, listbox), in reading
// order, the first offscreen node with ScrollIntoView after the visible
// ones (forward) or the last one before them (backward). 0: none.
func IOSScrollTarget(t *a11y.Tree, id a11y.NodeID, forward bool) a11y.NodeID {
	var scroller *a11y.Node
	for n := t.Node(id); n != nil; n = t.Node(n.Parent) {
		switch n.Role {
		case a11y.RoleScrollArea, a11y.RoleList, a11y.RoleListbox, a11y.RoleTable, a11y.RoleGrid, a11y.RoleTree:
			scroller = n
		}
		if scroller != nil || n.Parent == 0 {
			break
		}
	}
	if scroller == nil {
		return 0
	}
	var nodes []*a11y.Node
	var walk func(n *a11y.Node)
	walk = func(n *a11y.Node) {
		for _, k := range n.Children {
			if c := t.Node(k); c != nil {
				nodes = append(nodes, c)
				walk(c)
			}
		}
	}
	walk(scroller)
	first, last := -1, -1
	for i, n := range nodes {
		if !n.States.Has(a11y.Offscreen) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	ok := func(n *a11y.Node) bool { return n.States.Has(a11y.Offscreen) && n.Actions.Has(a11y.ScrollIntoView) }
	if forward {
		for _, n := range nodes[last+1:] {
			if ok(n) {
				return n.ID
			}
		}
		return 0
	}
	if first < 0 {
		first = len(nodes)
	}
	for i := first - 1; i >= 0; i-- {
		if ok(nodes[i]) {
			return nodes[i].ID
		}
	}
	return 0
}

// IOSNotice is the UIAccessibility notification an update calls for.
type IOSNotice struct {
	Layout bool        // UIAccessibilityLayoutChangedNotification: elements were added, removed or regrouped
	Screen a11y.NodeID // UIAccessibilityScreenChangedNotification: a modal node opened (move VoiceOver into it)
}

// IOSNotify returns the notification for an update from prev to next
// (changed as for CanvasAccessible.UpdateAccessibilityNodes). Value, name
// and bounds changes need none: VoiceOver reads them when it next visits
// the element.
func IOSNotify(prev, next *a11y.Tree, changed []a11y.NodeID) IOSNotice {
	var out IOSNotice
	for _, id := range changed {
		n, p := next.Node(id), prev.Node(id)
		c := Diff(p, n)
		switch {
		case c.Has(ChangeAdded), c.Has(ChangeRemoved), c.Has(ChangeChildren):
			out.Layout = true
		case c != 0:
			// An element that became a container (or was hidden because
			// it lost its size) changes the hierarchy too.
			s1, self1 := IOSLayout(p)
			s2, self2 := IOSLayout(n)
			if s1 != s2 || self1 != self2 {
				out.Layout = true
			}
		}
		if out.Screen == 0 && n != nil && IOSModal(n) && (p == nil || !IOSModal(p)) {
			out.Screen = id
			out.Layout = true
		}
	}
	return out
}
