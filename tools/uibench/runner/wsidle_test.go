package main

import (
	"strings"
	"testing"
	"time"
)

func TestWSNetClamps(t *testing.T) {
	for _, tc := range []struct{ raw, idle, want float64 }{
		{4.0, 3.5, 0.5},
		{3.5, 3.5, 0},
		{3.0, 3.8, 0}, // a quieter compositor than at idle: never negative
		{2.0, 0, 2.0},
	} {
		if got := wsNet(tc.raw, tc.idle); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("wsNet(%v, %v) = %v, want %v", tc.raw, tc.idle, got, tc.want)
		}
	}
	// applyBaseline scales the idle rate to the round's sampled seconds and
	// keeps the raw delta.
	r := Round{Seconds: 10, CPUSeconds: 1, WindowServerSeconds: 4.2}
	applyBaseline(&r, combineBaseline(idleSample{cores: 0.36}, idleSample{cores: 0.40}))
	if r.WindowServerSeconds != 4.2 {
		t.Fatalf("raw windowserver_s changed to %v", r.WindowServerSeconds)
	}
	if d := r.WindowServerIdleSeconds - 3.8; d > 1e-9 || d < -1e-9 {
		t.Fatalf("windowserver_idle_s = %v, want 3.8 (mean 0.38 cores × 10 s)", r.WindowServerIdleSeconds)
	}
	if d := r.WindowServerNetSeconds - 0.4; d > 1e-9 || d < -1e-9 {
		t.Fatalf("windowserver_net_s = %v, want 0.4", r.WindowServerNetSeconds)
	}
	if d := r.cpu() - 0.14; d > 1e-9 || d < -1e-9 {
		t.Fatalf("cpu() = %v, want (1 + 0.4) / 10", r.cpu())
	}
	if r.Disturbed {
		t.Fatal("a calm baseline marked the round disturbed")
	}
}

// The shared idle compositor load (0.38 cores in every round) must not
// decide the verdict: with the raw delta Tango's 0.05 + 0.38 is not half of
// the opponent's 0.30 + 0.38, with the net delta 0.05 is.
func TestJudgeUsesNetWS(t *testing.T) {
	mk := func(fw string, proc float64) []Round {
		return rounds(fw, 5, 0, 0, func(i int, r *Round) {
			r.Scene = "list"
			s := proc * (1 + 0.01*float64(i))
			r.CPUSeconds, r.WindowServerSeconds = s*10, 3.8+0.01*float64(i)
			applyBaseline(r, combineBaseline(idleSample{cores: 0.38}, idleSample{cores: 0.38}))
		})
	}
	tg, o := mk("tango", 0.05), mk("mygo", 0.30)
	v, why := judgeWhy(tg, o, 3)
	if v != Faster {
		t.Fatalf("verdict %s (%s), want Faster on the net WindowServer CPU", v, why)
	}
	if !strings.Contains(why, "ws_net") {
		t.Fatalf("verdict line %q does not say it uses proc + ws_net", why)
	}
	// Without a baseline (idle 0) the same raw numbers are NotFaster.
	for i := range tg {
		tg[i].WindowServerIdleSeconds, tg[i].WindowServerNetSeconds = 0, 0
		o[i].WindowServerIdleSeconds, o[i].WindowServerNetSeconds = 0, 0
	}
	if v, why := judgeWhy(tg, o, 3); v != NotFaster {
		t.Fatalf("raw verdict %s (%s), want NotFaster", v, why)
	}
}

