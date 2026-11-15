package buffer

import (
	"fmt"
	"strings"
	"testing"
)

func mygoCompareLargeText(n int) string {
	var s strings.Builder
	for i := range n {
		fmt.Fprintf(&s, "func f%d() int { return %d }\n", i, i)
	}
	return s.String()
}

// BenchmarkMyGoCompareBufferEditLarge matches MyGo's 100,000-line midpoint
// typing benchmark so the core buffer path can be compared directly.
func BenchmarkMyGoCompareBufferEditLarge(b *testing.B) {
	buf := New(mygoCompareLargeText(100_000))
	at := buf.Snapshot().Len() / 2
	b.ReportAllocs()
	for b.Loop() {
		if _, err := buf.Edit([]Edit{{Start: at, End: at, Text: "x"}}, nil, nil); err != nil {
			b.Fatal(err)
		}
		at++
	}
}
