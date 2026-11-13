package ui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func TestMoreMarkdownEdits(t *testing.T) {
	s, a, b := MarkdownWrap("hello world", 6, 11, "**", "**", "x")
	if s != "hello **world**" || a != 8 || b != 13 {
		t.Fatalf("wrap %q %d %d", s, a, b)
	}
	s, a, b = MarkdownWrap(s, a, b, "**", "**", "x")
	if s != "hello world" || a != 6 || b != 11 {
		t.Fatalf("unwrap %q %d %d", s, a, b)
	}
	s, a, b = MarkdownWrap("ab", 1, 1, "[", "](https://)", "链接")
	if s != "a[链接](https://)b" || string([]rune(s)[a:b]) != "链接" {
		t.Fatalf("placeholder %q %d %d", s, a, b)
	}
	s, _, _ = MarkdownLinePrefix("one\ntwo\nthree", 1, 5, "- ")
	if s != "- one\n- two\nthree" {
		t.Fatalf("prefix %q", s)
	}
	s, _, _ = MarkdownLinePrefix(s, 3, 8, "- ")
	if s != "one\ntwo\nthree" {
		t.Fatalf("unprefix %q", s)
	}
	s, a, _ = MarkdownInsertBlock("text", 4, "```\n\n```")
	if s != "text\n\n```\n\n```\n" || []rune(s)[a-1] != '\n' || []rune(s)[a] != '\n' {
		t.Fatalf("block %q caret %d", s, a)
	}
}

func TestMoreMarkdownEditorToolbarModes(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		src := "hello"
		changes := 0
		var w *MarkdownEditorW
		d := NewDriver(func(c *Context) {
			w = MarkdownEditor(c, &src).Height(300)
			if w.Changed() {
				changes++
			}
		}, DriverOptions{Width: 900, Height: 400, Theme: th})
		d.Frame()
		// Focus the source and type.
		d.Click(geom.Pt(300, 250)) // empty area below the source text: caret at end
		ed := d.r.c.focus
		d.Type(" world")
		if src != "hello world" || changes == 0 {
			t.Fatalf("%s: typing %q", th.Name, src)
		}
		// Bold with no selection inserts a placeholder; typing replaces it.
		d.ClickLabel("粗体")
		if src != "hello world**粗体文字**" || d.r.c.focus != ed {
			t.Fatalf("%s: bold %q focus=%v", th.Name, src, d.r.c.focus == ed)
		}
		d.Type("重点")
		if src != "hello world**重点**" {
			t.Fatalf("%s: replace placeholder %q", th.Name, src)
		}
		// Shortcut: Cmd/Ctrl+B at the caret inserts a bold placeholder.
		short := input.ModCtrl
		if runtime.GOOS == "darwin" {
			short = input.ModMeta
		}
		d.Key(input.KeyB, short)
		if src != "hello world**重点**粗体文字****" {
			t.Fatalf("%s: shortcut bold %q", th.Name, src)
		}
		d.Key(input.KeyZ, short) // undo is not ours: must not insert text
		src = "hello world**重点**"
		d.Frame()
		d.ClickLabel("引用")
		if !strings.HasPrefix(src, "> hello") {
			t.Fatalf("%s: quote %q", th.Name, src)
		}
		// Modes.
		d.ClickLabel("预览")
		if w.CurrentMode() != "preview" {
			t.Fatal("preview mode")
		}
		before := src
		d.ClickLabel("粗体") // disabled in preview
		if src != before {
			t.Fatal("toolbar active in preview")
		}
		d.ClickLabel("编辑")
		if w.CurrentMode() != "edit" {
			t.Fatal("edit mode")
		}
	}
}
