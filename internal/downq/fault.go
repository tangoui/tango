//go:build tango_dev || tango_selftest

package downq

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Fault injection for development and self-test builds (the build tags of
// internal/lane's fault.go): TANGO_IPC_LANE_FAULT=down:poll:<N> cuts off
// the poll response that would carry the generation's N+1th byte packet
// (kind Packet; messages and barriers are not counted), once per
// generation: the batch is written up to the middle of that item and the
// response ends there, as if WebKit had broken it off. The page cannot
// decode it, reports failure, and the generation falls back to Eval with
// the unacknowledged items sent again (Fail). Other lanes' values are
// internal/lane's; release builds ignore the variable (fault_release.go).

func loadFault() (n int, ok bool) {
	v := strings.TrimSpace(os.Getenv("TANGO_IPC_LANE_FAULT"))
	rest, found := strings.CutPrefix(v, "down:poll:")
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// noteFault logs an injected cut, so that tests can tell it happened.
func noteFault(gen, seq uint64) {
	slog.Warn("tango: downq: injected fault: poll response cut off", "gen", gen, "seq", seq)
}
