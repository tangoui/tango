//go:build darwin

package editor

// Spec 6.6 performance measurements (docs/specs/code-editor.md). Each
// benchmark matches one row of the 6.6 table and runs the real pipeline: a
// ui.Runner (input, build, layout and paint of the View) rendered by
// gpu.Renderer on the Metal device offscreen, in the reference window
// (1920×1080 logical; scale2 = Retina pixels). A frame's time is
// Runner.Frame (which ends with Renderer.Render submitting the command
// buffer) plus the wait for the GPU to finish it (see syncDevice).
//
// They are heavy (a 100 MB document) and only run when asked; record the
// load (uptime) next to the numbers:
//
//	go test ./ui/editor -run '^$' -bench 'Open100MB' -benchtime 3x
//	go test ./ui/editor -run '^$' -bench 'Memory100MB' -benchtime 1x  # one fresh process per sample
//	go test ./ui/editor -run '^$' -bench 'TypeAt1MLines' -benchtime 300x   # includes TypeAt1MLinesPartial
//	go test ./ui/editor -run '^$' -bench 'Scroll' -benchtime 200x        # includes ScrollCaretFar
//	go test ./ui/editor -run '^$' -bench 'HighlightTS20k' -benchtime 15x
//
// The generated documents are written by a child test process (see
// perfFile) and kept in $TMPDIR/tango-editor-perf, so generating them
// never inflates this process's memory and later runs reuse them; the open
// benchmarks therefore read from a warm page cache.
//
// Results and their analysis: docs/specs/code-editor-m1-report.md.

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/metal"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// perfSize is the reference window of spec 6.6.
var perfSize = geom.Size{W: 1920, H: 1080}

// perfTemplates are the TypeScript lines of the generated documents
// (genTS); %[1]d is the line number. They average about 100 bytes, so 1M
// lines are about 100 MB, and cover the main token kinds (keywords,
// identifiers, strings, template strings, numbers, comments).
// examples/editor-demo generates the same text.
var perfTemplates = []string{
	"export interface Row%[1]d { id: number; label: string; tags: string[]; nested?: { ok: boolean } }",
	"export function compute%[1]d(input: number[], scale = %[1]d): number { return input.length * scale; }",
	"  // line %[1]d: a comment explaining the next statement, with a few more words to fill it up nicely.",
	"  const message%[1]d = `row ${index + %[1]d} of the generated document: ${name.toUpperCase()}`;",
	"  if (value%[1]d !== null && typeof value%[1]d === \"object\") { counter += Object.keys(value).length; }",
	"class Widget%[1]d extends Base<string> implements Disposable { private readonly name = \"w-%[1]d\"; }",
	"  for (let i = 0; i < %[1]d; i++) { buffer.push({ index: i, hex: 0x%[1]x, ratio: i / 3.14159 }); }",
	"  return await fetch(`/api/items/%[1]d`, { method: \"POST\", body: JSON.stringify({ id: %[1]d }) });",
	"}  // end of block %[1]d: closes the class, function or loop that was opened above",
	"/* block %[1]d */ export const enumValue%[1]d = Direction.Up | Direction.Left; // trailing comment goes here",
}

