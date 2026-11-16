package main

import "testing"

func TestABVerdict(t *testing.T) {
	cases := []struct {
		name       string
		base, head []abMetric
		want       string
	}{
		{"keep", []abMetric{{100, 10}}, []abMetric{{88, 10}}, "keep"},
		{"regress", []abMetric{{100, 10}}, []abMetric{{88, 10.6}}, "regress"},
		{"noise", []abMetric{{100, 10}}, []abMetric{{98, 10}}, "noise"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := abVerdict(tc.base, tc.head); got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestParseABOutputGroupsNames(t *testing.T) {
	rows, err := parseABOutput("BenchmarkButton-8  1  100 ns/op  1 B/op  2 allocs/op\nBenchmarkTable-8  1  200 ns/op  2 B/op  3 allocs/op\n")
	if err != nil || len(rows) != 2 || rows[0].Name != "BenchmarkButton" || rows[1].Name != "BenchmarkTable" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}
