package editor

import (
	"image/color"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// refMiniCharsAt is miniCharsAt before its ASCII fast path: every
// character segmented and measured as a grapheme cluster.
func refMiniCharsAt(s string, tab int, g *miniGlyphs, colorAt func(i int) color.NRGBA) *miniBitmap {
	// Measure the last drawable column first. Keeping a growable cell list
	// allocated several buffers per miniature; two allocation-free walks
	// leave only the owned bitmap and pixels. Color lookup happens once,
	// during the drawing walk, because its token cursor is stateful.
	col, end := 0, 0
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		next := advanceCol(col, s[i:j], tab)
		if gr := s[i:j]; gr != " " && gr != "\t" {
			if minimapGlyphIndex(gr) >= 0 && next > col {
				end = next
			}
		}
		col, i = next, j
	}
	if end == 0 {
		return nil
	}
	cw := g.cw
	b := &miniBitmap{w: end * cw, h: 2 * cw}
	b.pix = make([]byte, 4*b.w*b.h)
	size := cw * 2 * cw
	col = 0
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		next := advanceCol(col, s[i:j], tab)
		if gr := s[i:j]; gr != " " && gr != "\t" {
			if gi := minimapGlyphIndex(gr); gi >= 0 && next > col {
				c := colorAt(i)
				src := g.alpha[gi*size : (gi+1)*size]
				for k := range next - col {
					x0 := (col + k) * cw
					for y := range 2 * cw {
						for x := range cw {
							p := b.pix[4*(y*b.w+x0+x):]
							p[0], p[1], p[2], p[3] = c.R, c.G, c.B, src[y*cw+x]
						}
					}
				}
			}
		}
		col, i = next, j
	}
	return b
}

// TestMiniCharsAtMatchesReference: the bitmap of any text (ASCII, tabs,
// control characters, DEL, a CR LF, combining marks after ASCII letters,
// wide CJK, emoji, invalid UTF-8) is the reference's, pixel for pixel, at
// both cell sizes, with a color that changes along the line.
func TestMiniCharsAtMatchesReference(t *testing.T) {
	pieces := []string{"a", "Z", "0", " ", "  ", "\t", "{", "}", "~", "\x7f", "\x01", "\x1f", "\r\n", "\r",
		"e\u0301", "a\u0308\u0301", "中", "文字", "😀", "👍🏽", "🇨🇳", "\xff", "x\xff", "é", "ß", "\u200d", "abc def", "\tif x {"}
	r := rand.New(rand.NewPCG(9, 9))
	for _, cw := range []int{1, 2} {
		alpha := make([]byte, 96*cw*2*cw)
		for i := range alpha {
			alpha[i] = byte(r.IntN(256))
		}
		g := &miniGlyphs{cw: cw, alpha: alpha}
		colorAt := func(i int) color.NRGBA { return color.NRGBA{R: byte(i), G: byte(3 * i), B: byte(7 * i), A: 255} }
		for range 3000 {
			var sb strings.Builder
			for range r.IntN(20) {
				sb.WriteString(pieces[r.IntN(len(pieces))])
			}
			s := sb.String()
			for _, tab := range []int{1, 4, 8} {
				got, want := miniCharsAt(s, tab, g, colorAt), refMiniCharsAt(s, tab, g, colorAt)
				if (got == nil) != (want == nil) {
					t.Fatalf("cw %d tab %d %q: bitmap %v, reference %v", cw, tab, s, got != nil, want != nil)
				}
				if got != nil && (got.w != want.w || got.h != want.h || !slices.Equal(got.pix, want.pix)) {
					t.Fatalf("cw %d tab %d %q: bitmap %dx%d differs from the reference %dx%d", cw, tab, s, got.w, got.h, want.w, want.h)
				}
			}
		}
	}
}
