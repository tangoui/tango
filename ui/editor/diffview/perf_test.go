//go:build darwin

package diffview

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/metal"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Performance gates of the diff view (docs/specs/diff-view.md, 4 C7), on
// the Metal device at 1920×1080, scale 2, every frame drawn in full and
// waited for (the conditions of ui/editor's perf_test.go):
//
//   - BenchmarkDiffScroll: both sides 100 000 lines with 1000 changes,
//     side by side, the wheel scrolling the modified side two lines per
//     frame (the original follows): p95 ≤ 16 ms.
//   - TestDiffTypingABAB: keystrokes in the modified side in bursts that
//     start while the debounced recomputation runs in the background (or
//     just delivered its result), against the same view with the
//     recomputation held back, the arms alternating over rounds: the
//     median ratio of the p95s ≤ 1.10. Run with DIFFVIEW_PERF=1
//     (DIFFVIEW_PERF_ROUNDS: rounds, 8).

var perfSize = geom.Size{W: 1920, H: 1080}

type perfWindow struct {
	size    geom.Size
	invalid atomic.Bool
}

func (w *perfWindow) Size() geom.Size              { return w.size }
func (w *perfWindow) Scale() float32               { return 2 }
func (w *perfWindow) Invalidate()                  { w.invalid.Store(true) }
func (w *perfWindow) InvalidateAt(time.Time)       {}
func (w *perfWindow) SetCursor(input.Cursor)       {}
func (w *perfWindow) ReadClipboard() string        { return "" }
func (w *perfWindow) WriteClipboard(string)        {}
func (w *perfWindow) SetTextInput(bool, geom.Rect) {}
func (w *perfWindow) Present(*image.RGBA) error    { return nil }

// syncDevice is the Metal device with a wait for the last frame (see
// ui/editor's perf_test.go).
type syncDevice struct {
	d    *metal.Device
	last gpu.Frame
}

func (s *syncDevice) Execute(f *gpu.Frame) error {
	s.last = gpu.Frame{Size: f.Size, Clear: f.Clear, MaskSize: f.MaskSize, ColorSize: f.ColorSize, RampSize: f.RampSize}
	return s.d.Execute(f)
}

func (s *syncDevice) wait() error {
	if s.last.Size.X == 0 {
		return nil
	}
	return s.d.Execute(&s.last)
}

var perfDev struct {
	once sync.Once
	d    *metal.Device
	err  error
}

func metalDevice(tb testing.TB) *metal.Device {
	tb.Helper()
	perfDev.once.Do(func() { perfDev.d, perfDev.err = metal.NewDevice() })
	if err := perfDev.err; err != nil {
		if errors.Is(err, metal.ErrUnsupported) || errors.Is(err, metal.ErrNoDevice) {
			tb.Skipf("no Metal device: %v", err)
		}
		tb.Fatal(err)
	}
	return perfDev.d
}

// perfHost shows a page on the Metal device.
type perfHost struct {
	w   *perfWindow
	r   *ui.Runner
	dev *syncDevice
	cpu durations // Runner.Frame times of the timed frames (no GPU wait)
}