func TestBaselineDisturbed(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after idleSample
		disturbed     bool
	}{
		{"calm", idleSample{cores: 0.35}, idleSample{cores: 0.40}, false},
		{"drift", idleSample{cores: 0.30}, idleSample{cores: 0.50}, true}, // 40% of the larger
		{"busy", idleSample{cores: 0.65}, idleSample{cores: 0.66}, true},  // above 0.6 cores
		{"tiny noise", idleSample{cores: 0.010}, idleSample{cores: 0.020}, false},
		{"failed", idleSample{err: errFake}, idleSample{cores: 0.3}, true},
	} {
		b := combineBaseline(tc.before, tc.after)
		if got := b.Disturbed != ""; got != tc.disturbed {
			t.Errorf("%s: disturbed %v (%q), want %v", tc.name, got, b.Disturbed, tc.disturbed)
		}
		r := Round{Seconds: 10, WindowServerSeconds: 5}
		applyBaseline(&r, b)
		if r.Disturbed != tc.disturbed {
			t.Errorf("%s: round disturbed %v, want %v", tc.name, r.Disturbed, tc.disturbed)
		}
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errFake = fakeErr("ps failed")

// fakeBench is a WindowServer clock, a wall clock and a launcher: a
// launched program runs (is a child) for the length of measure, and the
// compositor clock advances by idle cores while it sleeps.
type fakeBench struct {
	t        *testing.T
	g        groups
	now      time.Time
	ws       time.Duration
	idle     []float64 // cores for each idle sample, in turn
	idleRate float64
	samples  int
	log      []string
	measured map[string]int
	disturb  map[string]int // fw: attempts that come back load-disturbed
}

func (f *fakeBench) deps() roundDeps {
	return roundDeps{
		seconds:  10 * time.Second,
		children: func() int { f.g.mu.Lock(); defer f.g.mu.Unlock(); return len(f.g.pids) },
		now:      func() time.Time { return f.now },
		sleep: func(d time.Duration) {
			f.ws += time.Duration(f.idleRate * float64(d))
			f.now = f.now.Add(d)
		},
		wsClock: func() (time.Duration, error) {
			if n := len(f.g.pids); n > 0 {
				f.t.Errorf("WindowServer idle clock read with %d scene programs running", n)
			}
			if f.idleRate == 0 || f.samples%2 == 0 {
				// A sample's first read: pick this sample's rate.
				f.idleRate = f.idle[min(f.samples/2, len(f.idle)-1)]
			}
			f.samples++
			return f.ws, nil
		},
		measure: func(fw, scene string) Round {
			pid, _ := f.g.start(func() (int, error) { return 1000 + len(f.log), nil })
			defer f.g.remove(pid)
			f.log = append(f.log, "run "+fw)
			f.measured[fw]++
			r := Round{Scene: scene, Framework: fw, Ready: true, Renderer: "native", Seconds: 10, CPUSeconds: 1, WindowServerSeconds: 4.5}
			if f.disturb[fw] > 0 {
				f.disturb[fw]--
				r.Disturbed = true
			}
			return r
		},
	}
}

func newFakeBench(t *testing.T, idle ...float64) *fakeBench {
	return &fakeBench{t: t, now: time.Unix(1, 0), idle: idle, measured: map[string]int{}, disturb: map[string]int{}}
}

func TestIdleSampledWithNoChild(t *testing.T) {
	f := newFakeBench(t, 0.38, 0.38)
	d := f.deps()
	wsReads := 0
	inner := d.wsClock
	d.wsClock = func() (time.Duration, error) {
		wsReads++
		f.log = append(f.log, "ws")
		return inner()
	}
	var got []Round
	if err := runSceneRound(d, "list", 0, []string{"tango", "mygo", "nsgo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{"ws", "ws", "run tango", "run mygo", "run nsgo", "ws", "ws"}
	if strings.Join(f.log, ",") != strings.Join(want, ",") {
		t.Fatalf("order %v, want %v (one idle sample before the round's launches, one after)", f.log, want)
	}
	if len(got) != 3 {
		t.Fatalf("%d rounds, want 3", len(got))
	}
	for _, r := range got {
		if d := r.WindowServerIdleSeconds - 3.8; d > 1e-6 || d < -1e-6 {
			t.Fatalf("%s idle %v, want 3.8", r.Framework, r.WindowServerIdleSeconds)
		}
		if d := r.WindowServerNetSeconds - 0.7; d > 1e-6 || d < -1e-6 {
			t.Fatalf("%s net %v, want 0.7", r.Framework, r.WindowServerNetSeconds)
		}
		if r.Disturbed {
			t.Fatalf("%s disturbed on a calm baseline", r.Framework)
		}
	}
	// The idle sampler refuses while a scene program runs.
	pid, _ := f.g.start(func() (int, error) { return 42, nil })
	if s := sampleIdle(d); s.err == nil {
		t.Fatal("idle sample taken with a scene program running")
	}
	f.g.remove(pid)
}

// A drifting background compositor marks the whole round disturbed and the
// round is measured again (at most maxRetries more times); every attempt
// stays in the results.
func TestDriftRerunsRound(t *testing.T) {
	f := newFakeBench(t, 0.30, 0.50, 0.38, 0.38) // round 0: before 0.30, after 0.50; rerun calm
	var got []Round
	if err := runSceneRound(f.deps(), "list", 2, []string{"tango", "mygo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("%d attempts, want 4 (2 drifted + 2 calm)", len(got))
	}
	for i, r := range got {
		wantDisturbed := i < 2
		if r.Disturbed != wantDisturbed || r.Round != 2 {
			t.Fatalf("attempt %d: %s round %d disturbed %v, want round 2 disturbed %v", i, r.Framework, r.Round, r.Disturbed, wantDisturbed)
		}
		if r.Attempt != i/2 {
			t.Fatalf("attempt %d: %s attempt %d, want %d", i, r.Framework, r.Attempt, i/2)
		}
	}
	// Always busy (above the idle limit): 1 + maxRetries attempts, then give up.
	f = newFakeBench(t, 0.7)
	got = nil
	_ = runSceneRound(f.deps(), "list", 0, []string{"tango"}, func(r Round) error { got = append(got, r); return nil })
	if len(got) != 1+maxRetries {
		t.Fatalf("%d attempts on a busy compositor, want %d", len(got), 1+maxRetries)
	}
	// A load-disturbed attempt is still retried per framework inside the round.
	f = newFakeBench(t, 0.38)
	f.disturb["mygo"] = 1
	got = nil
	_ = runSceneRound(f.deps(), "list", 0, []string{"tango", "mygo"}, func(r Round) error { got = append(got, r); return nil })
	if len(got) != 3 || f.measured["mygo"] != 2 || got[2].Attempt != 1 {
		t.Fatalf("load retry: %d rounds, mygo measured %d", len(got), f.measured["mygo"])
	}
}

func TestScreenSharingMatch(t *testing.T) {
	ps := "/sbin/launchd\n/Applications/UURemote.app/Contents/MacOS/UURemoteServer\n/System/Library/CoreServices/RemoteManagement/screensharingd.bundle/Contents/MacOS/screensharingd\n/usr/bin/zsh\n/Applications/TeamViewer.app/Contents/MacOS/TeamViewer\n/Applications/TeamViewer.app/Contents/MacOS/TeamViewer\n"
	got := matchScreenSharing(ps)
	want := "TeamViewer,UURemoteServer,screensharingd"
	if strings.Join(got, ",") != want {
		t.Fatalf("matched %v, want %s", got, want)
	}
}
