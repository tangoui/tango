package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pprofEntry struct {
	Name                           string
	FlatNS, FlatPct, CumNS, CumPct float64
}
type hotInput struct {
	Component, Function   string
	FlatPct, CumPct, Over float64
}
type hotFailure struct{ Component, Stage, Reason string }
type hotScore struct {
	Function string
	// Score = Σ cum%/100 × over-budget factor; FlatScore uses flat% instead.
	Score, FlatScore float64
	// CumPct and FlatPct are means over the profiles the function appeared in.
	CumPct, FlatPct float64
	Components      []string
	NonTarget       bool
	Kind            frameKind
	n               int
}

// defaultHotNodeCount is the per-profile `pprof -nodecount`. Harness and
// call-chain rows (flat=0 since F1) take many of the top slots, so 30 pushed
// leaf hotspots out of the ranking (R1 concern 3, ruling R42).
const defaultHotNodeCount = 200

type frameKind int

const (
	frameComponent frameKind = iota
	framePipeline            // ui.(*Runner).* frame chain: shown, not ranked
	frameHarness             // test/benchmark harness: dropped from scoring
)

func (k frameKind) String() string {
	switch k {
	case framePipeline:
		return "pipeline"
	case frameHarness:
		return "harness"
	}
	return "component"
}

// keptRuntime are runtime frames that carry real allocation/GC signal.
var keptRuntime = map[string]bool{"runtime.mallocgc": true, "runtime.gcDrain": true, "runtime.madvise": true}

