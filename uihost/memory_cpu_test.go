package uihost

import (
	"bytes"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"image/color"
	"testing"
)

type oversizedTrimSource struct{ calls int }

func (s *oversizedTrimSource) GlyphPath(uint32, float32) *paint.Path {
	s.calls++
	return paint.NewPath().Rect(geom.R(0, -6, 4, 6))
}

func TestTrimClearsOversizedCPUFallbackMasksButKeepsFrame(t *testing.T) {
	src := &oversizedTrimSource{}
	list := &paint.List{Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	list.Glyphs(geom.Pt(4, 10), []paint.Glyph{{Src: src, ID: 1, Size: 12}}, color.NRGBA{A: 255})
	r := &gpuRenderer{big: paint.NewCPU(), bigFrame: true}
	size := geom.Size{W: 32, H: 24}
	if err := r.big.Render(list, size, 1); err != nil {
		t.Fatal(err)
	}
	pixels := append([]byte(nil), r.big.Image().Pix...)
	calls := src.calls
	r.trim()
	if r.big == nil || !bytes.Equal(pixels, r.big.Image().Pix) {
		t.Fatal("trim discarded the current oversized frame")
	}
	if err := r.big.Render(list, size, 1); err != nil {
		t.Fatal(err)
	}
	if src.calls <= calls {
		t.Fatal("oversized CPU fallback kept its glyph source cache")
	}
	if !bytes.Equal(pixels, r.big.Image().Pix) {
		t.Fatal("oversized fallback pixels changed after trim")
	}
}
