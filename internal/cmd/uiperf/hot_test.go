package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePprofTop(t *testing.T) {
	in := `Showing nodes accounting for 9.50s, 95% of 10s total
      flat  flat%   sum%        cum   cum%  Name
     1.00s 10.00% 10.00%      5.00s 50.00%  github.com/tangoui/tango/internal/render.Paint
     0.50s  5.00% 15.00%      3.00s 30.00%  github.com/tangoui/tango/ui.Button`
	got, err := parsePprofTop(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name == "" || got[0].CumPct != 50 || got[1].CumPct != 30 {
		t.Fatalf("got %+v", got)
	}
}
func TestHotScore(t *testing.T) {
	items := []hotInput{{Component: "A", CumPct: 50, Over: 2, Function: "F"}, {Component: "B", CumPct: 30, Over: 1.5, Function: "F"}, {Component: "C", CumPct: 10, Over: 2, Function: "github.com/tangoui/tango/internal/render.X"}}
	got := hotScores(items)
	if got[0].Function != "F" || got[0].Score != 1.45 {
		t.Fatalf("got %+v", got)
	}
	if !got[1].NonTarget {
		t.Fatalf("expected non-target: %+v", got)
	}
}

func TestHotBenchRegexQuotesNames(t *testing.T) {
	got := hotBenchRegex("A+B", "Scene.(x)")
	if got == "^BenchmarkComponents/A+B/Scene.(x)$" {
		t.Fatal("regex was not quoted")
	}
}

func TestParsePprofTopKeepsFlatZeroRows(t *testing.T) {
	in := `File: ui.test
Type: cpu
Showing nodes accounting for 9.50s, 95.00% of 10s total
      flat  flat%   sum%        cum   cum%
         0     0%     0%      8.12s 81.20%  github.com/tangoui/tango/ui.(*Runner).Frame
     1.20s 12.00% 12.00%      6.00s 60.00%  github.com/tangoui/tango/ui/flexbox.Solve
         0     0% 12.00%      4.50s 45.00%  github.com/tangoui/tango/ui.(*Runner).layoutAndPaint (inline)
    10.50ms  0.11% 12.11%   900.25us  9.00%  github.com/tangoui/tango/ui.measure`
	got, err := parsePprofTop(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d rows, want 4: %+v", len(got), got)
	}
	if got[0].Name != "github.com/tangoui/tango/ui.(*Runner).Frame" || got[0].FlatNS != 0 || math.Abs(got[0].CumNS-8.12e9) > 1 || got[0].CumPct != 81.2 {
		t.Fatalf("flat=0 row parsed as %+v", got[0])
	}
	if got[2].Name != "github.com/tangoui/tango/ui.(*Runner).layoutAndPaint (inline)" || got[2].FlatNS != 0 || got[2].CumPct != 45 {
		t.Fatalf("flat=0 inline row parsed as %+v", got[2])
	}
	if math.Abs(got[1].FlatNS-1.2e9) > 1 || math.Abs(got[3].FlatNS-10.5e6) > 1 || math.Abs(got[3].CumNS-900250) > 1 {
		t.Fatalf("units: %+v", got)
	}
}

func TestParsePprofTopFlatPct(t *testing.T) {
	in := `      flat  flat%   sum%        cum   cum%
     0.32s  4.27% 14.53%      1.60s 21.33%  github.com/go-text/typesetting/font/opentype/tables.PairSet.FindGlyph`
	got, err := parsePprofTop(in)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].FlatPct != 4.27 || got[0].CumPct != 21.33 {
		t.Fatalf("got %+v", got[0])
	}
}

