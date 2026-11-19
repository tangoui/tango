//go:build !darwin || ios || tango_gotext

package text

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-text/typesetting/shaping"
)

func TestReflowReusesShapingAtDifferentWidths(t *testing.T) {
	clearCache()
	text := strings.Repeat("office window layout vector editor ", 8)
	st := Style{Size: 13}
	calls := 0
	shapeHook = func(shaping.Input) { calls++ }
	defer func() { shapeHook = nil }()
	Shape(text, st, Options{MaxWidth: 450})
	first := calls
	if first == 0 {
		t.Skip("no font installed")
	}
	for width := 451; width < 460; width++ {
		Shape(text, st, Options{MaxWidth: float32(width)})
	}
	if calls != first {
		t.Fatalf("width-only reflow reshaped text: calls %d -> %d", first, calls)
	}
}

func TestReflowMatchesColdLayout(t *testing.T) {
	cases := []string{
		strings.Repeat("office AV window layout vector editor ", 4),
		strings.Repeat("中文排版测试，测试换行。office ", 4),
		"first paragraph office\nsecond paragraph AV\n\nthird paragraph",
		"العربية office السلام عليكم العربية office السلام عليكم",
	}
	for _, s := range cases {
		clearCache()
		st := Style{Size: 13}
		Shape(s, st, Options{MaxWidth: 450})
		for _, opt := range []Options{{MaxWidth: 180}, {MaxWidth: 220, MaxLines: 2, Ellipsis: true}, {MaxWidth: 90, Direction: DirRTL}, {NoWrap: true}, {MaxWidth: 400}} {
			warm := Shape(s, st, opt)
			clearCache()
			cold := Shape(s, st, opt)
			if !reflect.DeepEqual(warm, cold) {
				t.Fatalf("reflow differs for %q opt %+v", s, opt)
			}
		}
	}
}

func TestShapeCacheHasByteBound(t *testing.T) {
	UseSystemFonts(true)
	clearCache()
	s := strings.Repeat("window layout text ", 120)
	for i := 0; i < 400; i++ {
		Shape(s, Style{Size: 13}, Options{MaxWidth: float32(100 + i)})
	}
	mu.Lock()
	var bytes int
	for _, l := range cache {
		bytes += len(l.Glyphs)*48 + len(l.Runes)*4
		for _, ln := range l.Lines {
			bytes += len(ln.carets) * 4
		}
	}
	mu.Unlock()
	if bytes > 16<<20 {
		t.Fatalf("unbounded width-keyed layouts retain %d MiB", bytes>>20)
	}
}

func BenchmarkWidthOnlyReflow(b *testing.B) {
	s := strings.Repeat("office window layout vector editor ", 14)
	clearCache()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Shape(s, Style{Size: 13}, Options{MaxWidth: float32(100+i%10000) / 10})
	}
}

func TestNoWrapWidthDoesNotDuplicateShaping(t *testing.T) {
	clearCache()
	st := Style{Size: 11}
	a := Shape("123.45", st, Options{NoWrap: true})
	b := Shape("123.45", st, Options{NoWrap: true, MaxWidth: 40})
	if a != b {
		t.Fatal("non-wrapping label has duplicate width-keyed layouts")
	}
	ell := Shape("123.45", st, Options{NoWrap: true, MaxWidth: 10, Ellipsis: true})
	if !ell.Truncated || ell == a {
		t.Fatal("ellipsis must still honor width")
	}
}

func BenchmarkShapeChangingNumericLabel(b *testing.B) {
	st := Style{Size: 11}
	opt := Options{NoWrap: true}
	n := 0
	b.ReportAllocs()
	for b.Loop() {
		n++
		Shape(fmt.Sprintf("%.2f", float64(n%100000)/100), st, opt)
	}
}

func TestWidthsWithSameBreaksReusePlacement(t *testing.T) {
	clearCache()
	s := strings.Repeat("office window render ", 6)
	st := Style{Size: 13}
	a := Shape(s, st, Options{MaxWidth: 300})
	b := Shape(s, st, Options{MaxWidth: 300.01})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("test widths must produce identical line breaks")
	}
	if a != b {
		t.Fatal("identical line breaks rebuilt glyph placement")
	}
}

func TestNewlineHeavyPreparationIsNotRetainedOverBudget(t *testing.T) {
	clearCache()
	Shape(strings.Repeat("\n", 100000), Style{}, Options{MaxLines: 1})
	mu.Lock()
	defer mu.Unlock()
	if len(preparedCache) != 0 {
		t.Fatal("newline-only paragraph storage exceeds budget but was cached")
	}
}

func TestShapeCacheHitDoesNotBuildFontChain(t *testing.T) {
	clearCache()
	st := Style{Size: 11}
	opt := Options{NoWrap: true}
	Shape("cached label", st, opt)
	allocations := testing.AllocsPerRun(50, func() { Shape("cached label", st, opt) })
	if allocations != 0 {
		t.Fatalf("cache hit allocated %.0f times", allocations)
	}
}
