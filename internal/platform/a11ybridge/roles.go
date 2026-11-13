package a11ybridge

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
)

// ---------------------------------------------------------------------------
// macOS (NSAccessibility)

// MacRole returns the NSAccessibility role and subrole ("" when none) of
// n: a11y.Mappings plus the refinements of docs/accessibility.md section 3
// (toggle buttons, multi-line and password text fields).
func MacRole(n *a11y.Node) (role, subrole string) {
	p, ok := a11y.Mappings[n.Role]
	if !ok {
		return "AXGroup", ""
	}
	role, subrole, _ = strings.Cut(p.Mac, "/")
	switch n.Role {
	case a11y.RoleButton:
		if n.States.Has(a11y.Toggleable) {
			return "AXCheckBox", "AXToggle"
		}
	case a11y.RoleTextbox:
		if n.States.Has(a11y.Protected) {
			return "AXTextField", "AXSecureTextField"
		}
		if n.States.Has(a11y.Multiline) {
			return "AXTextArea", ""
		}
	case a11y.RoleSwitch:
		// AXSwitch is a subrole of AXCheckBox since macOS 14; older
		// releases read it as a checkbox.
	}
	return role, subrole
}

// MacToggle reports whether n's AXValue is its checked state (0 off, 1 on,
// 2 mixed) rather than text or a number.
func MacToggle(n *a11y.Node) bool {
	switch n.Role {
	case a11y.RoleCheckbox, a11y.RoleRadio, a11y.RoleSwitch:
		return true
	case a11y.RoleButton:
		return n.States.Has(a11y.Toggleable)
	}
	return n.States.Has(a11y.Checkable) && n.Role == a11y.RoleMenuItem
}

// CheckValue is the tri-state value of a checkable node: 0 off, 1 on, 2
// mixed.
func CheckValue(n *a11y.Node) int {
	switch {
	case n.States.Has(a11y.Mixed):
		return 2
	case n.States.Has(a11y.Checked), n.States.Has(a11y.Pressed):
		return 1
	}
	return 0
}

// MacAction pairs an NSAccessibility action with the a11y.Action it
// performs.
type MacAction struct {
	Name string // AXPress, AXIncrement, ...
	Act  a11y.Action
}

// MacActions lists the AX actions of n, AXPress first.
func MacActions(n *a11y.Node) []MacAction {
	var out []MacAction
	add := func(name string, a a11y.Action) {
		if n.Actions.Has(a) {
			out = append(out, MacAction{name, a})
		}
	}
	add("AXPress", a11y.Press)
	if !n.Actions.Has(a11y.Press) {
		// Expandable triggers without a press action open with AXPress.
		if n.States.Has(a11y.Expanded) {
			add("AXPress", a11y.Collapse)
		} else {
			add("AXPress", a11y.Expand)
		}
	}
	add("AXIncrement", a11y.Increment)
	add("AXDecrement", a11y.Decrement)
	if n.States.Has(a11y.HasPopup) && !n.States.Has(a11y.Expanded) {
		add("AXShowMenu", a11y.Expand)
	}
	add("AXCancel", a11y.Dismiss)
	add("AXScrollToVisible", a11y.ScrollIntoView)
	return out
}

// ---------------------------------------------------------------------------
// Windows (UI Automation)

// UIA control type IDs (UIAutomationClient.h, UIA_<X>ControlTypeId).
var uiaControlTypes = map[string]int32{
	"Button": 50000, "Calendar": 50001, "CheckBox": 50002, "ComboBox": 50003, "Edit": 50004,
	"Hyperlink": 50005, "Image": 50006, "ListItem": 50007, "List": 50008, "Menu": 50009,
	"MenuBar": 50010, "MenuItem": 50011, "ProgressBar": 50012, "RadioButton": 50013,
	"ScrollBar": 50014, "Slider": 50015, "Spinner": 50016, "StatusBar": 50017, "Tab": 50018,
	"TabItem": 50019, "Text": 50020, "ToolBar": 50021, "ToolTip": 50022, "Tree": 50023,
	"TreeItem": 50024, "Custom": 50025, "Group": 50026, "Thumb": 50027, "DataGrid": 50028,
	"DataItem": 50029, "Document": 50030, "SplitButton": 50031, "Window": 50032, "Pane": 50033,
	"Header": 50034, "HeaderItem": 50035, "Table": 50036, "TitleBar": 50037, "Separator": 50038,
	"SemanticZoom": 50039, "AppBar": 50040,
}

