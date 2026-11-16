package editor

import (
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/text"
)

var allocationBitmap *miniBitmap
var allocationRuns []text.Run
var allocationPaints []runPaint

func TestWarmStyleRunsAllocationBudget(t *testing.T) {
	for _, overrides := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "overrides"}[overrides], func(t *testing.T) {
			w := &ViewW{vs: &viewState{}, opts: DefaultOptions()}
			th := theme.Base("vs-dark")
			toks := []monarch.Token{{ByteStart: 0, Type: "keyword"}, {ByteStart: 5, Type: "string"}}
			var over []brSpan
			if overrides {
				over = []brSpan{{b0: 2, b1: 3, c: color.NRGBA{255, 0, 0, 255}}}
			}
			allocs := testing.AllocsPerRun(30, func() {
				allocationRuns, allocationPaints = w.styleRuns("const text", 0, toks, th, over)
			})
			if allocs > 1 {
				t.Fatalf("warm style runs use %.0f allocations, want <=1 (owned paints)", allocs)
			}
		})
	}
}

func TestStyleRunsPreservePreviousRowPaints(t *testing.T) {
	w := &ViewW{vs: &viewState{}, opts: DefaultOptions()}
	red := color.NRGBA{255, 0, 0, 255}
	blue := color.NRGBA{0, 0, 255, 255}
	_, first := w.styleRuns("abc", 0, nil, theme.Base("vs-dark"), []brSpan{{b0: 0, b1: 3, c: red}})
	want := slices.Clone(first)
	_, second := w.styleRuns("abc", 0, nil, theme.Base("vs"), []brSpan{{b0: 0, b1: 3, c: blue}})
	if len(first) != 1 || first[0].fg != red || len(second) != 1 || second[0].fg != blue || !slices.Equal(first, want) {
		t.Fatalf("row paints were overwritten: first=%v, second=%v", first, second)
	}
}

func TestMinimapGlyphAllocationBudget(t *testing.T) {
	s := strings.Repeat("abcdefghij", 12)
	glyphs := minimapGlyphsFor(2, false, 255)
	foreground := func(int) color.NRGBA { return color.NRGBA{100, 150, 200, 255} }
	allocs := testing.AllocsPerRun(30, func() { allocationBitmap = miniCharsAt(s, 4, glyphs, foreground) })
	if allocs > 3 {
		t.Fatalf("120-column glyph bitmap uses %.0f allocations, want <=3", allocs)
	}
}

func BenchmarkMinimapGlyphBitmap(b *testing.B) {
	s := strings.Repeat("abcdefghij", 12)
	glyphs := minimapGlyphsFor(2, false, 255)
	foreground := func(int) color.NRGBA { return color.NRGBA{100, 150, 200, 255} }
	b.ReportAllocs()
	for b.Loop() {
		allocationBitmap = miniCharsAt(s, 4, glyphs, foreground)
	}
}

func TestWarmMinimapAllocationBudget(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("const value = 1; // comment\n", 100), "a.ts", theme.Base("vs-dark"))
	defer st.Close()
	st.Highlighter().TokenWindow = 400
	st.Highlighter().HighlightFor(st.hlKey, st.Snapshot(), 101, time.Second)
	d, vw := viewDriver(st, 800, 600, nil)
	for range 5 {
		d.Frame()
	}
	w := *vw
	allocs := testing.AllocsPerRun(20, func() { w.prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second)) })
	if allocs > 25 {
		t.Fatalf("warm minimap uses %.0f allocations per frame, want <=25", allocs)
	}
}
