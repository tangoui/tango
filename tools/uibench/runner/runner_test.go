package main

import (
	"errors"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestDisturbedRoundsDropped(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	// Two disturbed Tango rounds were dear enough to lose the comparison
	// if they counted.
	tg := rounds("tango", 5, 0.4, 0.1, func(i int, r *Round) {
		if i == 1 || i == 3 {
			r.Disturbed = true
			r.CPUSeconds, r.WindowServerSeconds = 30, 3
		}
	})
	if got := usable(tg); len(got) != 3 {
		t.Fatalf("usable kept %d rounds, want 3", len(got))
	}
	for _, r := range usable(tg) {
		if r.Disturbed {
			t.Fatalf("usable kept disturbed round %d", r.Round)
		}
	}
	if v := judge(tg, o, 3); v != Faster {
		_, why := judgeWhy(tg, o, 3)
		t.Fatalf("verdict %s (%s), want Faster from the 3 calm rounds", v, why)
	}
	// Failed rounds and rounds behind a locked screen do not count either.
	tg[0].Error = "not ready"
	if got := usable(tg); len(got) != 2 {
		t.Fatalf("usable kept %d rounds after a failure, want 2", len(got))
	}
	tg[2].ScreenLocked = true
	if got := usable(tg); len(got) != 1 {
		t.Fatalf("usable kept %d rounds after a locked screen, want 1", len(got))
	}
}

func TestParseProtocol(t *testing.T) {
	for _, tc := range []struct {
		line string
		want event
		ok   bool
	}{
		{"bench ready", event{kind: "ready"}, true},
		{"bench renderer native", event{kind: "renderer", name: "native"}, true},
		{"bench frames 119.8", event{kind: "frames", value: 119.8}, true},
		{"bench work 71880.0", event{kind: "work", value: 71880}, true},
		{"bench intervals_us 8333,8334,16667", event{kind: "intervals", intervals: []int64{8333, 8334, 16667}}, true},
		{"bench intervals_us ", event{kind: "intervals"}, true},
		{"bench frames fast", event{}, false},
		{"bench intervals_us 1,x", event{}, false},
		{"2026/11/17 log line from the framework", event{}, false},
		{"bench unknown 1", event{}, false},
	} {
		got, ok := parseLine(tc.line)
		if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("parseLine(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRotate(t *testing.T) {
	fws := []string{"tango", "mygo", "nsgo"}
	want := [][]string{{"tango", "mygo", "nsgo"}, {"mygo", "nsgo", "tango"}, {"nsgo", "tango", "mygo"}, {"tango", "mygo", "nsgo"}}
	for round, w := range want {
		if got := rotate(fws, round); !reflect.DeepEqual(got, w) {
			t.Errorf("round %d: %v, want %v", round, got, w)
		}
	}
}

func TestLoadGate(t *testing.T) {
	loads := []float64{7.5, 6.0, 5.9}
	slept := 0
	read := func() (float64, error) { v := loads[0]; loads = loads[1:]; return v, nil }
	if err := loadGate(read, func(time.Duration) { slept++ }, 6, time.Hour); err != nil || slept != 2 {
		t.Fatalf("gate: err %v after %d sleeps, want nil after 2", err, slept)
	}
	high := func() (float64, error) { return 9, nil }
	if err := loadGate(high, func(time.Duration) {}, 6, time.Minute); err == nil {
		t.Fatal("gate passed a load that stayed high")
	}
	if v, err := parseLoadavg("{ 1.40 1.52 1.61 }\n"); err != nil || v != 1.40 {
		t.Fatalf("parseLoadavg = %v, %v", v, err)
	}
}

func TestParsePSTime(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"  0:01.50\n":   1500 * time.Millisecond,
		"12:34.56":      12*time.Minute + 34560*time.Millisecond,
		"1:02:03.00":    time.Hour + 2*time.Minute + 3*time.Second,
		"2-01:00:00.00": 49 * time.Hour,
	} {
		got, err := parsePSTime(in)
		if err != nil || (got-want).Abs() > time.Millisecond {
			t.Errorf("parsePSTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func TestCheckScreen(t *testing.T) {
	if err := checkScreen(func() bool { return true }); err != errScreenLocked {
		t.Fatalf("locked screen: err %v, want errScreenLocked", err)
	}
	if err := checkScreen(func() bool { return false }); err != nil {
		t.Fatalf("unlocked screen: err %v, want nil", err)
	}
}

func TestMonitorDropsFirstGroup(t *testing.T) {
	m := newMonitor()
	in := "bench ready\nbench frames 1\nbench work 1\nbench intervals_us 1\n" + // before sampling
		"bench frames 50\n" // a group that straddles the start
	rest := "bench work 50\nbench intervals_us 20000\n" +
		"bench frames 60\nbench work 600\nbench intervals_us 16000,17000\n" +
		"bench frames 62\nbench work 620\nbench intervals_us 16100\n"
	m.feed(in)
	m.setSampling(true)
	m.feed(rest)
	if !reflect.DeepEqual(m.frames, []float64{60, 62}) || !reflect.DeepEqual(m.work, []float64{600, 620}) ||
		!reflect.DeepEqual(m.intervals, []float64{16000, 17000, 16100}) {
		t.Fatalf("sampled frames %v work %v intervals %v", m.frames, m.work, m.intervals)
	}
}

func TestStopAllKillsGroups(t *testing.T) {
	var g groups
	for _, pid := range []int{101, 202} {
		if _, err := g.start(func() (int, error) { return pid, nil }); err != nil {
			t.Fatal(err)
		}
	}
	g.remove(101)
	var killed []int
	g.stopAll(func(pid int, sig syscall.Signal) error {
		if sig == syscall.SIGTERM {
			killed = append(killed, pid)
		}
		return nil
	})
	if !reflect.DeepEqual(killed, []int{-202}) {
		t.Fatalf("stopAll signalled %v, want the group of 202 only", killed)
	}
}

// After stopAll, no program starts: start refuses without calling the
// start function (it runs under the same lock as stopAll, so a child can
// never start between stopAll's check and its kill), and nothing is tracked.
func TestStopAllRefusesLateStarts(t *testing.T) {
	var g groups
	if _, err := g.start(func() (int, error) { return 101, nil }); err != nil {
		t.Fatalf("start before stopAll: %v", err)
	}
	g.stopAll(func(int, syscall.Signal) error { return nil })
	called := false
	if _, err := g.start(func() (int, error) { called = true; return 303, nil }); err != errStopping {
		t.Fatalf("start after stopAll: err %v, want errStopping", err)
	}
	if called {
		t.Fatal("start after stopAll ran the program")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.pids) != 0 {
		t.Fatalf("groups left after stopAll: %v", g.pids)
	}
}

// A start function that fails is not tracked.
func TestStartFailureNotTracked(t *testing.T) {
	var g groups
	if _, err := g.start(func() (int, error) { return 0, errors.New("no such file") }); err == nil {
		t.Fatal("failed start reported no error")
	}
	if len(g.pids) != 0 {
		t.Fatalf("failed start tracked: %v", g.pids)
	}
}
