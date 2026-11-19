package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// benchExec runs one command of the bench pipeline and returns its combined
// output. A nil env inherits the current environment. Tests replace it.
var benchExec = func(env []string, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Env = env
	return c.CombinedOutput()
}

// benchStderr receives bench's notes to the user. Tests replace it.
var benchStderr io.Writer = os.Stderr

// componentBenchRegex anchors every slash-separated element: go test matches
// each element on its own, so an unanchored "Checkbox" would also select
// CheckboxGroup.
func componentBenchRegex(name, scene string) string {
	return `^BenchmarkComponents$/^` + regexp.QuoteMeta(name) + `$/^` + regexp.QuoteMeta(scene) + `$`
}

func coldBenchRegex(name string) string { return componentBenchRegex(name, "MountCold") }

// Cold-start outcomes recorded per component in bench.json and status.md.
const (
	coldOK      = "ok"
	coldFailed  = "failed"
	coldMissing = "missing"
)

func runBench(count int, run, out string) error {
	if count < 1 {
		return fmt.Errorf("count must be positive")
	}
	if _, e := os.Stat(out); e != nil && !os.IsNotExist(e) {
		return e
	}
	if e := os.MkdirAll(out, 0755); e != nil {
		return e
	}
	// A full run takes about 65 minutes, far past go test's default 10-minute timeout.
	// The first element is anchored: unanchored, it would also select
	// BenchmarkComponentsRaster and run it as well.
	args := []string{"test", "./ui", "-run", "^$", "-bench", "^BenchmarkComponents$/" + run, "-benchmem", "-count", fmt.Sprint(count), "-timeout", "0"}
	o, e := benchExec(nil, "go", args...)
	if e != nil {
		return fmt.Errorf("benchmark: %w\n%s", e, o)
	}
	groups := map[string][]benchResult{}
	sc := bufio.NewScanner(strings.NewReader(string(o)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "BenchmarkComponents/") {
			continue
		}
		x, pe := parseBenchLine(line)
		if pe != nil {
			return fmt.Errorf("parse benchmark: %w", pe)
		}
		key := x.Name + "\x00" + x.Scene
		groups[key] = append(groups[key], x)
	}
	if e = sc.Err(); e != nil {
		return e
	}
	var results []benchResult
	for _, rows := range groups {
		results = append(results, aggregateBench(rows))
	}
	budget := runBudget(results)
	if budget.Status != "ok" {
		fmt.Fprintf(benchStderr, "uiperf bench: budget gate failed (%s); bench exits 0, check budget.status in %s\n", budget.Error, filepath.Join(out, "bench.json"))
	}
	runCold(out, results)
	sort.Slice(results, func(i, j int) bool { return results[i].Name+results[i].Scene < results[j].Name+results[j].Scene })
	return writeBenchReport(out, results, budget)
}

// runBudget runs the budget gate and merges its PERF lines into results. The
// gate exits non-zero whenever a scene is over budget and not in the debt
// table, which is exactly when the bench results matter, so a failure is
// recorded rather than returned. -p 1 keeps ./ui and ./examples/ui-gallery
// from running at the same time and inflating each other's timings.
func runBudget(results []benchResult) budgetRun {
	out, e := benchExec(append(os.Environ(), "TANGO_PERF_BUDGET=1"), "go", "test", "./ui", "./examples/ui-gallery", "-run", "Test(Perf|GalleryPage)Budgets", "-count=1", "-p", "1", "-timeout", "0", "-v")
	run := budgetRun{Status: "ok", Output: string(out)}
	if e != nil {
		run.Status, run.Error = "failed", e.Error()
	}
	for _, line := range strings.Split(string(out), "\n") {
		b, e := parseBudgetLine(line)
		if e != nil {
			continue
		}
		for i := range results {
			if results[i].Name+"/"+results[i].Scene == b.Key {
				results[i].Budget = b.Budget
				results[i].Pass = b.Status == "ok"
				if b.Status == "n/a" {
					// Pass stays false for an n/a scene: consumers check NA first.
					results[i].NA = b.Reason
					if results[i].NA == "" {
						results[i].NA = "n/a"
					}
				}
			}
		}
	}
	return run
}

// runCold measures each component's first build in a fresh test process. It
// is report-only, but every component gets an explicit ColdStatus so a failed
// or missing measurement never reads as a cold time of 0.
func runCold(out string, results []benchResult) {
	set := func(name string, ns float64, status, msg string) {
		for i := range results {
			if results[i].Name == name {
				results[i].ColdNS, results[i].ColdStatus, results[i].ColdError = ns, status, msg
			}
		}
	}
	setAll := func(msg string) {
		for i := range results {
			results[i].ColdNS, results[i].ColdStatus, results[i].ColdError = 0, coldFailed, msg
		}
	}
	tmp, e := os.MkdirTemp(out, ".cold-")
	if e != nil {
		setAll("temp dir: " + e.Error())
		return
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "ui.test")
	if o, e := benchExec(nil, "go", "test", "-c", "./ui", "-o", bin); e != nil {
		setAll(fmt.Sprintf("build: %v: %s", e, strings.TrimSpace(string(o))))
		return
	}
	seen := map[string]bool{}
	for i := range results {
		name := results[i].Name
		if seen[name] {
			continue
		}
		seen[name] = true
		x, e := benchExec(nil, bin, "-test.run", "^$", "-test.bench", coldBenchRegex(name), "-test.benchtime", "1x", "-test.timeout", "0")
		if e != nil {
			set(name, 0, coldFailed, fmt.Sprintf("%v: %s", e, strings.TrimSpace(string(x))))
			continue
		}
		found := false
		for _, line := range strings.Split(string(x), "\n") {
			if b, pe := parseBenchLine(line); pe == nil && b.Name == name && b.Scene == "MountCold" {
				set(name, b.NsOp, coldOK, "")
				found = true
				break
			}
		}
		if !found {
			set(name, 0, coldMissing, "no MountCold row in output")
		}
	}
}

func aggregateBench(rows []benchResult) benchResult {
	if len(rows) == 0 {
		return benchResult{}
	}
	out := rows[0]
	var ns, dirty, bytes, allocs []float64
	for _, r := range rows {
		ns = append(ns, r.NsOp)
		dirty = append(dirty, r.Dirty)
		bytes = append(bytes, r.BOp)
		allocs = append(allocs, r.AllocsOp)
	}
	out.NsOp, out.Dirty, out.BOp, out.AllocsOp = median(ns), median(dirty), median(bytes), median(allocs)
	return out
}
