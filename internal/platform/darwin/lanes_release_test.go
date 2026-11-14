//go:build darwin && !ios && !tango_dev

package darwin

import (
	"slices"
	"testing"
	"time"
)

// TestLaneCapsAllRelease: production builds ignore the experimental
// TANGO_IPC_LANES_DARWIN=all (it behaves like the default, up:post only);
// the off switches still work.
func TestLaneCapsAllRelease(t *testing.T) {
	for _, v := range []string{"all", "ALL", " All "} {
		env := map[string]string{"TANGO_IPC_LANES_DARWIN": v}
		if got := laneCapsFromEnv(func(k string) string { return env[k] }); !slices.Equal(got, []string{"up:post"}) {
			t.Errorf("DARWIN=%q: caps %q, want up:post only", v, got)
		}
	}
	for _, env := range []map[string]string{
		{"TANGO_IPC_LANES_DARWIN": "0"},
		{"TANGO_IPC_LANES_DARWIN": "off"},
		{"TANGO_IPC_LANES_DARWIN": "all", "TANGO_IPC_LANES": "0"},
	} {
		if got := laneCapsFromEnv(func(k string) string { return env[k] }); got != nil {
			t.Errorf("%v: caps %q, want none", env, got)
		}
	}
}

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
