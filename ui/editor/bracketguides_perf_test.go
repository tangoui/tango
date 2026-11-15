//go:build darwin

package editor

import (
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
)

// BenchmarkTypeJSONBracketGuides types near the top of a 4.4 MB JSON
// document (125 000 lines) whose two outer objects span all of it: per
// keystroke and frame. Bracket pair guides follow
// TANGO_EDITOR_PERF_ON=bracketguides (perf_ab_test.go; off by default), so
// ui/editor/perfab compares the two in paired runs. The guide columns of
// the two long pairs are what the indentation index keeps from reading
// the whole document on every key.
func BenchmarkTypeJSONBracketGuides(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	text := jsonGuideDoc(125_000)
	b.Logf("file: %.1f MB, 125000 lines", float64(len(text))/1e6)
	st := NewState(text, "gen.json", theme.Base("vs"))
	h := newPerfHost(b, st, 2)
	on := st.Options.Guides.BracketPairs != BracketGuidesOff
	b.Logf("bracket pair guides: %v", on)
	h.frame(b)
	if st.Highlighter() != nil { // JSON has no Monarch grammar: plain text
		highlightAllBackground(b, st)
	}
	h.focus(b)
	h.caretTo(b, 5)
	st.MoveCaret(st.Snapshot().LineStart(5)+6, false) // after the indentation
	for range 5 {
		h.frame(b)
	}
	var d durations
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.typeKey("x")
		h.frame(b)
		d = append(d, time.Since(start))
	}
	if on && !slices.ContainsFunc(h.vw.vs.bgRows, func(r bgRow) bool {
		return r.line == 6 && slices.ContainsFunc(r.g, func(g bracketGuide) bool { return g.kind == bgVertical && g.col == 2 })
	}) {
		b.Fatalf("the guide of \"packages\" is not drawn: %v", h.vw.vs.bgRows)
	}
	b.ReportMetric(ms(d.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(d.pct(0.95)), "p95-ms")
	h.reportSplit(b)
}
