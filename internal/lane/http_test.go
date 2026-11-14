package lane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const waitFor = 2 * time.Second

// mkPacket builds a packet of one frame per payload (stream 1, seq 1..n).
func mkPacket(payloads ...[]byte) []byte {
	b := binary.BigEndian.AppendUint16(nil, uint16(len(payloads)))
	for i, p := range payloads {
		b = binary.BigEndian.AppendUint64(b, 1)
		b = binary.BigEndian.AppendUint64(b, uint64(i+1))
		b = binary.BigEndian.AppendUint32(b, uint32(len(p)))
		b = append(b, p...)
	}
	return b
}

// readPacket reads one packet from r.
func readPacket(r *bufio.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	out := append([]byte(nil), hdr[:]...)
	for range binary.BigEndian.Uint16(hdr[:]) {
		var fh [20]byte
		if _, err := io.ReadFull(r, fh[:]); err != nil {
			return nil, err
		}
		body := make([]byte, binary.BigEndian.Uint32(fh[16:]))
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, err
		}
		out = append(append(out, fh[:]...), body...)
	}
	return out, nil
}

// laneServer serves s's endpoints; returns counts the ServeDown calls that
// have returned. deliver records upstream packets.
type laneServer struct {
	*httptest.Server
	s       *State
	returns chan struct{}
	mu      sync.Mutex
	got     [][]byte
}

func newLaneServer(t *testing.T, s *State) *laneServer {
	t.Helper()
	ls := &laneServer{s: s, returns: make(chan struct{}, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("/__tango/lane/down", func(w http.ResponseWriter, r *http.Request) {
		defer func() { ls.returns <- struct{}{} }()
		s.ServeDown(w, r)
	})
	mux.HandleFunc("/__tango/lane/up", func(w http.ResponseWriter, r *http.Request) {
		s.ServeUp(w, r, func(p []byte) {
			ls.mu.Lock()
			ls.got = append(ls.got, append([]byte(nil), p...))
			ls.mu.Unlock()
		})
	})
	ls.Server = httptest.NewServer(mux)
	t.Cleanup(ls.Close)
	return ls
}

func (ls *laneServer) delivered() [][]byte {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.got
}

func (ls *laneServer) url(path string, gen uint64, tok string) string {
	return fmt.Sprintf("%s/__tango/lane/%s?gen=%d&tok=%s", ls.URL, path, gen, tok)
}

// waitReturn waits for one ServeDown call to return.
func (ls *laneServer) waitReturn(t *testing.T) {
	t.Helper()
	select {
	case <-ls.returns:
	case <-time.After(waitFor):
		t.Fatal("ServeDown did not return")
	}
}

// down is an open down:scheme response.
type down struct {
	resp   *http.Response
	r      *bufio.Reader
	cancel context.CancelFunc
}

// openDown GETs the down endpoint and reads the initial empty packet.
func openDown(t *testing.T, ls *laneServer, gen uint64, tok string) *down {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", ls.url("down", gen, tok), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	if resp.StatusCode != 200 {
		t.Fatalf("GET down: %d", resp.StatusCode)
	}
	if ct, cc := resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"); ct != "application/octet-stream" || cc != "no-store" {
		t.Fatalf("headers: Content-Type %q, Cache-Control %q", ct, cc)
	}
	d := &down{resp: resp, r: bufio.NewReader(resp.Body), cancel: cancel}
	if p := d.next(t); !bytes.Equal(p, []byte{0, 0}) {
		t.Fatalf("first packet %v, want the empty packet", p)
	}
	return d
}

type result struct {
	p   []byte
	err error
}

// read reads the next packet within waitFor.
func (d *down) read(t *testing.T) ([]byte, error) {
	t.Helper()
	ch := make(chan result, 1)
	go func() { p, err := readPacket(d.r); ch <- result{p, err} }()
	select {
	case r := <-ch:
		return r.p, r.err
	case <-time.After(waitFor):
		t.Fatal("no packet in time")
		return nil, nil
	}
}

func (d *down) next(t *testing.T) []byte {
	t.Helper()
	p, err := d.read(t)
	if err != nil {
		t.Fatalf("reading packet: %v", err)
	}
	return p
}

