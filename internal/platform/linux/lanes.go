package linux

import (
	"strings"

	"github.com/tangoui/tango/internal/lane"
)

// Keep both lanes off by default until the benchmark meets the acceptance
// thresholds in the IPC duplex streams spec.
const (
	upPostDefault   = false
	downPollDefault = false
)

// laneCapsFromEnv reports only lanes supported by WebKitGTK on this host.
// TANGO_IPC_LANES_LINUX accepts "1"/"all", "up", "down", or "0"/"false"/"off",
// ignoring case and surrounding spaces. Other values use the defaults.
// TANGO_IPC_LANES=0 disables both regardless of the Linux switch.
func laneCapsFromEnv(getenv func(string) string, hasBody, hasStream bool) []string {
	if getenv("TANGO_IPC_LANES") == "0" {
		return nil
	}
	up, down := upPostDefault, downPollDefault
	switch strings.ToLower(strings.TrimSpace(getenv("TANGO_IPC_LANES_LINUX"))) {
	case "0", "false", "off":
		return nil
	case "1", "all":
		up, down = true, true
	case "up":
		up, down = true, false
	case "down":
		up, down = false, true
	}
	var caps []string
	if down && hasStream {
		caps = append(caps, lane.DownPoll)
	}
	if up && hasBody {
		caps = append(caps, lane.UpPost)
	}
	return caps
}
