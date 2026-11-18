package main

import (
	"encoding/json"
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

// -tango-env: KEY=VAL only, allowed only with -profile-dir, and reaches the
// Tango program alone.
func TestTangoEnv(t *testing.T) {
	var l envList
	for _, bad := range []string{"", "NOEQ", "=1", "A B=1"} {
		if err := l.Set(bad); err == nil {
			t.Fatalf("Set(%q) accepted", bad)
		}
	}
	if err := l.Set("TANGO_FRAME_TRACE=1"); err != nil {
		t.Fatal(err)
	}
	if err := l.Set("TANGO_X="); err != nil {
		t.Fatal(err)
	}
	if err := checkTangoEnv(l, ""); err == nil {
		t.Fatal("-tango-env without -profile-dir accepted")
	}
	if err := checkTangoEnv(l, "/p"); err != nil {
		t.Fatal(err)
	}
	if err := checkTangoEnv(nil, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TANGO_FRAME_TRACE", "inherited")
	has := func(cmd *exec.Cmd) (out []string) {
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "TANGO_") {
				out = append(out, kv)
			}
		}
		return out
	}
	cfg := config{bin: "/b", profileDir: "/p", tangoEnv: l}
	if got := has(sceneCommand(cfg, "tango", "list")); !reflect.DeepEqual(got, []string{"TANGO_FRAME_TRACE=1", "TANGO_X="}) {
		t.Fatalf("tango env %v", got)
	}
	for _, fw := range []string{"mygo", "nsgo"} {
		if got := has(sceneCommand(cfg, fw, "list")); len(got) != 0 {
			t.Fatalf("%s got %v", fw, got)
		}
	}
	if got := stderrPath("/p", "list"); got != "/p/tango-list.stderr" {
		t.Fatal(got)
	}
}

// -tango-renderer accepts exactly Tango's TANGO_RENDERER values (or none),
// and not together with a -tango-env that also sets the renderer.
func TestCheckTangoRenderer(t *testing.T) {
	for _, ok := range []string{"", "auto", "cpu", "gpu", "native"} {
		if err := checkTangoRenderer(ok, nil); err != nil {
			t.Fatalf("checkTangoRenderer(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"GPU", "metal", "opengl", " gpu", "gpu ", "native,gpu", "default"} {
		err := checkTangoRenderer(bad, nil)
		if err == nil {
			t.Fatalf("checkTangoRenderer(%q) accepted", bad)
		}
		if !strings.Contains(err.Error(), "auto, cpu, gpu, native") {
			t.Fatalf("checkTangoRenderer(%q) error does not list the values: %v", bad, err)
		}
	}
	if err := checkTangoRenderer("gpu", []string{"TANGO_FRAME_TRACE=1"}); err != nil {
		t.Fatal(err)
	}
	if err := checkTangoRenderer("gpu", []string{"TANGO_RENDERER=cpu"}); err == nil {
		t.Fatal("-tango-renderer with -tango-env TANGO_RENDERER accepted")
	}
	if err := checkTangoRenderer("", []string{"TANGO_RENDERER=cpu"}); err != nil {
		t.Fatal(err) // -tango-env alone is checkTangoEnv's business
	}
}

// -tango-renderer gives TANGO_RENDERER to the Tango program alone, in a
// measurement run (no -profile-dir) and in a profiling run; an inherited
// TANGO_RENDERER is still cleared everywhere.
func TestTangoRendererEnv(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	tangoVars := func(cmd *exec.Cmd) (out []string) {
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "TANGO_") {
				out = append(out, kv)
			}
		}
		return out
	}
	for _, fw := range []string{"tango", "mygo", "nsgo"} {
		if got := tangoVars(sceneCommand(config{bin: "/b"}, fw, "list")); len(got) != 0 {
			t.Fatalf("default run: %s got %v", fw, got)
		}
	}
	for _, cfg := range []config{
		{bin: "/b", tangoRenderer: "gpu"},
		{bin: "/b", tangoRenderer: "gpu", profileDir: "/p"},
	} {
		if got := tangoVars(sceneCommand(cfg, "tango", "list")); !reflect.DeepEqual(got, []string{"TANGO_RENDERER=gpu"}) {
			t.Fatalf("profileDir %q: tango env %v", cfg.profileDir, got)
		}
		for _, fw := range []string{"mygo", "nsgo"} {
			if got := tangoVars(sceneCommand(cfg, fw, "list")); len(got) != 0 {
				t.Fatalf("profileDir %q: %s got %v", cfg.profileDir, fw, got)
			}
		}
	}
	cfg := config{bin: "/b", tangoRenderer: "native"}
	if got := tangoVars(sceneCommand(cfg, "tango", "grid")); !reflect.DeepEqual(got, []string{"TANGO_RENDERER=native"}) {
		t.Fatalf("native: tango env %v", got)
	}
}

// A forced renderer is recorded in the meta and makes the run not
// adoptable, once; without it the meta is unchanged.
func TestForceTangoRendererMeta(t *testing.T) {
	m := Meta{Adoptable: true}
	forceTangoRenderer(&m, "")
	if !m.Adoptable || m.TangoRenderer != "" || len(m.NotAdoptable) != 0 {
		t.Fatalf("no -tango-renderer changed the meta: %+v", m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "tango_renderer") {
		t.Fatalf("unforced meta has tango_renderer: %s", b)
	}
	forceTangoRenderer(&m, "gpu")
	forceTangoRenderer(&m, "gpu")
	if m.Adoptable || m.TangoRenderer != "gpu" || !reflect.DeepEqual(m.NotAdoptable, []string{"tango renderer forced"}) {
		t.Fatalf("forced meta: adoptable %v, renderer %q, not adoptable %v", m.Adoptable, m.TangoRenderer, m.NotAdoptable)
	}
	if b, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"tango_renderer":"gpu"`) || !strings.Contains(string(b), `"not_adoptable":["tango renderer forced"]`) {
		t.Fatalf("forced meta JSON: %s", b)
	}

	// Alongside another reason: both kept, not adoptable.
	p := Meta{Adoptable: true}
	p.markNotAdoptable("profiling run")
	forceTangoRenderer(&p, "native")
	if p.Adoptable || !reflect.DeepEqual(p.NotAdoptable, []string{"profiling run", "tango renderer forced"}) {
		t.Fatalf("profiling + forced: %v %v", p.Adoptable, p.NotAdoptable)
	}
}

// Verdict lines of a forced-renderer run say so; the reason is otherwise
// untouched.
func TestForcedRendererWhy(t *testing.T) {
	if got := forcedRendererWhy("cpu 0.20 vs 0.30", ""); got != "cpu 0.20 vs 0.30" {
		t.Fatalf("unforced why %q", got)
	}
	got := forcedRendererWhy("cpu 0.20 vs 0.30", "gpu")
	if got != "TANGO RENDERER FORCED (gpu, not adoptable): cpu 0.20 vs 0.30" {
		t.Fatalf("forced why %q", got)
	}
}
