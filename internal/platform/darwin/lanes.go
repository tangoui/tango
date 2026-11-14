//go:build darwin && !ios

package darwin

// IPC fast lanes (platform.BinaryLane). Both lanes macOS offers are the
// core's, served through Host.ServeAsset by the asset scheme handler; there
// is no platform lane of our own.
//
//   - up:post: the page POSTs to /__tango/lane/up (the scheme handler reads
//     request bodies). On by default since the Apple lanes plan's Task 7
//     benchmark (upload 2.73x, docs/ipc-bench.md).
//   - down:poll: the page polls GET /__tango/lane/poll and the core answers
//     with batches of queued downstream items (messages, byte packets, Eval
//     barriers). It replaces down:scheme, a streamed response that WebKit
//     held back part of a large burst of (spec §6.2). Off by default until
//     it meets the down:poll design's §6 thresholds (downPollDefault,
//     flipped by Task 9 of that plan); without it downstream frames stay on the
//     message channel (sequenced Eval).
//
// Which lanes a surface reports is fixed when it is created, from
// TANGO_IPC_LANES_DARWIN (any case, surrounding spaces ignored; every
// build): unset or any other value, the default (up:post, plus down:poll
// once downPollDefault is on); "1" or "all", both; "up" or "down", only
// that one (for troubleshooting); "0", "false" or "off", none.
// TANGO_IPC_LANES=0 turns every lane off whatever the variable says, as
// does Options.DisableFastLanes in the core. The core also only opens a
// lane for a page loaded from the asset origin. runtime-js starts up:post
// with a probe POST, so a page whose CSP blocks the asset origin simply
// keeps the message channel. iOS reads the same variable but stays off by
// default (internal/platform/ios/lanes.go): the two differ on purpose.
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

// downPollDefault says whether a surface created with
// TANGO_IPC_LANES_DARWIN unset (or set to a value not listed in
// laneCapsFromEnv) reports down:poll. False until down:poll
// meets the design's §6 thresholds (Task 9 of the down-poll plan flips
// it); tests may change it.
var downPollDefault = false

// laneCapsFromEnv returns the lanes a new surface reports, from the
// environment read through getenv, in the order down:poll, up:post:
//
//   - TANGO_IPC_LANES=0: none (the kill switch wins).
//   - TANGO_IPC_LANES_DARWIN "1" or "all": down:poll and up:post.
//   - TANGO_IPC_LANES_DARWIN "up": up:post only.
//   - TANGO_IPC_LANES_DARWIN "down": down:poll only.
//   - TANGO_IPC_LANES_DARWIN "0", "false" or "off": none.
//   - anything else, unset and empty included: up:post, plus down:poll
//     when downPollDefault is on.
//
// TANGO_IPC_LANES_DARWIN is read in any case with surrounding spaces
// ignored, and means the same in every build.
//
// iOS has its own copy with the opposite default (off); keep them apart.
func laneCapsFromEnv(getenv func(string) string) []string {
	if getenv("TANGO_IPC_LANES") == "0" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(getenv("TANGO_IPC_LANES_DARWIN"))) {
	case "0", "false", "off":
		return nil
	case "1", "all":
		return []string{lane.DownPoll, lane.UpPost}
	case "up":
		return []string{lane.UpPost}
	case "down":
		return []string{lane.DownPoll}
	}
	if downPollDefault {
		return []string{lane.DownPoll, lane.UpPost}
	}
	return []string{lane.UpPost}
}

// envLaneCaps returns the lanes of a surface created now.
func envLaneCaps() []string { return laneCapsFromEnv(os.Getenv) }

// LaneCaps implements platform.BinaryLane. Any goroutine: laneCaps is set
// before the surface is handed to the core and never changes.
func (s *surface) LaneCaps() []string { return slices.Clone(s.laneCaps) }

// SendLanePacket implements platform.BinaryLane: macOS has no platform lane
// (down:poll is the core's), so no packet is ever taken. Any goroutine.
func (s *surface) SendLanePacket([]byte) bool { return false }

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
