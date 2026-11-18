//go:build darwin && !ios && tango_offmainprobe

package darwin

// Gating probe for per-window UI threads (plan Task 1): does Core Animation
// layer content committed from a background thread reach the screen on time
// while AppKit blocks the main thread (foreground switches)?
//
// TestOffMainLayerPresentProbe re-executes the test binary once per group;
// each child runs AppKit on the process's original thread (init below, as
// canvas_shortcut_test.go does) and:
//
//   - opens a real Tango canvas window (800×600, always on top) and takes
//     its root CALayer: the hosted layer of UseCoreGraphicsLayer (the native
//     path, arm64) or else the view's backing layer;
//   - adds a 16×16 CAMetalLayer sublayer (presentsWithTransaction=NO);
//   - at 60 Hz, in one explicit CATransaction, sets the root layer's contents
//     to one of two alternating full-window CGImages, commits (and, off the
//     main thread, flushes), then in the same tick presents a drawable of the
//     Metal layer with -[MTLCommandBuffer presentDrawable:] + commit, as the
//     Swift spike and render/gpu/metal/presenttrace_darwin.go do. The
//     drawable's addPresentedHandler: presentedTime is the PRIMARY proxy. It
//     measures that thread's frame cadence reaching the display, NOT the
//     contents layer itself: with presentsWithTransaction=NO the drawable is
//     shown independently of the CATransaction (with YES, on this system,
//     presentedTime was 0 for every drawable, in both groups);
//   - ground truth for the contents: every 250 ms a capture of just this
//     window (CGWindowListCreateImage, else ScreenCaptureKit) is sampled at
//     its centre and compared with the last committed image; it needs Screen
//     Recording permission (checked with CGPreflightScreenCaptureAccess, which
//     never prompts) and records "capture unavailable" otherwise;
//   - group "offmain": the commits run on a goroutine locked to its own OS
//     thread, paced by a 60 Hz wall clock;
//     group "mainthread" (control): the same commits run in the canvas
//     view's display link callback on the main thread;
//   - a 4 ms repeating CFRunLoopTimer on the main run loop (common modes)
//     records main-thread blockage;
//   - phases: warmup 2 s (recorded, not judged), quiet 15 s, switching 20 s
//     (Finder ↔ this window every 2 s via NSRunningApplication), then a short tail.
//
// TANGO_OFFMAIN_SHORT=1 is a ~10 s reproducer: warmup 1 s, quiet 3 s, no
// switching, both groups. Each child logs timestamped stages to stderr; its
// watchdog writes a goroutine dump and a native `sample` of
// the process before exiting.
//
// The child runs from TestMain (MainThreadTestHook, called by
// duplexbin_e2e_test.go before m.Run), not from init(): during package
// initialisation the runtime parks every Go callback that arrives on a
// thread Go did not create (runtime/cgocall.go, cgocallbackg: ncgo == 0 →
// <-mainInitDoneChan), and the presentedHandler block runs on Metal's
// completion queue. init only pins the main goroutine to the main thread.
//
// Metal feedback is diagnostic only. Short mode requires usable quiet-phase
// captures in both groups; full mode also requires usable switching captures
// taken entirely within measured main-thread gaps in both groups.
// TANGO_OFFMAIN_CAPTURE=0 turns the window captures off.

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

const (
	offMainChildEnv = "TANGO_OFFMAIN_PROBE_CHILD" // group name: the process is a probe child
	offMainFileEnv  = "TANGO_OFFMAIN_PROBE_FILE"  // where the child writes its group JSON
	offMainOutEnv   = "TANGO_OFFMAIN_OUT"         // final combined JSON

	offMainWidth, offMainHeight = 800, 600
	offMainHz                   = 60
	offMainProbeEvery           = 4 * time.Millisecond
	offMainMetalSide            = 16
	offMainSwitchEvery          = 2 * time.Second
	offMainTail                 = 300 * time.Millisecond // let the last drawables reach the screen
	offMainBlockMs              = 20.0                   // a main-thread gap or present interval above this is late
	offMainCaptureEvery         = 250 * time.Millisecond
	offMainCaptureFast          = 50 * time.Millisecond // during the switching phase
	offMainCaptureEnv           = "TANGO_OFFMAIN_CAPTURE"
	offMainImagesEnv            = "TANGO_OFFMAIN_IMAGES" // number of alternating contents images, 2 (default) … 6
	offMainCapturePNGs          = 60                     // per group
	offMainShortEnv             = "TANGO_OFFMAIN_SHORT"
)

var (
	offMainShort                                  = os.Getenv(offMainShortEnv) == "1"
	offMainWarmup, offMainQuiet, offMainSwitching = offMainDurations()
	offMainT0                                     = time.Now() // stage log clock
)

func offMainDurations() (warmup, quiet, switching time.Duration) {
	if offMainShort {
		return 1 * time.Second, 3 * time.Second, 0
	}
	return 2 * time.Second, 15 * time.Second, 20 * time.Second
}

// offMainChildTimeout is the child's watchdog; the parent waits 15 s more.
func offMainChildTimeout() time.Duration {
	return offMainWarmup + offMainQuiet + offMainSwitching + 30*time.Second
}

func offMainGroupNames() []string { return []string{"offmain", "mainthread"} }

// offMainStage logs a timestamped child milestone to stderr.
func offMainStage(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[offmain-probe %8.3fs] %s\n", time.Since(offMainT0).Seconds(), fmt.Sprintf(format, args...))
}

// ---- parent: the test ----

func TestOffMainLayerPresentProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a foreground window for ~80 s (~10 s with TANGO_OFFMAIN_SHORT=1)")
	}
	// Child files (group JSON, watchdog sample) go next to the result so a
	// hang's diagnostics survive the test.
	dir := t.TempDir()
	if out := os.Getenv(offMainOutEnv); out != "" {
		dir = filepath.Dir(out)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	res := offMainResult{
		Schema: "tango-offmain-probe/1",
		Date:   time.Now().Format(time.RFC3339),
		GOARCH: runtime.GOARCH,
		Params: offMainParams{
			Width: offMainWidth, Height: offMainHeight, Hz: offMainHz,
			ProbeMs: float64(offMainProbeEvery) / float64(time.Millisecond), MetalSide: offMainMetalSide,
			WarmupS: offMainWarmup.Seconds(), QuietS: offMainQuiet.Seconds(),
			SwitchingS: offMainSwitching.Seconds(), SwitchEveryS: offMainSwitchEvery.Seconds(),
			LateMs: offMainBlockMs, Short: offMainShort,
		},
		Groups: map[string]*offMainGroup{},
	}
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		res.MacOS = strings.TrimSpace(string(out))
	}
	var failed []string
	for _, g := range offMainGroupNames() {
		file := filepath.Join(dir, g+".json")
		// Never read a previous run's group file after a watchdog exit.
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), offMainChildTimeout()+15*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOffMainLayerPresentProbe$")
		cmd.Env = append(offMainChildEnviron(), offMainChildEnv+"="+g, offMainFileEnv+"="+file)
		out, err := cmd.CombinedOutput()
		cancel()
		t.Logf("child %s output:\n%s", g, strings.TrimSpace(string(out)))
		if err != nil {
			failed = append(failed, fmt.Sprintf("probe child %s: %v", g, err))
		}
		// Keep completed group JSON even if the child later exits nonzero.
		if data, rerr := os.ReadFile(file); rerr != nil {
			failed = append(failed, fmt.Sprintf("probe child %s wrote no result: %v", g, rerr))
		} else {
			var gr offMainGroup
			if jerr := json.Unmarshal(data, &gr); jerr != nil {
				failed = append(failed, fmt.Sprintf("probe child %s result: %v", g, jerr))
			} else {
				res.Groups[g] = &gr
			}
		}
		if err != nil {
			break // do not start the next window after a hang
		}
	}

	if issue := offMainEvidenceIssue(res.Groups, offMainShort); issue != "" {
		res.EvidenceIssue = "INCONCLUSIVE: " + issue
		failed = append(failed, res.EvidenceIssue)
	}
	for _, g := range offMainGroupNames() {
		gr := res.Groups[g]
		if gr == nil {
			continue
		}
		t.Logf("group %s: producer=%s layer=%s scale=%g maxFPS=%d content=%dx%d px frames=%d nilDrawables=%d errors=%v",
			g, gr.ProducerQoS, gr.Layer, gr.Scale, gr.MaxFPS, gr.ContentPx[0], gr.ContentPx[1], gr.Frames, gr.NilDrawables, gr.Errors)
		for _, ph := range []string{"quiet", "switching"} {
			p := gr.Phases[ph]
			if p == nil {
				t.Logf("  %-9s no data", ph)
				continue
			}
			t.Logf("  %-9s present n=%d p50=%.2f p99=%.2f max=%.2f ms >20ms=%.2f%% dropped=%d | commit n=%d p99=%.2f max=%.2f ms | main probe ticks=%d blocks>20ms=%d maxGap=%.1f ms",
				ph, p.Present.Count, p.Present.P50Ms, p.Present.P99Ms, p.Present.MaxMs, 100*p.Present.Over20Frac, p.PresentDropped,
				p.Commit.Count, p.Commit.P99Ms, p.Commit.MaxMs, p.ProbeTicks, p.ProbeBlocks, p.ProbeMaxGapMs)
			t.Logf("  %-9s contents capture n=%d match=%d mismatch=%d failed=%d | frame cost p50/p99/max ms: nextDrawable %.2f/%.2f/%.2f metal %.2f/%.2f/%.2f caCommit %.2f/%.2f/%.2f wakeLate %.2f/%.2f/%.2f",
				ph, p.Captures, p.CaptureMatch, p.CaptureMismatch, p.CaptureFailed,
				p.NextDrawable.P50Ms, p.NextDrawable.P99Ms, p.NextDrawable.MaxMs,
				p.Schedule.P50Ms, p.Schedule.P99Ms, p.Schedule.MaxMs,
				p.CACommit.P50Ms, p.CACommit.P99Ms, p.CACommit.MaxMs,
				p.WakeLate.P50Ms, p.WakeLate.P99Ms, p.WakeLate.MaxMs)
			t.Logf("  %-9s contents behind_at_done: in main block n=%d mean=%.2f max=%d %v | outside n=%d mean=%.2f max=%d %v | crossing n=%d mean=%.2f max=%d %v | main blocks=%d",
				ph, p.CaptureInBlock.Count, p.CaptureInBlock.Mean, p.CaptureInBlock.Max, p.CaptureInBlock.Histogram,
				p.CaptureOutBlock.Count, p.CaptureOutBlock.Mean, p.CaptureOutBlock.Max, p.CaptureOutBlock.Histogram,
				p.CaptureCrossBlock.Count, p.CaptureCrossBlock.Mean, p.CaptureCrossBlock.Max, p.CaptureCrossBlock.Histogram, len(p.MainBlocks))
		}
		deact := 0
		for _, e := range gr.Lifecycle {
			if e.Event == "background" && e.AtS >= gr.Marks.SwitchingS {
				deact++
			}
		}
		st := gr.SelfTest
		t.Logf("  proxy self-test: metal-only presented %d/%d, with contents presented %d/%d",
			st.MetalOnlyPresented, st.MetalOnlyHandlers, st.WithContentsPresented, st.WithContentsHandlers)
		t.Logf("  switches=%d (failed %d), app deactivations during switching=%d; capture: %s; pacing: %s",
			len(gr.Switches), offMainFailedSwitches(gr), deact, gr.CaptureStatus, gr.Pacing)
		if deact == 0 && offMainSwitching > 0 {
			t.Logf("  WARNING: group %s never lost activation: the foreground switching did not happen", g)
		}
	}

	path := os.Getenv(offMainOutEnv)
	if path == "" {
		path = filepath.Join(os.TempDir(), fmt.Sprintf("tango-offmain-probe-%d.json", time.Now().Unix()))
		t.Logf("%s not set; writing to %s", offMainOutEnv, path)
	}
	data, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("result: %s", path)
	if len(failed) > 0 {
		t.Fatal(strings.Join(failed, "; "))
	}
}

