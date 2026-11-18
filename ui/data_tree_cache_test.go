package ui

// Final review R65: Tree.DataVersion caches a Height tree's walk and
// flattened rows. The cached tree must draw, number (a11y) and navigate
// exactly like the uncached one; in-place structural edits show after a
// DataVersion change.

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// treeCacheNodes is groups × kids: groups "g<i>" titled "group i", leaves
// "g<i>-<k>" titled "leaf i-k".
func treeCacheNodes(groups, kids int) []TreeNode {
	out := make([]TreeNode, groups)
	for g := range out {
		ch := make([]TreeNode, kids)
		for k := range ch {
			ch[k] = TreeNode{Key: "g" + strconv.Itoa(g) + "-" + strconv.Itoa(k), Title: "leaf " + strconv.Itoa(g) + "-" + strconv.Itoa(k)}
		}
		out[g] = TreeNode{Key: "g" + strconv.Itoa(g), Title: "group " + strconv.Itoa(g), Children: ch}
	}
	return out
}

// treeCacheSnap is what a screen reader and the eye get from the tree:
// every built tree item (name, level, position, set size, states), the
// tree's active descendant, the built count and the pixels.
func treeCacheSnap(d *Driver, w *TreeW) (string, []byte) {
	t := d.AccessibilityTree()
	var b bytes.Buffer
	for _, n := range t.FindAll(a11y.RoleTreeItem, "") {
		fmt.Fprintf(&b, "%s L%d %d/%d s%d b%v\n", n.Name, n.Level, n.PosInSet, n.SetSize, n.States, n.Bounds)
	}
	if tn := t.Node(a11y.NodeID(w.box.n.id)); tn != nil && tn.ActiveDescendant != 0 {
		if ad := t.Node(tn.ActiveDescendant); ad != nil {
			fmt.Fprintf(&b, "active %s\n", ad.Name)
		}
	}
	fmt.Fprintf(&b, "built %d half %v", w.Built(), w.HalfChecked())
	return b.String(), d.Image().Pix
}

// TestTreeDataVersionIdentical runs the same script on a Height tree with
// and without DataVersion: scrolling, selecting, keyboard moves across the
// window, expanding and collapsing, checking (cascading half states) and
// filtering must give the same a11y tree and pixels after every step.
func TestTreeDataVersionIdentical(t *testing.T) {
	type twin struct {
		d   *Driver
		w   *TreeW
		exp []string
		chk []string
		sel []string
		q   string
	}
	mk := func(cached bool) *twin {
		tw := &twin{exp: []string{"g0", "g2", "g3", "g7"}}
		nodes := treeCacheNodes(30, 20)
		tw.d = NewDriver(func(c *Context) {
			Column(c, func() {
				tr := Tree(c, nodes).Expanded(&tw.exp).Checkable(&tw.chk).Selected(&tw.sel).Height(300).ShowLine()
				if tw.q != "" {
					tr.Search(tw.q).Filter()
				}
				if cached {
					tr.DataVersion(1)
				}
				tw.w = tr.Show()
			}, layout.W(360), layout.Pad(10))
		}, DriverOptions{Width: 400, Height: 360})
		tw.d.EnableAccessibility()
		tw.d.Settle()
		return tw
	}
	plain, cached := mk(false), mk(true)
	if !cached.w.st.vc.rowsOK || plain.w.st.vc.dataOK {
		t.Fatal("the cache should run with DataVersion only")
	}
	both := func(fn func(tw *twin)) { fn(plain); fn(cached) }
	steps := []struct {
		name string
		do   func(tw *twin)
	}{
		{"initial", func(*twin) {}},
		{"click leaf", func(tw *twin) { tw.d.Click(mustFind(t, tw.d, "leaf 0-3").Center()) }},
		{"down ×30 (across the window)", func(tw *twin) {
			for range 30 {
				tw.d.Key(input.KeyDown, 0)
			}
		}},
		{"end", func(tw *twin) { tw.d.Key(input.KeyEnd, 0) }},
		{"home", func(tw *twin) { tw.d.Key(input.KeyHome, 0) }},
		{"collapse g0 (left)", func(tw *twin) { tw.d.Key(input.KeyLeft, 0) }},
		{"down to g1, expand (right)", func(tw *twin) {
			tw.d.Key(input.KeyDown, 0)
			tw.d.Key(input.KeyRight, 0)
		}},
		{"enter child (right)", func(tw *twin) { tw.d.Key(input.KeyRight, 0) }},
		{"check leaf (space)", func(tw *twin) { tw.d.Key(input.KeySpace, 0) }},
		{"check parent (left, space)", func(tw *twin) {
			tw.d.Key(input.KeyLeft, 0)
			tw.d.Key(input.KeySpace, 0)
		}},
		{"scroll", func(tw *twin) { tw.d.Scroll(geom.Pt(150, 150), 400) }},
		{"bound expanded edited by the app", func(tw *twin) {
			tw.exp = append(tw.exp, "g12", "g13")
			tw.d.Settle()
		}},
		{"filter", func(tw *twin) { tw.q = "leaf 1"; tw.d.Settle() }},
		{"filter changed", func(tw *twin) { tw.q = "leaf 12-1"; tw.d.Settle() }},
		{"filter off", func(tw *twin) { tw.q = ""; tw.d.Settle() }},
		{"scroll back", func(tw *twin) { tw.d.Scroll(geom.Pt(150, 150), -2000) }},
	}
	for _, s := range steps {
		both(s.do)
		pa, pp := treeCacheSnap(plain.d, plain.w)
		ca, cp := treeCacheSnap(cached.d, cached.w)
		if pa != ca {
			t.Fatalf("%s: a11y differs\nwithout DataVersion:\n%s\nwith:\n%s", s.name, pa, ca)
		}
		if !bytes.Equal(pp, cp) {
			t.Fatalf("%s: pixels differ", s.name)
		}
		if fmt.Sprint(plain.exp, plain.chk, plain.sel) != fmt.Sprint(cached.exp, cached.chk, cached.sel) {
			t.Fatalf("%s: state differs: %v %v %v vs %v %v %v", s.name, plain.exp, plain.chk, plain.sel, cached.exp, cached.chk, cached.sel)
		}
	}
	if len(cached.chk) == 0 {
		t.Fatal("the script checked nothing")
	}
}

