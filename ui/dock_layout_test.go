package ui

import (
	"encoding/json"
	"reflect"
	"testing"
)

// dockShape renders a layout compactly: groups as [a b*] (active starred),
// splits as row(...) / column(...).
func dockShape(n *DockNode) string {
	if n == nil {
		return "nil"
	}
	if n.IsGroup() {
		s := n.ID + "["
		for i, k := range n.Tabs {
			if i > 0 {
				s += " "
			}
			s += k
			if k == n.Active {
				s += "*"
			}
		}
		return s + "]"
	}
	s := string(n.Axis) + "("
	for i, k := range n.Children {
		if i > 0 {
			s += " "
		}
		s += dockShape(k)
	}
	return s + ")"
}

func TestDockLayoutSplitMoveRemove(t *testing.T) {
	l := NewDockLayout("a", "b", "c")
	if got := dockShape(l.Root); got != "g1[a* b c]" {
		t.Fatalf("new: %s", got)
	}
	id, ok := l.SplitTab("c", "g1", DockRight)
	if !ok || id != "g2" || l.Focus != "g2" {
		t.Fatalf("split: %q %v focus %q", id, ok, l.Focus)
	}
	if got := dockShape(l.Root); got != "row(g1[a* b] g2[c*])" {
		t.Fatalf("split right: %s", got)
	}
	// Same axis: inserted beside the target, halving its share.
	l.SplitTab("b", "g2", DockRight)
	if got := dockShape(l.Root); got != "row(g1[a*] g2[c*] g3[b*])" {
		t.Fatalf("split right again: %s", got)
	}
	if !reflect.DeepEqual(l.Root.Sizes, []float64{0.5, 0.25, 0.25}) {
		t.Fatalf("sizes %v", l.Root.Sizes)
	}
	// Other axis: the target becomes a column.
	l.SplitTab("a", "g3", DockTop)
	if got := dockShape(l.Root); got != "row(g2[c*] column(g4[a*] g3[b*]))" {
		t.Fatalf("split top: %s", got)
	}
	// Moving the last tab of a group removes the group and collapses the
	// column into the row.
	if !l.MoveTab("a", "g2", 0) {
		t.Fatal("move")
	}
	if got := dockShape(l.Root); got != "row(g2[a* c] g3[b*])" {
		t.Fatalf("merge: %s", got)
	}
	// Reorder inside a group: index counts the tab itself.
	l.MoveTab("a", "g2", 2)
	if got := dockShape(l.Root); got != "row(g2[c a*] g3[b*])" {
		t.Fatalf("reorder: %s", got)
	}
	l.RemoveTab("b")
	if got := dockShape(l.Root); got != "g2[c a*]" {
		t.Fatalf("remove: %s", got)
	}
	l.RemoveTab("a")
	l.RemoveTab("c")
	if got := dockShape(l.Root); got != "g2[]" || l.Focus != "g2" {
		t.Fatalf("empty: %s focus %q", got, l.Focus)
	}
	l.AddTab("x", "")
	if got := dockShape(l.Root); got != "g2[x*]" {
		t.Fatalf("add: %s", got)
	}
	// A group's only tab cannot split off itself.
	if _, ok := l.SplitTab("x", "g2", DockLeft); ok {
		t.Fatal("split of the only tab")
	}
}

func TestDockLayoutJSONRoundTrip(t *testing.T) {
	l := NewDockLayout("a", "b")
	l.SplitTab("b", "g1", DockBottom)
	l.Root.Sizes = []float64{0.7, 0.3}
	l.Maximized = "g2"
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"root":{"axis":"column","children":[{"id":"g1","tabs":["a"],"active":"a"},{"id":"g2","tabs":["b"],"active":"b"}],"sizes":[0.7,0.3]},"focus":"g2","maximized":"g2"}`
	if string(data) != want {
		t.Fatalf("json:\n%s\nwant\n%s", data, want)
	}
	back, err := ParseDockLayout(data)
	if err != nil {
		t.Fatal(err)
	}
	if dockShape(back.Root) != dockShape(l.Root) || !reflect.DeepEqual(back.Root.Sizes, l.Root.Sizes) || back.Maximized != "g2" || back.Focus != "g2" {
		t.Fatalf("round trip: %s %v", dockShape(back.Root), back.Root.Sizes)
	}
}

func TestDockLayoutRepair(t *testing.T) {
	// Unknown axis, nested split of the same axis, a split with one child,
	// repeated keys and IDs, bad sizes, a dangling maximized group.
	in := `{"root":{"axis":"diagonal","children":[
		{"id":"x","tabs":["a","a","b"],"active":"zz"},
		{"axis":"row","children":[{"id":"x","tabs":["b","c"]},{"axis":"column","children":[{"tabs":["d"]}]}],"sizes":[-1,2]},
		{"id":"empty"}
	],"sizes":[1]},"maximized":"nope"}`
	l, err := ParseDockLayout([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := dockShape(l.Root); got != "row(x[a* b] g1[c*] g2[d*])" {
		t.Fatalf("repair: %s", got)
	}
	sum := 0.0
	for _, s := range l.Root.Sizes {
		if s <= 0 {
			t.Fatalf("sizes %v", l.Root.Sizes)
		}
		sum += s
	}
	if sum < 0.999 || sum > 1.001 || l.Maximized != "" || l.Focus != "x" {
		t.Fatalf("sizes %v maximized %q focus %q", l.Root.Sizes, l.Maximized, l.Focus)
	}
	if _, err := ParseDockLayout([]byte("{")); err == nil {
		t.Fatal("bad json accepted")
	}
	if l, _ := ParseDockLayout([]byte(`{}`)); dockShape(l.Root) != "g1[]" {
		t.Fatalf("empty: %s", dockShape(l.Root))
	}
}
