package linux

// Pure logic of the AT-SPI2 bridge (canvas_a11y_linux.go), free of build
// tags so it is unit tested on every OS: object paths, GVariant text for
// replies and events, attributes, relations and text slicing. Interface
// definitions follow the at-spi2-core D-Bus XML (org.a11y.atspi.*).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

const (
	atspiBase     = "/org/a11y/atspi/accessible" // our subtree (flat: one segment per object)
	atspiRootNode = "root"                       // the application object libatspi looks up
	atspiRootPath = atspiBase + "/" + atspiRootNode
	atspiNullPath = "/org/a11y/atspi/null"

	atspiRegistryName = "org.a11y.atspi.Registry"

	atspiIfaceAccessible   = "org.a11y.atspi.Accessible"
	atspiIfaceApplication  = "org.a11y.atspi.Application"
	atspiIfaceComponent    = "org.a11y.atspi.Component"
	atspiIfaceAction       = "org.a11y.atspi.Action"
	atspiIfaceValue        = "org.a11y.atspi.Value"
	atspiIfaceText         = "org.a11y.atspi.Text"
	atspiIfaceEditableText = "org.a11y.atspi.EditableText"
	atspiIfaceSocket       = "org.a11y.atspi.Socket"
	atspiEventObject       = "org.a11y.atspi.Event.Object"

	// AtspiCoordType
	atspiCoordScreen = 0
	atspiCoordWindow = 1
	atspiCoordParent = 2

	// AtspiRelationType
	atspiRelationLabelledBy    = 2
	atspiRelationControllerFor = 3
	atspiRelationDescribedBy   = 18

	// AtspiLive (politeness of an Announcement)
	atspiLivePolite    = 1
	atspiLiveAssertive = 2

	// AtspiTextGranularity
	atspiGranularityChar = 0
	atspiGranularityWord = 1
)

// atspiObj names one exported object: the application root, the alias of
// a canvas's root (stable before any tree exists, the plug id of its
// AtkSocket) or a node of a canvas.
type atspiObj struct {
	app   bool
	key   uint64 // canvas
	alias bool   // the canvas root, whatever tree.Root is
	id    a11y.NodeID
}

// atspiNodeName is the subtree node (one path segment) of an object.
func atspiNodeName(o atspiObj) string {
	switch {
	case o.app:
		return atspiRootNode
	case o.alias:
		return "c" + strconv.FormatUint(o.key, 16)
	}
	return "c" + strconv.FormatUint(o.key, 16) + "_" + strconv.FormatUint(uint64(o.id), 16)
}

func atspiPath(o atspiObj) string { return atspiBase + "/" + atspiNodeName(o) }

// atspiParse resolves an object path (or a bare node name) of our subtree.
func atspiParse(path string) (atspiObj, bool) {
	node := strings.TrimPrefix(path, atspiBase+"/")
	if node == atspiRootNode {
		return atspiObj{app: true}, true
	}
	if !strings.HasPrefix(node, "c") || strings.Contains(node, "/") {
		return atspiObj{}, false
	}
	k, id, hasID := strings.Cut(node[1:], "_")
	key, err := strconv.ParseUint(k, 16, 64)
	if err != nil {
		return atspiObj{}, false
	}
	if !hasID {
		return atspiObj{key: key, alias: true}, true
	}
	n, err := strconv.ParseUint(id, 16, 64)
	if err != nil {
		return atspiObj{}, false
	}
	return atspiObj{key: key, id: a11y.NodeID(n)}, true
}

// gvRef is an AT-SPI object reference (so) in GVariant text.
func gvRef(bus, path string) string {
	if bus == "" || path == "" {
		bus, path = "", atspiNullPath
	}
	return "(" + gvString(bus) + ", objectpath " + gvString(path) + ")"
}

// gvRefs is an a(so).
func gvRefs(refs []string) string {
	if len(refs) == 0 {
		return "@a(so) []"
	}
	return "[" + strings.Join(refs, ", ") + "]"
}

