package lane

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

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

// laneServer serves s's up:post endpoint and records the packets
// delivered.
type laneServer struct {
	*httptest.Server
	s   *State
	mu  sync.Mutex
	got [][]byte
}

func newLaneServer(t *testing.T, s *State) *laneServer {
	t.Helper()
	ls := &laneServer{s: s}
	mux := http.NewServeMux()
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

// postState returns a State with down:poll and up:post ready.
func postState(t *testing.T) (*State, helloJSON) {
	t.Helper()
	s := New(nil, (&recorder{}).send)
	h := parseHello(t, s.Hello([]string{DownPoll, UpPost}, true))
	s.Control(ctl("ready", DownPoll, h.Gen))
	s.Control(ctl("ready", UpPost, h.Gen))
	return s, h
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
	s, h := postState(t)
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
	s, h := postState(t)
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

func TestUpEndpointRejectsBadTokenOrGen(t *testing.T) {
	s, h := postState(t)
	ls := newLaneServer(t, s)
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
		resp, err := http.Post(ls.URL+"/__tango/lane/up?"+q, "application/octet-stream", bytes.NewReader(mkPacket([]byte("x"))))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST up?%s: %d, want 403", q, resp.StatusCode)
		}
	}
	if len(ls.delivered()) != 0 {
		t.Fatal("packet delivered with a bad token")
	}
	if code := post(t, ls, h.Gen, h.Tok, nil); code != http.StatusNoContent {
		t.Errorf("sanity POST up: %d", code)
	}

	// A generation that did not offer up:post refuses it.
	s2 := New(nil, nil)
	h2 := parseHello(t, s2.Hello([]string{UpAndroid, DownAndroid, DownPoll}, true))
	ls2 := newLaneServer(t, s2)
	if code := post(t, ls2, h2.Gen, h2.Tok, mkPacket()); code != http.StatusForbidden {
		t.Errorf("up without up:post: %d, want 403", code)
	}
}