// expectEnd checks that the response ends with no further packet.
func (d *down) expectEnd(t *testing.T) {
	t.Helper()
	p, err := d.read(t)
	if err == nil {
		t.Fatalf("got packet %v, want the response to end", p)
	}
}

// schemeState returns a State with down:scheme and up:post ready.
func schemeState(t *testing.T) (*State, helloJSON) {
	t.Helper()
	s := New(nil)
	h := parseHello(t, s.Hello([]string{DownScheme, UpPost}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	s.Control(ctl("ready", UpPost, h.Gen))
	return s, h
}

func TestDownEndpointStreams(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	d := openDown(t, ls, h.Gen, h.Tok)
	p1, p2 := mkPacket([]byte("one")), mkPacket(bytes.Repeat([]byte{7}, 5000), []byte("x"))
	if !s.Send(p1) {
		t.Fatal("Send p1 refused")
	}
	// p1 arrives before p2 is even queued: packets are flushed one by one.
	if got := d.next(t); !bytes.Equal(got, p1) {
		t.Fatalf("got %v, want p1", got)
	}
	if !s.Send(p2) {
		t.Fatal("Send p2 refused")
	}
	if got := d.next(t); !bytes.Equal(got, p2) {
		t.Fatalf("got %d bytes, want p2", len(got))
	}
	// Packets queued before the response connects are written on connect.
	d.cancel()
	ls.waitReturn(t)
}

func TestDownEndpointQueuedBeforeConnect(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	p := mkPacket([]byte("early"))
	if !s.Send(p) {
		t.Fatal("Send refused")
	}
	d := openDown(t, ls, h.Gen, h.Tok)
	if got := d.next(t); !bytes.Equal(got, p) {
		t.Fatalf("got %v, want the early packet", got)
	}
}

func TestDownEndpointEndsOnNewGen(t *testing.T) {
	ends := []struct {
		name string
		do   func(s *State, h helloJSON)
	}{
		{"Hello", func(s *State, _ helloJSON) { s.Hello([]string{DownScheme}, true) }},
		{"Reset", func(s *State, _ helloJSON) { s.Reset() }},
		{"SuspendDown", func(s *State, _ helloJSON) { s.SuspendDown() }},
		{"Close", func(s *State, _ helloJSON) { s.Close() }},
		{"fail", func(s *State, h helloJSON) { s.Control(ctl("fail", DownScheme, h.Gen)) }},
	}
	for _, e := range ends {
		t.Run(e.name, func(t *testing.T) {
			s, h := schemeState(t)
			ls := newLaneServer(t, s)
			d := openDown(t, ls, h.Gen, h.Tok)
			e.do(s, h)
			d.expectEnd(t)
			ls.waitReturn(t)
		})
	}
	t.Run("old queue dropped", func(t *testing.T) {
		s, h := schemeState(t)
		ls := newLaneServer(t, s)
		if !s.Send(mkPacket([]byte("old"))) {
			t.Fatal("Send refused")
		}
		h2 := parseHello(t, s.Hello([]string{DownScheme}, true))
		s.Control(ctl("ready", DownScheme, h2.Gen))
		// The old generation's token is dead.
		resp, err := http.Get(ls.url("down", h.Gen, h.Tok))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("old token: %d", resp.StatusCode)
		}
		d := openDown(t, ls, h2.Gen, h2.Tok)
		pn := mkPacket([]byte("new"))
		s.Send(pn)
		if got := d.next(t); !bytes.Equal(got, pn) {
			t.Fatalf("got %q, want only the new generation's packet", got)
		}
	})
}

func TestDownEndpointOneConnection(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	d1 := openDown(t, ls, h.Gen, h.Tok)
	d2 := openDown(t, ls, h.Gen, h.Tok)
	d1.expectEnd(t)
	ls.waitReturn(t)
	p := mkPacket([]byte("to the second"))
	if !s.Send(p) {
		t.Fatal("Send refused: a replaced connection must not fail the lane")
	}
	if got := d2.next(t); !bytes.Equal(got, p) {
		t.Fatalf("got %v", got)
	}
}

func TestDownEndpointClientGone(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	d := openDown(t, ls, h.Gen, h.Tok)
	d.cancel()
	ls.waitReturn(t)
	// Nobody reads the queue any more: frames must go as messages.
	if s.Send(mkPacket([]byte("x"))) || s.MinSize() != -1 {
		t.Fatal("down:scheme still usable after its response ended")
	}
}

func TestDownEndpointNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	func() {
		s, h := schemeState(t)
		ls := newLaneServer(t, s)
		for range 5 {
			d := openDown(t, ls, h.Gen, h.Tok)
			h2 := parseHello(t, s.Hello([]string{DownScheme}, true))
			d.expectEnd(t)
			ls.waitReturn(t)
			d.resp.Body.Close()
			h = h2
		}
		d := openDown(t, ls, h.Gen, h.Tok)
		d.cancel()
		ls.waitReturn(t)
		d.resp.Body.Close()
		ls.Close()
		http.DefaultClient.CloseIdleConnections()
	}()
	deadline := time.Now().Add(waitFor)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines: %d before, %d after\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}

