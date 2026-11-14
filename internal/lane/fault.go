//go:build tango_dev || tango_selftest

package lane

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Fault injection for development builds (-tags tango_dev) and self-test
// builds (-tags tango_selftest, e.g. the iOS simulator selftest, which tan
// build cannot give tango_dev): TANGO_IPC_LANE_FAULT=<lane>:<N>
// makes the lane fail after N data packets in each generation (sent for a
// downstream lane, received for an upstream one; empty packets, count 0,
// such as down:scheme's leading [0,0] and the up:post start probe, are not
// counted, though a faulted lane refuses them too): later downstream packets
// are refused (Send returns false and the lane is marked failed; a failing
// down:scheme ends its response), later upstream packets are dropped
// (UpAllowed returns false). Platform plans use it to test a lane breaking
// mid-stream.

func loadFault() (string, int, bool) {
	v := os.Getenv("TANGO_IPC_LANE_FAULT")
	name, n, ok := parseFault(v)
	if !ok && strings.TrimSpace(v) != "" {
		slog.Warn("tango: ignoring malformed TANGO_IPC_LANE_FAULT (want <lane>:<N>)", "value", v)
	}
	return name, n, ok
}

// parseFault parses "<lane>:<N>" with a known lane name and N >= 0.
func parseFault(v string) (name string, n int, ok bool) {
	v = strings.TrimSpace(v)
	i := strings.LastIndexByte(v, ':')
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(v[i+1:])
	if err != nil || n < 0 || !known(v[:i]) {
		return "", 0, false
	}
	return v[:i], n, true
}
