package a11ybridge

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

func TestIOSMappingsResolve(t *testing.T) {
	for role, p := range a11y.Mappings {
		traits, container := iosMapping(role)
		s, _, _ := strings.Cut(p.IOS, "(")
		s = strings.TrimSpace(s)
		switch s {
		case "container", "none":
			if traits != 0 {
				t.Errorf("%s: %q gave traits %s", role, p.IOS, traits)
			}
		default:
			// Every named trait must resolve to a bit.
			if got := strings.Count(s, "+") + 1; got != len(strings.Split(traits.String(), "|")) {
				t.Errorf("%s: %q resolved to %s", role, p.IOS, traits)
			}
		}
		if container != (s == "container" || role == a11y.RoleTabList) {
			t.Errorf("%s: container %v for %q", role, container, p.IOS)
		}
	}
}

func TestIOSTraits(t *testing.T) {
	cases := []struct {
		n    a11y.Node
		want IOSTraitSet
	}{
		{a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press}, IOSButton},
		{a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press, States: a11y.Disabled}, IOSButton | IOSNotEnabled},
		{a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press, States: a11y.Toggleable | a11y.Pressed}, IOSButton | IOSToggleButton},
		{a11y.Node{Role: a11y.RoleSwitch, Actions: a11y.Press, States: a11y.Checkable | a11y.Checked}, IOSButton | IOSToggleButton},
		{a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Checkable}, IOSButton | IOSToggleButton},
		{a11y.Node{Role: a11y.RoleRadio, States: a11y.Checkable | a11y.Checked}, IOSButton | IOSSelected},
		{a11y.Node{Role: a11y.RoleTab, Actions: a11y.Press | a11y.Focus, States: a11y.Selectable | a11y.Selected}, IOSButton | IOSSelected},
		{a11y.Node{Role: a11y.RoleHeading}, IOSHeader | IOSStaticText},
		{a11y.Node{Role: a11y.RoleColumnHeader, Actions: a11y.Press}, IOSHeader | IOSButton},
		{a11y.Node{Role: a11y.RoleLink, Actions: a11y.Press}, IOSLink},
		{a11y.Node{Role: a11y.RoleText}, IOSStaticText},
		{a11y.Node{Role: a11y.RoleImage}, IOSImage},
		{a11y.Node{Role: a11y.RoleSearchbox}, IOSSearchField},
		{a11y.Node{Role: a11y.RoleTextbox}, 0},
		{a11y.Node{Role: a11y.RoleSlider, Actions: a11y.Increment | a11y.Decrement}, IOSAdjustable},
		{a11y.Node{Role: a11y.RoleSeparator}, 0},
		{a11y.Node{Role: a11y.RoleSeparator, Actions: a11y.Increment}, IOSAdjustable},
		{a11y.Node{Role: a11y.RoleProgressBar}, IOSUpdatesFrequently},
		{a11y.Node{Role: a11y.RoleTimer}, IOSStaticText | IOSUpdatesFrequently},
		{a11y.Node{Role: a11y.RoleTabList}, IOSTabBar},
		{a11y.Node{Role: a11y.RoleListItem, Actions: a11y.Press}, IOSButton},
		{a11y.Node{Role: a11y.RoleListItem}, IOSStaticText},
		{a11y.Node{Role: a11y.RoleList, States: a11y.Busy}, IOSUpdatesFrequently},
		{a11y.Node{Role: a11y.RoleLink, States: a11y.Current}, IOSLink | IOSSelected},
	}
	for _, c := range cases {
		if got := IOSTraits(&c.n); got != c.want {
			t.Errorf("%s: traits %s, want %s", c.n.String(), got, c.want)
		}
	}
}

