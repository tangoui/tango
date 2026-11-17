package main

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseRendererLines(t *testing.T) {
	for _, tc := range []struct {
		line string
		want event
		ok   bool
	}{
		// The timestamped line: name and the program's clock in Unix ns.
		{"bench renderer native 1763337600123456789", event{kind: "renderer", name: "native", t: 1763337600123456789}, true},
		// The old line, without a time, still parses (t 0: the runner's clock is used).
		{"bench renderer native", event{kind: "renderer", name: "native"}, true},
		{"bench renderer gpu x", event{}, false},
		{"bench renderer", event{}, false},
		{"bench scene list 800x600", event{kind: "scene", name: "list", window: "800x600"}, true},
		{"bench scene list", event{}, false},
	} {
		got, ok := parseLine(tc.line)
		if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("parseLine(%q) = %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

// The monitor keeps every renderer line as a timeline; an untimed line
// gets the runner's receive time and says so.
func TestMonitorRendererTimeline(t *testing.T) {
	m := newMonitor()
	m.now = func() time.Time { return time.Unix(0, 500) }
	m.feed("bench scene grid 800x600\nbench renderer cpu\nbench ready\nbench renderer cpu 1000\nbench renderer gpu 2000\n")
	want := []RendererSample{{T: 500, Name: "cpu", RunnerClock: true}, {T: 1000, Name: "cpu"}, {T: 2000, Name: "gpu"}}
	if !reflect.DeepEqual(m.timeline, want) {
		t.Fatalf("timeline %+v, want %+v", m.timeline, want)
	}
	if m.window != "800x600" || m.sceneName != "grid" {
		t.Fatalf("scene %q window %q", m.sceneName, m.window)
	}
}

func TestSummarizeRenderer(t *testing.T) {
	s := func(t int64, name string) RendererSample { return RendererSample{T: t, Name: name} }
	const t0, t1 = 100, 200
	for _, tc := range []struct {
		name     string
		timeline []RendererSample
		summary  string
		seq      []string
		switches int // all
		inSample int
	}{
		{"none", nil, "", nil, 0, 0},
		{"steady", []RendererSample{s(10, "native"), s(110, "native"), s(150, "native"), s(200, "native")}, "native", []string{"native"}, 0, 0},
		// Switched while settling: the window saw one path.
		{"switch before", []RendererSample{s(10, "native"), s(50, "gpu"), s(150, "gpu")}, "gpu", []string{"gpu"}, 1, 0},
		{"switch inside", []RendererSample{s(10, "native"), s(110, "native"), s(150, "gpu"), s(190, "gpu")}, "mixed", []string{"native", "gpu"}, 1, 1},
		{"switch after", []RendererSample{s(10, "native"), s(150, "native"), s(250, "cpu")}, "native", []string{"native"}, 1, 0},
		{"there and back", []RendererSample{s(10, "native"), s(120, "gpu"), s(180, "native")}, "mixed", []string{"native", "gpu", "native"}, 2, 2},
		// No sample before the window: the window starts with its first sample.
		{"late first", []RendererSample{s(150, "cpu")}, "cpu", []string{"cpu"}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary, seq, switches := summarizeRenderer(tc.timeline, t0, t1)
			in := 0
			for _, sw := range switches {
				if sw.InSample {
					in++
				}
			}
			if summary != tc.summary || !reflect.DeepEqual(seq, tc.seq) || len(switches) != tc.switches || in != tc.inSample {
				t.Fatalf("got %q %v %+v; want %q %v, %d switches (%d in sample)", summary, seq, switches, tc.summary, tc.seq, tc.switches, tc.inSample)
			}
		})
	}
}

// mixed makes a round a mixed one with the given in-sample sequence.
func mixed(seq ...string) func(int, *Round) {
	return func(_ int, r *Round) {
		r.Renderer, r.RendererSequence, r.SampleSwitches = "mixed", seq, len(seq)-1
	}
}

// A mixed round is usable and judged, but never as a single path: the
// verdict names it mixed, with its switches.
func TestJudgeMixedRoundsUsable(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, mixed("native", "gpu"))
	if got := usable(tg); len(got) != 5 {
		t.Fatalf("usable kept %d mixed rounds, want 5", len(got))
	}
	v, why := judgeWhy(tg, o, 5)
	if v != Faster {
		t.Fatalf("verdict %s (%s), want Faster", v, why)
	}
	if !strings.Contains(why, "mixed[native>gpu]") || !strings.Contains(why, "5 switches") {
		t.Fatalf("why %q: want the mixed path and its switches", why)
	}
	if strings.Contains(why, "tango native") {
		t.Fatalf("why %q labels a mixed side as native", why)
	}
	key, sw := rendererSummary(tg)
	if key != "mixed[native>gpu]" || sw != 5 {
		t.Fatalf("rendererSummary = %q, %d", key, sw)
	}
}

// Consistency across rounds uses the per-round summary: a mixed round
// matches only mixed rounds with the same sequence.
func TestJudgeMixedConsistency(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	for _, tc := range []struct {
		name string
		mod  func(int, *Round)
		want Verdict
	}{
		{"same sequence", mixed("native", "gpu"), Faster},
		{"one other sequence", func(i int, r *Round) {
			mixed("native", "gpu")(i, r)
			if i == 3 {
				mixed("native", "cpu")(i, r)
			}
		}, Invalid},
		{"mixed and single", func(i int, r *Round) {
			if i == 1 {
				mixed("native", "gpu")(i, r)
			}
		}, Invalid},
		{"reverse sequence", func(i int, r *Round) {
			mixed("native", "gpu")(i, r)
			if i == 0 {
				mixed("gpu", "native")(i, r)
			}
		}, Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tg := rounds("tango", 5, 0.4, 0.1, tc.mod)
			if v, why := judgeWhy(tg, o, 5); v != tc.want {
				t.Fatalf("verdict %s (%s), want %s", v, why, tc.want)
			}
		})
	}
}

