package android

import (
	"encoding/json"

	"github.com/tangoui/tango/internal/platform"
)

// Pure logic of the Shell / Notifier / Network / Power capabilities
// (sysx_android.go): decoding what org.tango.host.Sysx answers.

// javaNetwork is the "networkStatus" answer; Kind is a
// platform.NetworkKind value.
type javaNetwork struct {
	Online      bool `json:"online"`
	Kind        int  `json:"kind"`
	Metered     bool `json:"metered"`
	Constrained bool `json:"constrained"`
}

func decodeNetwork(s string) (platform.NetworkStatus, error) {
	var j javaNetwork
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		return platform.NetworkStatus{}, err
	}
	return platform.NetworkStatus{Online: j.Online, Kind: platform.NetworkKind(j.Kind),
		Metered: j.Metered, Constrained: j.Constrained}, nil
}

// javaPower is the "powerStatus" answer: the battery broadcast's facts
// and PowerManager.isPowerSaveMode.
type javaPower struct {
	Present  bool    `json:"present"`
	Plugged  bool    `json:"plugged"`
	Charging bool    `json:"charging"`
	Level    float64 `json:"level"`
	LowPower bool    `json:"lowPower"`
}

func decodePower(s string) (platform.PowerStatus, error) {
	var j javaPower
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		return platform.PowerStatus{Battery: -1}, err
	}
	return powerFromJava(j), nil
}

// powerFromJava maps the battery broadcast to a PowerStatus: plugged in
// counts as AC, a device without a battery (an emulator may report one)
// has Battery -1.
func powerFromJava(j javaPower) platform.PowerStatus {
	st := platform.PowerStatus{Battery: -1, LowPower: j.LowPower, HasBattery: j.Present}
	if j.Present {
		st.Source = platform.PowerSourceBattery
		st.Charging = j.Charging
		if j.Level >= 0 {
			st.Battery = min(j.Level, 1)
		}
	}
	if j.Plugged {
		st.Source = platform.PowerSourceAC
	}
	return st
}
