package lane

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"testing"
)

type helloJSON struct {
	Gen  uint64   `json:"gen"`
	Tok  string   `json:"tok"`
	List []string `json:"list"`
}

func parseHello(t *testing.T, raw json.RawMessage) helloJSON {
	t.Helper()
	if raw == nil {
		t.Fatal("Hello returned nil")
	}
	var h helloJSON
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatalf("hello %s: %v", raw, err)
	}
	return h
}

// recorder is a platformSend that records what it was given.
type recorder struct {
	mu   sync.Mutex
	got  [][]byte
	gens []uint64 // generation of each packet in got
	fail bool
}

func (r *recorder) send(gen uint64, p []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return false
	}
	r.got = append(r.got, p)
	r.gens = append(r.gens, gen)
	return true
}

func (r *recorder) n() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.got) }

func (r *recorder) setFail(v bool) { r.mu.Lock(); r.fail = v; r.mu.Unlock() }

func ctl(op, lane string, gen uint64) string {
	return fmt.Sprintf(`{"op":%q,"lane":%q,"gen":%d}`, op, lane, gen)
}

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestHelloNewGenAndToken(t *testing.T) {
	s := New(nil, nil)
	h1 := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	if h1.Gen != 1 || !hex32.MatchString(h1.Tok) {
		t.Fatalf("first hello = %+v", h1)
	}
	if len(h1.List) != 2 || h1.List[0] != DownPoll || h1.List[1] != UpPost {
		t.Fatalf("list = %v", h1.List)
	}
	if !s.CheckToken(1, h1.Tok) {
		t.Fatal("current token refused")
	}
	h2 := parseHello(t, s.Hello([]string{DownPoll}, true))
	if h2.Gen != 2 || !hex32.MatchString(h2.Tok) || h2.Tok == h1.Tok {
		t.Fatalf("second hello = %+v (first %+v)", h2, h1)
	}
	for _, c := range []struct {
		gen uint64
		tok string
		ok  bool
	}{
		{2, h2.Tok, true},
		{1, h1.Tok, false},
		{2, h1.Tok, false},
		{1, h2.Tok, false},
		{3, h2.Tok, false},
		{2, "", false},
		{2, h2.Tok[:31], false},
	} {
		if got := s.CheckToken(c.gen, c.tok); got != c.ok {
			t.Errorf("CheckToken(%d, %q) = %v, want %v", c.gen, c.tok, got, c.ok)
		}
	}
	s.Close()
	if s.CheckToken(2, h2.Tok) {
		t.Fatal("token valid after Close")
	}
	if s.Hello([]string{DownPoll}, true) != nil {
		t.Fatal("Hello after Close returned lanes")
	}
}

func TestHelloNilWhenDisabledOrNoCaps(t *testing.T) {
	s := New(nil, nil)
	if raw := s.Hello([]string{DownPoll}, false); raw != nil {
		t.Fatalf("disabled: %s", raw)
	}
	if raw := s.Hello(nil, true); raw != nil {
		t.Fatalf("no caps: %s", raw)
	}
	if raw := s.Hello([]string{"down:carrier-pigeon"}, true); raw != nil {
		t.Fatalf("only unknown caps: %s", raw)
	}
	h := parseHello(t, s.Hello([]string{DownAndroid, "nope", DownAndroid}, true))
	if len(h.List) != 1 || h.List[0] != DownAndroid {
		t.Fatalf("list = %v, want unknown and duplicate names dropped", h.List)
	}
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != 0 {
		t.Fatal("lane not ready")
	}
	// A disabled hello still ends the previous generation.
	if s.Hello([]string{DownAndroid}, false) != nil {
		t.Fatal("disabled hello returned lanes")
	}
	if s.MinSize() != -1 || s.CheckToken(h.Gen, h.Tok) {
		t.Fatal("disabled hello kept the previous generation alive")
	}
}

