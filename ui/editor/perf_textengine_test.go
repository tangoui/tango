//go:build darwin

package editor

// User-visible editor timings for choosing the macOS text engine: run the
// same tests in the default build (CoreText on macOS) and with
// -tags tango_gotext (go-text) and compare (docs/dev-builds.md, text engine). They reuse the
// offscreen Metal host of perf_test.go with partial redraws (a retained
// target, as in an application window after its first keystroke), at
// 1920×944 logical points, scale 2. No window is opened.
//
//	TANGO_EDITOR_TEXTENGINE=1 go test ./ui/editor -run 'TestPerfTextEngine' -count 1 -v
//	TANGO_EDITOR_TEXTENGINE=1 go test -tags tango_gotext ./ui/editor -run 'TestPerfTextEngine' -count 1 -v
//
//   - TestPerfTextEngineTyping: one keystroke (key down, committed text,
//     key up) plus the frame it causes, 600 timed keys after 20 warm-up
//     keys, in a 2 000-line Go file (the package's own sources) and a
//     2 000-line Markdown file mixing Chinese and English (docs/*.md); the
//     Markdown keys commit a CJK character every third key.
//   - TestPerfTextEngineScroll: one wheel event plus its frame, by one page
//     (the text area's height) down to the end and back up, and by three
//     lines down to the end, in the 20 000-line TypeScript file (perfFile)
//     and a 20 000-line mixed Markdown file.
//   - TestPerfTextEngineOpen: reading a 20 000-line file to its first frame
//     finished by the GPU, each sample in a fresh test process (font and
//     shaping caches cold, the Metal device already created).
//
// Every frame's time is from the event to the GPU finishing the frame
// ("total"); "cpu" is the event handling plus Runner.Frame alone. Each
// test logs one line per case starting with "TEXTENGINE" (build=default or
// build=tango_gotext; engine= is text.Engine, "coretext" in both). Times
// depend on the machine load: only compare runs interleaved in one session.

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

// textEngineEnv turns the tests on.
const textEngineEnv = "TANGO_EDITOR_TEXTENGINE"

// textEngineOpenEnv names the file the child process of
// TestPerfTextEngineOpen opens.
const textEngineOpenEnv = "TANGO_EDITOR_TEXTENGINE_OPEN"

// textEngineSize is the viewport: a 1920×1080 screen's maximized window
// less the menu bar and title bar.
var textEngineSize = geom.Size{W: 1920, H: 944}

func textEngineSkip(t *testing.T) {
	t.Helper()
	if os.Getenv(textEngineEnv) == "" {
		t.Skipf("measurement only: set %s=1", textEngineEnv)
	}
}

// textEngineFixture writes, once per content, n lines made by cycling
// through the files matching glob (sorted; *_test.go left out) to a file
// with extension ext in $TMPDIR/tango-editor-perf and returns its path.
func textEngineFixture(tb testing.TB, glob, ext string, n int) string {
	tb.Helper()
	files, err := filepath.Glob(glob)
	if err != nil || len(files) == 0 {
		tb.Fatalf("no files match %s: %v", glob, err)
	}
	slices.Sort(files)
	var lines []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			tb.Fatal(err)
		}
		lines = append(lines, strings.Split(strings.TrimRight(string(b), "\n"), "\n")...)
	}
	var sb strings.Builder
	for i := range n {
		sb.WriteString(lines[i%len(lines)])
		sb.WriteByte('\n')
	}
	s := sb.String()
	sum := fnv.New32a()
	sum.Write([]byte(s))
	dir := filepath.Join(os.TempDir(), "tango-editor-perf")
	p := filepath.Join(dir, fmt.Sprintf("te-%d-%08x%s", n, sum.Sum32(), ext))
	if fi, err := os.Stat(p); err == nil && fi.Size() == int64(len(s)) {
		return p
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(s), 0o600); err != nil {
		tb.Fatal(err)
	}
	if err := os.Rename(tmp, p); err != nil {
		tb.Fatal(err)
	}
	return p
}

// textEngineFile returns the path of a fixture: "go" (Go sources), "md"
// (mixed Chinese/English Markdown) or "ts" (perfFile's TypeScript).
func textEngineFile(tb testing.TB, kind string, n int) string {
	switch kind {
	case "go":
		return textEngineFixture(tb, "*.go", ".go", n)
	case "md":
		return textEngineFixture(tb, filepath.Join("..", "..", "docs", "*.md"), ".md", n)
	case "ts":
		return perfFile(tb, n)
	}
	tb.Fatalf("unknown fixture %q", kind)
	return ""
}

