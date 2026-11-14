package text

import "testing"

func TestItalicSetsSkewKeepsAdvances(t *testing.T) {
	// CJK fonts ship no italic, so the upright face is slanted and the
	// advances stay those of the upright face.
	needFont(t, "中文")
	up := Shape("中文", Style{Size: 14}, Options{NoWrap: true})
	it := Shape("中文", Style{Size: 14, Italic: true}, Options{NoWrap: true})
	if up.Width != it.Width || len(up.Glyphs) != len(it.Glyphs) {
		t.Fatalf("italic changed metrics: %v/%v %d/%d", up.Width, it.Width, len(up.Glyphs), len(it.Glyphs))
	}
	for i, g := range it.Glyphs {
		if g.Skew != ItalicSkew || up.Glyphs[i].Skew != 0 || g.X != up.Glyphs[i].X {
			t.Fatalf("glyph %d: %+v vs %+v", i, g, up.Glyphs[i])
		}
	}
	// Latin may come from a real italic face (SF Pro Italic, Segoe UI
	// Italic, Noto Sans Italic): those glyphs are not slanted again.
	for i, g := range Shape("Tango 中文", Style{Size: 14, Italic: true}, Options{NoWrap: true}).Glyphs {
		want := float32(ItalicSkew)
		if g.Src.(*Face).Italic {
			want = 0
		}
		if g.Skew != want {
			t.Fatalf("glyph %d skew %v want %v", i, g.Skew, want)
		}
	}
}
