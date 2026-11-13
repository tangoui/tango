package android

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestDecodeNetwork(t *testing.T) {
	st, err := decodeNetwork(`{"online":true,"kind":4,"metered":true,"constrained":false}`)
	if err != nil || st != (platform.NetworkStatus{Online: true, Kind: platform.NetworkCellular, Metered: true}) {
		t.Fatalf("%+v %v", st, err)
	}
	st, err = decodeNetwork(`{"online":false,"kind":1,"metered":false,"constrained":false}`)
	if err != nil || st.Online || st.Kind != platform.NetworkNone {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := decodeNetwork("!"); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestDecodePower(t *testing.T) {
	for in, want := range map[string]platform.PowerStatus{
		`{"present":true,"plugged":false,"charging":false,"level":0.8,"lowPower":true}`: {
			Source: platform.PowerSourceBattery, HasBattery: true, Battery: 0.8, LowPower: true},
		`{"present":true,"plugged":true,"charging":true,"level":0.5,"lowPower":false}`: {
			Source: platform.PowerSourceAC, HasBattery: true, Battery: 0.5, Charging: true},
		// A device without a battery (a TV box) on mains power.
		`{"present":false,"plugged":true,"charging":false,"level":-1,"lowPower":false}`: {
			Source: platform.PowerSourceAC, Battery: -1},
	} {
		got, err := decodePower(in)
		if err != nil || got != want {
			t.Errorf("decodePower(%s) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	if st, err := decodePower("x"); err == nil || st.Battery != -1 {
		t.Fatalf("%+v %v", st, err)
	}
}
