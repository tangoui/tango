package text

import (
	"os"
	"runtime"
	"testing"
)

func TestPickItalic(t *testing.T) {
	up4 := &Face{Weight: Regular}
	up7 := &Face{Weight: Bold}
	it4 := &Face{Weight: Regular, Italic: true}
	fm := &family{faces: []*Face{up4, it4, up7}, loaded: true}
	if f, synth := fm.pick(Regular, false); f != up4 || synth {
		t.Fatal("upright regular")
	}
	if f, synth := fm.pick(Regular, true); f != it4 || synth {
		t.Fatal("real italic should win")
	}
	// Bold italic: the only italic is Regular; CSS picks it over a
	// synthetic slant of the bold upright.
	if f, synth := fm.pick(Bold, true); f != it4 || synth {
		t.Fatal("nearest-weight italic")
	}
	// Upright requests never get the italic face.
	if f, _ := fm.pick(Medium, false); f.Italic {
		t.Fatal("upright request got italic")
	}
	noIt := &family{faces: []*Face{up4, up7}, loaded: true}
	if f, synth := noIt.pick(Bold, true); f != up7 || !synth {
		t.Fatal("missing italic should synthesize from the upright face")
	}
	only := &family{faces: []*Face{it4}, loaded: true}
	if f, _ := only.pick(Regular, false); f != it4 {
		t.Fatal("italic-only family still usable")
	}
}

func TestItalicApproxMetrics(t *testing.T) {
	withoutFonts(t)
	a := Width("italic", Style{Size: 14})
	b := Width("italic", Style{Size: 14, Italic: true})
	if a != b || a == 0 {
		t.Fatalf("approx widths %v %v", a, b)
	}
	if Shape("x", Style{Italic: true}, Options{}) == Shape("x", Style{}, Options{}) {
		t.Fatal("italic must not share the upright cache entry")
	}
}

func TestSystemItalic(t *testing.T) {
	if len(Families()) == 0 {
		t.Skip("no system fonts")
	}
	// CJK families ship no italic: the upright face is slanted.
	cjk := Shape("中文", Style{Size: 14, Italic: true}, Options{})
	if len(cjk.Glyphs) == 0 {
		t.Skip("no CJK font")
	}
	for _, g := range cjk.Glyphs {
		if f := g.Src.(*Face); !f.Italic && g.Skew != ItalicSkew {
			t.Fatalf("synthetic oblique missing: %+v", g)
		}
	}
	up := Shape("中文", Style{Size: 14}, Options{})
	if up.Glyphs[0].Skew != 0 {
		t.Fatal("upright text slanted")
	}
	if cjk.Width != up.Width {
		t.Fatalf("synthetic italic changes width %v vs %v", cjk.Width, up.Width)
	}
	if runtime.GOOS != "darwin" {
		return
	}
	if _, err := os.Stat("/System/Library/Fonts/SFNSItalic.ttf"); err != nil {
		return
	}
	l := Shape("Italic", Style{Size: 14, Italic: true}, Options{})
	for _, g := range l.Glyphs {
		f := g.Src.(*Face)
		if !f.Italic || g.Skew != 0 {
			t.Fatalf("want SF Pro Italic face, got %s italic=%v skew=%v", f.Family, f.Italic, g.Skew)
		}
	}
	bold := Shape("Italic", Style{Size: 14, Weight: Bold, Italic: true}, Options{})
	if f := bold.Glyphs[0].Src.(*Face); !f.Italic || f.Weight != Bold {
		t.Fatalf("bold italic face %+v", f.Weight)
	}
}
