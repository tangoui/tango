package regex

import (
	"slices"
	"testing"
)

var performanceMatch []int

func TestBacktrackASCIIAllocationBudget(t *testing.T) {
	re := mustCompile(t, `foo(?=(bar))`, "")
	rs := []rune("foobar")
	allocs := testing.AllocsPerRun(30, func() { performanceMatch, _ = re.TryMatchStart("foobar", rs) })
	if allocs > 9 {
		t.Fatalf("ASCII backtracking uses %.0f allocations, want <=9 without offset conversion buffer", allocs)
	}
}

func BenchmarkBacktrackASCIIOffsets(b *testing.B) {
	re, err := Compile(`foo(?=(bar))`, "")
	if err != nil {
		b.Fatal(err)
	}
	input := "foobar"
	runes := []rune(input)
	b.ReportAllocs()
	for b.Loop() {
		performanceMatch, _ = re.TryMatchStart(input, runes)
	}
}

func TestBacktrackCaptureByteOffsets(t *testing.T) {
	cases := []struct {
		pattern, input string
		want           []int
	}{
		{`foo(?=(bar))`, "foobar", []int{0, 3, 3, 6}},
		{`foo(?=(中))`, "foo中", []int{0, 3, 3, 6}},
		{`中(.)(?=(.+))`, "中文😀x", []int{0, 6, 3, 6, 6, 11}},
		{`(.)(?=(.))`, "\xffx", []int{0, 1, 0, 1, 1, 2}},
		{`(?=(x))`, "x", []int{0, 0, 0, 1}},
		{`foo(?=bar)`, "foobar\xff", []int{0, 3}},
	}
	for _, tc := range cases {
		re := mustCompile(t, tc.pattern, "")
		got, err := re.TryMatchStart(tc.input, []rune(tc.input))
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("%q on %q: got %v, %v; want %v", tc.pattern, tc.input, got, err, tc.want)
		}
	}
}