// genTS returns n lines of generated TypeScript, each ending in '\n'.
func genTS(n int) string {
	var sb strings.Builder
	sb.Grow(n * 101)
	for i := range n {
		fmt.Fprintf(&sb, perfTemplates[i%len(perfTemplates)], i)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// perfGenEnv names the file a child process of perfFile writes.
const perfGenEnv = "TANGO_EDITOR_PERF_GEN"

// perfFile returns the path of a .ts file holding genTS(n), read from disk
// by the benchmarks like an application would. The file is kept in
// $TMPDIR/tango-editor-perf (its name carries a hash of perfTemplates, so
// changed templates make a new file). A missing file is written by a child
// process (TestPerfGenFile), so the ~200 MB the generation allocates never
// shows in this process's heap or resident size.
func perfFile(tb testing.TB, n int) string {
	tb.Helper()
	sum := fnv.New32a()
	for _, t := range perfTemplates {
		sum.Write([]byte(t))
	}
	dir := filepath.Join(os.TempDir(), "tango-editor-perf")
	p := filepath.Join(dir, fmt.Sprintf("gen%d-%08x.ts", n, sum.Sum32()))
	if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
		return p
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPerfGenFile$")
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d:%s", perfGenEnv, n, p))
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("generating %s: %v\n%s", p, err, out)
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
		tb.Fatalf("generator did not write %s: %v", p, err)
	}
	return p
}

// TestPerfGenFile is perfFile's child process: with TANGO_EDITOR_PERF_GEN
// set to "n:path" it writes genTS(n) to path (atomically); otherwise it
// does nothing.
func TestPerfGenFile(t *testing.T) {
	spec := os.Getenv(perfGenEnv)
	if spec == "" {
		t.Skip("only run by perfFile")
	}
	ns, path, ok := strings.Cut(spec, ":")
	n, err := strconv.Atoi(ns)
	if !ok || err != nil {
		t.Fatalf("bad %s %q", perfGenEnv, spec)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(genTS(n)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// perfWindow is a ui.Window without a screen (the GPU renders offscreen).
type perfWindow struct {
	size    geom.Size
	scale   float32
	invalid atomic.Bool
	clip    string
}

func (w *perfWindow) Size() geom.Size              { return w.size }
func (w *perfWindow) Scale() float32               { return w.scale }
func (w *perfWindow) Invalidate()                  { w.invalid.Store(true) }
func (w *perfWindow) InvalidateAt(time.Time)       {}
func (w *perfWindow) SetCursor(input.Cursor)       {}
func (w *perfWindow) ReadClipboard() string        { return w.clip }
func (w *perfWindow) WriteClipboard(s string)      { w.clip = s }
func (w *perfWindow) SetTextInput(bool, geom.Rect) {}
func (w *perfWindow) Present(*image.RGBA) error    { return nil }

// syncDevice is the Metal device with a way to wait for the last frame:
// metal.Device.Execute commits without waiting and waits for the previous
// command buffer at its start, so wait executes an empty frame of the same
// sizes (a clear of the target, no uploads, no texture changes). The time
// it returns includes encoding that clear, a few microseconds. syncDevice
// is not a gpu.Retainer, so the renderer draws every frame in full and
// clearing the target does not change what the next frame draws (the
// View's Damage/ScrollRegion hints are ignored: the M1 conditions).
//
// With keep (retainDevice, the partial-redraw benchmarks) the wait frame
// is a partial frame clearing only the bottom-left pixel to Frame.Clear,
// the color it already has: the page leaves a row of the window
// background (the List's Background, which is Frame.Clear) below the
// view, so the target still holds the last frame for the next partial
// one.
type syncDevice struct {
	d    *metal.Device
	last gpu.Frame
	keep bool

	frames, partial int // frames executed (not counting waits), partial ones
}

func (s *syncDevice) Execute(f *gpu.Frame) error {
	s.last = gpu.Frame{Size: f.Size, Clear: f.Clear, MaskSize: f.MaskSize, ColorSize: f.ColorSize, RampSize: f.RampSize}
	if s.keep {
		s.last.Dirty = []image.Rectangle{image.Rect(0, f.Size.Y-1, 1, f.Size.Y)}
	}
	s.frames++
	if len(f.Dirty) > 0 {
		s.partial++
	}
	return s.d.Execute(f)
}

// retainDevice is a syncDevice (with keep) that reports the Metal
// device's target retained, so the renderer draws partial frames.
type retainDevice struct{ *syncDevice }

func (r retainDevice) Retained(size image.Point) bool { return r.d.Retained(size) }

func (s *syncDevice) wait() error {
	if s.last.Size.X == 0 {
		return nil
	}
	return s.d.Execute(&s.last)
}

var perfDevice struct {
	once sync.Once
	d    *metal.Device
	err  error
}

// metalDevice returns the process's Metal device, skipping tb without one.
func metalDevice(tb testing.TB) *metal.Device {
	tb.Helper()
	perfDevice.once.Do(func() { perfDevice.d, perfDevice.err = metal.NewDevice() })
	if err := perfDevice.err; err != nil {
		if errors.Is(err, metal.ErrUnsupported) || errors.Is(err, metal.ErrNoDevice) {
			tb.Skipf("no Metal device: %v", err)
		}
		tb.Fatal(err)
	}
	return perfDevice.d
}

// perfHost runs a page showing one State in a perfWindow, rendered on the
// Metal device.
type perfHost struct {
	st  *State
	w   *perfWindow
	r   *ui.Runner
	dev *syncDevice
	vw  *ViewW

	// Sums over frames: Runner.Frame (CPU: input, build, layout, paint,
	// encode and submit) and the wait for the GPU after it.
	cpuT, waitT time.Duration
	frames      int
}

// newPerfHost shows st at scale (1 or 2) through a new Renderer on dev
// (the device and so its pipelines are shared; the glyph atlas is the
// Renderer's).
func newPerfHost(tb testing.TB, st *State, scale float32) *perfHost {
	tb.Helper()
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: scale}}
	h.dev = &syncDevice{d: metalDevice(tb)}
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st)
	}, ui.RunnerOptions{Renderer: gpu.NewRenderer(h.dev), NoAnimations: true})
	h.r.Attach(h.w)
	return h
}

