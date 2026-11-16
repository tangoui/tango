//go:build darwin && !ios

package darwin

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.BinaryLane = (*surface)(nil)

func TestLaneCapsFromEnv(t *testing.T) {
	both := []string{"down:poll", "up:post"}
	up := []string{"up:post"}
	down := []string{"down:poll"}
	cases := []struct {
		darwin, lanes string
		pollDefault   bool // downPollDefault
		want          []string
	}{
		// Unset/empty (and unknown values): the default, which offers
		// down:poll only once downPollDefault is on (Task 9).
		{"", "", false, up},
		{"", "", true, both},
		{" ", "", false, up},
		{" ", "", true, both},
		{"yes", "", false, up},
		{"yes", "", true, both},
		// "1" and "all" (any case, trimmed, any build): both lanes.
		{"1", "", false, both},
		{"1", "", true, both},
		{"ALL ", "", false, both},
		{"all", "", false, both},
		{" All ", "", true, both},
		{"1", "1", false, both},
		// "up" / "down": one lane, for troubleshooting.
		{"up", "", false, up},
		{"up", "", true, up},
		{" UP", "", false, up},
		{"down", "", false, down},
		{"down", "", true, down},
		{"Down ", "", false, down},
		// "0", "false", "off" (any case, trimmed): none.
		{"0", "", false, nil},
		{"0", "", true, nil},
		{"false", "", false, nil},
		{"False", "", true, nil},
		{"Off", "", false, nil},
		{" OFF ", "", true, nil},
		// The kill switch wins over everything.
		{"", "0", false, nil},
		{"", "0", true, nil},
		{"1", "0", false, nil},
		{"all", "0", false, nil},
		{"all", "0", true, nil},
		{"down", "0", false, nil},
	}
	saved := downPollDefault
	t.Cleanup(func() { downPollDefault = saved })
	for _, c := range cases {
		downPollDefault = c.pollDefault
		env := map[string]string{"TANGO_IPC_LANES_DARWIN": c.darwin, "TANGO_IPC_LANES": c.lanes}
		got := laneCapsFromEnv(func(k string) string { return env[k] })
		if !slices.Equal(got, c.want) {
			t.Errorf("DARWIN=%q LANES=%q downPollDefault=%v: caps %q, want %q", c.darwin, c.lanes, c.pollDefault, got, c.want)
		}
	}
}

// TestSurfaceLaneCaps: the caps are fixed at creation, callers get their
// own copy, SendLanePacket never takes a packet, and both are safe from
// any goroutine (run with -race).
func TestSurfaceLaneCaps(t *testing.T) {
	s := &surface{laneCaps: laneCapsFromEnv(func(k string) string {
		if k == "TANGO_IPC_LANES_DARWIN" {
			return "1"
		}
		return ""
	})}
	got := s.LaneCaps()
	if !slices.Equal(got, []string{"down:poll", "up:post"}) {
		t.Fatalf("LaneCaps %q", got)
	}
	got[0] = "x"
	if s.LaneCaps()[0] != "down:poll" {
		t.Fatal("LaneCaps returned the surface's own slice")
	}
	if s.SendLanePacket(1, []byte{1, 2, 3}) {
		t.Fatal("SendLanePacket took a packet; darwin has no platform lane")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = s.LaneCaps()
				_ = s.SendLanePacket(1, nil)
			}
		}()
	}
	wg.Wait()
	if (&surface{}).LaneCaps() != nil {
		t.Fatal("a surface created with lanes off reports caps")
	}
}
