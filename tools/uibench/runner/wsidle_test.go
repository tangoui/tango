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
	applyBaseline(&r, combineBaseline(idleSample{cores: 0.37}, idleSample{cores: 0.39}))
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
		{"calm", idleSample{cores: 0.37}, idleSample{cores: 0.40}, false},
		{"drift above 10%", idleSample{cores: 0.35}, idleSample{cores: 0.40}, true}, // 0.05 > max(0.02, 0.04)
		{"floor", idleSample{cores: 0.10}, idleSample{cores: 0.115}, false},         // 0.015 < 0.02
		{"drift", idleSample{cores: 0.30}, idleSample{cores: 0.50}, true},
		{"busy", idleSample{cores: 0.65}, idleSample{cores: 0.66}, true}, // above 0.6 cores
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
	gates    int
	wsErr    map[int]error // ws read number: error
	log      []string
	measured map[string]int
	disturb  map[string]int // fw: attempts that come back load-disturbed
}

func (f *fakeBench) deps() roundDeps {
	return roundDeps{
		seconds:  10 * time.Second,
		settle:   3 * time.Second,
		gate:     func() { f.gates++ },
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
			n := f.samples
			f.samples++
			if err := f.wsErr[n]; err != nil {
				return 0, err
			}
			return f.ws, nil
		},
		measure: func(fw, scene string) Round {
			pid, _ := f.g.start(func() (int, error) { return 1000 + len(f.log), nil })
			defer f.g.remove(pid)
			f.log = append(f.log, "run "+fw)
			f.measured[fw]++
			r := Round{Scene: scene, Framework: fw, Ready: true, Renderer: "native", Seconds: 10, CPUSeconds: 1, WindowServerSeconds: 4.5}
			f.now = f.now.Add(4 * time.Second) // start, ready, settle
			r.SampleStartNs = f.now.UnixNano()
			f.now = f.now.Add(10 * time.Second)
			r.SampleEndNs = f.now.UnixNano()
			if f.disturb[fw] > 0 {
				f.disturb[fw]--
				r.Disturbed = true
			}
			return r
		},
	}
}