// newPartialPerfHost is newPerfHost with partial redraws: the device is a
// retainDevice and verify mode is off (as in an application), and the view
// leaves the bottom row of the window to the background (see syncDevice).
func newPartialPerfHost(tb testing.TB, st *State, scale float32) *perfHost {
	tb.Helper()
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: scale}}
	h.dev = &syncDevice{d: metalDevice(tb), keep: true}
	ren := gpu.NewRenderer(retainDevice{h.dev})
	ren.SetVerify(false)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st).Layout(layout.Grow(0), layout.H(perfSize.H-1))
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	h.r.Attach(h.w)
	return h
}

// frame runs one frame and waits for the GPU to finish it.
func (h *perfHost) frame(tb testing.TB) {
	h.w.invalid.Store(false)
	t0 := time.Now()
	if err := h.r.Frame(); err != nil {
		tb.Fatal(err)
	}
	t1 := time.Now()
	if err := h.dev.wait(); err != nil {
		tb.Fatal(err)
	}
	h.cpuT += t1.Sub(t0)
	h.waitT += time.Since(t1)
	h.frames++
}

// resetSplit clears the CPU / GPU-wait sums.
func (h *perfHost) resetSplit() { h.cpuT, h.waitT, h.frames = 0, 0, 0 }

// reportSplit reports the mean CPU and GPU-wait parts of the frames since
// resetSplit.
func (h *perfHost) reportSplit(b *testing.B) {
	if h.frames == 0 {
		return
	}
	n := time.Duration(h.frames)
	b.ReportMetric(ms(h.cpuT/n), "cpu-ms")
	b.ReportMetric(ms(h.waitT/n), "gpuwait-ms")
}

// timedFrame is frame, timed.
func (h *perfHost) timedFrame(tb testing.TB) time.Duration {
	start := time.Now()
	h.frame(tb)
	return time.Since(start)
}

// focus clicks into the text area.
func (h *perfHost) focus(tb testing.TB) {
	g := h.vw.vs
	p := geom.Pt(g.text.X+g.text.W/2, g.text.Y+g.text.H/2)
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
	h.frame(tb)
	h.frame(tb)
	if !h.vw.Focused() {
		tb.Fatal("view not focused")
	}
}

// caretTo puts the caret at the start of line and scrolls it into view.
func (h *perfHost) caretTo(tb testing.TB, line int) {
	h.st.MoveCaret(h.st.Snapshot().LineStart(line), false)
	h.st.RevealCaret()
	h.frame(tb)
	h.frame(tb)
}

// typeKey delivers one keystroke as the platform does: key down, the
// committed text (Enter for "\n"), key up.
func (h *perfHost) typeKey(s string) {
	if s == "\n" {
		h.r.HandleEvent(input.KeyEvent{Key: input.KeyEnter, Down: true})
		h.r.HandleEvent(input.KeyEvent{Key: input.KeyEnter})
		return
	}
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX, Down: true})
	h.r.HandleEvent(input.TextInput{Text: s})
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX})
}

// visibleFinal reports whether every visible line has final tokens.
func (h *perfHost) visibleFinal() bool {
	hl := h.st.Highlighter()
	if hl == nil {
		return true
	}
	snap := h.st.Snapshot()
	first, last := h.st.VisibleLines()
	for i := first; i <= last && i < snap.LineCount(); i++ {
		if _, final := hl.Tokens(snap, i, nil); !final {
			return false
		}
	}
	return true
}

// keyAt returns the i-th keystroke of the typing scenario of MinCode's
// e2e-perf ("编辑器连续输入 200 个字符"): 'x', with Enter every 40th key.
func keyAt(i int) string {
	if i%40 == 39 {
		return "\n"
	}
	return "x"
}

// durations collects frame times.
type durations []time.Duration

func (d durations) pct(p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	i := int(float64(len(s)-1) * p)
	return s[i]
}

