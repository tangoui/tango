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
