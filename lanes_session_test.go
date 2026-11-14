package tango

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/ipc"
)

func TestHTTPAuthorizedCallbackCannotReachReloadedStream(t *testing.T) {
	a, fb := laneApp(t, Options{})
	r := ipc.NewRouter()
	entered, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	// Pause inside actual callback dispatch, after State authorization and
	// before Router lookup. State.mu must not be held across this callback.
	a.OnBinary(func(_ *Surface, _ uint64, _ []byte) { close(entered); <-resume })
	a.OnBinary(func(s *Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })
	s := laneSurface(t, a, fb, "", lane.UpPost)
	r.HandleMessage(s, `{"tango":"hello","proto":2}`)
	gen, transport := s.BinarySession()
	_ = transport
	old, err := r.OpenStream(s, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	var hello laneHelloJSON
	// LaneHello already ran through Router. Get the token without rotating it
	// from the emitted hello reply.
	waitUntil(t, "hello token", func() bool {
		for _, m := range ipcReplies(t, fb, s) {
			if v, ok := m["lanes"].(map[string]any); ok {
				hello.Gen = uint64(v["gen"].(float64))
				hello.Tok = v["tok"].(string)
				return true
			}
		}
		return false
	})
	packet := ipc.AppendPacket(nil, ipc.PacketFrame{Stream: 2, Seq: 0, Payload: []byte("stale")})
	req := httptest.NewRequest("POST", fmt.Sprintf("/__tango/lane/up?gen=%d&tok=%s", hello.Gen, hello.Tok), bytes.NewReader(packet))
	go func() { a.serveLane(s.ID(), httptest.NewRecorder(), req); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback not reached")
	}
	// Go ids are never reused across hello; Drop (page closed, surface
	// reused) forgets them, so the successor gets the old stream's id 2.
	r.Drop(s)
	r.HandleMessage(s, `{"tango":"hello","proto":2}`)
	fresh, err := r.OpenStream(s, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID() != old.ID() {
		t.Fatal("test requires reused ID")
	}
	close(resume)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not finish")
	}
	nextGen, _ := s.BinarySession()
	if nextGen == gen {
		t.Fatal("generation did not rotate")
	}
	r.HandleBinarySession(s, nextGen, ipc.AppendPacket(nil, ipc.PacketFrame{Stream: 2, Seq: 0, Payload: []byte("fresh")}))
	p, err := fresh.ReceiveBytes()
	if err != nil || string(p) != "fresh" {
		t.Fatalf("successor got %q, %v", p, err)
	}
	if old.Context().Err() == nil {
		t.Fatal("old stream survived Drop")
	}
	r.Drop(s)
}

// runtime-js's up:post start probe POSTs the empty packet [0,0] (count 0)
// and treats only a 204 as ready: the core must answer it 204, the Router
// must ignore it, and the lane must stay usable for the frames after it.
func TestLaneUpStartProbeEmptyPacket(t *testing.T) {
	a, fb := laneApp(t, Options{})
	r := ipc.NewRouter()
	var mu sync.Mutex
	var got [][]byte
	a.OnBinary(func(s *Surface, gen uint64, p []byte) {
		mu.Lock()
		got = append(got, append([]byte(nil), p...))
		mu.Unlock()
		r.HandleBinarySession(s, gen, p)
	})
	s := laneSurface(t, a, fb, "", lane.UpPost)
	r.HandleMessage(s, `{"tango":"hello","proto":2}`)
	var hello laneHelloJSON
	waitUntil(t, "hello token", func() bool {
		for _, m := range ipcReplies(t, fb, s) {
			if v, ok := m["lanes"].(map[string]any); ok {
				hello.Gen = uint64(v["gen"].(float64))
				hello.Tok = v["tok"].(string)
				return true
			}
		}
		return false
	})
	st, err := r.OpenStream(s, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	post := func(body []byte) int {
		req := httptest.NewRequest("POST", fmt.Sprintf("/__tango/lane/up?gen=%d&tok=%s", hello.Gen, hello.Tok), bytes.NewReader(body))
		w := httptest.NewRecorder()
		a.serveLane(s.ID(), w, req)
		return w.Code
	}
	if code := post([]byte{0, 0}); code != http.StatusNoContent {
		t.Fatalf("start probe: HTTP %d, want 204", code)
	}
	if code := post(ipc.AppendPacket(nil, ipc.PacketFrame{Stream: st.ID(), Seq: 0, Payload: []byte("after-probe")})); code != http.StatusNoContent {
		t.Fatalf("data after the probe: HTTP %d, want 204", code)
	}
	p, err := st.ReceiveBytes()
	if err != nil || string(p) != "after-probe" {
		t.Fatalf("stream got %q, %v; want the frame after the probe", p, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || !bytes.Equal(got[0], []byte{0, 0}) {
		t.Fatalf("OnBinary got %q, want the empty packet then the data packet", got)
	}
	r.Drop(s)
}
