package a11ybridge

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

func TestMappingsResolve(t *testing.T) {
	for role, p := range a11y.Mappings {
		if UIAControlTypeID(p.UIA) == 0 {
			t.Errorf("%s: UIA control type %q unknown", role, p.UIA)
		}
		if _, ok := ATSPIRoleID(p.ATSPI); !ok {
			t.Errorf("%s: AT-SPI role %q unknown", role, p.ATSPI)
		}
		n := &a11y.Node{Role: role}
		r, sub := MacRole(n)
		if !strings.HasPrefix(r, "AX") || (sub != "" && !strings.HasPrefix(sub, "AX")) {
			t.Errorf("%s: mac role %q/%q", role, r, sub)
		}
		if want, _, _ := strings.Cut(p.Mac, "/"); r != want {
			t.Errorf("%s: mac role %q, want %q", role, r, want)
		}
	}
}

func TestRoleRefinements(t *testing.T) {
	toggle := &a11y.Node{Role: a11y.RoleButton, States: a11y.Toggleable | a11y.Pressed}
	if r, s := MacRole(toggle); r != "AXCheckBox" || s != "AXToggle" {
		t.Errorf("toggle button: %s/%s", r, s)
	}
	if ATSPIRole(toggle) != 62 {
		t.Errorf("toggle button AT-SPI role %d", ATSPIRole(toggle))
	}
	if !slices.Contains(UIAPatterns(toggle), UIATogglePattern) || ToggleState(toggle) != 1 {
		t.Errorf("toggle button UIA: %v %d", UIAPatterns(toggle), ToggleState(toggle))
	}
	pw := &a11y.Node{Role: a11y.RoleTextbox, States: a11y.Protected}
	if r, s := MacRole(pw); r != "AXTextField" || s != "AXSecureTextField" {
		t.Errorf("password: %s/%s", r, s)
	}
	if ATSPIRole(pw) != 40 {
		t.Errorf("password AT-SPI %d", ATSPIRole(pw))
	}
	ml := &a11y.Node{Role: a11y.RoleTextbox, States: a11y.Multiline}
	if r, _ := MacRole(ml); r != "AXTextArea" {
		t.Errorf("multiline: %s", r)
	}
	if ATSPIRole(ml) != 61 {
		t.Errorf("multiline AT-SPI %d", ATSPIRole(ml))
	}
	btn := &a11y.Node{Role: a11y.RoleButton, Actions: a11y.Press}
	if UIAControlType(btn) != 50000 || !slices.Equal(UIAPatterns(btn), []int32{UIAInvokePattern}) {
		t.Errorf("button UIA %d %v", UIAControlType(btn), UIAPatterns(btn))
	}
	if acts := MacActions(btn); len(acts) != 1 || acts[0].Name != "AXPress" {
		t.Errorf("button mac actions %v", acts)
	}
	mixed := &a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Checkable | a11y.Checked | a11y.Mixed}
	if CheckValue(mixed) != 2 || !MacToggle(mixed) {
		t.Errorf("mixed checkbox %d", CheckValue(mixed))
	}
}

func TestATSPIStates(t *testing.T) {
	n := &a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Focusable | a11y.Focused | a11y.Checkable | a11y.Checked}
	has := func(set [2]uint32, s int) bool { return set[s/32]&(1<<(s%32)) != 0 }
	set := ATSPIStates(n, true)
	for _, s := range []int{StateEnabled, StateSensitive, StateShowing, StateVisible, StateFocusable, StateFocused, StateCheckable, StateChecked} {
		if !has(set, s) {
			t.Errorf("state %d missing", s)
		}
	}
	if has(ATSPIStates(n, false), StateFocused) {
		t.Error("FOCUSED set while the canvas has no focus")
	}
	d := ATSPIStates(&a11y.Node{Role: a11y.RoleButton, States: a11y.Disabled | a11y.Offscreen}, false)
	if has(d, StateEnabled) || has(d, StateShowing) {
		t.Error("disabled/offscreen")
	}
	tb := ATSPIStates(&a11y.Node{Role: a11y.RoleTextbox, States: a11y.Required | a11y.Invalid}, false)
	for _, s := range []int{StateEditable, StateSingleLine, StateRequired, StateInvalidEntry} {
		if !has(tb, s) {
			t.Errorf("textbox state %d missing", s)
		}
	}
}

// tree builds: root(1) [button(2) "OK", group(3, live polite) [text(4) "a"], dialog(5) [button(6) "X"]]
func sample() *a11y.Tree {
	t := a11y.NewTree(1)
	t.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 3, 5}, Bounds: a11y.Rect{W: 400, H: 300}})
	t.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "OK", Bounds: a11y.Rect{X: 10, Y: 10, W: 80, H: 30}})
	t.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleGroup, Live: a11y.LivePolite, Children: []a11y.NodeID{4}, Bounds: a11y.Rect{X: 0, Y: 100, W: 400, H: 50}})
	t.Add(&a11y.Node{ID: 4, Parent: 3, Role: a11y.RoleText, Name: "a", Bounds: a11y.Rect{X: 0, Y: 100, W: 50, H: 20}})
	t.Add(&a11y.Node{ID: 5, Parent: 1, Role: a11y.RoleDialog, Children: []a11y.NodeID{6}, Bounds: a11y.Rect{X: 0, Y: 0, W: 200, H: 90}})
	t.Add(&a11y.Node{ID: 6, Parent: 5, Role: a11y.RoleButton, Name: "X", Bounds: a11y.Rect{X: 20, Y: 15, W: 20, H: 20}})
	return t
}

