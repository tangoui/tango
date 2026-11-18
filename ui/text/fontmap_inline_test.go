//go:build !darwin || ios || !tango_coretext

package text

import (
	"github.com/go-text/typesetting/font"
	"testing"
)

func TestFontmapKeepsSingleFaceInline(t *testing.T) {
	m := &fontmap{}
	a, b := &font.Face{}, &font.Face{}
	x, y := &Face{}, &Face{}
	m.remember(a, x)
	if m.byFF != nil || m.fromFont(a) != x {
		t.Fatal("single face must not allocate a font map")
	}
	m.remember(b, y)
	if m.fromFont(a) != x || m.fromFont(b) != y {
		t.Fatal("fallback font mapping lost")
	}
}

func TestFontmapInlineFaceDoesNotAllocate(t *testing.T) {
	a := &font.Face{}
	x := &Face{}
	allocations := testing.AllocsPerRun(20, func() {
		m := &fontmap{}
		m.remember(a, x)
		if m.fromFont(a) != x {
			panic("missing font")
		}
	})
	if allocations != 0 {
		t.Fatalf("single-face mapping allocated %.0f times", allocations)
	}
}