// offMainEvidenceIssue checks evidence availability, not native performance.
func offMainEvidenceIssue(groups map[string]*offMainGroup, short bool) string {
	method := ""
	for _, name := range offMainGroupNames() {
		g := groups[name]
		if g == nil || !strings.HasPrefix(g.CaptureStatus, "ok: ") {
			return name + ": capture unavailable"
		}
		if method != "" && method != g.CaptureStatus {
			return fmt.Sprintf("CAPTURE METHOD MISMATCH: %q vs %q", method, g.CaptureStatus)
		}
		method = g.CaptureStatus
		quiet := g.Phases["quiet"]
		if quiet == nil || quiet.CaptureUsable == 0 {
			return name + ": no usable quiet captures"
		}
		if !short {
			switching := g.Phases["switching"]
			if switching == nil || switching.CaptureInBlock.Count == 0 {
				return name + ": no usable switching capture entirely inside a main-thread gap"
			}
		}
	}
	return ""
}

// offMainChildEnviron is the parent's environment without
// TANGO_LAUNCH_BACKGROUND: the child's window must be visible and active.
func offMainChildEnviron() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TANGO_LAUNCH_BACKGROUND=") {
			env = append(env, kv)
		}
	}
	return env
}

func offMainFailedSwitches(g *offMainGroup) int {
	n := 0
	for _, s := range g.Switches {
		if s.Err != "" {
			n++
		}
	}
	return n
}

// ---- result types (child writes a group, parent merges) ----

type offMainResult struct {
	EvidenceIssue string                   `json:"evidence_issue,omitempty"`
	Schema        string                   `json:"schema"`
	Date          string                   `json:"date"`
	MacOS         string                   `json:"macos"`
	GOARCH        string                   `json:"goarch"`
	Params        offMainParams            `json:"params"`
	Groups        map[string]*offMainGroup `json:"groups"`
}

type offMainParams struct {
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Hz           int     `json:"hz"`
	ProbeMs      float64 `json:"probe_ms"`
	MetalSide    int     `json:"metal_layer_side"`
	WarmupS      float64 `json:"warmup_s"`
	QuietS       float64 `json:"quiet_s"`
	SwitchingS   float64 `json:"switching_s"`
	SwitchEveryS float64 `json:"switch_every_s"`
	LateMs       float64 `json:"late_ms"`
	Short        bool    `json:"short"`
}

type offMainStats struct {
	Count      int     `json:"count"`
	P50Ms      float64 `json:"p50_ms"`
	P99Ms      float64 `json:"p99_ms"`
	MaxMs      float64 `json:"max_ms"`
	Over20Frac float64 `json:"frac_over_20ms"`
}

type offMainPhase struct {
	StartS         float64      `json:"start_s"`
	EndS           float64      `json:"end_s"`
	Present        offMainStats `json:"present_interval"`
	PresentDropped int          `json:"present_dropped"` // presentedHandler reported presentedTime 0
	Commit         offMainStats `json:"commit_interval"`
	// Contents ground truth (window captures whose request falls in the phase).
	Captures        int `json:"captures"`
	CaptureMatch    int `json:"capture_match"`    // shows the image of the last commit at request time
	CaptureMismatch int `json:"capture_mismatch"` // differs from the latest image at request time
	CaptureFailed   int `json:"capture_failed"`
	CaptureUsable   int `json:"capture_usable"`
	// behind_at_done of classified captures, split by whether the capture
	// lies entirely in a main-thread gap (>20 ms), crosses a gap boundary, or is outside.
	CaptureInBlock    offMainBehind `json:"capture_behind_in_main_block"`
	CaptureOutBlock   offMainBehind `json:"capture_behind_outside_main_block"`
	CaptureCrossBlock offMainBehind `json:"capture_behind_crossing_main_block"`
	MainBlocks        [][2]float64  `json:"main_blocks_s"` // [start, end] of each probe gap > 20 ms
	// Per-frame cost breakdown (by commit time): nextDrawable, Metal encode +
	// presentDrawable + commit, CATransaction begin…commit(+flush), and how
	// late the producer woke against its absolute deadline (off-main only).
	NextDrawable  offMainStats `json:"frame_next_drawable_ms"`
	Schedule      offMainStats `json:"frame_metal_ms"`
	CACommit      offMainStats `json:"frame_ca_commit_ms"`
	WakeLate      offMainStats `json:"frame_wake_late_ms"`
	ProbeTicks    int          `json:"probe_ticks"`
	ProbeBlocks   int          `json:"probe_blocks_over_20ms"`
	ProbeMaxGapMs float64      `json:"probe_max_gap_ms"`

	PresentIntervalsMs []float64 `json:"present_intervals_ms"`
	CommitIntervalsMs  []float64 `json:"commit_intervals_ms"`
	ProbeGapsMs        []float64 `json:"probe_gaps_ms"`
}

type offMainMarks struct {
	QuietS     float64 `json:"quiet_s"`
	SwitchingS float64 `json:"switching_s"`
	EndS       float64 `json:"end_s"`
}

type offMainSwitch struct {
	AtS    float64 `json:"at_s"`
	DoneS  float64 `json:"done_s"`
	Target string  `json:"target"` // "finder" or "self"
	Method string  `json:"method"`
	Err    string  `json:"err,omitempty"`
}

type offMainEvent struct {
	AtS   float64 `json:"at_s"`
	Event string  `json:"event"`
}

// offMainCheck samples the window's state on the main thread one second
// after each switch (and at the phase starts).
type offMainCheck struct {
	AtS      float64 `json:"at_s"`
	Label    string  `json:"label"`
	Active   bool    `json:"app_active"`
	Visible  bool    `json:"window_visible"`
	Occluded bool    `json:"window_occluded"`
}

type offMainGroup struct {
	Group        string                   `json:"group"`
	Layer        string                   `json:"layer"` // which root layer was driven
	Scale        float64                  `json:"scale"`
	MaxFPS       int                      `json:"screen_max_fps"`
	ContentPx    [2]int                   `json:"content_px"`
	Frames       int                      `json:"frames"`
	NilDrawables int                      `json:"nil_drawables"`
	Errors       []string                 `json:"errors"`
	Marks        offMainMarks             `json:"marks"`
	Phases       map[string]*offMainPhase `json:"phases"` // warmup, quiet, switching, tail
	Switches     []offMainSwitch          `json:"switches"`
	Lifecycle    []offMainEvent           `json:"lifecycle"`
	Checks       []offMainCheck           `json:"checks"`

	// Every sample, seconds since the group's t0 (CACurrentMediaTime base).
	RawPresentedS []float64        `json:"raw_presented_s"`
	RawDroppedS   []float64        `json:"raw_dropped_s"` // handler callback time of presentedTime==0
	RawCaptures   []offMainCapture `json:"raw_captures"`
	CaptureStatus string           `json:"capture_status"`
	Images        int              `json:"images"`
	Diag          []string         `json:"diag"`
	Pacing        string           `json:"pacing"`
	RawFrames     []offMainFrame   `json:"raw_frames"`
	RawHandlers   []offMainHandler `json:"raw_handlers"`
	SelfTest      offMainSelfTest  `json:"proxy_selftest"`
	ProducerQoS   string           `json:"producer_qos"`
	RawCommitS    []float64        `json:"raw_commit_s"`
	RawProbeS     []float64        `json:"raw_probe_s"`
}

// offMainHandler is one presentedHandler callback.
type offMainHandler struct {
	Frame      int     `json:"frame"`
	MetalOnly  bool    `json:"metal_only,omitempty"`
	CallbackS  float64 `json:"callback_s"`
	PresentedS float64 `json:"presented_s"` // 0: presentedTime was 0 (discarded)
}