// shortFrame strips the " (inline)" marker and the import path up to the last
// "/" before any generic type arguments, e.g.
// "github.com/tangoui/tango/ui.(*Runner).Frame (inline)" -> "ui.(*Runner).Frame".
func shortFrame(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), " (inline)")
	head := name
	if i := strings.IndexByte(head, '['); i >= 0 {
		head = head[:i]
	}
	if i := strings.LastIndexByte(head, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func classifyFrame(name string) frameKind {
	s := shortFrame(name)
	switch {
	case strings.HasPrefix(s, "testing."), strings.HasPrefix(s, "main."):
		return frameHarness
	case strings.HasPrefix(s, "runtime."):
		if keptRuntime[s] {
			return frameComponent
		}
		return frameHarness
	case strings.HasPrefix(s, "ui.(*Driver)."), strings.HasPrefix(s, "ui.benchPerfScene"), strings.HasPrefix(s, "ui.Benchmark"):
		return frameHarness
	case strings.HasPrefix(s, "ui.perf") && len(s) > len("ui.perf") && s[len("ui.perf")] >= 'A' && s[len("ui.perf")] <= 'Z':
		// ui.perfScenes, ui.perfStep, ui.perfTime*, ui.perfNav*, … all live in
		// ui/perf_*_test.go; ui.performX would be real code.
		return frameHarness
	case strings.HasPrefix(s, "ui.(*Runner)."):
		return framePipeline
	}
	return frameComponent
}

// pprofRE matches one `pprof -top` row: flat flat% sum% cum cum% name.
// pprof prints a zero duration as a bare "0" without a unit; those rows are
// the cumulative-only callers (Frame, layoutAndPaint, …) and must be kept.
var pprofRE = regexp.MustCompile(`^\s*([0-9.]+(?:s|ms|us|µs|ns)?)\s+([0-9.]+)%\s+[0-9.]+%\s+([0-9.]+(?:s|ms|us|µs|ns)?)\s+([0-9.]+)%\s+(.+?)\s*$`)

func durationNS(s string) float64 {
	u := "ns"
	s = strings.Replace(s, "µs", "us", 1)
	for _, x := range []string{"ms", "us", "ns", "s"} {
		if strings.HasSuffix(s, x) {
			u = x
			break
		}
	}
	var n float64
	fmt.Sscanf(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "ms"), "us"), "ns"), "s"), "%f", &n)
	switch u {
	case "s":
		return n * 1e9
	case "ms":
		return n * 1e6
	case "us":
		return n * 1e3
	}
	return n
}
func parsePprofTop(s string) ([]pprofEntry, error) {
	var out []pprofEntry
	for _, line := range strings.Split(s, "\n") {
		m := pprofRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		flatPct, e := strconv.ParseFloat(m[2], 64)
		if e != nil {
			return nil, e
		}
		cumPct, e := strconv.ParseFloat(m[4], 64)
		if e != nil {
			return nil, e
		}
		out = append(out, pprofEntry{Name: strings.TrimSpace(m[5]), FlatNS: durationNS(m[1]), FlatPct: flatPct, CumNS: durationNS(m[3]), CumPct: cumPct})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pprof entries")
	}
	return out, nil
}
func isNonTarget(s string) bool {
	for _, p := range []string{"github.com/tangoui/tango/internal/render", "/uihost", "/macoslite", "/ui/terminal", "/ui/editor"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// hotScores aggregates per-profile rows by function. Harness frames are
// dropped; pipeline frames are scored but carry Kind=framePipeline so the
// report can keep them out of the component ranking.
func hotScores(in []hotInput) []hotScore {
	m := map[string]*hotScore{}
	for _, x := range in {
		kind := classifyFrame(x.Function)
		if kind == frameHarness {
			continue
		}
		h := m[x.Function]
		if h == nil {
			h = &hotScore{Function: x.Function, Kind: kind, NonTarget: isNonTarget(x.Function)}
			m[x.Function] = h
		}
		h.Score += x.CumPct / 100 * x.Over
		h.FlatScore += x.FlatPct / 100 * x.Over
		h.CumPct += x.CumPct
		h.FlatPct += x.FlatPct
		h.n++
		found := false
		for _, c := range h.Components {
			if c == x.Component {
				found = true
			}
		}
		if !found {
			h.Components = append(h.Components, x.Component)
		}
	}
	out := make([]hotScore, 0, len(m))
	for _, h := range m {
		h.CumPct /= float64(h.n)
		h.FlatPct /= float64(h.n)
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Function < out[j].Function
	})
	return out
}

func hotBenchRegex(name, scene string) string { return componentBenchRegex(name, scene) }

type hotOptions struct {
	NodeCount     int    // pprof -nodecount per profile
	ReuseProfiles bool   // score existing prof/*.pprof without re-running benchmarks
	Output        string // report file name inside dir
}

// profileBench and pprofTopText are variables so tests can run runHot without
// benchmarks or the go tool.
var profileBench = func(regex, prof string) ([]byte, error) {
	return exec.Command("go", "test", "./ui", "-run", "^$", "-bench", regex, "-benchtime=2s", "-timeout", "0", "-cpuprofile", prof).CombinedOutput()
}
var pprofTopText = func(prof string, nodecount int) ([]byte, error) {
	return exec.Command("go", "tool", "pprof", "-top", "-cum", "-nodecount="+fmt.Sprint(nodecount), prof).Output()
}

// flatTableRows caps the "top by flat" table; the cum ranking stays complete.
const flatTableRows = 50

func runHot(dir string, opt hotOptions) error {
	if opt.NodeCount < 1 {
		return fmt.Errorf("top must be positive")
	}
	if opt.Output == "" {
		opt.Output = "hot.md"
	}
	data, err := os.ReadFile(filepath.Join(dir, "bench.json"))
	if err != nil {
		return err
	}
	var bf benchFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return err
	}
	var inputs []hotInput
	var failures []hotFailure
	attempted, succeeded := 0, 0
	profDir := filepath.Join(dir, "prof")
	if err := os.MkdirAll(profDir, 0755); err != nil {
		return err
	}
	for _, b := range bf.Results {
		if b.Budget <= 0 || b.NsOp <= b.Budget {
			continue
		}
		attempted++
		component := b.Name + "/" + b.Scene
		safe := strings.NewReplacer("/", "-", "\\", "-").Replace(component)
		prof := filepath.Join(profDir, safe+".pprof")
		if opt.ReuseProfiles {
			if _, e := os.Stat(prof); e != nil {
				failures = append(failures, hotFailure{component, "reuse", e.Error()})
				continue
			}
		} else if out, e := profileBench(hotBenchRegex(b.Name, b.Scene), prof); e != nil {
			failures = append(failures, hotFailure{component, "profile", fmt.Sprintf("%v: %s", e, strings.TrimSpace(string(out)))})
			continue
		}
		text, e := pprofTopText(prof, opt.NodeCount)
		if e != nil {
			failures = append(failures, hotFailure{component, "pprof", e.Error()})
			continue
		}
		es, e := parsePprofTop(string(text))
		if e != nil {
			failures = append(failures, hotFailure{component, "parse", e.Error()})
			continue
		}
		succeeded++
		for _, x := range es {
			inputs = append(inputs, hotInput{Component: component, Function: x.Name, FlatPct: x.FlatPct, CumPct: x.CumPct, Over: b.NsOp / b.Budget})
		}
	}
	f, err := os.Create(filepath.Join(dir, opt.Output))
	if err != nil {
		return err
	}
	defer f.Close()
	writeHotReport(f, hotScores(inputs), failures, opt)
	if attempted > 0 && succeeded == 0 {
		return fmt.Errorf("all %d hotspot profiles failed; see %s", attempted, filepath.Join(dir, opt.Output))
	}
	return nil
}

