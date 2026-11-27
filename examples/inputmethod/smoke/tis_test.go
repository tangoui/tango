//go:build darwin && !ios

package main

import "testing"

func TestStatusUsesEffectiveEnabledList(t *testing.T) {
	const (
		all     = 1
		enabled = 2
		zh      = 3
		en      = 4
	)
	fake := tis{
		createList: func(_ uintptr, includeDisabled bool) uintptr {
			if includeDisabled {
				return all
			}
			return enabled
		},
		count: func(list uintptr) int {
			if list == all {
				return 2
			}
			return 1
		},
		at:       func(_ uintptr, index int) uintptr { return zh + uintptr(index) },
		property: func(source, _ uintptr) uintptr { return source },
		string: func(source uintptr, buf []byte, _ int, _ uint32) bool {
			copy(buf, map[uintptr]string{zh: "tango.zh", en: "tango.en"}[source])
			return true
		},
		release: func(uintptr) {},
	}
	// Both modes are registered. Only the Chinese mode is effectively enabled.
	for _, tc := range []struct{ id, want string }{
		{"tango.zh", "enabled"},
		{"tango.en", "disabled"},
		{"other", "missing"},
	} {
		got, err := fake.status(tc.id)
		if err != nil || got != tc.want {
			t.Errorf("status %s = %q, %v; want %q", tc.id, got, err, tc.want)
		}
	}
}