// plainWriter is a ResponseWriter without Flush.
type plainWriter struct {
	h    http.Header
	code int
	buf  bytes.Buffer
}

func (w *plainWriter) Header() http.Header         { return w.h }
func (w *plainWriter) WriteHeader(c int)           { w.code = c }
func (w *plainWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }

func TestDownEndpointNeedsFlusher(t *testing.T) {
	s, h := schemeState(t)
	w := &plainWriter{h: http.Header{}}
	s.ServeDown(w, httptest.NewRequest("GET", fmt.Sprintf("/__tango/lane/down?gen=%d&tok=%s", h.Gen, h.Tok), nil))
	if w.code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", w.code)
	}
	if s.Send(mkPacket()) || s.MinSize() != -1 {
		t.Fatal("down:scheme not failed")
	}
}

// failWriter fails every write after the first n.
type failWriter struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
	n  int
}

func (w *failWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.n == 0 {
		return 0, errors.New("broken pipe")
	}
	w.n--
	return w.ResponseRecorder.Write(b)
}

func TestDownEndpointWriteError(t *testing.T) {
	s, h := schemeState(t)
	w := &failWriter{ResponseRecorder: httptest.NewRecorder(), n: 1}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.ServeDown(w, httptest.NewRequest("GET", fmt.Sprintf("/__tango/lane/down?gen=%d&tok=%s", h.Gen, h.Tok), nil))
	}()
	// Wait until the empty packet went out, then queue one that fails.
	for deadline := time.Now().Add(waitFor); ; {
		w.mu.Lock()
		n := w.n
		w.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("empty packet not written")
		}
		time.Sleep(time.Millisecond)
	}
	if !s.Send(mkPacket([]byte("lost"))) {
		t.Fatal("Send refused")
	}
	select {
	case <-done:
	case <-time.After(waitFor):
		t.Fatal("ServeDown did not return after a write error")
	}
	if s.Send(mkPacket()) || s.MinSize() != -1 {
		t.Fatal("down:scheme not failed after a write error")
	}
}