// UIAControlTypeID returns the UIA_<name>ControlTypeId for a control type
// name of a11y.Mappings (0 when unknown).
func UIAControlTypeID(name string) int32 { return uiaControlTypes[name] }

// UIAControlType returns the control type ID of n.
func UIAControlType(n *a11y.Node) int32 {
	p, ok := a11y.Mappings[n.Role]
	if !ok {
		return uiaControlTypes["Custom"]
	}
	return uiaControlTypes[p.UIA]
}

// UIA control pattern IDs (UIA_<X>PatternId).
const (
	UIAInvokePattern         = 10000
	UIASelectionPattern      = 10001
	UIAValuePattern          = 10002
	UIARangeValuePattern     = 10003
	UIAScrollPattern         = 10004
	UIAExpandCollapsePattern = 10005
	UIAGridPattern           = 10006
	UIAGridItemPattern       = 10007
	UIASelectionItemPattern  = 10010
	UIATogglePattern         = 10015
	UIAScrollItemPattern     = 10017
)

// UIAPatterns returns the control patterns n supports.
func UIAPatterns(n *a11y.Node) []int32 {
	var out []int32
	toggle := MacToggle(n) && n.Role != a11y.RoleRadio
	if toggle {
		out = append(out, UIATogglePattern)
	} else if n.Actions.Has(a11y.Press) && !n.States.Has(a11y.Selectable) {
		out = append(out, UIAInvokePattern)
	}
	if n.States.Has(a11y.Selectable) || n.Role == a11y.RoleRadio {
		out = append(out, UIASelectionItemPattern)
	}
	if n.HasRange {
		out = append(out, UIARangeValuePattern)
	} else if n.Actions.Has(a11y.SetValue) || isTextInput(n) {
		out = append(out, UIAValuePattern)
	}
	if n.States.Has(a11y.Expandable) || n.Actions.Has(a11y.Expand) || n.Actions.Has(a11y.Collapse) {
		out = append(out, UIAExpandCollapsePattern)
	}
	if n.Actions.Has(a11y.ScrollIntoView) {
		out = append(out, UIAScrollItemPattern)
	}
	return out
}

func isTextInput(n *a11y.Node) bool {
	switch n.Role {
	case a11y.RoleTextbox, a11y.RoleSearchbox, a11y.RoleSpinButton:
		return true
	case a11y.RoleCombobox:
		return n.States.Has(a11y.Editable)
	}
	return false
}

// ToggleState is UIA's ToggleState: 0 off, 1 on, 2 indeterminate.
func ToggleState(n *a11y.Node) int32 { return int32(CheckValue(n)) }

// ExpandCollapseState is UIA's ExpandCollapseState: 0 collapsed, 1
// expanded, 3 leaf.
func ExpandCollapseState(n *a11y.Node) int32 {
	switch {
	case n.States.Has(a11y.Expanded):
		return 1
	case n.States.Has(a11y.Expandable), n.Actions.Has(a11y.Expand):
		return 0
	}
	return 3
}

// UIALiveSetting is UIA's LiveSetting: 0 off, 1 polite, 2 assertive.
func UIALiveSetting(l a11y.Live) int32 {
	switch l {
	case a11y.LivePolite:
		return 1
	case a11y.LiveAssertive:
		return 2
	}
	return 0
}

// ---------------------------------------------------------------------------
// Linux (AT-SPI2)

