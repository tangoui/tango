package windows

// Platform-independent parts of the UI Automation bridge of canvases
// (canvas_a11y_windows.go, docs/specs/a11y-bridge.md): the UIA constants
// and vtable layouts used there, runtime IDs, property values, tree
// navigation and the machine-code thunks that hand floating-point
// arguments to Go callbacks. Kept free of build tags so they are tested on
// every OS. Names and values follow UIAutomationCore.h,
// UIAutomationClient.h and UIAutomationCoreApi.h of the Windows SDK.

import (
	"encoding/binary"
	"math"

	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

// Vtable slots (after IUnknown's three, see com_windows.go), in the
// declaration order of UIAutomationCore.h.
const (
	// IRawElementProviderSimple
	uiaSimpleGetProviderOptions = 3 + iota
	uiaSimpleGetPatternProvider
	uiaSimpleGetPropertyValue
	uiaSimpleGetHostRawElementProvider
	uiaSimpleSlots // 7
)

const (
	// IRawElementProviderFragment
	uiaFragNavigate = 3 + iota
	uiaFragGetRuntimeId
	uiaFragGetBoundingRectangle
	uiaFragGetEmbeddedFragmentRoots
	uiaFragSetFocus
	uiaFragGetFragmentRoot
	uiaFragSlots // 9
)

const (
	// IRawElementProviderFragmentRoot
	uiaRootElementProviderFromPoint = 3 + iota
	uiaRootGetFocus
	uiaRootSlots // 5
)

const (
	uiaInvokeInvoke = 3 // IInvokeProvider
	uiaInvokeSlots  = 4

	// IToggleProvider
	uiaToggleToggle         = 3
	uiaToggleGetToggleState = 4
	uiaToggleSlots          = 5

	// IValueProvider
	uiaValueSetValue      = 3
	uiaValueGetValue      = 4
	uiaValueGetIsReadOnly = 5
	uiaValueSlots         = 6

	// IRangeValueProvider
	uiaRangeSetValue       = 3
	uiaRangeGetValue       = 4
	uiaRangeGetIsReadOnly  = 5
	uiaRangeGetMaximum     = 6
	uiaRangeGetMinimum     = 7
	uiaRangeGetLargeChange = 8
	uiaRangeGetSmallChange = 9
	uiaRangeSlots          = 10

	// IExpandCollapseProvider
	uiaExpandExpand   = 3
	uiaExpandCollapse = 4
	uiaExpandGetState = 5
	uiaExpandSlots    = 6

	// ISelectionItemProvider
	uiaSelSelect              = 3
	uiaSelAddToSelection      = 4
	uiaSelRemoveFromSelection = 5
	uiaSelGetIsSelected       = 6
	uiaSelGetContainer        = 7
	uiaSelSlots               = 8

	// IScrollItemProvider
	uiaScrollItemScrollIntoView = 3
	uiaScrollItemSlots          = 4
)

// Interface IDs (UIAutomationCore.h).
const (
	iidIRawElementProviderSimple       = "d6dd68d1-86fd-4332-8666-9abedea2d24c"
	iidIRawElementProviderFragment     = "f7063da8-8359-439c-9297-bbc5299a7d87"
	iidIRawElementProviderFragmentRoot = "620ce2a5-ab8f-40a9-86cb-de3c75599b58"
	iidIInvokeProvider                 = "54fcb24b-e18e-47a2-b4d3-eccbe77599a2"
	iidIToggleProvider                 = "56d00bd0-c4f4-433c-a836-1a52a57e0892"
	iidIValueProvider                  = "c7935180-6fb3-4201-b174-7df73adbf64a"
	iidIRangeValueProvider             = "36dc7aef-33e6-4691-afe1-2be7274b3d33"
	iidIExpandCollapseProvider         = "d847d3a5-cab0-4a98-8c32-ecb45c59ad24"
	iidISelectionItemProvider          = "2acad808-b2d4-452d-a407-91ff1ad167b2"
	iidIScrollItemProvider             = "2360c714-4bf1-4b26-ba65-9b21316127eb"
	iidIUnknown                        = "00000000-0000-0000-C000-000000000046"
)

// ProviderOptions, NavigateDirection, StructureChangeType,
// NotificationKind / NotificationProcessing.
const (
	uiaProviderServerSide      = 0x2
	uiaProviderUseComThreading = 0x20

	uiaNavParent          = 0
	uiaNavNextSibling     = 1
	uiaNavPreviousSibling = 2
	uiaNavFirstChild      = 3
	uiaNavLastChild       = 4

	uiaStructChildAdded          = 0
	uiaStructChildRemoved        = 1
	uiaStructChildrenInvalidated = 2

	uiaNotificationOther                     = 4
	uiaProcessingImportantMostRecent         = 1
	uiaProcessingMostRecent                  = 3
	uiaAppendRuntimeID                       = 3   // UiaAppendRuntimeId
	uiaRootObjectID                          = -25 // UiaRootObjectId (lParam of WM_GETOBJECT)
	uiaEElementNotAvailable          uintptr = 0x80040201
	uiaENotSupported                 uintptr = 0x80040204 // UIA_E_NOTSUPPORTED
	uiaEInvalidOperation             uintptr = 0x80131509 // UIA_E_INVALIDOPERATION
	eInvalidArg                      uintptr = 0x80070057
	ePointer                         uintptr = 0x80004003
	eFail                            uintptr = 0x80004005
)

// Event IDs (UIA_<X>EventId).
const (
	uiaStructureChangedEventID       = 20002
	uiaAutomationFocusChangedEventID = 20005
	uiaLiveRegionChangedEventID      = 20024
)

// Property IDs (UIA_<X>PropertyId).
const (
	uiaPropControlType          = 30003
	uiaPropLocalizedControlType = 30004
	uiaPropName                 = 30005
	uiaPropAcceleratorKey       = 30006
	uiaPropHasKeyboardFocus     = 30008
	uiaPropIsKeyboardFocusable  = 30009
	uiaPropIsEnabled            = 30010
	uiaPropAutomationID         = 30011
	uiaPropClassName            = 30012
	uiaPropHelpText             = 30013
	uiaPropIsControlElement     = 30016
	uiaPropIsContentElement     = 30017
	uiaPropIsPassword           = 30019
	uiaPropIsOffscreen          = 30022
	uiaPropOrientation          = 30023
	uiaPropFrameworkID          = 30024
	uiaPropIsRequiredForForm    = 30025
	uiaPropItemStatus           = 30026
	uiaPropValueValue           = 30045
	uiaPropRangeValueValue      = 30047
	uiaPropExpandCollapseState  = 30070
	uiaPropToggleState          = 30086
	uiaPropIsDataValidForForm   = 30103
	uiaPropLiveSetting          = 30135
	uiaPropPositionInSet        = 30152
	uiaPropSizeOfSet            = 30153
	uiaPropLevel                = 30154
	uiaPropFullDescription      = 30159
	uiaPropIsDialog             = 30174
)

// uiaRuntimeID is the runtime ID of a node: UiaAppendRuntimeId followed by
// the two halves of its NodeID, which is unique within the canvas (UIA
// prefixes the host window's ID).
func uiaRuntimeID(id a11y.NodeID) [3]int32 {
	return [3]int32{uiaAppendRuntimeID, int32(uint32(id)), int32(uint32(uint64(id) >> 32))}
}

// uiaValueKind tags uiaValue.
type uiaValueKind uint8

const (
	uiaEmpty uiaValueKind = iota // VT_EMPTY: not supported, UIA uses the default
	uiaI4                        // VT_I4
	uiaBool                      // VT_BOOL
	uiaStr                       // VT_BSTR
	uiaR8                        // VT_R8
)

// uiaValue is a property value before it becomes a VARIANT.
type uiaValue struct {
	Kind uiaValueKind
	I    int32
	B    bool
	S    string
	F    float64
}

func vI4(v int32) uiaValue   { return uiaValue{Kind: uiaI4, I: v} }
func vBool(b bool) uiaValue  { return uiaValue{Kind: uiaBool, B: b} }
func vR8(f float64) uiaValue { return uiaValue{Kind: uiaR8, F: f} }
func vStr(s string) uiaValue {
	if s == "" {
		return uiaValue{}
	}
	return uiaValue{Kind: uiaStr, S: s}
}

// uiaProperty returns property prop of n. focused says whether the
// canvas window has keyboard focus; root whether n is the fragment root.
func uiaProperty(t *a11y.Tree, n *a11y.Node, prop int32, focused, root bool) uiaValue {
	st := n.States
	switch prop {
	case uiaPropControlType:
		return vI4(a11ybridge.UIAControlType(n))
	case uiaPropLocalizedControlType:
		return vStr(n.RoleDescription)
	case uiaPropName:
		name := n.Name
		if name == "" && n.LabelledBy != 0 {
			name = t.Text(n.LabelledBy)
		}
		return vStr(name)
	case uiaPropAcceleratorKey:
		return vStr(n.Shortcut)
	case uiaPropHasKeyboardFocus:
		return vBool(focused && a11ybridge.EffectiveFocus(t) == n.ID)
	case uiaPropIsKeyboardFocusable:
		return vBool(st.Has(a11y.Focusable))
	case uiaPropIsEnabled:
		return vBool(!st.Has(a11y.Disabled))
	case uiaPropAutomationID:
		if root {
			return uiaValue{}
		}
		return vStr(uitoa(uint64(n.ID)))
	case uiaPropClassName:
		if root {
			return vStr("TangoCanvas") // canvasClass
		}
	case uiaPropHelpText:
		if n.Placeholder != "" && n.Value == "" {
			return vStr(n.Placeholder)
		}
		return vStr(n.Description)
	case uiaPropFullDescription:
		return vStr(n.Description)
	case uiaPropIsControlElement, uiaPropIsContentElement:
		return vBool(true)
	case uiaPropIsPassword:
		return vBool(st.Has(a11y.Protected))
	case uiaPropIsOffscreen:
		return vBool(st.Has(a11y.Offscreen))
	case uiaPropOrientation: // OrientationType: 0 none, 1 horizontal, 2 vertical
		if st.Has(a11y.Vertical) {
			return vI4(2)
		}
	case uiaPropFrameworkID:
		return vStr("Tango")
	case uiaPropIsRequiredForForm:
		return vBool(st.Has(a11y.Required))
	case uiaPropIsDataValidForForm:
		return vBool(!st.Has(a11y.Invalid))
	case uiaPropItemStatus:
		if st.Has(a11y.Busy) {
			return vStr("busy")
		}
	case uiaPropLiveSetting:
		if n.Live != a11y.LiveOff {
			return vI4(a11ybridge.UIALiveSetting(n.Live))
		}
	case uiaPropPositionInSet:
		if n.PosInSet > 0 {
			return vI4(int32(n.PosInSet))
		}
	case uiaPropSizeOfSet:
		if n.SetSize > 0 {
			return vI4(int32(n.SetSize))
		}
	case uiaPropLevel:
		if n.Level > 0 {
			return vI4(int32(n.Level))
		}
	case uiaPropIsDialog:
		return vBool(n.Role == a11y.RoleDialog || n.Role == a11y.RoleAlertDialog)
	case uiaPropValueValue:
		return vStr(uiaValueText(n))
	case uiaPropRangeValueValue:
		if n.HasRange {
			return vR8(n.ValueNow)
		}
	case uiaPropToggleState:
		return vI4(a11ybridge.ToggleState(n))
	case uiaPropExpandCollapseState:
		return vI4(a11ybridge.ExpandCollapseState(n))
	}
	return uiaValue{}
}

// uiaValueText is IValueProvider.Value: never the content of a password
// field.
func uiaValueText(n *a11y.Node) string {
	if n.States.Has(a11y.Protected) {
		return ""
	}
	return n.Value
}

// uiaHasPattern reports whether n supports pattern.
func uiaHasPattern(n *a11y.Node, pattern int32) bool {
	for _, p := range a11ybridge.UIAPatterns(n) {
		if p == pattern {
			return true
		}
	}
	return false
}

// uiaNavigate follows dir from node id (0: the element is not in t).
func uiaNavigate(t *a11y.Tree, id a11y.NodeID, dir int32) a11y.NodeID {
	n := t.Node(id)
	if n == nil {
		return 0
	}
	switch dir {
	case uiaNavParent:
		if id == t.Root {
			return 0 // the host window is the parent
		}
		return n.Parent
	case uiaNavNextSibling:
		if id == t.Root {
			return 0
		}
		return a11ybridge.Sibling(t, id, 1)
	case uiaNavPreviousSibling:
		if id == t.Root {
			return 0
		}
		return a11ybridge.Sibling(t, id, -1)
	case uiaNavFirstChild:
		if len(n.Children) > 0 {
			return n.Children[0]
		}
	case uiaNavLastChild:
		if len(n.Children) > 0 {
			return n.Children[len(n.Children)-1]
		}
	}
	return 0
}

// uiaSelectionContainer returns the nearest ancestor of id that holds a
// selection (ISelectionItemProvider.SelectionContainer), or 0.
func uiaSelectionContainer(t *a11y.Tree, id a11y.NodeID) a11y.NodeID {
	n := t.Node(id)
	for n != nil && n.Parent != 0 {
		n = t.Node(n.Parent)
		if n == nil {
			return 0
		}
		switch n.Role {
		case a11y.RoleListbox, a11y.RoleTabList, a11y.RoleRadioGroup, a11y.RoleTree, a11y.RoleGrid,
			a11y.RoleTable, a11y.RoleMenu, a11y.RoleMenuBar:
			return n.ID
		}
	}
	return 0
}

// uiaPropertyEvents lists the property-changed events to raise for a node
// whose Change is c.
func uiaPropertyEvents(c a11ybridge.Change, n *a11y.Node) []int32 {
	var out []int32
	if c.Has(a11ybridge.ChangeName) {
		out = append(out, uiaPropName, uiaPropHelpText)
	}
	if c.Has(a11ybridge.ChangeValue) {
		if n.HasRange {
			out = append(out, uiaPropRangeValueValue)
		} else {
			out = append(out, uiaPropValueValue)
		}
	}
	if c.Has(a11ybridge.ChangeStates) {
		out = append(out, uiaPropIsEnabled, uiaPropToggleState, uiaPropExpandCollapseState, uiaPropIsOffscreen)
	}
	return out
}

// uiaProcessing is the NotificationProcessing for an announcement.
func uiaProcessing(l a11y.Live) int32 {
	if l == a11y.LiveAssertive {
		return uiaProcessingImportantMostRecent
	}
	return uiaProcessingMostRecent
}

func uitoa(v uint64) string {
	var b [20]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			return string(b[i:])
		}
	}
}

