package main

import (
	"math"
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
