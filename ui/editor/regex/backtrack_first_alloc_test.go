//go:build !race

package regex

import (
	"strings"
	"testing"
)

func TestBacktrackRejectedFirstByteAllocationBudget(t *testing.T) {
	for _, flags := range []string{"", "i"} {
		re := mustCompile(t, `[a-z](?=:)`, flags)
		input := strings.Repeat("!", 200)
		var scratch [2]int
		for _, method := range []struct {
			name string
			call func() ([]int, error)
		}{
			{"capture", func() ([]int, error) { return re.TryMatchStart(input, nil) }},
			{"span", func() ([]int, error) { return TryMatchStartNoCaptures(re, input, nil, scratch[:0]) }},
			{"match", func() ([]int, error) { return re.MatchStart(input), nil }},
			{"runes", func() ([]int, error) { return re.MatchStartRunes(input, nil), nil }},
		} {
			t.Run(method.name+flags, func(t *testing.T) {
				allocs := testing.AllocsPerRun(100, func() {
					m, err := method.call()
					if m != nil || err != nil {
						panic("impossible first byte matched or returned an error")
					}
				})
				if allocs != 0 {
					t.Fatalf("rejected first byte allocates %.0f times, want zero", allocs)
				}
			})
		}
	}
}
