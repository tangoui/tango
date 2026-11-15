package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/theme"
)

func TestANSIParse(t *testing.T) {
	plain, spans := ParseANSI("a\x1b[1;31mbold red\x1b[0m b \x1b[38;5;208mo\x1b[38;2;1;2;3mt\x1b[39m\x1b[4mu\x1b[24m")
	if plain != "abold red b otu" {
		t.Fatalf("plain = %q", plain)
	}
	want := []ANSISpan{
		{1, 9, ANSIStyle{Fg: ANSIColor{Kind: ANSIIndexed, Index: 1}, Bold: true}},
		{12, 13, ANSIStyle{Fg: ANSIColor{Kind: ANSIIndexed, Index: 208}}},
		{13, 14, ANSIStyle{Fg: ANSIColor{Kind: ANSIRGB, R: 1, G: 2, B: 3}}},
		{14, 15, ANSIStyle{Underline: true}},
	}
	if len(spans) != len(want) {
		t.Fatalf("spans = %+v", spans)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, spans[i], want[i])
		}
	}
}

func TestANSIParserCarriesStyle(t *testing.T) {
	var p ANSIParser
	_, s1 := p.Line("\x1b[32mgreen", nil)
	plain, s2 := p.Line("still green\x1b[0m plain", nil)
	if len(s1) != 1 || len(s2) != 1 || s2[0].End != len("still green") || plain != "still green plain" {
		t.Errorf("s1=%+v s2=%+v plain=%q", s1, s2, plain)
	}
	if _, s3 := p.Line("x", nil); len(s3) != 0 {
		t.Error("reset must end the style")
	}
}

func TestANSIStripsOtherSequences(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b]0;title\x07text":            "text",
		"\x1b]8;;http://x\x1b\\link":      "link",
		"a\x1b[2Kb\x1b[?25lc":             "abc",
		"50%\r100%":                       "100%",
		"tab\there\x00\x7f":               "tab\there",
		"\x1b(Bok":                        "ok",
		"\x1b[38:2::10:20:30mcolon\x1b[m": "colon",
		"bright\x1b[91m!":                 "bright!",
		"中文\x1b[1m粗":                      "中文粗",
	} {
		if got, _ := ParseANSI(in); got != want {
			t.Errorf("ParseANSI(%q) = %q, want %q", in, got, want)
		}
	}
	_, sp := ParseANSI("\x1b[38:2::10:20:30mx")
	if len(sp) != 1 || sp[0].Style.Fg != (ANSIColor{Kind: ANSIRGB, R: 10, G: 20, B: 30}) {
		t.Errorf("colon true color: %+v", sp)
	}
	_, sp = ParseANSI("\x1b[7;9;3;2;101mx")
	if st := sp[0].Style; !st.Inverse || !st.Strike || !st.Italic || !st.Faint || st.Bg.Index != 9 {
		t.Errorf("attributes: %+v", st)
	}
}

func TestANSIColorTable(t *testing.T) {
	tk := &theme.AntDesign().Components.LogView
	if ansiColor(tk, ANSIColor{Kind: ANSIIndexed, Index: 1}) != tk.Red {
		t.Error("index 1 is the theme red")
	}
	if c := ansiColor(tk, ANSIColor{Kind: ANSIIndexed, Index: 196}); c != (theme.Color{R: 255, A: 255}) {
		t.Errorf("cube red = %v", c)
	}
	if c := ansiColor(tk, ANSIColor{Kind: ANSIIndexed, Index: 232}); c != (theme.Color{R: 8, G: 8, B: 8, A: 255}) {
		t.Errorf("gray = %v", c)
	}
	if ansiColor(tk, ANSIColor{}).A != 0 {
		t.Error("default color is zero")
	}
}

func BenchmarkANSILine(b *testing.B) {
	var p ANSIParser
	line := "2026-11-14 10:00:00.000 \x1b[33mWARN\x1b[0m cache: miss ratio \x1b[1m42%\x1b[22m above \x1b[38;5;208mthreshold\x1b[0m"
	var dst []ANSISpan
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, dst = p.Line(line, dst[:0])
	}
}
