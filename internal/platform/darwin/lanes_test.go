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
	cases := []struct {
		darwin, lanes string
		want          []string
	}{
		{"", "", []string{"up:post"}}, // unset/empty: on by default (macOS only; iOS is off)
		{" ", "", []string{"up:post"}},
		{"1", "", []string{"up:post"}},
		{"1", "1", []string{"up:post"}},
		{"true", "", []string{"up:post"}},
		{"yes", "", []string{"up:post"}}, // any other value: on
		{"0", "", nil},                   // "0", "false", "off" (any case, trimmed) mean off
		{"false", "", nil},
		{" OFF ", "", nil},
		{"False", "", nil},
		{"", "0", nil}, // the kill switch wins
		{"1", "0", nil},
		{"all", "0", nil},
		// "all" alone depends on the build: lanes_release_test.go and
		// lanes_dev_test.go.
	}
	for _, c := range cases {
		env := map[string]string{"TANGO_IPC_LANES_DARWIN": c.darwin, "TANGO_IPC_LANES": c.lanes}
		got := laneCapsFromEnv(func(k string) string { return env[k] })
		if !slices.Equal(got, c.want) {
			t.Errorf("DARWIN=%q LANES=%q: caps %q, want %q", c.darwin, c.lanes, got, c.want)
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
	if !slices.Equal(got, []string{"up:post"}) {
		t.Fatalf("LaneCaps %q", got)
	}
	got[0] = "x"
	if s.LaneCaps()[0] != "up:post" {
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