// TestTreeDataVersionInPlace: titles are read live; an in-place structural
// edit (a child added to a node of the same slice) shows only after a
// DataVersion change, and the steady state reuses the flattened rows.
func TestTreeDataVersionInPlace(t *testing.T) {
	nodes := treeCacheNodes(40, 3)
	exp := []string{"g0"}
	ver := 0
	var w *TreeW
	d := NewDriver(func(c *Context) {
		Column(c, func() { w = Tree(c, nodes).Expanded(&exp).Height(300).DataVersion(ver).Show() }, layout.W(360))
	}, DriverOptions{Width: 400, Height: 360})
	d.Settle()
	rows := &w.st.vc.rows[0]
	d.Frame()
	d.Frame()
	if &w.st.vc.rows[0] != rows {
		t.Fatal("steady frames flattened the rows again")
	}
	nodes[0].Children[1].Title = "renamed leaf"
	d.Frame()
	if _, ok := d.Find("renamed leaf"); !ok {
		t.Fatal("an in-place title edit is not shown (titles are read live)")
	}
	nodes[0].Children = append(nodes[0].Children, TreeNode{Key: "g0-new", Title: "added leaf"})
	d.Frame()
	if _, ok := d.Find("added leaf"); ok {
		t.Fatal("without a DataVersion change the cached rows changed (documented: they stay)")
	}
	ver++
	d.Frame()
	if _, ok := d.Find("added leaf"); !ok {
		t.Fatal("DataVersion change: the child added in place is not shown")
	}
	// A new slice needs no DataVersion change.
	nodes = append([]TreeNode{{Key: "top", Title: "new top"}}, nodes...)
	d.Frame()
	if _, ok := d.Find("new top"); !ok {
		t.Fatal("a new nodes slice is not shown")
	}
}

// TestTreeDataVersionFilterRename: under Filter, a title renamed in place
// to match joins the filtered rows after a DataVersion change.
func TestTreeDataVersionFilterRename(t *testing.T) {
	nodes := treeCacheNodes(40, 10)
	exp := []string{}
	ver := 0
	d := NewDriver(func(c *Context) {
		Column(c, func() { Tree(c, nodes).Expanded(&exp).Search("leaf 3-").Filter().Height(300).DataVersion(ver).Show() }, layout.W(360))
	}, DriverOptions{Width: 400, Height: 360})
	d.Settle()
	if _, ok := d.Find("leaf 3-0"); !ok {
		t.Fatal("filtered match not shown")
	}
	exp = append(exp, "g1") // g1 has no match yet: still filtered out
	d.Frame()
	if _, ok := d.Find("group 1"); ok {
		t.Fatal("group 1 shown under the filter")
	}
	nodes[1].Children[0].Title = "leaf 3-x renamed"
	d.Frame()
	if _, ok := d.Find("leaf 3-x renamed"); ok {
		t.Fatal("without a DataVersion change the filter changed (documented: it stays)")
	}
	ver++
	d.Frame()
	if _, ok := d.Find("leaf 3-x renamed"); !ok {
		t.Fatal("DataVersion change: the renamed match did not join the filtered rows")
	}
}

// TestTreeDataVersionNeedsHeight: without Height, DataVersion changes
// nothing (no cache).
func TestTreeDataVersionNeedsHeight(t *testing.T) {
	nodes := treeCacheNodes(3, 3)
	var w *TreeW
	d := driver(func(c *Context) { w = Tree(c, nodes).DataVersion(1).Show() })
	d.Settle()
	if w.st.vc.dataOK || w.index != nil {
		t.Fatal("DataVersion without Height used the cache")
	}
}
