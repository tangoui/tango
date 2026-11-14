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
	fail bool
}

func (r *recorder) send(p []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return false
	}
	r.got = append(r.got, p)
	return true
}

func (r *recorder) n() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.got) }

func (r *recorder) setFail(v bool) { r.mu.Lock(); r.fail = v; r.mu.Unlock() }

func ctl(op, lane string, gen uint64) string {
	return fmt.Sprintf(`{"op":%q,"lane":%q,"gen":%d}`, op, lane, gen)
}

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestHelloNewGenAndToken(t *testing.T) {
	s := New(nil)
	h1 := parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
	if h1.Gen != 1 || !hex32.MatchString(h1.Tok) {
		t.Fatalf("first hello = %+v", h1)
	}
	if len(h1.List) != 2 || h1.List[0] != DownScheme || h1.List[1] != UpPost {
		t.Fatalf("list = %v", h1.List)
	}
	if !s.CheckToken(1, h1.Tok) {
		t.Fatal("current token refused")
	}
	h2 := parseHello(t, s.Hello([]string{DownScheme}, true))
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
	if s.Hello([]string{DownScheme}, true) != nil {
		t.Fatal("Hello after Close returned lanes")
	}
}

func TestHelloNilWhenDisabledOrNoCaps(t *testing.T) {
	s := New(nil)
	if raw := s.Hello([]string{DownScheme}, false); raw != nil {
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
	s := New(rec.send)
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

func TestSchemeMinSize(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send)
	h := parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	if s.MinSize() != 4096 {
		t.Fatalf("MinSize = %d, want 4096", s.MinSize())
	}
	if !s.Send([]byte{0, 0}) || rec.n() != 0 {
		t.Fatal("down:scheme packet not queued (or sent to the platform)")
	}
	s.mu.Lock()
	q := s.down
	queued := len(q.pkts)
	s.mu.Unlock()
	if queued != 1 {
		t.Fatalf("queued = %d", queued)
	}
	select {
	case <-q.wake:
	default:
		t.Fatal("queue not woken by Send")
	}
	// A new generation ends the old downlink and drops its queue.
	s.Hello([]string{DownScheme}, true)
	select {
	case <-q.done:
	default:
		t.Fatal("old downlink not ended by Hello")
	}
	s.mu.Lock()
	if len(s.down.pkts) != 0 || s.down == q {
		t.Error("old queue carried into the new generation")
	}
	s.mu.Unlock()
}

func TestSchemeQueueLimit(t *testing.T) {
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	big := make([]byte, 1<<20)
	for i := 0; i < maxDownQueue/len(big); i++ {
		if !s.Send(big) {
			t.Fatalf("packet %d refused below the limit", i)
		}
	}
	if s.Send([]byte{0, 0, 1}) {
		t.Fatal("packet accepted over the queue limit")
	}
	if s.MinSize() != 4096 {
		t.Fatal("a full queue failed the lane")
	}
	// Failing down:scheme ends its downlink.
	s.mu.Lock()
	q := s.down
	s.mu.Unlock()
	s.Control(ctl("fail", DownScheme, h.Gen))
	select {
	case <-q.done:
	default:
		t.Fatal("downlink not ended by fail")
	}
	if s.Send([]byte{0, 0}) {
		t.Fatal("failed down:scheme still queues")
	}
}

func TestPlatformLanePreferredOverScheme(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send)
	h := parseHello(t, s.Hello([]string{DownScheme, DownShm}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	s.Control(ctl("ready", DownShm, h.Gen))
	if s.MinSize() != 0 || !s.Send([]byte{0, 0}) || rec.n() != 1 {
		t.Fatal("platform lane not preferred")
	}
}

func TestNilPlatformSend(t *testing.T) {
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.Send([]byte{0, 0}) {
		t.Fatal("Send true without a platform sender")
	}
}

func TestConcurrentUse(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send)
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
		raw := s.Hello([]string{DownAndroid, DownScheme}, true)
		var h helloJSON
		json.Unmarshal(raw, &h)
		s.Control(ctl("ready", DownAndroid, h.Gen))
	}
	wg.Wait()
	s.Close()
}

func TestReset(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send)
	h := parseHello(t, s.Hello([]string{DownAndroid, DownScheme, UpAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	s.Control(ctl("ready", UpAndroid, h.Gen))
	s.mu.Lock()
	q := s.down
	s.mu.Unlock()
	s.Reset()
	if s.Send([]byte{0, 0}) || rec.n() != 0 || s.MinSize() != -1 || s.UpAllowed() || s.CheckToken(h.Gen, h.Tok) {
		t.Fatal("lanes usable after Reset")
	}
	select {
	case <-q.done:
	default:
		t.Fatal("down:scheme queue not ended by Reset")
	}
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != -1 {
		t.Fatal("old generation revived after Reset")
	}
	if parseHello(t, s.Hello([]string{DownAndroid}, true)).Gen != h.Gen+2 {
		t.Fatal("Hello after Reset did not start a newer generation")
	}
}

func TestSuspendDown(t *testing.T) {
	rec := &recorder{}
	s := New(rec.send)
	h := parseHello(t, s.Hello([]string{DownAndroid, DownScheme, UpAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	s.Control(ctl("ready", UpAndroid, h.Gen))
	s.mu.Lock()
	q := s.down
	s.mu.Unlock()
	s.SuspendDown()
	if s.Send([]byte{0, 0}) || rec.n() != 0 || s.MinSize() != -1 {
		t.Fatal("down lane usable while suspended")
	}
	select {
	case <-q.done:
	default:
		t.Fatal("down:scheme response not ended by SuspendDown")
	}
	if !s.UpAllowed() || !s.CheckToken(h.Gen, h.Tok) {
		t.Fatal("SuspendDown touched the uplink or the token")
	}
	// A late ready for this generation does not lift the suspension.
	s.Control(ctl("ready", DownScheme, h.Gen))
	if s.MinSize() != -1 || s.Send([]byte{0, 0}) {
		t.Fatal("late ready lifted the suspension")
	}
	// The next generation starts unsuspended.
	h = parseHello(t, s.Hello([]string{DownAndroid}, true))
	s.Control(ctl("ready", DownAndroid, h.Gen))
	if s.MinSize() != 0 || !s.Send([]byte{0, 0}) || rec.n() != 1 {
		t.Fatal("suspension carried into the next generation")
	}
}

func TestQueued(t *testing.T) {
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	s.Send(make([]byte, 10))
	s.Send(make([]byte, 5))
	if n, b := s.Queued(); n != 2 || b != 15 {
		t.Fatalf("Queued = %d, %d", n, b)
	}
}
