package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEntryMentionsTriggerInsert(t *testing.T) {
	val := ""
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Mentions(c, &val, Opts("Lucy", "Lily", "李雷")).Prefix("@", "#").Width(300)
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(geom.Pt(60, 36))
	d.Type("hi")
	if w.IsOpen() {
		t.Fatal("open without trigger")
	}
	d.Type(" @l")
	if !w.IsOpen() {
		t.Fatal("trigger did not open")
	}
	if _, ok := d.Find("李雷"); ok {
		t.Fatal("not filtered")
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if val != "hi @Lily " || w.IsOpen() {
		t.Fatalf("insert: %q open=%v", val, w.IsOpen())
	}
	if w.st.edit.cur != len([]rune(val)) {
		t.Fatalf("caret %d", w.st.edit.cur)
	}
	d.Type("#")
	if !w.IsOpen() {
		t.Fatal("# trigger")
	}
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() {
		t.Fatal("escape")
	}
	d.Type("李")
	if w.IsOpen() {
		t.Fatal("dismissed trigger reopened")
	}
	d.Type(" @")
	d.ClickLabel("李雷")
	if val != "hi @Lily #李 @李雷 " {
		t.Fatalf("click insert: %q", val)
	}
	d.Type("a@b")
	if w.IsOpen() {
		t.Fatal("@ inside a word must not trigger")
	}
}

func TestEntryMentionsDisabled(t *testing.T) {
	val := "@"
	var w *MentionsW
	d := driver(func(c *Context) { w = Mentions(c, &val, Opts("a")).Disabled(true) })
	d.Frame()
	d.Click(geom.Pt(20, 16))
	d.Type("x")
	if val != "@" || w.IsOpen() {
		t.Fatalf("disabled edited: %q", val)
	}
}
