package downq

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const waitFor = 2 * time.Second

// recorder collects the items handed to fallback, one slice per call.
type recorder struct {
	mu    sync.Mutex
	calls [][]Item
}

func (r *recorder) fallback(items []Item) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]Item(nil), items...))
}

// seqs returns the sequence numbers of every item handed over so far.
func (r *recorder) seqs() []uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []uint64
	for _, c := range r.calls {
		for _, it := range c {
			out = append(out, it.Seq)
		}
	}
	return out
}

// setVar sets *p to v for the rest of the test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// newState returns a State whose fallback records into the returned
// recorder, with HoldMax and WatchdogAfter long enough not to fire unless
// the test shrinks them. It is closed when the test ends.
func newState(t *testing.T) (*State, *recorder) {
	t.Helper()
	setVar(t, &HoldMax, time.Minute)
	setVar(t, &WatchdogAfter, time.Minute)
	rec := &recorder{}
	s := New(rec.fallback)
	t.Cleanup(s.Close)
	return s, rec
}

// pollURL is the poll request path for gen, tok and ack.
func pollURL(gen uint64, tok string, ack uint64) string {
	return fmt.Sprintf("/__tango/lane/poll?gen=%d&tok=%s&ack=%d", gen, tok, ack)
}

// poll serves one poll request with ctx and returns the recorded response.
func poll(ctx context.Context, s *State, gen uint64, tok string, ack uint64) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, pollURL(gen, tok, ack), nil)
	w := httptest.NewRecorder()
	s.Serve(w, r)
	return w
}

// batch decodes a 200 response and fails the test otherwise.
func batch(t *testing.T, w *httptest.ResponseRecorder) (uint64, []Item) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", w.Code, w.Body.String())
	}
	h := w.Header()
	if h.Get("Content-Type") != "application/octet-stream" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", h)
	}
	first, items, err := DecodeBatch(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return first, items
}

// wantBatch checks that w holds count items starting at first.
func wantBatch(t *testing.T, w *httptest.ResponseRecorder, first uint64, count int) []Item {
	t.Helper()
	f, items := batch(t, w)
	if len(items) != count || (count > 0 && f != first) {
		t.Fatalf("batch = firstSeq %d count %d, want firstSeq %d count %d", f, len(items), first, count)
	}
	for i, it := range items {
		if it.Seq != first+uint64(i) {
			t.Fatalf("item %d seq = %d", i, it.Seq)
		}
	}
	return items
}

// activate makes the first poll of the generation, which must come back
// empty (HoldMax is shrunk for it).
func activate(t *testing.T, s *State, gen uint64, tok string) {
	t.Helper()
	old := HoldMax
	HoldMax = time.Millisecond
	w := poll(context.Background(), s, gen, tok, 0)
	HoldMax = old
	wantBatch(t, w, 0, 0)
	if !s.Active() {
		t.Fatal("not active after the first poll")
	}
}

// waitHeld waits until n poll requests are in progress.
func waitHeld(t *testing.T, s *State, n int) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for {
		s.mu.Lock()
		got := s.reqs
		s.mu.Unlock()
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("held requests = %d, want %d", got, n)
		}
		time.Sleep(time.Millisecond)
	}
}

// goPoll serves a poll request on its own goroutine.
func goPoll(s *State, gen uint64, tok string, ack uint64) <-chan *httptest.ResponseRecorder {
	ch := make(chan *httptest.ResponseRecorder, 1)
	go func() { ch <- poll(context.Background(), s, gen, tok, ack) }()
	return ch
}

// await returns the response of a goPoll request.
func await(t *testing.T, ch <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case w := <-ch:
		return w
	case <-time.After(waitFor):
		t.Fatal("poll request did not end")
		return nil
	}
}

func TestPollTakesQueued(t *testing.T) {
	s, rec := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	if seq := s.Push(Msg, []byte("a")); seq != 1 {
		t.Fatalf("seq = %d", seq)
	}
	s.Push(Packet, []byte{1, 2})
	items := wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 2)
	if items[0].Kind != Msg || string(items[0].Data) != "a" || items[1].Kind != Packet || !bytes.Equal(items[1].Data, []byte{1, 2}) {
		t.Fatalf("items = %+v", items)
	}
	if got := rec.seqs(); len(got) != 0 {
		t.Fatalf("fallback got %v", got)
	}
}

func TestPollHoldsUntilPush(t *testing.T) {
	s, _ := newState(t)
	gen, tok := s.Start(true)
	ch := goPoll(s, gen, tok, 0)
	waitHeld(t, s, 1)
	time.Sleep(50 * time.Millisecond)
	select {
	case w := <-ch:
		t.Fatalf("poll ended before Push: %d", w.Code)
	default:
	}
	pushed := time.Now()
	s.Push(Msg, []byte("x"))
	w := await(t, ch)
	if d := time.Since(pushed); d > 20*time.Millisecond {
		t.Fatalf("response ended %v after Push", d)
	}
	items := wantBatch(t, w, 1, 1)
	if string(items[0].Data) != "x" {
		t.Fatalf("data = %q", items[0].Data)
	}
}

