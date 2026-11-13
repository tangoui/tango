package a11ybridge

// atspiRoleNames is AtspiRole -> role name (the GetRoleName form: lower
// case, words separated by spaces, as atspi_role_get_name returns).
var atspiRoleNames = func() map[uint32]string {
	m := make(map[uint32]string, len(atspiRoles))
	for name, id := range atspiRoles {
		m[id] = atspiLower(name)
	}
	return m
}()

func atspiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c == '_':
			b[i] = ' '
		case c >= 'A' && c <= 'Z':
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// ATSPIRoleName returns the role name of an AtspiRole value ("push
// button"), "unknown" for values the bridge does not use.
func ATSPIRoleName(id uint32) string {
	if s, ok := atspiRoleNames[id]; ok {
		return s
	}
	return "unknown"
}

// atspiStateNames are the AtspiStateType nicks in enum order (the detail
// string of object:state-changed events).
var atspiStateNames = []string{
	"invalid", "active", "armed", "busy", "checked", "collapsed", "defunct", "editable",
	"enabled", "expandable", "expanded", "focusable", "focused", "has-tooltip", "horizontal",
	"iconified", "modal", "multi-line", "multiselectable", "opaque", "pressed", "resizable",
	"selectable", "selected", "sensitive", "showing", "single-line", "stale", "transient",
	"vertical", "visible", "manages-descendants", "indeterminate", "required", "truncated",
	"animated", "invalid-entry", "supports-autocompletion", "selectable-text", "is-default",
	"visited", "checkable", "has-popup", "read-only",
}

// ATSPIStateName returns the nick of AtspiStateType s ("" when unknown).
func ATSPIStateName(s int) string {
	if s < 0 || s >= len(atspiStateNames) {
		return ""
	}
	return atspiStateNames[s]
}

// StateDelta is one AT-SPI state that differs between two state sets.
type StateDelta struct {
	State int
	On    bool
}

// ATSPIStateDiff lists the states that differ between a and b (two
// ATSPIStates results), in enum order.
func ATSPIStateDiff(a, b [2]uint32) []StateDelta {
	var out []StateDelta
	for s := range len(atspiStateNames) {
		w, bit := s/32, uint32(1)<<(s%32)
		if (a[w]&bit != 0) != (b[w]&bit != 0) {
			out = append(out, StateDelta{s, b[w]&bit != 0})
		}
	}
	return out
}
