//go:build tango_dev || tango_selftest

package tango

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/tangoui/tango/internal/lane"
)

// A faulted platform upstream lane makes OnLanePacket report the refusal
// (the backend then tells the page, which falls back to messages).
func TestOnLanePacketFaulted(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", lane.UpAndroid+":1")
	a, fb := laneApp(t, Options{})
	a.OnBinary(func(*Surface, uint64, []byte) {})
	s := laneSurface(t, a, fb, "", lane.UpAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	// One frame: stream 1, seq 1, payload "x".
	data := binary.BigEndian.AppendUint16(nil, 1)
	data = binary.BigEndian.AppendUint64(data, 1)
	data = binary.BigEndian.AppendUint64(data, 1)
	data = binary.BigEndian.AppendUint32(data, 1)
	data = append(data, 'x')
	if !fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte{0, 0}) {
		t.Fatal("empty probe refused")
	}
	if !fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, data) {
		t.Fatal("first data packet refused before the fault")
	}
	if fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, data) {
		t.Fatal("second data packet reported delivered after the fault")
	}
}
