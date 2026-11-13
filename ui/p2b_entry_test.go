package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func TestP2BCascaderLazyLoad(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		roots := []CascaderOption{{Label: "Zhejiang", Value: "zj"}, {Label: "Tibet", Value: "xz", IsLeaf: true}}
		var val []string
		var calls [][]string
		var pending func([]CascaderOption)
		var w *CascaderW
		d := fillDriver(th, func(c *Context) {
			w = Cascader(c, &val, roots).Placeholder("region").Width(220).
				LoadData(func(path []string, done func([]CascaderOption)) {
					calls = append(calls, path)
					if len(path) == 1 {
						pending = done // answered later (async)
						return
					}
					done(nil) // a node without children becomes a leaf
				})
		})
		d.EnableAccessibility()
		d.ClickLabel("region")
		d.ClickLabel("Zhejiang")
		if len(calls) != 1 || calls[0][0] != "zj" {
			t.Fatalf("calls %v", calls)
		}
		if n, ok := d.A11yFind(a11y.RoleOption, "Zhejiang"); !ok || !n.States.Has(a11y.Busy) {
			t.Fatalf("loading option should be busy: %+v", n)
		}
		// The answer arrives from another goroutine.
		ch := make(chan struct{})
		go func() { pending([]CascaderOption{{Label: "Hangzhou", Value: "hz"}}); close(ch) }()
		<-ch
		d.Advance(60 * time.Millisecond)
		d.Settle()
		if _, ok := d.Find("Hangzhou"); !ok {
			t.Fatal("loaded children not shown")
		}
		d.ClickLabel("Hangzhou") // loads (empty) and is then a leaf: selects
		d.Settle()
		if len(calls) != 2 {
			t.Fatalf("second load %v", calls)
		}
		d.ClickLabel("Hangzhou")
		if casKey(val) != casKey([]string{"zj", "hz"}) {
			t.Fatalf("val %v", val)
		}
		if w.box.single != "Zhejiang / Hangzhou" {
			t.Fatalf("display %q", w.box.single)
		}
		// IsLeaf nodes never load.
		d.ClickLabel("region")
		d.ClickLabel("Tibet")
		if len(calls) != 2 || casKey(val) != "xz" {
			t.Fatalf("leaf: calls %v val %v", calls, val)
		}
	})
}

var p2bCasTree = []CascaderOption{
	{Label: "Fruit", Value: "f", Children: []CascaderOption{{Label: "Apple", Value: "a"}, {Label: "Pear", Value: "p"}}},
	{Label: "Veg", Value: "v", Children: []CascaderOption{{Label: "Leek", Value: "l"}, {Label: "Kale", Value: "k"}}},
}

func TestP2BCascaderCheckedStrategy(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		for _, tc := range []struct {
			s    CheckedStrategy
			tags int
		}{{ShowParent, 2}, {ShowChild, 3}, {ShowAll, 4}} {
			multi := [][]string{{"f", "a"}, {"f", "p"}, {"v", "l"}}
			var w *CascaderW
			fillDriver(th, func(c *Context) {
				w = Cascader(c, nil, p2bCasTree).Multiple(&multi).ShowCheckedStrategy(tc.s).Width(400)
			})
			got := w.CheckedPaths()
			if len(got) != tc.tags {
				t.Fatalf("strategy %d: paths %v", tc.s, got)
			}
			if tc.s == ShowParent && casKey(got[0]) != "f" {
				t.Fatalf("parent: %v", got)
			}
		}
		// Removing a parent tag unchecks its subtree.
		multi := [][]string{{"f", "a"}, {"f", "p"}}
		d := fillDriver(th, func(c *Context) {
			Cascader(c, nil, p2bCasTree).Multiple(&multi).ShowCheckedStrategy(ShowAll).Width(400)
		})
		r, ok := d.Find("Fruit")
		if !ok {
			t.Fatal("parent tag missing")
		}
		d.Click(geom.Pt(r.Right()-10, r.Y+r.H/2))
		if len(multi) != 0 {
			t.Fatalf("after remove %v", multi)
		}
	})
}