func writeHotReport(w io.Writer, scores []hotScore, failures []hotFailure, opt hotOptions) {
	var comp, pipe []hotScore
	for _, x := range scores {
		if x.Kind == framePipeline {
			pipe = append(pipe, x)
		} else {
			comp = append(comp, x)
		}
	}
	src := "fresh profiles"
	if opt.ReuseProfiles {
		src = "reused prof/*.pprof"
	}
	fmt.Fprintf(w, "# UI hotspots\n\n`pprof -top -cum -nodecount=%d` per over-budget profile (%s). "+
		"Score = Σ cum%%/100 × over-budget factor; flat score uses flat%%. cum%%/flat%% are means over the listed components. "+
		"Harness frames (testing.*, runtime.* except mallocgc/gcDrain/madvise, main.*, ui.perf*/benchPerfScene/Benchmark*, ui.(*Driver).*) are excluded; "+
		"ui.(*Runner).* is listed under Pipeline frames.\n\n", opt.NodeCount, src)
	row := func(x hotScore, score, a, b float64, kind bool) {
		fmt.Fprintf(w, "| `%s` | %.4f | %.2f | %.2f | %s | %t |", x.Function, score, a, b, strings.Join(x.Components, ", "), x.NonTarget)
		if kind {
			fmt.Fprintf(w, " %s |", x.Kind)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "| Function | Score | cum% | flat% | Components | Non-target |\n|---|---:|---:|---:|---|:---:|")
	for _, x := range comp {
		row(x, x.Score, x.CumPct, x.FlatPct, false)
	}
	flat := append([]hotScore(nil), comp...)
	sort.SliceStable(flat, func(i, j int) bool {
		if flat[i].FlatScore != flat[j].FlatScore {
			return flat[i].FlatScore > flat[j].FlatScore
		}
		return flat[i].Function < flat[j].Function
	})
	fmt.Fprintf(w, "\n## Top by flat\n\nLeaf cost: Σ flat%%/100 × over-budget factor (top %d, flat > 0).\n\n", flatTableRows)
	fmt.Fprintln(w, "| Function | Flat score | flat% | cum% | Components | Non-target |\n|---|---:|---:|---:|---|:---:|")
	for i, x := range flat {
		if i >= flatTableRows || x.FlatScore <= 0 {
			break
		}
		row(x, x.FlatScore, x.FlatPct, x.CumPct, false)
	}
	fmt.Fprint(w, "\n## Pipeline frames\n\nui.(*Runner).* frame chain; scored the same way but not a component hotspot.\n\n")
	fmt.Fprintln(w, "| Function | Score | cum% | flat% | Components | Non-target | Kind |\n|---|---:|---:|---:|---|:---:|---|")
	for _, x := range pipe {
		row(x, x.Score, x.CumPct, x.FlatPct, true)
	}
	if len(failures) > 0 {
		fmt.Fprintln(w, "\n## Failed profiles\n\n| Component | Stage | Reason |\n|---|---|---|")
		for _, x := range failures {
			fmt.Fprintf(w, "| %s | %s | %s |\n", x.Component, x.Stage, strings.ReplaceAll(x.Reason, "|", "\\|"))
		}
	}
}
