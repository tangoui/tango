package a11ybridge

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

func TestATSPINames(t *testing.T) {
	for _, c := range []struct {
		s    int
		name string
	}{
		{StateActive, "active"}, {StateBusy, "busy"}, {StateChecked, "checked"}, {StateEnabled, "enabled"},
		{StateFocused, "focused"}, {StateSensitive, "sensitive"}, {StateShowing, "showing"},
		{StateIndeterminate, "indeterminate"}, {StateInvalidEntry, "invalid-entry"},
		{StateVisited, "visited"}, {StateCheckable, "checkable"}, {StateHasPopup, "has-popup"},
		{StateReadOnly, "read-only"},
	} {
		if got := ATSPIStateName(c.s); got != c.name {
			t.Errorf("state %d: %q, want %q", c.s, got, c.name)
		}
	}
	if ATSPIRoleName(43) != "push button" || ATSPIRoleName(116) != "static" || ATSPIRoleName(9999) != "unknown" {
		t.Errorf("role names: %q %q", ATSPIRoleName(43), ATSPIRoleName(116))
	}
}

func TestATSPIStateDiff(t *testing.T) {
	a := ATSPIStates(&a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Checkable}, false)
	b := ATSPIStates(&a11y.Node{Role: a11y.RoleCheckbox, States: a11y.Checkable | a11y.Checked | a11y.Required}, false)
	d := ATSPIStateDiff(a, b)
	if len(d) != 2 || d[0] != (StateDelta{StateChecked, true}) || d[1] != (StateDelta{StateRequired, true}) {
		t.Errorf("diff %+v", d)
	}
	if len(ATSPIStateDiff(b, b)) != 0 {
		t.Error("no diff")
	}
}