func p2bBigTree(n int) []TreeSelectNode {
	out := make([]TreeSelectNode, 0, 10)
	for g := 0; g < 10; g++ {
		kids := make([]TreeSelectNode, n/10)
		for i := range kids {
			v := fmt.Sprintf("n%d-%d", g, i)
			kids[i] = TreeSelectNode{Label: "Node " + v, Value: v}
		}
		out = append(out, TreeSelectNode{Label: fmt.Sprintf("Group %d", g), Value: fmt.Sprint("g", g), Children: kids})
	}
	return out
}

func TestP2BTreeSelectVirtualLazyStrategy(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		// Virtual dropdown over 5000 expanded nodes.
		nodes := p2bBigTree(5000)
		v := ""
		var w *TreeSelectW
		d := fillDriver(th, func(c *Context) {
			w = TreeSelect(c, &v, nodes).DefaultExpandAll().Placeholder("tree").Width(260)
		})
		d.ClickLabel("tree")
		if _, ok := d.Find("Node n0-1"); !ok {
			t.Fatal("first rows missing")
		}
		if _, ok := d.Find("Node n0-40"); ok {
			t.Fatal("rows far below the window are built")
		}
		d.Key(input.KeyEnd, 0)
		d.Settle()
		if _, ok := d.Find("Node n9-499"); !ok {
			t.Fatal("End did not reveal the last row")
		}
		d.Key(input.KeyEnter, 0)
		if v != "n9-499" || w.IsOpen() {
			t.Fatalf("pick %q", v)
		}

		// Lazy loading with Tree's polling contract.
		lazy := []TreeSelectNode{{Label: "Remote", Value: "r"}, {Label: "Leaf", Value: "x", IsLeaf: true}}
		polls := 0
		d2 := fillDriver(th, func(c *Context) {
			TreeSelect(c, &v, lazy).Placeholder("lazy").Width(260).LoadData(func(val string) bool {
				polls++
				if polls < 3 {
					return true // still loading
				}
				lazy[0].Children = []TreeSelectNode{{Label: "Child", Value: "r1"}}
				return false
			})
		})
		d2.EnableAccessibility()
		d2.ClickLabel("lazy")
		r := mustFind(t, d2, "Remote")
		d2.Click(geom.Pt(r.X+12, r.Y+r.H/2))
		if n, ok := d2.A11yFind(a11y.RoleTreeItem, "Remote"); !ok || !n.States.Has(a11y.Busy) {
			t.Fatalf("loading node should be busy: %+v", n)
		}
		for i := 0; i < 4; i++ {
			d2.Advance(60 * time.Millisecond)
			d2.Settle()
		}
		if _, ok := d2.Find("Child"); !ok {
			t.Fatalf("loaded child missing (polls %d)", polls)
		}
		if n, ok := d2.A11yFind(a11y.RoleTreeItem, "Leaf"); !ok || n.States.Has(a11y.Expandable) {
			t.Fatalf("IsLeaf node expandable: %+v", n)
		}

		// Checked strategy: ShowParent folds a fully checked group.
		small := []TreeSelectNode{{Label: "Fruits", Value: "f", Children: []TreeSelectNode{{Label: "Apple", Value: "a"}, {Label: "Pear", Value: "p"}}}, {Label: "Kale", Value: "k"}}
		var vals = []string{"a", "p", "k"}
		var ts *TreeSelectW
		d3 := fillDriver(th, func(c *Context) {
			ts = TreeSelect(c, nil, small).Checkable().Multiple(&vals).ShowCheckedStrategy(ShowParent).Width(320)
		})
		if got := ts.Checked(); len(got) != 2 || got[0] != "f" || got[1] != "k" {
			t.Fatalf("show parent %v", got)
		}
		tag := mustFind(t, d3, "Fruits")
		d3.Click(geom.Pt(tag.Right()-10, tag.Y+tag.H/2))
		if len(vals) != 1 || vals[0] != "k" {
			t.Fatalf("remove parent tag: %v", vals)
		}
	})
}

func p2bItems(n int) []TransferItem {
	out := make([]TransferItem, n)
	for i := range out {
		out[i] = TransferItem{Key: fmt.Sprint(i), Title: fmt.Sprintf("Item %04d", i)}
	}
	return out
}