// ---- floating-point thunks ----
//
// syscall.NewCallback only takes integer-sized, non-floating arguments:
// Windows passes doubles in the first four positions in XMM registers
// (x64) or D registers (arm64), which the Go callback wrapper does not
// read. Two UIA methods take doubles by value:
//
//	IRawElementProviderFragmentRoot::ElementProviderFromPoint(this, double x, double y, out)
//	IRangeValueProvider::SetValue(this, double val)
//
// For them a few instructions in executable memory move the doubles'
// bits into the integer argument registers and jump to the Go callback,
// which reads them as uint64 (math.Float64frombits).

// thunkFromPoint returns the code for (this, double, double, ptr) →
// (this, bits, bits, ptr), ending in a jump to target.
func thunkFromPoint(arch string, target uint64) []byte {
	switch arch {
	case "amd64":
		// rcx=this, xmm1=x, xmm2=y, r9=out (positions 1-4).
		code := []byte{
			0x66, 0x48, 0x0F, 0x7E, 0xCA, // movq rdx, xmm1
			0x66, 0x49, 0x0F, 0x7E, 0xD0, // movq r8, xmm2
		}
		return appendJumpAMD64(code, target)
	case "arm64":
		// x0=this, d0=x, d1=y, x1=out (floats and integers count separately).
		return appendJumpARM64([]uint32{
			0xAA0103E3, // mov x3, x1
			0x9E660001, // fmov x1, d0
			0x9E660022, // fmov x2, d1
		}, target)
	}
	return nil
}

