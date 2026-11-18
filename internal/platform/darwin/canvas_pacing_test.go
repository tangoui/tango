//go:build darwin && !ios

package darwin

import "testing"

// A frame asked for after a pause draws in the run loop pass that asked for
// it instead of waiting a whole refresh for the display link's next tick;
// while frames follow each other the link paces them, one per refresh.
func TestFrameAtOnce(t *testing.T) {
	for _, c := range []struct {
		name                 string
		idleTicks            int
		linkRunning, inFrame bool
		want                 bool
	}{
		{"link paused after idle ticks", linkIdleTicks, false, false, true},
		{"link running, last tick drew nothing", 1, true, false, true},
		{"link running, last tick drew a frame", 0, true, false, false},
		{"asked during a frame", 2, true, true, false},
		{"asked during a frame, link paused", linkIdleTicks, false, true, false},
	} {
		if got := frameAtOnce(c.idleTicks, c.linkRunning, c.inFrame); got != c.want {
			t.Errorf("%s: frameAtOnce = %v, want %v", c.name, got, c.want)
		}
	}
}
