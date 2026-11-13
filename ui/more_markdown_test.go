package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func mdParseString(s string) []*mdBlock {
	p := &mdParser{}
	return p.parse(strings.Split(s, "\n"))
}

func TestMoreMarkdownBlocks(t *testing.T) {
	src := "# 标题\n\n段落第一行\n第二行  \n硬换行\n\nSetext\n===\n\n- a\n- b\n  - b1\n\n  b 的第二段\n- [x] 任务\n\n3. 三\n4. 四\n\n> 引用\n续行\n\n```go\nx := 1\n```\n\n    缩进代码\n\n***\n\n| A | B |\n|:-|-:|\n| 1 | 2 |\n\n![图](a.png)"
	bs := mdParseString(src)
	kinds := []int{mdHeading, mdPara, mdHeading, mdList, mdList, mdQuote, mdCode, mdCode, mdHR, mdTable, mdImage}
	if len(bs) != len(kinds) {
		for _, b := range bs {
			t.Logf("%d %q", b.kind, b.text)
		}
		t.Fatalf("got %d blocks", len(bs))
	}
	for i, k := range kinds {
		if bs[i].kind != k {
			t.Fatalf("block %d kind %d want %d", i, bs[i].kind, k)
		}
	}
	if bs[1].text != "段落第一行第二行\n硬换行" {
		t.Fatalf("para %q", bs[1].text)
	}
	l := bs[3]
	if len(l.items) != 3 || len(l.items[1].blocks) != 3 || l.items[1].blocks[1].kind != mdList || l.items[2].task != 1 {
		t.Fatalf("list %+v", l.items)
	}
	if !bs[4].ordered || bs[4].start != 3 {
		t.Fatal("ordered start")
	}
	if q := bs[5].kids; len(q) != 1 || q[0].text != "引用续行" {
		t.Fatalf("quote %+v", q)
	}
	if bs[6].lang != "go" || bs[6].text != "x := 1" || bs[7].text != "缩进代码" {
		t.Fatalf("code %q %q", bs[6].text, bs[7].text)
	}
	if tb := bs[9]; len(tb.rows) != 1 || tb.aligns[1] != AlignRight || tb.rows[0][1] != "2" {
		t.Fatalf("table %+v", tb)
	}
	if bs[10].src != "a.png" || bs[10].text != "图" {
		t.Fatal("image")
	}
}

func TestMoreMarkdownInline(t *testing.T) {
	sp := ParseMarkdownInline(`a **b *c*** ~~d~~ ==e== ` + "`f*g`" + ` [h](u "t") <https://x.y> https://z.cn. \*i\* snake_case_x _j_`)
	want := []struct {
		text string
		chk  func(Span) bool
	}{
		{"b ", func(s Span) bool { return s.Bold && !s.Italic }},
		{"c", func(s Span) bool { return s.Bold && s.Italic }},
		{"d", func(s Span) bool { return s.Strike }},
		{"e", func(s Span) bool { return s.Mark }},
		{"f*g", func(s Span) bool { return s.Code }},
		{"h", func(s Span) bool { return s.Link == "u" }},
		{"https://x.y", func(s Span) bool { return s.Link == "https://x.y" }},
		{"https://z.cn", func(s Span) bool { return s.Link == "https://z.cn" }},
		{"j", func(s Span) bool { return s.Italic }},
	}
	for _, w := range want {
		found := false
		for _, s := range sp {
			if s.Text == w.text && w.chk(s) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in %+v", w.text, sp)
		}
	}
	if plain := PlainText(sp); !strings.Contains(plain, "*i*") || !strings.Contains(plain, "snake_case_x") {
		t.Fatalf("escapes / intraword: %q", plain)
	}
}

func TestMoreMarkdownToggleTask(t *testing.T) {
	src := "- [ ] a\n```\n- [ ] in code\n```\n> - [x] quoted\n1. [ ] b"
	got := ToggleMarkdownTask(src, 1)
	if !strings.Contains(got, "> - [ ] quoted") {
		t.Fatalf("toggle 1: %q", got)
	}
	got = ToggleMarkdownTask(got, 2)
	if !strings.Contains(got, "1. [x] b") || !strings.Contains(got, "- [ ] in code") {
		t.Fatalf("toggle 2: %q", got)
	}
}

func TestMoreMarkdownRenderInteract(t *testing.T) {
	src := "# 文档\n\n见 [官网](https://tango.dev)。\n\n- [ ] 待办\n\n```\ncode\n```"
	link := ""
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		src := src
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				md := Markdown(c, src)
				if u := md.ClickedLink(); u != "" {
					link = u
				}
				if i, ok := md.TaskToggled(); ok {
					src = ToggleMarkdownTask(src, i)
				}
			}, layout.Pad(20), layout.W(500))
		}, DriverOptions{Width: 600, Height: 500, Theme: th})
		d.Frame()
		d.Frame()
		// Click the link via the recorded link rect.
		var lr *moreRichLink
		for _, e := range d.r.c.states {
			if st, ok := e.v.(*moreRichState); ok && len(st.links) > 0 {
				lr = &st.links[0]
			}
		}
		if lr == nil {
			t.Fatalf("%s: no link drawn", th.Name)
		}
		d.Click(lr.r.Center())
		if link != "https://tango.dev" {
			t.Fatalf("%s: link %q", th.Name, link)
		}
		// Toggle the task checkbox (Tab to it, Space).
		d.r.c.SetFocus(0)
		for i := 0; i < 4 && !strings.Contains(src, "[x]"); i++ {
			d.Key(input.KeyTab, 0)
			d.Key(input.KeySpace, 0)
		}
		if !strings.Contains(src, "- [x] 待办") {
			t.Fatalf("%s: task not toggled: %q", th.Name, src)
		}
	}
}