func (d durations) mean() time.Duration {
	if len(d) == 0 {
		return 0
	}
	var t time.Duration
	for _, v := range d {
		t += v
	}
	return t / time.Duration(len(d))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func loadAvg() string {
	out, err := sysctlLoad()
	if err != nil {
		return "?"
	}
	return out
}

// sysctlLoad returns the 1-minute load average (vm.loadavg).
func sysctlLoad() (string, error) {
	b, err := syscall.Sysctl("vm.loadavg")
	if err != nil {
		return "", err
	}
	// struct loadavg { fixpt_t ldavg[3]; long fscale; } (sysctl returns the
	// raw bytes, possibly without the trailing NUL the wrapper strips).
	raw := []byte(b)
	for len(raw) < 24 {
		raw = append(raw, 0)
	}
	le := func(p []byte) uint32 { return uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24 }
	scale := float64(le(raw[16:]))
	if scale == 0 {
		return "", errors.New("no fscale")
	}
	return fmt.Sprintf("%.2f %.2f %.2f", float64(le(raw[0:]))/scale, float64(le(raw[4:]))/scale, float64(le(raw[8:]))/scale), nil
}

func logLoad(b *testing.B) {
	b.Logf("load average: %s, GOMAXPROCS %d", loadAvg(), runtime.GOMAXPROCS(0))
}

// openFile is what an application does to open path: read it, make the
// State (buffer, language, highlighter) and show it.
func openFile(tb testing.TB, path string, scale float32) *perfHost {
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	st := NewState(string(data), path, theme.Base("vs"))
	return newPerfHost(tb, st, scale)
}

// BenchmarkOpen100MB (spec 6.6 row 1): open a 100 MB / 1M-line TypeScript
// file and show it. open-ms runs from reading the file to the first frame
// finished by the GPU (the view is editable from then on); hl-ms to the
// first frame whose visible lines all have final tokens (the View
// tokenizes them inside the frame within its budget, so usually the same
// frame). The Metal device exists already (a running application); the
// Renderer and its glyph atlas are new.
func BenchmarkOpen100MB(b *testing.B) {
	path := perfFile(b, 1_000_000)
	fi, _ := os.Stat(path)
	metalDevice(b)
	logLoad(b)
	b.Logf("file: %.1f MB, 1000000 lines", float64(fi.Size())/1e6)
	var open, hl durations
	for b.Loop() {
		runtime.GC()
		start := time.Now()
		h := openFile(b, path, 2)
		h.frame(b)
		open = append(open, time.Since(start))
		for i := 0; !h.visibleFinal(); i++ {
			if i > 1000 {
				b.Fatal("visible lines never final")
			}
			h.frame(b)
		}
		hl = append(hl, time.Since(start))
	}
	b.ReportMetric(ms(open.mean()), "open-ms")
	b.ReportMetric(ms(open.pct(1)), "open-max-ms")
	b.ReportMetric(ms(hl.mean()), "hl-ms")
}

// BenchmarkOpen100MBWrap is BenchmarkOpen100MB with word wrap on: the
// first frame wraps only the lines around the viewport (viewlines.go), so
// open-ms must stay within 1 s and within 10% of BenchmarkOpen100MB.
func BenchmarkOpen100MBWrap(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	var open durations
	for b.Loop() {
		runtime.GC()
		start := time.Now()
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		st := NewState(string(data), path, theme.Base("vs"))
		st.Options.WordWrap = WrapOn
		h := newPerfHost(b, st, 2)
		h.frame(b)
		open = append(open, time.Since(start))
		if !st.wrapping() {
			b.Fatal("not wrapping")
		}
	}
	b.ReportMetric(ms(open.mean()), "open-ms")
	b.ReportMetric(ms(open.pct(1)), "open-max-ms")
}

// BenchmarkOpen100MBSteps splits the open into its steps (read, State,
// first frame) to show where the time goes.
func BenchmarkOpen100MBSteps(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	var read, state, first durations
	for b.Loop() {
		runtime.GC()
		t0 := time.Now()
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		s := string(data)
		t1 := time.Now()
		st := NewState(s, path, theme.Base("vs"))
		t2 := time.Now()
		h := newPerfHost(b, st, 2)
		h.frame(b)
		t3 := time.Now()
		read, state, first = append(read, t1.Sub(t0)), append(state, t2.Sub(t1)), append(first, t3.Sub(t2))
	}
	b.ReportMetric(ms(read.mean()), "read-ms")
	b.ReportMetric(ms(state.mean()), "state-ms")
	b.ReportMetric(ms(first.mean()), "frame-ms")
}

// memSample is the process's memory after a full GC with the freed
// pages returned to the OS (debug.FreeOSMemory).
type memSample struct {
	heap      uint64 // runtime.MemStats.HeapInuse
	rss       uint64 // resident size (ps)
	footprint uint64 // phys_footprint (footprint(1)): what Activity Monitor shows as Memory
	detail    string // footprint's largest categories
}

func sampleMem(tb testing.TB) memSample {
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := memSample{heap: m.HeapInuse}
	pid := strconv.Itoa(os.Getpid())
	if out, err := exec.Command("ps", "-o", "rss=", "-p", pid).Output(); err == nil {
		if kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
			s.rss = kb * 1024
		}
	}
	out, err := exec.Command("footprint", pid).Output()
	if err != nil {
		tb.Logf("footprint: %v", err)
		return s
	}
	s.footprint, s.detail = parseFootprint(string(out))
	return s
}