func TestHitTest(t *testing.T) {
	tr := sample()
	cases := []struct {
		x, y float32
		want a11y.NodeID
	}{
		{25, 20, 6},   // the dialog's button is above the page button
		{50, 20, 5},   // inside the dialog, which covers the page button
		{300, 120, 3}, // live group, outside its text
		{10, 105, 4},
		{390, 290, 1}, // root only
		{500, 10, 0},  // outside
	}
	for _, c := range cases {
		if got := HitTest(tr, c.x, c.y); got != c.want {
			t.Errorf("HitTest(%v,%v) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
	tr.Node(5).States |= a11y.Offscreen
	if got := HitTest(tr, 25, 20); got != 2 {
		t.Errorf("offscreen dialog: hit %d, want 2", got)
	}
}

func TestCoordinates(t *testing.T) {
	r := ToDevice(a11y.Rect{X: 10.25, Y: 5, W: 20.5, H: 10}, 1.5, 100, 200)
	if r != (DeviceRect{X: 115, Y: 208, W: 31, H: 15}) { // 15.375→15, 46.125→46; 7.5→8, 22.5→23
		t.Errorf("ToDevice = %+v", r)
	}
	x, y := FromDevice(115, 208, 1.5, 100, 200)
	if x != 10 || y < 5.33 || y > 5.34 {
		t.Errorf("FromDevice = %v,%v", x, y)
	}
	if r := ToDevice(a11y.Rect{X: 1, Y: 1, W: 2, H: 2}, 2, 0, 0); r != (DeviceRect{2, 2, 4, 4}) {
		t.Errorf("retina %+v", r)
	}
	if x, y, w, h := Offset(a11y.Rect{X: 1, Y: 2, W: 3, H: 4}, 10, 20); x != 11 || y != 22 || w != 3 || h != 4 {
		t.Errorf("Offset %d %d %d %d", x, y, w, h)
	}
}

func TestFocusAndSiblings(t *testing.T) {
	tr := sample()
	if EffectiveFocus(tr) != 0 {
		t.Error("no focus")
	}
	tr.Focus = 2
	if EffectiveFocus(tr) != 2 {
		t.Error("focus 2")
	}
	tr.Node(2).ActiveDescendant = 6
	if EffectiveFocus(tr) != 6 {
		t.Error("active descendant")
	}
	tr.Node(2).ActiveDescendant = 99
	if EffectiveFocus(tr) != 2 {
		t.Error("missing active descendant")
	}
	if IndexInParent(tr, 5) != 2 || IndexInParent(tr, 1) != -1 {
		t.Error("IndexInParent")
	}
	if Sibling(tr, 3, 1) != 5 || Sibling(tr, 3, -1) != 2 || Sibling(tr, 5, 1) != 0 {
		t.Error("Sibling")
	}
}

func TestLiveChanges(t *testing.T) {
	prev := sample()
	next := sample()
	next.Node(4).Name = "b"
	changed := a11y.Diff(prev, next)
	got := LiveChanges(prev, next, changed)
	if len(got) != 1 || got[0].Root != 3 || got[0].Text != "b" || got[0].Live != a11y.LivePolite {
		t.Fatalf("LiveChanges = %+v", got)
	}
	if LiveChanges(nil, next, a11y.Diff(nil, next)) != nil {
		t.Error("first tree must not announce")
	}
	// Unchanged text (only bounds moved): nothing.
	next2 := sample()
	next2.Node(4).Bounds.X = 5
	if got := LiveChanges(prev, next2, a11y.Diff(prev, next2)); len(got) != 0 {
		t.Errorf("bounds only: %+v", got)
	}
	// Removed child inside the live region: the region reports its new text.
	next3 := sample()
	next3.Node(3).Children = nil
	next3.Remove(4)
	next3.Node(3).Name = "empty now"
	if got := LiveChanges(prev, next3, a11y.Diff(prev, next3)); len(got) != 1 || got[0].Text != "empty now" {
		t.Errorf("removed child: %+v", got)
	}
}

func TestDiffKinds(t *testing.T) {
	a := &a11y.Node{ID: 1, Name: "a", Value: "1", States: a11y.Focusable}
	b := *a
	if Diff(a, &b) != 0 {
		t.Error("equal")
	}
	b.Value = "2"
	b.States |= a11y.Focused
	if c := Diff(a, &b); !c.Has(ChangeValue) || !c.Has(ChangeStates) || c.Has(ChangeName) {
		t.Errorf("diff %b", c)
	}
	if Diff(nil, a) != ChangeAdded || Diff(a, nil) != ChangeRemoved {
		t.Error("added/removed")
	}
	b = *a
	b.Level = 2
	if Diff(a, &b) != ChangeOther {
		t.Error("other")
	}
}

func TestRangeAction(t *testing.T) {
	n := &a11y.Node{HasRange: true, ValueNow: 5, Actions: a11y.Increment | a11y.Decrement}
	if RangeAction(n, 7) != a11y.Increment || RangeAction(n, 3) != a11y.Decrement || RangeAction(n, 5) != 0 {
		t.Error("step actions")
	}
	n.Actions |= a11y.SetValue
	if RangeAction(n, 7) != a11y.SetValue {
		t.Error("set value")
	}
}