func TestIOSLayout(t *testing.T) {
	b := a11y.Rect{W: 10, H: 10}
	cases := []struct {
		n     a11y.Node
		shape IOSShape
		self  bool
	}{
		{a11y.Node{Role: a11y.RoleButton, Bounds: b}, IOSElement, false},
		{a11y.Node{Role: a11y.RoleButton}, IOSSkip, false}, // no area
		{a11y.Node{Role: a11y.RoleList, Name: "Net", Children: []a11y.NodeID{2}, Bounds: b}, IOSContainer, false},
		{a11y.Node{Role: a11y.RoleList, Bounds: b}, IOSSkip, false},
		{a11y.Node{Role: a11y.RoleGroup, Name: "Card", Bounds: b}, IOSElement, false},
		{a11y.Node{Role: a11y.RoleTabList, Children: []a11y.NodeID{2}}, IOSContainer, false},
		{a11y.Node{Role: a11y.RoleListItem, Name: "Wi-Fi", Children: []a11y.NodeID{2}, Bounds: b}, IOSContainer, true},
		{a11y.Node{Role: a11y.RoleListItem, Children: []a11y.NodeID{2}, Bounds: b}, IOSContainer, false},
		{a11y.Node{Role: a11y.RoleButton, Name: "OK", Children: []a11y.NodeID{2}, Bounds: b}, IOSElement, false}, // atomic
	}
	for _, c := range cases {
		s, self := IOSLayout(&c.n)
		if s != c.shape || self != c.self {
			t.Errorf("%s: shape %d self %v, want %d %v", c.n.String(), s, self, c.shape, c.self)
		}
	}
	if k := IOSContainerKind(&a11y.Node{Role: a11y.RoleList}); k != IOSContainerList {
		t.Errorf("list container %d", k)
	}
	if k := IOSContainerKind(&a11y.Node{Role: a11y.RoleGrid}); k != IOSContainerDataTable {
		t.Errorf("grid container %d", k)
	}
	if k := IOSContainerKind(&a11y.Node{Role: a11y.RoleNavigation}); k != IOSContainerLandmark {
		t.Errorf("navigation container %d", k)
	}
	if k := IOSContainerKind(&a11y.Node{Role: a11y.RoleDialog}); k != IOSContainerSemanticGroup {
		t.Errorf("dialog container %d", k)
	}
	if !IOSModal(&a11y.Node{Role: a11y.RoleDialog, States: a11y.Modal}) || IOSModal(&a11y.Node{Role: a11y.RoleDialog}) {
		t.Error("modal")
	}
}

func TestIOSStrings(t *testing.T) {
	cases := []struct {
		n    a11y.Node
		want IOSText
	}{
		{a11y.Node{Role: a11y.RoleButton, Name: "Count", Value: "3", Description: "Adds one"}, IOSText{"Count", "3", "Adds one"}},
		{a11y.Node{Role: a11y.RoleSwitch, Name: "Wi-Fi", States: a11y.Checkable | a11y.Checked}, IOSText{"Wi-Fi", "1", ""}},
		{a11y.Node{Role: a11y.RoleSwitch, Name: "BT", States: a11y.Checkable}, IOSText{"BT", "0", ""}},
		{a11y.Node{Role: a11y.RoleCheckbox, Name: "All", States: a11y.Checkable | a11y.Mixed}, IOSText{"All", "mixed", ""}},
		{a11y.Node{Role: a11y.RoleButton, Name: "Bold", States: a11y.Toggleable | a11y.Pressed}, IOSText{"Bold", "1", ""}},
		{a11y.Node{Role: a11y.RoleTextbox, Name: "Pass", Value: "secret", States: a11y.Protected}, IOSText{"Pass", "", ""}},
		{a11y.Node{Role: a11y.RoleTextbox, Placeholder: "Search"}, IOSText{"Search", "", ""}},
		{a11y.Node{Role: a11y.RoleTextbox, Name: "Mail", Placeholder: "a@b.c"}, IOSText{"Mail", "a@b.c", ""}},
		{a11y.Node{Role: a11y.RoleTextbox, Name: "Mail", Value: "x", States: a11y.Required | a11y.Invalid, Description: "Bad address"},
			IOSText{"Mail", "x, invalid", "Bad address, required"}},
		{a11y.Node{Role: a11y.RoleCombobox, Name: "City", Value: "Paris", States: a11y.Expandable}, IOSText{"City", "Paris, collapsed", ""}},
		{a11y.Node{Role: a11y.RoleTreeItem, Name: "src", States: a11y.Expandable | a11y.Expanded}, IOSText{"src", "expanded", ""}},
		{a11y.Node{Role: a11y.RoleSlider, Name: "Vol", HasRange: true, ValueNow: 2.5, ValueMax: 10}, IOSText{"Vol", "2.5", ""}},
		{a11y.Node{Role: a11y.RoleProgressBar, HasRange: true, ValueNow: 0.25, ValueMax: 1}, IOSText{"", "25%", ""}},
		{a11y.Node{Role: a11y.RoleListItem, Name: "Platform", Value: "ios/arm64"}, IOSText{"Platform", "ios/arm64", ""}},
	}
	for _, c := range cases {
		if got := IOSStrings(&c.n); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.n.String(), got, c.want)
		}
	}
}

