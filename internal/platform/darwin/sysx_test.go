//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestPowerStatusFrom(t *testing.T) {
	desktop := powerStatusFrom("AC Power", nil, false)
	if desktop != (platform.PowerStatus{Source: platform.PowerSourceAC, Battery: -1}) {
		t.Fatalf("%+v", desktop)
	}
	laptop := powerStatusFrom("Battery Power", []powerSource{
		{internal: false, present: true, current: 5, max: 10}, // a UPS / accessory: ignored
		{internal: true, present: true, current: 42, max: 100},
	}, true)
	want := platform.PowerStatus{Source: platform.PowerSourceBattery, HasBattery: true, Battery: 0.42, LowPower: true}
	if laptop != want {
		t.Fatalf("%+v", laptop)
	}
	charging := powerStatusFrom("AC Power", []powerSource{{internal: true, present: true, current: 120, max: 100, charging: true}}, false)
	if charging.Battery != 1 || !charging.Charging || charging.Source != platform.PowerSourceAC {
		t.Fatalf("%+v", charging)
	}
	if u := powerStatusFrom("", []powerSource{{internal: true, present: false}}, false); u.Source != platform.PowerSourceUnknown || u.HasBattery {
		t.Fatalf("%+v", u)
	}
}

func TestNetworkStatusFrom(t *testing.T) {
	for _, c := range []struct {
		p    netPath
		want platform.NetworkStatus
	}{
		{netPath{status: nwPathStatusSatisfied, wifi: true, expensive: true}, platform.NetworkStatus{Online: true, Kind: platform.NetworkWiFi, Metered: true}},
		{netPath{status: nwPathStatusSatisfied, wired: true, wifi: true}, platform.NetworkStatus{Online: true, Kind: platform.NetworkWired}},
		{netPath{status: nwPathStatusSatisfied, cellular: true, constrained: true}, platform.NetworkStatus{Online: true, Kind: platform.NetworkCellular, Constrained: true}},
		{netPath{status: nwPathStatusSatisfied, other: true}, platform.NetworkStatus{Online: true, Kind: platform.NetworkOther}},
		{netPath{status: nwPathStatusUnsatisfied, expensive: true}, platform.NetworkStatus{Kind: platform.NetworkNone}},
		{netPath{status: nwPathStatusSatisfiable}, platform.NetworkStatus{Kind: platform.NetworkNone}},
		{netPath{status: nwPathStatusInvalid}, platform.NetworkStatus{}},
	} {
		if got := networkStatusFrom(c.p); got != c.want {
			t.Errorf("%+v: %+v, want %+v", c.p, got, c.want)
		}
	}
}

func TestDragOperationFrom(t *testing.T) {
	for op, want := range map[uint]platform.DragOperation{
		0: platform.DragNone, nsDragOpCopy: platform.DragCopy, nsDragOpGeneric: platform.DragCopy,
		nsDragOpLink: platform.DragLink, nsDragOpMove | nsDragOpCopy: platform.DragMove,
	} {
		if got := dragOperationFrom(op); got != want {
			t.Errorf("%d: %d", op, got)
		}
	}
}
