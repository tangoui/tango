package lane

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// The first payload write is attempted but held; the queue tail must remain
// charged and visible to the replacement, without retransmitting that write.
type barrierWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	resume  chan struct{}
}

func (w *barrierWriter) Write(p []byte) (int, error) {
	if len(p) > 2 {
		close(w.entered)
		<-w.resume
	}
	return w.ResponseRecorder.Write(p)
}
func waitBarrier(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("barrier timeout")
	}
}

func TestDownReplacementPreservesUnattemptedTail(t *testing.T) {
	for _, ending := range []string{"replace", "reset", "fail"} {
		t.Run(ending, func(t *testing.T) {
			s, h := schemeState(t)
			p1, p2, p3, p4 := mkPacket([]byte("attempted")), mkPacket([]byte("tail-1")), mkPacket([]byte("tail-2")), mkPacket([]byte("newer"))
			for _, p := range [][]byte{p1, p2, p3} {
				if !s.Send(p) {
					t.Fatal("admission refused")
				}
			}
			url := fmt.Sprintf("/__tango/lane/down?gen=%d&tok=%s", h.Gen, h.Tok)
			old := &barrierWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
			done := make(chan struct{})
			go func() { s.ServeDown(old, httptest.NewRequest("GET", url, nil)); close(done) }()
			waitBarrier(t, old.entered)
			_, charged := s.Queued()
			if charged != len(p1)+len(p2)+len(p3) {
				t.Fatalf("flight uncharged: %d", charged)
			}
			if ending == "reset" {
				s.Reset()
			} else if ending == "fail" {
				s.Control(ctl("fail", DownScheme, h.Gen))
			}
			if ending != "replace" {
				close(old.resume)
				waitBarrier(t, done)
				if old.Body.Len() != 2+len(p1) {
					t.Fatal("ended generation wrote an unattempted tail")
				}
				if _, n := s.Queued(); n != 0 {
					t.Fatal("ended queue retained bytes")
				}
				return
			}
			if !s.Send(p4) {
				t.Fatal("newer admission refused")
			}
			// A real replacement serves the queued tail even while old Write blocks.
			ls := newLaneServer(t, s)
			fresh := openDown(t, ls, h.Gen, h.Tok)
			for _, want := range [][]byte{p2, p3, p4} {
				if got := fresh.next(t); !bytes.Equal(got, want) {
					t.Fatalf("replacement got %q, want %q", got, want)
				}
			}
			close(old.resume)
			waitBarrier(t, done)
			if old.Body.Len() != 2+len(p1) {
				t.Fatal("attempted packet retransmitted or old tail written")
			}
			s.Close()
		})
	}
}

func TestUpAuthorizationRetainsGenerationAcrossCallback(t *testing.T) {
	s, h := schemeState(t)
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

// A detached write still counts against admission until it returns.
func TestDownFlightCountsAgainstBound(t *testing.T) {
	s, h := schemeState(t)
	p := make([]byte, maxDownQueue)
	if !s.Send(p) {
		t.Fatal("initial admission")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &barrierWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	req := httptest.NewRequest("GET", fmt.Sprintf("/__tango/lane/down?gen=%d&tok=%s", h.Gen, h.Tok), nil).WithContext(ctx)
	go func() { s.ServeDown(w, req); close(done) }()
	waitBarrier(t, w.entered)
	if s.Send([]byte{0, 0}) {
		t.Fatal("in-flight bytes bypassed queue limit")
	}
	s.Close()
	close(w.resume)
	waitBarrier(t, done)
}

type flushBarrierWriter struct {
	*httptest.ResponseRecorder
	entered, resume chan struct{}
	flushes         int
}

func (w *flushBarrierWriter) Flush() {
	w.flushes++
	if w.flushes == 2 {
		close(w.entered)
		<-w.resume
	}
	w.ResponseRecorder.Flush()
}

func TestDownBlockedFlushCountsAgainstBound(t *testing.T) {
	s, h := schemeState(t)
	if !s.Send(make([]byte, maxDownQueue)) {
		t.Fatal("initial admission")
	}
	w := &flushBarrierWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), resume: make(chan struct{})}
	done := make(chan struct{})
	req := httptest.NewRequest("GET", fmt.Sprintf("/__tango/lane/down?gen=%d&tok=%s", h.Gen, h.Tok), nil)
	go func() { s.ServeDown(w, req); close(done) }()
	waitBarrier(t, w.entered)
	if s.Send([]byte{0, 0}) {
		t.Fatal("pending flush bypassed queue bound")
	}
	s.Close()
	close(w.resume)
	waitBarrier(t, done)
}

func TestGenerationZeroHandleCannotEnterNewQueue(t *testing.T) {
	s := New(nil)
	gen := s.Generation()
	h := parseHello(t, s.Hello([]string{DownScheme}, true))
	s.Control(ctl("ready", DownScheme, h.Gen))
	if s.SendGeneration(gen, []byte{0, 0}) {
		t.Fatal("generation zero used as an unscoped wildcard")
	}
	if n, b := s.Queued(); n != 0 || b != 0 {
		t.Fatal("stale handle retained bytes")
	}
	s.Close()
}
