//go:build tango_dev

package lane

import (
	"net/http"
	"testing"
)

func TestFaultAfterN(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", DownAndroid+":2")
	rec := &recorder{}
	s := New(rec.send)
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
	s := New(rec.send)
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
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
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
	h = parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
	if code := post(t, ls, h.Gen, h.Tok, []byte{0, 0}); code != http.StatusNoContent {
		t.Fatalf("probe in the next generation: %d, want 204", code)
	}
}

func TestFaultSchemeEndsDownlink(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", DownScheme+":0")
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	s.mu.Lock()
	q := s.down
	s.mu.Unlock()
	if s.Send(mkPacket([]byte("x"))) {
		t.Fatal("packet accepted on a lane faulted at 0")
	}
	select {
	case <-q.done:
	default:
		t.Fatal("faulted down:scheme downlink not ended")
	}
}

func TestFaultUpLane(t *testing.T) {
	t.Setenv("TANGO_IPC_LANE_FAULT", UpPost+":3")
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
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
		{"down:scheme:0", DownScheme, 0, true},
		{"up:shm:17", UpShm, 17, true},
		{" down:android:2 ", DownAndroid, 2, true},
		{"down:scheme", "", 0, false},
		{"down:scheme:x", "", 0, false},
		{"down:scheme:-1", "", 0, false},
		{"down:nope:3", "", 0, false},
	} {
		name, n, ok := parseFault(c.env)
		if name != c.name || n != c.n || ok != c.ok {
			t.Errorf("parseFault(%q) = %q %d %v, want %q %d %v", c.env, name, n, ok, c.name, c.n, c.ok)
		}
	}
}
