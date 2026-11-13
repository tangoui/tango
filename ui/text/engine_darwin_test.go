//go:build darwin

package text

import (
	"testing"

	"github.com/go-text/typesetting/font"
)

func coreTextOn(t *testing.T) {
	t.Helper()
	withEngine(t, true)
	if Engine() != "coretext" {
		t.Skip("CoreText unavailable")
	}
}

func TestCoreTextPingFang(t *testing.T) {
	coreTextOn(t)
	fams := Families()
	if len(fams) < 2 || fams[0] != "SF Pro" || fams[1] != "PingFang SC" {
		t.Fatalf("chain %v", fams)
	}
	for _, w := range []Weight{Regular, Bold} {
		l := Shape("中文，排版。", Style{Size: 13, Weight: w}, Options{})
		for _, g := range l.Glyphs {
			if f := g.Src.(*Face); f.Family != "PingFang SC" {
				t.Fatalf("weight %d: CJK from %s", w, f.Family)
			}
		}
	}
	// PingFang has no 700: CSS matching takes Semibold.
	if f := Shape("中", Style{Weight: Bold}, Options{}).Glyphs[0].Src.(*Face); f.Weight != Semibold {
		t.Fatalf("bold PingFang weight %d", f.Weight)
	}
	// Latin stays on the system UI font.
	if f := Shape("A", Style{}, Options{}).Glyphs[0].Src.(*Face); f.Family != "SF Pro" {
		t.Fatalf("latin from %s", f.Family)
	}
}

// TestCoreTextAdvancesMatchShaping: go-text shapes from the tables CoreText
// renders, at the same variation instance. CoreText's own advances only add
// the font's uniform tracking (trak, which our layout does not apply), so
// the per-glyph difference is one constant per run.
func TestCoreTextAdvancesMatchShaping(t *testing.T) {
	coreTextOn(t)
	for _, s := range []string{"Tango Wiiillm 0123", "你好，中文。"} {
		for _, w := range []Weight{Regular, Semibold, Bold} {
			for _, sz := range []float32{11, 13, 20, 32} {
				l := Shape(s, Style{Size: sz, Weight: w}, Options{NoWrap: true})
				lo, hi := float32(1e9), float32(-1e9)
				for _, g := range l.Glyphs {
					f := g.Src.(*Face)
					d := f.ras.(*ctRaster).advance(g.ID, sz) - f.face.HorizontalAdvance(font.GID(g.ID))*sz/f.upem
					lo, hi = min(lo, d), max(hi, d)
				}
				if hi-lo > 0.01 {
					t.Errorf("%q w%d %vpx: advance differences %v..%v are not one tracking constant", s, w, sz, lo, hi)
				}
			}
		}
	}
}

func TestCoreTextFallbackScripts(t *testing.T) {
	coreTextOn(t)
	for s, fam := range map[string]string{"مرحبا": ".SF Arabic", "😀": "Apple Color Emoji", "한국어": "Apple SD Gothic Neo"} {
		l := Shape(s, Style{Size: 14}, Options{})
		for _, g := range l.Glyphs {
			if f := g.Src.(*Face); f.Family != fam || g.ID == 0 {
				t.Errorf("%q: glyph %d from %s, want %s", s, g.ID, f.Family, fam)
			}
		}
	}
	// Colour emoji are drawn (as gray levels), not left blank.
	g := Shape("😀", Style{Size: 14}, Options{}).Glyphs[0]
	if m, _, ok := g.Src.(*Face).RasterGlyph(g.ID, 14, 0, 0); !ok || m == nil || inkBounds(m).Empty() {
		t.Fatal("emoji mask empty")
	}
}

// TestCoreTextOutline: transformed text still needs outlines; PingFang's
// come from CoreText (CTFontCreatePathForGlyph).
func TestCoreTextOutline(t *testing.T) {
	coreTextOn(t)
	g := Shape("永", Style{Size: 20}, Options{}).Glyphs[0]
	p := g.Src.GlyphPath(g.ID, 20)
	if p == nil || len(p.Segs) < 10 {
		t.Fatal("no PingFang outline")
	}
	if b := p.Bounds(); b.H < 14 || b.H > 21 || b.Y > -12 {
		t.Fatalf("outline bounds %+v", b)
	}
}
