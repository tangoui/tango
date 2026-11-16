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
