//go:build linux && !android

package linux

// Linux offers the core's HTTP lanes through Host.ServeAsset. WebKitGTK's
// scheme request identifies the WebView but does not identify the frame.
// As on Darwin, requests need a lane token that only the main frame's
// runtime receives.

import (
	"slices"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.BinaryLane = (*surface)(nil)

// LaneCaps implements platform.BinaryLane. The capabilities are set at
// surface creation and immutable, so they are safe to read from any goroutine.
func (s *surface) LaneCaps() []string { return slices.Clone(s.laneCaps) }

// SendLanePacket implements platform.BinaryLane. The core handles both
// HTTP lanes, so the Linux backend never takes a packet directly.
func (s *surface) SendLanePacket(uint64, []byte) bool { return false }