// offMainSelfTest compares presentation of drawables from frames without a
// contents commit (the first half of warmup) with frames that have one.
type offMainSelfTest struct {
	MetalOnlyHandlers     int `json:"metal_only_handlers"`
	MetalOnlyPresented    int `json:"metal_only_presented"`
	WithContentsHandlers  int `json:"with_contents_handlers"`
	WithContentsPresented int `json:"with_contents_presented"`
}

// offMainFrame is one commitFrame's cost breakdown.
type offMainFrame struct {
	AtS            float64 `json:"at_s"`                 // commit done
	MetalOnly      bool    `json:"metal_only,omitempty"` // proxy self-test frame: no contents commit
	NextDrawableMs float64 `json:"next_drawable_ms"`
	ScheduleMs     float64 `json:"metal_ms"` // encode + presentDrawable + commit
	CACommitMs     float64 `json:"ca_commit_ms"`
	WakeLateMs     float64 `json:"wake_late_ms"`
}

// offMainBehind summarises behind_at_done of a set of captures.
type offMainBehind struct {
	Count     int         `json:"count"`
	Mean      float64     `json:"mean"`
	Max       int         `json:"max"`
	Histogram map[int]int `json:"histogram"`
}

func offMainSummarizeBehind(v []int) offMainBehind {
	b := offMainBehind{Histogram: map[int]int{}}
	sum := 0
	for _, x := range v {
		b.Count++
		sum += x
		b.Max = max(b.Max, x)
		b.Histogram[x]++
	}
	if b.Count > 0 {
		b.Mean = float64(sum) / float64(b.Count)
	}
	return b
}

// offMainCapture is one ground-truth sample of the window's contents. With
// two alternating images only the parity of the staleness is known: a
// match means 0 (or an even number of) commits behind, a mismatch an odd
// number.
type offMainCapture struct {
	AtS        float64 `json:"at_s"`   // request
	DoneS      float64 `json:"done_s"` // image in hand
	Method     string  `json:"method"`
	Size       [2]int  `json:"size_px"`
	RGB        [3]int  `json:"rgb"`           // centre pixel of the content area
	MetalRGB   [3]int  `json:"metal_rgb"`     // centre of the 16×16 Metal view (top-left)
	Expected   [3]int  `json:"expected_rgb"`  // colour of the last committed image at request
	Shown      int     `json:"shown"`         // palette index the pixel matches, -1 unknown
	BehindReq  int     `json:"behind_at_req"` // commits behind the latest at request, modulo the image count; -1 unknown
	BehindDone int     `json:"behind_at_done"`
	PNG        string  `json:"png,omitempty"`
	LatestReq  int     `json:"latest_at_req"`  // index of the last committed frame at request
	LatestDone int     `json:"latest_at_done"` // … when the image was in hand
	MatchReq   bool    `json:"match_at_req"`
	MatchDone  bool    `json:"match_at_done"`
	Err        string  `json:"err,omitempty"`
}

// ---- child: runs on the main thread from TestMain, before the testing runner ----

func init() {
	if os.Getenv(offMainChildEnv) == "" {
		return
	}
	// Keep the main goroutine on the main thread until TestMain calls the
	// hook; the child itself must run after init (see the file comment).
	runtime.LockOSThread()
	MainThreadTestHook = offMainChildMain
}

// offMainChildMain is MainThreadTestHook: the child's whole life, on the
// main thread after package initialisation.
func offMainChildMain() (int, bool) {
	group := os.Getenv(offMainChildEnv)
	if group == "" {
		return 0, false
	}
	if !isMainThread() {
		fmt.Fprintln(os.Stderr, "offmain probe child: hook not on the main thread")
		return 1, true
	}
	outPath := os.Getenv(offMainFileEnv)
	offMainStage("child %s pid %d start from TestMain (short=%v, watchdog %v)", group, os.Getpid(), offMainShort, offMainChildTimeout())
	go func() {
		time.Sleep(offMainChildTimeout())
		offMainWatchdog(outPath)
		os.Exit(3)
	}()
	if err := offMainChild(group, outPath); err != nil {
		offMainStage("child error: %v", err)
		fmt.Fprintln(os.Stderr, "offmain probe child:", err)
		return 1, true
	}
	offMainStage("child exit 0")
	return 0, true
}

// offMainWatchdog reports a hung child without reading probe state: every goroutine's
// stack, and a 1 s native `sample` of the process (C stacks of threads
// stuck inside AppKit / Core Animation / Metal, which the Go dump shows
// only as a purego syscall).
func offMainWatchdog(outPath string) {
	offMainStage("WATCHDOG timeout after %v", offMainChildTimeout())
	buf := make([]byte, 8<<20)
	n := runtime.Stack(buf, true)
	fmt.Fprintf(os.Stderr, "---- goroutine dump ----\n%s\n---- end goroutine dump ----\n", buf[:n])
	if outPath != "" {
		file := outPath + ".sample.txt"
		out, err := exec.Command("/usr/bin/sample", fmt.Sprint(os.Getpid()), "1", "-mayDie", "-file", file).CombinedOutput()
		offMainStage("watchdog: sample → %s err=%v %s", file, err, strings.TrimSpace(string(out)))
	}
}

// Bindings the product code does not have.
var (
	offMainCANow      func() float64
	offMainMTLDevice  func() id
	offMainSetClear   func(obj id, s objc.SEL, c offMainClearColor)
	offMainGetSize    func(obj id, s objc.SEL) nsSize
	offMainGetF32     func(obj id, s objc.SEL) float32
	offMainF64        func(obj id, s objc.SEL) float64 // as metal's msgF64
	offMainSetID      func(obj id, s objc.SEL, v id)   // as metal's msgSetID
	offMainTimerCB    = purego.NewCallback(func(timer, info uintptr) { offMainRun.probeTick() })
	offMainRun        *offMainState // the child's single probe
	offMainTargetOnce sync.Once
)

type offMainClearColor struct{ R, G, B, A float64 }

// offMainState is one child's probe. Fields without a comment are set on
// the main thread before the producer starts and only read afterwards.
type offMainState struct {
	b       *backend
	group   string
	outPath string
	offMain bool

	c         *canvas
	root      id // CALayer whose contents change
	metal     id // 16×16 CAMetalLayer sublayer (retained)
	queue     id // MTLCommandQueue
	images    []uintptr
	metalView id // 16×16 layer-hosting NSView holding the CAMetalLayer, sibling of the canvas view
	device    id
	winW      float64 // window frame size in points (capture geometry)
	winH      float64
	t0        float64

	frame    int     // producer only
	wakeLate float64 // producer only: lateness of the current frame's wake-up, s

	// Main-thread group: display link and pacing (main thread only).
	link    id
	target  id
	nextDue float64
	running bool

	// Off-main group.
	stop chan struct{}
	done chan struct{}

	timer uintptr // main-thread probe CFRunLoopTimer

	mu                        sync.Mutex   // guards everything below
	header                    offMainGroup // static facts about the run
	presented                 []float64
	dropped                   []float64
	captures                  []offMainCapture
	captureStatus             string
	diag                      []string
	pacing                    string
	frameRecs                 []offMainFrame
	handlers                  []offMainHandler
	qos                       string
	commits                   []float64
	probe                     []float64
	nilDraw                   int
	frames                    int
	errs                      []string
	lifecycle                 []offMainEvent
	checks                    []offMainCheck
	switches                  []offMainSwitch
	marks                     offMainMarks
	firstCommit, firstPresent bool

	lastCommitted atomic.Int64 // index of the last frame whose CA commit returned (-1 none)
	metalOnly     atomic.Bool  // proxy self-test: present drawables without committing contents
	captureStop   chan struct{}
	captureDone   chan struct{}

	hostMu sync.Mutex
	queued []func()
}

func (p *offMainState) now() float64 { return offMainCANow() - p.t0 }

