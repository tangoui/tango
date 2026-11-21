package text

import (
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestExplicitFontFamiliesAcrossShapingPaths(t *testing.T) {
	withoutFonts(t)
	reg.mu.Lock()
	users := reg.user
	reg.mu.Unlock()
	t.Cleanup(func() {
		reg.mu.Lock()
		reg.user = users
		reg.version++
		reg.mu.Unlock()
		clearCache()
	})
	for name, data := range map[string][]byte{"Tango Test Sans": goregular.TTF, "Tango Test Mono": gomono.TTF} {
		if err := RegisterFont(name, data); err != nil {
			t.Fatal(err)
		}
	}
	sans := Style{FontFamily: "Tango Test Sans"}
	mono := Style{FontFamily: "Unavailable Tango Family, tango test mono, Tango Test Sans"}
	opt := Options{NoWrap: true}
	check := func(l *Layout, first, last string) {
		t.Helper()
		if len(l.Glyphs) == 0 {
			t.Fatal("missing glyphs")
		}
		for _, g := range []struct {
			index int
			want  string
		}{{0, first}, {len(l.Glyphs) - 1, last}} {
			if f := l.Glyphs[g.index].Src.(*Face); f.Family != g.want {
				t.Errorf("glyph %d uses %q, want %q", g.index, f.Family, g.want)
			}
		}
	}
	check(Shape("140", sans, opt), "Tango Test Sans", "Tango Test Sans")
	check(ShapeInto(nil, "140", mono, opt), "Tango Test Mono", "Tango Test Mono")
	mono.Mono = true
	check(Shape("140", mono, opt), "Tango Test Mono", "Tango Test Mono")
	runs := []Run{{0, 1, sans}, {1, 2, mono}}
	check(ShapeRuns("ii", runs, opt), "Tango Test Sans", "Tango Test Mono")
	runs[0].Style, runs[1].Style = mono, sans
	check(ShapeRuns("ii", runs, opt), "Tango Test Mono", "Tango Test Sans")
	c := NewLineCache(4)
	customFixed := sans
	customFixed.Mono = true
	check(c.Shape("ii", []Run{{0, 2, customFixed}}, LineOptions{}), "Tango Test Sans", "Tango Test Sans")
	check(c.Shape("ii", runs, LineOptions{}), "Tango Test Mono", "Tango Test Sans")
	runs[0].Style, runs[1].Style = sans, mono
	check(c.Shape("ii", runs, LineOptions{}), "Tango Test Sans", "Tango Test Mono")
	if sw, mw := Width("iii", sans), Width("iii", mono); sw >= mw {
		t.Errorf("expected narrower proportional text: sans=%v mono=%v", sw, mw)
	}
}