func post(t *testing.T, ls *laneServer, gen uint64, tok string, body []byte) int {
	t.Helper()
	resp, err := http.Post(ls.url("up", gen, tok), "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestUpEndpointDelivers(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	p := mkPacket([]byte("hello"), []byte("world"))
	if code := post(t, ls, h.Gen, h.Tok, p); code != http.StatusNoContent {
		t.Fatalf("POST: %d, want 204", code)
	}
	if got := ls.delivered(); len(got) != 1 || !bytes.Equal(got[0], p) {
		t.Fatalf("delivered %v", got)
	}
	// A failed upstream lane answers 503 (runtime-js then fails up:post).
	s.Control(ctl("fail", UpPost, h.Gen))
	if code := post(t, ls, h.Gen, h.Tok, p); code != http.StatusServiceUnavailable {
		t.Fatalf("POST to a failed lane: %d, want 503", code)
	}
	if len(ls.delivered()) != 1 {
		t.Fatal("packet delivered on a failed lane")
	}
	// Only POST.
	resp, err := http.Get(ls.url("up", h.Gen, h.Tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET up: %d, want 405", resp.StatusCode)
	}
}

// countingReader is an endless body that counts what was read from it.
type countingReader struct{ n int }

func (r *countingReader) Read(b []byte) (int, error) {
	r.n += len(b)
	return len(b), nil
}

func TestUpEndpointTooLarge(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	if code := post(t, ls, h.Gen, h.Tok, make([]byte, MaxUpBody+1)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST of MaxUpBody+1: %d, want 413", code)
	}
	if len(ls.delivered()) != 0 {
		t.Fatal("oversized packet delivered")
	}
	// The largest packet runtime-js makes: one frame of MaxPostPacket bytes.
	if code := post(t, ls, h.Gen, h.Tok, make([]byte, MaxUpBody)); code != http.StatusNoContent {
		t.Fatalf("POST of MaxUpBody: %d, want 204", code)
	}
	// An endless body of unknown length is not read to its end.
	cr := &countingReader{}
	r := httptest.NewRequest("POST", fmt.Sprintf("/__tango/lane/up?gen=%d&tok=%s", h.Gen, h.Tok), io.LimitReader(cr, 64<<20))
	r.ContentLength = -1
	w := httptest.NewRecorder()
	s.ServeUp(w, r, func([]byte) { t.Error("deliver called") })
	if w.Code != http.StatusRequestEntityTooLarge || cr.n > 2*MaxUpBody {
		t.Fatalf("endless body: status %d after reading %d bytes", w.Code, cr.n)
	}
	// A declared length over the limit is refused without reading.
	cr = &countingReader{}
	r = httptest.NewRequest("POST", fmt.Sprintf("/__tango/lane/up?gen=%d&tok=%s", h.Gen, h.Tok), io.LimitReader(cr, 64<<20))
	r.ContentLength = 64 << 20
	w = httptest.NewRecorder()
	s.ServeUp(w, r, func([]byte) { t.Error("deliver called") })
	if w.Code != http.StatusRequestEntityTooLarge || cr.n != 0 {
		t.Fatalf("declared length: status %d after reading %d bytes", w.Code, cr.n)
	}
}

func TestEndpointsRejectBadTokenGenOrigin(t *testing.T) {
	s, h := schemeState(t)
	ls := newLaneServer(t, s)
	d := openDown(t, ls, h.Gen, h.Tok)
	badTok := strings.Repeat("0", 32)
	queries := []string{
		fmt.Sprintf("gen=%d&tok=%s", h.Gen, badTok),
		fmt.Sprintf("gen=%d&tok=%s", h.Gen-1, h.Tok),
		fmt.Sprintf("gen=%d&tok=%s", h.Gen+1, h.Tok),
		fmt.Sprintf("gen=%d", h.Gen),
		fmt.Sprintf("tok=%s", h.Tok),
		fmt.Sprintf("gen=x&tok=%s", h.Tok),
		fmt.Sprintf("gen=%d&tok=", h.Gen),
		"",
	}
	for _, q := range queries {
		for _, ep := range []struct{ method, path string }{{"GET", "down"}, {"POST", "up"}} {
			req, _ := http.NewRequest(ep.method, ls.URL+"/__tango/lane/"+ep.path+"?"+q, bytes.NewReader(mkPacket([]byte("x"))))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s?%s: %d, want 403", ep.method, ep.path, q, resp.StatusCode)
			}
		}
	}
	if len(ls.delivered()) != 0 {
		t.Fatal("packet delivered with a bad token")
	}
	// The current down connection is unaffected.
	p := mkPacket([]byte("still here"))
	if !s.Send(p) {
		t.Fatal("Send refused")
	}
	if got := d.next(t); !bytes.Equal(got, p) {
		t.Fatalf("got %v", got)
	}

	// A lane the generation did not offer is refused too.
	s2 := New(nil)
	h2 := parseHello(t, s2.Hello([]string{UpAndroid, DownAndroid}, true))
	ls2 := newLaneServer(t, s2)
	resp, err := http.Get(ls2.url("down", h2.Gen, h2.Tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("down without down:scheme: %d, want 403", resp.StatusCode)
	}
	if code := post(t, ls2, h2.Gen, h2.Tok, mkPacket()); code != http.StatusForbidden {
		t.Errorf("up without up:post: %d, want 403", code)
	}
	// Wrong method on down.
	if code := post(t, ls, h.Gen, h.Tok, nil); code != http.StatusNoContent {
		t.Errorf("sanity POST up: %d", code)
	}
	req, _ := http.NewRequest("POST", ls.url("down", h.Gen, h.Tok), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST down: %d, want 405", resp.StatusCode)
	}
}
