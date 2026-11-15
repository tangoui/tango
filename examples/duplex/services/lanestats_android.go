//go:build android

package services

import "github.com/tangoui/tango/internal/platform/android"

// backendLaneStats fills st from the Android backend's lane counters
// (counted in tango_dev and tango_selftest builds only): upstream lane
// messages as upRequests, those the core took as upAccepted, down packets
// posted to the page as downWrites, and the lane* fields. upPackets,
// upFrames and upBytes stay those of OnBinary (CountUp).
func backendLaneStats(st *Stats) {
	l := android.LaneStatsSnapshot()
	st.UpRequests = l.UpPackets
	st.UpAccepted = l.UpPackets - l.UpRefused - l.UpBadToken
	st.DownWrites = l.DownPackets
	st.LaneUpProbes, st.LaneUpBadToken, st.LaneUpRefused = l.UpProbes, l.UpBadToken, l.UpRefused
	st.LaneDownDropped, st.LaneDownQueued = l.DownDropped, l.DownQueued
}
