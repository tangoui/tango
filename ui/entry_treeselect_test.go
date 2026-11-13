package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var tsTestTree = []TreeSelectNode{
	{Label: "浙江省", Value: "zj", Children: []TreeSelectNode{
		{Label: "杭州市", Value: "hz", Children: []TreeSelectNode{
			{Label: "西湖区", Value: "xh"}, {Label: "余杭区", Value: "yh"}, {Label: "滨江区", Value: "bj", Disabled: true},
		}},
		{Label: "宁波市", Value: "nb"},
	}},
	{Label: "北京市", Value: "bjs"},
}

func tsDriver(build func(c *Context) *TreeSelectW) (*Driver, **TreeSelectW) {
	var w *TreeSelectW
	d := NewDriver(func(c *Context) {
		Column(c, func() { w = build(c) })
	}, DriverOptions{Width: 600, Height: 500})
	d.Frame()
	return d, &w
}

func TestEntryTreeSelectExpandAndPick(t *testing.T) {
	v := ""
	changed := 0
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		ts := TreeSelect(c, &v, tsTestTree).Width(240)
		if ts.Changed() {
			changed++
		}
		return ts
	})
	d.Click(d.r.c.rects[(*w).n.id].Center())
	if !(*w).IsOpen() {
		t.Fatal("click did not open")
	}
	if _, ok := d.Find("杭州市"); ok {
		t.Fatal("collapsed child visible")
	}
	r := mustFind(t, d, "浙江省")
	d.Click(geom.Pt(r.X+12, r.Y+r.H/2))
	if _, ok := d.Find("杭州市"); !ok || v != "" {
		t.Fatalf("switcher should expand without selecting (v=%q)", v)
	}
	d.ClickLabel("宁波市")
	if v != "nb" || changed != 1 || (*w).IsOpen() {
		t.Fatalf("v=%q changed=%d open=%v", v, changed, (*w).IsOpen())
	}
	// Reopening reveals the selected node; disabled nodes cannot be picked.
	d.Click(d.r.c.rects[(*w).n.id].Center())
	r = mustFind(t, d, "杭州市")
	d.Click(geom.Pt(r.X+30, r.Y+r.H/2))
	d.ClickLabel("滨江区")
	if v != "nb" {
		t.Fatalf("disabled node selected: %q", v)
	}
}

func TestEntryTreeSelectKeyboard(t *testing.T) {
	v := ""
	d, w := tsDriver(func(c *Context) *TreeSelectW { return TreeSelect(c, &v, tsTestTree) })
	d.Click(d.r.c.rects[(*w).n.id].Center()) // open + focus
	d.Key(input.KeyEscape, 0)
	if (*w).IsOpen() {
		t.Fatal("escape did not close")
	}
	d.Key(input.KeyDown, 0) // open, active = 浙江省
	if !(*w).IsOpen() {
		t.Fatal("down did not open")
	}
	d.Key(input.KeyRight, 0) // expand 浙江省
	d.Key(input.KeyRight, 0) // move to 杭州市
	d.Key(input.KeyRight, 0) // expand 杭州市
	d.Key(input.KeyDown, 0)  // 西湖区
	d.Key(input.KeyEnter, 0)
	if v != "xh" {
		t.Fatalf("v = %q", v)
	}
	d.Key(input.KeyDown, 0) // reopen on 西湖区
	d.Key(input.KeyLeft, 0) // to parent 杭州市
	d.Key(input.KeyLeft, 0) // collapse 杭州市
	if _, ok := d.Find("西湖区"); ok {
		t.Fatal("left did not collapse")
	}
	d.Key(input.KeyEnter, 0)
	if v != "hz" {
		t.Fatalf("v = %q", v)
	}
}

func TestEntryTreeSelectCheckable(t *testing.T) {
	v := ""
	var vals []string
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		return TreeSelect(c, &v, tsTestTree).Multiple(&vals).Checkable().DefaultExpandAll().Width(400)
	})
	d.Click(d.r.c.rects[(*w).n.id].Center())
	d.ClickLabel("杭州市")
	if len(vals) != 2 || !contains(vals, "xh") || !contains(vals, "yh") {
		t.Fatalf("parent check should check enabled leaves: %v", vals)
	}
	set := (*w).valueSet()
	if st := (*w).checkState(tsFind(tsTestTree, "zj"), set); st != tsHalf {
		t.Fatalf("province state = %d, want half", st)
	}
	if st := (*w).checkState(tsFind(tsTestTree, "hz"), set); st != tsOn {
		t.Fatalf("city state = %d, want on", st)
	}
	d.ClickLabel("宁波市")
	if st := (*w).checkState(tsFind(tsTestTree, "zj"), (*w).valueSet()); st != tsOn || !(*w).IsOpen() {
		t.Fatalf("province state = %d open=%v", st, (*w).IsOpen())
	}
	d.ClickLabel("浙江省") // fully checked -> uncheck all
	if len(vals) != 0 {
		t.Fatalf("uncheck parent: %v", vals)
	}
}

func TestEntryTreeSelectSearchKeepsAncestors(t *testing.T) {
	v := ""
	d, w := tsDriver(func(c *Context) *TreeSelectW { return TreeSelect(c, &v, tsTestTree).Searchable() })
	d.Click(d.r.c.rects[(*w).n.id].Center())
	d.Type("西湖")
	rows := (*w).rows()
	if len(rows) != 3 || rows[0].nd.Value != "zj" || rows[1].nd.Value != "hz" || rows[2].nd.Value != "xh" || !rows[2].match {
		t.Fatalf("rows = %+v", rows)
	}
	if _, ok := d.Find("北京市"); ok {
		t.Fatal("non-matching node shown")
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if v != "xh" {
		t.Fatalf("v = %q", v)
	}
}

func TestEntryTreeSelectChipsAndDisabled(t *testing.T) {
	v := ""
	vals := []string{"xh", "nb", "bjs"}
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		return TreeSelect(c, &v, tsTestTree).Multiple(&vals).MaxTagCount(2).Width(400)
	})
	if _, ok := d.Find("+ 1 ..."); !ok {
		t.Fatal("collapsed chip missing")
	}
	r := mustFind(t, d, "西湖区")
	d.Click(geom.Pt(r.Right()-10, r.Y+r.H/2))
	if len(vals) != 2 || vals[0] != "nb" {
		t.Fatalf("chip close: %v", vals)
	}
	_ = w
	d2, w2 := tsDriver(func(c *Context) *TreeSelectW { return TreeSelect(c, &v, tsTestTree).Disabled(true) })
	d2.Click(d2.r.c.rects[(*w2).n.id].Center())
	if (*w2).IsOpen() {
		t.Fatal("disabled tree select opened")
	}
}

// TreeSelect shares the selector shell: Backspace on an empty search
// removes the last tag like MultiSelect.
func TestEntryTreeSelectBackspaceRemovesTag(t *testing.T) {
	v := ""
	vals := []string{"xh", "nb"}
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		return TreeSelect(c, &v, tsTestTree).Multiple(&vals).Searchable().Width(400)
	})
	d.Click(d.r.c.rects[(*w).n.id].Center())
	if !(*w).IsOpen() {
		t.Fatal("click did not open")
	}
	d.Key(input.KeyBackspace, 0)
	if len(vals) != 1 || vals[0] != "xh" {
		t.Fatalf("backspace: %v", vals)
	}
	d.Type("宁")
	d.Key(input.KeyBackspace, 0) // deletes the typed text, not a tag
	if len(vals) != 1 {
		t.Fatalf("backspace while typing removed a tag: %v", vals)
	}
}