// Editable text inputs are marked so the iOS bridge can hand them to
// UIKit as text (accessibilityTextInputResponder, iOS 18.1+) or, on older
// systems, tell VoiceOver users they can edit them.
func TestIOSEditable(t *testing.T) {
	cases := []struct {
		n    a11y.Node
		want bool
	}{
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus}, true},
		{a11y.Node{Role: a11y.RoleSearchbox, Actions: a11y.SetValue | a11y.Focus}, true},
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus, States: a11y.Protected}, true},
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus, States: a11y.Multiline}, true},
		{a11y.Node{Role: a11y.RoleCombobox, Actions: a11y.SetValue | a11y.Focus, States: a11y.Editable}, true},
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.Focus, States: a11y.Disabled}, false},
		{a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.Focus, States: a11y.ReadOnly}, false},
		{a11y.Node{Role: a11y.RoleCombobox, Actions: a11y.Press}, false},
		{a11y.Node{Role: a11y.RoleSpinButton, Actions: a11y.Increment | a11y.Decrement | a11y.SetValue}, false},
		{a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press}, false},
	}
	for _, c := range cases {
		if got := IOSEditable(&c.n); got != c.want {
			t.Errorf("%s: editable %v, want %v", c.n.String(), got, c.want)
		}
	}
	// Without the iOS 18.1 text responder the hint says how to edit, after
	// the node's own description; with it UIKit speaks the field as text
	// and the hint stays as it was.
	n := &a11y.Node{Role: a11y.RoleTextbox, Name: "Mail", Description: "Work address", Actions: a11y.SetValue | a11y.Focus}
	if got := IOSEditHint(IOSStrings(n).Hint, n, false); got != "Work address, "+IOSPhrases.EditHint {
		t.Errorf("legacy hint %q", got)
	}
	if got := IOSEditHint(IOSStrings(n).Hint, n, true); got != "Work address" {
		t.Errorf("hint with text responder %q", got)
	}
	plain := &a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus}
	if got := IOSEditHint("", plain, false); got != IOSPhrases.EditHint {
		t.Errorf("legacy hint without description %q", got)
	}
	btn := &a11y.Node{Role: a11y.RoleButton, Description: "Adds one", Actions: a11y.Press}
	if got := IOSEditHint("Adds one", btn, false); got != "Adds one" {
		t.Errorf("button hint changed: %q", got)
	}
}

func TestIOSActions(t *testing.T) {
	btn := IOSActions(&a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press | a11y.Focus})
	if btn.Activate != a11y.Press || len(btn.Custom) != 1 || btn.Custom[0].Act != a11y.Focus {
		t.Errorf("button %+v", btn)
	}
	tab := IOSActions(&a11y.Node{Role: a11y.RoleTab, Actions: a11y.Press | a11y.Focus, States: a11y.Selectable})
	if tab.Activate != a11y.Press || len(tab.Custom) != 0 {
		t.Errorf("tab %+v", tab)
	}
	field := IOSActions(&a11y.Node{Role: a11y.RoleTextbox, Actions: a11y.SetValue | a11y.Focus, States: a11y.Focusable})
	if field.Activate != a11y.Focus || len(field.Custom) != 0 {
		t.Errorf("text field %+v", field)
	}
	tree := IOSActions(&a11y.Node{Role: a11y.RoleTreeItem, Actions: a11y.Expand | a11y.Collapse, States: a11y.Expandable})
	if tree.Activate != a11y.Expand || len(tree.Custom) != 0 {
		t.Errorf("collapsed tree item %+v", tree)
	}
	open := IOSActions(&a11y.Node{Role: a11y.RoleTreeItem, Actions: a11y.Expand | a11y.Collapse, States: a11y.Expandable | a11y.Expanded})
	if open.Activate != a11y.Collapse {
		t.Errorf("expanded tree item %+v", open)
	}
	combo := IOSActions(&a11y.Node{Role: a11y.RoleCombobox, Actions: a11y.Press | a11y.Expand | a11y.Collapse, States: a11y.Expandable})
	if combo.Activate != a11y.Press || len(combo.Custom) != 1 || combo.Custom[0].Act != a11y.Expand || combo.Custom[0].Name != "Expand" {
		t.Errorf("combobox %+v", combo)
	}
	slider := IOSActions(&a11y.Node{Role: a11y.RoleSlider, Actions: a11y.Increment | a11y.Decrement})
	if slider.Activate != 0 || !slider.Increment || !slider.Decrement {
		t.Errorf("slider %+v", slider)
	}
	if none := IOSActions(&a11y.Node{Role: a11y.RoleText}); none.Activate != 0 || none.Custom != nil {
		t.Errorf("text %+v", none)
	}
}

