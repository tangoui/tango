package buffer

import (
	"strings"
	"testing"
)

// bigText returns about 100 MB of text in 1,000,000 lines of 100 bytes each
// (99 visible bytes plus the line break eol).
func bigText(eol string) string {
	line := strings.Repeat("0123456789", 10)[:100-len(eol)] + eol
	return strings.Repeat(line, 1_000_000)
}

func BenchmarkInsertIn1MLines(b *testing.B) {
	s := NewSnapshot(bigText("\n"))
	if s.LineCount() != 1_000_001 {
		b.Fatalf("LineCount = %d", s.LineCount())
	}
	mid := s.LineStart(500_000) + 17
	edits := []Edit{{Start: mid, End: mid, Text: "x"}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Apply(edits); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewSnapshot100MB(b *testing.B) {
	text := bigText("\n")
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		NewSnapshot(text)
	}
}

func BenchmarkNewSnapshot100MBCRLF(b *testing.B) {
	text := bigText("\r\n")
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		NewSnapshot(text)
	}
}

func BenchmarkLineQueries1MLines(b *testing.B) {
	s := NewSnapshot(bigText("\n"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i = (i + 7919) % 1_000_000
		_ = s.Line(i)
		_ = s.LineOf(s.LineStart(i) + 5)
	}
}

// BenchmarkReplaceAcrossLeaves exercises the general edit path: a 10 KB
// deletion spanning several leaves plus a pasted line, in a 100 MB document.
func BenchmarkReplaceAcrossLeaves(b *testing.B) {
	s := NewSnapshot(bigText("\n"))
	start := s.LineStart(500_000) + 3
	edits := []Edit{{Start: start, End: start + 10_000, Text: "pasted line\n"}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Apply(edits); err != nil {
			b.Fatal(err)
		}
	}
}
