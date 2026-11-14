//go:build ios

package ios

// IPC fast lanes (platform.BinaryLane). The tango:// scheme handler reads
// request bodies, so the page can use the core's up:post lane (POST
// /__tango/lane/up, served by the core through Host.ServeAsset). It also
// streams responses (scheme.go), but WebKit holds back part of a large
// burst of a streamed response until more data arrives on it (spec §6.2;
// seen on the iOS 27.2 simulator as on macOS), so down:scheme is not
// offered: downstream frames stay on the message channel. There is no
// platform lane of our own.
//
// Default off: a surface reports lanes only when TANGO_IPC_LANES_DARWIN was
// set to an "on" value (not empty, 0, false or off) when it was created, and
// TANGO_IPC_LANES=0 still turns them off, until measured on a real device
// (macOS is default-on).
// TANGO_IPC_LANES_DARWIN=all also offers down:scheme, for experiments only
// (laneCapsFromEnv, lanes.go).
//
// Frames: a WKURLSchemeTask does not say which frame asked, so a lane
// request from a subframe cannot be refused here. The lanes rely on the
// token instead: it is only sent in the hello reply, which only the main
// frame's runtime gets (bridge.m injects the runtime into the main frame
// only and drops subframe script messages). The macOS test
// TestLaneSubframeAccess checks the same arrangement.

import (
	"os"
	"slices"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.BinaryLane = (*surface)(nil)

// LaneCaps implements platform.BinaryLane. Any goroutine: laneCaps is set
// before the surface is handed to the core and never changes.
func (s *surface) LaneCaps() []string { return slices.Clone(s.laneCaps) }

// SendLanePacket implements platform.BinaryLane: iOS has no platform lane
// (down:scheme is the core's), so no packet is ever taken. Any goroutine.
func (s *surface) SendLanePacket([]byte) bool { return false }

// envLaneCaps returns the lanes of a WebView surface created now.
func envLaneCaps() []string { return laneCapsFromEnv(os.Getenv, testHooks) }
