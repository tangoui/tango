package windows

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

// TestUIAVtableLayout pins the slot numbers to UIAutomationCore.h: the
// methods each interface declares after IUnknown, in order.
func TestUIAVtableLayout(t *testing.T) {
	cases := []struct {
		name  string
		first int
		slots int
		want  int // number of own methods
	}{
		{"IRawElementProviderSimple", uiaSimpleGetProviderOptions, uiaSimpleSlots, 4},
		{"IRawElementProviderFragment", uiaFragNavigate, uiaFragSlots, 6},
		{"IRawElementProviderFragmentRoot", uiaRootElementProviderFromPoint, uiaRootSlots, 2},
		{"IInvokeProvider", uiaInvokeInvoke, uiaInvokeSlots, 1},
		{"IToggleProvider", uiaToggleToggle, uiaToggleSlots, 2},
		{"IValueProvider", uiaValueSetValue, uiaValueSlots, 3},
		{"IRangeValueProvider", uiaRangeSetValue, uiaRangeSlots, 7},
		{"IExpandCollapseProvider", uiaExpandExpand, uiaExpandSlots, 3},
		{"ISelectionItemProvider", uiaSelSelect, uiaSelSlots, 5},
		{"IScrollItemProvider", uiaScrollItemScrollIntoView, uiaScrollItemSlots, 1},
	}
	for _, c := range cases {
		if c.first != 3 || c.slots != 3+c.want {
			t.Errorf("%s: first %d, %d slots; want %d, %d", c.name, c.first, c.slots, 3, 3+c.want)
		}
	}
	// Spot checks of the order inside the interfaces.
	if uiaSimpleGetPropertyValue != 5 || uiaSimpleGetHostRawElementProvider != 6 {
		t.Error("IRawElementProviderSimple order")
	}
	if uiaFragGetBoundingRectangle != 5 || uiaFragSetFocus != 7 || uiaFragGetFragmentRoot != 8 {
		t.Error("IRawElementProviderFragment order")
	}
	if uiaRootGetFocus != 4 || uiaRangeGetSmallChange != 9 || uiaSelGetContainer != 7 {
		t.Error("FragmentRoot / RangeValue / SelectionItem order")
	}
	for _, s := range []string{
		iidIRawElementProviderSimple, iidIRawElementProviderFragment, iidIRawElementProviderFragmentRoot,
		iidIInvokeProvider, iidIToggleProvider, iidIValueProvider, iidIRangeValueProvider,
		iidIExpandCollapseProvider, iidISelectionItemProvider, iidIScrollItemProvider, iidIUnknown,
	} {
		if _, ok := parseGUID(s); !ok {
			t.Errorf("bad IID %s", s)
		}
	}
}

func TestUIARuntimeID(t *testing.T) {
	if got := uiaRuntimeID(0x1234_0000_0005); got != [3]int32{3, 5, 0x1234} {
		t.Errorf("runtime id %v", got)
	}
	if got := uiaRuntimeID(0xFFFF_FFFF); got[1] != -1 || got[2] != 0 {
		t.Errorf("low half %v", got)
	}
}

func uiaTree() *a11y.Tree {
	tr := a11y.NewTree(1)
	tr.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 3, 4}})
	tr.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "OK", Actions: a11y.Press | a11y.Focus,
		States: a11y.Focusable | a11y.Focused, Shortcut: "Ctrl+O"})
	tr.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleTextbox, Value: "secret", Placeholder: "pw",
		States: a11y.Protected | a11y.Required | a11y.Invalid, Description: "too short", Actions: a11y.SetValue})
	tr.Add(&a11y.Node{ID: 4, Parent: 1, Role: a11y.RoleListbox, Children: []a11y.NodeID{5}})
	tr.Add(&a11y.Node{ID: 5, Parent: 4, Role: a11y.RoleOption, Name: "A", States: a11y.Selectable | a11y.Selected,
		PosInSet: 1, SetSize: 3, Actions: a11y.Press})
	tr.Focus = 2
	return tr
}