func (p *offMainState) errorf(format string, args ...any) {
	p.mu.Lock()
	p.errs = append(p.errs, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

// post runs fn on the main thread (Host.Drain).
func (p *offMainState) post(fn func()) {
	p.hostMu.Lock()
	p.queued = append(p.queued, fn)
	p.hostMu.Unlock()
	p.b.Wake()
}

// platform.Host for the child.
func (p *offMainState) Drain() {
	for {
		p.hostMu.Lock()
		q := p.queued
		p.queued = nil
		p.hostMu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

func (p *offMainState) OnLifecycle(ev platform.LifecycleEvent) {
	name := ""
	switch ev {
	case platform.LifecycleStarted:
		offMainStage("lifecycle started (applicationDidFinishLaunching)")
		if err := p.start(); err != nil {
			offMainStage("start failed: %v", err)
			p.errorf("start: %v", err)
			p.finish()
		}
		return
	case platform.LifecycleForeground:
		name = "foreground"
	case platform.LifecycleBackground:
		name = "background"
	default:
		return
	}
	if offMainCANow == nil || p.t0 == 0 {
		return
	}
	p.mu.Lock()
	p.lifecycle = append(p.lifecycle, offMainEvent{AtS: p.now(), Event: name})
	p.mu.Unlock()
}

// Only the scripted shutdown stops the producer before AppKit tears down its layers.
func (p *offMainState) ShouldClose(platform.SurfaceID) bool                      { return false }
func (p *offMainState) OnSurfaceEvent(platform.SurfaceID, platform.SurfaceEvent) {}
func (p *offMainState) OnMessage(platform.SurfaceID, string, string)             {}
func (p *offMainState) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// offMainSink ignores the canvas's own frames: the probe drives the layer.
type offMainSink struct{}

func (offMainSink) CanvasFrame()                            {}
func (offMainSink) CanvasInput(input.Event)                 {}
func (offMainSink) CanvasResized(float32, float32, float32) {}
func (offMainSink) CanvasFocus(bool)                        {}

func offMainChild(group, outPath string) error {
	if group != "offmain" && group != "mainthread" {
		return fmt.Errorf("unknown group %q", group)
	}
	if outPath == "" {
		return fmt.Errorf("%s not set", offMainFileEnv)
	}
	b := New().(*backend)
	p := &offMainState{b: b, group: group, outPath: outPath, offMain: group == "offmain"}
	offMainRun = p
	if err := b.Init(p, platform.AppOptions{Name: "Tango offmain probe"}); err != nil {
		return err
	}
	offMainStage("backend initialised; entering NSApp run")
	if err := b.Run(); err != nil { // applicationDidFinishLaunching → start
		return err
	}
	offMainStage("NSApp run returned")
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.errs) > 0 {
		return fmt.Errorf("%s", strings.Join(p.errs, "; "))
	}
	return nil
}

// start runs on the main thread once the app has launched (and activated).
func (p *offMainState) start() error {
	libQC := mustDlopen(frameworksDir + "QuartzCore.framework/QuartzCore")
	libMetal := mustDlopen(frameworksDir + "Metal.framework/Metal")
	purego.RegisterLibFunc(&offMainCANow, libQC, "CACurrentMediaTime")
	purego.RegisterLibFunc(&offMainMTLDevice, libMetal, "MTLCreateSystemDefaultDevice")
	purego.RegisterFunc(&offMainSetClear, msgSendAddr)
	purego.RegisterFunc(&offMainGetSize, msgSendAddr)
	purego.RegisterFunc(&offMainGetF32, msgSendAddr)
	purego.RegisterFunc(&offMainF64, msgSendAddr)
	purego.RegisterFunc(&offMainSetID, msgSendAddr)
	p.t0 = offMainCANow()

	_, pc, err := p.b.NewCanvasSurface(1, platform.SurfaceOptions{
		Title: "Tango offmain probe (" + p.group + ")", Width: offMainWidth, Height: offMainHeight, Visible: true,
	}, platform.SurfaceExtras{Centered: true, Appearance: platform.Appearance{AlwaysOnTop: true}}, offMainSink{})
	if err != nil {
		return fmt.Errorf("NewCanvasSurface: %w", err)
	}
	p.c = pc.(*canvas)
	offMainStage("window shown (visible=%v)", sendBool(p.c.s.win, "isVisible"))
	if cg, ok := pc.(platform.CanvasCoreGraphics); ok {
		if l, err := cg.UseCoreGraphicsLayer(); err == nil && l != 0 {
			p.root = id(l)
		}
	}
	layerKind := "UseCoreGraphicsLayer"
	if p.root == 0 {
		p.root = send(p.c.view, "layer")
		layerKind = "view-backing-layer"
	}
	if p.root == 0 {
		return fmt.Errorf("canvas view has no layer")
	}
	if !hasClass("CAMetalLayer") {
		return fmt.Errorf("no CAMetalLayer")
	}
	dev := offMainMTLDevice()
	if dev == 0 {
		return fmt.Errorf("MTLCreateSystemDefaultDevice returned nil")
	}
	p.queue = send(dev, "newCommandQueue")

	w, h, scale := p.c.geometry()
	pw, ph := int(math.Round(w*scale)), int(math.Round(h*scale))
	for _, rgb := range offMainPalette[:offMainImageCount()] {
		pix := make([]byte, pw*ph*4)
		for k := 0; k < len(pix); k += 4 {
			copy(pix[k:k+4], []byte{rgb[0], rgb[1], rgb[2], 0xff})
		}
		img, err := newCGImage(pix, pw*4, pw, ph)
		if err != nil {
			return err
		}
		p.images = append(p.images, img)
	}
	p.device = dev
	fr := msgRect(p.c.s.win, sel("frame"))
	p.winW, p.winH = fr.Size.Width, fr.Size.Height

	withPool(func() {
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		msgSetFloat(p.root, sel("setContentsScale:"), scale)
		m := retain(send(class("CAMetalLayer"), "layer"))
		send(m, "setActions:", uintptr(noActions()))
		send(m, "setDevice:", uintptr(dev))
		send(m, "setPixelFormat:", 80) // MTLPixelFormatBGRA8Unorm
		send(m, "setFramebufferOnly:", 1)
		send(m, "setOpaque:", 1)
		send(m, "setPresentsWithTransaction:", 0) // present via the command buffer (see the file comment)
		// As presenter_darwin.go: sRGB colorspace, nextDrawable timeout, and
		// (as the canvas does for its hosted layer) contentsScale = backing
		// scale with a drawable in pixels.
		send(m, "setAllowsNextDrawableTimeout:", 1)
		if srgbSpace != 0 {
			send(m, "setColorspace:", srgbSpace)
		}
		msgSetFloat(m, sel("setContentsScale:"), scale)
		msgSetSize(m, sel("setDrawableSize:"), nsSize{offMainMetalSide * scale, offMainMetalSide * scale})
		p.metal = m
		send(ca, "commit")
		// The Metal layer is hosted by its own 16×16 view, a sibling of the
		// canvas view above it (top-left), the way the Swift spike and the
		// product presenter host a CAMetalLayer (a view's own layer), not a
		// sublayer of the canvas's hosted CALayer: as a sublayer its
		// drawables were never reported presented (short runs 2 and 3).
		super := send(p.c.view, "superview")
		cf := msgRect(p.c.view, sel("frame"))
		r := nsRect{Origin: nsPoint{cf.Origin.X, cf.Origin.Y + cf.Size.Height - offMainMetalSide}, Size: nsSize{offMainMetalSide, offMainMetalSide}}
		if sendBool(super, "isFlipped") {
			r.Origin.Y = cf.Origin.Y
		}
		mv := msgInitRect(send(class("NSView"), "alloc"), sel("initWithFrame:"), r)
		send(mv, "setLayer:", uintptr(m)) // layer-hosting: setLayer before setWantsLayer
		send(mv, "setWantsLayer:", 1)
		const nsWindowAbove = 1
		send(super, "addSubview:positioned:relativeTo:", uintptr(mv), nsWindowAbove, uintptr(p.c.view))
		p.metalView = mv
	})

	maxFPS := 0
	if scr := send(class("NSScreen"), "mainScreen"); scr != 0 && respondsTo(scr, "maximumFramesPerSecond") {
		maxFPS = sendInt(scr, "maximumFramesPerSecond")
	}
	p.mu.Lock()
	p.header = offMainGroup{Group: p.group, Layer: layerKind, Scale: scale, MaxFPS: maxFPS, ContentPx: [2]int{pw, ph}}
	p.mu.Unlock()

	// Main-thread blockage probe.
	var tctx [5]uintptr // CFRunLoopTimerContext
	every := offMainProbeEvery.Seconds()
	p.timer = cfRunLoopTimerCreate(0, cfAbsoluteTimeGetCurrent()+every, every, 0, 0, offMainTimerCB, unsafe.Pointer(&tctx[0]))
	cfRunLoopAddTimer(cfRunLoopGetMain(), p.timer, kCFRunLoopCommonModes)

	offMainStage("layers ready: root=%s %dx%d px @%g, metal %dx%d, maxFPS %d; probe timer armed",
		layerKind, pw, ph, scale, offMainMetalSide, offMainMetalSide, maxFPS)
	if p.offMain {
		p.stop, p.done = make(chan struct{}), make(chan struct{})
		go p.offMainLoop()
	} else if err := p.startDisplayLink(); err != nil {
		return err
	}
	offMainStage("producer started (%s)", p.group)
	p.lastCommitted.Store(-1)
	p.captureStop, p.captureDone = make(chan struct{}), make(chan struct{})
	go p.captureLoop(uint32(sendInt(p.c.s.win, "windowNumber")))
	go p.script()
	return nil
}

// probeTick is the 4 ms main-thread timer.
func (p *offMainState) probeTick() {
	t := p.now()
	p.mu.Lock()
	p.probe = append(p.probe, t)
	p.mu.Unlock()
}

// commitFrame is the work under test: one CATransaction carrying the new
// root contents (committed, and flushed off the main thread), then in the
// same tick a Metal drawable presented through its command buffer, whose
// presentedHandler timestamps this thread's frame reaching the display.
// The handler is registered before presentDrawable/commit and reads
// presentedTime from the drawable it is given (as presenttrace_darwin.go;
// 0 there means the drawable was discarded). Inside an autorelease pool.
func (p *offMainState) commitFrame() {
	i := p.frame
	p.frame++
	metalOnly := p.metalOnly.Load()
	t0 := p.now()
	drawable := send(p.metal, "nextDrawable") // nil after a 1 s timeout
	t1 := p.now()
	var cb id
	if drawable != 0 {
		rpd := send(class("MTLRenderPassDescriptor"), "renderPassDescriptor")
		att := send(send(rpd, "colorAttachments"), "objectAtIndexedSubscript:", 0)
		send(att, "setTexture:", uintptr(send(drawable, "texture")))
		send(att, "setLoadAction:", 2)  // MTLLoadActionClear
		send(att, "setStoreAction:", 1) // MTLStoreActionStore
		v := float64(i%2) * 0.8
		offMainSetClear(att, sel("setClearColor:"), offMainClearColor{v, 0.5, 0.8 - v, 1})
		cb = send(p.queue, "commandBuffer")
		send(send(cb, "renderCommandEncoderWithDescriptor:", uintptr(rpd)), "endEncoding")
	}
	t2 := p.now()

	if !metalOnly {
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		send(p.root, "setContents:", p.images[i%len(p.images)])
		send(ca, "commit")
		if p.offMain {
			send(ca, "flush") // no run loop observer commits for this thread
		}
		p.lastCommitted.Store(int64(i))
	}
	t3 := p.now()
	if drawable != 0 {
		// As presenttrace_darwin.go watchDrawable: the handler is added
		// right before presentDrawable is encoded, reads presentedTime from
		// the drawable it is handed, and our block reference is released at
		// once (the native API holds its copy).
		frame := i
		blk := objc.NewBlock(func(_ objc.Block, shown id) {
			pt := offMainF64(shown, sel("presentedTime"))
			now := p.now()
			h := offMainHandler{Frame: frame, MetalOnly: metalOnly, CallbackS: now}
			p.mu.Lock()
			first := false
			if pt > 0 {
				h.PresentedS = pt - p.t0
				p.presented = append(p.presented, h.PresentedS)
				first = !p.firstPresent
				p.firstPresent = true
			} else {
				p.dropped = append(p.dropped, now)
			}
			p.handlers = append(p.handlers, h)
			p.mu.Unlock()
			if first {
				offMainStage("first presentedHandler (frame %d, presentedTime t=%.3fs, callback at %.3fs, metalOnly=%v)", frame, h.PresentedS, now, metalOnly)
			}
		})
		offMainSetID(drawable, sel("addPresentedHandler:"), id(blk))
		blk.Release()
		offMainSetID(cb, sel("presentDrawable:"), drawable)
		send(cb, "commit")
	}
	t := p.now()
	rec := offMainFrame{AtS: t, NextDrawableMs: (t1 - t0) * 1000, ScheduleMs: ((t2 - t1) + (t - t3)) * 1000,
		CACommitMs: (t3 - t2) * 1000, WakeLateMs: p.wakeLate * 1000, MetalOnly: metalOnly}
	p.mu.Lock()
	p.commits = append(p.commits, t3)
	p.frameRecs = append(p.frameRecs, rec)
	p.frames++
	if drawable == 0 {
		p.nilDraw++
	}
	first := !p.firstCommit
	p.firstCommit = true
	p.mu.Unlock()
	if first {
		offMainStage("first commit (t=%.3fs, drawable=%v)", t3, drawable != 0)
	}
}

// offMainLoop is the off-main producer: its own OS thread at
// user-interactive QoS (as the Swift spike's Thread), paced on absolute
// 60 Hz deadlines: the deadline advances by one period per frame whatever
// the frame cost, and only resyncs when a whole period was missed. It waits
// with mach_wait_until on the absolute deadline: time.Sleep on a locked
// thread goes through the Go timer and a hand-off to the locked M, which
// woke ~1.9 ms late (short run 2).
func (p *offMainState) offMainLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(p.done)
	defer offMainStage("off-main producer stopped")
	qos := offMainSetQoS()
	wait, pacing := offMainDeadlineWaiter()
	p.mu.Lock()
	p.qos, p.pacing = qos, pacing
	p.mu.Unlock()
	offMainStage("off-main producer running on its own OS thread (qos %s, pacing %s)", qos, pacing)
	period := 1.0 / offMainHz
	next := offMainCANow()
	for {
		select {
		case <-p.stop:
			return
		default:
		}
		p.wakeLate = offMainCANow() - next
		pool := arPoolPush()
		p.commitFrame()
		arPoolPop(pool)
		next += period
		now := offMainCANow()
		if next < now-period {
			next = now // a whole period behind: skip ahead instead of bursting
		}
		if next > now {
			wait(next)
		}
	}
}

// offMainDeadlineWaiter returns a wait-until for CACurrentMediaTime
// deadlines: mach_wait_until (CACurrentMediaTime is mach_absolute_time in
// seconds), else time.Sleep.
func offMainDeadlineWaiter() (func(deadline float64), string) {
	sleep := func(deadline float64) {
		if d := deadline - offMainCANow(); d > 0 {
			time.Sleep(time.Duration(d * float64(time.Second)))
		}
	}
	waitFn, err1 := purego.Dlsym(purego.RTLD_DEFAULT, "mach_wait_until")
	tbFn, err2 := purego.Dlsym(purego.RTLD_DEFAULT, "mach_timebase_info")
	if err1 != nil || err2 != nil || waitFn == 0 || tbFn == 0 {
		return sleep, "time.Sleep (mach_wait_until missing)"
	}
	var tb struct{ numer, denom uint32 }
	if r, _, _ := purego.SyscallN(tbFn, uintptr(unsafe.Pointer(&tb))); r != 0 || tb.numer == 0 || tb.denom == 0 {
		return sleep, "time.Sleep (mach_timebase_info failed)"
	}
	ticksPerSecond := 1e9 * float64(tb.denom) / float64(tb.numer)
	return func(deadline float64) {
		purego.SyscallN(waitFn, uintptr(uint64(deadline*ticksPerSecond)))
	}, fmt.Sprintf("mach_wait_until (timebase %d/%d)", tb.numer, tb.denom)
}

// offMainSetQoS puts the calling thread at QOS_CLASS_USER_INTERACTIVE.
func offMainSetQoS() string {
	const qosClassUserInteractive = 0x21
	fn, err := purego.Dlsym(purego.RTLD_DEFAULT, "pthread_set_qos_class_self_np")
	if err != nil || fn == 0 {
		return "default (pthread_set_qos_class_self_np missing)"
	}
	if r, _, _ := purego.SyscallN(fn, qosClassUserInteractive, 0); r != 0 {
		return fmt.Sprintf("default (pthread_set_qos_class_self_np: %d)", r)
	}
	return "user-interactive"
}

// startDisplayLink sets up the control group: the same commits from the
// canvas view's display link on the main thread, decimated to 60 Hz on
// faster displays.
func (p *offMainState) startDisplayLink() error {
	offMainTargetOnce.Do(func() {
		classDef("TangoOffMainProbeTarget", "NSObject", nil, []objc.MethodDef{
			method("offMainTick:", func(_ id, _ objc.SEL, _ id) { offMainRun.displayTick() }),
		})
	})
	if !respondsTo(p.c.view, "displayLinkWithTarget:selector:") || nsRunLoopCommon == 0 {
		return fmt.Errorf("NSView display link unavailable (macOS 14+)")
	}
	p.target = alloc("TangoOffMainProbeTarget")
	p.link = retain(send(p.c.view, "displayLinkWithTarget:selector:", uintptr(p.target), uintptr(sel("offMainTick:"))))
	if p.link == 0 {
		return fmt.Errorf("displayLinkWithTarget:selector: returned nil")
	}
	p.running = true
	p.nextDue = offMainCANow()
	send(p.link, "addToRunLoop:forMode:", uintptr(send(class("NSRunLoop"), "mainRunLoop")), nsRunLoopCommon)
	return nil
}

func (p *offMainState) displayTick() {
	if !p.running {
		return
	}
	const slack = 0.004 // a tick up to 4 ms early still takes the slot
	period := 1.0 / offMainHz
	now := offMainCANow()
	if now < p.nextDue-slack {
		return
	}
	withPool(p.commitFrame)
	p.nextDue += period
	if p.nextDue < now {
		p.nextDue = now + period
	}
}

// script drives the phases from a goroutine; the main thread only runs the
// activation calls, the check samples and the final collection.
func (p *offMainState) script() {
	sleepUntil := func(t float64) {
		if d := t - p.now(); d > 0 {
			time.Sleep(time.Duration(d * float64(time.Second)))
		}
	}
	p.check("warmup")
	// Proxy self-test: the first half of warmup presents drawables without
	// any contents commit, to tell whether the contents commits are what
	// makes presentedTime 0 (proxy_selftest in the JSON).
	p.metalOnly.Store(true)
	offMainStage("proxy self-test: metal-only frames for %v", offMainWarmup/2)
	sleepUntil(offMainWarmup.Seconds() / 2)
	p.metalOnly.Store(false)
	offMainStage("proxy self-test done; contents commits on")
	sleepUntil(offMainWarmup.Seconds())
	quiet := p.now()
	p.mu.Lock()
	p.marks.QuietS = quiet
	p.mu.Unlock()
	offMainStage("phase quiet starts (t=%.3fs)", quiet)
	p.check("quiet")
	p.post(p.diagnose)
	sleepUntil(quiet + offMainQuiet.Seconds())

	sw := p.now()
	p.mu.Lock()
	p.marks.SwitchingS = sw
	p.mu.Unlock()
	offMainStage("phase switching starts (t=%.3fs, %v)", sw, offMainSwitching)
	n := int(offMainSwitching / offMainSwitchEvery)
	for i := 0; i < n; i++ {
		target := "finder"
		if i%2 == 1 {
			target = "self"
		}
		offMainStage("switch %d → %s", i, target)
		p.activate(target)
		at := sw + float64(i)*offMainSwitchEvery.Seconds()
		sleepUntil(at + 1)
		p.check(fmt.Sprintf("switch %d → %s +1s", i, target))
		sleepUntil(at + offMainSwitchEvery.Seconds())
	}
	end := p.now()
	p.mu.Lock()
	p.marks.EndS = end
	p.mu.Unlock()
	offMainStage("phases done (t=%.3fs); stopping producer", end)

	if p.offMain {
		close(p.stop)
		<-p.done
	} else {
		stopped := make(chan struct{})
		p.post(func() {
			p.running = false
			send(p.link, "invalidate")
			close(stopped)
		})
		<-stopped
		offMainStage("display link invalidated")
	}
	close(p.captureStop)
	<-p.captureDone
	time.Sleep(offMainTail)
	offMainStage("posting finish to the main thread")
	p.post(p.finish)
}

// activate brings Finder or this process to the front with direct AppKit
// calls on the main thread (no osascript: no Automation permission, no
// dialog). The script goroutine waits for the call to return; activation
// itself completes asynchronously and is verified by the lifecycle events
// and check samples.
func (p *offMainState) activate(target string) {
	s := offMainSwitch{AtS: p.now(), Target: target}
	done := make(chan struct{})
	p.post(func() {
		defer close(done)
		const (
			activateAllWindows       = 1 << 0 // NSApplicationActivateAllWindows
			activateIgnoringOtherApp = 1 << 1 // NSApplicationActivateIgnoringOtherApps (ignored on macOS 14+)
		)
		if target == "finder" {
			s.Method = "NSRunningApplication(com.apple.finder) activateWithOptions:"
			apps := send(class("NSRunningApplication"), "runningApplicationsWithBundleIdentifier:", uintptr(nsString("com.apple.finder")))
			finder := send(apps, "firstObject")
			if finder == 0 {
				s.Err = "Finder is not running"
				return
			}
			if !sendBool(finder, "activateWithOptions:", activateAllWindows|activateIgnoringOtherApp) {
				s.Err = "activateWithOptions: returned NO"
			}
			return
		}
		s.Method = "NSRunningApplication.currentApplication activateWithOptions: + NSApp activateIgnoringOtherApps:YES"
		self := send(class("NSRunningApplication"), "currentApplication")
		if !sendBool(self, "activateWithOptions:", activateAllWindows|activateIgnoringOtherApp) {
			s.Err = "activateWithOptions: returned NO"
		}
		send(p.b.app, "activateIgnoringOtherApps:", 1)
	})
	<-done
	s.DoneS = p.now()
	p.mu.Lock()
	p.switches = append(p.switches, s)
	p.mu.Unlock()
}

// check samples app activation and window visibility on the main thread.
func (p *offMainState) check(label string) {
	p.post(func() {
		const nsWindowOcclusionStateVisible = 1 << 1
		win := p.c.s.win
		c := offMainCheck{
			AtS:      p.now(),
			Label:    label,
			Active:   sendBool(p.b.app, "isActive"),
			Visible:  sendBool(win, "isVisible"),
			Occluded: sendInt(win, "occlusionState")&nsWindowOcclusionStateVisible == 0,
		}
		p.mu.Lock()
		p.checks = append(p.checks, c)
		p.mu.Unlock()
	})
}

// finish runs on the main thread: stop the probe, write the group JSON and
// stop the app.
func (p *offMainState) finish() {
	offMainStage("finish called on the main thread")
	if p.timer != 0 {
		cfRunLoopTimerInvalidate(p.timer)
		p.timer = 0
	}
	g := p.snapshot()
	if err := offMainWriteJSON(p.outPath, g); err != nil {
		p.errorf("write %s: %v", p.outPath, err)
		offMainStage("JSON write failed: %v", err)
	} else {
		offMainStage("JSON written to %s", p.outPath)
	}
	for _, ph := range []string{"quiet", "switching"} {
		if s := g.Phases[ph]; s != nil {
			fmt.Printf("%s %s: present n=%d p99=%.2f ms max=%.2f ms; main blocks>20ms=%d maxGap=%.1f ms\n",
				p.group, ph, s.Present.Count, s.Present.P99Ms, s.Present.MaxMs, s.ProbeBlocks, s.ProbeMaxGapMs)
		}
	}
	offMainStage("Quit")
	p.b.Quit()
}

// snapshot copies the samples on the main thread for normal JSON output.
func (p *offMainState) snapshot() offMainGroup {
	p.mu.Lock()
	g := p.header
	g.Group = p.group
	g.Frames, g.NilDrawables = p.frames, p.nilDraw
	g.Errors = append([]string{}, p.errs...)
	g.Marks = p.marks
	g.Switches = append([]offMainSwitch{}, p.switches...)
	g.Lifecycle = append([]offMainEvent{}, p.lifecycle...)
	g.Checks = append([]offMainCheck{}, p.checks...)
	g.RawPresentedS = offMainSorted(p.presented)
	g.RawDroppedS = offMainSorted(p.dropped)
	g.RawCommitS = offMainSorted(p.commits)
	g.RawProbeS = offMainSorted(p.probe)
	g.RawCaptures = append([]offMainCapture{}, p.captures...)
	g.CaptureStatus = p.captureStatus
	g.Images = len(p.images)
	g.Diag = append([]string{}, p.diag...)
	g.Pacing = p.pacing
	g.RawFrames = append([]offMainFrame{}, p.frameRecs...)
	g.RawHandlers = append([]offMainHandler{}, p.handlers...)
	for _, h := range p.handlers {
		st := &g.SelfTest
		if h.MetalOnly {
			st.MetalOnlyHandlers++
			if h.PresentedS > 0 {
				st.MetalOnlyPresented++
			}
		} else {
			st.WithContentsHandlers++
			if h.PresentedS > 0 {
				st.WithContentsPresented++
			}
		}
	}
	g.ProducerQoS = p.qos
	if !p.offMain {
		g.ProducerQoS = "main thread"
		g.Pacing = "NSView display link, decimated to 60 Hz"
	}
	p.mu.Unlock()
	g.Phases = offMainPhases(&g)
	return g
}

func offMainWriteJSON(path string, g offMainGroup) error {
	data, err := json.MarshalIndent(g, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func offMainSorted(v []float64) []float64 {
	out := append([]float64{}, v...)
	sort.Float64s(out)
	return out
}

// offMainPhases splits every sample into phases by the time of the later
// sample of each interval; nothing is dropped. "tail" is after the end mark.
func offMainPhases(g *offMainGroup) map[string]*offMainPhase {
	m := g.Marks
	names := []string{"warmup", "quiet", "switching", "tail"}
	bounds := [][2]float64{{0, m.QuietS}, {m.QuietS, m.SwitchingS}, {m.SwitchingS, m.EndS}, {m.EndS, math.Inf(1)}}
	phaseOf := func(t float64) int {
		for i := len(bounds) - 1; i > 0; i-- {
			if t >= bounds[i][0] {
				return i
			}
		}
		return 0
	}
	ph := make([]*offMainPhase, len(names))
	for i := range ph {
		ph[i] = &offMainPhase{StartS: bounds[i][0], EndS: bounds[i][1]}
		if math.IsInf(ph[i].EndS, 1) {
			ph[i].EndS = -1 // JSON has no +Inf: open-ended
		}
	}
	intervals := func(ts []float64, add func(p *offMainPhase, ms float64)) {
		for i := 1; i < len(ts); i++ {
			add(ph[phaseOf(ts[i])], (ts[i]-ts[i-1])*1000)
		}
	}
	intervals(g.RawPresentedS, func(p *offMainPhase, ms float64) { p.PresentIntervalsMs = append(p.PresentIntervalsMs, ms) })
	intervals(g.RawCommitS, func(p *offMainPhase, ms float64) { p.CommitIntervalsMs = append(p.CommitIntervalsMs, ms) })
	intervals(g.RawProbeS, func(p *offMainPhase, ms float64) { p.ProbeGapsMs = append(p.ProbeGapsMs, ms) })
	for _, t := range g.RawDroppedS {
		ph[phaseOf(t)].PresentDropped++
	}
	var blocks [][2]float64
	for i := 1; i < len(g.RawProbeS); i++ {
		if (g.RawProbeS[i]-g.RawProbeS[i-1])*1000 > offMainBlockMs {
			b := [2]float64{g.RawProbeS[i-1], g.RawProbeS[i]}
			blocks = append(blocks, b)
			q := ph[phaseOf(b[1])]
			q.MainBlocks = append(q.MainBlocks, b)
		}
	}
	behindIn := make([][]int, len(ph))
	behindOut := make([][]int, len(ph))
	behindCross := make([][]int, len(ph))
	for _, c := range g.RawCaptures {
		k := phaseOf(c.AtS)
		q := ph[k]
		usable := c.Err == "" && c.Shown >= 0 && c.BehindDone >= 0 && c.DoneS >= c.AtS
		if usable {
			q.CaptureUsable++
			j := sort.Search(len(blocks), func(i int) bool { return blocks[i][1] >= c.AtS })
			switch {
			case j == len(blocks) || blocks[j][0] > c.DoneS:
				behindOut[k] = append(behindOut[k], c.BehindDone)
			case blocks[j][0] <= c.AtS && c.DoneS <= blocks[j][1]:
				// The entire acquisition must be inside the gap and the phase.
				if ph[k].EndS < 0 || c.DoneS < ph[k].EndS {
					behindIn[k] = append(behindIn[k], c.BehindDone)
				} else {
					behindCross[k] = append(behindCross[k], c.BehindDone)
				}
			default:
				behindCross[k] = append(behindCross[k], c.BehindDone)
			}
		}
		q.Captures++
		switch {
		case !usable:
			q.CaptureFailed++
		case c.MatchReq:
			q.CaptureMatch++
		default:
			q.CaptureMismatch++
		}
	}
	frames := make([][4][]float64, len(ph))
	for _, f := range g.RawFrames {
		k := phaseOf(f.AtS)
		for j, v := range [4]float64{f.NextDrawableMs, f.ScheduleMs, f.CACommitMs, f.WakeLateMs} {
			frames[k][j] = append(frames[k][j], v)
		}
	}
	out := map[string]*offMainPhase{}
	for i, p := range ph {
		p.Present = offMainSummarize(p.PresentIntervalsMs)
		p.Commit = offMainSummarize(p.CommitIntervalsMs)
		p.CaptureInBlock = offMainSummarizeBehind(behindIn[i])
		p.CaptureOutBlock = offMainSummarizeBehind(behindOut[i])
		p.CaptureCrossBlock = offMainSummarizeBehind(behindCross[i])
		p.NextDrawable = offMainSummarize(frames[i][0])
		p.Schedule = offMainSummarize(frames[i][1])
		p.CACommit = offMainSummarize(frames[i][2])
		p.WakeLate = offMainSummarize(frames[i][3])
		p.ProbeTicks = len(p.ProbeGapsMs)
		for _, gap := range p.ProbeGapsMs {
			if gap > offMainBlockMs {
				p.ProbeBlocks++
			}
			p.ProbeMaxGapMs = math.Max(p.ProbeMaxGapMs, gap)
		}
		out[names[i]] = p
	}
	return out
}

// offMainSummarize gives nearest-rank percentiles of intervals in ms.
func offMainSummarize(ms []float64) offMainStats {
	if len(ms) == 0 {
		return offMainStats{}
	}
	s := append([]float64{}, ms...)
	sort.Float64s(s)
	rank := func(q float64) float64 {
		i := int(math.Ceil(q*float64(len(s)))) - 1
		return s[max(0, min(i, len(s)-1))]
	}
	over := 0
	for _, v := range s {
		if v > offMainBlockMs {
			over++
		}
	}
	return offMainStats{
		Count: len(s), P50Ms: rank(0.50), P99Ms: rank(0.99), MaxMs: s[len(s)-1],
		Over20Frac: float64(over) / float64(len(s)),
	}
}

// ---- contents ground truth: window captures ----

var offMainCG struct {
	preflight  func() bool
	listImage  func(bounds nsRect, list, window, opts uint32) uintptr // CGWindowListCreateImage, nil if missing
	bitmap     func(data unsafe.Pointer, w, h, bpc, bpr uintptr, space uintptr, info uint32) uintptr
	drawImage  func(ctx uintptr, r nsRect, img uintptr)
	release    func(ctx uintptr) // CGContextRelease
	retainImg  func(img uintptr) uintptr
	width      func(img uintptr) uintptr
	height     func(img uintptr) uintptr
	pngs       int // captures saved as PNG (capture thread only)
	sckWindow  id  // SCContentFilter for this window (ScreenCaptureKit), 0 if not set up
	sckConfig  id
	sckSetupOK bool
}

// captureLoop samples the window every offMainCaptureEvery on its own
// thread until captureStop. It never fails the probe: every problem is
// recorded in capture_status or the sample's err.
func (p *offMainState) captureLoop(window uint32) {
	defer close(p.captureDone)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status := p.captureSetup(window)
	p.mu.Lock()
	p.captureStatus = status
	p.mu.Unlock()
	offMainStage("capture: %s", status)
	if !strings.HasPrefix(status, "ok") {
		<-p.captureStop
		return
	}
	// Every 250 ms, every 50 ms once the switching phase has started (so
	// captures land inside main-thread blocks), on fixed deadlines.
	next := time.Now()
	for {
		p.mu.Lock()
		sw := p.marks.SwitchingS
		p.mu.Unlock()
		every := offMainCaptureEvery
		if sw > 0 && p.now() >= sw && offMainSwitching > 0 {
			every = offMainCaptureFast
		}
		next = next.Add(every)
		if d := time.Until(next); d > 0 {
			select {
			case <-p.captureStop:
				return
			case <-time.After(d):
			}
		} else {
			next = time.Now()
			select {
			case <-p.captureStop:
				return
			default:
			}
		}
		pool := arPoolPush()
		c := p.captureOnce(window)
		arPoolPop(pool)
		p.mu.Lock()
		p.captures = append(p.captures, c)
		p.mu.Unlock()
	}
}

// captureSetup binds CoreGraphics and checks Screen Recording permission
// without prompting.
func (p *offMainState) captureSetup(window uint32) string {
	if os.Getenv(offMainCaptureEnv) == "0" {
		return "capture unavailable: disabled by " + offMainCaptureEnv + "=0"
	}
	libCG, err := purego.Dlopen(frameworksDir+"CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return "capture unavailable: " + err.Error()
	}
	bind := func(fn any, name string) bool {
		addr, err := purego.Dlsym(libCG, name)
		if err != nil || addr == 0 {
			return false
		}
		purego.RegisterFunc(fn, addr)
		return true
	}
	cg := &offMainCG
	if !bind(&cg.preflight, "CGPreflightScreenCaptureAccess") || !bind(&cg.bitmap, "CGBitmapContextCreate") ||
		!bind(&cg.drawImage, "CGContextDrawImage") || !bind(&cg.release, "CGContextRelease") ||
		!bind(&cg.retainImg, "CGImageRetain") || !bind(&cg.width, "CGImageGetWidth") || !bind(&cg.height, "CGImageGetHeight") {
		return "capture unavailable: CoreGraphics symbols missing"
	}
	if !cg.preflight() {
		return "capture unavailable: no Screen Recording permission (CGPreflightScreenCaptureAccess false; not prompted)"
	}
	if !bind(&cg.listImage, "CGWindowListCreateImage") {
		cg.listImage = nil
	}
	// One method for both groups: CGWindowListCreateImage whenever the
	// symbol exists (the window may not be composited yet right after it is
	// shown, so the first image is retried for up to 2 s), ScreenCaptureKit
	// only when it does not. The parent fails if the groups still differ.
	if cg.listImage != nil {
		for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			if img := p.listCapture(window); img != 0 {
				cgImageRelease(img)
				return "ok: CGWindowListCreateImage"
			}
			if time.Now().After(deadline) {
				return "capture unavailable: CGWindowListCreateImage returned NULL for 2 s"
			}
		}
	}
	if err := p.sckSetup(window); err != nil {
		return "capture unavailable: no CGWindowListCreateImage; ScreenCaptureKit: " + err.Error()
	}
	return "ok: ScreenCaptureKit SCScreenshotManager"
}

func (p *offMainState) listCapture(window uint32) uintptr {
	const (
		kCGWindowListOptionIncludingWindow = 1 << 3
		kCGWindowImageBoundsIgnoreFraming  = 1 << 0
		kCGWindowImageNominalResolution    = 1 << 4
	)
	null := nsRect{Origin: nsPoint{math.Inf(1), math.Inf(1)}} // CGRectNull: the window's own bounds
	return offMainCG.listImage(null, kCGWindowListOptionIncludingWindow, window,
		kCGWindowImageBoundsIgnoreFraming|kCGWindowImageNominalResolution)
}

// sckSetup finds this window in SCShareableContent and builds a filter and
// a small configuration for SCScreenshotManager (macOS 14+).
func (p *offMainState) sckSetup(window uint32) error {
	purego.Dlopen(frameworksDir+"ScreenCaptureKit.framework/ScreenCaptureKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if !hasClass("SCShareableContent") || !hasClass("SCScreenshotManager") {
		return fmt.Errorf("SCScreenshotManager unavailable")
	}
	got := make(chan id, 1)
	blk := objc.NewBlock(func(_ objc.Block, content, nsErr id) {
		got <- retain(content) // 0 on error
	})
	send(class("SCShareableContent"), "getShareableContentExcludingDesktopWindows:onScreenWindowsOnly:completionHandler:",
		1, 1, uintptr(blk))
	blk.Release()
	var content id
	select {
	case content = <-got:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("getShareableContent timed out")
	}
	if content == 0 {
		return fmt.Errorf("getShareableContent returned an error")
	}
	defer release(content)
	windows := send(content, "windows")
	var win id
	for i := 0; i < sendInt(windows, "count"); i++ {
		w := send(windows, "objectAtIndex:", uintptr(i))
		if uint32(sendInt(w, "windowID")) == window {
			win = w
			break
		}
	}
	if win == 0 {
		return fmt.Errorf("window %d not in SCShareableContent", window)
	}
	filter := send(send(class("SCContentFilter"), "alloc"), "initWithDesktopIndependentWindow:", uintptr(win))
	cfg := send(class("SCStreamConfiguration"), "new")
	send(cfg, "setWidth:", uintptr(math.Max(p.winW, 64))) // points: same geometry as a nominal-resolution CGWindowList image
	send(cfg, "setHeight:", uintptr(math.Max(p.winH, 48)))
	send(cfg, "setShowsCursor:", 0)
	offMainCG.sckWindow, offMainCG.sckConfig, offMainCG.sckSetupOK = filter, cfg, filter != 0 && cfg != 0
	if !offMainCG.sckSetupOK {
		return fmt.Errorf("cannot build SCContentFilter")
	}
	return nil
}

func (p *offMainState) sckCapture() (uintptr, error) {
	got := make(chan uintptr, 1)
	blk := objc.NewBlock(func(_ objc.Block, img uintptr, nsErr id) {
		if img != 0 {
			img = offMainCG.retainImg(img)
		}
		got <- img
	})
	send(class("SCScreenshotManager"), "captureImageWithFilter:configuration:completionHandler:",
		uintptr(offMainCG.sckWindow), uintptr(offMainCG.sckConfig), uintptr(blk))
	blk.Release()
	select {
	case img := <-got:
		if img == 0 {
			return 0, fmt.Errorf("SCScreenshotManager returned no image")
		}
		return img, nil
	case <-time.After(time.Second):
		return 0, fmt.Errorf("SCScreenshotManager timed out")
	}
}

// captureOnce takes one capture, samples the centre pixel (the image drawn
// into an 8×8 sRGB bitmap) and compares it with the last committed frame.
func (p *offMainState) captureOnce(window uint32) offMainCapture {
	c := offMainCapture{AtS: p.now(), LatestReq: int(p.lastCommitted.Load()), Shown: -1, BehindReq: -1, BehindDone: -1}
	var img uintptr
	if offMainCG.sckSetupOK {
		c.Method = "ScreenCaptureKit"
		var err error
		if img, err = p.sckCapture(); err != nil {
			c.Err = err.Error()
		}
	} else {
		c.Method = "CGWindowListCreateImage"
		if img = p.listCapture(window); img == 0 {
			c.Err = "CGWindowListCreateImage returned NULL"
		}
	}
	c.DoneS = p.now()
	c.LatestDone = int(p.lastCommitted.Load())
	if img == 0 {
		return c
	}
	defer cgImageRelease(img)
	return p.sample(c, img)
}

// offMainPalette holds the contents images' colours (sRGB): the first two
// are the brief's alternating pair; TANGO_OFFMAIN_IMAGES=3…6 adds more so
// a capture tells exactly how many commits behind the screen is.
var offMainPalette = [][3]byte{
	{0x30, 0x60, 0xc0}, // blue
	{0xc0, 0x60, 0x30}, // orange
	{0x30, 0xc0, 0x60}, // green
	{0xc0, 0x30, 0xc0}, // magenta
	{0xc0, 0xc0, 0x30}, // yellow
	{0x30, 0xc0, 0xc0}, // cyan
}

func offMainImageCount() int {
	var n int
	if _, err := fmt.Sscan(os.Getenv(offMainImagesEnv), &n); err != nil || n < 2 {
		return 2
	}
	return min(n, len(offMainPalette))
}

// sample draws the capture into a bitmap of its own size, reads the
// content-area centre and the Metal view's centre, classifies the content
// colour against the palette, and saves the capture as a PNG (the first
// offMainCapturePNGs per group) next to the result.
func (p *offMainState) sample(c offMainCapture, img uintptr) offMainCapture {
	w, h := int(offMainCG.width(img)), int(offMainCG.height(img))
	c.Size = [2]int{w, h}
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		c.Err = fmt.Sprintf("capture size %dx%d", w, h)
		return c
	}
	buf := make([]byte, w*h*4)
	ctx := offMainCG.bitmap(unsafe.Pointer(&buf[0]), uintptr(w), uintptr(h), 8, uintptr(w*4), srgbSpace,
		kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big)
	if ctx == 0 {
		c.Err = "CGBitmapContextCreate failed"
		return c
	}
	offMainCG.drawImage(ctx, nsRect{Size: nsSize{float64(w), float64(h)}}, img)
	offMainCG.release(ctx)
	// Geometry in capture pixels: the window is winW×winH points, the
	// content (offMainWidth×offMainHeight) sits below the title bar; row 0
	// of the bitmap is the top of the image.
	scale := float64(w) / math.Max(p.winW, 1)
	top := float64(h) - offMainHeight*scale // title bar height
	px := func(x, y float64) [3]int {
		xi := min(max(int(x), 0), w-1)
		yi := min(max(int(y), 0), h-1)
		k := (yi*w + xi) * 4
		return [3]int{int(buf[k]), int(buf[k+1]), int(buf[k+2])}
	}
	c.RGB = px(offMainWidth/2*scale, top+offMainHeight/2*scale)
	c.MetalRGB = px(offMainMetalSide/2*scale, top+offMainMetalSide/2*scale)
	n := len(p.images)
	if c.LatestReq >= 0 {
		e := offMainPalette[c.LatestReq%n]
		c.Expected = [3]int{int(e[0]), int(e[1]), int(e[2])}
	}
	best, bestD := -1, 1<<30
	for i, q := range offMainPalette[:n] {
		d := 0
		for j := 0; j < 3; j++ {
			d += offMainAbs(c.RGB[j] - int(q[j]))
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	if bestD <= 90 { // colour management moves each channel by a few levels
		c.Shown = best
		if c.LatestReq >= 0 {
			c.BehindReq = ((c.LatestReq-best)%n + n) % n
		}
		if c.LatestDone >= 0 {
			c.BehindDone = ((c.LatestDone-best)%n + n) % n
		}
		c.MatchReq = c.BehindReq == 0
		c.MatchDone = c.BehindDone == 0
	}
	if offMainCG.pngs < offMainCapturePNGs && p.outPath != "" {
		offMainCG.pngs++
		name := fmt.Sprintf("capture-%s-%03d-latest%d-expect%02x%02x%02x.png", p.group, offMainCG.pngs, c.LatestReq,
			c.Expected[0], c.Expected[1], c.Expected[2])
		path := filepath.Join(filepath.Dir(p.outPath), name)
		im := &image.RGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
		if f, err := os.Create(path); err != nil {
			c.Err = "png: " + err.Error()
		} else {
			if err := png.Encode(f, im); err != nil {
				c.Err = "png: " + err.Error()
			}
			f.Close()
			c.PNG = name
		}
	}
	return c
}

func offMainAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// diagnose logs the window, view and layer state once at the start of the
// quiet phase (main thread): where the window is, whether it is visible,
// and every layer under the content view with its geometry and contents,
// plus the Metal layer's configuration.
func (p *offMainState) diagnose() {
	var lines []string
	add := func(format string, args ...any) {
		l := fmt.Sprintf(format, args...)
		lines = append(lines, l)
		offMainStage("diag: %s", l)
	}
	name := func(o id) string {
		if o == 0 {
			return "nil"
		}
		return goString(send(o, "className"))
	}
	rect := func(r nsRect) string {
		return fmt.Sprintf("(%g,%g %gx%g)", r.Origin.X, r.Origin.Y, r.Size.Width, r.Size.Height)
	}
	withPool(func() {
		win := p.c.s.win
		add("window %s frame=%s visible=%v onActiveSpace=%v occlusionState=%#x key=%v main=%v level=%d windowNumber=%d appActive=%v",
			name(win), rect(msgRect(win, sel("frame"))), sendBool(win, "isVisible"), sendBool(win, "isOnActiveSpace"),
			sendInt(win, "occlusionState"), sendBool(win, "isKeyWindow"), sendBool(win, "isMainWindow"),
			sendInt(win, "level"), sendInt(win, "windowNumber"), sendBool(p.b.app, "isActive"))
		if scr := send(win, "screen"); scr != 0 {
			sn := ""
			if respondsTo(scr, "localizedName") {
				sn = goString(send(scr, "localizedName"))
			}
			add("screen %q frame=%s backingScale=%g", sn, rect(msgRect(scr, sel("frame"))), msgGetFloat(scr, sel("backingScaleFactor")))
		} else {
			add("screen nil (window off every screen)")
		}
		content := send(win, "contentView")
		var views func(v id, depth int)
		views = func(v id, depth int) {
			add("%*sview %s frame=%s hidden=%v wantsLayer=%v layer=%s flipped=%v", 2*depth, "", name(v), rect(msgRect(v, sel("frame"))),
				sendBool(v, "isHidden"), sendBool(v, "wantsLayer"), name(send(v, "layer")), sendBool(v, "isFlipped"))
			subs := send(v, "subviews")
			for i := 0; i < sendInt(subs, "count"); i++ {
				views(send(subs, "objectAtIndex:", uintptr(i)), depth+1)
			}
		}
		views(content, 0)
		var layers func(l id, depth int)
		layers = func(l id, depth int) {
			add("%*slayer %s frame=%s bounds=%s hidden=%v opacity=%g zPosition=%g contents=%v contentsScale=%g opaque=%v masksToBounds=%v",
				2*depth, "", name(l), rect(msgRect(l, sel("frame"))), rect(msgRect(l, sel("bounds"))), sendBool(l, "isHidden"),
				offMainGetF32(l, sel("opacity")), msgGetFloat(l, sel("zPosition")), send(l, "contents") != 0,
				msgGetFloat(l, sel("contentsScale")), sendBool(l, "isOpaque"), sendBool(l, "masksToBounds"))
			subs := send(l, "sublayers")
			for i := 0; i < sendInt(subs, "count"); i++ {
				layers(send(subs, "objectAtIndex:", uintptr(i)), depth+1)
			}
		}
		root := send(content, "layer")
		for root != 0 && send(root, "superlayer") != 0 {
			root = send(root, "superlayer")
		}
		if root == 0 {
			add("content view has no layer")
		} else {
			layers(root, 0)
		}
		m := p.metal
		ds := offMainGetSize(m, sel("drawableSize"))
		add("metal %s superlayer=%s view=%s drawableSize=%gx%g contentsScale=%g pixelFormat=%d framebufferOnly=%v presentsWithTransaction=%v maximumDrawableCount=%d device=%q",
			name(m), name(send(m, "superlayer")), name(p.metalView), ds.Width, ds.Height, msgGetFloat(m, sel("contentsScale")),
			sendInt(m, "pixelFormat"), sendBool(m, "framebufferOnly"), sendBool(m, "presentsWithTransaction"),
			sendInt(m, "maximumDrawableCount"), goString(send(p.device, "name")))
	})
	p.mu.Lock()
	p.diag = append(p.diag, lines...)
	p.mu.Unlock()
}

func TestOffMainProbeCaptureWindows(t *testing.T) {
	g := offMainGroup{
		Marks:     offMainMarks{QuietS: 0.5, SwitchingS: 1, EndS: 2},
		RawProbeS: []float64{1.0, 1.1, 1.104, 1.2},
	}
	for i, span := range [][2]float64{
		{1.01, 1.05},   // fully inside the first gap
		{1.08, 1.102},  // exits a gap
		{1.102, 1.15},  // enters a gap
		{1.095, 1.15},  // crosses two gaps
		{1.101, 1.103}, // fully outside
	} {
		g.RawCaptures = append(g.RawCaptures, offMainCapture{AtS: span[0], DoneS: span[1], BehindDone: i})
	}
	q := offMainPhases(&g)["switching"]
	if q.CaptureInBlock.Count != 1 || q.CaptureInBlock.Histogram[0] != 1 || q.CaptureOutBlock.Count != 1 || q.CaptureOutBlock.Histogram[4] != 1 {
		t.Fatalf("cross-boundary captures must not count as inside or outside: in=%+v out=%+v", q.CaptureInBlock, q.CaptureOutBlock)
	}
	if q.CaptureCrossBlock.Count != 3 || q.CaptureCrossBlock.Histogram[1] != 1 || q.CaptureCrossBlock.Histogram[2] != 1 || q.CaptureCrossBlock.Histogram[3] != 1 {
		t.Fatalf("cross-boundary summary = %+v, want three crossings", q.CaptureCrossBlock)
	}
}

func TestOffMainProbeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, a, b    string
		short         bool
		quiet, inside int
		want          string
	}{
		{"short ignores Metal", "ok: CG", "ok: CG", true, 1, 0, ""},
		{"both unavailable", "capture unavailable", "capture unavailable", true, 1, 0, "capture unavailable"},
		{"different methods", "ok: CG", "ok: SCK", true, 1, 0, "CAPTURE METHOD MISMATCH"},
		{"missing quiet evidence", "ok: CG", "ok: CG", true, 0, 0, "no usable quiet captures"},
		{"full needs block evidence", "ok: CG", "ok: CG", false, 1, 0, "no usable switching capture"},
		{"full with block evidence", "ok: CG", "ok: CG", false, 1, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group := func(status string) *offMainGroup {
				return &offMainGroup{CaptureStatus: status, Phases: map[string]*offMainPhase{
					"quiet":     {CaptureUsable: tc.quiet},
					"switching": {CaptureInBlock: offMainBehind{Count: tc.inside}},
				}}
			}
			got := offMainEvidenceIssue(map[string]*offMainGroup{"offmain": group(tc.a), "mainthread": group(tc.b)}, tc.short)
			if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
				t.Fatalf("evidence issue = %q, want %q", got, tc.want)
			}
		})
	}
}
