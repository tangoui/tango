package a11y

// Platform names how a role is exposed by each native accessibility API.
// Bridges look roles up in Mappings; docs/accessibility.md prints the same
// table (TestMappingsDocumented keeps them in sync).
type Platform struct {
	Mac     string // NSAccessibility role, "/Subrole" when one is needed
	UIA     string // UI Automation ControlType (UIA_<X>ControlTypeId)
	ATSPI   string // AT-SPI role (ATSPI_ROLE_<X>)
	IOS     string // UIAccessibilityTraits (or a note for containers)
	Android string // AccessibilityNodeInfo.className (TalkBack derives the spoken role from it)
}

// Mappings maps every exposed role to its native counterparts.
var Mappings = map[Role]Platform{
	RoleWindow:       {"AXGroup", "Pane", "PANEL", "container", "android.view.View"},
	RoleGroup:        {"AXGroup", "Group", "PANEL", "container", "android.view.ViewGroup"},
	RoleText:         {"AXStaticText", "Text", "STATIC", "staticText", "android.widget.TextView"},
	RoleHeading:      {"AXHeading", "Text", "HEADING", "header", "android.widget.TextView"},
	RoleParagraph:    {"AXStaticText", "Text", "PARAGRAPH", "staticText", "android.widget.TextView"},
	RoleLink:         {"AXLink", "Hyperlink", "LINK", "link", "android.widget.TextView"},
	RoleButton:       {"AXButton", "Button", "PUSH_BUTTON", "button", "android.widget.Button"},
	RoleCheckbox:     {"AXCheckBox", "CheckBox", "CHECK_BOX", "button+toggleButton", "android.widget.CheckBox"},
	RoleRadio:        {"AXRadioButton", "RadioButton", "RADIO_BUTTON", "button", "android.widget.RadioButton"},
	RoleRadioGroup:   {"AXRadioGroup", "Group", "GROUPING", "container", "android.widget.RadioGroup"},
	RoleSwitch:       {"AXCheckBox/AXSwitch", "Button", "TOGGLE_BUTTON", "button+toggleButton", "android.widget.Switch"},
	RoleTextbox:      {"AXTextField", "Edit", "ENTRY", "none (UITextInput)", "android.widget.EditText"},
	RoleSearchbox:    {"AXTextField/AXSearchField", "Edit", "ENTRY", "searchField", "android.widget.EditText"},
	RoleSpinButton:   {"AXIncrementor", "Spinner", "SPIN_BUTTON", "adjustable", "android.widget.EditText"},
	RoleCombobox:     {"AXComboBox", "ComboBox", "COMBO_BOX", "button", "android.widget.Spinner"},
	RoleListbox:      {"AXList", "List", "LIST_BOX", "container", "android.widget.ListView"},
	RoleOption:       {"AXStaticText", "ListItem", "LIST_ITEM", "button", "android.widget.CheckedTextView"},
	RoleMenu:         {"AXMenu", "Menu", "MENU", "container", "android.widget.ListView"},
	RoleMenuBar:      {"AXMenuBar", "MenuBar", "MENU_BAR", "container", "android.view.ViewGroup"},
	RoleMenuItem:     {"AXMenuItem", "MenuItem", "MENU_ITEM", "button", "android.view.View"},
	RoleTab:          {"AXRadioButton/AXTabButton", "TabItem", "PAGE_TAB", "button", "android.view.View"},
	RoleTabList:      {"AXTabGroup", "Tab", "PAGE_TAB_LIST", "tabBar", "android.widget.TabWidget"},
	RoleTabPanel:     {"AXGroup", "Pane", "SCROLL_PANE", "container", "android.view.ViewGroup"},
	RoleDialog:       {"AXGroup/AXApplicationDialog", "Pane", "DIALOG", "container (accessibilityViewIsModal)", "android.view.ViewGroup"},
	RoleAlertDialog:  {"AXGroup/AXApplicationAlertDialog", "Pane", "ALERT", "container (accessibilityViewIsModal)", "android.view.ViewGroup"},
	RoleAlert:        {"AXGroup/AXApplicationAlert", "Group", "NOTIFICATION", "staticText", "android.view.View"},
	RoleStatus:       {"AXGroup/AXApplicationStatus", "StatusBar", "STATUS_BAR", "staticText", "android.view.View"},
	RoleTooltip:      {"AXGroup/AXUserInterfaceTooltip", "ToolTip", "TOOL_TIP", "staticText", "android.widget.TextView"},
	RoleSlider:       {"AXSlider", "Slider", "SLIDER", "adjustable", "android.widget.SeekBar"},
	RoleProgressBar:  {"AXProgressIndicator", "ProgressBar", "PROGRESS_BAR", "updatesFrequently", "android.widget.ProgressBar"},
	RoleTable:        {"AXTable", "Table", "TABLE", "container (dataTable)", "android.widget.GridView"},
	RoleGrid:         {"AXTable", "DataGrid", "TABLE", "container (dataTable)", "android.widget.GridView"},
	RoleRow:          {"AXRow", "DataItem", "TABLE_ROW", "container", "android.view.View"},
	RoleCell:         {"AXCell", "DataItem", "TABLE_CELL", "staticText", "android.view.View"},
	RoleColumnHeader: {"AXCell", "HeaderItem", "COLUMN_HEADER", "header", "android.view.View"},
	RoleRowHeader:    {"AXCell", "HeaderItem", "ROW_HEADER", "header", "android.view.View"},
	RoleTree:         {"AXOutline", "Tree", "TREE", "container", "android.widget.ExpandableListView"},
	RoleTreeItem:     {"AXRow/AXOutlineRow", "TreeItem", "TREE_ITEM", "button", "android.view.View"},
	RoleList:         {"AXList", "List", "LIST", "container", "android.widget.ListView"},
	RoleListItem:     {"AXGroup", "ListItem", "LIST_ITEM", "staticText", "android.view.View"},
	RoleImage:        {"AXImage", "Image", "IMAGE", "image", "android.widget.ImageView"},
	RoleNavigation:   {"AXGroup/AXLandmarkNavigation", "Group", "LANDMARK", "container", "android.view.ViewGroup"},
	RoleToolbar:      {"AXToolbar", "ToolBar", "TOOL_BAR", "container", "android.widget.Toolbar"},
	RoleSeparator:    {"AXSplitter", "Separator", "SEPARATOR", "adjustable (splitter only)", "android.view.View"},
	RoleScrollArea:   {"AXScrollArea", "Pane", "SCROLL_PANE", "container", "android.widget.ScrollView"},
	RoleForm:         {"AXGroup/AXLandmarkForm", "Group", "FORM", "container", "android.view.ViewGroup"},
	RoleFigure:       {"AXGroup", "Group", "CHART", "image (AXChart descriptor)", "android.view.View"},
	RoleTimer:        {"AXGroup/AXApplicationTimer", "Text", "TIMER", "staticText+updatesFrequently", "android.widget.TextView"},
	RoleMeter:        {"AXLevelIndicator", "ProgressBar", "LEVEL_BAR", "staticText", "android.widget.ProgressBar"},
}
