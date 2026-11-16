package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	args := []string{"test", "./ui", "-run", "^$", "-bench", "BenchmarkComponents/" + run, "-benchmem", "-count", fmt.Sprint(count)}
	o, e := exec.Command("go", args...).CombinedOutput()
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
	var results []benchResult
	for _, rows := range groups {
		results = append(results, aggregateBench(rows))
	}
	if e = sc.Err(); e != nil {
		return e
	}
	budgetCmd := exec.Command("go", "test", "./ui", "./examples/ui-gallery", "-run", "Test(Perf|GalleryPage)Budgets", "-v")
	budgetCmd.Env = append(os.Environ(), "TANGO_PERF_BUDGET=1")
	budgetOut, e := budgetCmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("budget: %w\n%s", e, budgetOut)
	}
	for _, line := range strings.Split(string(budgetOut), "\n") {
		b, e := parseBudgetLine(line)
		if e != nil {
			continue
		}
		for i := range results {
			if results[i].Name+"/"+results[i].Scene == b.Key {
				results[i].Budget = b.Budget
				results[i].Pass = b.Status == "ok"
			}
		}
	}
	// Cold starts are deliberately isolated in a fresh test process and are report-only.
	tmp, e := os.MkdirTemp(out, ".cold-")
	if e == nil {
		defer os.RemoveAll(tmp)
		bin := filepath.Join(tmp, "ui.test")
		if e = exec.Command("go", "test", "-c", "./ui", "-o", bin).Run(); e == nil {
			seen := map[string]bool{}
			for i := range results {
				if seen[results[i].Name] {
					continue
				}
				seen[results[i].Name] = true
				cmd := exec.Command(bin, "-test.run", "^$", "-test.bench", "^BenchmarkComponents/"+results[i].Name+"/MountCold$", "-test.benchtime", "1x")
				x, ce := cmd.CombinedOutput()
				if ce == nil {
					for _, line := range strings.Split(string(x), "\n") {
						if b, pe := parseBenchLine(line); pe == nil {
							for j := range results {
								if results[j].Name == b.Name {
									results[j].ColdNS = b.NsOp
								}
							}
							break
						}
					}
				}
			}
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name+results[i].Scene < results[j].Name+results[j].Scene })
	_ = time.Second // keep timing policy visible at this command boundary
	return writeBenchReport(out, results)
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
func coldBenchmark(out, component string) {} // reserved for the process-isolated cold pass
var _ = filepath.Join
