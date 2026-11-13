package ios

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestPowerStatus(t *testing.T) {
	for _, c := range []struct {
		state int
		level float64
		low   bool
		want  platform.PowerStatus
	}{
		// The simulator: no battery information at all.
		{0, -1, false, platform.PowerStatus{Battery: -1}},
		{1, 0.42, true, platform.PowerStatus{Source: platform.PowerSourceBattery, HasBattery: true, Battery: 0.42, LowPower: true}},
		{2, 0.5, false, platform.PowerStatus{Source: platform.PowerSourceAC, HasBattery: true, Battery: 0.5, Charging: true}},
		{3, 1, false, platform.PowerStatus{Source: platform.PowerSourceAC, HasBattery: true, Battery: 1}},
		{1, -1, false, platform.PowerStatus{Source: platform.PowerSourceBattery, HasBattery: true, Battery: -1}},
	} {
		if got := powerStatus(c.state, c.level, c.low); got != c.want {
			t.Errorf("powerStatus(%d, %v, %v) = %+v, want %+v", c.state, c.level, c.low, got, c.want)
		}
	}
}
