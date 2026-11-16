package paint

import (
	"bytes"
	"github.com/tangoui/tango/ui/geom"
	"image/color"
	"testing"
)

type trimGlyphSource struct{ calls int }

func (s *trimGlyphSource) GlyphPath(uint32, float32) *Path {
	s.calls++
	return NewPath().Rect(geom.R(0, -8, 6, 8))
}

func TestCPUTrimReleasesRebuildableCachesWithoutChangingFrame(t *testing.T) {
	source := &trimGlyphSource{}
	l := &List{Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	l.Glyphs(geom.Pt(4, 12), []Glyph{{Src: source, ID: 1, Size: 12}}, color.NRGBA{A: 255})
	cpu := NewCPU()
	size := geom.Size{W: 32, H: 24}
	if err := cpu.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), cpu.Image().Pix...)
	calls := source.calls
	if len(cpu.glyphs) == 0 {
		t.Fatal("test did not populate glyph cache")
	}
	cpu.Trim()
	if len(cpu.glyphs) != 0 || len(cpu.imgs) != 0 || len(cpu.shadows) != 0 {
		t.Fatal("rebuildable caches survived trim")
	}
	if !bytes.Equal(before, cpu.Image().Pix) {
		t.Fatal("trim destroyed the currently presented frame")
	}
	if err := cpu.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	if source.calls <= calls {
		t.Fatal("glyph was not rebuilt after trim")
	}
	if !bytes.Equal(before, cpu.Image().Pix) {
		t.Fatal("trim changed rendered pixels")
	}
	cpu.Trim()
	cpu.Trim()
}
