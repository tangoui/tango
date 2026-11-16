package lane

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func waitBarrier(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("barrier timeout")
	}
}

func TestUpAuthorizationRetainsGenerationAcrossCallback(t *testing.T) {
	s, h := postState(t)
	authorized, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var delivered uint64
	req := httptest.NewRequest("POST", fmt.Sprintf("/__tango/lane/up?gen=%d&tok=%s", h.Gen, h.Tok), bytes.NewReader(mkPacket([]byte("old"))))
	go func() {
		s.ServeUpSession(httptest.NewRecorder(), req, func(gen uint64, _ []byte) { close(authorized); <-resume; delivered = gen })
		close(done)
	}()
	waitBarrier(t, authorized)
	s.Hello([]string{UpPost}, true)
	close(resume)
	waitBarrier(t, done)
	if delivered != h.Gen || delivered == s.Generation() {
		t.Fatal("authorization rebound to successor")
	}
	s.Close()
}

func TestGenerationZeroHandleCannotEnterNewQueue(t *testing.T) {
	poll := &recorder{}
	s := New(nil, poll.send)
	gen := s.Generation()
	h := parseHello(t, s.Hello([]string{DownPoll}, true))
	s.Control(ctl("ready", DownPoll, h.Gen))
	if s.SendGeneration(gen, []byte{0, 0}) {
		t.Fatal("generation zero used as an unscoped wildcard")
	}
	if poll.n() != 0 {
		t.Fatal("stale handle reached the poll queue")
	}
	if !s.SendGeneration(h.Gen, []byte{0, 0}) || poll.n() != 1 {
		t.Fatal("current generation refused")
	}
	s.Close()
}