// newTextEngineHost shows st at textEngineSize, scale 2, with partial
// redraws (newPartialPerfHost without its 1px row).
func newTextEngineHost(tb testing.TB, st *State) *perfHost {
	tb.Helper()
	perfApplyOffEnv(tb, st)
	h := &perfHost{st: st, w: &perfWindow{size: textEngineSize, scale: 2}}
	h.dev = &syncDevice{d: metalDevice(tb)}
	ren := gpu.NewRenderer(retainDevice{h.dev})
	ren.SetVerify(false)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st)
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	h.r.Attach(h.w)
	return h
}

// openTextEngine opens path as an application does and shows it.
func openTextEngine(tb testing.TB, path string) *perfHost {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	st := NewView(NewModelWithOptions(path, string(data), DefaultRegistry(), perfModelOptions()), theme.Base("vs-dark"))
	tb.Cleanup(st.Close)
	h := newTextEngineHost(tb, st)
	tb.Cleanup(h.r.Close)
	return h
}

// timedEvent runs ev, then one frame, and returns the CPU part (ev plus
// Runner.Frame) and the total (to the GPU finishing the frame).
func (h *perfHost) timedEvent(tb testing.TB, ev func()) (cpu, total time.Duration) {
	h.w.invalid.Store(false)
	start := time.Now()
	ev()
	if err := h.r.Frame(); err != nil {
		tb.Fatal(err)
	}
	t1 := time.Now()
	if err := h.dev.wait(); err != nil {
		tb.Fatal(err)
	}
	end := time.Now()
	return t1.Sub(start), end.Sub(start)
}

// textEngineBuild names the build: "default" (CoreText shapes, breaks and
// rasterizes) or "tango_gotext" (go-text shapes and breaks; text.Engine
// still reports the CoreText rasterizer of system fonts).
func textEngineBuild() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "-tags" && slices.Contains(strings.Split(s.Value, ","), "tango_gotext") {
				return "tango_gotext"
			}
		}
	}
	return "default"
}

// textEngineReport logs one result line.
func textEngineReport(tb testing.TB, name string, cpu, total durations, extra string) {
	tb.Helper()
	tb.Logf("TEXTENGINE build=%s engine=%s case=%s n=%d total-p50=%.3f total-p99=%.3f total-max=%.3f cpu-p50=%.3f cpu-p99=%.3f cpu-mean=%.3f%s",
		textEngineBuild(), text.Engine(), name, len(total), ms(total.pct(.5)), ms(total.pct(.99)), ms(total.pct(1)),
		ms(cpu.pct(.5)), ms(cpu.pct(.99)), ms(cpu.mean()), extra)
}

func textEngineLogLoad(tb testing.TB) {
	tb.Helper()
	tb.Logf("build %s, engine %s, load average: %s", textEngineBuild(), text.Engine(), loadAvg())
}

// TestPerfTextEngineTyping: see the file comment.
func TestPerfTextEngineTyping(t *testing.T) {
	textEngineSkip(t)
	textEngineLogLoad(t)
	for _, kind := range []string{"go", "md"} {
		t.Run(kind, func(t *testing.T) {
			path := textEngineFile(t, kind, 2000)
			h := openTextEngine(t, path)
			h.frame(t)
			if h.st.Highlighter() == nil {
				t.Fatalf("%s: no highlighter", path)
			}
			highlightAllBackground(t, h.st)
			h.frame(t)
			h.focus(t)
			// Type at the start of the first line from 1 000 with at least
			// 20 characters (and, in Markdown, a CJK one), so the edited
			// line keeps its text after the caret.
			snap := h.st.Snapshot()
			line := -1
			for i := 1000; i < snap.LineCount(); i++ {
				s := snap.Line(i)
				if len([]rune(s)) >= 20 && (kind != "md" || strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0) {
					line = i
					break
				}
			}
			if line < 0 {
				t.Fatal("no line to type in")
			}
			h.caretTo(t, line)
			key := func(i int) string {
				if kind == "md" && i%3 == 2 && i%40 != 39 {
					return "字"
				}
				return keyAt(i)
			}
			n0 := snap.LineCount()
			const warm, timed = 20, 600
			var cpu, total durations
			for i := range warm + timed {
				c, d := h.timedEvent(t, func() { h.typeKey(key(i)) })
				if i >= warm {
					cpu, total = append(cpu, c), append(total, d)
				}
			}
			if got, want := h.st.Snapshot().LineCount(), n0+(warm+timed)/40; got != want {
				t.Fatalf("line count %d, want %d", got, want)
			}
			textEngineReport(t, "type/"+kind+"2k", cpu, total, fmt.Sprintf(" line=%d", line))
		})
	}
}

