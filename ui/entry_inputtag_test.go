package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEntryInputTagAddRemove(t *testing.T) {
	var tags []string
	added := ""
	d := driver(func(c *Context) {
		Column(c, func() {
			w := InputTag(c, &tags).Placeholder("标签").MaxTags(4).Width(500)
			if a := w.Added(); a != "" {
				added = a
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.ClickLabel("标签")
	d.Type("Go")
	d.Key(input.KeyEnter, 0)
	if len(tags) != 1 || tags[0] != "Go" || added != "Go" {
		t.Fatalf("enter add: %v %q", tags, added)
	}
	d.Type("Rust，Zig,Go,")
	if len(tags) != 3 || tags[1] != "Rust" || tags[2] != "Zig" {
		t.Fatalf("comma add / dedupe: %v", tags)
	}
	d.Type("C")
	d.Key(input.KeyEnter, 0)
	d.Type("D")
	d.Key(input.KeyEnter, 0)
	if len(tags) != 4 {
		t.Fatalf("MaxTags: %v", tags)
	}
	d.Key(input.KeyBackspace, 0) // removes "D" text
	d.Key(input.KeyBackspace, 0) // empty: removes last tag
	if len(tags) != 3 {
		t.Fatalf("backspace: %v", tags)
	}
	r := mustFind(t, d, "Rust")
	d.Click(geom.Pt(r.Right()-8, r.Y+r.H/2))
	if len(tags) != 2 || contains(tags, "Rust") {
		t.Fatalf("close: %v", tags)
	}
}

func TestEntryInputTagCollapseDisabled(t *testing.T) {
	tags := []string{"a", "b", "c", "d"}
	d := driver(func(c *Context) {
		InputTag(c, &tags).MaxTagCount(1).Disabled(true)
	})
	d.Frame()
	if _, ok := d.Find("+ 3 ..."); !ok {
		t.Fatal("collapse chip missing")
	}
	r := mustFind(t, d, "a")
	d.Click(geom.Pt(r.Right()-6, r.Y+r.H/2))
	if len(tags) != 4 {
		t.Fatal("disabled tag removed")
	}
}

func TestEntryInputTagSeparators(t *testing.T) {
	var tags, only []string
	d := driver(func(c *Context) {
		Column(c, func() {
			InputTag(c, &tags).Placeholder("custom").Separators(";", " | ").Width(400)
			InputTag(c, &only).Placeholder("enter").Separators().Width(400)
		}, layout.Pad(20), layout.Gap(10))
	})
	d.Frame()
	d.ClickLabel("custom")
	d.Type("a;b | c,d")
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Fatalf("custom separators: %v", tags)
	}
	d.Type(";")
	if len(tags) != 3 || tags[2] != "c,d" {
		t.Fatalf("comma is no separator now: %v", tags)
	}
	d.ClickLabel("enter")
	d.Type("x,y")
	if len(only) != 0 {
		t.Fatalf("Separators() split: %v", only)
	}
	d.Key(input.KeyEnter, 0)
	if len(only) != 1 || only[0] != "x,y" {
		t.Fatalf("enter only: %v", only)
	}
}

func TestSplitTags(t *testing.T) {
	parts, rest := splitTags("a;;b;c", []string{";"})
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b" || rest != "c" {
		t.Fatalf("%q %q", parts, rest)
	}
	// The longer separator wins at the same position.
	parts, rest = splitTags("a--b-", []string{"-", "--"})
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b" || rest != "" {
		t.Fatalf("%q %q", parts, rest)
	}
}

func TestEntryInputTagSaveOnBlur(t *testing.T) {
	var tags, keep []string

	d := driver(func(c *Context) {
		Column(c, func() {
			InputTag(c, &tags).Placeholder("save").SaveOnBlur().Width(400)
			InputTag(c, &keep).Placeholder("keep").Width(400)
			Button(c, "other")
		}, layout.Pad(20), layout.Gap(10))
	})
	d.Frame()
	d.ClickLabel("save")
	d.Type("draft")
	if len(tags) != 0 {
		t.Fatal("added before blur")
	}
	d.ClickLabel("other")
	if len(tags) != 1 || tags[0] != "draft" {
		t.Fatalf("save on blur: %v", tags)
	}
	d.ClickLabel("keep")
	d.Type("typed")
	d.ClickLabel("other")
	if len(keep) != 0 {
		t.Fatalf("default should keep the text: %v", keep)
	}
}

func TestEntryInputTagDragSort(t *testing.T) {
	tags := []string{"one", "two", "three"}
	changed := false
	d := driver(func(c *Context) {
		Column(c, func() {
			if InputTag(c, &tags).Draggable().Width(500).A11yLabel("tags").Changed() {
				changed = true
			}
		}, layout.Pad(20))
	})
	d.Frame()
	one, three := mustFind(t, d, "one"), mustFind(t, d, "three")
	// Drag "one" past the middle of "three": it moves to the end.
	d.Drag(geom.Pt(one.X+6, one.Y+one.H/2), geom.Pt(three.Right()-4, three.Y+three.H/2))
	if !changed || len(tags) != 3 || tags[0] != "two" || tags[1] != "three" || tags[2] != "one" {
		t.Fatalf("drag to end: %v %v", tags, changed)
	}
	// Drag "one" (now last) before "two" (first).
	one, two := mustFind(t, d, "one"), mustFind(t, d, "two")
	d.Drag(geom.Pt(one.X+6, one.Y+one.H/2), geom.Pt(two.X+3, two.Y+two.H/2))
	if tags[0] != "one" || tags[1] != "two" || tags[2] != "three" {
		t.Fatalf("drag to start: %v", tags)
	}
	// Not draggable: nothing moves.
	plain := []string{"a", "b"}
	d2 := driver(func(c *Context) { InputTag(c, &plain).Width(400) })
	d2.Frame()
	a, b := mustFind(t, d2, "a"), mustFind(t, d2, "b")
	d2.Drag(geom.Pt(a.X+4, a.Y+a.H/2), geom.Pt(b.Right()-2, b.Y+b.H/2))
	if plain[0] != "a" {
		t.Fatalf("plain moved: %v", plain)
	}
}