// AT-SPI roles (atspi-constants.h, AtspiRole) used by a11y.Mappings and
// the bridge.
var atspiRoles = map[string]uint32{
	"INVALID": 0, "ALERT": 2, "CHECK_BOX": 7, "COLUMN_HEADER": 10, "COMBO_BOX": 11,
	"DIALOG": 16, "DRAWING_AREA": 18, "FILLER": 20, "FRAME": 23, "IMAGE": 27, "LABEL": 29,
	"LIST": 31, "LIST_ITEM": 32, "MENU": 33, "MENU_BAR": 34, "MENU_ITEM": 35, "PAGE_TAB": 37,
	"PAGE_TAB_LIST": 38, "PANEL": 39, "PASSWORD_TEXT": 40, "PROGRESS_BAR": 42,
	"PUSH_BUTTON": 43, "RADIO_BUTTON": 44, "ROW_HEADER": 47, "SCROLL_PANE": 49,
	"SEPARATOR": 50, "SLIDER": 51, "SPIN_BUTTON": 52, "STATUS_BAR": 54, "TABLE": 55,
	"TABLE_CELL": 56, "TEXT": 61, "TOGGLE_BUTTON": 62, "TOOL_BAR": 63, "TOOL_TIP": 64,
	"TREE": 65, "UNKNOWN": 67, "WINDOW": 69, "PARAGRAPH": 73, "APPLICATION": 75, "ENTRY": 79,
	"CHART": 80, "HEADING": 83, "FORM": 87, "LINK": 88, "TABLE_ROW": 90, "TREE_ITEM": 91,
	"LIST_BOX": 98, "GROUPING": 99, "NOTIFICATION": 101, "LEVEL_BAR": 103, "LANDMARK": 110,
	"TIMER": 115, "STATIC": 116,
}

// ATSPIRoleID returns the AtspiRole value of a role name ("PUSH_BUTTON"),
// and false when the name is unknown.
func ATSPIRoleID(name string) (uint32, bool) {
	v, ok := atspiRoles[name]
	return v, ok
}

// ATSPIRole returns the AtspiRole of n, with the refinements of
// docs/accessibility.md (toggle buttons, multi-line and password text).
func ATSPIRole(n *a11y.Node) uint32 {
	switch n.Role {
	case a11y.RoleButton:
		if n.States.Has(a11y.Toggleable) {
			return atspiRoles["TOGGLE_BUTTON"]
		}
	case a11y.RoleTextbox:
		if n.States.Has(a11y.Protected) {
			return atspiRoles["PASSWORD_TEXT"]
		}
		if n.States.Has(a11y.Multiline) {
			return atspiRoles["TEXT"]
		}
	}
	p, ok := a11y.Mappings[n.Role]
	if !ok {
		return atspiRoles["UNKNOWN"]
	}
	return atspiRoles[p.ATSPI]
}

// AT-SPI states (atspi-constants.h, AtspiStateType).
const (
	StateActive          = 1
	StateBusy            = 3
	StateChecked         = 4
	StateCollapsed       = 5
	StateDefunct         = 6
	StateEditable        = 7
	StateEnabled         = 8
	StateExpandable      = 9
	StateExpanded        = 10
	StateFocusable       = 11
	StateFocused         = 12
	StateHorizontal      = 14
	StateModal           = 16
	StateMultiLine       = 17
	StateMultiselectable = 18
	StatePressed         = 20
	StateSelectable      = 22
	StateSelected        = 23
	StateSensitive       = 24
	StateShowing         = 25
	StateSingleLine      = 26
	StateVertical        = 29
	StateVisible         = 30
	StateIndeterminate   = 32
	StateRequired        = 33
	StateInvalidEntry    = 36
	StateVisited         = 40
	StateCheckable       = 41
	StateHasPopup        = 42
	StateReadOnly        = 43
)

