package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

var msTestOpts = []Option{{Label: "北京", Value: "bj"}, {Label: "上海", Value: "sh"}, {Label: "广州", Value: "gz"}, {Label: "成都", Value: "cd", Disabled: true}, {Label: "杭州", Value: "hz"}}

func TestEntryMultiSelectToggleAndKeyboard(t *testing.T) {
	var vals []string
	var w *MultiSelectW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = MultiSelect(c, &vals, msTestOpts).Placeholder("选择城市").Width(300)
		}, layout.Pad(20))
	})
	d.Frame()
	d.ClickLabel("选择城市")
	if !w.IsOpen() {
		t.Fatal("dropdown not open after click")
	}
	d.ClickLabel("上海")
	if len(vals) != 1 || vals[0] != "sh" || !w.IsOpen() {
		t.Fatalf("vals=%v open=%v", vals, w.IsOpen())
	}
	d.ClickLabel("成都") // disabled
	if len(vals) != 1 {
		t.Fatalf("disabled option selected: %v", vals)
	}
	// Keyboard: active starts at hovered row; move with Down and toggle with Enter.
	d.Move(geom.Pt(500, 380))
	w.st.active = 0
	d.Key(input.KeyDown, 0) // -> 上海
	d.Key(input.KeyEnter, 0)
	if len(vals) != 0 {
		t.Fatalf("enter should untoggle 上海: %v", vals)
	}
	d.Key(input.KeyDown, 0) // -> 广州
	d.Key(input.KeyDown, 0) // skips disabled 成都 -> 杭州
	d.Key(input.KeyEnter, 0)
	if len(vals) != 1 || vals[0] != "hz" {
		t.Fatalf("keyboard toggle: %v", vals)
	}
	d.Key(input.KeyBackspace, 0)
	if len(vals) != 0 {
		t.Fatalf("backspace should remove last: %v", vals)
	}
	d.Click(geom.Pt(580, 390)) // outside
	if w.IsOpen() {
		t.Fatal("outside click did not close")
	}
}

func TestEntryMultiSelectTagsCollapseRemoveClear(t *testing.T) {
	vals := []string{"bj", "sh", "gz", "hz"}
	var w *MultiSelectW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = MultiSelect(c, &vals, msTestOpts).MaxTagCount(2).AllowClear().Width(400)
		}, layout.Pad(20))
	})
	d.Frame()
	if _, ok := d.Find("+ 2 ..."); !ok {
		t.Fatal("collapsed chip missing")
	}
	if _, ok := d.Find("广州"); ok {
		t.Fatal("chip beyond MaxTagCount shown")
	}
	r := mustFind(t, d, "上海")
	d.Click(geom.Pt(r.Right()-8, r.Y+r.H/2))
	if len(vals) != 3 || contains(vals, "sh") {
		t.Fatalf("close icon: %v", vals)
	}
	fr := d.r.c.rects[w.box.n.id]
	d.Move(fr.Center())
	d.Click(geom.Pt(fr.Right()-17, fr.Y+fr.H/2))
	if len(vals) != 0 {
		t.Fatalf("clear: %v", vals)
	}
}

func TestEntryMultiSelectSearchDisabledStatus(t *testing.T) {
	var vals []string
	var w *MultiSelectW
	status := ""
	d := driver(func(c *Context) {
		f := Form(c)
		f.Body(func() {
			f.Item("城市").Error("必选").Body(func() {
				w = MultiSelect(c, &vals, msTestOpts).Placeholder("搜索").Searchable()
				status = w.box.status
			})
		})
		MultiSelect(c, &vals, msTestOpts).Placeholder("禁用").Disabled(true)
	})
	d.Frame()
	if status != "error" {
		t.Fatalf("form status not inherited: %q", status)
	}
	d.ClickLabel("搜索")
	d.Type("广")
	if _, ok := d.Find("北京"); ok {
		t.Fatal("search did not filter")
	}
	d.Key(input.KeyEnter, 0)
	if len(vals) != 1 || vals[0] != "gz" {
		t.Fatalf("search+enter: %v", vals)
	}
	if w.box.st.query != "" {
		t.Fatal("query not reset after pick")
	}
	d.Key(input.KeyEscape, 0)
	d.ClickLabel("禁用")
	if _, ok := d.Find("杭州"); ok {
		t.Fatal("disabled select opened")
	}
}
