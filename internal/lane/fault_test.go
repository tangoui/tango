//go:build tango_dev || tango_selftest

package lane

import (
	"net/http"
	"testing"
)

func TestFaultAfterN(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", DownAndroid+":2")
	rec := &recorder{}
	s := New(rec.send, nil)
	h := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	data := mkPacket([]byte("x"))
	for i := 0; i < 2; i++ {
		if !s.Send(data) {
			t.Fatalf("packet %d refused before the fault", i+1)
		}
	}
	if s.Send(data) || s.Send(data) || rec.n() != 2 {
		t.Fatal("packets after the fault not refused")
	}
	if s.MinSize() != -1 {
		t.Fatal("faulted lane not marked failed")
	}
	// The count starts over with the next generation.
	h = parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if !s.Send(data) {
		t.Fatal("fault carried into the next generation")
	}
}

// Empty packets (count 0) are not counted on a downstream lane: N means N
// data packets.
func TestFaultSkipsEmptyDownPackets(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", DownAndroid+":2")
	rec := &recorder{}
	s := New(rec.send, nil)
	h := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	data := mkPacket([]byte("x"))
	for i, p := range [][]byte{{0, 0}, data, {0, 0}, {0, 0}, data, {0, 0}} {
		if !s.Send(p) {
			t.Fatalf("packet %d (%d bytes) refused before the fault", i+1, len(p))
		}
	}
	if s.Send(data) {
		t.Fatal("third data packet accepted")
	}
	if s.Send([]byte{0, 0}) {
		t.Fatal("empty packet accepted on a faulted lane")
	}
	if rec.n() != 6 {
		t.Fatalf("platform got %d packets, want 6", rec.n())
	}
}

// The up:post start probe is the empty packet [0,0]: it must not advance
// the fault count (TANGO_IPC_LANE_FAULT=up:post:N lets N data packets
// through), but it is still refused once the lane has faulted, so a probe
// on a broken lane fails.
func TestFaultSkipsEmptyUpPackets(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", UpPost+":2")
	s := New(nil, nil)
	h := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	ls := newLaneServer(t, s)
	data := mkPacket([]byte("data"))
	// The probe comes before the page reports up:post ready.
	for i := 0; i < 3; i++ {
		if code := post(t, ls, h.Gen, h.Tok, []byte{0, 0}); code != http.StatusNoContent {
			t.Fatalf("start probe %d: %d, want 204", i+1, code)
		}
	}
	s.Control(ctl("ready", UpPost, h.Gen))
	for i := 0; i < 2; i++ {
		if code := post(t, ls, h.Gen, h.Tok, data); code != http.StatusNoContent {
			t.Fatalf("data packet %d: %d, want 204", i+1, code)
		}
		if code := post(t, ls, h.Gen, h.Tok, []byte{0, 0}); code != http.StatusNoContent {
			t.Fatalf("empty packet after data %d: %d, want 204", i+1, code)
		}
	}
	if code := post(t, ls, h.Gen, h.Tok, data); code == http.StatusNoContent {
		t.Fatal("third data packet accepted")
	}
	if code := post(t, ls, h.Gen, h.Tok, []byte{0, 0}); code == http.StatusNoContent {
		t.Fatal("start probe accepted on a faulted lane")
	}
	if n := len(ls.delivered()); n != 7 {
		t.Fatalf("delivered %d packets, want 7 (5 empty, 2 data)", n)
	}
	// A new generation starts counting over; its probe succeeds.
	h = parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	if code := post(t, ls, h.Gen, h.Tok, []byte{0, 0}); code != http.StatusNoContent {
		t.Fatalf("probe in the next generation: %d, want 204", code)
	}
}

func TestFaultUpLane(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", UpPost+":3")
	s := New(nil, nil)
	h := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	s.Control(ctl("ready", UpPost, h.Gen))
	for i := 0; i < 3; i++ {
		if !s.UpAllowed() {
			t.Fatalf("up packet %d refused before the fault", i+1)
		}
	}
	if s.UpAllowed() || s.UpAllowed() {
		t.Fatal("up packets after the fault allowed")
	}
}

func TestFaultParse(t *testing.T) {
	for _, c := range []struct {
		env  string
		name string
		n    int
		ok   bool
	}{
		{"", "", 0, false},
		{"down:poll:0", DownPoll, 0, true},
		{"down:poll:2", DownPoll, 2, true},
		{"down:scheme:0", "", 0, false}, // removed lane
		{"up:shm:17", UpShm, 17, true},
		{" down:android:2 ", DownAndroid, 2, true},
		{"down:poll", "", 0, false},
		{"down:poll:x", "", 0, false},
		{"down:poll:-1", "", 0, false},
		{"down:nope:3", "", 0, false},
	} {
		name, n, ok := parseFault(c.env)
		if name != c.name || n != c.n || ok != c.ok {
			t.Errorf("parseFault(%q) = %q %d %v, want %q %d %v", c.env, name, n, ok, c.name, c.n, c.ok)
		}
	}
}

// TANGO_IPC_LANE_FAULT=down:poll:N is internal/downq's (it cuts a poll
// response off): lane.State keeps handing every packet to pollSend.
func TestDownPollFaultIsDownqs(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", "down:poll:2")
	poll := &recorder{}
	s := New(nil, poll.send)
	h := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	s.Control(ctl("ready", DownPoll, h.Gen))
	data := mkPacket([]byte("x"))
	for i := 0; i < 5; i++ {
		if !s.Send(data) {
			t.Fatalf("packet %d refused: lane.State injected a down:poll fault", i+1)
		}
	}
	if poll.n() != 5 || s.MinSize() != 0 {
		t.Fatalf("pollSend got %d packets, MinSize %d", poll.n(), s.MinSize())
	}
}
