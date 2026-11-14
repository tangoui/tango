//go:build darwin && !ios && !tango_dev

package darwin

// Production builds leave out the lanes' test hooks (lanes_dev.go):
// TANGO_IPC_LANES_DARWIN=all behaves like the default (up:post only) and
// TANGO_SCHEME_PROBE is ignored, so nothing is counted. The off switches
// (TANGO_IPC_LANES_DARWIN=0/false/off, TANGO_IPC_LANES=0) work in every
// build.

import "time"

// devTestHooks is false in production builds.
const devTestHooks = false

// LaneDownStats returns zeros: production builds do not count. Any
// goroutine.
func LaneDownStats() LaneStats { return LaneStats{} }

// laneTrack is empty in production builds; laneTrackFor always returns
// nil and the methods do nothing.
type laneTrack struct{}

func laneTrackFor(string) *laneTrack { return nil }

func (*laneTrack) respond(int)                   {}
func (*laneTrack) mainTime(time.Duration, int64) {}
func (*laneTrack) data()                         {}
func (*laneTrack) done()                         {}
