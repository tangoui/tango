package downq

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestPushBeforeActiveUsesFallback(t *testing.T) {
	s, rec := newState(t)
	if seq := s.Push(Msg, []byte("none")); seq != 0 {
		t.Fatalf("Push before Start = %d, want 0", seq)
	}
	gen, tok := s.Start(true)
	if gen == 0 || len(tok) != 32 {
		t.Fatalf("Start = %d %q", gen, tok)
	}
	for i := range 3 {
		if seq := s.Push(Msg, []byte{byte(i)}); seq != uint64(i+1) {
			t.Fatalf("Push %d = seq %d", i, seq)
		}
	}
	if got := rec.seqs(); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Fatalf("fallback got %v", got)
	}
	if s.Active() {
		t.Fatal("active before the first poll")
	}

	gen2, tok2 := s.Start(false)
	if gen2 != gen+1 || tok2 != "" {
		t.Fatalf("Start(false) = %d %q", gen2, tok2)
	}
	if seq := s.Push(Barrier, []byte{0, 0, 0, 0, 0, 0, 0, 1}); seq != 1 {
		t.Fatalf("seq = %d", seq)
	}
	if got := rec.seqs(); !slices.Equal(got, []uint64{1, 2, 3, 1}) {
		t.Fatalf("fallback got %v", got)
	}
	w := poll(context.Background(), s, gen2, "", 0)
	if w.Code != http.StatusForbidden || s.Active() {
		t.Fatalf("poll of a generation without polling: %d, active %v", w.Code, s.Active())
	}
}

func TestFailResendsUnacked(t *testing.T) {
	s, rec := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	for range 4 {
		s.Push(Msg, []byte("m"))
	}
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 4)

	s.Fail(gen+1, 0) // another generation's report: ignored
	if !s.Active() || len(rec.seqs()) != 0 {
		t.Fatal("Fail of another generation took effect")
	}

	s.Fail(gen, 2)
	if got := rec.seqs(); !slices.Equal(got, []uint64{3, 4}) {
		t.Fatalf("fallback got %v", got)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("fallback called %d times", len(rec.calls))
	}
	if s.Active() {
		t.Fatal("active after Fail")
	}
	if seq := s.Push(Msg, []byte("n")); seq != 5 {
		t.Fatalf("seq = %d", seq)
	}
	if got := rec.seqs(); !slices.Equal(got, []uint64{3, 4, 5}) {
		t.Fatalf("fallback got %v", got)
	}
	if w := poll(context.Background(), s, gen, tok, 5); w.Code != http.StatusGone {
		t.Fatalf("poll after Fail: status = %d, want 410", w.Code)
	}
	s.Fail(gen, 5) // repeated: nothing more to hand over
	if got := rec.seqs(); !slices.Equal(got, []uint64{3, 4, 5}) {
		t.Fatalf("fallback got %v", got)
	}
}

func TestNewGenerationEndsPoll(t *testing.T) {
	s, rec := newState(t)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	s.Push(Msg, []byte("a"))
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 1)
	ch := goPoll(s, gen, tok, 1)
	waitHeld(t, s, 1)

	gen2, tok2 := s.Start(true)
	if w := await(t, ch); w.Code != http.StatusGone {
		t.Fatalf("held poll: status = %d, want 410", w.Code)
	}
	if gen2 == gen || tok2 == tok || tok2 == "" {
		t.Fatalf("Start = %d %q after %d %q", gen2, tok2, gen, tok)
	}
	if seq := s.Push(Msg, []byte("b")); seq != 1 {
		t.Fatalf("first seq of the new generation = %d", seq)
	}
	if got := rec.seqs(); !slices.Equal(got, []uint64{1}) {
		t.Fatalf("fallback got %v", got)
	}
	if w := poll(context.Background(), s, gen, tok, 1); w.Code != http.StatusForbidden {
		t.Fatalf("poll with the old token: status = %d, want 403", w.Code)
	}
	waitHeld(t, s, 0)
}

