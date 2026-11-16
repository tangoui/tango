package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type pprofEntry struct {
	Name                  string
	FlatNS, CumNS, CumPct float64
}
type hotInput struct {
	Component, Function string
	CumPct, Over        float64
}
type hotFailure struct{ Component, Stage, Reason string }
type hotScore struct {
	Function   string
	Score      float64
	Components []string
	NonTarget  bool
}

// pprofRE matches one `pprof -top` row: flat flat% sum% cum cum% name.
// pprof prints a zero duration as a bare "0" without a unit; those rows are
// the cumulative-only callers (Frame, layoutAndPaint, …) and must be kept.
var pprofRE = regexp.MustCompile(`^\s*([0-9.]+(?:s|ms|us|µs|ns)?)\s+[0-9.]+%\s+[0-9.]+%\s+([0-9.]+(?:s|ms|us|µs|ns)?)\s+([0-9.]+)%\s+(.+?)\s*$`)

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
		fp, e := strconv.ParseFloat(m[3], 64)
		if e != nil {
			return nil, e
		}
		out = append(out, pprofEntry{Name: strings.TrimSpace(m[4]), FlatNS: durationNS(m[1]), CumNS: durationNS(m[2]), CumPct: fp})
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
func hotScores(in []hotInput) []hotScore {
	m := map[string]*hotScore{}
	for _, x := range in {
		h := m[x.Function]
		if h == nil {
			h = &hotScore{Function: x.Function}
			m[x.Function] = h
		}
		h.Score += x.CumPct / 100 * x.Over
		h.NonTarget = h.NonTarget || isNonTarget(x.Function)
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
		out = append(out, *h)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Score > out[j-1].Score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func hotBenchRegex(name, scene string) string { return componentBenchRegex(name, scene) }

func runHot(dir string, top int) error {
	if top < 1 {
		return fmt.Errorf("top must be positive")
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
		cmd := exec.Command("go", "test", "./ui", "-run", "^$", "-bench", hotBenchRegex(b.Name, b.Scene), "-benchtime=2s", "-timeout", "0", "-cpuprofile", prof)
		if out, e := cmd.CombinedOutput(); e != nil {
			failures = append(failures, hotFailure{component, "profile", fmt.Sprintf("%v: %s", e, strings.TrimSpace(string(out)))})
			continue
		}
		p := exec.Command("go", "tool", "pprof", "-top", "-cum", "-nodecount="+fmt.Sprint(top), prof)
		text, e := p.Output()
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
			inputs = append(inputs, hotInput{Component: component, Function: x.Name, CumPct: x.CumPct, Over: b.NsOp / b.Budget})
		}
	}
	f, err := os.Create(filepath.Join(dir, "hot.md"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "# UI hotspots\n\n| Function | Score | Components | Non-target |\n|---|---:|---|:---:|")
	for _, x := range hotScores(inputs) {
		fmt.Fprintf(f, "| `%s` | %.4f | %s | %t |\n", x.Function, x.Score, strings.Join(x.Components, ", "), x.NonTarget)
	}
	if len(failures) > 0 {
		fmt.Fprintln(f, "\n## Failed profiles\n\n| Component | Stage | Reason |\n|---|---|---|")
		for _, x := range failures {
			fmt.Fprintf(f, "| %s | %s | %s |\n", x.Component, x.Stage, strings.ReplaceAll(x.Reason, "|", "\\|"))
		}
	}
	if attempted > 0 && succeeded == 0 {
		return fmt.Errorf("all %d hotspot profiles failed; see %s", attempted, filepath.Join(dir, "hot.md"))
	}
	return nil
}