func TestControlReadyFail(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send, nil)
	h := parseHello(t, s.Hello([]string{DownAndroid, UpAndroid}, true))
	if s.MinSize() != -1 || s.Send([]byte{0, 0}) {
		t.Fatal("lane usable before ready")
	}
	if !s.UpAllowed() {
		// The up lane is offered and not failed: its packets may race its
		// ready message.
		t.Fatal("up packet refused while its lane is pending")
	}
	s.Control(ctl("ready", DownAndroid, h.Gen+1)) // wrong gen
	s.Control(ctl("ready", DownShm, h.Gen))       // not offered
	s.Control(`{"op":"ready"`)                    // malformed
	s.Control(ctl("bogus", DownAndroid, h.Gen))   // unknown op
	if s.MinSize() != -1 {
		t.Fatal("ignored control message made a lane ready")
	}
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != 0 {
		t.Fatalf("MinSize = %d, want 0", s.MinSize())
	}
	p := []byte{0, 1, 2}
	if !s.Send(p) || rec.n() != 1 || &rec.got[0][0] != &p[0] {
		t.Fatal("Send did not hand the packet to platformSend")
	}
	rec.setFail(true)
	if s.Send(p) {
		t.Fatal("Send true although platformSend refused")
	}
	rec.setFail(false)
	s.Control(ctl("fail", DownAndroid, h.Gen))
	if s.MinSize() != -1 || s.Send(p) || rec.n() != 1 {
		t.Fatal("failed lane still used")
	}
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != -1 {
		t.Fatal("failed lane came back within its generation")
	}
	s.Control(ctl("ready", UpAndroid, h.Gen))
	if !s.UpAllowed() {
		t.Fatal("ready up lane refused")
	}
	s.Control(ctl("fail", UpAndroid, h.Gen))
	if s.UpAllowed() {
		t.Fatal("failed up lane allowed")
	}
	// A new generation starts over; the old generation's ready is ignored.
	h2 := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != -1 {
		t.Fatal("old generation's ready accepted")
	}
	if s.UpAllowed() {
		t.Fatal("up allowed without an up lane")
	}
	s.Control(ctl("ready", DownAndroid, h2.Gen))
	if s.MinSize() != 0 {
		t.Fatal("new generation's ready ignored")
	}
	s.Close()
	if s.MinSize() != -1 || s.Send(p) || s.UpAllowed() {
		t.Fatal("lanes usable after Close")
	}
}

func TestPlatformLanePreferredOverPoll(t *testing.T) {
	rec, poll := &recorder{}, &recorder{}
	s := New(rec.send, poll.send)
	h := parseHello(t, s.Hello([]string{DownPoll, DownShm}, true))
	s.Control(ctl("ready", DownPoll, h.Gen))
	s.Control(ctl("ready", DownShm, h.Gen))
	if s.MinSize() != 0 || !s.Send([]byte{0, 0}) || rec.n() != 1 || poll.n() != 0 {
		t.Fatal("platform lane not preferred")
	}
}

func TestNilPlatformSend(t *testing.T) {
	s := New(nil, nil)
	h := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.Send([]byte{0, 0}) {
		t.Fatal("Send true without a platform sender")
	}
}

func TestConcurrentUse(t *testing.T) {
	rec, poll := &recorder{}, &recorder{}
	s := New(rec.send, poll.send)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.Send([]byte{0, 0})
				s.MinSize()
				s.UpAllowed()
				s.CheckToken(1, "x")
			}
		}()
	}
	for j := 0; j < 50; j++ {
		raw := s.Hello([]string{DownAndroid, DownPoll}, true)
		var h helloJSON
		json.Unmarshal(raw, &h)
		s.Control(ctl("ready", DownAndroid, h.Gen))
		s.Control(ctl("ready", DownPoll, h.Gen))
		if j%2 == 0 {
			s.Control(ctl("fail", DownAndroid, h.Gen))
		}
	}
	wg.Wait()
	s.Close()
}