// thunkOneDouble returns the code for (this, double) → (this, bits).
func thunkOneDouble(arch string, target uint64) []byte {
	switch arch {
	case "amd64":
		return appendJumpAMD64([]byte{0x66, 0x48, 0x0F, 0x7E, 0xCA}, target) // movq rdx, xmm1
	case "arm64":
		return appendJumpARM64([]uint32{0x9E660001}, target) // fmov x1, d0
	}
	return nil
}

// appendJumpAMD64 appends jmp qword ptr [rip+0] followed by the address.
func appendJumpAMD64(code []byte, target uint64) []byte {
	code = append(code, 0xFF, 0x25, 0, 0, 0, 0)
	return binary.LittleEndian.AppendUint64(code, target)
}

// appendJumpARM64 appends ldr x16, <literal>; br x16; <literal: target>.
func appendJumpARM64(insns []uint32, target uint64) []byte {
	// The literal follows the br: 8 bytes after the ldr (imm19 = 2 words).
	insns = append(insns, 0x58000050, 0xD61F0200) // ldr x16, #8; br x16
	code := make([]byte, 0, 4*len(insns)+8)
	for _, in := range insns {
		code = binary.LittleEndian.AppendUint32(code, in)
	}
	return binary.LittleEndian.AppendUint64(code, target)
}

// f64 decodes a double passed through a thunk.
func f64(bits uintptr) float64 { return math.Float64frombits(uint64(bits)) }
