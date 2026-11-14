//go:build darwin

package editor

import (
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
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

// BenchmarkScrollZones1000 is BenchmarkScroll (scale 2) with 1000 view
// zones (docs/specs/diff-view.md, 4 B3: p99 ≤ 33 ms): from line 100 000
// of the 1M-line file, a zone of 1 to 3 rows every 20 lines, half of them
// without a Version (their drawing fingerprinted every frame), all
// drawing a few fills and a line, so every frame shows several.
func BenchmarkScrollZones1000(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	st := NewState(string(data), path, theme.Base("vs"))
	h := newPerfHost(b, st, 2)
	h.frame(b)
	fill := color.NRGBA{0x80, 0x80, 0xc0, 0xff}
	paintZone := func(l *paint.List, z ZonePaint) {
		for k := range int(z.Rect.H / z.RowH) {
			l.Fill(geom.Rect{X: z.Rect.X, Y: z.Rect.Y + float32(k)*z.RowH + 2, W: z.Rect.W, H: z.RowH - 4}, geom.Radii{}, fill)
		}
		l.Line(geom.Pt(z.OriginX, z.Rect.Y), geom.Pt(z.OriginX+z.Rect.W, z.Rect.Y+z.Rect.H), 1, fill)
	}
	zs := make([]ViewZone, 0, 1000)
	for i := range 1000 {
		zs = append(zs, ViewZone{ID: uint64(i), AfterLine: 100_000 + 20*i, Rows: 1 + i%3, Version: uint64(i % 2), Paint: paintZone, PaintGutter: paintZone})
	}
	st.SetViewZones("diff", zs)
	h.caretTo(b, 100_000)
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * st.Viewport.LineHeight
	var d durations
	top := st.Viewport.TopLine
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.frame(b)
		d = append(d, time.Since(start))
	}
	if st.Viewport.TopLine <= top {
		b.Fatalf("did not scroll: TopLine %d", st.Viewport.TopLine)
	}
	b.ReportMetric(1000/ms(d.mean()), "fps")
	b.ReportMetric(ms(d.mean()), "mean-ms")
	b.ReportMetric(ms(d.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	h.reportSplit(b)
}