// tree builds a tree from nodes given parent-first; Children are derived
// from Parent.
func iosTree(nodes ...*a11y.Node) *a11y.Tree {
	t := a11y.NewTree(nodes[0].ID)
	byID := map[a11y.NodeID]*a11y.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
		if p := byID[n.Parent]; p != nil && n.Parent != 0 {
			p.Children = append(p.Children, n.ID)
		}
		t.Add(n)
	}
	return t
}

func TestIOSEscapeAndScroll(t *testing.T) {
	tr := iosTree(
		&a11y.Node{ID: 1, Role: a11y.RoleWindow},
		&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleDialog, Actions: a11y.Dismiss, States: a11y.Modal},
		&a11y.Node{ID: 3, Parent: 2, Role: a11y.RoleButton, Actions: a11y.Press},
		&a11y.Node{ID: 4, Parent: 1, Role: a11y.RoleList},
		&a11y.Node{ID: 5, Parent: 4, Role: a11y.RoleListItem, States: a11y.Offscreen, Actions: a11y.ScrollIntoView},
		&a11y.Node{ID: 6, Parent: 4, Role: a11y.RoleListItem, Actions: a11y.ScrollIntoView},
		&a11y.Node{ID: 7, Parent: 4, Role: a11y.RoleListItem, Actions: a11y.ScrollIntoView},
		&a11y.Node{ID: 8, Parent: 4, Role: a11y.RoleListItem, States: a11y.Offscreen},
		&a11y.Node{ID: 9, Parent: 4, Role: a11y.RoleListItem, States: a11y.Offscreen, Actions: a11y.ScrollIntoView},
	)
	if got := IOSEscapeTarget(tr, 3); got != 2 {
		t.Errorf("escape from the dialog's button: %d", got)
	}
	if got := IOSEscapeTarget(tr, 6); got != 0 {
		t.Errorf("escape outside a dialog: %d", got)
	}
	if got := IOSScrollTarget(tr, 6, true); got != 9 {
		t.Errorf("scroll forward: %d", got)
	}
	if got := IOSScrollTarget(tr, 7, false); got != 5 {
		t.Errorf("scroll backward: %d", got)
	}
	if got := IOSScrollTarget(tr, 3, true); got != 0 {
		t.Errorf("scroll outside a scroller: %d", got)
	}
}

func TestIOSNotify(t *testing.T) {
	b := a11y.Rect{W: 10, H: 10}
	prev := iosTree(
		&a11y.Node{ID: 1, Role: a11y.RoleWindow, Bounds: b},
		&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "Count", Value: "0", Bounds: b},
	)
	value := iosTree(
		&a11y.Node{ID: 1, Role: a11y.RoleWindow, Bounds: b},
		&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "Count", Value: "1", Bounds: b},
	)
	if n := IOSNotify(prev, value, a11y.Diff(prev, value)); n.Layout || n.Screen != 0 {
		t.Errorf("value change: %+v", n)
	}
	dialog := iosTree(
		&a11y.Node{ID: 1, Role: a11y.RoleWindow, Bounds: b},
		&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "Count", Value: "0", Bounds: b},
		&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleDialog, States: a11y.Modal, Bounds: b},
		&a11y.Node{ID: 4, Parent: 3, Role: a11y.RoleButton, Name: "OK", Bounds: b},
	)
	if n := IOSNotify(prev, dialog, a11y.Diff(prev, dialog)); !n.Layout || n.Screen != 3 {
		t.Errorf("dialog opened: %+v", n)
	}
	if n := IOSNotify(dialog, prev, a11y.Diff(dialog, prev)); !n.Layout || n.Screen != 0 {
		t.Errorf("dialog closed: %+v", n)
	}
	hidden := iosTree(
		&a11y.Node{ID: 1, Role: a11y.RoleWindow, Bounds: b},
		&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "Count", Value: "0"},
	)
	if n := IOSNotify(prev, hidden, a11y.Diff(prev, hidden)); !n.Layout {
		t.Errorf("element lost its area: %+v", n)
	}
}
