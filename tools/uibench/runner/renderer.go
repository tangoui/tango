package main

// Renderer recording (spec §6.3): the scene programs report their drawing
// path with a timestamp at ready, with every per-second report and the
// moment it changes. A round keeps the whole timeline and its switches;
// its Renderer is the one path seen in the sample window, or "mixed".

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// mixedRenderer is a round's Renderer when the sample window saw more
// than one path.
const mixedRenderer = "mixed"

// RendererSample is one renderer line: the path and when the program
// reported it (Unix ns, the program's clock), or when the runner read it
// for an old line without a time (RunnerClock).
type RendererSample struct {
	T           int64  `json:"t_ns"`
	Name        string `json:"name"`
	RunnerClock bool   `json:"runner_clock,omitempty"`
}

// RendererSwitch is a change of path between two consecutive samples, at
// the later one's time.
type RendererSwitch struct {
	T        int64  `json:"t_ns"`
	From     string `json:"from"`
	To       string `json:"to"`
	InSample bool   `json:"in_sample"`
}

// summarizeRenderer reads a timeline against the sample window (t0, t1]:
// seq is the path in effect at t0 followed by the paths reported inside
// the window, repeats collapsed; summary is its one path, "mixed" when it
// has more, "" when nothing was reported. switches are all of the
// timeline's changes, each marked whether it fell inside the window.
func summarizeRenderer(tl []RendererSample, t0, t1 int64) (summary string, seq []string, switches []RendererSwitch) {
	add := func(name string) {
		if len(seq) == 0 || seq[len(seq)-1] != name {
			seq = append(seq, name)
		}
	}
	atStart := ""
	for i, s := range tl {
		if i > 0 && s.Name != tl[i-1].Name {
			switches = append(switches, RendererSwitch{T: s.T, From: tl[i-1].Name, To: s.Name, InSample: s.T > t0 && s.T <= t1})
		}
		switch {
		case s.T <= t0:
			atStart = s.Name
		case s.T <= t1:
			if atStart != "" {
				add(atStart)
				atStart = ""
			}
			add(s.Name)
		}
	}
	if atStart != "" { // nothing reported inside the window
		add(atStart)
	}
	switch len(seq) {
	case 0:
		return "", nil, switches
	case 1:
		return seq[0], seq, switches
	}
	return mixedRenderer, seq, switches
}

// rendererKey is what the consistency check compares between rounds: the
// path, or for a mixed round its in-sample sequence, so that a mixed round
// matches only rounds that switched the same way.
func rendererKey(r Round) string {
	if r.Renderer == mixedRenderer {
		return mixedRenderer + "[" + strings.Join(r.RendererSequence, ">") + "]"
	}
	return r.Renderer
}

