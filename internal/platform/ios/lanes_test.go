package ios

// Pure Go (no build tag): runs on the host too.

import (
	"slices"
	"testing"
)

func TestLaneCapsFromEnv(t *testing.T) {
	cases := []struct {
		darwin, lanes string
		want          []string
	}{
		{"1", "", []string{"up:post"}},
		{"1", "1", []string{"up:post"}},
		{"true", "", []string{"up:post"}}, // any other non-empty value is "1"
		{"0", "", nil},                    // "0", "false", "off" (any case, trimmed) mean off
		{"false", "", nil},
		{" OFF ", "", nil},
		{"False", "", nil},
		{" ", "", nil},
		{"ALL", "", []string{"down:scheme", "up:post"}},
		{"all", "", []string{"down:scheme", "up:post"}}, // experimental
		{"", "", nil},                                   // unset/empty: off on iOS (macOS differs on purpose: default on)
		{"1", "0", nil},                                 // the kill switch wins
		{"all", "0", nil},
		{"", "0", nil},
	}
	for _, c := range cases {
		env := map[string]string{"TANGO_IPC_LANES_DARWIN": c.darwin, "TANGO_IPC_LANES": c.lanes}
		got := laneCapsFromEnv(func(k string) string { return env[k] }, true)
		if !slices.Equal(got, c.want) {
			t.Errorf("DARWIN=%q LANES=%q: caps %q, want %q", c.darwin, c.lanes, got, c.want)
		}
	}
}

// Without the self-test hooks (a release build) "all" is a plain "on".
func TestLaneCapsAllWithoutHooks(t *testing.T) {
	env := map[string]string{"TANGO_IPC_LANES_DARWIN": "all"}
	if got := laneCapsFromEnv(func(k string) string { return env[k] }, false); !slices.Equal(got, []string{"up:post"}) {
		t.Fatalf("caps %q, want [up:post]", got)
	}
}

func TestLaneProbeCounters(t *testing.T) {
	var c laneCounters
	if c.track("/index.html") != nil || c.track("/__tango/lane/other") != nil {
		t.Fatal("non-lane paths are tracked")
	}

	// A refused down request: counted as a request, not a connection.
	d := c.track("/__tango/lane/down")
	d.respond(403)
	d.data()
	d.done()
	// An accepted one: a connection, active until done, its data blocks.
	a := c.track("/__tango/lane/down")
	a.respond(200)
	a.data()
	a.data()
	if s := c.stats(); s.DownActive != 1 || s.DownConns != 1 {
		t.Fatalf("while open: %+v", s)
	}
	a.done()
	a.done() // repeated: stop after finish must not count twice
	// Up: a refused and an accepted request.
	u := c.track("/__tango/lane/up")
	u.respond(403)
	u.done()
	u = c.track("/__tango/lane/up")
	u.respond(204)
	if s := c.stats(); s.UpActive != 1 {
		t.Fatalf("up request open: %+v", s)
	}
	u.done()
	u.done()                // repeated: must not count twice
	var nilTrack *laneTrack // untracked tasks call these too
	nilTrack.respond(200)
	nilTrack.data()
	nilTrack.done()

	want := LaneStats{DownRequests: 2, DownConns: 1, DownActive: 0, DownWrites: 2, UpRequests: 2, UpAccepted: 1}
	if s := c.stats(); s != want {
		t.Fatalf("stats %+v, want %+v", s, want)
	}
}