func newFakeBench(t *testing.T, idle ...float64) *fakeBench {
	return &fakeBench{t: t, now: time.Unix(1, 0), idle: idle, measured: map[string]int{}, disturb: map[string]int{}, wsErr: map[int]error{}}
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
	innerSleep := d.sleep
	d.sleep = func(dur time.Duration) {
		f.log = append(f.log, "sleep "+dur.String())
		innerSleep(dur)
	}
	d.gate = func() { f.log = append(f.log, "gate") }
	var got []Round
	if err := runSceneRound(d, "list", 0, []string{"tango", "mygo", "nsgo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	// Load gate, settle, idle sample; the launches; settle, idle sample.
	want := []string{"gate", "sleep 3s", "ws", "sleep 10s", "ws", "run tango", "run mygo", "run nsgo", "sleep 3s", "ws", "sleep 10s", "ws"}
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
	if f.gates != 2 {
		t.Fatalf("load gate ran %d times, want once per attempt of the round (2)", f.gates)
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

// Each attempt's idle rate is before and after interpolated at its sample
// midpoint.
func TestIdleInterpolated(t *testing.T) {
	b := combineBaseline(idleSample{cores: 0.36, midNs: 1000}, idleSample{cores: 0.39, midNs: 2000})
	for _, tc := range []struct {
		start, end int64
		want       float64
	}{
		{1250, 1350, 0.369}, // midpoint 1300: 30% of the way
		{500, 700, 0.36},    // before the first sample: clamped
		{0, 0, 0.375},       // no sample window: the mean
	} {
		r := Round{Seconds: 10, WindowServerSeconds: 5, SampleStartNs: tc.start, SampleEndNs: tc.end}
		applyBaseline(&r, b)
		if d := r.WSBaseline.Cores - tc.want; d > 1e-9 || d < -1e-9 {
			t.Errorf("window %d..%d: idle %v cores, want %v", tc.start, tc.end, r.WSBaseline.Cores, tc.want)
		}
		if d := r.WindowServerIdleSeconds - tc.want*10; d > 1e-9 || d < -1e-9 {
			t.Errorf("window %d..%d: idle_s %v, want %v", tc.start, tc.end, r.WindowServerIdleSeconds, tc.want*10)
		}
	}
	// In a round: later attempts get a larger idle on a rising compositor.
	f := newFakeBench(t, 0.36, 0.38)
	var got []Round
	_ = runSceneRound(f.deps(), "list", 0, []string{"tango", "mygo", "nsgo"}, func(r Round) error { got = append(got, r); return nil })
	if len(got) != 3 || !(got[0].WindowServerIdleSeconds < got[1].WindowServerIdleSeconds && got[1].WindowServerIdleSeconds < got[2].WindowServerIdleSeconds) {
		t.Fatalf("idle not rising across the round: %+v", got)
	}
	if got[0].Disturbed {
		t.Fatalf("calm round disturbed: %v", got[0].Notes)
	}
}

// A failed idle sample disturbs the whole round, which is measured again
// after the load gate; the attempts of the failed round stay, with a note.
func TestIdleFailureRerunsRound(t *testing.T) {
	f := newFakeBench(t, 0.38)
	f.wsErr[0] = errFake // the round's first idle read
	var got []Round
	if err := runSceneRound(f.deps(), "list", 1, []string{"tango", "mygo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || f.gates != 2 {
		t.Fatalf("%d attempts, %d gates; want 4 and 2", len(got), f.gates)
	}
	for i, r := range got {
		if i < 2 {
			if !r.Disturbed || len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "before: ps failed") {
				t.Fatalf("attempt %d: disturbed %v notes %v, want disturbed by the failed sample", i, r.Disturbed, r.Notes)
			}
			continue
		}
		if r.Disturbed || r.Attempt != 1 {
			t.Fatalf("rerun attempt %d: disturbed %v attempt %d", i, r.Disturbed, r.Attempt)
		}
	}
}

// usableIdle is a calm, usable round with idle compositor cores.
func usableIdle(cores float64) Round {
	return Round{Ready: true, Renderer: "native", WSBaseline: &WSBaseline{MeanCores: cores}}
}

// R46: adoptability follows the measured idle compositor only.
func TestCompositorBusyByIdleOnly(t *testing.T) {
	busy := []Round{usableIdle(0.38), usableIdle(0.36), usableIdle(0.40)}
	m := Meta{Adoptable: true}
	got := assessCompositor(&m, "list", busy, false)
	if !strings.Contains(got, "compositor busy: idle WindowServer 0.380 cores") || m.Adoptable || len(m.NotAdoptable) != 1 || m.NotAdoptable[0] != "compositor busy" {
		t.Fatalf("busy idle: %q, adoptable %v %v", got, m.Adoptable, m.NotAdoptable)
	}
	assessCompositor(&m, "grid", busy, false)
	if len(m.NotAdoptable) != 1 {
		t.Fatalf("reason recorded twice: %v", m.NotAdoptable)
	}
	calm := []Round{usableIdle(0.02), usableIdle(0.03), usableIdle(0.05)}
	m = Meta{Adoptable: true}
	if got := assessCompositor(&m, "list", calm, false); got != "" || !m.Adoptable {
		t.Fatalf("calm: %q, adoptable %v", got, m.Adoptable)
	}
}

// A screen-share daemon left running with nobody connected is a note, not
// a busy compositor.
func TestScreenShareNoteWhenIdleLow(t *testing.T) {
	calm := []Round{usableIdle(0.02), usableIdle(0.03), usableIdle(0.04)}
	m := Meta{Adoptable: true, ScreenSharing: []string{"UURemoteServer"}, ScreenSharingFinish: []string{"UURemoteServer", "AnyDesk"}}
	got := assessCompositor(&m, "list", calm, false)
	if got != " (screen-share app running: UURemoteServer, AnyDesk, idle 0.030 cores)" {
		t.Fatalf("note %q", got)
	}
	if !m.Adoptable || len(m.NotAdoptable) != 0 {
		t.Fatalf("a screen-share process alone made the run not adoptable: %v", m.NotAdoptable)
	}
	if len(m.CompositorNotes) != 1 || !strings.HasPrefix(m.CompositorNotes[0], "list: screen-share app running") {
		t.Fatalf("meta notes %v", m.CompositorNotes)
	}
	// ps failed: the state is unknown, still adoptable.
	m = Meta{Adoptable: true, ScreenSharingFinishError: "ps: exit 1"}
	if got := assessCompositor(&m, "list", calm, false); !strings.Contains(got, "screen-share state unknown") || !m.Adoptable {
		t.Fatalf("unknown state: %q adoptable %v", got, m.Adoptable)
	}
}

// Disturbed rounds (a failed sample leaves a 0 baseline) do not pull the
// median down.
func TestSceneIdleExcludesDisturbed(t *testing.T) {
	rs := []Round{usableIdle(0.30), usableIdle(0.32), usableIdle(0.34)}
	for range 4 {
		r := usableIdle(0)
		r.Disturbed = true
		rs = append(rs, r)
	}
	if v, n := sceneIdle(rs); n != 3 || v != 0.32 {
		t.Fatalf("median %v over %d rounds, want 0.32 over 3", v, n)
	}
	m := Meta{Adoptable: true}
	if assessCompositor(&m, "list", rs, false); m.Adoptable {
		t.Fatal("disturbed zero baselines hid a busy compositor")
	}
}

func TestCheckIdle(t *testing.T) {
	for _, tc := range []struct {
		cores float64
		pass  bool
	}{{0.04, true}, {0.10, true}, {0.38, false}} {
		f := newFakeBench(t, tc.cores)
		var out strings.Builder
		got := checkIdle(f.deps(), []string{"UURemoteServer"}, nil, &out)
		if got != tc.pass {
			t.Fatalf("idle %v: pass %v, want %v\n%s", tc.cores, got, tc.pass, out.String())
		}
		want := "FAIL"
		if tc.pass {
			want = "PASS"
		}
		for _, part := range []string{want, "threshold 0.10", "UURemoteServer"} {
			if !strings.Contains(out.String(), part) {
				t.Fatalf("idle %v: output lacks %q:\n%s", tc.cores, part, out.String())
			}
		}
		if f.measured["tango"]+f.measured["mygo"]+f.measured["nsgo"] != 0 || f.samples != 2 {
			t.Fatalf("check-idle launched a program or read the clock %d times", f.samples)
		}
	}
	// A failed sample fails the check; an unknown screen-share state is said.
	f := newFakeBench(t, 0.01)
	f.wsErr[0] = errFake
	var out strings.Builder
	if checkIdle(f.deps(), nil, errFake, &out) || !strings.Contains(out.String(), "unknown") {
		t.Fatalf("failed sample passed:\n%s", out.String())
	}
}

// R49: with -accept-busy-compositor a busy idle compositor is a note, the
// run stays adoptable, and the scene's idle median is recorded.
func TestAcceptBusyCompositor(t *testing.T) {
	busy := []Round{usableIdle(0.38), usableIdle(0.36), usableIdle(0.40)}
	m := Meta{Adoptable: true, AcceptedBusyCompositor: true}
	got := assessCompositor(&m, "list", busy, true)
	if got != " (normal desktop, idle WindowServer 0.380 cores, accepted by user)" {
		t.Fatalf("note %q", got)
	}
	if !m.Adoptable || len(m.NotAdoptable) != 0 {
		t.Fatalf("accepted busy compositor made the run not adoptable: %v", m.NotAdoptable)
	}
	if m.SceneIdleCores["list"] != 0.38 || len(m.CompositorNotes) != 1 {
		t.Fatalf("meta idle %v notes %v", m.SceneIdleCores, m.CompositorNotes)
	}
	// Without the flag: unchanged, not adoptable.
	m = Meta{Adoptable: true}
	if assessCompositor(&m, "list", busy, false); m.Adoptable {
		t.Fatal("busy compositor adoptable without the flag")
	}
	// The flag does not touch the drift guard or the round cap.
	if b := combineBaseline(idleSample{cores: 0.30}, idleSample{cores: 0.50}); b.Disturbed == "" {
		t.Fatal("drift not disturbed")
	}
	if b := combineBaseline(idleSample{cores: 0.65}, idleSample{cores: 0.65}); b.Disturbed == "" {
		t.Fatal("0.65 cores not disturbed")
	}
}

// Apple's CoreParsec daemons are not the Parsec remote-desktop app.
func TestScreenSharingIgnoresCoreParsec(t *testing.T) {
	ps := "/System/Library/PrivateFrameworks/CoreParsec.framework/parsecd\n" +
		"/System/Library/PrivateFrameworks/CoreParsec.framework/Versions/A/parsec-fbf\n"
	if got := matchScreenSharing(ps); len(got) != 0 {
		t.Fatalf("CoreParsec matched: %v", got)
	}
	ps += "/Applications/Parsec.app/Contents/MacOS/parsecd\n"
	if got := matchScreenSharing(ps); strings.Join(got, ",") != "parsecd" {
		t.Fatalf("Parsec app: %v, want parsecd", got)
	}
}

// R50: -cpu-metric process judges on process CPU only, takes no idle
// samples, and WindowServer never disturbs a round.
func TestProcessMetric(t *testing.T) {
	// Tango 0.10 process but a heavy compositor; opponent 0.30 process.
	mk := func(fw string, proc, ws float64, metric string) []Round {
		return rounds(fw, 5, 0, 0, func(i int, r *Round) {
			r.Scene, r.CPUMetric = "list", metric
			k := 1 + 0.01*float64(i)
			r.CPUSeconds, r.WindowServerSeconds = proc*10*k, ws*10*k
		})
	}
	v, why := judgeWhy(mk("tango", 0.10, 0.5, metricProcess), mk("mygo", 0.30, 0.0, metricProcess), 3)
	if v != Faster || !strings.Contains(why, "process only") {
		t.Fatalf("process metric: %s (%s), want Faster on process CPU", v, why)
	}
	// Default metric: the same rounds count WindowServer, NotFaster.
	if v, why := judgeWhy(mk("tango", 0.10, 0.5, ""), mk("mygo", 0.30, 0.0, ""), 3); v != NotFaster || !strings.Contains(why, "proc + ws_net") {
		t.Fatalf("default metric: %s (%s), want NotFaster", v, why)
	}
	// The cheap gate reads process CPU: 0.04 process + 1 core WS is too cheap.
	if v, why := judgeWhy(mk("tango", 0.01, 0, metricProcess), mk("mygo", 0.04, 1, metricProcess), 3); v != Info {
		t.Fatalf("cheap gate on process: %s (%s), want Info", v, why)
	}

	// A drifting, busy compositor (0.2 → 0.7) does not disturb anything and
	// is never sampled; the raw WindowServer delta stays.
	f := newFakeBench(t, 0.2, 0.7)
	d := f.deps()
	d.processOnly = true
	var got []Round
	if err := runSceneRound(d, "list", 0, []string{"tango", "mygo", "nsgo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if f.samples != 0 || len(got) != 3 || f.gates != 1 {
		t.Fatalf("process mode: %d WS idle reads, %d attempts, %d gates; want 0, 3, 1", f.samples, len(got), f.gates)
	}
	for _, r := range got {
		if r.Disturbed || r.WSBaseline != nil || r.WindowServerSeconds != 4.5 || r.CPUMetric != metricProcess {
			t.Fatalf("process mode round %+v", r)
		}
	}
	// Load ≥ 6 still disturbs and retries.
	f = newFakeBench(t, 0.2)
	f.disturb["mygo"] = 1
	d = f.deps()
	d.processOnly = true
	got = nil
	_ = runSceneRound(d, "list", 0, []string{"tango", "mygo"}, func(r Round) error { got = append(got, r); return nil })
	if len(got) != 3 || !got[1].Disturbed || got[2].Disturbed {
		t.Fatalf("load retry in process mode: %d attempts", len(got))
	}
}
