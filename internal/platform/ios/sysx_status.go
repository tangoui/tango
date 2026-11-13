package ios

import "github.com/tangoui/tango/internal/platform"

// Pure logic of the Power capability (sysx.go), testable on any host.

// powerStatus maps UIDeviceBatteryState (0 unknown, 1 unplugged,
// 2 charging, 3 full) and the level to a PowerStatus.
func powerStatus(state int, level float64, low bool) platform.PowerStatus {
	st := platform.PowerStatus{Battery: -1, LowPower: low}
	switch state {
	case 1:
		st.Source = platform.PowerSourceBattery
	case 2, 3:
		st.Source = platform.PowerSourceAC
	}
	st.HasBattery = state != 0
	st.Charging = state == 2
	if st.HasBattery && level >= 0 {
		st.Battery = min(level, 1)
	}
	return st
}
