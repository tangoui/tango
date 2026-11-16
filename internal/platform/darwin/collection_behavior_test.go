//go:build darwin && !ios

package darwin

import "testing"

func TestWindowCollectionBehavior(t *testing.T) {
	for _, tc := range []struct {
		panel, allSpaces bool
		want             uint
	}{
		{false, false, 128}, // FullScreenPrimary
		{true, false, 320},  // FullScreenAuxiliary | IgnoresCycle
		{false, true, 257},  // CanJoinAllSpaces | FullScreenAuxiliary
		{true, true, 321},   // plus IgnoresCycle
	} {
		if got := windowCollectionBehavior(tc.panel, tc.allSpaces); got != tc.want {
			t.Errorf("panel=%v allSpaces=%v: behavior=%d, want %d", tc.panel, tc.allSpaces, got, tc.want)
		}
	}
}
