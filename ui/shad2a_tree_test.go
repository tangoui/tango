package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

var shadTreeNodes = []TreeSelectNode{
	{Label: "Fruits", Value: "f", Children: []TreeSelectNode{{Label: "Apple", Value: "a"}, {Label: "Pear", Value: "p"}}},
	{Label: "Kale", Value: "k"},
}

func shadTreeThemes(t *testing.T, fn func(t *testing.T, th func() *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{
		{"ant", func() *theme.Theme { return theme.AntDesign() }},
		{"element", func() *theme.Theme { return theme.ElementPlus() }},
		{"shadcn", func() *theme.Theme { return theme.Shadcn() }},
		{"shadcn-dark", func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
	} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th) })
	}
}

func TestShad2aTreeSelectStrategyValue(t *testing.T) {
	shadTreeThemes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := []string{"f", "k"} // a parent value stands for its subtree
		var w *TreeSelectW
		d := fillDriver(th, func(c *Context) {
			w = TreeSelect(c, nil, shadTreeNodes).Multiple(&vals).Checkable().
				ShowCheckedStrategy(ShowParent).StrategyValue(true).DefaultExpandAll().Width(320).Open()
		})
		if got := w.Checked(); len(got) != 2 || got[0] != "f" || got[1] != "k" {
			t.Fatalf("checked %v", got)
		}
		r := mustFind(t, d, "Pear")
		d.Click(geom.Pt(r.Right()-20, r.Y+r.H/2))
		d.Settle()
		if len(vals) != 2 || vals[0] != "a" || vals[1] != "k" {
			t.Fatalf("after unchecking Pear: %v", vals)
		}
		r = mustFind(t, d, "Pear")
		d.Click(geom.Pt(r.Right()-20, r.Y+r.H/2))
		d.Settle()
		if len(vals) != 2 || vals[0] != "f" || vals[1] != "k" {
			t.Fatalf("after rechecking Pear: %v", vals)
		}
		// Without StrategyValue the value keeps the leaves.
		leaves := []string{"a", "p"}
		fillDriver(th, func(c *Context) {
			TreeSelect(c, nil, shadTreeNodes).Multiple(&leaves).Checkable().ShowCheckedStrategy(ShowParent).Width(320)
		})
		if len(leaves) != 2 {
			t.Fatalf("compat value changed: %v", leaves)
		}
	})
}

func TestShad2aTreeSelectExpandedKeys(t *testing.T) {
	shadTreeThemes(t, func(t *testing.T, th func() *theme.Theme) {
		keys := []string{"f"}
		v := ""
		d := fillDriver(th, func(c *Context) {
			TreeSelect(c, &v, shadTreeNodes).ExpandedKeys(&keys).Width(320).Open()
		})
		mustFind(t, d, "Apple")
		keys = nil // the application collapses it
		d.Settle()
		if _, ok := d.Find("Apple"); ok {
			t.Fatal("controlled collapse ignored")
		}
		r := mustFind(t, d, "Fruits") // click the switcher
		d.Click(geom.Pt(r.X+12, r.Y+r.H/2))
		d.Settle()
		if len(keys) != 1 || keys[0] != "f" {
			t.Fatalf("expand not written back: %v", keys)
		}
		mustFind(t, d, "Apple")
	})
}

func TestShad2aCascaderStrategyValueAndLazy(t *testing.T) {
	shadTreeThemes(t, func(t *testing.T, th func() *theme.Theme) {
		multi := [][]string{{"f"}}
		var w *CascaderW
		d := fillDriver(th, func(c *Context) {
			w = Cascader(c, nil, p2bCasTree).Multiple(&multi).StrategyValue(true).Width(400).Open().Expand("v")
		})
		if got := w.CheckedPaths(); len(got) != 1 || casKey(got[0]) != "f" {
			t.Fatalf("checked %v", got)
		}
		r := mustFind(t, d, "Leek")
		d.Click(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
		d.Settle()
		if len(multi) != 2 || casKey(multi[0]) != "f" || casKey(multi[1]) != casKey([]string{"v", "l"}) {
			t.Fatalf("after Leek: %v", multi)
		}

		// A lazy option checked before loading hands its check to its
		// children; it is never reported as a search leaf.
		roots := []CascaderOption{{Label: "Zhejiang", Value: "zj"}, {Label: "Taiwan", Value: "tw", IsLeaf: true}}
		lazyM := [][]string{{"zj"}}
		load := func(path []string, done func([]CascaderOption)) {
			done([]CascaderOption{{Label: "Hangzhou", Value: "hz", IsLeaf: true}, {Label: "Ningbo", Value: "nb", IsLeaf: true}})
		}
		fillDriver(th, func(c *Context) {
			Cascader(c, nil, roots).Multiple(&lazyM).LoadData(load).Width(400).Open().Expand("zj")
		})
		if len(lazyM) != 2 || casKey(lazyM[0]) != casKey([]string{"zj", "hz"}) {
			t.Fatalf("lazy check not inherited: %v", lazyM)
		}
		var cw *CascaderW
		var single []string
		fillDriver(th, func(c *Context) {
			cw = Cascader(c, &single, roots).LoadData(func([]string, func([]CascaderOption)) {}).Searchable()
		})
		if hits := cw.searchResults("zhe"); len(hits) != 0 {
			t.Fatalf("unloaded parent listed as a leaf: %v", hits)
		}
	})
}

func TestShad2aTreeShadcnLook(t *testing.T) {
	th := func() *theme.Theme { return theme.Shadcn() }
	v := "a"
	d := fillDriver(th, func(c *Context) {
		Column(c, func() {
			TreeSelect(c, &v, shadTreeNodes).DefaultExpandAll().Width(320).Open()
		})
	})
	r := mustFind(t, d, "Apple")
	if r.H != shadTreeItemH {
		t.Fatalf("tree item height %v, want %v", r.H, shadTreeItemH)
	}
	var cas []string
	d2 := fillDriver(th, func(c *Context) {
		Cascader(c, &cas, p2bCasTree).Width(300).Open().Expand("f")
	})
	o := mustFind(t, d2, "Apple")
	if o.H != shadTreeItemH || o.W < 128-8 {
		t.Fatalf("cascader item %v", o)
	}
	// Ant keeps its 28px tree rows.
	d3 := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
		TreeSelect(c, &v, shadTreeNodes).DefaultExpandAll().Width(320).Open()
	})
	if r := mustFind(t, d3, "Apple"); r.H != 28 {
		t.Fatalf("ant row %v", r.H)
	}
}

func TestShad2aTreeDefaults(t *testing.T) {
	th := func() *theme.Theme { return theme.Shadcn() }
	var ts *TreeSelectW
	var cs *CascaderW
	v := ""
	var p []string
	fillDriver(th, func(c *Context) {
		ConfigProvider(c).
			Defaults("TreeSelect", Props{"size": "small", "status": "error", "strategyValue": true}).
			Defaults("Cascader", Props{"size": Large, "placeholder": "Pick"}).Body(func() {
			ts = TreeSelect(c, &v, shadTreeNodes)
			cs = Cascader(c, &p, p2bCasTree)
			Cascader(c, &p, p2bCasTree).Size(Small).Placeholder("Explicit")
		})
	})
	if ts.box.size != Small || ts.box.status != "error" || !ts.stratVal {
		t.Fatalf("treeselect defaults: %v %q %v", ts.box.size, ts.box.status, ts.stratVal)
	}
	if cs.box.size != Large || cs.box.placeholder != "Pick" {
		t.Fatalf("cascader defaults: %v %q", cs.box.size, cs.box.placeholder)
	}
}