// The runner clears every inherited TANGO_* variable from every scene
// program's environment: Tango runs its default renderer policy and no
// Tango-only diagnostics (TANGO_FRAME_TRACE adds per-frame cost).
func TestChildEnvClearsTangoRenderer(t *testing.T) {
	env, cleared := childEnv([]string{"HOME=/h", "TANGO_RENDERER=gpu", "PATH=/bin", "TANGO_FRAME_TRACE=1", "MY_TANGO_X=1", "TANGO_RENDERER=cpu"})
	if !reflect.DeepEqual(env, []string{"HOME=/h", "PATH=/bin", "MY_TANGO_X=1"}) || !reflect.DeepEqual(cleared, []string{"TANGO_RENDERER=gpu", "TANGO_FRAME_TRACE=1", "TANGO_RENDERER=cpu"}) {
		t.Fatalf("childEnv = %v, cleared %v", env, cleared)
	}
	t.Setenv("TANGO_RENDERER", "gpu")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	cmd := sceneCommand(config{bin: "/b"}, "tango", "list")
	if cmd.Env == nil {
		t.Fatal("scene command inherits the runner's environment as is")
	}
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "TANGO_") {
			t.Fatalf("scene command env has %s", kv)
		}
	}
	if !reflect.DeepEqual(cmd.Args, []string{"/b/tango", "-scene", "list"}) {
		t.Fatalf("args %v", cmd.Args)
	}
	var _ *exec.Cmd = cmd
}

// UIBENCH_CPUPROFILE never reaches a measurement run; -profile-dir sets it
// for the Tango program only, to the scene's file.
func TestChildEnvProfile(t *testing.T) {
	env, cleared := childEnv([]string{"HOME=/h", "UIBENCH_CPUPROFILE=/tmp/x.pprof", "UIBENCH_OTHER=1"})
	if !reflect.DeepEqual(env, []string{"HOME=/h", "UIBENCH_OTHER=1"}) || !reflect.DeepEqual(cleared, []string{"UIBENCH_CPUPROFILE=/tmp/x.pprof"}) {
		t.Fatalf("childEnv = %v, cleared %v", env, cleared)
	}
	t.Setenv("UIBENCH_CPUPROFILE", "/tmp/inherited.pprof")
	profiles := func(cmd *exec.Cmd) (out []string) {
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "UIBENCH_CPUPROFILE=") {
				out = append(out, kv)
			}
		}
		return out
	}
	for _, fw := range []string{"tango", "mygo", "nsgo"} {
		if got := profiles(sceneCommand(config{bin: "/b"}, fw, "list")); len(got) != 0 {
			t.Fatalf("measurement run of %s inherits %v", fw, got)
		}
	}
	cfg := config{bin: "/b", profileDir: "/p"}
	if got := profiles(sceneCommand(cfg, "tango", "list")); !reflect.DeepEqual(got, []string{"UIBENCH_CPUPROFILE=/p/tango-list.pprof"}) {
		t.Fatalf("-profile-dir tango env %v", got)
	}
	for _, fw := range []string{"mygo", "nsgo"} {
		if got := profiles(sceneCommand(cfg, fw, "list")); len(got) != 0 {
			t.Fatalf("-profile-dir gives %s %v", fw, got)
		}
	}
}
