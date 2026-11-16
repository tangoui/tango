//go:build darwin && !ios && !tango_dev

package darwin

// Production builds leave out the lanes' test hooks (lanes_dev.go):
// TANGO_SCHEME_PROBE is ignored, so nothing is counted.
// TANGO_IPC_LANES_DARWIN and TANGO_IPC_LANES mean the same in every build
// (laneCapsFromEnv).

import "time"

// LaneDownStats returns zeros: production builds do not count. Any
// goroutine.
func LaneDownStats() LaneStats { return LaneStats{} }

// laneTrack is empty in production builds; laneTrackFor always returns
// nil and the methods do nothing.
type laneTrack struct{}

func laneTrackFor(string) *laneTrack { return nil }

func (*laneTrack) respond(int)                   {}
func (*laneTrack) mainTime(time.Duration, int64) {}
func (*laneTrack) data(...int)                   {}
func (*laneTrack) done()                         {}
