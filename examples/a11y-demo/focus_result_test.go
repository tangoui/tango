package main

import "testing"

func TestFocusResult(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  focusEvidence
		pass bool
		want string
	}{
		{"button notification", focusEvidence{Total: 2, Target: 1, Focused: true}, true, "AXFocusedUIElementChanged for the button"},
		{"only another element", focusEvidence{Total: 1, Last: "I agree", Focused: true}, false, "the button is focused but its notification did not arrive (1 notifications, last \"I agree\")"},
		{"no notification", focusEvidence{Focused: true}, false, "the button is AXFocusedUIElement but no notification arrived"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pass, got := focusResult(tt.got)
			if pass != tt.pass || got != tt.want {
				t.Fatalf("focusResult = %v, %q; want %v, %q", pass, got, tt.pass, tt.want)
			}
		})
	}
}
