package ui

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func shortcutMod() input.Modifiers {
	if runtime.GOOS == "darwin" {
		return input.ModMeta
	}
	return input.ModCtrl
}

func TestMarkdownEditorItalicShortcut(t *testing.T) {
	src := "abc"
	d := NewDriver(func(c *Context) { MarkdownEditor(c, &src).Height(300) }, DriverOptions{Width: 900, Height: 400})
	d.Frame()
	d.Click(geom.Pt(300, 250))
	d.Key(input.KeyA, shortcutMod()) // select all
	d.Key(input.KeyI, shortcutMod())
	if src != "*abc*" {
		t.Fatalf("Cmd/Ctrl+I: %q", src)
	}
	d.Key(input.KeyA, shortcutMod())
	d.Key(input.KeyB, shortcutMod())
	if src != "***abc***" {
		t.Fatalf("Cmd/Ctrl+B: %q", src)
	}
}

func TestMarkdownPreviewSelectCopy(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() { Markdown(c, "Hello **world** and *more*") }, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	d.Frame()
	r, ok := d.FindText("Hello")
	if !ok {
		t.Fatal("paragraph not found")
	}
	d.Drag(geom.Pt(r.X+1, r.Y+r.H/2), geom.Pt(r.Right()-1, r.Y+r.H/2))
	d.Key(input.KeyC, shortcutMod())
	if got := d.Clipboard(); !strings.HasPrefix(got, "Hello wor") {
		t.Fatalf("copied %q", got)
	}
}

func TestRichTextItalicRenders(t *testing.T) {
	shot := func(italic bool) []byte {
		d := NewDriver(func(c *Context) {
			Column(c, func() { RichText(c, Span{Text: "Italic 斜体 lllll", Italic: italic}).Size(20) }, layout.Pad(10))
		}, DriverOptions{Width: 300, Height: 60})
		return append([]byte(nil), d.Frame().Pix...)
	}
	if bytes.Equal(shot(false), shot(true)) {
		t.Fatal("italic span rendered upright")
	}
}