func TestReset(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send, nil)
	h := parseHello(t, s.Hello([]string{DownAndroid, DownPoll, UpAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	s.Control(ctl("ready", UpAndroid, h.Gen))
	s.Reset()
	if s.Send([]byte{0, 0}) || rec.n() != 0 || s.MinSize() != -1 || s.UpAllowed() || s.CheckToken(h.Gen, h.Tok) {
		t.Fatal("lanes usable after Reset")
	}
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != -1 {
		t.Fatal("old generation revived after Reset")
	}
	if parseHello(t, s.Hello([]string{DownAndroid}, true)).Gen != h.Gen+2 {
		t.Fatal("Hello after Reset did not start a newer generation")
	}
}

func TestDownPollOfferedAndReady(t *testing.T) {
	plat, poll := &recorder{}, &recorder{}
	s := New(plat.send, poll.send)
	h := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	if len(h.List) != 2 || h.List[0] != DownPoll || h.List[1] != UpPost {
		t.Fatalf("list = %v, want [down:poll up:post]", h.List)
	}
	if s.MinSize() != -1 || s.Send([]byte{0, 0}) || poll.n() != 0 {
		t.Fatal("down:poll usable before ready")
	}
	s.Control(ctl("ready", DownPoll, h.Gen))
	if s.MinSize() != 0 {
		t.Fatalf("MinSize = %d, want 0", s.MinSize())
	}
	p := mkPacket([]byte("x"))
	if !s.Send(p) || poll.n() != 1 || &poll.got[0][0] != &p[0] || plat.n() != 0 {
		t.Fatal("Send did not hand the packet to pollSend")
	}
	// Failing down:poll stops using it for the generation.
	s.Control(ctl("fail", DownPoll, h.Gen))
	if s.MinSize() != -1 || s.Send(p) || poll.n() != 1 {
		t.Fatal("failed down:poll still used")
	}
}

func TestDownPollRefusedFallsBack(t *testing.T) {
	poll := &recorder{}
	s := New(nil, poll.send)
	h := parseHello(t, s.Hello([]string{DownPoll}, true))
	s.Control(ctl("ready", DownPoll, h.Gen))
	poll.setFail(true)
	if s.Send(mkPacket([]byte("x"))) {
		t.Fatal("Send true although pollSend refused")
	}
	if s.MinSize() != 0 {
		t.Fatal("a refused packet failed down:poll")
	}
	poll.setFail(false)
	if !s.Send(mkPacket([]byte("y"))) || poll.n() != 1 {
		t.Fatal("down:poll unusable after a refused packet")
	}
	// Without a poll sender the lane takes nothing.
	s2 := New(nil, nil)
	h2 := parseHello(t, s2.Hello([]string{DownPoll}, true))
	s2.Control(ctl("ready", DownPoll, h2.Gen))
	if s2.Send([]byte{0, 0}) {
		t.Fatal("Send true without a poll sender")
	}
}

func TestDownSchemeGone(t *testing.T) {
	s := New(nil, nil)
	if raw := s.Hello([]string{"down:scheme"}, true); raw != nil {
		t.Fatalf("down:scheme still offered: %s", raw)
	}
	h := parseHello(t, s.Hello([]string{"down:scheme", UpPost}, true))
	if len(h.List) != 1 || h.List[0] != UpPost {
		t.Fatalf("list = %v, want [up:post]", h.List)
	}
}

func TestSuspendDown(t *testing.T) {
	rec, poll := &recorder{}, &recorder{}
	s := New(rec.send, poll.send)
	h := parseHello(t, s.Hello([]string{DownAndroid, DownPoll, UpAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	s.Control(ctl("ready", UpAndroid, h.Gen))
	s.SuspendDown()
	if s.Send([]byte{0, 0}) || rec.n() != 0 || s.MinSize() != -1 {
		t.Fatal("down lane usable while suspended")
	}
	if !s.UpAllowed() || !s.CheckToken(h.Gen, h.Tok) {
		t.Fatal("SuspendDown touched the uplink or the token")
	}
	// A late ready for this generation does not lift the suspension.
	s.Control(ctl("ready", DownPoll, h.Gen))
	if s.MinSize() != -1 || s.Send([]byte{0, 0}) || poll.n() != 0 {
		t.Fatal("late ready lifted the suspension")
	}
	// The next generation starts unsuspended.
	h = parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != 0 || !s.Send([]byte{0, 0}) || rec.n() != 1 {
		t.Fatal("suspension carried into the next generation")
	}
}

// SendGenerationOrdered reports ordered only for a packet down:poll took.
func TestSendGenerationOrdered(t *testing.T) {
	plat, poll := &recorder{}, &recorder{}
	s := New(plat.send, poll.send)
	h := parseHello(t, s.Hello([]string{DownPoll, DownShm}, true))
	if sent, _ := s.SendGenerationOrdered(h.Gen, mkPacket([]byte("x"))); sent {
		t.Fatal("sent before any lane is ready")
	}
	s.Control(ctl("ready", DownPoll, h.Gen))
	if sent, ordered := s.SendGenerationOrdered(h.Gen, mkPacket([]byte("x"))); !sent || !ordered || poll.n() != 1 {
		t.Fatalf("down:poll: sent %v ordered %v", sent, ordered)
	}
	if sent, _ := s.SendGenerationOrdered(h.Gen+1, mkPacket([]byte("x"))); sent {
		t.Fatal("sent for another generation")
	}
	poll.setFail(true)
	if sent, ordered := s.SendGenerationOrdered(h.Gen, mkPacket([]byte("x"))); sent || ordered {
		t.Fatalf("refused: sent %v ordered %v", sent, ordered)
	}
	poll.setFail(false)
	// A platform lane is preferred and is not ordered.
	s.Control(ctl("ready", DownShm, h.Gen))
	if sent, ordered := s.SendGenerationOrdered(h.Gen, mkPacket([]byte("x"))); !sent || ordered || plat.n() != 1 {
		t.Fatalf("down:shm: sent %v ordered %v", sent, ordered)
	}
	if sent, ordered := s.SendOrdered(mkPacket([]byte("x"))); !sent || ordered {
		t.Fatalf("SendOrdered down:shm: sent %v ordered %v", sent, ordered)
	}
}

// A platform upstream packet carries the generation it was sent in; the
// core checks it against the current one atomically with the lane's
// usability, so a delayed packet of an old generation never counts for a
// new one.
func TestAllowUpGenRejectsOldGeneration(t *testing.T) {
	s := New(nil, nil)
	pkt := mkPacket([]byte("x"))
	g1 := parseHello(t, s.Hello([]string{UpAndroid}, true)).Gen
	if !s.AllowUpGen(g1, pkt) {
		t.Fatal("current generation's packet refused")
	}
	g2 := parseHello(t, s.Hello([]string{UpAndroid}, true)).Gen
	if g2 == g1 {
		t.Fatal("second hello kept the generation")
	}
	if s.AllowUpGen(g1, pkt) {
		t.Fatal("old generation's packet allowed")
	}
	if !s.AllowUpGen(g2, pkt) {
		t.Fatal("new generation's packet refused")
	}
	if s.AllowUpGen(g2+1, pkt) {
		t.Fatal("future generation's packet allowed")
	}
	s.Reset()
	if s.AllowUpGen(g1, pkt) || s.AllowUpGen(g2, pkt) || s.AllowUpGen(g2+1, pkt) {
		t.Fatal("packet allowed after Reset")
	}
}

// Empty packets (count 0, such as the up:android probe) are checked but
// not counted for fault injection; once the lane has faulted they are
// refused too. The fault fields are set directly so the test runs in every
// build.
func TestAllowUpGenSkipsEmptyForFault(t *testing.T) {
	s := New(nil, nil)
	s.faultLane, s.faultN = UpAndroid, 2 // as TANGO_IPC_LANE_FAULT=up:android:2
	g := parseHello(t, s.Hello([]string{UpAndroid}, true)).Gen
	empty := []byte{0, 0}
	data := mkPacket([]byte("x"))
	for i := 0; i < 3; i++ {
		if !s.AllowUpGen(g, empty) {
			t.Fatalf("empty packet %d refused", i+1)
		}
	}
	for i := 0; i < 2; i++ {
		if !s.AllowUpGen(g, data) {
			t.Fatalf("data packet %d refused before the fault (empty packets counted?)", i+1)
		}
	}
	if s.AllowUpGen(g, data) {
		t.Fatal("third data packet allowed after the fault")
	}
	if s.AllowUpGen(g, empty) {
		t.Fatal("empty packet allowed on a faulted lane")
	}
}

// The platform downstream lane is handed each packet with the generation
// it was admitted in; after a new hello the old lane is not ready and the
// backend is not called.
func TestSendLanePacketCarriesGeneration(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send, nil)
	g1 := parseHello(t, s.Hello([]string{DownAndroid}, true)).Gen
	s.Control(ctl("ready", DownAndroid, g1))
	pkt := mkPacket([]byte("x"))
	if !s.Send(pkt) {
		t.Fatal("Send refused on a ready lane")
	}
	rec.mu.Lock()
	if len(rec.got) != 1 || &rec.got[0][0] != &pkt[0] || rec.gens[0] != g1 {
		gens := rec.gens
		rec.mu.Unlock()
		t.Fatalf("platformSend got gens %v, want one packet in generation %d", gens, g1)
	}
	rec.mu.Unlock()
	parseHello(t, s.Hello([]string{DownAndroid}, true))
	if s.Send(pkt) {
		t.Fatal("Send true before the new generation's lane is ready")
	}
	if rec.n() != 1 {
		t.Fatal("platformSend called after a new hello")
	}
}

// down:poll's pollSend gets the generation the packet was admitted in, as
// platformSend does, and DownCurrent tells whether that generation may
// still deliver: not after a new hello, SuspendDown, Reset or Close.
func TestPollSendCarriesGeneration(t *testing.T) {
	poll := &recorder{}
	s := New(nil, poll.send)
	g1 := parseHello(t, s.Hello([]string{DownPoll}, true)).Gen
	s.Control(ctl("ready", DownPoll, g1))
	if sent, ordered := s.SendOrdered(mkPacket([]byte("x"))); !sent || !ordered {
		t.Fatalf("SendOrdered on down:poll: sent %v ordered %v", sent, ordered)
	}
	poll.mu.Lock()
	gens := append([]uint64(nil), poll.gens...)
	poll.mu.Unlock()
	if len(gens) != 1 || gens[0] != g1 {
		t.Fatalf("pollSend got gens %v, want [%d]", gens, g1)
	}
	if !s.DownCurrent(g1) || s.DownCurrent(g1+1) {
		t.Fatal("DownCurrent wrong for the current generation")
	}
	s.SuspendDown()
	if s.DownCurrent(g1) {
		t.Fatal("DownCurrent true after SuspendDown")
	}
	g2 := parseHello(t, s.Hello([]string{DownPoll}, true)).Gen
	if s.DownCurrent(g1) || !s.DownCurrent(g2) {
		t.Fatal("DownCurrent wrong after a new hello")
	}
	s.Reset()
	if s.DownCurrent(g2) {
		t.Fatal("DownCurrent true for a generation Reset ended")
	}
	g3 := s.Generation()
	s.Close()
	if s.DownCurrent(g3) {
		t.Fatal("DownCurrent true after Close")
	}
}