func TestClassifyFrame(t *testing.T) {
	const m = "github.com/tangoui/tango/"
	cases := map[string]frameKind{
		"testing.(*B).runN":                                       frameHarness,
		"testing.(*B).run1.func1":                                 frameHarness,
		"runtime.systemstack":                                     frameHarness,
		"runtime.gcBgMarkWorker":                                  frameHarness,
		"runtime.mallocgc":                                        frameComponent,
		"runtime.gcDrain":                                         frameComponent,
		"runtime.madvise":                                         frameComponent,
		"runtime.mallocgc (inline)":                               frameComponent,
		"main.main":                                               frameHarness,
		m + "ui.perfScenes.func2.func1.1":                         frameHarness,
		m + "ui.perfStep":                                         frameHarness,
		m + "ui.perfTimeFrame":                                    frameHarness,
		m + "ui.perfNavClick":                                     frameHarness,
		m + "ui.benchPerfScene":                                   frameHarness,
		m + "ui.BenchmarkComponents.func1.1":                      frameHarness,
		m + "ui.(*Driver).Frame":                                  frameHarness,
		m + "ui.(*Driver).Settle":                                 frameHarness,
		m + "ui.(*Runner).Frame":                                  framePipeline,
		m + "ui.(*Runner).layoutAndPaint (inline)":                framePipeline,
		m + "ui.(*TabsW).tab.func10":                              frameComponent,
		m + "ui/internal/flexbox.measureUncached":                 frameComponent,
		m + "ui/text.Shape":                                       frameComponent,
		m + "ui.performLayout":                                    frameComponent,
		m + "ui.Signal[go.shape.*" + m + "ui.perfX].Get":          frameComponent,
		"github.com/go-text/typesetting/harfbuzz.(*Buffer).Shape": frameComponent,
	}
	for name, want := range cases {
		if got := classifyFrame(name); got != want {
			t.Errorf("classifyFrame(%q) = %v, want %v", name, got, want)
		}
	}
}

