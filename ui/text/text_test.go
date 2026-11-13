package text

import (
	"strings"
	"testing"
)

// Tests run against the approximate metrics (no fonts) so they are
// deterministic on every machine, then repeat key checks with system fonts
// when available.
func withoutFonts(t *testing.T) {
	UseSystemFonts(false)
	t.Cleanup(func() { UseSystemFonts(true) })
}

func lines(l *Layout) []string {
	var out []string
	for _, ln := range l.Lines {
		out = append(out, string(l.Runes[ln.Start:ln.End]))
	}
	return out
}

func TestChineseBreaksPerCharacterWithKinsoku(t *testing.T) {
	withoutFonts(t)
	// 14px CJK chars are 14px wide: 5 per 70px line.
	s := "一二三四五，六七八九十。甲乙丙"
	l := Shape(s, Style{Size: 14}, Options{MaxWidth: 70})
	got := lines(l)
	for _, ln := range got {
		if strings.HasPrefix(ln, "，") || strings.HasPrefix(ln, "。") {
			t.Fatalf("line starts with closing punctuation: %q", got)
		}
	}
	if len(got) < 3 {
		t.Fatalf("expected wrapping, got %q", got)
	}
	if strings.Join(got, "") != s {
		t.Fatalf("lost text: %q", got)
	}
}

func TestOpeningBracketNotAtLineEnd(t *testing.T) {
	withoutFonts(t)
	l := Shape("一二三四「五六」七八", Style{Size: 14}, Options{MaxWidth: 70})
	for _, ln := range lines(l) {
		if strings.HasSuffix(ln, "「") {
			t.Fatalf("line ends with opening bracket: %q", lines(l))
		}
	}
}

func TestLatinWrapsAtSpaces(t *testing.T) {
	withoutFonts(t)
	// 0.55*14 = 7.7px per char.
	l := Shape("hello world again", Style{Size: 14}, Options{MaxWidth: 100})
	got := lines(l)
	if got[0] != "hello world " && got[0] != "hello " {
		t.Fatalf("unexpected first line %q", got)
	}
	for _, ln := range got {
		if strings.HasPrefix(ln, " ") {
			t.Fatalf("leading space: %q", got)
		}
	}
}

func TestLongWordBreaksPerCharacter(t *testing.T) {
	withoutFonts(t)
	l := Shape(strings.Repeat("x", 40), Style{Size: 14}, Options{MaxWidth: 77})
	if len(l.Lines) != 4 {
		t.Fatalf("lines = %d (%q)", len(l.Lines), lines(l))
	}
	if l.Width > 77 {
		t.Fatalf("overflow %v", l.Width)
	}
}

func TestEllipsisAndMaxLines(t *testing.T) {
	withoutFonts(t)
	l := Shape("这是一段非常非常长的文本需要截断", Style{Size: 14}, Options{MaxWidth: 70, NoWrap: true, Ellipsis: true})
	if len(l.Lines) != 1 || !l.Truncated || l.Width > 70 {
		t.Fatalf("ellipsis: lines=%d trunc=%v width=%v", len(l.Lines), l.Truncated, l.Width)
	}
	l2 := Shape(strings.Repeat("字", 30), Style{Size: 14}, Options{MaxWidth: 70, MaxLines: 2, Ellipsis: true})
	if len(l2.Lines) != 2 || !l2.Truncated {
		t.Fatalf("max lines: %d", len(l2.Lines))
	}
}

func TestMeasureAndLineHeight(t *testing.T) {
	withoutFonts(t)
	st := Style{Size: 14}
	if LineHeightOf(st) != 22 || LineHeightOf(Style{Size: 12}) != 20 {
		t.Fatal("line heights")
	}
	a := Width("ab", st)
	b := Width("abcd", st)
	if !(b > a && a > 0) {
		t.Fatalf("widths %v %v", a, b)
	}
	sz := Measure("a\nb\nc", st, Options{})
	if sz.H != 66 {
		t.Fatalf("3 paragraphs height %v", sz.H)
	}
}

func TestCaretAndHitTest(t *testing.T) {
	withoutFonts(t)
	l := Shape("abcdef", Style{Size: 14}, Options{NoWrap: true})
	x0, _, _ := l.Caret(0)
	x3, _, h := l.Caret(3)
	if x0 != 0 || x3 <= 0 || h != 22 {
		t.Fatalf("caret %v %v %v", x0, x3, h)
	}
	if i := l.IndexAt(x3+1, 5); i != 3 {
		t.Fatalf("IndexAt = %d", i)
	}
	if i := l.IndexAt(1000, 5); i != 6 {
		t.Fatalf("IndexAt end = %d", i)
	}
}

func TestCacheReturnsSameLayout(t *testing.T) {
	withoutFonts(t)
	a := Shape("cache", Style{Size: 14}, Options{})
	b := Shape("cache", Style{Size: 14}, Options{})
	if a != b {
		t.Fatal("expected cached layout")
	}
}

func TestSystemFontsShapeChinese(t *testing.T) {
	if len(Families()) == 0 {
		t.Skip("no system fonts")
	}
	l := Shape("你好，Tango", Style{Size: 14}, Options{})
	if len(l.Glyphs) < 7 {
		t.Fatalf("glyphs = %d", len(l.Glyphs))
	}
	for _, g := range l.Glyphs {
		if g.Src == nil {
			t.Fatal("glyph without face")
		}
	}
	bold := Width("你好", Style{Size: 14, Weight: Bold})
	if bold <= 0 {
		t.Fatal("bold width")
	}
	l2 := Shape("这是一段很长的中文文本，用来测试按字断行与避头尾规则。", Style{Size: 14}, Options{MaxWidth: 100})
	for _, ln := range lines(l2) {
		if strings.HasPrefix(ln, "，") || strings.HasPrefix(ln, "。") {
			t.Fatalf("kinsoku violated with real fonts: %q", lines(l2))
		}
	}
}