// TestPerfTextEngineScroll: see the file comment.
func TestPerfTextEngineScroll(t *testing.T) {
	textEngineSkip(t)
	textEngineLogLoad(t)
	for _, kind := range []string{"ts", "md"} {
		for _, step := range []string{"page", "3lines"} {
			t.Run(kind+"/"+step, func(t *testing.T) {
				h := openTextEngine(t, textEngineFile(t, kind, 20_000))
				h.frame(t)
				highlightAllBackground(t, h.st)
				h.frame(t)
				h.frame(t)
				p := geom.Pt(textEngineSize.W/2, textEngineSize.H/2)
				dy := 3 * h.st.Viewport.LineHeight
				if step == "page" {
					dy = h.vw.vs.text.H
				}
				var cpu, total durations
				scroll := func(dy float32) bool {
					before := h.st.Viewport
					c, d := h.timedEvent(t, func() { h.r.HandleEvent(input.Scroll{Pos: p, DY: dy}) })
					if h.st.Viewport.TopLine == before.TopLine && h.st.Viewport.OffsetY == before.OffsetY {
						return false
					}
					cpu, total = append(cpu, c), append(total, d)
					return true
				}
				const limit = 20_000
				for i := 0; scroll(dy); i++ {
					if i > limit {
						t.Fatal("never reached the end")
					}
				}
				end := h.st.Viewport.TopLine
				if end < 19_000 {
					t.Fatalf("stopped at line %d", end)
				}
				if step == "page" {
					for i := 0; scroll(-dy); i++ {
						if i > limit {
							t.Fatal("never reached the top")
						}
					}
				}
				textEngineReport(t, "scroll/"+kind+"20k/"+step, cpu, total, fmt.Sprintf(" dy=%.1f end=%d", dy, end))
			})
		}
	}
}

// TestPerfTextEngineOpen: see the file comment.
func TestPerfTextEngineOpen(t *testing.T) {
	textEngineSkip(t)
	textEngineLogLoad(t)
	samples := 7
	if s, err := strconv.Atoi(os.Getenv("TANGO_EDITOR_TEXTENGINE_OPEN_N")); err == nil && s > 0 {
		samples = s
	}
	for _, kind := range []string{"ts", "md", "go"} {
		t.Run(kind, func(t *testing.T) {
			path := textEngineFile(t, kind, 20_000)
			var open, first durations
			for range samples {
				cmd := exec.Command(os.Args[0], "-test.run=^TestPerfTextEngineOpenChild$", "-test.v")
				cmd.Env = append(os.Environ(), textEngineOpenEnv+"="+path)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("child: %v\n%s", err, out)
				}
				_, rest, ok := strings.Cut(string(out), "OPEN open=")
				o, f, ok2 := strings.Cut(rest, " frame=")
				if !ok || !ok2 {
					t.Fatalf("child printed no result:\n%s", out)
				}
				f, _, _ = strings.Cut(f, "\n")
				ov, err1 := time.ParseDuration(strings.TrimSpace(o))
				fv, err2 := time.ParseDuration(strings.TrimSpace(f))
				if err1 != nil || err2 != nil {
					t.Fatalf("bad child result %q %q", o, f)
				}
				open, first = append(open, ov), append(first, fv)
			}
			t.Logf("TEXTENGINE build=%s engine=%s case=open/%s20k n=%d open-p50=%.3f open-min=%.3f open-max=%.3f frame-p50=%.3f",
				textEngineBuild(), text.Engine(), kind, len(open), ms(open.pct(.5)), ms(open.pct(0)), ms(open.pct(1)), ms(first.pct(.5)))
		})
	}
}

// TestPerfTextEngineOpenChild is TestPerfTextEngineOpen's fresh process:
// it times reading $TANGO_EDITOR_TEXTENGINE_OPEN to its first frame done
// ("open") and the first frame alone ("frame").
func TestPerfTextEngineOpenChild(t *testing.T) {
	path := os.Getenv(textEngineOpenEnv)
	if path == "" {
		t.Skip("only run by TestPerfTextEngineOpen")
	}
	metalDevice(t) // a running application has its device
	start := time.Now()
	h := openTextEngine(t, path)
	t1 := time.Now()
	h.frame(t)
	end := time.Now()
	if !h.vw.vs.text.Empty() && h.st.Viewport.LineHeight == 0 {
		t.Fatal("no layout")
	}
	fmt.Printf("OPEN open=%v frame=%v\n", end.Sub(start), end.Sub(t1))
}
