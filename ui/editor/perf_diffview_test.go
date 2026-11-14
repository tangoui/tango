//go:build darwin

package editor

import (
	"image/color"
	"os"
	"testing"
)

// Performance gates of the diff view's editor features
// (docs/specs/diff-view.md, 2.1 and 4 B3), on the 1M-line document of
// perf_test.go.

// BenchmarkTypeAt1MLinesDeco10k is BenchmarkTypeAt1MLines/mid with 10 000
// decorations on the document (budget: p95 ≤ 16 ms).
func BenchmarkTypeAt1MLinesDeco10k(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	if _, err := os.Stat(path); err != nil {
		b.Fatal(err)
	}
	h := openFile(b, path, 2)
	h.frame(b)
	snap := h.st.Snapshot()
	whole := &DecorationOptions{WholeLine: true, Background: color.NRGBA{0, 0x80, 0, 0x40}, Overview: &OverviewStyle{Color: color.NRGBA{0, 0x80, 0, 0xff}, Lane: 4}}
	inner := &DecorationOptions{Background: color.NRGBA{0x80, 0, 0, 0x40}}
	ds := make([]Decoration, 0, 10_000)
	for l := 0; l < 1_000_000; l += 100 {
		if l%200 == 0 {
			ds = append(ds, LineDecoration(snap, l, l, whole))
		} else if n := len(snap.Line(l)); n > 4 {
			ds = append(ds, Decoration{Start: snap.LineStart(l) + 1, End: snap.LineStart(l) + 4, Options: inner})
		}
	}
	h.st.Model.SetDecorations("diff", ds)
	h.focus(b)
	h.caretTo(b, 500_000)
	typeBench(b, h)
}