func TestEndDropsQueue(t *testing.T) {
	s, rec := newState(t)
	setVar(t, &BatchItems, 2)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	for range 3 {
		s.Push(Msg, []byte("m"))
	}
	wantBatch(t, poll(context.Background(), s, gen, tok, 0), 1, 2) // 1, 2 unacked; 3 queued
	s.End()
	s.mu.Lock()
	n := len(s.unacked) + len(s.queue)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d items kept after End", n)
	}
	if seq := s.Push(Msg, []byte("x")); seq != 0 {
		t.Fatalf("Push after End = %d, want 0", seq)
	}
	if got := rec.seqs(); len(got) != 0 {
		t.Fatalf("fallback got %v", got)
	}
	if s.Active() {
		t.Fatal("active after End")
	}
	if w := poll(context.Background(), s, gen, tok, 0); w.Code != http.StatusGone {
		t.Fatalf("poll after End: status = %d, want 410", w.Code)
	}

	gen, tok = s.Start(true)
	ch := goPoll(s, gen, tok, 0)
	waitHeld(t, s, 1)
	s.End()
	if w := await(t, ch); w.Code != http.StatusGone {
		t.Fatalf("held poll: status = %d, want 410", w.Code)
	}

	// A held poll ends when its request goes away, and on Close.
	gen, tok = s.Start(true)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan int, 1)
	go func() { got <- poll(ctx, s, gen, tok, 0).Body.Len() }()
	waitHeld(t, s, 1)
	cancel()
	select {
	case n := <-got:
		if n != 0 {
			t.Fatalf("canceled poll wrote %d bytes", n)
		}
	case <-time.After(waitFor):
		t.Fatal("canceled poll did not end")
	}
	waitHeld(t, s, 0)
	ch = goPoll(s, gen, tok, 0)
	waitHeld(t, s, 1)
	s.Close()
	if w := await(t, ch); w.Code != http.StatusGone {
		t.Fatalf("held poll at Close: status = %d, want 410", w.Code)
	}
	if gen2, tok2 := s.Start(true); gen2 != gen || tok2 != "" {
		t.Fatalf("Start after Close = %d %q", gen2, tok2)
	}
	if seq := s.Push(Msg, []byte("x")); seq != 0 {
		t.Fatalf("Push after Close = %d, want 0", seq)
	}
}

func TestWatchdogFallsBack(t *testing.T) {
	s, rec := newState(t)
	setVar(t, &WatchdogAfter, 50*time.Millisecond)
	gen, tok := s.Start(true)
	activate(t, s, gen, tok)
	pushed := time.Now()
	s.Push(Msg, []byte("a"))
	s.Push(Msg, []byte("b"))
	if len(rec.seqs()) != 0 {
		t.Fatal("fallback before the watchdog")
	}
	deadline := time.Now().Add(waitFor)
	for len(rec.seqs()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("fallback got %v", rec.seqs())
		}
		time.Sleep(time.Millisecond)
	}
	if d := time.Since(pushed); d < 45*time.Millisecond {
		t.Fatalf("watchdog fired after %v", d)
	}
	if got := rec.seqs(); !slices.Equal(got, []uint64{1, 2}) || len(rec.calls) != 1 {
		t.Fatalf("fallback calls %v", rec.calls)
	}
	if s.Active() {
		t.Fatal("active while fallen back")
	}
	s.Push(Msg, []byte("c")) // still fallen back
	if got := rec.seqs(); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Fatalf("fallback got %v", got)
	}

	// The next poll resumes polling.
	setVar(t, &HoldMax, 5*time.Millisecond)
	wantBatch(t, poll(context.Background(), s, gen, tok, 3), 0, 0)
	if !s.Active() {
		t.Fatal("not active after the poll")
	}
	s.Push(Msg, []byte("d"))
	items := wantBatch(t, poll(context.Background(), s, gen, tok, 3), 4, 1)
	if string(items[0].Data) != "d" {
		t.Fatalf("data = %q", items[0].Data)
	}
	// The poll stopped the watchdog that Push armed.
	time.Sleep(100 * time.Millisecond)
	if got := rec.seqs(); !slices.Equal(got, []uint64{1, 2, 3}) {
		t.Fatalf("fallback got %v", got)
	}
}
