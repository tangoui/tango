//go:build darwin && !ios && !tango_dev

package darwin

import (
	"testing"
	"time"
)

// TestTestHooksInertRelease: with TANGO_SCHEME_PROBE=1 set, a production
// build still has no streaming probe and counts nothing.
func TestTestHooksInertRelease(t *testing.T) {
	t.Setenv("TANGO_SCHEME_PROBE", "1")
	if schemeProbe() {
		t.Fatal("schemeProbe on in a production build")
	}
	if serveProbe(0) {
		t.Fatal("serveProbe answered in a production build")
	}
	for _, p := range []string{"/__tango/lane/up", "/__tango/lane/down", "/__tango/probe/stream"} {
		lt := laneTrackFor(p)
		if lt != nil {
			t.Fatalf("laneTrackFor(%q) = %v, want nil", p, lt)
		}
		lt.respond(200) // the nil track's methods are no-ops
		lt.mainTime(time.Millisecond, 10)
		lt.data()
		lt.done()
	}
	if s := LaneDownStats(); s != (LaneStats{}) {
		t.Fatalf("LaneDownStats %+v, want zeros", s)
	}
}
