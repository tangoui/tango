//go:build darwin

package editor

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// cssDoc returns n lines of CSS, each ending in '\n': with colors, every
// rule line holds three color literals (hex, rgb(), hsl(); injected gaps,
// injected.go); without, the same shape with named colors (no literal,
// but the "#rh" letters every line is searched for).
func cssDoc(n int, colors bool) string {
	tpl := [...]string{
		".rule%[1]d { color: #%06[2]x; background: rgb(%[3]d, 128, 0); border: 1px solid hsl(%[3]d, 50%%, 50%%); }",
		"  /* rule %[1]d: a comment about the rules around it, with a few more words */",
		".box%[1]d > .item { margin: %[3]dpx 4px; padding: 0 %[3]dpx; outline-color: #%03[4]x; }",
	}
	if !colors {
		tpl = [...]string{
			".rule%[1]d { color: red; background: green; border: 1px solid blue; margin-right: %[3]dpx; z: %[2]d; }",
			"  /* rule %[1]d: a comment about the rules around it, with a few more words */",
			".box%[1]d > .item { margin: %[3]dpx 4px; padding: 0 %[3]dpx; outline-color: white; z: %[4]d; }",
		}
	}
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, tpl[i%len(tpl)], i, (i*2654435761)&0xffffff, i%256, i&0xfff)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestCSSDocColors checks cssDoc: two thirds of the lines of the colored
// document have color literals, none of the plain one.
func TestCSSDocColors(t *testing.T) {
	for _, colors := range []bool{true, false} {
		n := 0
		for _, l := range strings.Split(strings.TrimSuffix(cssDoc(30, colors), "\n"), "\n") {
			n += len(computeColors(l, nil))
		}
		if want := map[bool]int{true: 40, false: 0}[colors]; n != want {
			t.Errorf("colors %v: %d literals, want %d", colors, n, want)
		}
	}
}

// BenchmarkTypeCSS20k types (keyAt) at line 10 000 of a 20 000-line CSS
// file, fully highlighted: "colors" has color literals on most lines (the
// gaps of the color decorators on the drawn rows), "plain" none. With
// TANGO_EDITOR_PERF_OFF=colors (perfab/arms.sh) "colors" measures the
// color decorators' cost and "plain" that the injected layout path costs
// nothing on lines without colors.
func BenchmarkTypeCSS20k(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	for _, colors := range []bool{true, false} {
		name := map[bool]string{true: "colors", false: "plain"}[colors]
		text := cssDoc(20_000, colors)
		b.Run(name, func(b *testing.B) {
			st := NewState(text, "gen.css", theme.Base("vs"))
			h := newPerfHost(b, st, 2)
			h.frame(b)
			highlightAllBackground(b, st)
			h.focus(b)
			h.caretTo(b, 10_000)
			typeBench(b, h)
		})
	}
}

// BenchmarkScrollCSS20k is BenchmarkScroll (scale 2) in the documents of
// BenchmarkTypeCSS20k, from line 2 000.
func BenchmarkScrollCSS20k(b *testing.B) {
	metalDevice(b)
	logLoad(b)
	for _, colors := range []bool{true, false} {
		name := map[bool]string{true: "colors", false: "plain"}[colors]
		text := cssDoc(20_000, colors)
		b.Run(name, func(b *testing.B) {
			st := NewState(text, "gen.css", theme.Base("vs"))
			h := newPerfHost(b, st, 2)
			h.frame(b)
			highlightAllBackground(b, st)
			h.caretTo(b, 2_000)
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