// parseFootprint reads "Footprint: 123 MB" and the first category lines
// of footprint(1)'s report.
func parseFootprint(out string) (uint64, string) {
	var total uint64
	var cats []string
	lines := strings.Split(out, "\n")
	for k, l := range lines {
		if i := strings.Index(l, "Footprint: "); i >= 0 && total == 0 {
			f := strings.Fields(l[i+len("Footprint: "):])
			if len(f) >= 2 {
				total = parseSize(f[0], f[1])
			}
		}
		if strings.Contains(l, "Category") {
			for _, c := range lines[k+2:] {
				if strings.TrimSpace(c) == "" || strings.HasPrefix(strings.TrimSpace(c), "---") || len(cats) == 8 {
					break
				}
				cats = append(cats, strings.Join(strings.Fields(c), " "))
			}
			break
		}
	}
	return total, strings.Join(cats, "; ")
}

func parseSize(v, unit string) uint64 {
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case "B":
	case "KB":
		x *= 1 << 10
	case "MB":
		x *= 1 << 20
	case "GB":
		x *= 1 << 30
	}
	return uint64(x)
}

func mb(v uint64) float64 { return float64(v) / 1e6 }

// delta is a - b, or 0.
func delta(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// BenchmarkMemory100MB (spec 6.6 row 5: ≤ 3× the file size). Three
// samples, each after GC and debug.FreeOSMemory: base (Metal device and
// test process up, nothing open), open (the 100 MB file open and its
// first frame rendered at 2x) and full (after the background goroutine
// highlighted the whole document; bg-hl-s is how long that took). For
// each, the Go heap (HeapInuse), the resident size and phys_footprint
// (which on Apple Silicon also counts the GPU render target, glyph atlas
// and driver memory) are reported as deltas over base; the *-base-MB
// metrics are the base values. maxrss-MB is the peak resident size,
// including the transient ReadFile buffer and its string copy. The
// benchmark fails when the heap delta exceeds 3× the file size; which
// process-level metric the spec means is for the user to decide (see the
// report).
func BenchmarkMemory100MB(b *testing.B) {
	path := perfFile(b, 1_000_000)
	fi, _ := os.Stat(path)
	size := uint64(fi.Size())
	metalDevice(b)
	logLoad(b)
	b.Logf("file: %.1f MB", mb(size))
	measured := false
	for b.Loop() {
		// Only the first iteration is measured: later ones would start
		// with the first one's GPU textures and freed pages in their base.
		if measured {
			continue
		}
		measured = true
		base := sampleMem(b)
		h := openFile(b, path, 2)
		h.frame(b)
		open := sampleMem(b)
		bg := highlightAllBackground(b, h.st)
		h.frame(b)
		full := sampleMem(b)
		var ru syscall.Rusage
		syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
		b.ReportMetric(mb(base.heap), "heap-base-MB")
		b.ReportMetric(mb(base.rss), "rss-base-MB")
		b.ReportMetric(mb(base.footprint), "fp-base-MB")
		b.ReportMetric(mb(delta(open.heap, base.heap)), "heap-open-MB")
		b.ReportMetric(mb(delta(full.heap, base.heap)), "heap-full-MB")
		b.ReportMetric(mb(delta(open.rss, base.rss)), "rss-open-MB")
		b.ReportMetric(mb(delta(full.rss, base.rss)), "rss-full-MB")
		b.ReportMetric(mb(delta(open.footprint, base.footprint)), "fp-open-MB")
		b.ReportMetric(mb(delta(full.footprint, base.footprint)), "fp-full-MB")
		b.ReportMetric(bg.Seconds(), "bg-hl-s")
		b.ReportMetric(float64(ru.Maxrss)/1e6, "maxrss-MB") // bytes on darwin
		b.Logf("footprint base: %s", base.detail)
		b.Logf("footprint full: %s", full.detail)
		if limit := 3 * size; delta(full.heap, base.heap) > limit || delta(open.heap, base.heap) > limit {
			b.Errorf("heap over 3x the %.0f MB file", mb(size))
		}
		runtime.KeepAlive(h)
	}
}

// highlightAllBackground runs st's background highlighter until the whole
// document is highlighted and returns how long it took.
func highlightAllBackground(tb testing.TB, st *State) time.Duration {
	hl := st.Highlighter()
	snap := st.Snapshot()
	done := make(chan struct{})
	var once sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan buffer.Snapshot, 1)
	ch <- snap
	start := time.Now()
	hl.Start(ctx, ch, func() {
		if hl.Done(snap) {
			once.Do(func() { close(done) })
		}
	})
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		tb.Fatal("background highlight not done after 10 minutes")
	}
	return time.Since(start)
}