func newPerfHost(tb testing.TB, page func(c *ui.Context)) *perfHost {
	h := &perfHost{w: &perfWindow{size: perfSize}, dev: &syncDevice{d: metalDevice(tb)}}
	ren := gpu.NewRenderer(h.dev)
	ren.SetVerify(false)
	h.r = ui.NewRunner(page, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	h.r.Attach(h.w)
	return h
}

func (h *perfHost) frame(tb testing.TB) {
	h.w.invalid.Store(false)
	if err := h.r.Frame(); err != nil {
		tb.Fatal(err)
	}
	if err := h.dev.wait(); err != nil {
		tb.Fatal(err)
	}
}

func (h *perfHost) timed(tb testing.TB) time.Duration {
	t0 := time.Now()
	h.w.invalid.Store(false)
	if err := h.r.Frame(); err != nil {
		tb.Fatal(err)
	}
	h.cpu = append(h.cpu, time.Since(t0))
	if err := h.dev.wait(); err != nil {
		tb.Fatal(err)
	}
	return time.Since(t0)
}

type durations []time.Duration

func (d durations) pct(p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	return s[int(float64(len(s)-1)*p)]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func loadAvg() string {
	b, err := syscall.Sysctl("vm.loadavg")
	if err != nil {
		return "?"
	}
	raw := []byte(b)
	for len(raw) < 24 {
		raw = append(raw, 0)
	}
	le := func(p []byte) uint32 { return uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24 }
	scale := float64(le(raw[16:]))
	if scale == 0 {
		return "?"
	}
	return fmt.Sprintf("%.2f %.2f %.2f", float64(le(raw[0:]))/scale, float64(le(raw[4:]))/scale, float64(le(raw[8:]))/scale)
}

// perfDocs returns the two sides: 100 000 TypeScript-like lines, and the
// same with a change every 100 lines (replaced, inserted, deleted lines in
// turn): 1000 changes.
func perfDocs() (orig, mod string) {
	lines := make([]string, 100_000)
	for i := range lines {
		lines[i] = fmt.Sprintf("  const value%d = compute(%d, \"row %d\") + offset * %d; // note %d", i, i, i, i%97, i)
	}
	return strings.Join(lines, "\n") + "\n", strings.Join(modify(lines, 100), "\n") + "\n"
}

// newPerfDiff makes the diff view of perfDocs and waits for its result.
func newPerfDiff(tb testing.TB) *State {
	o, m := perfDocs()
	reg := editor.DefaultRegistry()
	d := New(editor.NewModel("a.ts", o, reg), editor.NewModel("b.ts", m, reg), theme.Base("vs"))
	d.Options.UseInlineWhenSpaceIsLimited = false
	d.Options.MaxComputationTime = time.Minute
	d.run.wait()
	return d
}

func BenchmarkDiffScroll(b *testing.B) {
	useTestText(b)
	metalDevice(b)
	b.Logf("load average: %s, GOMAXPROCS %d", loadAvg(), runtime.GOMAXPROCS(0))
	d := newPerfDiff(b)
	defer d.Close()
	h := newPerfHost(b, func(c *ui.Context) { View(c, d) })
	for range 3 {
		h.frame(b)
	}
	if r, ok := d.Result(); !ok || len(r.Changes) != 1000 {
		b.Fatalf("%d changes, up to date %v", len(r.Changes), ok)
	}
	d.Modified.SetLineAtTop(10_000, 0)
	h.frame(b)
	h.frame(b)
	p := geom.Pt(perfSize.W*3/4, perfSize.H/2)
	dy := 2 * d.Modified.Viewport.LineHeight
	var ds durations
	top := d.Modified.ScrollTop()
	for b.Loop() {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		ds = append(ds, h.timed(b))
	}
	if d.Modified.ScrollTop() <= top {
		b.Fatal("did not scroll")
	}
	checkAligned(b, d, "after scrolling")
	b.ReportMetric(ms(ds.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(ds.pct(0.95)), "p95-ms")
	b.ReportMetric(ms(ds.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(ds.pct(1)), "max-ms")
}

// typing scenarios of TestDiffTypingABAB.
type typing int

const (
	// typeIdle: the recomputation is held back (Debounce of an hour):
	// the diff view without background work, the B arm.
	typeIdle typing = iota
	// typePaused: the default debounce, the keys in bursts of 10 after
	// untimed pauses of 210 to 300 ms, so that each burst starts while a
	// recomputation runs (it takes about 55 ms for these texts) or right
	// after its result arrived: the A arm of the gate.
	typePaused
	// typeBusy: Debounce 0, every key cancels and restarts the
	// recomputation (the worst case, reported, not gated).
	typeBusy
)

// typeRound types n keys (an Enter every 40th) into the modified side of
// d, in the conditions of scenario sc (pauses before every 10th key in
// every scenario), and returns the keystroke-to-frame times and their CPU
// part (Runner.Frame without the GPU wait).
func typeRound(t *testing.T, d *State, sc typing, n int) (total, cpu durations) {
	switch sc {
	case typeIdle:
		d.Options.Debounce = time.Hour
	case typePaused:
		d.Options.Debounce = 200 * time.Millisecond
	case typeBusy:
		d.Options.Debounce = 0
	}
	// Tokenize both sides first, so that the frames do not spend their
	// highlighting budget catching up (ui/editor's "mid-hl" condition).
	for _, st := range d.sides() {
		if hl := st.Highlighter(); hl != nil {
			hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Minute)
		}
	}
	h := newPerfHost(t, func(c *ui.Context) { View(c, d) })
	for range 3 {
		h.frame(t)
	}
	r := d.side(diff.Modified)
	er, _ := h.r.Context().RectOf(d.boxID)
	p := geom.Pt(er.X+er.W*3/4, er.Y+er.H/2)
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
	h.frame(t)
	h.frame(t)
	if d.focus != diff.Modified {
		t.Fatal("modified side not focused")
	}
	snap := r.Snapshot()
	lines := snap.LineCount()
	r.MoveCaret(snap.LineStart(50_000), false)
	r.RevealCaret()
	h.frame(t)
	h.frame(t)
	h.cpu = nil
	pauses := []time.Duration{210, 230, 250, 270, 300}
	for i := range n {
		if i%10 == 0 {
			deadline := time.Now().Add(pauses[(i/10)%len(pauses)] * time.Millisecond)
			for time.Now().Before(deadline) {
				if h.w.invalid.Load() {
					h.frame(t) // a result arrived (not timed)
				}
				time.Sleep(time.Millisecond)
			}
		}
		if i%40 == 39 {
			h.r.HandleEvent(input.KeyEvent{Key: input.KeyEnter, Down: true})
			h.r.HandleEvent(input.KeyEvent{Key: input.KeyEnter})
		} else {
			h.r.HandleEvent(input.KeyEvent{Key: input.KeyX, Down: true})
			h.r.HandleEvent(input.TextInput{Text: "x"})
			h.r.HandleEvent(input.KeyEvent{Key: input.KeyX})
		}
		total = append(total, h.timed(t))
	}
	if got := r.Snapshot().LineCount(); got != lines+n/40 {
		t.Fatalf("%d lines after typing, want %d", got, lines+n/40)
	}
	return total, h.cpu
}

func TestDiffTypingABAB(t *testing.T) {
	if os.Getenv("DIFFVIEW_PERF") == "" {
		t.Skip("set DIFFVIEW_PERF=1 to run the typing A/B measurement")
	}
	useTestText(t)
	metalDevice(t)
	rounds := 8
	if s := os.Getenv("DIFFVIEW_PERF_ROUNDS"); s != "" {
		fmt.Sscan(s, &rounds)
	}
	ratio := func(a, b durations) float64 { return float64(a.pct(0.95)) / float64(b.pct(0.95)) }
	var gate, gateCPU, worst []float64
	for round := range rounds {
		order := []typing{typePaused, typeIdle, typeBusy}
		if round%2 == 1 {
			order = []typing{typeBusy, typeIdle, typePaused}
		}
		var tot, cpu [3]durations
		load0 := loadAvg()
		for _, sc := range order {
			d := newPerfDiff(t)
			tot[sc], cpu[sc] = typeRound(t, d, sc, 200)
			d.Close()
			d.run.wait()
			runtime.GC()
		}
		gate = append(gate, ratio(tot[typePaused], tot[typeIdle]))
		gateCPU = append(gateCPU, ratio(cpu[typePaused], cpu[typeIdle]))
		worst = append(worst, ratio(tot[typeBusy], tot[typeIdle]))
		t.Logf("round %d: p95 idle %.2f (cpu %.2f), paused %.2f (cpu %.2f), busy %.2f (cpu %.2f) ms; load %s → %s",
			round, ms(tot[typeIdle].pct(0.95)), ms(cpu[typeIdle].pct(0.95)), ms(tot[typePaused].pct(0.95)), ms(cpu[typePaused].pct(0.95)),
			ms(tot[typeBusy].pct(0.95)), ms(cpu[typeBusy].pct(0.95)), load0, loadAvg())
	}
	med := func(v []float64) float64 {
		v = slices.Clone(v)
		slices.Sort(v)
		return (v[(len(v)-1)/2] + v[len(v)/2]) / 2
	}
	t.Logf("median p95 ratios over %d rounds (GOMAXPROCS %d): paused/idle %.3f (cpu %.3f), busy/idle %.3f (worst case, not gated)",
		rounds, runtime.GOMAXPROCS(0), med(gate), med(gateCPU), med(worst))
	if m := med(gate); m > 1.10 {
		t.Errorf("typing p95 with the background diff is %.3f× the p95 without it (gate 1.10)", m)
	}
}
