//go:build !darwin || ios || !tango_coretext

package text

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/go-text/typesetting/shaping"
)

// Bidi tests that look into the go-text engine (shapeHook, the frozen
// pre-bidi layout of legacy_layout_test.go); moved out of bidi_test.go so
// the engine-neutral bidi tests also build with tango_coretext.

func TestShapeLanguage(t *testing.T) {
	UseSystemFonts(true)
	if len(Shape("字", Style{}, Options{}).Glyphs) == 0 {
		t.Skip("no fonts")
	}
	var langs []string
	shapeHook = func(in shaping.Input) { langs = append(langs, string(in.Language)+"/"+in.Script.String()) }
	defer func() { shapeHook = nil }()
	shapeWith := func(s string, st Style) []string {
		langs = nil
		clearCache()
		Shape(s, st, Options{})
		return langs
	}
	if got := shapeWith("直", Style{Lang: "ja"}); len(got) != 1 || !strings.HasPrefix(got[0], "ja/") {
		t.Errorf("ja: %v", got)
	}
	if got := shapeWith("直", Style{Lang: "zh-Hans"}); len(got) != 1 || !strings.HasPrefix(got[0], "zh-hans/") {
		t.Errorf("zh-Hans: %v", got)
	}
	// Empty Lang follows the default.
	if got := shapeWith("直", Style{}); len(got) != 1 || !strings.HasPrefix(got[0], "zh-hans/") {
		t.Errorf("default: %v", got)
	}
	SetDefaultLang("ko")
	defer SetDefaultLang("")
	if got := shapeWith("直", Style{}); len(got) != 1 || !strings.HasPrefix(got[0], "ko/") {
		t.Errorf("default ko: %v", got)
	}
	// The cache keys on the resolved language.
	a := Shape("直", Style{}, Options{})
	SetDefaultLang("ja")
	if b := Shape("直", Style{}, Options{}); a == b {
		t.Errorf("changing the default language reused the cached layout")
	}
	// Each bidi item gets its script; a foreign-script run gets that
	// script's language.
	if got := shapeWith("abc مرحبا", Style{Lang: "en"}); len(got) != 2 || !strings.HasPrefix(got[0], "en/") || !strings.HasPrefix(got[1], "ar/") {
		t.Errorf("en + Arabic: %v", got)
	}

	// Glyph forms: the same ideograph picks its Japanese form under "ja"
	// when the CJK face has locl forms (PingFang on macOS does).
	differ := 0
	for _, r := range []string{"直", "骨", "化", "写"} {
		ja := Shape(r, Style{Lang: "ja"}, Options{}).Glyphs
		zh := Shape(r, Style{Lang: "zh-Hans"}, Options{}).Glyphs
		if len(ja) == 1 && len(zh) == 1 && ja[0].ID != zh[0].ID {
			differ++
			t.Logf("%s: ja glyph %d, zh-Hans glyph %d (locl applied)", r, ja[0].ID, zh[0].ID)
		}
	}
	// ui.SetDefaultLocale(ZhCN) sets "zh-CN": same forms as the initial
	// "zh-Hans".
	for _, r := range []string{"直骨化写，。", "门户 English"} {
		a := Shape(r, Style{Lang: "zh-CN"}, Options{}).Glyphs
		b := Shape(r, Style{Lang: "zh-Hans"}, Options{}).Glyphs
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%q: zh-CN and zh-Hans glyphs differ", r)
		}
	}
	if differ == 0 {
		if runtime.GOOS == "darwin" {
			t.Errorf("no ja/zh-Hans glyph difference with the macOS CJK face")
		} else {
			t.Logf("CJK face has no ja locl forms; only Input.Language is checked")
		}
	}
}

// TestPureLTRUnchanged lays out left-to-right text with the current code
// and with the frozen pre-bidi copy (legacy_layout_test.go) and requires
// identical layouts, with and without fonts.
func TestPureLTRUnchanged(t *testing.T) {
	texts := []string{
		"", "hello", "hello world again and again and again",
		"一二三四五，六七八九十。甲乙丙", "中文 English 混排 123，标点「引号」",
		"a\tb\tc", "line one\nline two\n\nline four", "é café ﬁ office",
		"emoji 😀👍🏽 mix", "  leading and trailing  ", "ＡＢＣ全角　空格",
	}
	styles := []Style{{}, {Size: 12}, {Size: 20, Weight: Bold}, {Mono: true}, {Italic: true}}
	opts := []Options{{}, {MaxWidth: 80}, {MaxWidth: 60, MaxLines: 2, Ellipsis: true}, {MaxWidth: 50, NoWrap: true, Ellipsis: true}, {MaxWidth: 120, MaxLines: 1}}
	check := func(t *testing.T) {
		glyphs := 0
		defer func() {
			if len(reg.chain(false)) > 0 && glyphs == 0 {
				t.Errorf("no glyphs laid out with fonts")
			}
		}()
		for _, s := range texts {
			for _, st := range styles {
				for _, opt := range opts {
					st := st.norm()
					chain := reg.chain(st.Mono)
					mu.Lock()
					got := layout(s, st, opt, chain, resolveLang(st))
					want := legacyLayout(s, st, opt, chain)
					mu.Unlock()
					if len(got.GlyphCluster) != len(got.Glyphs) {
						t.Fatalf("%q: GlyphCluster len %d != %d", s, len(got.GlyphCluster), len(got.Glyphs))
					}
					got.GlyphCluster = nil
					if len(got.GlyphRun) != len(got.Glyphs) || slices.ContainsFunc(got.GlyphRun, func(r int32) bool { return r != 0 }) {
						t.Fatalf("%q: GlyphRun %v, want %d zeros", s, got.GlyphRun, len(got.Glyphs))
					}
					got.GlyphRun = nil
					glyphs += len(got.Glyphs)
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%q %+v %+v: layout changed\n got %+v\nwant %+v", s, st, opt, got.Lines, want.Lines)
					}
				}
			}
		}
	}
	t.Run("fonts", func(t *testing.T) { UseSystemFonts(true); check(t) })
	t.Run("nofonts", func(t *testing.T) { withoutFonts(t); check(t) })
}
