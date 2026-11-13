package mobilecanvas

import (
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

func ms(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }

// step is one scheduler input and the expected outcome.
type step struct {
	op   string // inv, at, wake, tick, timer, sync, bg, fg, close
	t    int    // ms: the time of the call (at: the requested deadline)
	wake bool   // inv/at: expected wake result
	want Action // other ops: expected action (At given in ms via arm)
	arm  int    // >0: want.Arm with At = ms(arm); -1: want.Arm with zero At
}

func runSteps(t *testing.T, s *Sched, steps []step) {
	t.Helper()
	for i, st := range steps {
		want := st.want
		switch {
		case st.arm > 0:
			want.Arm, want.At = true, ms(st.arm)
		case st.arm < 0:
			want.Arm, want.At = true, time.Time{}
		}
		var got Action
		switch st.op {
		case "inv":
			if w := s.Invalidate(); w != st.wake {
				t.Fatalf("step %d inv: wake = %v, want %v", i, w, st.wake)
			}
			continue
		case "at":
			if w := s.InvalidateAt(ms(st.t)); w != st.wake {
				t.Fatalf("step %d at(%d): wake = %v, want %v", i, st.t, w, st.wake)
			}
			continue
		case "wake":
			got = s.Wake(ms(st.t))
		case "tick":
			got = s.Tick(ms(st.t))
		case "timer":
			got = s.TimerFired(ms(st.t))
		case "sync":
			got = s.SyncFrame()
		case "bg":
			got = s.Background()
		case "fg":
			got = s.Foreground(ms(st.t))
		case "close":
			got = s.Close()
		default:
			t.Fatalf("bad op %q", st.op)
		}
		if got.Link != want.Link || got.Frame != want.Frame || got.Arm != want.Arm || !got.At.Equal(want.At) {
			t.Fatalf("step %d %s@%d: got %+v, want %+v", i, st.op, st.t, got, want)
		}
	}
}

var (
	resume = Action{Link: LinkResume}
	pause  = Action{Link: LinkPause}
	frame  = Action{Frame: true}
	none   = Action{}
)

func TestSched(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{"still page draws nothing", []step{
			{op: "wake", t: 0, want: none},
			{op: "tick", t: 16, want: none},
			{op: "timer", t: 20, want: none},
		}},
		{"invalidate resumes, frames, idles out after 4 ticks", []step{
			{op: "inv", wake: true},
			{op: "inv", wake: false}, // coalesced
			{op: "wake", t: 1, want: resume},
			{op: "tick", t: 16, want: frame},
			{op: "tick", t: 33, want: none},
			{op: "tick", t: 50, want: none},
			{op: "tick", t: 66, want: none},
			{op: "tick", t: 83, want: pause},
			{op: "tick", t: 100, want: none}, // stray tick after pause
		}},
		{"at most one frame per tick", []step{
			{op: "inv", wake: true},
			{op: "wake", want: resume},
			{op: "tick", t: 16, want: frame},
			{op: "inv", wake: true}, // during the frame
			{op: "inv", wake: false},
			{op: "wake", t: 17, want: none}, // link already running
			{op: "tick", t: 33, want: frame},
			{op: "tick", t: 50, want: none},
		}},
		{"request mid-idle restarts the idle count", []step{
			{op: "inv", wake: true},
			{op: "wake", want: resume},
			{op: "tick", t: 16, want: frame},
			{op: "tick", t: 33, want: none},
			{op: "tick", t: 50, want: none},
			{op: "tick", t: 66, want: none},
			{op: "inv", wake: true},
			{op: "tick", t: 83, want: frame},
			{op: "tick", t: 100, want: none},
			{op: "tick", t: 116, want: none},
			{op: "tick", t: 133, want: none},
			{op: "tick", t: 150, want: pause},
		}},
		{"invalidateAt keeps the earliest deadline", []step{
			{op: "at", t: 500, wake: true},
			{op: "at", t: 800, wake: false},
			{op: "wake", t: 0, arm: 500},
			{op: "at", t: 300, wake: true},
			{op: "wake", t: 1, arm: 300},
			{op: "at", t: 300, wake: false},
			{op: "timer", t: 300, want: resume},
			{op: "tick", t: 316, want: frame},
		}},
		{"timer firing slightly early counts as due", []step{
			{op: "at", t: 100, wake: true},
			{op: "wake", t: 0, arm: 100},
			{op: "timer", t: 99, want: resume},
			{op: "tick", t: 110, want: frame},
		}},
		{"timer firing much too early re-arms", []step{
			{op: "at", t: 100, wake: true},
			{op: "wake", t: 0, arm: 100},
			{op: "timer", t: 50, arm: 100},
			{op: "timer", t: 100, want: resume},
		}},
		{"any frame satisfies a pending deadline", []step{
			{op: "at", t: 500, wake: true},
			{op: "wake", t: 0, arm: 500},
			{op: "inv", wake: true},
			{op: "wake", t: 1, want: resume},
			{op: "tick", t: 16, want: frame, arm: -1}, // timer cancelled
			{op: "timer", t: 500, want: none},
		}},
		{"deadline already due on wake", []step{
			{op: "at", t: 5, wake: true},
			{op: "wake", t: 10, want: resume},
			{op: "tick", t: 16, want: frame},
		}},
		{"deadline due on a tick frames", []step{
			{op: "inv", wake: true},
			{op: "wake", t: 0, want: resume},
			{op: "tick", t: 16, want: frame},
			{op: "at", t: 30, wake: true},
			{op: "wake", t: 20, arm: 30},
			{op: "tick", t: 33, want: frame, arm: -1},
		}},
		{"never frames in the background; catch-up on foreground", []step{
			{op: "inv", wake: true},
			{op: "wake", want: resume},
			{op: "bg", want: pause},
			{op: "tick", t: 16, want: none},
			{op: "sync", want: none},
			{op: "inv", wake: false}, // still pending
			{op: "wake", t: 20, want: none},
			{op: "fg", t: 1000, want: resume},
			{op: "tick", t: 1016, want: frame},
		}},
		{"background without request: no catch-up", []step{
			{op: "bg", want: none},
			{op: "bg", want: none},
			{op: "fg", t: 10, want: none},
			{op: "fg", t: 10, want: none},
			{op: "tick", t: 16, want: none},
		}},
		{"request in background wakes but waits", []step{
			{op: "bg", want: none},
			{op: "inv", wake: true},
			{op: "wake", t: 1, want: none},
			{op: "fg", t: 2, want: resume},
		}},
		{"timer cancelled in background, re-armed or due on foreground", []step{
			{op: "at", t: 500, wake: true},
			{op: "at", t: 900, wake: false},
			{op: "wake", t: 0, arm: 500},
			{op: "bg", arm: -1},
			{op: "timer", t: 500, want: none}, // stale firing: due, but no frame yet
			{op: "fg", t: 600, want: resume},
		}},
		{"deadline passed while in background", []step{
			{op: "at", t: 500, wake: true},
			{op: "wake", t: 0, arm: 500},
			{op: "bg", arm: -1},
			{op: "fg", t: 600, want: resume},
			{op: "tick", t: 616, want: frame},
		}},
		{"sync frame consumes the request", []step{
			{op: "inv", wake: true},
			{op: "sync", want: frame},
			{op: "wake", t: 1, want: none},
			{op: "inv", wake: true},
		}},
		{"close stops everything", []step{
			{op: "at", t: 500, wake: true},
			{op: "inv", wake: true},
			{op: "wake", t: 0, want: resume, arm: 500},
			{op: "close", want: pause, arm: -1},
			{op: "inv", wake: false},
			{op: "at", t: 100, wake: false},
			{op: "wake", t: 1, want: none},
			{op: "tick", t: 16, want: none},
			{op: "sync", want: none},
			{op: "close", want: none},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Sched
			runSteps(t, &s, tt.steps)
		})
	}
}

// A deadline still ahead when the app returns re-arms the timer.
func TestSchedForegroundRearms(t *testing.T) {
	var s Sched
	s.InvalidateAt(ms(500))
	s.Wake(ms(0))
	s.Background()
	if a := s.Foreground(ms(400)); a.Link != LinkKeep || !a.Arm || !a.At.Equal(ms(500)) {
		t.Fatalf("foreground: %+v, want re-arm at 500", a)
	}
	if s.InBackground() {
		t.Fatal("still in background")
	}
}

func TestSchedConcurrentInvalidate(t *testing.T) {
	var s Sched
	var wg sync.WaitGroup
	wakes := make(chan bool, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wakes <- s.Invalidate()
			s.InvalidateAt(ms(100))
		}()
	}
	wg.Wait()
	close(wakes)
	n := 0
	for w := range wakes {
		if w {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d wakes, want 1", n)
	}
	if !s.Pending() || !s.Deadline().Equal(ms(100)) {
		t.Fatalf("pending %v deadline %v", s.Pending(), s.Deadline())
	}
	if a := s.Wake(ms(0)); a.Link != LinkResume || !s.Running() {
		t.Fatalf("wake: %+v", a)
	}
}
