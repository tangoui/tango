//go:build tango_dev || tango_selftest

package downq

import (
	"context"
	"testing"
)

// TANGO_IPC_LANE_FAULT=down:poll:N cuts off the response carrying the
// generation's N+1th byte packet in the middle of that item; the items
// stay unacknowledged, so Fail sends them all to fallback. Only once per
// generation; the next generation counts again.
func TestFaultCutsResponse(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", "down:poll:2")
	s, rec := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	s.Push(Packet, []byte("p1"))
	s.Push(Msg, []byte("m2"))
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 2)
	s.Push(Packet, []byte("p3"))
	s.Push(Packet, []byte("p4-cut-here"))
	s.Push(Msg, []byte("m5"))
	w := poll(context.Background(), s, gen, tok, 2)
	if w.Code != 200 {
		t.Fatalf("cut response status %d", w.Code)
	}
	if _, _, err := DecodeBatch(w.Body.Bytes()); err == nil {
		t.Fatal("cut response decodes")
	}
	full := EncodeBatch(3, []Item{{Kind: Packet, Data: []byte("p3")}, {Kind: Packet, Data: []byte("p4-cut-here")}, {Kind: Msg, Data: []byte("m5")}})
	if got := w.Body.Len(); got >= len(full)-len("m5")-itemHeader || got <= batchHeader+itemHeader+2 {
		t.Fatalf("cut response is %d bytes of %d: not cut inside the second item", got, len(full))
	}
	s.Fail(gen, 2)
	if got := rec.seqs(); len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Fatalf("fallback got %v, want 3 4 5", got)
	}

	// A new generation counts from zero again and cuts once.
	gen, tok = s.Start(true)
	activate(t, s, gen, tok)
	s.Push(Packet, []byte("a"))
	s.Push(Packet, []byte("b"))
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 2)
	s.Push(Packet, []byte("c"))
	if _, _, err := DecodeBatch(poll(context.Background(), s, gen, tok, 2).Body.Bytes()); err == nil {
		t.Fatal("third packet of the next generation not cut")
	}
	// The cut item is sent again in full on the next poll (no Fail).
	wantBatch(t, poll(context.Background(), s, gen, tok, 2), 3, 1)
}

// Other lanes' faults are internal/lane's: downq ignores them.
func TestFaultOtherLane(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", "up:post:0")
	s, _ := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	s.Push(Packet, []byte("p1"))
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 1)
}
