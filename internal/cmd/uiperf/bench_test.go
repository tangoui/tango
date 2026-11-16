package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// matchesBenchName mimics how `go test -bench` applies a slash-separated
// pattern: each element of the pattern must match the corresponding element
// of the benchmark name.
func matchesBenchName(t *testing.T, pattern, name string) bool {
	t.Helper()
	pe, ne := strings.Split(pattern, "/"), strings.Split(name, "/")
	if len(pe) != len(ne) {
		t.Fatalf("pattern %q and name %q have different depth", pattern, name)
	}
	for i := range pe {
		if !regexp.MustCompile(pe[i]).MatchString(ne[i]) {
			return false
		}
	}
	return true
}

func TestColdBenchRegexAnchoredPerElement(t *testing.T) {
	if got, want := coldBenchRegex("Checkbox"), `^BenchmarkComponents$/^Checkbox$/^MountCold$`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := coldBenchRegex("A+B"), `^BenchmarkComponents$/^A\+B$/^MountCold$`; got != want {
		t.Fatalf("got %q, want %q (name must be quoted)", got, want)
	}
	re := coldBenchRegex("Checkbox")
	if !matchesBenchName(t, re, "BenchmarkComponents/Checkbox/MountCold") {
		t.Fatal("does not match its own fixture")
	}
	for _, other := range []string{"BenchmarkComponents/CheckboxGroup/MountCold", "BenchmarkComponents/MyCheckbox/MountCold", "BenchmarkComponentsRaster/Checkbox/MountCold", "BenchmarkComponents/Checkbox/MountColder"} {
		if matchesBenchName(t, re, other) {
			t.Fatalf("%q also matches %q", re, other)
		}
	}
}

func TestHotBenchRegexAnchoredPerElement(t *testing.T) {
	re := hotBenchRegex("Checkbox", "Interact")
	if re != `^BenchmarkComponents$/^Checkbox$/^Interact$` {
		t.Fatalf("got %q", re)
	}
	if matchesBenchName(t, re, "BenchmarkComponents/CheckboxGroup/Interact") {
		t.Fatalf("%q matches CheckboxGroup", re)
	}
}

type benchCall struct {
	env  []string
	name string
	args []string
}

