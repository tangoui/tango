package a11y

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func sample() *Tree {
	t := NewTree(1)
	t.Add(&Node{ID: 1, Role: RoleWindow, Children: []NodeID{2, 3}})
	t.Add(&Node{ID: 2, Parent: 1, Role: RoleButton, Name: "OK", States: Focusable, Actions: Press | Focus})
	t.Add(&Node{ID: 3, Parent: 1, Role: RoleList, Children: []NodeID{4}})
	t.Add(&Node{ID: 4, Parent: 3, Role: RoleListItem, Name: "one"})
	return t
}

func TestDiff(t *testing.T) {
	a := sample()
	if got := Diff(nil, a); len(got) != 4 {
		t.Fatalf("initial diff %v", got)
	}
	if got := Diff(a, sample()); len(got) != 0 {
		t.Fatalf("identical trees differ: %v", got)
	}
	b := sample()
	b.Node(2).States |= Focused
	b.Node(3).Children = nil
	b.Remove(4)
	b.Add(&Node{ID: 5, Parent: 1, Role: RoleText, Name: "new"})
	b.Node(1).Children = append(b.Node(1).Children, 5)
	got := Diff(a, b)
	want := []NodeID{1, 2, 3, 5, 4}
	if !slices.Equal(got, want) {
		t.Fatalf("diff %v, want %v", got, want)
	}
	if b.Node(4) != nil || b.Len() != 4 {
		t.Error("removed node still present")
	}
}

func TestNodeEqual(t *testing.T) {
	x := &Node{ID: 1, Role: RoleSlider, HasRange: true, ValueNow: 3, Children: []NodeID{2}}
	y := *x
	y.Children = []NodeID{2}
	if !x.Equal(&y) {
		t.Fatal("copies differ")
	}
	y.ValueNow = 4
	if x.Equal(&y) {
		t.Fatal("value change not seen")
	}
	y = *x
	y.Children = []NodeID{3}
	if x.Equal(&y) {
		t.Fatal("children change not seen")
	}
	y = *x
	y.RTL = true
	if x.Equal(&y) {
		t.Fatal("direction change not seen")
	}
	if (*Node)(nil).Equal(x) || !(*Node)(nil).Equal(nil) {
		t.Fatal("nil handling")
	}
}

func TestTreeQueries(t *testing.T) {
	tr := sample()
	if n := tr.Find(RoleButton, "OK"); n == nil || n.ID != 2 {
		t.Fatalf("find %v", n)
	}
	if n := tr.Find(RoleNone, "one"); n == nil || n.ID != 4 {
		t.Fatalf("find any role %v", n)
	}
	if len(tr.FindAll(RoleNone, "")) != 4 {
		t.Fatal("find all")
	}
	if a := tr.Ancestor(4, RoleList); a == nil || a.ID != 3 {
		t.Fatalf("ancestor %v", a)
	}
	if s := tr.Text(1); s != "OK one" {
		t.Fatalf("text %q", s)
	}
	dump := tr.Dump()
	if !strings.Contains(dump, "  button \"OK\" [focusable]") || !strings.Contains(dump, "    listitem \"one\"") {
		t.Fatalf("dump:\n%s", dump)
	}
	var depths []int
	tr.Walk(func(n *Node, d int) bool { depths = append(depths, d); return n.Role != RoleList })
	if !slices.Equal(depths, []int{0, 1, 1}) {
		t.Fatalf("walk %v", depths)
	}
}

func TestStringers(t *testing.T) {
	if (Checked | Focusable).String() != "focusable|checked" {
		t.Error((Checked | Focusable).String())
	}
	if (Press | Dismiss).String() != "press|dismiss" {
		t.Error((Press | Dismiss).String())
	}
	for _, r := range Roles() {
		if strings.HasPrefix(r.String(), "role(") {
			t.Errorf("role %d has no name", r)
		}
		if p, ok := ParseRole(r.String()); !ok || p != r {
			t.Errorf("ParseRole(%s)", r)
		}
	}
}

// TestMappingsComplete checks every role has all five platform names and
// that docs/accessibility.md prints the same table.
func TestMappingsComplete(t *testing.T) {
	doc, err := os.ReadFile("../../docs/accessibility.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Roles() {
		m, ok := Mappings[r]
		if !ok {
			t.Errorf("%s: no mapping", r)
			continue
		}
		if m.Mac == "" || m.UIA == "" || m.ATSPI == "" || m.IOS == "" || m.Android == "" {
			t.Errorf("%s: incomplete mapping %+v", r, m)
		}
		row := "| `" + r.String() + "` | " + m.Mac + " | " + m.UIA + " | " + m.ATSPI + " | " + m.IOS + " | " + m.Android + " |"
		if !strings.Contains(string(doc), row) {
			t.Errorf("docs/accessibility.md lacks the row\n%s", row)
		}
	}
	if len(Mappings) != len(Roles()) {
		t.Errorf("%d mappings for %d roles", len(Mappings), len(Roles()))
	}
}