// gvStrings is an as.
func gvStrings(ss []string) string {
	if len(ss) == 0 {
		return "@as []"
	}
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = gvString(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// gvDict is an a{ss} (keys sorted, so replies are deterministic).
func gvDict(m map[string]string) string {
	if len(m) == 0 {
		return "@a{ss} {}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = gvString(k) + ": " + gvString(m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// gvStates is the au of Accessible.GetState.
func gvStates(set [2]uint32) string {
	return fmt.Sprintf("[uint32 %d, %d]", set[0], set[1])
}

// gvDouble formats a double that g_variant_parse reads as "d".
func gvDouble(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnN") { // "3" would parse as int32
		s += ".0"
	}
	return "double " + s
}

// atspiEventArgs is the (siiva{sv}) body of an org.a11y.atspi.Event.*
// signal; anyData is a GVariant text value, boxed here.
func atspiEventArgs(detail string, d1, d2 int32, anyData string) string {
	if anyData == "" {
		anyData = "0"
	}
	return fmt.Sprintf("(%s, %d, %d, <%s>, @a{sv} {})", gvString(detail), d1, d2, anyData)
}

// atspiText is the text the Text interface exposes for n: the value of
// text inputs (nothing for passwords), the name otherwise.
func atspiText(n *a11y.Node) string {
	switch n.Role {
	case a11y.RoleTextbox, a11y.RoleSearchbox, a11y.RoleSpinButton, a11y.RoleCombobox:
		if n.States.Has(a11y.Protected) {
			return ""
		}
		return n.Value
	}
	return n.Name
}

// atspiHasText reports whether n implements Text: inputs and text roles.
func atspiHasText(n *a11y.Node) bool {
	switch n.Role {
	case a11y.RoleTextbox, a11y.RoleSearchbox, a11y.RoleSpinButton, a11y.RoleText,
		a11y.RoleHeading, a11y.RoleParagraph, a11y.RoleLink:
		return true
	case a11y.RoleCombobox:
		return n.States.Has(a11y.Editable)
	}
	return false
}

// atspiEditable reports whether n implements EditableText.
func atspiEditable(n *a11y.Node) bool {
	return n.Actions.Has(a11y.SetValue) && !n.HasRange && !n.States.Has(a11y.ReadOnly)
}

// atspiInterfaces lists the interfaces of n (GetInterfaces and the
// subtree's introspection).
func atspiInterfaces(n *a11y.Node) []string {
	out := []string{atspiIfaceAccessible, atspiIfaceComponent}
	if len(a11ybridge.ATSPIActions(n)) > 0 {
		out = append(out, atspiIfaceAction)
	}
	if n.HasRange {
		out = append(out, atspiIfaceValue)
	}
	if atspiHasText(n) {
		out = append(out, atspiIfaceText)
	}
	if atspiEditable(n) {
		out = append(out, atspiIfaceEditableText)
	}
	return out
}

// runeSlice returns s[start:end] counted in characters; end < 0 means the
// end of s. Out-of-range offsets are clamped.
func runeSlice(s string, start, end int) string {
	n := utf8.RuneCountInString(s)
	if end < 0 || end > n {
		end = n
	}
	start = max(0, min(start, end))
	r := []rune(s)
	return string(r[start:end])
}

// runeWord returns the word around offset (spaces separate words) and its
// bounds in characters.
func runeWord(s string, offset int) (string, int, int) {
	r := []rune(s)
	if offset < 0 || offset >= len(r) {
		return "", 0, 0
	}
	start, end := offset, offset
	for start > 0 && r[start-1] != ' ' {
		start--
	}
	for end < len(r) && r[end] != ' ' {
		end++
	}
	return string(r[start:end]), start, end
}

// atspiStringAt answers Text.GetStringAtOffset / GetTextAtOffset: a
// character, a word, or the whole text for the coarser granularities
// (the canvas does not expose line layout).
func atspiStringAt(s string, offset int, gran uint32) (string, int, int) {
	n := utf8.RuneCountInString(s)
	switch gran {
	case atspiGranularityChar:
		if offset < 0 || offset >= n {
			return "", offset, offset
		}
		return runeSlice(s, offset, offset+1), offset, offset + 1
	case atspiGranularityWord:
		return runeWord(s, offset)
	}
	return s, 0, n
}

// atspiAttributes is the a{ss} of Accessible.GetAttributes. liveRoot is
// the live region containing n (nil if none).
func atspiAttributes(n *a11y.Node, liveRoot *a11y.Node) map[string]string {
	m := map[string]string{"toolkit": "tango"}
	set := func(k string, v int) {
		if v > 0 {
			m[k] = strconv.Itoa(v)
		}
	}
	set("level", n.Level)
	set("posinset", n.PosInSet)
	set("setsize", n.SetSize)
	set("rowindex", n.RowIndex)
	set("colindex", n.ColIndex)
	set("rowcount", n.RowCount)
	set("colcount", n.ColCount)
	if n.Placeholder != "" {
		m["placeholder-text"] = n.Placeholder
	}
	if n.Shortcut != "" {
		m["keyshortcuts"] = n.Shortcut
	}
	if n.RoleDescription != "" {
		m["roledescription"] = n.RoleDescription
	}
	if n.Sort != a11y.SortNone {
		m["sort"] = n.Sort.String()
	}
	if n.States.Has(a11y.Current) {
		m["current"] = "true"
	}
	if n.Live != a11y.LiveOff {
		m["live"] = n.Live.String()
	}
	if liveRoot != nil {
		m["container-live"] = liveRoot.Live.String()
		m["container-atomic"] = "true" // the region is read as a whole (tree.Text)
	}
	return m
}

// atspiTextAttributes is the text attribute set of a node with the Text
// interface (GetAttributes, GetAttributeRun, GetDefaultAttributes). The
// canvas exposes no per-character styling, so the whole text is one run
// whose only attribute is its direction ("direction": "ltr" / "rtl", the
// values of ATK_TEXT_ATTR_DIRECTION), taken from the node's scope.
func atspiTextAttributes(n *a11y.Node) map[string]string {
	dir := "ltr"
	if n.RTL {
		dir = "rtl"
	}
	return map[string]string{"direction": dir}
}

// atspiRelation is one entry of GetRelationSet: a relation type and its
// target node.
type atspiRelation struct {
	Type   uint32
	Target a11y.NodeID
}

// atspiRelations lists n's relations whose target is in t.
func atspiRelations(t *a11y.Tree, n *a11y.Node) []atspiRelation {
	var out []atspiRelation
	add := func(typ uint32, id a11y.NodeID) {
		if id != 0 && t.Node(id) != nil {
			out = append(out, atspiRelation{typ, id})
		}
	}
	add(atspiRelationLabelledBy, n.LabelledBy)
	add(atspiRelationDescribedBy, n.DescribedBy)
	add(atspiRelationControllerFor, n.Controls)
	return out
}

// atspiPoliteness maps a11y.Live to AtspiLive.
func atspiPoliteness(l a11y.Live) int32 {
	if l == a11y.LiveAssertive {
		return atspiLiveAssertive
	}
	return atspiLivePolite
}

// atspiPlugID is the plug id atk_socket_embed takes: "<bus name>:<path>".
func atspiPlugID(bus, path string) string { return bus + ":" + path }

// atspiIntrospection describes the interfaces of our objects (a subset
// of the at-spi2-core XML: the members the bridge implements).
const atspiIntrospection = `<node>
<interface name="org.a11y.atspi.Accessible">
  <property name="Name" type="s" access="read"/>
  <property name="Description" type="s" access="read"/>
  <property name="Parent" type="(so)" access="read"/>
  <property name="ChildCount" type="i" access="read"/>
  <property name="Locale" type="s" access="read"/>
  <property name="AccessibleId" type="s" access="read"/>
  <property name="HelpText" type="s" access="read"/>
  <method name="GetChildAtIndex"><arg direction="in" name="index" type="i"/><arg direction="out" type="(so)"/></method>
  <method name="GetChildren"><arg direction="out" type="a(so)"/></method>
  <method name="GetIndexInParent"><arg direction="out" type="i"/></method>
  <method name="GetRelationSet"><arg direction="out" type="a(ua(so))"/></method>
  <method name="GetRole"><arg direction="out" type="u"/></method>
  <method name="GetRoleName"><arg direction="out" type="s"/></method>
  <method name="GetLocalizedRoleName"><arg direction="out" type="s"/></method>
  <method name="GetState"><arg direction="out" type="au"/></method>
  <method name="GetAttributes"><arg direction="out" type="a{ss}"/></method>
  <method name="GetApplication"><arg direction="out" type="(so)"/></method>
  <method name="GetInterfaces"><arg direction="out" type="as"/></method>
</interface>
<interface name="org.a11y.atspi.Application">
  <property name="ToolkitName" type="s" access="read"/>
  <property name="Version" type="s" access="read"/>
  <property name="AtspiVersion" type="s" access="read"/>
  <property name="Id" type="i" access="readwrite"/>
  <method name="GetLocale"><arg direction="in" name="lctype" type="u"/><arg direction="out" type="s"/></method>
</interface>
<interface name="org.a11y.atspi.Component">
  <method name="Contains"><arg direction="in" name="x" type="i"/><arg direction="in" name="y" type="i"/><arg direction="in" name="coord_type" type="u"/><arg direction="out" type="b"/></method>
  <method name="GetAccessibleAtPoint"><arg direction="in" name="x" type="i"/><arg direction="in" name="y" type="i"/><arg direction="in" name="coord_type" type="u"/><arg direction="out" type="(so)"/></method>
  <method name="GetExtents"><arg direction="in" name="coord_type" type="u"/><arg direction="out" type="(iiii)"/></method>
  <method name="GetPosition"><arg direction="in" name="coord_type" type="u"/><arg direction="out" name="x" type="i"/><arg direction="out" name="y" type="i"/></method>
  <method name="GetSize"><arg direction="out" name="width" type="i"/><arg direction="out" name="height" type="i"/></method>
  <method name="GetLayer"><arg direction="out" type="u"/></method>
  <method name="GetMDIZOrder"><arg direction="out" type="n"/></method>
  <method name="GrabFocus"><arg direction="out" type="b"/></method>
  <method name="GetAlpha"><arg direction="out" type="d"/></method>
  <method name="ScrollTo"><arg direction="in" name="type" type="u"/><arg direction="out" type="b"/></method>
  <method name="ScrollToPoint"><arg direction="in" name="type" type="u"/><arg direction="in" name="x" type="i"/><arg direction="in" name="y" type="i"/><arg direction="out" type="b"/></method>
</interface>
<interface name="org.a11y.atspi.Action">
  <property name="NActions" type="i" access="read"/>
  <method name="GetDescription"><arg direction="in" name="index" type="i"/><arg direction="out" type="s"/></method>
  <method name="GetName"><arg direction="in" name="index" type="i"/><arg direction="out" type="s"/></method>
  <method name="GetLocalizedName"><arg direction="in" name="index" type="i"/><arg direction="out" type="s"/></method>
  <method name="GetKeyBinding"><arg direction="in" name="index" type="i"/><arg direction="out" type="s"/></method>
  <method name="GetActions"><arg direction="out" type="a(sss)"/></method>
  <method name="DoAction"><arg direction="in" name="index" type="i"/><arg direction="out" type="b"/></method>
</interface>
<interface name="org.a11y.atspi.Value">
  <property name="MinimumValue" type="d" access="read"/>
  <property name="MaximumValue" type="d" access="read"/>
  <property name="MinimumIncrement" type="d" access="read"/>
  <property name="CurrentValue" type="d" access="readwrite"/>
  <property name="Text" type="s" access="read"/>
</interface>
<interface name="org.a11y.atspi.Text">
  <property name="CharacterCount" type="i" access="read"/>
  <property name="CaretOffset" type="i" access="read"/>
  <method name="GetText"><arg direction="in" name="startOffset" type="i"/><arg direction="in" name="endOffset" type="i"/><arg direction="out" type="s"/></method>
  <method name="GetCharacterAtOffset"><arg direction="in" name="offset" type="i"/><arg direction="out" type="i"/></method>
  <method name="GetStringAtOffset"><arg direction="in" name="offset" type="i"/><arg direction="in" name="granularity" type="u"/><arg direction="out" type="s"/><arg direction="out" name="startOffset" type="i"/><arg direction="out" name="endOffset" type="i"/></method>
  <method name="GetTextAtOffset"><arg direction="in" name="offset" type="i"/><arg direction="in" name="type" type="u"/><arg direction="out" type="s"/><arg direction="out" name="startOffset" type="i"/><arg direction="out" name="endOffset" type="i"/></method>
  <method name="GetNSelections"><arg direction="out" type="i"/></method>
  <method name="GetCharacterExtents"><arg direction="in" name="offset" type="i"/><arg direction="in" name="coordType" type="u"/><arg direction="out" name="x" type="i"/><arg direction="out" name="y" type="i"/><arg direction="out" name="width" type="i"/><arg direction="out" name="height" type="i"/></method>
  <method name="GetOffsetAtPoint"><arg direction="in" name="x" type="i"/><arg direction="in" name="y" type="i"/><arg direction="in" name="coordType" type="u"/><arg direction="out" type="i"/></method>
  <method name="GetAttributeValue"><arg direction="in" name="offset" type="i"/><arg direction="in" name="attributeName" type="s"/><arg direction="out" type="s"/></method>
  <method name="GetAttributes"><arg direction="in" name="offset" type="i"/><arg direction="out" type="a{ss}"/><arg direction="out" name="startOffset" type="i"/><arg direction="out" name="endOffset" type="i"/></method>
  <method name="GetAttributeRun"><arg direction="in" name="offset" type="i"/><arg direction="in" name="includeDefaults" type="b"/><arg direction="out" type="a{ss}"/><arg direction="out" name="startOffset" type="i"/><arg direction="out" name="endOffset" type="i"/></method>
  <method name="GetDefaultAttributes"><arg direction="out" type="a{ss}"/></method>
  <method name="GetDefaultAttributeSet"><arg direction="out" type="a{ss}"/></method>
</interface>
<interface name="org.a11y.atspi.EditableText">
  <method name="SetTextContents"><arg direction="in" name="newContents" type="s"/><arg direction="out" type="b"/></method>
</interface>
<interface name="org.a11y.atspi.Socket">
  <method name="Embedded"><arg direction="in" name="path" type="s"/></method>
</interface>
</node>`

// ---- GType subclass of GtkDrawingArea (TangoCanvasArea) ----

// gObjectClassMirror mirrors GObjectClass (gobject/gobject.h) on LP64:
// GTypeClass, construct_properties, eight method slots, flags,
// n_construct_properties, pspecs, n_pspecs and pdummy[3].
type gObjectClassMirror struct {
	gType                     uintptr // GTypeClass { GType g_type; }
	constructProperties       uintptr
	constructor               uintptr
	setProperty               uintptr
	getProperty               uintptr
	dispose                   uintptr
	finalize                  uintptr
	dispatchPropertiesChanged uintptr
	notify                    uintptr
	constructed               uintptr
	flags                     uintptr
	nConstructProperties      uintptr
	pspecs                    uintptr
	nPspecs                   uintptr
	pdummy                    [3]uintptr
}

// gtkWidgetClassMirror mirrors GtkWidgetClass (gtk/gtkwidget.h, GTK 3)
// up to get_accessible: GInitiallyUnownedClass parent_class, guint
// activate_signal, then the virtual functions in declaration order.
type gtkWidgetClassMirror struct {
	parent                         gObjectClassMirror
	activateSignal                 uint32
	dispatchChildPropertiesChanged uintptr
	// destroy, show, show_all, hide, map, unmap, realize, unrealize,
	// size_allocate, state_changed, state_flags_changed, parent_set,
	// hierarchy_changed, style_set, direction_changed, grab_notify,
	// child_notify, draw
	basics [18]uintptr
	// get_request_mode, get_preferred_height,
	// get_preferred_width_for_height, get_preferred_width,
	// get_preferred_height_for_width
	sizeRequests     [5]uintptr
	mnemonicActivate uintptr
	grabFocus, focus uintptr
	moveFocus        uintptr
	keynavFailed     uintptr
	// event, button_press_event … grab_broken_event (26 event handlers)
	events            [26]uintptr
	selectionGet      uintptr
	selectionReceived uintptr
	// drag_begin, drag_end, drag_data_get, drag_data_delete
	dragSource [4]uintptr
	// drag_leave, drag_motion, drag_drop, drag_data_received, drag_failed
	dragTarget    [5]uintptr
	popupMenu     uintptr
	showHelp      uintptr
	getAccessible uintptr
}

// gtkWidgetClassGetAccessible is the byte offset of
// GtkWidgetClass.get_accessible on LP64 (checked against the mirror in
// tests).
const gtkWidgetClassGetAccessible = 688

// gTypeQueryMirror mirrors GTypeQuery { GType type; const gchar
// *type_name; guint class_size; guint instance_size; }.
type gTypeQueryMirror struct {
	typ          uintptr
	name         uintptr
	classSize    uint32
	instanceSize uint32
}
