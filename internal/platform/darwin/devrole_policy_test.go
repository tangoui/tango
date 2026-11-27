//go:build darwin && !ios

package darwin

import "testing"

func TestPolicyFor(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		worker, background, ui bool
		want                   uintptr
	}{
		{"regular", false, false, false, 0},
		{"background", false, true, false, 2},
		{"ui element", false, false, true, 1},
		{"dev worker", true, false, false, 1},
		{"background takes precedence", true, true, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := policyFor(tc.worker, tc.background, tc.ui); got != tc.want {
				t.Errorf("policy = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPlistBool(t *testing.T) {
	withPool(func() {
		for _, tc := range []struct {
			value id
			want  bool
		}{
			{0, false},
			{send(class("NSNumber"), "numberWithBool:", 1), true},
			{nsString("1"), true},
			{nsString("YES"), true},
			{nsString("NO"), false},
			{send(class("NSArray"), "array"), false},
		} {
			if got := plistBool(tc.value); got != tc.want {
				t.Errorf("plistBool = %t, want %t", got, tc.want)
			}
		}
	})
}
