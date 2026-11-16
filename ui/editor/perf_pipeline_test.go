//go:build darwin

package editor

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func partialTS20k(tb testing.TB) *perfHost {
	tb.Helper()
	metalDevice(tb)
	tb.Logf("load average: %s, GOMAXPROCS %d, TANGO_EDITOR_PERF_OFF=%q", loadAvg(), runtime.GOMAXPROCS(0), os.Getenv("TANGO_EDITOR_PERF_OFF"))
	path := perfFile(tb, 20000)
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	st := NewState(string(data), path, theme.Base("vs"))
	tb.Cleanup(st.Close)
	h := newPartialPerfHost(tb, st, 2)
	tb.Cleanup(h.r.Close)
	h.frame(tb)
	highlightAllBackground(tb, st)
	h.frame(tb)
	return h
}

func BenchmarkTypeTS20kPartial(b *testing.B) {
	h := partialTS20k(b)
	h.focus(b)
	h.caretTo(b, 10000)
	f0, p0 := h.dev.frames, h.dev.partial
	typeBench(b, h)
	if n := h.dev.frames - f0; n > 0 {
		b.ReportMetric(float64(h.dev.partial-p0)/float64(n), "partial")
	}
}

func BenchmarkScrollTS20kPartial(b *testing.B) {
	h := partialTS20k(b)
	h.caretTo(b, 2000)
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * h.st.Viewport.LineHeight
	top := h.st.Viewport.TopLine
	var d durations
	f0, p0 := h.dev.frames, h.dev.partial
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.frame(b)
		d = append(d, time.Since(start))
	}
	if h.st.Viewport.TopLine <= top {
		b.Fatal("did not scroll")
	}
	b.ReportMetric(ms(d.pct(.99)), "p99-ms")
	h.reportSplit(b)
	if n := h.dev.frames - f0; n > 0 {
		b.ReportMetric(float64(h.dev.partial-p0)/float64(n), "partial")
	}
}

// Opt-in observation, no timing/working-set threshold on shared machines.
// Undo history remains enabled, so editing is expected to retain history.
func TestEditorSustainedProfile(t *testing.T) {
	if os.Getenv("TANGO_EDITOR_SUSTAINED") == "" {
		t.Skip("set TANGO_EDITOR_SUSTAINED=1 for sustained memory observations")
	}
	h := partialTS20k(t)
	h.focus(t)
	h.caretTo(t, 10000)
	initial := h.st.Snapshot()
	insertAt := h.st.Sels[0].Head
	assertFeatures := func() {
		if h.st.LargeFile() || !h.st.Options.Minimap.Enabled || !h.st.Options.Minimap.RenderCharacters || !h.st.Options.Guides.Indentation || h.st.Highlighter() == nil || h.st.Highlighter().LexerIdentity() == nil {
			t.Fatal("sustained workload lost enabled editor display features")
		}
		coloredBitmap := false
		for _, ln := range h.vw.vs.mini.cache {
			for _, tk := range ln.tokens {
				coloredBitmap = coloredBitmap || ln.bmp != nil && tk.Type != ""
			}
		}
		if !coloredBitmap || h.vw.vs.mini.stripsComposed == 0 {
			t.Fatal("sustained workload has no highlighted minimap bitmap")
		}
	}
	observe := func(label string) {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf("%s: heap=%.2f MiB objects=%d", label, float64(m.HeapAlloc)/(1<<20), m.HeapObjects)
	}
	observe("warm")
	assertFeatures()
	key := 0
	for phase := 0; phase < 3; phase++ {
		for range 400 {
			h.typeKey(keyAt(key))
			h.frame(t)
			key++
		}
		snap := h.st.Snapshot()
		wantText := strings.Repeat(strings.Repeat("x", 39)+"\n", key/40)
		if snap.Len() != initial.Len()+key || snap.LineCount() != initial.LineCount()+key/40 || snap.Slice(insertAt, insertAt+key) != wantText {
			t.Fatalf("sustained input did not accept all %d keys", key)
		}
		assertFeatures()
		observe("after 400 more keys")
	}
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * h.st.Viewport.LineHeight
	for phase := 0; phase < 3; phase++ {
		for k := 0; k < 600; k++ {
			before := h.st.Viewport
			delta := dy
			if k%200 >= 100 {
				delta = -dy
			}
			h.r.HandleEvent(input.Scroll{Pos: p, DY: delta})
			h.frame(t)
			if h.st.Viewport.TopLine == before.TopLine && h.st.Viewport.OffsetY == before.OffsetY {
				t.Fatalf("sustained scroll did not move at phase %d event %d", phase, k)
			}
		}
		assertFeatures()
		observe("after 600 more scrolls")
	}
	runtime.KeepAlive(h)
}
