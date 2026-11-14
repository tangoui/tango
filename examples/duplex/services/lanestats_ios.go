//go:build ios

package services

import "github.com/tangoui/tango/internal/platform/ios"

// backendLaneStats fills st's scheme handler counters from the iOS backend.
func backendLaneStats(st *Stats) {
	l := ios.LaneDownStats()
	st.DownRequests, st.DownConns, st.DownActive, st.DownWrites = l.DownRequests, l.DownConns, l.DownActive, l.DownWrites
	st.UpRequests, st.UpAccepted, st.UpActive = l.UpRequests, l.UpAccepted, l.UpActive
}