func TestP2BTransferPaginationRenderOneWay(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		items := p2bItems(25)
		var target []string
		d := fillDriver(th, func(c *Context) {
			Transfer(c, items, &target).Operations("R", "L").Pagination(10).OneWay().Render(func(o Option) {
				Text(c, "#"+o.Label)
			})
		})
		if _, ok := d.FindText("#Item 0004"); !ok {
			t.Fatal("custom row missing")
		}
		if _, ok := d.FindText("#Item 0010"); ok {
			t.Fatal("second page shown on the first")
		}
		// The pager is a simple Pagination: "[1] / 3".
		if _, ok := d.FindText("/"); !ok {
			t.Fatal("pager missing")
		}
		if _, ok := d.FindText("3"); !ok {
			t.Fatal("page count missing")
		}
		d.ClickLabel(ZhCN.NextPage)
		if _, ok := d.FindText("#Item 0010"); !ok {
			t.Fatal("next page not shown")
		}
		// One way: only the move-right button, right items are removable.
		d.ClickLabel("Item 0012")
		d.ClickLabel("R")
		if len(target) != 1 || target[0] != "12" {
			t.Fatalf("target %v", target)
		}
		if _, ok := d.Find("L"); ok {
			t.Fatal("one-way transfer has a move-left button")
		}
		d.ClickLabel(ZhCN.RemoveItem("Item 0012"))
		if len(target) != 0 {
			t.Fatalf("remove: %v", target)
		}
	})
}

func TestP2BTransferTreeSource(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		tree := []TreeNode{
			{Key: "fe", Title: "Frontend", Children: []TreeNode{{Key: "vue", Title: "Vue"}, {Key: "react", Title: "React"}}},
			{Key: "go", Title: "Go"},
		}
		target := []string{"go"}
		var w *TransferW
		d := fillDriver(th, func(c *Context) {
			w = Transfer(c, nil, &target).Operations("R", "L").TreeSource(tree)
		})
		d.ClickLabel("Vue")
		if got := w.Checked(0); len(got) != 1 || got[0] != "vue" {
			t.Fatalf("checked %v", got)
		}
		d.ClickLabel("R")
		if len(target) != 2 || target[1] != "vue" {
			t.Fatalf("target %v", target)
		}
		// Moved leaves stay in the tree, checked and disabled; the right
		// list holds them.
		d.ClickLabel("Vue")
		if len(w.Checked(0)) != 0 {
			t.Fatal("moved leaf can be checked again")
		}
	})
}

func TestP2BTransferVirtual(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		items := p2bItems(5000)
		var target []string
		var w *TransferW
		d := fillDriver(th, func(c *Context) { w = Transfer(c, items, &target) })
		if _, ok := d.Find("Item 0001"); !ok {
			t.Fatal("first rows missing")
		}
		if _, ok := d.Find("Item 0100"); ok {
			t.Fatal("rows below the window are built")
		}
		d.ClickLabel("Item 0001")
		d.Key(input.KeyEnd, 0)
		d.Settle()
		if _, ok := d.Find("Item 4999"); !ok {
			t.Fatal("End did not reveal the last item")
		}
		d.Key(input.KeySpace, 0)
		d.Settle()
		if got := w.Checked(0); len(got) != 2 || got[1] != "4999" {
			t.Fatalf("checked %v", got)
		}
	})
}

func TestP2BConfigDefaultsTreeCascaderTransfer(t *testing.T) {
	v := ""
	var cv []string
	var target []string
	var ts *TreeSelectW
	var cs *CascaderW
	var tf *TransferW
	fillDriver(func() *theme.Theme { return theme.ElementPlus() }, func(c *Context) {
		ConfigProvider(c).
			Defaults("TreeSelect", Props{"allowClear": true, "showCheckedStrategy": "parent"}).
			Defaults("Cascader", Props{"expandTrigger": "hover", "showCheckedStrategy": ShowChild}).
			Defaults("Transfer", Props{"pageSize": 5, "oneWay": true, "showSearch": true}).
			Body(func() {
				ts = TreeSelect(c, &v, nil)
				cs = Cascader(c, &cv, nil)
				tf = Transfer(c, p2bItems(3), &target)
			})
	})
	if !ts.box.allowClear || ts.strategy != ShowParent {
		t.Fatal("tree select defaults")
	}
	if !cs.hoverExpand || cs.strategy != ShowChild {
		t.Fatal("cascader defaults")
	}
	if tf.pageSize != 5 || !tf.oneWay || !tf.search {
		t.Fatal("transfer defaults")
	}
}