func TestPollEmptyAfterHoldMax(t *testing.T) {
	s, _ := newState(t)
	setVar(t, &HoldMax, 100*time.Millisecond)
	gen, tok := s.Start(true)
	start := time.Now()
	w := poll(context.Background(), s, gen, tok, 0)
	d := time.Since(start)
	wantBatch(t, w, 0, 0)
	if d < 90*time.Millisecond || d > time.Second {
		t.Fatalf("held %v, want about 100ms", d)
	}
}

func TestBatchLimits(t *testing.T) {
	s, _ := newState(t)
	setVar(t, &BatchItems, 3)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	for range 5 {
		s.Push(Msg, []byte("m"))
	}
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 3)
	wantBatch(t, poll(context.Background(), s, gen, tok, 3), 4, 2)
}

func TestOversizeItemAlone(t *testing.T) {
	s, _ := newState(t)
	setVar(t, &BatchBytes, 1024)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	s.Push(Packet, make([]byte, 4096))
	s.Push(Msg, []byte("small"))
	items := wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 1)
	if len(items[0].Data) != 4096 {
		t.Fatalf("first batch item has %d bytes", len(items[0].Data))
	}
	items = wantBatch(t, poll(context.Background(), s, gen, tok, 1), 2, 1)
	if string(items[0].Data) != "small" {
		t.Fatalf("second batch item = %q", items[0].Data)
	}
}

func TestAckDropsAndUnackedKept(t *testing.T) {
	s, _ := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	for i := range 3 {
		s.Push(Msg, []byte{byte('a' + i)})
	}
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 3)
	s.Push(Msg, []byte("d"))
	items := wantBatch(t, poll(context.Background(), s, gen, tok, 2), 3, 2)
	if string(items[0].Data) != "c" || string(items[1].Data) != "d" {
		t.Fatalf("items = %q %q", items[0].Data, items[1].Data)
	}
	// Everything acknowledged: the next poll holds, then comes back empty.
	setVar(t, &HoldMax, 10*time.Millisecond)
	wantBatch(t, poll(context.Background(), s, gen, tok, 4), 0, 0)
	s.mu.Lock()
	n := len(s.unacked) + len(s.queue)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d items kept after the last ack", n)
	}
}

func TestBadTokenForbidden(t *testing.T) {
	s, rec := newState(t)
	gen, tok := s.Start(true)
	bad := []string{
		pollURL(gen, tok+"0", 0),
		pollURL(gen+1, tok, 0),
		pollURL(gen-1, tok, 0),
		fmt.Sprintf("/__tango/lane/poll?gen=%d&ack=0", gen),
		fmt.Sprintf("/__tango/lane/poll?tok=%s&ack=0", tok),
	}
	check := func() {
		t.Helper()
		for _, u := range bad {
			w := httptest.NewRecorder()
			s.Serve(w, httptest.NewRequest(http.MethodGet, u, nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s: status = %d, want 403", u, w.Code)
			}
		}
	}
	check()
	if s.Active() {
		t.Fatal("a refused poll activated the generation")
	}
	s.Push(Msg, []byte("a"))
	if got := rec.seqs(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("fallback got %v", got)
	}

	activate(t, s, gen, tok)
	s.Push(Msg, []byte("b"))
	s.Push(Msg, []byte("c"))
	wantBatch(t, poll(context.Background(), s, gen, tok, 1), 2, 2)
	bad = append(bad, pollURL(gen, tok+"0", 3))
	check() // must not acknowledge 2 and 3
	wantBatch(t, poll(context.Background(), s, gen, tok, 1), 2, 2)

	w := httptest.NewRecorder()
	s.Serve(w, httptest.NewRequest(http.MethodPost, pollURL(gen, tok, 0), nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: status = %d, want 405", w.Code)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := []Item{
		{Kind: Msg, Data: []byte("héllo")},
		{Kind: Packet, Data: []byte{0, 1, 0xff}},
		{Kind: Barrier, Data: []byte{0, 0, 0, 0, 0, 0, 0, 7}},
		{Kind: Msg, Data: []byte{}},
	}
	b := EncodeBatch(41, in)
	if len(b) != 12+4*5+len("héllo")+3+8 {
		t.Fatalf("encoded %d bytes", len(b))
	}
	first, out, err := DecodeBatch(b)
	if err != nil {
		t.Fatal(err)
	}
	if first != 41 || len(out) != len(in) {
		t.Fatalf("decoded firstSeq %d count %d", first, len(out))
	}
	for i := range in {
		if out[i].Seq != 41+uint64(i) || out[i].Kind != in[i].Kind || !bytes.Equal(out[i].Data, in[i].Data) {
			t.Fatalf("item %d = %+v, want %+v", i, out[i], in[i])
		}
	}

	first, out, err = DecodeBatch(EncodeBatch(9, nil))
	if err != nil || first != 9 || len(out) != 0 {
		t.Fatalf("empty batch: %d %v %v", first, out, err)
	}

	for _, bad := range [][]byte{
		nil,
		b[:11],
		b[:12+4],                          // item header cut short
		b[:len(b)-1],                      // payload cut short
		append(b[:len(b):len(b)], 0),      // trailing byte
		EncodeBatch(1, []Item{{Kind: 3}}), // unknown kind
	} {
		if _, _, err := DecodeBatch(bad); err == nil {
			t.Fatalf("DecodeBatch(%x) succeeded", bad)
		}
	}
}
