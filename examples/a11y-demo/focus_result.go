package main

import "fmt"

// focusEvidence is what the AX focus smoke check saw. Target counts only
// notifications whose element is the button, so another element's focus
// change cannot satisfy or misreport the check.
type focusEvidence struct {
	Total, Target int
	Last          string
	Focused       bool
}

func focusResult(e focusEvidence) (bool, string) {
	if e.Target > 0 {
		return true, "AXFocusedUIElementChanged for the button"
	}
	if e.Focused && e.Total > 0 {
		return false, fmt.Sprintf("the button is focused but its notification did not arrive (%d notifications, last %q)", e.Total, e.Last)
	}
	if e.Focused {
		return false, "the button is AXFocusedUIElement but no notification arrived"
	}
	return false, fmt.Sprintf("notifications %d (last %q), button notifications 0; window not key? (locked screen)", e.Total, e.Last)
}