// ATSPIStates returns n's AT-SPI state set as the two 32-bit words of
// Accessible.GetState. focused says whether the canvas itself has
// keyboard focus (FOCUSED is only set then).
func ATSPIStates(n *a11y.Node, focused bool) [2]uint32 {
	var set [2]uint32
	add := func(s int) { set[s/32] |= 1 << (s % 32) }
	st := n.States
	if !st.Has(a11y.Disabled) {
		add(StateEnabled)
		add(StateSensitive)
	}
	add(StateVisible)
	if !st.Has(a11y.Offscreen) {
		add(StateShowing)
	}
	m := map[a11y.State]int{
		a11y.Focusable: StateFocusable, a11y.Checkable: StateCheckable, a11y.Checked: StateChecked,
		a11y.Mixed: StateIndeterminate, a11y.Selectable: StateSelectable, a11y.Selected: StateSelected,
		a11y.Expandable: StateExpandable, a11y.Expanded: StateExpanded, a11y.Pressed: StatePressed,
		a11y.Required: StateRequired, a11y.Invalid: StateInvalidEntry, a11y.ReadOnly: StateReadOnly,
		a11y.Modal: StateModal, a11y.Multiselectable: StateMultiselectable, a11y.Busy: StateBusy,
		a11y.HasPopup: StateHasPopup, a11y.Visited: StateVisited, a11y.Vertical: StateVertical,
		a11y.Editable: StateEditable,
	}
	for s, v := range m {
		if st.Has(s) {
			add(v)
		}
	}
	if st.Has(a11y.Focused) && focused {
		add(StateFocused)
	}
	if st.Has(a11y.Expandable) && !st.Has(a11y.Expanded) {
		add(StateCollapsed)
	}
	if isTextInput(n) {
		if !st.Has(a11y.ReadOnly) && !st.Has(a11y.Disabled) {
			add(StateEditable)
		}
		if st.Has(a11y.Multiline) {
			add(StateMultiLine)
		} else {
			add(StateSingleLine)
		}
	}
	if n.Role == a11y.RoleSlider || n.Role == a11y.RoleSeparator || n.Role == a11y.RoleTabList ||
		n.Role == a11y.RoleToolbar || n.Role == a11y.RoleMenu || n.Role == a11y.RoleMenuBar {
		if !st.Has(a11y.Vertical) {
			add(StateHorizontal)
		}
	}
	return set
}

// ATSPIAction pairs an AT-SPI action name with its a11y.Action.
type ATSPIAction struct {
	Name string
	Act  a11y.Action
}

// ATSPIActions lists the AT-SPI Action interface entries of n.
func ATSPIActions(n *a11y.Node) []ATSPIAction {
	var out []ATSPIAction
	for _, a := range []ATSPIAction{
		{"click", a11y.Press}, {"expand", a11y.Expand}, {"collapse", a11y.Collapse},
		{"increment", a11y.Increment}, {"decrement", a11y.Decrement}, {"dismiss", a11y.Dismiss},
		{"scroll-into-view", a11y.ScrollIntoView},
	} {
		if n.Actions.Has(a.Act) {
			out = append(out, a)
		}
	}
	return out
}

// RangeStep returns the step of a range node (1 when unset, for the
// increment fallback of Value.SetCurrentValue).
func RangeStep(n *a11y.Node) float64 {
	if n.Step > 0 {
		return n.Step
	}
	return 1
}

// RangeAction turns a requested new value of a range node into the
// action to perform: SetValue when the node supports it, else one
// Increment or Decrement toward v (0 when v equals the current value).
func RangeAction(n *a11y.Node, v float64) a11y.Action {
	switch {
	case n.Actions.Has(a11y.SetValue):
		return a11y.SetValue
	case v > n.ValueNow && n.Actions.Has(a11y.Increment):
		return a11y.Increment
	case v < n.ValueNow && n.Actions.Has(a11y.Decrement):
		return a11y.Decrement
	}
	return 0
}
