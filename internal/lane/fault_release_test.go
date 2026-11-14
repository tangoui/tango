//go:build !tango_dev && !tango_selftest

package lane

import "testing"

// In release builds TANGO_IPC_LANE_FAULT is ignored.
func TestFaultReleaseNoop(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", DownAndroid+":0")
	rec := &recorder{}
	s := New(rec.send)
	h := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	for i := 0; i < 5; i++ {
		if !s.Send([]byte{0, 0}) {
			t.Fatal("release build injected a fault")
		}
	}
}
