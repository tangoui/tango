package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

var casTestOpts = []CascaderOption{
	{Label: "浙江", Value: "zj", Children: []CascaderOption{
		{Label: "杭州", Value: "hz", Children: []CascaderOption{{Label: "西湖", Value: "xh"}, {Label: "滨江", Value: "bj"}}},
		{Label: "宁波", Value: "nb"},
		{Label: "温州", Value: "wz", Disabled: true},
	}},
	{Label: "江苏", Value: "js", Children: []CascaderOption{{Label: "南京", Value: "nj"}}},
}

func TestEntryCascaderClickPath(t *testing.T) {
	var val []string
	var w *CascaderW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Cascader(c, &val, casTestOpts).Placeholder("地区").Width(200)
		}, layout.Pad(20))
	})
	d.Frame()
	d.ClickLabel("地区")
	if !w.IsOpen() {
		t.Fatal("not open")
	}
	d.ClickLabel("浙江")
	d.ClickLabel("杭州")
	if len(val) != 0 {
		t.Fatal("non-leaf selected without ChangeOnSelect")
	}
	d.ClickLabel("滨江")
	if casKey(val) != casKey([]string{"zj", "hz", "bj"}) || w.IsOpen() {
		t.Fatalf("val=%v open=%v", val, w.IsOpen())
	}
	if w.box.single != "浙江 / 杭州 / 滨江" {
		t.Fatalf("display %q", w.box.single)
	}
	// Reopen: expanded path follows the value.
	d.ClickLabel("地区")
	if _, ok := d.Find("西湖"); !ok {
		t.Fatal("reopen should expand to value")
	}
	d.Click(geom.Pt(590, 390))
	if w.IsOpen() {
		t.Fatal("outside click")
	}
}

func TestEntryCascaderKeyboard(t *testing.T) {
	var val []string
	var w *CascaderW
	d := driver(func(c *Context) {
		w = Cascader(c, &val, casTestOpts).Placeholder("地区").Width(200)
	})
	d.Frame()
	d.ClickLabel("地区")
	d.Key(input.KeyDown, 0) // 浙江
	d.Key(input.KeyRight, 0)
	if w.st.kcol != 1 || casKey(w.st.path) != casKey([]string{"zj", "hz"}) {
		t.Fatalf("right: %v col %d", w.st.path, w.st.kcol)
	}
	d.Key(input.KeyDown, 0) // 宁波
	d.Key(input.KeyDown, 0) // skips 温州 -> 杭州
	if w.st.path[1] != "hz" {
		t.Fatalf("down wrap/skip: %v", w.st.path)
	}
	d.Key(input.KeyDown, 0) // 宁波 (leaf)
	d.Key(input.KeyEnter, 0)
	if casKey(val) != casKey([]string{"zj", "nb"}) {
		t.Fatalf("enter: %v", val)
	}
	d.Key(input.KeyDown, 0) // reopen
	d.Key(input.KeyLeft, 0)
	if w.st.kcol != 0 {
		t.Fatal("left")
	}
}

func TestEntryCascaderMultipleHalfCheck(t *testing.T) {
	var val []string
	var multi [][]string
	var w *CascaderW
	d := driver(func(c *Context) {
		w = Cascader(c, &val, casTestOpts).Multiple(&multi).Placeholder("多选").Width(300)
	})
	d.Frame()
	d.ClickLabel("多选")
	d.ClickLabel("浙江") // label click on non-leaf expands
	if len(multi) != 0 {
		t.Fatal("expanding should not check")
	}
	d.ClickLabel("宁波") // leaf toggles
	if len(multi) != 1 {
		t.Fatalf("leaf: %v", multi)
	}
	all, some := casCheckState(casTestOpts[0], []string{"zj"}, w.checked())
	if all || !some {
		t.Fatal("parent should be half checked")
	}
	r := mustFind(t, d, "杭州")
	d.Click(geom.Pt(r.X+14, r.Y+r.H/2)) // checkbox
	if len(multi) != 3 {
		t.Fatalf("subtree check: %v", multi)
	}
	all, _ = casCheckState(casTestOpts[0], []string{"zj"}, w.checked())
	if !all {
		t.Fatal("all enabled leaves checked -> parent checked (disabled ignored)")
	}
	if _, ok := d.Find("浙江"); !ok {
		t.Fatal("collapsed chip missing")
	}
	labels, _ := w.chips()
	if len(labels) != 1 || labels[0] != "浙江" {
		t.Fatalf("chips: %v", labels)
	}
}

func TestEntryCascaderSearch(t *testing.T) {
	var val []string
	d := driver(func(c *Context) {
		Cascader(c, &val, casTestOpts).Placeholder("搜索").Searchable().Width(300)
	})
	d.Frame()
	d.ClickLabel("搜索")
	d.Type("京")
	if _, ok := d.Find("江苏 / 南京"); !ok {
		t.Fatal("flattened result missing")
	}
	if _, ok := d.Find("浙江 / 宁波"); ok {
		t.Fatal("search did not filter")
	}
	d.Key(input.KeyEnter, 0)
	if casKey(val) != casKey([]string{"js", "nj"}) {
		t.Fatalf("search pick: %v", val)
	}
}