// typeBench types keyAt(i) for every iteration at the caret of h and
// reports the frame time from the key to the GPU finishing the frame.
func typeBench(b *testing.B, h *perfHost) durations {
	n := h.st.Snapshot().LineCount()
	var d durations
	i := 0
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.typeKey(keyAt(i))
		h.frame(b)
		d = append(d, time.Since(start))
		i++
	}
	if got := h.st.Snapshot().LineCount(); got != n+i/40 {
		b.Fatalf("line count %d after %d keys, want %d", got, i, n+i/40)
	}
	b.ReportMetric(ms(d.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(d.pct(0.95)), "p95-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	h.reportSplit(b)
	return d
}

// BenchmarkTypeAt1MLines (spec 6.6 row 2): keystroke to frame done in the
// 100 MB / 1M-line file, p95 ≤ 16 ms. "mid" types at line 500 000 (every
// edit shifts the line index and the highlighter's line states behind
// it), "end" at the last line; "mid-bg" is "mid" while the background
// goroutine highlights the document (just after opening it); "mid-hl" is
// "mid" after the whole document was highlighted (no in-frame
// tokenization budget spent catching up on the lines above the view);
// "mid-wrap" is "mid" with word wrap on (Task 10).
func BenchmarkTypeAt1MLines(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	for _, c := range []struct {
		name string
		line int
		bg   bool
		full bool
		wrap bool
	}{{"mid", 500_000, false, false, false}, {"end", 999_999, false, false, false}, {"mid-bg", 500_000, true, false, false}, {"mid-hl", 500_000, false, true, false}, {"mid-wrap", 500_000, false, false, true}} {
		b.Run(c.name, func(b *testing.B) {
			h := openFile(b, path, 2)
			if c.wrap {
				h.st.Options.WordWrap = WrapOn
			}
			h.frame(b)
			if c.full {
				highlightAllBackground(b, h.st)
			}
			h.focus(b)
			h.caretTo(b, c.line)
			if c.bg {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ch := make(chan buffer.Snapshot, 1)
				ch <- h.st.Snapshot()
				h.st.Highlighter().Start(ctx, ch, h.w.Invalidate)
				time.Sleep(50 * time.Millisecond) // let it get going
			}
			typeBench(b, h)
		})
	}
}

// BenchmarkTypeAt1MLinesPartial is BenchmarkTypeAt1MLines ("mid" and
// "end") with partial redraws (newPartialPerfHost: a Retainer device,
// verify mode off): each keystroke redraws only the rows the View declares
// changed (damage.go). partial is the fraction of frames drawn partially.
// Compare p50/p95 with BenchmarkTypeAt1MLines/mid and /end (full redraws,
// the milestone 1 numbers).
func BenchmarkTypeAt1MLinesPartial(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	text := string(data)
	for _, c := range []struct {
		name string
		line int
	}{{"mid", 500_000}, {"end", 999_999}} {
		b.Run(c.name, func(b *testing.B) {
			h := newPartialPerfHost(b, NewState(text, path, theme.Base("vs")), 2)
			h.frame(b)
			h.focus(b)
			h.caretTo(b, c.line)
			f0, p0 := h.dev.frames, h.dev.partial
			typeBench(b, h)
			if n := h.dev.frames - f0; n > 0 {
				b.ReportMetric(float64(h.dev.partial-p0)/float64(n), "partial")
			}
		})
	}
}

// BenchmarkScroll (spec 6.6 row 3): continuous scrolling, average ≥ 60 fps
// and p99 frame time ≤ 33 ms. Every iteration is one frame with a wheel
// event of two lines (a fast trackpad swipe), moving down from line
// 100 000 of the 1M-line file; fps is 1000 / mean frame time (frames are
// not paced by a display, so it is the throughput the pipeline could
// sustain). The device is not a Retainer (syncDevice), so every scroll
// frame is a full redraw, as in milestone 1, although the View now emits
// ScrollRegion hints (damage.go).
func BenchmarkScroll(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	text := string(data)
	for _, c := range []struct {
		scale float32
		wrap  bool
	}{{2, false}, {1, false}, {2, true}} {
		name := fmt.Sprintf("scale%v", c.scale)
		if c.wrap {
			name += "-wrap" // word wrap on (Task 10), against scale2
		}
		scale := c.scale
		b.Run(name, func(b *testing.B) {
			st := NewState(text, path, theme.Base("vs"))
			if c.wrap {
				st.Options.WordWrap = WrapOn
			}
			h := newPerfHost(b, st, scale)
			h.frame(b)
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
		})
	}
}

// BenchmarkScrollCaretFar is BenchmarkScroll (scale 2) with the view
// focused and the caret at line 0, far above the scrolled lines: every
// frame still reports the IME caret rect, which must not lay out the caret
// line (that moved the highlighter's token window to it and back every
// frame, re-tokenizing the visible lines). window-away is the fraction of
// frames that ended with the token window away from the visible lines
// (1 when the caret line pulls it back every frame, 0 when fixed).
func BenchmarkScrollCaretFar(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	h := openFile(b, path, 2)
	h.frame(b)
	h.focus(b)
	h.st.MoveCaret(0, false)
	h.frame(b)
	h.st.Viewport.TopLine = 100_000
	h.frame(b)
	h.frame(b)
	hl := h.st.Highlighter()
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * h.st.Viewport.LineHeight
	var d durations
	away := 0
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.frame(b)
		d = append(d, time.Since(start))
		first, last := h.st.VisibleLines()
		if lo, hi := hl.WindowRangeFor(h.st.hlKey); lo > first || hi <= last {
			away++
		}
	}
	if h.st.Viewport.TopLine <= 100_000 || h.st.Primary().Head != 0 || !h.vw.Focused() {
		b.Fatalf("setup: TopLine %d, head %d, focused %v", h.st.Viewport.TopLine, h.st.Primary().Head, h.vw.Focused())
	}
	b.ReportMetric(1000/ms(d.mean()), "fps")
	b.ReportMetric(ms(d.mean()), "mean-ms")
	b.ReportMetric(ms(d.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	b.ReportMetric(float64(away)/float64(len(d)), "window-away")
	h.reportSplit(b)
}

// BenchmarkScrollJump scrolls by a whole page per frame (dragging the
// scrollbar, page down held): every frame shows lines never shaped before.
func BenchmarkScrollJump(b *testing.B) {
	path := perfFile(b, 1_000_000)
	metalDevice(b)
	logLoad(b)
	h := openFile(b, path, 2)
	h.frame(b)
	h.caretTo(b, 0)
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	var d durations
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.r.HandleEvent(input.Scroll{Pos: p, DY: h.st.Viewport.Height})
		h.frame(b)
		d = append(d, time.Since(start))
	}
	b.ReportMetric(1000/ms(d.mean()), "fps")
	b.ReportMetric(ms(d.mean()), "mean-ms")
	b.ReportMetric(ms(d.pct(0.99)), "p99-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	h.reportSplit(b)
}

// BenchmarkHighlightTS20kBackground (spec 6.6 row 4): highlighting a
// 20 000-line TypeScript file in the background takes ≤ 2 s and typing
// meanwhile is not affected. Every iteration opens the file, starts the
// background goroutine and types at line 10 (as MinCode's e2e-perf
// scenario does) one key per frame until the document is fully
// highlighted; bg-ms is the time to full highlight (with the typing going
// on), p50/p95 the key-to-frame times during it, base-p95 the same typing
// with highlighting finished (the "not affected" reference).
func BenchmarkHighlightTS20kBackground(b *testing.B) {
	path := perfFile(b, 20_000)
	metalDevice(b)
	logLoad(b)
	var bgT, during, after durations
	for b.Loop() {
		h := openFile(b, path, 2)
		h.frame(b)
		h.focus(b)
		h.caretTo(b, 10)
		hl := h.st.Highlighter()
		var ready atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan buffer.Snapshot, 1)
		ch <- h.st.Snapshot()
		start := time.Now()
		hl.Start(ctx, ch, func() { ready.Add(1); h.w.Invalidate() })
		i := 0
		var bg time.Duration
		for {
			t := time.Now()
			h.typeKey(keyAt(i))
			h.frame(b)
			during = append(during, time.Since(t))
			i++
			if hl.Done(h.st.Snapshot()) {
				bg = time.Since(start)
				break
			}
			if time.Since(start) > time.Minute {
				b.Fatal("not highlighted after a minute")
			}
			// One key per 16 ms frame at most, as a fast typist on a
			// 60 Hz display; frames are not paced otherwise.
			if el := time.Since(t); el < 16*time.Millisecond {
				time.Sleep(16*time.Millisecond - el)
			}
		}
		bgT = append(bgT, bg)
		for j := range 100 {
			t := time.Now()
			h.typeKey(keyAt(i + j))
			h.frame(b)
			after = append(after, time.Since(t))
		}
		cancel()
		b.Logf("background ready callbacks %d, keys typed during highlight %d", ready.Load(), i)
	}
	b.ReportMetric(ms(bgT.mean()), "bg-ms")
	b.ReportMetric(ms(during.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(during.pct(0.95)), "p95-ms")
	b.ReportMetric(ms(during.pct(1)), "max-ms")
	b.ReportMetric(ms(after.pct(0.95)), "base-p95-ms")
	b.ReportMetric(float64(len(during)), "samples")
	b.ReportMetric(float64(len(after)), "base-samples")
	if len(during) < 300 {
		b.Logf("only %d keystrokes during highlighting: run with -benchtime 15x or more for a meaningful p95", len(during))
	}
}

// BenchmarkHighlightTS20kAlone is the full background highlight of the
// 20 000-line file with nothing else going on (the bare 6.6 row 4 time).
func BenchmarkHighlightTS20kAlone(b *testing.B) {
	data, err := os.ReadFile(perfFile(b, 20_000))
	if err != nil {
		b.Fatal(err)
	}
	logLoad(b)
	text := string(data)
	var d durations
	for b.Loop() {
		d = append(d, highlightAllBackground(b, NewState(text, "a.ts", theme.Base("vs"))))
	}
	b.ReportMetric(ms(d.mean()), "bg-ms")
}

// TestPerfHostRenders checks the benchmarks' harness: the offscreen Metal
// frame of a highlighted document has the theme's background and many
// other (text) pixels, and the line count of genTS is right. With
// TANGO_EDITOR_PERF_PNG=path it also writes the frame (for the report).
func TestPerfHostRenders(t *testing.T) {
	if got := strings.Count(genTS(123), "\n"); got != 123 {
		t.Fatalf("genTS(123) has %d lines", got)
	}
	st := NewState(genTS(2000), "a.ts", theme.Base("vs-dark"))
	h := newPerfHost(t, st, 1)
	h.frame(t)
	h.caretTo(t, 1000)
	if !h.visibleFinal() {
		h.frame(t)
	}
	// One more frame without syncDevice.wait (its clear would be read).
	if err := h.r.Frame(); err != nil {
		t.Fatal(err)
	}
	img, err := h.dev.d.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dx() != int(perfSize.W) || img.Rect.Dy() != int(perfSize.H) {
		t.Fatalf("frame %v", img.Rect)
	}
	colors := map[[3]uint8]int{}
	for i := 0; i < len(img.Pix); i += 4 {
		colors[[3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}]++
	}
	bg := defaultColor(st.Theme, "editor.background")
	nbg := colors[[3]uint8{bg.R, bg.G, bg.B}]
	total := len(img.Pix) / 4
	if nbg < total/2 || nbg > total*98/100 || len(colors) < 50 {
		t.Errorf("background %d of %d pixels, %d colors: not an editor frame", nbg, total, len(colors))
	}
	if p := os.Getenv("TANGO_EDITOR_PERF_PNG"); p != "" {
		if err := savePNG(p, img); err != nil {
			t.Fatal(err)
		}
	}
}
