package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// fakeHL marks every "kw" as a keyword and "#…" as a comment; the second
// line's spans are not final on the first request.
type fakeHL struct{ asked int }

type fakeDoc struct {
	h     *fakeHL
	lines []string
}

func (h *fakeHL) Document(lang, code string) CodeDocument {
	if lang != "fake" {
		return nil
	}
	return &fakeDoc{h, strings.Split(code, "\n")}
}

func (d *fakeDoc) Line(i int, dst []CodeSpan) ([]CodeSpan, bool) {
	d.h.asked++
	dst = dst[:0]
	s := d.lines[i]
	for off := 0; ; {
		k := strings.Index(s[off:], "kw")
		if k < 0 {
			break
		}
		dst = append(dst, CodeSpan{off + k, off + k + 2, CodeKeyword})
		off += k + 2
	}
	if k := strings.IndexByte(s, '#'); k >= 0 {
		dst = append(dst, CodeSpan{k, len(s), CodeComment})
	}
	return dst, true
}

func TestHighlightSpans(t *testing.T) {
	tk := &theme.AntDesign().Components.CodeBlock
	spans, final := highlightSpans(&fakeHL{}, "fake", "a kw b\n# c", tk)
	if !final {
		t.Fatal("final")
	}
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
		if !s.Mono {
			t.Error("code spans are monospace")
		}
	}
	if b.String() != "a kw b\n# c" {
		t.Errorf("text %q", b.String())
	}
	if spans[1].Text != "kw" || spans[1].Color != tk.Keyword {
		t.Errorf("keyword span %+v", spans[1])
	}
	if last := spans[len(spans)-1]; last.Text != "# c" || !last.Italic || last.Color != tk.Comment {
		t.Errorf("comment span %+v", last)
	}
	plain, _ := highlightSpans(nil, "fake", "x", tk)
	if len(plain) != 1 || plain[0].Color.A != 0 {
		t.Error("no highlighter: plain text")
	}
	unknown, _ := highlightSpans(&fakeHL{}, "other", "x", tk)
	if len(unknown) != 1 {
		t.Error("unknown language: plain text")
	}
}

func TestMarkdownCodeUsesHighlighter(t *testing.T) {
	h := &fakeHL{}
	d := NewDriver(func(c *Context) {
		Markdown(c, "```fake\nkw x\n```\n").Highlighter(h)
	}, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	if h.asked == 0 {
		t.Fatal("the highlighter was not asked")
	}
	n := h.asked
	d.Frame()
	if h.asked != n {
		t.Error("final spans are cached")
	}
	SetCodeHighlighter(h)
	defer SetCodeHighlighter(nil)
	if DefaultCodeHighlighter() != h {
		t.Error("default highlighter")
	}
}

func TestCodeBlockCopyAndSelect(t *testing.T) {
	var w *CodeBlockW
	code := "kw one\r\nline two\n# three"
	d := NewDriver(func(c *Context) {
		w = CodeBlock(c, code).Lang("fake").Highlighter(&fakeHL{}).Title("f").LineNumbers(true)
	}, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	d.Frame()
	if err := d.ClickLabel("复制代码"); err != nil {
		t.Fatal(err)
	}
	if d.Clipboard() != "kw one\nline two\n# three" {
		t.Errorf("copy all = %q", d.Clipboard())
	}
	v := w.v
	if v.count != 3 || v.line(1) != "line two" {
		t.Fatalf("lines: %d %q", v.count, v.line(1))
	}
	// Clicking a line number selects the line.
	st := v.st
	y := st.originY + v.padY + v.lineH*1.5
	d.Click(geom.Pt(st.view.X+4, y))
	if w.Selection() != "line two" {
		t.Errorf("gutter click selects the line: %q", w.Selection())
	}
	// Double click selects a word.
	d.Advance(time.Second)
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(st.textX+v.cellW()*6, y)})
	d.Click(geom.Pt(st.textX+v.cellW()*6, y))
	d.Click(geom.Pt(st.textX+v.cellW()*6, y))
	if w.Selection() != "two" {
		t.Errorf("double click selects the word: %q", w.Selection())
	}
	d.Key(input.KeyC, input.ModCtrl|input.ModMeta)
	if d.Clipboard() != "two" {
		t.Errorf("copy selection = %q", d.Clipboard())
	}
}

func TestCodeBlockHeightAndScroll(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&b, "line %d %s\n", i, strings.Repeat("x", i%300))
	}
	var w *CodeBlockW
	d := NewDriver(func(c *Context) {
		w = CodeBlock(c, b.String()).MaxHeight(200).LineNumbers(true)
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[w.b.n.id]
	if r.H > 210 || r.H < 190 {
		t.Errorf("height = %v, want MaxHeight", r.H)
	}
	if built := w.v.end - w.v.first; built > 30 {
		t.Errorf("built %d rows for a 200px viewport", built)
	}
	if w.v.sb.s.content.X < 2000 {
		t.Errorf("content width %v covers the longest line", w.v.sb.s.content.X)
	}
	d.Scroll(r.Center(), 5000)
	if w.v.first == 0 {
		t.Error("scrolling moves the window of built rows")
	}
}

func TestTxvCellsAndWrap(t *testing.T) {
	for in, want := range map[string]int{"abc": 3, "\tx": 5, "a\tb": 5, "中文": 4, "é": 1, "": 0} {
		if got := txvCells(in); got != want {
			t.Errorf("txvCells(%q) = %d, want %d", in, got, want)
		}
	}
	v := &txView{wrap: true, st: &txvState{wrapCols: 4}}
	for seg, want := range []string{"abcd", "efgh", "ij"} {
		b0, b1, _ := v.segment("abcdefghij", seg)
		if got := "abcdefghij"[b0:b1]; got != want {
			t.Errorf("segment %d = %q, want %q", seg, got, want)
		}
	}
	b0, b1, r0 := v.segment("中文中文", 1)
	if "中文中文"[b0:b1] != "中文" || r0 != 2 {
		t.Errorf("wide segment %q r0=%d", "中文中文"[b0:b1], r0)
	}
}

func BenchmarkCodeBlockFrame100k(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 100_000; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int { return x*%d + 1 } // kw %d\n", i, i, i)
	}
	code := sb.String()
	h := &fakeHL{}
	var w *CodeBlockW
	d := NewDriver(func(c *Context) {
		w = CodeBlock(c, code).Lang("fake").Highlighter(h).LineNumbers(true).Height(600)
	}, DriverOptions{Width: 1000, Height: 700})
	d.Frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.v.sb.s.off.Y = float64(i%1000) * 200
		d.Frame()
	}
}