func hasArgs(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		ok := true
		for j := range want {
			if args[i+j] != want[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// fakeBenchPipeline replaces benchExec: the benchmark run succeeds, the
// budget run fails (as it does whenever a scene is over budget), and the
// cold run succeeds for Checkbox but fails for CheckboxGroup.
func fakeBenchPipeline(t *testing.T) *[]benchCall {
	t.Helper()
	var calls []benchCall
	old := benchExec
	t.Cleanup(func() { benchExec = old })
	benchExec = func(env []string, name string, args ...string) ([]byte, error) {
		calls = append(calls, benchCall{env, name, args})
		switch {
		case name == "go" && hasArgs(args, "-c"):
			return nil, nil
		case name == "go" && hasArgs(args, "-run", "Test(Perf|GalleryPage)Budgets"):
			return []byte("=== RUN   TestPerfBudgets\nPERF Checkbox/Interact 3000 2000000 ok\nPERF CheckboxGroup/Interact 2500000 2000000 over\n--- FAIL: TestPerfBudgets\nFAIL\n"), errors.New("exit status 1")
		case name == "go":
			return []byte("goos: darwin\nBenchmarkComponents/Checkbox/Interact-10  100  3000 ns/op  0.5 dirty/op  10 B/op  1 allocs/op\nBenchmarkComponents/CheckboxGroup/Interact-10  1  2500000 ns/op  1 dirty/op  20 B/op  2 allocs/op\nPASS\n"), nil
		case strings.HasSuffix(name, "ui.test"):
			if hasArgs(args, "-test.bench", coldBenchRegex("CheckboxGroup")) {
				return []byte("panic: boom\n"), errors.New("exit status 2")
			}
			if hasArgs(args, "-test.bench", coldBenchRegex("Checkbox")) {
				return []byte("BenchmarkComponents/Checkbox/MountCold-10  1  9000000 ns/op  1 dirty/op  10 B/op  1 allocs/op\nPASS\n"), nil
			}
			return []byte("PASS\n"), nil
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil, nil
	}
	return &calls
}

func readBenchFile(t *testing.T, dir string) benchFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "bench.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bf benchFile
	if err := json.Unmarshal(b, &bf); err != nil {
		t.Fatal(err)
	}
	return bf
}

func TestRunBenchCommandFlags(t *testing.T) {
	calls := fakeBenchPipeline(t)
	if err := runBench(3, ".*", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	budgetRuns := 0
	for _, c := range *calls {
		switch {
		case c.name == "go" && hasArgs(c.args, "-c"):
		case c.name == "go":
			if !hasArgs(c.args, "-timeout", "0") {
				t.Errorf("go %v lacks -timeout 0", c.args)
			}
			if hasArgs(c.args, "-run", "Test(Perf|GalleryPage)Budgets") {
				budgetRuns++
				if !hasArgs(c.args, "-count=1") || !hasArgs(c.args, "-p", "1") {
					t.Errorf("budget run %v lacks -count=1 / -p 1", c.args)
				}
				if !hasArgs(c.env, "TANGO_PERF_BUDGET=1") && !strings.Contains(strings.Join(c.env, "\n"), "TANGO_PERF_BUDGET=1") {
					t.Errorf("budget run lacks TANGO_PERF_BUDGET=1")
				}
			}
		default:
			if !hasArgs(c.args, "-test.timeout", "0") {
				t.Errorf("cold run %v lacks -test.timeout 0", c.args)
			}
		}
	}
	if budgetRuns != 1 {
		t.Fatalf("budget runs = %d", budgetRuns)
	}
}

func TestRunBenchBudgetFailureStillWritesResults(t *testing.T) {
	fakeBenchPipeline(t)
	dir := t.TempDir()
	if err := runBench(1, ".*", dir); err != nil {
		t.Fatalf("bench aborted on budget failure: %v", err)
	}
	bf := readBenchFile(t, dir)
	if len(bf.Results) != 2 {
		t.Fatalf("results %+v", bf.Results)
	}
	if bf.Budget.Status != "failed" || !strings.Contains(bf.Budget.Error, "exit status 1") || !strings.Contains(bf.Budget.Output, "PERF CheckboxGroup/Interact") {
		t.Fatalf("budget run not recorded: %+v", bf.Budget)
	}
	for _, r := range bf.Results {
		switch r.Name {
		case "Checkbox":
			if r.Budget != 2000000 || !r.Pass {
				t.Errorf("Checkbox %+v", r)
			}
		case "CheckboxGroup":
			if r.Budget != 2000000 || r.Pass {
				t.Errorf("CheckboxGroup %+v", r)
			}
		}
	}
	st, err := os.ReadFile(filepath.Join(dir, "status.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(st), "Budget run: failed") || !strings.Contains(string(st), "exit status 1") {
		t.Fatalf("status.md does not record the budget failure:\n%s", st)
	}
}

func TestRunBenchColdFailureExplicit(t *testing.T) {
	fakeBenchPipeline(t)
	dir := t.TempDir()
	if err := runBench(1, ".*", dir); err != nil {
		t.Fatal(err)
	}
	for _, r := range readBenchFile(t, dir).Results {
		switch r.Name {
		case "Checkbox":
			if r.ColdStatus != "ok" || r.ColdNS != 9000000 || r.ColdError != "" {
				t.Errorf("Checkbox cold %+v", r)
			}
		case "CheckboxGroup":
			if r.ColdStatus != "failed" || r.ColdNS != 0 || !strings.Contains(r.ColdError, "exit status 2") || !strings.Contains(r.ColdError, "panic: boom") {
				t.Errorf("CheckboxGroup cold %+v", r)
			}
		}
	}
	st, _ := os.ReadFile(filepath.Join(dir, "status.md"))
	if !strings.Contains(string(st), "| — | failed: exit status 2: panic: boom |") {
		t.Fatalf("status.md hides the cold failure:\n%s", st)
	}
}

func TestRunBenchColdMissingRowExplicit(t *testing.T) {
	fakeBenchPipeline(t)
	inner := benchExec
	benchExec = func(env []string, name string, args ...string) ([]byte, error) {
		if strings.HasSuffix(name, "ui.test") {
			return []byte("PASS\n"), nil // ran, but no MountCold row
		}
		return inner(env, name, args...)
	}
	dir := t.TempDir()
	if err := runBench(1, ".*", dir); err != nil {
		t.Fatal(err)
	}
	for _, r := range readBenchFile(t, dir).Results {
		if r.ColdStatus != "missing" || r.ColdError == "" {
			t.Errorf("%s cold %+v", r.Name, r)
		}
	}
}

func TestRunBenchColdBuildFailureExplicit(t *testing.T) {
	fakeBenchPipeline(t)
	inner := benchExec
	benchExec = func(env []string, name string, args ...string) ([]byte, error) {
		if name == "go" && hasArgs(args, "-c") {
			return []byte("compile error"), errors.New("exit status 1")
		}
		return inner(env, name, args...)
	}
	dir := t.TempDir()
	if err := runBench(1, ".*", dir); err != nil {
		t.Fatal(err)
	}
	for _, r := range readBenchFile(t, dir).Results {
		if r.ColdStatus != "failed" || !strings.Contains(r.ColdError, "compile error") {
			t.Errorf("%s cold %+v", r.Name, r)
		}
	}
}

// The bench step's first -bench element is anchored: an unanchored
// "BenchmarkComponents" also selects BenchmarkComponentsRaster.
func TestRunBenchFirstElementAnchored(t *testing.T) {
	calls := fakeBenchPipeline(t)
	if err := runBench(1, "Checkbox/Interact", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if c.name == "go" && hasArgs(c.args, "-run", "^$") {
			if !hasArgs(c.args, "-bench", "^BenchmarkComponents$/Checkbox/Interact") {
				t.Fatalf("bench run %v: want -bench ^BenchmarkComponents$/Checkbox/Interact", c.args)
			}
			return
		}
	}
	t.Fatal("no bench run")
}

// A failed budget gate leaves bench's exit status 0, so bench says so on
// stderr in one line.
func TestRunBenchBudgetFailureNote(t *testing.T) {
	fakeBenchPipeline(t)
	var buf strings.Builder
	old := benchStderr
	benchStderr = &buf
	t.Cleanup(func() { benchStderr = old })
	if err := runBench(1, ".*", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, "budget.status") || !strings.Contains(got, "exits 0") {
		t.Fatalf("stderr note %q: want one line naming exit 0 and budget.status", got)
	}
}
