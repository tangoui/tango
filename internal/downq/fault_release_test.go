//go:build !tango_dev && !tango_selftest

package downq

import (
	"context"
	"testing"
)

// In release builds TANGO_IPC_LANE_FAULT is ignored: no response is cut.
func TestFaultReleaseNoop(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", "down:poll:0")
	s, _ := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	for i := uint64(1); i <= 3; i++ {
		s.Push(Packet, []byte{byte(i)})
		wantBatch(t, poll(context.Background(), s, gen, tok, i-1), i, 1)
	}
}
