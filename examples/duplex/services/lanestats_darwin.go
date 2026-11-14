//go:build darwin && !ios

package services

import "github.com/tangoui/tango/internal/platform/darwin"

// backendLaneStats fills st's scheme handler counters from the macOS backend.
// They are counted only in development builds (-tags tango_dev); in a
// plain build darwin.LaneDownStats returns zeros.
func backendLaneStats(st *Stats) {
	l := darwin.LaneDownStats()
	st.DownRequests, st.DownConns, st.DownActive, st.DownWrites = l.DownRequests, l.DownConns, l.DownActive, l.DownWrites
	st.UpRequests, st.UpAccepted, st.UpActive = l.UpRequests, l.UpAccepted, l.UpActive
	st.UpMainNanos, st.UpMainMaxNanos, st.UpBodyBytes = l.UpMainNanos, l.UpMainMaxNanos, l.UpBodyBytes
}
