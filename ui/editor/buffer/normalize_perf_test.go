package buffer

import (
	"strings"
	"testing"
)

func TestNormalizeMixedEOLSingleOutputAllocation(t *testing.T) {
	src := strings.Repeat("alpha\r\nbeta\rgamma\n", 128)
	if !strings.ContainsRune(src, '\r') {
		t.Fatal("fixture has no CR")
	}
	want := modelEOL.Replace(src)
	allocs := testing.AllocsPerRun(100, func() {
		if got := normalizeEOL(src); got != want {
			panic("wrong normalization")
		}
	})
	if allocs > 1 {
		t.Fatalf("mixed CRLF/CR allocates %.0f outputs, want one", allocs)
	}
}

func BenchmarkNormalizeEOL(b *testing.B) {
	for _, tc := range []struct{ name, line string }{
		{"lf", "alpha beta gamma\n"},
		{"crlf", "alpha beta gamma\r\n"},
		{"mixed", "alpha\r\nbeta\rgamma\n"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			src := strings.Repeat(tc.line, 65536)
			want := modelEOL.Replace(src)
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if normalizeEOL(src) != want {
					b.Fatal("wrong normalization")
				}
			}
		})
	}
}

func TestNormalizeEOLPreservesGeneratedBytes(t *testing.T) {
	parts := []string{"a", "\r", "\n", "\r\n", "中", "😀", "\x00", "\xff"}
	var seed uint32 = 143
	for n := 0; n < 4000; n++ {
		src := ""
		for i := 0; i < n%35; i++ {
			seed = seed*1664525 + 1013904223
			src += parts[int(seed>>8)%len(parts)]
		}
		if got, want := normalizeEOL(src), modelEOL.Replace(src); got != want {
			t.Fatalf("normalize(%q)=%q want %q", src, got, want)
		}
		snap := NewSnapshot(src)
		if snap.String() != modelEOL.Replace(src) {
			t.Fatalf("snapshot text differs for %q", src)
		}
	}
}