func TestUIAProperties(t *testing.T) {
	tr := uiaTree()
	btn, pw, opt := tr.Node(2), tr.Node(3), tr.Node(5)
	cases := []struct {
		n    *a11y.Node
		prop int32
		want uiaValue
	}{
		{btn, uiaPropControlType, vI4(50000)},
		{btn, uiaPropName, vStr("OK")},
		{btn, uiaPropHasKeyboardFocus, vBool(true)},
		{btn, uiaPropIsKeyboardFocusable, vBool(true)},
		{btn, uiaPropIsEnabled, vBool(true)},
		{btn, uiaPropAutomationID, vStr("2")},
		{btn, uiaPropAcceleratorKey, vStr("Ctrl+O")},
		{btn, uiaPropFrameworkID, vStr("Tango")},
		{pw, uiaPropIsPassword, vBool(true)},
		{pw, uiaPropValueValue, uiaValue{}}, // never the password
		{pw, uiaPropIsRequiredForForm, vBool(true)},
		{pw, uiaPropIsDataValidForForm, vBool(false)},
		{pw, uiaPropHelpText, vStr("too short")},
		{opt, uiaPropPositionInSet, vI4(1)},
		{opt, uiaPropSizeOfSet, vI4(3)},
		{opt, uiaPropLevel, uiaValue{}},
	}
	for _, c := range cases {
		if got := uiaProperty(tr, c.n, c.prop, true, false); got != c.want {
			t.Errorf("%s prop %d = %+v, want %+v", c.n, c.prop, got, c.want)
		}
	}
	if got := uiaProperty(tr, btn, uiaPropHasKeyboardFocus, false, false); got != vBool(false) {
		t.Error("focus without window focus")
	}
	if got := uiaProperty(tr, tr.Node(1), uiaPropAutomationID, true, true); got.Kind != uiaEmpty {
		t.Error("root automation id")
	}
	if !uiaHasPattern(btn, a11ybridge.UIAInvokePattern) || !uiaHasPattern(opt, a11ybridge.UIASelectionItemPattern) ||
		!uiaHasPattern(pw, a11ybridge.UIAValuePattern) {
		t.Error("patterns")
	}
	if uiaSelectionContainer(tr, 5) != 4 || uiaSelectionContainer(tr, 2) != 0 {
		t.Error("selection container")
	}
}

func TestUIANavigate(t *testing.T) {
	tr := uiaTree()
	cases := []struct {
		id   a11y.NodeID
		dir  int32
		want a11y.NodeID
	}{
		{1, uiaNavParent, 0}, {1, uiaNavNextSibling, 0}, {1, uiaNavFirstChild, 2}, {1, uiaNavLastChild, 4},
		{2, uiaNavParent, 1}, {2, uiaNavNextSibling, 3}, {2, uiaNavPreviousSibling, 0},
		{4, uiaNavFirstChild, 5}, {5, uiaNavParent, 4}, {5, uiaNavFirstChild, 0}, {99, uiaNavParent, 0},
	}
	for _, c := range cases {
		if got := uiaNavigate(tr, c.id, c.dir); got != c.want {
			t.Errorf("navigate(%d, %d) = %d, want %d", c.id, c.dir, got, c.want)
		}
	}
}

func TestUIAPropertyEvents(t *testing.T) {
	n := &a11y.Node{HasRange: true}
	ev := uiaPropertyEvents(a11ybridge.ChangeValue|a11ybridge.ChangeName, n)
	want := []int32{uiaPropName, uiaPropHelpText, uiaPropRangeValueValue}
	if len(ev) != len(want) {
		t.Fatalf("events %v", ev)
	}
	for i := range want {
		if ev[i] != want[i] {
			t.Errorf("events %v, want %v", ev, want)
		}
	}
	if uiaProcessing(a11y.LiveAssertive) != uiaProcessingImportantMostRecent || uiaProcessing(a11y.LivePolite) != uiaProcessingMostRecent {
		t.Error("processing")
	}
}

func TestUIAThunks(t *testing.T) {
	const target = 0x1122334455667788
	addr := binary.LittleEndian.AppendUint64(nil, target)

	amd := thunkFromPoint("amd64", target)
	wantAMD := append([]byte{0x66, 0x48, 0x0F, 0x7E, 0xCA, 0x66, 0x49, 0x0F, 0x7E, 0xD0, 0xFF, 0x25, 0, 0, 0, 0}, addr...)
	if !bytes.Equal(amd, wantAMD) {
		t.Errorf("amd64 from-point thunk % x", amd)
	}
	if one := thunkOneDouble("amd64", target); !bytes.Equal(one, append([]byte{0x66, 0x48, 0x0F, 0x7E, 0xCA, 0xFF, 0x25, 0, 0, 0, 0}, addr...)) {
		t.Errorf("amd64 one-double thunk % x", one)
	}

	arm := thunkFromPoint("arm64", target)
	if len(arm) != 5*4+8 || !bytes.Equal(arm[20:], addr) {
		t.Fatalf("arm64 thunk layout % x", arm)
	}
	words := []uint32{0xAA0103E3, 0x9E660001, 0x9E660022, 0x58000050, 0xD61F0200}
	for i, w := range words {
		if got := binary.LittleEndian.Uint32(arm[4*i:]); got != w {
			t.Errorf("arm64 insn %d = %#08x, want %#08x", i, got, w)
		}
	}
	// ldr x16 is 8 bytes before the literal (imm19 = 2 words).
	ldr := binary.LittleEndian.Uint32(arm[12:])
	if imm := (ldr >> 5) & 0x7FFFF; 12+int(imm)*4 != 20 || ldr&0x1F != 16 {
		t.Errorf("ldr literal offset %d", imm)
	}
	if one := thunkOneDouble("arm64", target); len(one) != 3*4+8 || binary.LittleEndian.Uint32(one) != 0x9E660001 {
		t.Errorf("arm64 one-double thunk % x", one)
	}
	if thunkFromPoint("386", target) != nil {
		t.Error("unsupported arch")
	}
	if f64(uintptr(math.Float64bits(12.5))) != 12.5 {
		t.Error("f64")
	}
	if uitoa(0) != "0" || uitoa(18446744073709551615) != "18446744073709551615" {
		t.Error("uitoa")
	}
}