// rendererSummary is one side's path over its usable rounds (their common
// key, or "inconsistent: a, b") and the switches inside their sample windows.
func rendererSummary(rs []Round) (key string, switches int) {
	var keys []string
	seen := map[string]bool{}
	for _, r := range usable(rs) {
		switches += r.SampleSwitches
		if k := rendererKey(r); !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if len(keys) > 1 {
		return "inconsistent: " + strings.Join(keys, ", "), switches
	}
	if len(keys) == 1 {
		return keys[0], switches
	}
	return "", switches
}

// describeRenderer is one side's path for a verdict line.
func describeRenderer(rs []Round) string {
	key, sw := rendererSummary(rs)
	if sw > 0 || strings.HasPrefix(key, mixedRenderer) {
		return fmt.Sprintf("%s (%d switches in sample windows)", key, sw)
	}
	return key
}

// childEnv is the scene programs' environment: the runner's, without any
// TANGO_* variable, so that Tango takes its default renderer policy (spec
// §6.3: TANGO_RENDERER) and pays for no Tango-only diagnostics (such as
// TANGO_FRAME_TRACE), and without UIBENCH_CPUPROFILE, the Tango program's
// opt-in CPU profile, which only -profile-dir sets. cleared lists what was
// removed.
func childEnv(parent []string) (env, cleared []string) {
	for _, kv := range parent {
		if strings.HasPrefix(kv, "TANGO_") || strings.HasPrefix(kv, profileEnv+"=") {
			cleared = append(cleared, kv)
			continue
		}
		env = append(env, kv)
	}
	return env, cleared
}

// profileEnv is the Tango program's CPU profile variable (tango/profile.go).
const profileEnv = "UIBENCH_CPUPROFILE"

// profilePath is where -profile-dir has the Tango program write scene's
// CPU profile.
func profilePath(dir, scene string) string {
	return filepath.Join(dir, "tango-"+scene+".pprof")
}

// stderrPath is where -profile-dir keeps the Tango program's stderr.
func stderrPath(dir, scene string) string {
	return filepath.Join(dir, "tango-"+scene+".stderr")
}

// envList is -tango-env: repeatable KEY=VAL.
type envList []string

func (l *envList) String() string { return strings.Join(*l, ",") }

func (l *envList) Set(v string) error {
	k, _, ok := strings.Cut(v, "=")
	if !ok || k == "" || strings.ContainsAny(k, " \t") {
		return fmt.Errorf("want KEY=VAL, got %q", v)
	}
	*l = append(*l, v)
	return nil
}

// checkTangoEnv allows -tango-env only in a profiling run (-profile-dir):
// a diagnostic variable must never reach a measurement.
func checkTangoEnv(env []string, profileDir string) error {
	if len(env) > 0 && profileDir == "" {
		return errors.New("-tango-env is a diagnostic: allowed only with -profile-dir")
	}
	return nil
}

// tangoRenderers are the TANGO_RENDERER values Tango accepts
// (uihost/gpu.go setupGPU; docs/dev-builds.md): any other value, like an
// unset one, runs the default policy, so -tango-renderer refuses it.
var tangoRenderers = []string{"auto", "cpu", "gpu", "native"}

// rendererEnv is the variable -tango-renderer sets for the Tango program.
const rendererEnv = "TANGO_RENDERER"

// forcedRendererReason is the meta.not_adoptable reason of a run with
// -tango-renderer.
const forcedRendererReason = "tango renderer forced"

// checkTangoRenderer validates -tango-renderer: empty (Tango's default
// policy) or one of tangoRenderers, and not also given as -tango-env.
func checkTangoRenderer(v string, tangoEnv []string) error {
	if v == "" {
		return nil
	}
	if !contains(tangoRenderers, v) {
		return fmt.Errorf("-tango-renderer %q: want one of %s (Tango's TANGO_RENDERER values)", v, strings.Join(tangoRenderers, ", "))
	}
	for _, kv := range tangoEnv {
		if strings.HasPrefix(kv, rendererEnv+"=") {
			return fmt.Errorf("-tango-renderer and -tango-env %s both set the renderer: give one", kv)
		}
	}
	return nil
}

// forceTangoRenderer records -tango-renderer in the results meta and, when
// set, marks the run not adoptable: a verdict against MyGo or NSGo must
// compare Tango's default policy. The rounds are kept as usual (Tango-only
// A/B with -frameworks tango).
func forceTangoRenderer(m *Meta, v string) {
	if v == "" {
		return
	}
	m.TangoRenderer = v
	m.markNotAdoptable(forcedRendererReason)
}

// forcedRendererWhy marks a verdict reason measured with -tango-renderer.
func forcedRendererWhy(why, v string) string {
	if v == "" {
		return why
	}
	return "TANGO RENDERER FORCED (" + v + ", not adoptable): " + why
}

// sceneCommand is the command that starts one scene program. Only with
// -profile-dir (cfg.profileDir) does the Tango program get
// UIBENCH_CPUPROFILE; no other program and no measurement run ever does.
// -tango-renderer (cfg.tangoRenderer) gives the Tango program alone
// TANGO_RENDERER, in measurement and profiling runs.
func sceneCommand(cfg config, fw, scene string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(cfg.bin, fw), "-scene", scene)
	cmd.Env, _ = childEnv(os.Environ())
	if cfg.tangoRenderer != "" && fw == "tango" {
		cmd.Env = append(cmd.Env, rendererEnv+"="+cfg.tangoRenderer)
	}
	if cfg.profileDir != "" && fw == "tango" {
		cmd.Env = append(cmd.Env, profileEnv+"="+profilePath(cfg.profileDir, scene))
		cmd.Env = append(cmd.Env, cfg.tangoEnv...)
	}
	if cmd.Env == nil {
		cmd.Env = []string{} // an empty environment, not the runner's
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}
