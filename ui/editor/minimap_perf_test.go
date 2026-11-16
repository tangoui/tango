package editor

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/text"
)

// minimapPerfView exercises the real view, highlighter and token windows.
// The measured operation below is only the warm miniature preparation, not
// CPU rasterization, font discovery or background tokenization.
func minimapPerfView(tb testing.TB) *ViewW {
	tb.Helper()
	prev := text.UseSystemEngine(false)
	tb.Cleanup(func() { text.UseSystemEngine(prev) })
	var doc strings.Builder
	for i := range 80 {
		fmt.Fprintf(&doc, "var value%d = \"text%d\" // line\n", i, i)
	}
	st := NewState(doc.String(), "mini.go", theme.Base("vs-dark"))
	tb.Cleanup(st.Close)
	hl := st.Highlighter()
	if hl == nil {
		tb.Fatal("fixture has no Go highlighter")
	}
	hl.TokenWindow = 256
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	hl.HighlightFor(st.hlKey, st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	for range 3 {
		(*vw).prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	}
	if len((*vw).vs.mini.cache) < 70 {
		tb.Fatalf("only %d miniature rows cached", len((*vw).vs.mini.cache))
	}
	if toks, ok := hl.PeekTokenRange(st.Snapshot(), 20, 0, 120, nil); !ok || len(toks) < 4 {
		tb.Fatalf("fixture row tokens: %v cached=%v", toks, ok)
	}
	return *vw
}

// Reintroducing a nil destination for each PeekTokenRange allocates once per
// miniature row even when nothing changed. Warm preparation must instead use
// bounded view-owned scratch, not one allocation per sampled row.
func TestMinimapWarmPreparationAllocations(t *testing.T) {
	w := minimapPerfView(t)
	snap, th := w.st.Snapshot(), w.st.Theme
	rebuilt := w.vs.mini.rebuilt
	allocs := testing.AllocsPerRun(20, func() {
		w.prepareMinimap(snap, th, time.Now().Add(time.Second))
	})
	if w.vs.mini.rebuilt != rebuilt {
		t.Fatal("warm preparation rebuilt unchanged rows")
	}
	if allocs > 12 {
		t.Fatalf("warm preparation allocated %.0f times for %d rows; want <=12, independent of cached row count", allocs, len(w.vs.mini.cache))
	}
}

// A shared scratch must never become a cached line's owned tokens. Editing one
// row and preparing the next must leave the old tokens and bitmaps unchanged.
func TestMinimapPreparationDoesNotAliasCachedTokens(t *testing.T) {
	w := minimapPerfView(t)
	ln := w.vs.mini.cache[20]
	oldTokens := slices.Clone(ln.tokens)
	oldPixels := slices.Clone(ln.bmp.pix)
	oldText := ln.text
	off := w.st.Snapshot().LineStart(21)
	if err := w.st.Edit([]buffer.Edit{{Start: off, End: off + 3, Text: "// "}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap := w.st.Snapshot()
	w.st.Highlighter().HighlightFor(w.st.hlKey, snap, snap.LineCount(), time.Second)
	w.prepareMinimap(snap, w.st.Theme, time.Now().Add(time.Second))
	if !slices.Equal(ln.tokens, oldTokens) || !slices.Equal(ln.bmp.pix, oldPixels) || ln.text != oldText {
		t.Fatal("preparing another row mutated a cached line")
	}
	if got := w.vs.mini.cache[21]; !strings.HasPrefix(got.text, "// ") {
		t.Fatalf("edited miniature stayed stale: %q", got.text)
	}
	// Force tokens and bitmaps to rebuild for a new theme; old row ownership
	// still holds, even though the view no longer uses that row object.
	w.st.Theme = theme.Base("vs")
	w.prepareMinimap(snap, w.st.Theme, time.Now().Add(time.Second))
	if !slices.Equal(ln.tokens, oldTokens) || !slices.Equal(ln.bmp.pix, oldPixels) {
		t.Fatal("theme rebuild mutated old cache storage")
	}
}

func BenchmarkMinimapWarmPreparation(b *testing.B) {
	w := minimapPerfView(b)
	snap, th := w.st.Snapshot(), w.st.Theme
	b.ReportAllocs()
	for b.Loop() {
		w.prepareMinimap(snap, th, time.Now().Add(time.Second))
	}
}

var minimapPerfBitmap *miniBitmap

func TestMinimapRasterAllocations(t *testing.T) {
	g := minimapGlyphsFor(2, false, 255)
	input := strings.Repeat("a", 120)
	fg := color.NRGBA{R: 201, G: 127, B: 73, A: 255}
	allocs := testing.AllocsPerRun(30, func() {
		minimapPerfBitmap = miniCharsAt(input, 4, g, func(int) color.NRGBA { return fg })
	})
	if allocs > 2 {
		t.Fatalf("rasterizing a 120-column miniature allocated %.0f times, want only bitmap and pixels (<=2)", allocs)
	}
	if minimapPerfBitmap == nil || minimapPerfBitmap.w != 240 || minimapPerfBitmap.h != 4 {
		t.Fatalf("bitmap geometry = %+v, want 240x4", minimapPerfBitmap)
	}
}

func TestMinimapRasterColorCallbackOrder(t *testing.T) {
	// A, tab, 中, e+combining, emoji, U+0080 (no glyph), invalid UTF-8,
	// trailing space. colorAt must see byte offsets, once per drawn cluster.
	input := "A\t中é😀\u0080\xff "
	var got []int
	bmp := miniCharsAt(input, 4, minimapGlyphsFor(2, false, 255), func(i int) color.NRGBA {
		got = append(got, i)
		return color.NRGBA{R: byte(i + 1), A: 255}
	})
	if want := []int{0, 2, 5, 8, 14}; !slices.Equal(got, want) {
		t.Fatalf("callback offsets = %v, want %v (once, increasing)", got, want)
	}
	if bmp == nil {
		t.Fatal("mixed text produced no bitmap")
	}
}

func BenchmarkMinimapRaster(b *testing.B) {
	g := minimapGlyphsFor(2, false, 255)
	input := strings.Repeat("a", 120)
	fg := color.NRGBA{R: 201, G: 127, B: 73, A: 255}
	b.ReportAllocs()
	for b.Loop() {
		minimapPerfBitmap = miniCharsAt(input, 4, g, func(int) color.NRGBA { return fg })
	}
}