// hotFixture writes a minimal hot output directory: one over-budget result
// (2x budget) with an existing (empty) profile so -reuse-profiles can run
// without invoking go test.
func hotFixture(t *testing.T, name, scene string) string {
	t.Helper()
	dir := t.TempDir()
	bf := benchFile{Results: []benchResult{
		{Name: name, Scene: scene, NsOp: 200, Budget: 100},
		{Name: "Fast", Scene: "Mount", NsOp: 50, Budget: 100},
	}}
	data, err := json.Marshal(bf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bench.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "prof"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prof", name+"-"+scene+".pprof"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fakePprof replaces the pprof runner with one that returns the fixture rows
// truncated to the requested nodecount, as `pprof -top -nodecount=N` does, and
// fails the test if a benchmark would be re-run.
func fakePprof(t *testing.T, rows []string) {
	t.Helper()
	oldTop, oldProf := pprofTopText, profileBench
	t.Cleanup(func() { pprofTopText, profileBench = oldTop, oldProf })
	pprofTopText = func(prof string, nodecount int) ([]byte, error) {
		if _, err := os.Stat(prof); err != nil {
			return nil, err
		}
		n := len(rows)
		if nodecount < n {
			n = nodecount
		}
		head := "File: ui.test\nType: cpu\n      flat  flat%   sum%        cum   cum%\n"
		return []byte(head + strings.Join(rows[:n], "\n") + "\n"), nil
	}
	profileBench = func(regex, prof string) ([]byte, error) {
		t.Fatalf("profileBench called in reuse mode (%s)", regex)
		return nil, nil
	}
}

func readHot(t *testing.T, path string) (main, flat, pipeline string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	fi := strings.Index(s, "## Top by flat")
	pi := strings.Index(s, "## Pipeline frames")
	if fi < 0 || pi < 0 || fi > pi {
		t.Fatalf("missing/misordered sections:\n%s", s)
	}
	return tableRows(s[:fi]), tableRows(s[fi:pi]), tableRows(s[pi:])
}

// tableRows keeps the header and data rows of markdown tables, dropping the
// prose (which names the excluded prefixes).
func tableRows(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "|") {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func TestRunHotReuseExcludesHarnessAndMarksPipeline(t *testing.T) {
	const m = "github.com/tangoui/tango/"
	dir := hotFixture(t, "Tabs", "Large100k")
	fakePprof(t, []string{
		"         0     0%     0%      5.12s 68.27%  " + m + "ui.(*Driver).Frame",
		"         0     0%     0%      5.12s 68.27%  " + m + "ui.(*Runner).Frame",
		"     0.14s  1.87%  1.87%      4.97s 66.27%  " + m + "ui/internal/flexbox.flex",
		"         0     0%  1.87%      4.96s 66.13%  " + m + "ui.(*Runner).layoutAndPaint",
		"         0     0%  2.27%      4.91s 65.47%  " + m + "ui.benchPerfScene",
		"     0.01s  0.13%  2.40%      4.78s 63.73%  " + m + "ui.(*TabsW).tab.func10",
		"         0     0%  2.67%      4.71s 62.80%  " + m + "ui.BenchmarkComponents.func1.1",
		"         0     0%  2.67%      4.52s 60.27%  testing.(*B).runN",
		"         0     0%  4.40%      3.54s 47.20%  " + m + "ui.perfStep",
		"         0     0%  4.40%      3.49s 46.53%  " + m + "ui.perfScenes.func2.func1.1",
		"         0     0%  9.20%      2.01s 26.80%  runtime.systemstack",
		"     0.32s  4.27% 14.53%      1.60s 21.33%  github.com/go-text/typesetting/font/opentype/tables.PairSet.FindGlyph",
		"     0.02s  0.27% 18.80%      1.26s 16.80%  runtime.gcDrain",
		"     0.05s  0.67% 19.47%      0.90s 12.00%  runtime.mallocgc",
		"     0.40s  5.33% 24.80%      0.40s  5.33%  runtime.memmove",
		"         0     0% 24.80%      0.30s  4.00%  main.main",
	})
	if err := hotCLI([]string{"-out", dir, "-reuse-profiles", "-md", "hot-f2.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hot.md")); err == nil {
		t.Fatal("-md hot-f2.md must not write hot.md")
	}
	mainTbl, flatTbl, pipe := readHot(t, filepath.Join(dir, "hot-f2.md"))
	all := mainTbl + flatTbl + pipe
	for _, h := range []string{"(*Driver).Frame", "benchPerfScene", "BenchmarkComponents", "testing.", "perfStep", "perfScenes", "runtime.systemstack", "runtime.memmove", "main.main"} {
		if strings.Contains(all, h) {
			t.Errorf("harness frame %q not excluded:\n%s", h, all)
		}
	}
	for _, k := range []string{"flexbox.flex`", "(*TabsW).tab.func10`", "PairSet.FindGlyph`", "runtime.gcDrain`", "runtime.mallocgc`"} {
		if !strings.Contains(mainTbl, k) {
			t.Errorf("component frame %q missing from main ranking:\n%s", k, mainTbl)
		}
	}
	if strings.Contains(mainTbl, "(*Runner)") || strings.Contains(flatTbl, "(*Runner)") {
		t.Errorf("pipeline frame ranked as component hotspot:\n%s%s", mainTbl, flatTbl)
	}
	if !strings.Contains(pipe, "(*Runner).Frame`") || !strings.Contains(pipe, "(*Runner).layoutAndPaint`") || !strings.Contains(pipe, "| pipeline |") {
		t.Errorf("pipeline frames not listed as pipeline:\n%s", pipe)
	}
	// Main ranking: score = cum%/100 × over (2×); flat% sits next to cum%.
	if !strings.Contains(mainTbl, "| Function | Score | cum% | flat% |") {
		t.Errorf("main table header:\n%s", mainTbl)
	}
	if !strings.Contains(mainTbl, "| `"+m+"ui/internal/flexbox.flex` | 1.3254 | 66.27 | 1.87 |") {
		t.Errorf("flexbox.flex row:\n%s", mainTbl)
	}
	if strings.Index(mainTbl, "flexbox.flex`") > strings.Index(mainTbl, "PairSet.FindGlyph`") {
		t.Errorf("main ranking not by cum score:\n%s", mainTbl)
	}
}

func TestRunHotDefaultNodeCountKeepsDeepLeaf(t *testing.T) {
	const m = "github.com/tangoui/tango/"
	var rows []string
	for i := 0; i < 40; i++ {
		cum := 90.0 - float64(i)
		name := fmt.Sprintf("%sui.chain%02d", m, i)
		if i == 34 {
			name = m + "ui.(*CommandW).item.func1"
		}
		rows = append(rows, fmt.Sprintf("     0.01s  0.10%%  1.00%%      1.00s %.2f%%  %s", cum, name))
	}
	leaf := "(*CommandW).item.func1`"

	dir := hotFixture(t, "Command", "Large100k")
	fakePprof(t, rows)
	if err := hotCLI([]string{"-out", dir, "-reuse-profiles"}); err != nil {
		t.Fatal(err)
	}
	mainTbl, _, _ := readHot(t, filepath.Join(dir, "hot.md"))
	if !strings.Contains(mainTbl, leaf) {
		t.Fatalf("leaf at position 35 missing with default nodecount:\n%s", mainTbl)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "hot.md")); !strings.Contains(string(data), "nodecount=200") {
		t.Errorf("hot.md should record nodecount=200:\n%s", mainTbl)
	}

	dir30 := hotFixture(t, "Command", "Large100k")
	if err := hotCLI([]string{"-out", dir30, "-reuse-profiles", "-top", "30"}); err != nil {
		t.Fatal(err)
	}
	mainTbl, _, _ = readHot(t, filepath.Join(dir30, "hot.md"))
	if strings.Contains(mainTbl, leaf) {
		t.Fatalf("fixture sanity: leaf should be cut at nodecount=30:\n%s", mainTbl)
	}
}

func TestRunHotFlatTable(t *testing.T) {
	const m = "github.com/tangoui/tango/"
	dir := hotFixture(t, "Tabs", "Large100k")
	fakePprof(t, []string{
		"         0     0%     0%      5.12s 68.27%  " + m + "ui.(*Runner).Frame",
		"     0.14s  1.87%  1.87%      4.97s 66.27%  " + m + "ui/internal/flexbox.flex",
		"         0     0%  1.87%      4.95s 66.00%  " + m + "ui/internal/flexbox.measureAxis",
		"     0.32s  4.27% 14.53%      1.60s 21.33%  github.com/go-text/typesetting/font/opentype/tables.PairSet.FindGlyph",
		"     0.30s  4.00% 18.53%      1.28s 17.07%  github.com/go-text/typesetting/font/opentype/tables.pairValueRecords.get",
	})
	if err := hotCLI([]string{"-out", dir, "-reuse-profiles"}); err != nil {
		t.Fatal(err)
	}
	_, flatTbl, _ := readHot(t, filepath.Join(dir, "hot.md"))
	if !strings.Contains(flatTbl, "| Function | Flat score | flat% | cum% |") {
		t.Fatalf("flat table header:\n%s", flatTbl)
	}
	// Flat score = flat%/100 × over (2×).
	if !strings.Contains(flatTbl, "| `github.com/go-text/typesetting/font/opentype/tables.PairSet.FindGlyph` | 0.0854 | 4.27 | 21.33 |") {
		t.Fatalf("FindGlyph flat row:\n%s", flatTbl)
	}
	a := strings.Index(flatTbl, "PairSet.FindGlyph`")
	b := strings.Index(flatTbl, "pairValueRecords.get`")
	c := strings.Index(flatTbl, "flexbox.flex`")
	if !(a >= 0 && a < b && b < c) {
		t.Fatalf("flat table not ordered by flat score:\n%s", flatTbl)
	}
	if strings.Contains(flatTbl, "measureAxis") {
		t.Fatalf("flat=0 rows belong only in the cum ranking:\n%s", flatTbl)
	}
}
