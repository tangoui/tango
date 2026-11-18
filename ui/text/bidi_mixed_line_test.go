package text

import (
	"reflect"
	"testing"
	"unicode/utf8"
)

// An editor line with an Arabic phrase followed by a number (the CoreText
// regression of the editor demo): "مرحبا بالعالم 123" is one right-to-left
// stretch with the number nested in it, so the space before "123" shows
// between the number and the Arabic, and the space after it stays before
// "and". The space must neither vanish nor move next to "and".
func TestBidiArabicNumberLine(t *testing.T) {
	const s = "// RTL: مرحبا بالعالم 123 and שלום עולם"
	needFont(t, "مرحبا بالعالم")
	needFont(t, "שלום עולם")
	n := utf8.RuneCountInString(s)
	// Levels (UAX #9, left-to-right paragraph): the Arabic and the spaces
	// between it and the number at 1, "123" (AN after AL) at 2, the space
	// before "and" (between AN and L) at 0, the Hebrew at 1.
	var want []int
	for i := 0; i <= 7; i++ {
		want = append(want, i)
	}
	want = append(want, 22, 23, 24)
	for i := 21; i >= 8; i-- {
		want = append(want, i)
	}
	for i := 25; i <= 29; i++ {
		want = append(want, i)
	}
	for i := 38; i >= 30; i-- {
		want = append(want, i)
	}
	mono := Style{Mono: true, Size: 13}
	layouts := map[string]*Layout{
		"Shape":     Shape(s, mono, Options{NoWrap: true, Direction: DirLTR}),
		"ShapeRuns": ShapeRuns(s, []Run{{0, 8, mono}, {8, n, mono}}, Options{NoWrap: true, Direction: DirLTR}),
		"LineCache": NewLineCache(8).Shape(s, []Run{{0, n, mono}}, LineOptions{Direction: DirLTR}),
		"LineCache tokens": NewLineCache(8).Shape(s, []Run{{0, 2, mono}, {2, 8, Style{Mono: true, Size: 13, Weight: Bold}}, {8, n, mono}},
			LineOptions{Direction: DirLTR}),
	}
	for name, l := range layouts {
		if got := visualRunes(l, 0); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: visual order %v\nwant %v", name, got, want)
			continue
		}
		bl := l.Lines[0].bidi
		// Every space has a box of its own width, where the visual order
		// puts it.
		for _, i := range []int{13, 21, 25, 29, 34} {
			if w := bl.xr[i] - bl.xl[i]; w <= 0 {
				t.Errorf("%s: space %d has width %v", name, i, w)
			}
		}
		// Glyphs are placed in their runes' boxes: each glyph's x lies in
		// the box of its cluster (a ligature's glyph in the first rune's
		// box or one it covers), give or take a mark's offset (3 px; a
		// shifted item is off by a whole space, 8 px).
		for g, c := range l.GlyphCluster {
			x := l.Glyphs[g].X
			if c < 0 || int(c) >= n {
				continue
			}
			if x < bl.xl[c]-3 || x > bl.xr[c]+3 && !l.coveredBy(int(c), x) {
				t.Errorf("%s: glyph %d of rune %d (%q) at x %v, box [%v,%v]", name, g, c, []rune(s)[c], x, bl.xl[c], bl.xr[c])
			}
		}
		// The space after the Arabic (rune 21) has a glyph, and it sits
		// between "123" and the Arabic.
		x21 := float32(-1)
		for g, c := range l.GlyphCluster {
			if c == 21 {
				x21 = l.Glyphs[g].X
			}
		}
		if x21 < 0 || x21 < bl.xr[24]-0.5 || x21 > bl.xl[20]+0.5 {
			t.Errorf("%s: space 21 glyph at %v, want between %v and %v", name, x21, bl.xr[24], bl.xl[20])
		}
	}
}

// coveredBy reports whether x lies in the box of a rune of the cluster
// starting at rune c (the runes up to the next glyph's cluster).
func (l *Layout) coveredBy(c int, x float32) bool {
	bl := l.Lines[0].bidi
	for i := c; i < len(bl.xl); i++ {
		if i > c {
			for _, gc := range l.GlyphCluster {
				if int(gc) == i {
					return false
				}
			}
		}
		if x >= bl.xl[i]-3 && x <= bl.xr[i]+3 {
			return true
		}
	}
	return false
}
