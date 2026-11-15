//go:build darwin && !ios

package darwin

// IPC fast lanes (platform.BinaryLane). The asset scheme handler reads
// request bodies, so the page can use the core's up:post lane (POST
// /__tango/lane/up, served by the core through Host.ServeAsset). It also
// streams responses (scheme.go), but WebKit holds back part of a large
// burst of a streamed response until more data arrives on it (spec §6.2,
// measured in the Apple lanes plan's Task 4), so down:scheme is not offered:
// downstream frames stay on the message channel. There is no platform lane
// of our own.
//
// Default on (up:post) since the Apple lanes plan's Task 7 benchmark
// (upload 2.73x, docs/ipc-bench.md): a surface reports up:post unless
// TANGO_IPC_LANES_DARWIN was 0, false or off when it was created, or
// TANGO_IPC_LANES=0. In development builds (-tags tango_dev)
// TANGO_IPC_LANES_DARWIN=all also offers down:scheme, for experiments only
// (laneCapsFromEnv). runtime-js starts up:post with a
// probe POST, so a page whose CSP blocks the asset origin simply keeps the
// message channel. iOS reads the same variable but stays off by default
// (internal/platform/ios/lanes.go): the two differ on purpose.
//
// Frames: a WKURLSchemeTask does not say which frame asked, so a lane
// request from a subframe cannot be refused here. The lanes rely on the
// token instead: it is only sent in the hello reply, which only the main
// frame's runtime gets (the runtime is injected into the main frame only,
// and receiveScriptMessage drops subframe messages). TestLaneSubframeAccess
// checks this.

import (
	"os"
	"slices"
	"strings"

	"github.com/tangoui/tango/internal/lane"
)

// laneCapsFromEnv returns the lanes a new surface reports, from the
// environment read through getenv:
//
//   - TANGO_IPC_LANES=0: none (the kill switch wins).
//   - TANGO_IPC_LANES_DARWIN "0", "false" or "off" (any case, surrounding
//     spaces ignored): none.
//   - TANGO_IPC_LANES_DARWIN=all, development builds (-tags tango_dev)
//     only: down:scheme and up:post. Experimental: WebKit holds back large
//     down:scheme bursts, see spec §6.2
//     (docs/specs/2026-11-13-ipc-duplex-streams.md). Production builds
//     treat "all" like the default.
//   - anything else, unset and empty included: up:post (the default since
//     the Task 7 benchmark).
//
// iOS has its own copy with the opposite default (off); keep them apart.
func laneCapsFromEnv(getenv func(string) string) []string {
	if getenv("TANGO_IPC_LANES") == "0" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(getenv("TANGO_IPC_LANES_DARWIN"))) {
	case "0", "false", "off":
		return nil
	case "all":
		if devTestHooks {
			return []string{lane.DownScheme, lane.UpPost}
		}
	}
	return []string{lane.UpPost}
}

// envLaneCaps returns the lanes of a surface created now.
func envLaneCaps() []string { return laneCapsFromEnv(os.Getenv) }

// LaneCaps implements platform.BinaryLane. Any goroutine: laneCaps is set
// before the surface is handed to the core and never changes.
func (s *surface) LaneCaps() []string { return slices.Clone(s.laneCaps) }

// SendLanePacket implements platform.BinaryLane: macOS has no platform lane
// (down:scheme is the core's), so no packet is ever taken. Any goroutine.
func (s *surface) SendLanePacket(uint64, []byte) bool { return false }

// ---------------------------------------------------------------------------
// Lane counters (test hook). In development builds (-tags tango_dev,
// lanes_dev.go) with TANGO_SCHEME_PROBE=1 the scheme handler counts the
// lane requests it sees and how they were answered, for the platform tests
// (examples/duplex's Debug.Stats reads LaneDownStats). Production builds
// (lanes_release.go) never count: LaneDownStats returns zeros there.

// LaneStats are the scheme handler's lane counters.
type LaneStats struct {
	DownRequests int64 // GET /__tango/lane/down requests
	DownConns    int64 // of them, answered 200: accepted down connections
	DownActive   int64 // accepted down connections not yet finished or stopped
	DownWrites   int64 // data blocks handed to WebKit on accepted down connections
	UpRequests   int64 // POST /__tango/lane/up requests
	UpAccepted   int64 // of them, answered 2xx
	UpActive     int64 // POST /__tango/lane/up requests not yet finished or stopped
	// Main-thread cost of the up requests: time serveSchemeTask spent on
	// each (copying the request and its body, before the handler
	// goroutine starts), summed and the largest; and the body bytes read.
	UpMainNanos    int64
	UpMainMaxNanos int64
	UpBodyBytes    int64
}
