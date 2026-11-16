//go:build darwin

package text

import (
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func TestCompactColdLabelKeepsOriginalVariableFont(t *testing.T) {
	coreTextOn(t)
	l := Shape("0", Style{Size: 56, Weight: Bold}, Options{NoWrap: true})
	f := l.Glyphs[0].Src.(*Face)
	if len(f.gt().Coords()) == 0 {
		t.Skip("system UI font is not variable")
	}
	in := shaping.Input{Text: []rune("0"), RunEnd: 1, Face: f.gt(), Size: fixed.I(56), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
	var s compactShaper
	s.append(nil, in, f, 0)
	if s.fonts[in.Face].font.Face().Font != in.Face.Font {
		t.Fatal("a single static label materialized an additional variable font")
	}
}

func TestCompactBusyFontSpecializesOnlyOnce(t *testing.T) {
	coreTextOn(t)
	l := Shape("office 0123", Style{Size: 16}, Options{NoWrap: true})
	f := l.Glyphs[0].Src.(*Face)
	if len(f.gt().Coords()) == 0 {
		t.Skip("system UI font is not variable")
	}
	in := shaping.Input{Text: []rune("office 0123"), RunEnd: 11, Face: f.gt(), Size: fixed.I(16), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
	var s compactShaper
	s.append(nil, in, f, 0)
	previous := s.fonts[in.Face].font
	changes := 0
	for range 256 {
		s.append(nil, in, f, 0)
		current := s.fonts[in.Face].font
		if current != previous {
			changes++
			previous = current
		}
	}
	if changes != 1 {
		t.Fatalf("busy font rebuilt %d times, want one specialization after its initial use", changes)
	}
	x, y := in.Face.Ppem()
	defer in.Face.SetPpem(x, y)
	in.Face.SetPpem(x+1, y+1)
	s.append(nil, in, f, 0)
	if s.fonts[in.Face].font.Face().Font != in.Face.Font {
		t.Fatal("ppem-only change reused a specialized font from the previous state")
	}
	if gotX, gotY := s.fonts[in.Face].font.Face().Ppem(); gotX != x+1 || gotY != y+1 {
		t.Fatal("ppem-only change lost the new pixel metrics")
	}
}
