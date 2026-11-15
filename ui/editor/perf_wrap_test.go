//go:build darwin

package editor

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
)

// longWrapLine returns a single line of about n bytes: the generated
// TypeScript of genTS joined by spaces, with a tab and a CJK phrase every
// few statements (a minified bundle with comments in Chinese).
func longWrapLine(n int) string {
	var sb strings.Builder
	sb.Grow(n + 256)
	for i := 0; sb.Len() < n; i++ {
		if i%3 == 0 {
			sb.WriteString("\t/* 生成的注释，包含中文与全角标点。 */ ")
		}
		fmt.Fprintf(&sb, perfTemplates[i%len(perfTemplates)], i)
		sb.WriteByte(' ')
	}
	return sb.String()
}

// BenchmarkTypeLongLineWrap (Task 5 of the M2 acceptance plan): word wrap
// on, a single model line of 1 MB, typing "x" in the middle of it; every
// keystroke runs to the GPU finishing the frame. p95 ≤ 16 ms.
//
//	go test ./ui/editor -run '^$' -bench 'TypeLongLineWrap' -benchtime 300x
func BenchmarkTypeLongLineWrap(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	text := longWrapLine(1 << 20)
	st := NewState(text, "bundle.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	h := newPerfHost(b, st, 2)
	h.frame(b)
	if !st.wrapping() {
		b.Fatal("not wrapping")
	}
	h.focus(b)
	st.MoveCaret(len(text)/2, false)
	st.RevealCaret()
	for range 5 {
		h.frame(b)
	}
	if st.Snapshot().LineCount() != 1 {
		b.Fatal("not a single line")
	}
	n := st.Snapshot().Len()
	var d durations
	h.resetSplit()
	for b.Loop() {
		start := time.Now()
		h.typeKey("x")
		h.frame(b)
		d = append(d, time.Since(start))
	}
	if got := st.Snapshot().Len(); got != n+len(d) {
		b.Fatalf("length %d after %d keys, want %d", got, len(d), n+len(d))
	}
	b.ReportMetric(ms(d.pct(0.5)), "p50-ms")
	b.ReportMetric(ms(d.pct(0.95)), "p95-ms")
	b.ReportMetric(ms(d.pct(1)), "max-ms")
	h.reportSplit(b)
}
