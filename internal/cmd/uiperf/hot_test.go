package main

import "testing"

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
func TestCleanupWorktreeRejectsUnownedPath(t *testing.T) {
	if err := cleanupWorktree("/tmp", "relative-tree"); err == nil {
		t.Fatal("expected ownership/path error")
	}
}
