package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

func moreRunsText(lay *moreRichLayout) []string {
	var out []string
	for _, ln := range lay.lines {
		var b strings.Builder
		for _, r := range ln.runs {
			b.WriteString(r.text)
		}
		out = append(out, b.String())
	}
	return out
}

func TestMoreRichTextWrapAndKinsoku(t *testing.T) {
	var w *RichTextW
	d := driver(func(c *Context) {
		w = RichText(c, Span{Text: "这是一段中文，"}, Span{Text: "粗体", Bold: true}, Span{Text: "后面还有 English words here。"})
	})
	d.Frame()
	full := w.layout(geom.Inf)
	if len(full.lines) != 1 {
		t.Fatalf("unbounded: %d lines", len(full.lines))
	}
	// Runs are laid out without gaps or overlaps.
	x := float32(0)
	for _, r := range full.lines[0].runs {
		if r.x != x {
			t.Fatalf("run %q at %v, want %v", r.text, r.x, x)
		}
		x += r.w
	}
	for wd := float32(40); wd < full.w; wd += 7 {
		lay := w.layout(wd)
		for i, s := range moreRunsText(lay) {
			r, _ := moreFirstRune(s)
			if i > 0 && strings.ContainsRune("，。", r) {
				t.Fatalf("width %v: line %d starts with punctuation %q", wd, i, s)
			}
			if strings.HasPrefix(s, " ") {
				t.Fatalf("width %v: leading space %q", wd, s)
			}
		}
		for _, ln := range lay.lines {
			if ln.w > wd+0.01 && len(ln.runs) > 1 {
				t.Fatalf("width %v: line %v overflows", wd, ln.w)
			}
		}
		// Latin words are never split when they fit (at 40 the word may
		// be wider than the line with wide fonts such as DejaVu Sans).
		if fits := text.Shape("English", w.styleOf(2), text.Options{NoWrap: true}).Width <= wd; fits && strings.Contains(strings.Join(moreRunsText(lay), "|"), "Eng|lish") {
			t.Fatalf("word split at %v", wd)
		}
	}
	// Spaces have width (text.Width trims them).
	sp := moreSpaceWidth(w.styleOf(0))
	if sp <= 1 {
		t.Fatalf("space width %v", sp)
	}
}

func TestMoreRichTextLinksAndThemes(t *testing.T) {
	clicked := ""
	d := driver(func(c *Context) {
		Column(c, func() {
			w := RichText(c, Span{Text: "请阅读 "}, Span{Text: "使用文档", Link: "https://doc"}, Span{Text: " 了解更多。"})
			if u := w.ClickedLink(); u != "" {
				clicked = u
			}
		}, layout.Pad(20), layout.W(400))
	})
	d.Frame()
	d.Frame()
	st := State[moreRichState](d.r.c, d.r.c.root.kids[0].kids[0].id, nil)
	if len(st.links) != 1 {
		t.Fatalf("links %v", st.links)
	}
	d.Click(st.links[0].r.Center())
	if clicked != "https://doc" {
		t.Fatalf("clicked %q", clicked)
	}
	clicked = ""
	d.Click(geom.Pt(25, 30)) // on plain text
	if clicked != "" {
		t.Fatal("plain text reported a link")
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if r := d.r.c.rects[d.r.c.root.kids[0].kids[0].id]; r.H < 20 {
		t.Fatalf("element render %v", r)
	}
}

func TestMoreRichTextPreAndAlign(t *testing.T) {
	var w *RichTextW
	d := driver(func(c *Context) {
		w = RichText(c, Span{Text: "a\n\tb  c", Mono: true}).Pre()
	})
	d.Frame()
	got := moreRunsText(w.layout(geom.Inf))
	if len(got) != 2 || got[1] != "    b  c" {
		t.Fatalf("pre lines %q", got)
	}
}

func TestMoreRichTextSelectCopy(t *testing.T) {
	var w *RichTextW
	link := ""
	d := driver(func(c *Context) {
		Column(c, func() {
			w = RichText(c, Span{Text: "Hello "}, Span{Text: "world", Bold: true}, Span{Text: "，你好 "}, Span{Text: "链接", Link: "u"}).Selectable()
			if u := w.ClickedLink(); u != "" {
				link = u
			}
			Button(c, "其他")
		}, layout.Pad(20), layout.Gap(8))
	})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[w.ID()]
	short := input.ModCtrl
	if runtime.GOOS == "darwin" {
		short = input.ModMeta
	}
	// Drag from the start across "Hello wor".
	end := w.layout(r.W + 0.5).lines[0].runs[1]
	x := r.X + end.x + end.padL + text.Width("wor", w.styleOf(1))
	d.Drag(geom.Pt(r.X+1, r.Y+r.H/2), geom.Pt(x, r.Y+r.H/2))
	if got := w.Selection(); got != "Hello wor" {
		t.Fatalf("drag selection %q", got)
	}
	if d.r.c.focus != w.ID() {
		t.Fatalf("not focused after drag: %v", d.r.c.focus)
	}
	d.Key(input.KeyC, short)
	if d.Clipboard() != "Hello wor" {
		t.Fatalf("copied %q", d.Clipboard())
	}
	// Select all.
	d.Key(input.KeyA, short)
	if w.Selection() != "Hello world，你好 链接" {
		t.Fatalf("select all %q", w.Selection())
	}
	// Double click selects a word.
	wp := geom.Pt(r.X+end.x+end.padL+3, r.Y+r.H/2)
	d.Advance(time.Second)
	d.Click(wp)
	d.Click(wp)
	if w.Selection() != "world" {
		t.Fatalf("double click %q", w.Selection())
	}
	// A plain click on the link still follows it; Tab skips the text.
	d.Advance(time.Second)
	st := State[moreRichState](d.r.c, w.ID(), nil)
	d.Click(st.links[0].r.Center())
	if link != "u" {
		t.Fatalf("link %q", link)
	}
	d.r.c.SetFocus(0)
	d.Key(input.KeyTab, 0)
	if d.r.c.focus == w.ID() {
		t.Fatal("Tab focused selectable text")
	}
}
