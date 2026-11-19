package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeFront is a desktop whose frontmost process is fronts[i] after the
// i-th raise (the last one repeats).
type fakeFront struct {
	fronts   []int
	raiseErr error
	raises   int
	slept    time.Duration
}

func (f *fakeFront) probe() frontProbe {
	return frontProbe{
		raise: func(int) error { f.raises++; return f.raiseErr },
		front: func() (int, error) {
			if len(f.fronts) == 0 {
				return 0, errors.New("no front")
			}
			return f.fronts[min(f.raises, len(f.fronts))-1], nil
		},
		sleep: func(d time.Duration) { f.slept += d },
	}
}

func TestRaiseVerified(t *testing.T) {
	f := &fakeFront{fronts: []int{42}}
	if ok, notes := raiseVerified(f.probe(), 42); !ok || f.raises != 1 || len(notes) != 0 {
		t.Fatalf("frontmost at once: ok %v raises %d notes %v", ok, f.raises, notes)
	}
	// Another app stays in front twice, then the scene comes up.
	f = &fakeFront{fronts: []int{7, 7, 42}}
	ok, notes := raiseVerified(f.probe(), 42)
	if !ok || f.raises != 3 || f.slept != 2*frontRetryWait || !reflect.DeepEqual(notes, []string{"frontmost after 3 raises"}) {
		t.Fatalf("third raise: ok %v raises %d slept %v notes %v", ok, f.raises, f.slept, notes)
	}
	// Never in front: the first raise and frontRetries more, then unverified.
	f = &fakeFront{fronts: []int{7}}
	ok, notes = raiseVerified(f.probe(), 42)
	if ok || f.raises != 1+frontRetries || len(notes) != 1 || !strings.Contains(notes[0], "frontmost pid 7, not the scene's 42") {
		t.Fatalf("never in front: ok %v raises %d notes %v", ok, f.raises, notes)
	}
	// A failing raise or check is unverified too, never verified.
	f = &fakeFront{fronts: []int{42}, raiseErr: errors.New("not allowed")}
	if ok, notes = raiseVerified(f.probe(), 42); ok || f.raises != 1+frontRetries || !strings.Contains(notes[0], "raise: not allowed") {
		t.Fatalf("raise fails: ok %v raises %d notes %v", ok, f.raises, notes)
	}
	f = &fakeFront{}
	if ok, notes = raiseVerified(f.probe(), 42); ok || !strings.Contains(notes[0], "frontmost check: no front") {
		t.Fatalf("check fails: ok %v notes %v", ok, notes)
	}
}

func TestParseLsappinfoPID(t *testing.T) {
	if pid, err := parseLsappinfoPID(`"pid"=1234` + "\n"); err != nil || pid != 1234 {
		t.Fatalf("pid %d err %v", pid, err)
	}
	for _, s := range []string{"", `"pid"=`, `"pid"=x`, `"pid"=0`} {
		if _, err := parseLsappinfoPID(s); err == nil {
			t.Errorf("parseLsappinfoPID(%q) accepted", s)
		}
	}
}

func TestCheckDrawing(t *testing.T) {
	defer func(m map[string]sceneMeta) { sceneMetas = m }(sceneMetas)
	sceneMetas = map[string]sceneMeta{"dialog": {period: 30}, "sparse": {period: 30, sparseFrames: true}}
	for _, tc := range []struct {
		scene string
		fps   float64
		ready bool
		err   string
		want  bool
	}{
		{"list", 0, true, "", true},            // occluded MyGo
		{"list", 29.9, true, "", true},         // below half of 60
		{"list", 30, true, "", false},          // half of 60
		{"list", 60, true, "", false},          // drawing
		{"dialog", 0, true, "", true},          // periodic scenes still draw every frame
		{"counter", 0, true, "", false},        // expected fps 0
		{"sparse", 2, true, "", false},         // frame rate defined by the protocol
		{"list", 0, false, "not ready", false}, // already unusable, not retried as occluded
		{"list", 0, true, "exited", false},
	} {
		r := Round{Scene: tc.scene, FPS: tc.fps, Ready: tc.ready, Error: tc.err}
		checkDrawing(&r)
		checkDrawing(&r) // the note once
		noted := reflect.DeepEqual(r.Notes, []string{noteNotDrawing})
		if r.Disturbed != tc.want || noted != tc.want {
			t.Errorf("%s fps %v ready %v err %q: disturbed %v notes %v, want %v", tc.scene, tc.fps, tc.ready, tc.err, r.Disturbed, r.Notes, tc.want)
		}
	}
}

// An attempt that drew nothing (an occluded MyGo) is disturbed and
// measured again by the existing per-framework retries; counter is not.
func TestNotDrawingRerunsAttempt(t *testing.T) {
	f := newFakeBench(t, 0.38)
	f.stalled["mygo"] = 2
	var got []Round
	if err := runSceneRound(f.deps(), "list", 0, []string{"tango", "mygo"}, func(r Round) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || f.measured["mygo"] != 3 || f.measured["tango"] != 1 {
		t.Fatalf("%d attempts, mygo measured %d, tango %d; want 4, 3, 1", len(got), f.measured["mygo"], f.measured["tango"])
	}
	for i, r := range got[1:] {
		wantDisturbed := i < 2
		if r.Framework != "mygo" || r.Disturbed != wantDisturbed || r.Attempt != i || contains(r.Notes, noteNotDrawing) != wantDisturbed {
			t.Fatalf("mygo attempt %d: %+v, want disturbed %v", i, r, wantDisturbed)
		}
	}
	if len(usable(got)) != 2 {
		t.Fatalf("usable %d, want tango's and mygo's third attempt", len(usable(got)))
	}
	f = newFakeBench(t, 0.38)
	f.stalled["mygo"] = 1
	got = nil
	_ = runSceneRound(f.deps(), "counter", 0, []string{"mygo"}, func(r Round) error { got = append(got, r); return nil })
	if len(got) != 1 || got[0].Disturbed {
		t.Fatalf("counter at 0 fps: %d attempts, disturbed %v; want 1 calm", len(got), got[0].Disturbed)
	}
}

func TestScenesFromInvalid(t *testing.T) {
	dir := t.TempDir()
	res := &Results{Verdicts: []VerdictLine{
		{Scene: "counter", Opponent: "mygo", Verdict: Info},
		{Scene: "list", Opponent: "mygo", Verdict: Invalid},
		{Scene: "list", Opponent: "nsgo", Verdict: Invalid},
		{Scene: "grid", Opponent: "mygo", Verdict: NotFaster},
		{Scene: "grid", Opponent: "nsgo", Verdict: Invalid}, // one opponent is enough
		{Scene: "tree", Opponent: "nsgo", Verdict: Faster},
	}}
	path := filepath.Join(dir, "r.json")
	if err := write(path, res); err != nil {
		t.Fatal(err)
	}
	got, err := scenesFromInvalid(path)
	if err != nil || !reflect.DeepEqual(got, []string{"list", "grid"}) {
		t.Fatalf("scenes %v err %v, want [list grid]", got, err)
	}
	if err := write(path, &Results{}); err != nil {
		t.Fatal(err)
	}
	if _, err := scenesFromInvalid(path); err == nil {
		t.Fatal("a results file without verdicts accepted")
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scenesFromInvalid(path); err == nil {
		t.Fatal("broken JSON accepted")
	}
}
