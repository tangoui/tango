package ui

import (
	"reflect"
	"sort"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func treeTestNodes() []TreeNode {
	return []TreeNode{
		{Key: "p", Title: "parent", Children: []TreeNode{
			{Key: "a", Title: "child a", Children: []TreeNode{{Key: "a1", Title: "leaf a1"}, {Key: "a2", Title: "leaf a2"}}},
			{Key: "b", Title: "child b"},
		}},
		{Key: "q", Title: "other"},
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestTreeConductCascade(t *testing.T) {
	nodes := treeTestNodes()
	// Checking the parent checks the whole subtree.
	got := treeToggleCheck(nodes, nil, "p", true, false)
	if want := []string{"a", "a1", "a2", "b", "p"}; !reflect.DeepEqual(sorted(got), want) {
		t.Fatalf("check parent = %v", got)
	}
	// Unchecking one leaf un-checks its ancestors; they become half-checked.
	got = treeToggleCheck(nodes, got, "a1", false, false)
	if want := []string{"a2", "b"}; !reflect.DeepEqual(sorted(got), want) {
		t.Fatalf("uncheck leaf = %v", got)
	}
	full, half := treeConduct(nodes, got)
	if full["p"] || full["a"] || !half["p"] || !half["a"] || half["q"] {
		t.Fatalf("full=%v half=%v", full, half)
	}
	// One child checked -> parent half; both children -> parent checked.
	got = treeToggleCheck(nodes, nil, "a1", true, false)
	if _, half := treeConduct(nodes, got); !half["a"] || !half["p"] {
		t.Fatal("parent of one checked child must be half-checked")
	}
	got = treeToggleCheck(nodes, got, "a2", true, false)
	if !reflect.DeepEqual(sorted(got), []string{"a", "a1", "a2"}) {
		t.Fatalf("both children = %v", got)
	}
	// Strict mode does not cascade.
	if got := treeToggleCheck(nodes, nil, "p", true, true); !reflect.DeepEqual(got, []string{"p"}) {
		t.Fatalf("strict = %v", got)
	}
	// Disabled children are skipped by the cascade.
	nodes[0].Children[1].Disabled = true
	got = treeToggleCheck(nodes, nil, "p", true, false)
	if contains(got, "b") || !contains(got, "p") {
		t.Fatalf("disabled cascade = %v", got)
	}
}

func TestTreeMove(t *testing.T) {
	nodes := treeTestNodes()
	keys := func(ns []TreeNode) []string {
		var out []string
		treeWalk(ns, func(n *TreeNode, path []string) { out = append(out, n.Key) })
		return out
	}
	if got := keys(TreeMove(nodes, "q", "a1", TreeDropBefore)); !reflect.DeepEqual(got, []string{"p", "a", "q", "a1", "a2", "b"}) {
		t.Fatalf("before = %v", got)
	}
	if got := keys(TreeMove(nodes, "b", "q", TreeDropInside)); !reflect.DeepEqual(got, []string{"p", "a", "a1", "a2", "q", "b"}) {
		t.Fatalf("inside = %v", got)
	}
	if got := keys(TreeMove(nodes, "a1", "q", TreeDropAfter)); !reflect.DeepEqual(got, []string{"p", "a", "a2", "b", "q", "a1"}) {
		t.Fatalf("after = %v", got)
	}
	// A node cannot move into its own subtree.
	if got := keys(TreeMove(nodes, "p", "a1", TreeDropInside)); !reflect.DeepEqual(got, keys(nodes)) {
		t.Fatalf("into self = %v", got)
	}
	if len(keys(nodes)) != 6 {
		t.Fatal("TreeMove mutated its input")
	}
}

func TestTreeExpandSelectSearch(t *testing.T) {
	nodes := treeTestNodes()
	var exp, sel []string
	kw := ""
	var w *TreeW
	d := driver(func(c *Context) {
		w = Tree(c, nodes).Expanded(&exp).Selected(&sel).Search(kw).Show()
	})
	d.Frame()
	if _, ok := d.Find("child a"); ok {
		t.Fatal("collapsed child visible")
	}
	d.Click(mustFind(t, d, "switch:p").Center())
	if !reflect.DeepEqual(exp, []string{"p"}) {
		t.Fatalf("expanded = %v", exp)
	}
	d.Click(mustFind(t, d, "child b").Center())
	if !reflect.DeepEqual(sel, []string{"b"}) || !w.SelectChanged() && len(sel) == 0 {
		t.Fatalf("selected = %v", sel)
	}
	// Collapse again.
	d.Click(mustFind(t, d, "switch:p").Center())
	if len(exp) != 0 {
		t.Fatalf("expanded after collapse = %v", exp)
	}
	d.Settle()
	if _, ok := d.Find("child a"); ok {
		t.Fatal("collapsed child still visible")
	}
	// Search expands the parents of matches.
	kw = "a2"
	d.Settle()
	if !reflect.DeepEqual(exp, []string{"p", "a"}) {
		t.Fatalf("search expanded = %v", exp)
	}
	if _, ok := d.Find("leaf a2"); !ok {
		t.Fatal("match not visible")
	}
}

func TestTreeCheckboxAndHalf(t *testing.T) {
	nodes := treeTestNodes()
	exp := []string{"p", "a"}
	var chk []string
	var w *TreeW
	d := driver(func(c *Context) {
		w = Tree(c, nodes).Expanded(&exp).Checkable(&chk).Show()
	})
	d.Frame()
	r := mustFind(t, d, "leaf a1")
	// The checkbox sits just left of the title (16px box + 8px gap).
	d.Click(geom.Pt(r.X-16, r.Y+r.H/2))
	if !reflect.DeepEqual(chk, []string{"a1"}) {
		t.Fatalf("checked = %v", chk)
	}
	d.Frame()
	if h := w.HalfChecked(); !reflect.DeepEqual(h, []string{"p", "a"}) {
		t.Fatalf("half = %v", h)
	}
}

func TestTreeLazyLoadAndDrag(t *testing.T) {
	nodes := []TreeNode{{Key: "x", Title: "lazy"}, {Key: "y", Title: "yy"}, {Key: "z", Title: "zz"}}
	var exp []string
	calls := 0
	var dropped [3]any
	d := driver(func(c *Context) {
		w := Tree(c, nodes).Expanded(&exp).Draggable().LoadData(func(k string) bool {
			calls++
			if calls >= 3 {
				nodes[0].Children = []TreeNode{{Key: "x1", Title: "loaded", IsLeaf: true}}
				return false
			}
			return true
		}).Show()
		if a, b, p, ok := w.Dropped(); ok {
			dropped = [3]any{a, b, p}
		}
	})
	d.Frame()
	d.Click(mustFind(t, d, "switch:x").Center())
	for i := 0; i < 5; i++ {
		d.Frame()
	}
	if calls < 3 {
		t.Fatalf("LoadData calls = %d", calls)
	}
	d.Settle()
	if _, ok := d.Find("loaded"); !ok {
		t.Fatal("lazy children not shown")
	}
	// Drag "zz" onto the lower quarter of "yy" (drop after).
	src := mustFind(t, d, "zz")
	dst := mustFind(t, d, "yy")
	d.Inject(input.Pointer{Kind: input.Move, Pos: src.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: src.Center()})
	d.Frame()
	d.Move(geom.Pt(dst.X+10, dst.Y+dst.H/2))
	d.Move(geom.Pt(dst.X+10, dst.Y+dst.H-2))
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(dst.X+10, dst.Y+dst.H-2)})
	d.Frame()
	if dropped != [3]any{"z", "y", TreeDropAfter} {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestTreeKeyboard(t *testing.T) {
	nodes := treeTestNodes()
	var expanded, selected []string
	var w *TreeW
	d := NewDriver(func(c *Context) {
		w = Tree(c, nodes).Expanded(&expanded).Selected(&selected).Show()
	}, DriverOptions{})
	d.Settle()
	key := func(k input.Key) { d.Key(k, 0); d.Settle() }
	key(input.KeyTab) // focus the tree
	key(input.KeyDown)
	if w.st.cursor != "p" {
		t.Fatalf("first Down should land on the first row, cursor=%q", w.st.cursor)
	}
	key(input.KeyRight) // expand p
	if !contains(expanded, "p") {
		t.Fatalf("Right should expand: %v", expanded)
	}
	key(input.KeyRight) // enter first child
	if w.st.cursor != "a" {
		t.Fatalf("Right on an open node should enter its first child, cursor=%q", w.st.cursor)
	}
	key(input.KeyDown)
	if w.st.cursor != "b" {
		t.Fatalf("Down should skip the collapsed subtree, cursor=%q", w.st.cursor)
	}
	key(input.KeyEnter)
	if !reflect.DeepEqual(selected, []string{"b"}) {
		t.Fatalf("Enter should select: %v", selected)
	}
	key(input.KeyLeft) // b is a leaf: back to parent p
	if w.st.cursor != "p" {
		t.Fatalf("Left on a leaf should go to the parent, cursor=%q", w.st.cursor)
	}
	key(input.KeyLeft) // collapse p
	if contains(expanded, "p") {
		t.Fatalf("Left should collapse: %v", expanded)
	}
	key(input.KeyEnd)
	if w.st.cursor != "q" {
		t.Fatalf("End cursor=%q", w.st.cursor)
	}
	key(input.KeyHome)
	if w.st.cursor != "p" {
		t.Fatalf("Home cursor=%q", w.st.cursor)
	}
	key(input.KeyUp) // stays on the first row
	if w.st.cursor != "p" {
		t.Fatalf("Up at top cursor=%q", w.st.cursor)
	}
	key(input.KeySpace) // not checkable: Space selects
	if !reflect.DeepEqual(selected, []string{"p"}) {
		t.Fatalf("Space should select: %v", selected)
	}
}

func TestTreeKeyboardCheck(t *testing.T) {
	nodes := treeTestNodes()
	expanded := []string{"p"}
	var checked []string
	var w *TreeW
	d := NewDriver(func(c *Context) {
		w = Tree(c, nodes).Expanded(&expanded).Checkable(&checked).Show()
	}, DriverOptions{})
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Settle()
	for _, k := range []input.Key{input.KeyDown, input.KeyDown, input.KeySpace} {
		d.Key(k, 0)
		d.Settle()
	}
	if w.st.cursor != "a" || !reflect.DeepEqual(sorted(checked), []string{"a", "a1", "a2"}) {
		t.Fatalf("Space should check the cursor row with cascade: cursor=%q checked=%v", w.st.cursor, checked)
	}
	if !contains(w.HalfChecked(), "p") {
		t.Fatalf("parent should be half-checked: %v", w.HalfChecked())
	}
	d.Key(input.KeySpace, 0)
	d.Settle()
	if len(checked) != 0 {
		t.Fatalf("second Space should uncheck: %v", checked)
	}
}

func TestTreeClickFocusesForKeys(t *testing.T) {
	nodes := treeTestNodes()
	var selected []string
	var w *TreeW
	d := NewDriver(func(c *Context) {
		w = Tree(c, nodes).Selected(&selected).Show()
	}, DriverOptions{})
	d.Settle()
	if err := d.ClickLabel("other"); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyUp, 0)
	d.Settle()
	if w.st.cursor != "p" {
		t.Fatalf("Up after clicking the last row should move to the first, cursor=%q", w.st.cursor)
	}
}
