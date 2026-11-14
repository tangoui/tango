//go:build darwin

package editor

// Measurement-only benchmarks added for the milestone 2 acceptance report
// (docs/plans/2026-11-14-editor-m2-acceptance.md Task 10):
//
//   - BenchmarkScrollPartial: BenchmarkScroll/scale2 on the partial-redraw
//     path (a Retainer device, as in an application), where the View's
//     ScrollRegion hints become copies of the retained target and only the
//     newly exposed rows are drawn — the work that Monaco's scrolling does
//     (tools/monaco-bench). It also reports the CPU part of every frame on
//     its own (cpu-p50/p99, cpu-fps), the analogue of Monaco's main-thread
//     "work" (which leaves out raster and GPU composition).
//   - BenchmarkTypeAt1MLinesCPU: BenchmarkTypeAt1MLines/mid with the CPU
//     part of every keystroke's frame as a distribution (cpu-p50/p95), the
//     analogue of Monaco's input "work".
//   - BenchmarkTypeTS20k / BenchmarkScrollTS20k: typing and scrolling in the
//     20 000-line TypeScript file (below every large-file limit, so the
//     display features turned off on the 1M-line file are active), for the
//     per-feature cost on non-large files (TANGO_EDITOR_PERF_OFF).

import (
	"os"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// copyCountDevice is a retainDevice that also counts the partial frames
// carrying copies (ScrollRegion hints the Renderer turned into moves of
// the retained pixels).
type copyCountDevice struct {
	retainDevice
	copies int
}

func (c *copyCountDevice) Execute(f *gpu.Frame) error {
	if len(f.Copies) > 0 {
		c.copies++
	}
	return c.retainDevice.Execute(f)
}

// BenchmarkScrollPartial is BenchmarkScroll/scale2 (1M-line file, from line
// 100 000, a wheel event of two lines per frame) with partial redraws: the
// host of newPartialPerfHost with a device counting the frames with
// copies. fps/mean-ms/p99-ms are event to GPU done (as BenchmarkScroll);
// cpu-* are the event handling plus Runner.Frame (build, layout, paint,
// encode, submit) of each frame, without the GPU wait; partial and copied
// are the fractions of frames drawn partially and with copies.
func BenchmarkScrollPartial(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	st := NewState(string(data), path, theme.Base("vs"))
	perfApplyOffEnv(b, st)
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: 2}}
	h.dev = &syncDevice{d: metalDevice(b), keep: true}
	dev := &copyCountDevice{retainDevice: retainDevice{h.dev}}
	ren := gpu.NewRenderer(dev)
	ren.SetVerify(false)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st).Layout(layout.Grow(0), layout.H(perfSize.H-1))
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	h.r.Attach(h.w)
	h.frame(b)
	h.caretTo(b, 100_000)
	for range 3 {
		h.frame(b)
	}
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * st.Viewport.LineHeight
	var d, cpu durations
	top := st.Viewport.TopLine
	f0, p0, c0 := h.dev.frames, h.dev.partial, dev.copies
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.w.invalid.Store(false)
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		if err := h.r.Frame(); err != nil {
			b.Fatal(err)
		}
		t1 := time.Now()
		if err := h.dev.wait(); err != nil {
			b.Fatal(err)
		}
		end := time.Now()
		h.cpuT += t1.Sub(start)
		h.waitT += end.Sub(t1)
		h.frames++
		cpu = append(cpu, t1.Sub(start))
		d = append(d, end.Sub(start))
	}
	if st.Viewport.TopLine <= top {
		b.Fatalf("did not scroll: TopLine %d", st.Viewport.TopLine)
	}
	b.ReportMetric(1000/ms(d.mean()), "fps")
	b.ReportMetric(ms(d.mean()), "mean-ms")
	b.ReportMetric(ms(d.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	b.ReportMetric(1000/ms(cpu.mean()), "cpu-fps")
	b.ReportMetric(ms(cpu.pct(0.5)), "cpu-p50-ms")
	b.ReportMetric(ms(cpu.pct(0.99)), "cpu-p99-ms")
	h.reportSplit(b)
	if n := h.dev.frames - f0; n > 0 {
		b.ReportMetric(float64(h.dev.partial-p0)/float64(n), "partial")
		b.ReportMetric(float64(dev.copies-c0)/float64(n), "copied")
	}
}

// openTS20k opens the 20 000-line TypeScript file (perfFile) with full
// redraws and highlights all of it.
func openTS20k(b *testing.B) *perfHost {
	path := perfFile(b, 20_000)
	h := openFile(b, path, 2)
	h.frame(b)
	highlightAllBackground(b, h.st)
	h.frame(b)
	return h
}

// BenchmarkTypeTS20k types (keyAt) at line 10 000 of the 20 000-line
// TypeScript file, fully highlighted, full redraws (as TypeAt1MLines).
func BenchmarkTypeTS20k(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	h := openTS20k(b)
	h.focus(b)
	h.caretTo(b, 10_000)
	typeBench(b, h)
}

// BenchmarkScrollTS20k is BenchmarkScroll (scale 2, two lines per frame,
// full redraws) in the 20 000-line TypeScript file, from line 2 000.
func BenchmarkScrollTS20k(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	h := openTS20k(b)
	h.caretTo(b, 2_000)
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * h.st.Viewport.LineHeight
	var d durations
	top := h.st.Viewport.TopLine
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.frame(b)
		d = append(d, time.Since(start))
	}
	if h.st.Viewport.TopLine <= top {
		b.Fatalf("did not scroll: TopLine %d", h.st.Viewport.TopLine)
	}
	b.ReportMetric(1000/ms(d.mean()), "fps")
	b.ReportMetric(ms(d.mean()), "mean-ms")
	b.ReportMetric(ms(d.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	h.reportSplit(b)
}

// BenchmarkTypeAt1MLinesCPU is BenchmarkTypeAt1MLines/mid (full redraws)
// that also reports the CPU part of each keystroke's frame (the key events
// plus Runner.Frame, without the GPU wait) as a distribution: cpu-p50/p95,
// the analogue of Monaco's input "work" (tools/monaco-bench), which leaves
// out raster and GPU composition.
func BenchmarkTypeAt1MLinesCPU(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	h := openFile(b, path, 2)
	h.frame(b)
	h.focus(b)
	h.caretTo(b, 500_000)
	n := h.st.Snapshot().LineCount()
	var d, cpu durations
	i := 0
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.w.invalid.Store(false)
		h.typeKey(keyAt(i))
		if err := h.r.Frame(); err != nil {
			b.Fatal(err)
		}
		t1 := time.Now()
		if err := h.dev.wait(); err != nil {
			b.Fatal(err)
		}
		end := time.Now()
		h.cpuT += t1.Sub(start)
		h.waitT += end.Sub(t1)
		h.frames++
		cpu = append(cpu, t1.Sub(start))
		d = append(d, end.Sub(start))
		i++
	}
	if got := h.st.Snapshot().LineCount(); got != n+i/40 {
		b.Fatalf("line count %d after %d keys, want %d", got, i, n+i/40)
	}
	b.ReportMetric(ms(d.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(d.pct(0.95)), "p95-ms")
	b.ReportMetric(ms(cpu.pct(0.5)), "cpu-p50-ms")
	b.ReportMetric(ms(cpu.pct(0.95)), "cpu-p95-ms")
	h.reportSplit(b)
}
