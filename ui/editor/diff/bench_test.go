package diff

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
)

func benchLines(b *testing.B, a, m []string, opts Options) {
	b.ReportAllocs()
	for b.Loop() {
		res := Lines(bg, a, m, opts)
		if res.TimedOut {
			b.Fatal("timed out")
		}
	}
}

// BenchmarkLargeFileSmallEdit: 100k lines, one changed and one inserted
// line far apart.
func BenchmarkLargeFileSmallEdit(b *testing.B) {
	a := bigFile(100_000)
	m := slices.Clone(a)
	m[30_001] = "\tx = x*9 + compute(x, \"edited\")"
	m = slices.Insert(m, 70_000, "\t// added")
	benchLines(b, a, m, Options{Timeout: DefaultTimeout})
}

// BenchmarkSnapshotsLargeFile is the same through buffer.Snapshot.
func BenchmarkSnapshotsLargeFile(b *testing.B) {
	a := buffer.NewSnapshot(strings.Join(bigFile(100_000), "\n"))
	off := a.LineStart(50_001)
	m, err := a.Apply([]buffer.Edit{{Start: off, End: off, Text: "// edited "}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		Snapshots(bg, a, m, Options{Timeout: DefaultTimeout})
	}
}

// BenchmarkScatteredEdits: 10k lines with an edit every 100 lines, so the
// whole file goes through Myers.
func BenchmarkScatteredEdits(b *testing.B) {
	a := bigFile(10_000)
	m := slices.Clone(a)
	for i := 3; i < len(m); i += 100 {
		m[i] += " // touched"
	}
	benchLines(b, a, m, Options{Timeout: DefaultTimeout})
}

// BenchmarkSmallFile: a 300-line file with a few edits (dynamic
// programming path), with move detection.
func BenchmarkSmallFile(b *testing.B) {
	a := bigFile(300)
	m := slices.Clone(a)
	m[20] = "\tx = x*2 + compute(x, \"edit\")"
	m = slices.Insert(m, 150, "\tlog(x)", "\tlog(x + 1)")
	m = slices.Delete(m, 250, 255)
	benchLines(b, a, m, Options{Timeout: DefaultTimeout, ComputeMoves: true})
}

// BenchmarkLongLine: one 400 KB line with an insertion in the middle.
func BenchmarkLongLine(b *testing.B) {
	long := strings.Repeat("abc def ", 50_000)
	mod := long[:200_000] + "XYZ" + long[200_000:]
	benchLines(b, []string{long}, []string{mod}, Options{Timeout: DefaultTimeout})
}
