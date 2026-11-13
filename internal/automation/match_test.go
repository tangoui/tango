package automation

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

// sampleTree: window > [dialog "记一笔" > [button "保存", button "取消"], button "保存" (disabled), textbox "备注" value "x"]
func sampleTree() *a11y.Tree {
	t := a11y.NewTree(1)
	t.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2, 5, 6}})
	t.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleDialog, Name: "记一笔", Children: []a11y.NodeID{3, 4}})
	t.Add(&a11y.Node{ID: 3, Parent: 2, Role: a11y.RoleButton, Name: "保存", States: a11y.Focusable | a11y.Focused, Actions: a11y.Press})
	t.Add(&a11y.Node{ID: 4, Parent: 2, Role: a11y.RoleButton, Name: "取消", States: a11y.Focusable, Actions: a11y.Press})
	t.Add(&a11y.Node{ID: 5, Parent: 1, Role: a11y.RoleButton, Name: "保存", States: a11y.Disabled})
	t.Add(&a11y.Node{ID: 6, Parent: 1, Role: a11y.RoleTextbox, Name: "Note 备注", Value: "x"})
	return t
}

func pick(t *testing.T, q Query) (a11y.NodeID, int) {
	t.Helper()
	m, err := q.Compile()
	if err != nil {
		t.Fatalf("%v: %v", q, err)
	}
	n, c := m.Pick(sampleTree())
	if n == nil {
		return 0, c
	}
	return n.ID, c
}

func TestMatch(t *testing.T) {
	x := "x"
	y := "y"
	for _, tc := range []struct {
		q     Query
		id    a11y.NodeID
		count int
	}{
		{Query{Role: "button", Name: "保存"}, 3, 2},
		{Query{Role: "button", Name: "保存", Nth: 1}, 5, 2},
		{Query{Role: "button", Name: "保存", Nth: -1}, 5, 2},
		{Query{Role: "button", Name: "保存", Nth: 2}, 0, 2},
		{Query{Role: "button", Name: "保存", Nth: -3}, 0, 2},
		{Query{Role: "button"}, 3, 3},
		{Query{Name: "记一笔"}, 2, 1},
		{Query{Role: "button", Name: "保"}, 0, 0}, // exact by default
		{Query{Role: "button", Name: "保", Match: MatchContains}, 3, 2},
		{Query{Role: "textbox", Name: "note", Match: MatchContains}, 6, 1}, // case-insensitive
		{Query{Name: "^(取消|关闭)$", Match: MatchRegexp}, 4, 1},
		{Query{Name: "备注$", Match: MatchRegexp}, 6, 1},
		{Query{Role: "button", States: []string{"disabled"}}, 5, 1},
		{Query{Role: "button", Name: "保存", States: []string{"!disabled"}}, 3, 1},
		{Query{Role: "button", States: []string{"focusable", "!focused"}}, 4, 1},
		{Query{Role: "textbox", Value: &x}, 6, 1},
		{Query{Role: "textbox", Value: &y}, 0, 0},
		{Query{}, 1, 6}, // everything, reading order from the root
	} {
		id, c := pick(t, tc.q)
		if id != tc.id || c != tc.count {
			t.Errorf("%v: got node %d of %d, want %d of %d", tc.q, id, c, tc.id, tc.count)
		}
	}
}

func TestMatchReadingOrder(t *testing.T) {
	// Insertion order differs from the tree's reading order.
	tr := a11y.NewTree(1)
	tr.Add(&a11y.Node{ID: 9, Parent: 1, Role: a11y.RoleButton, Name: "B"})
	tr.Add(&a11y.Node{ID: 8, Parent: 1, Role: a11y.RoleButton, Name: "A"})
	tr.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{8, 9}})
	m, _ := Query{Role: "button"}.Compile()
	if all := m.All(tr); len(all) != 2 || all[0].Name != "A" || all[1].Name != "B" {
		t.Fatalf("order %v", all)
	}
}

func TestCompileErrors(t *testing.T) {
	for _, q := range []Query{{Role: "buton"}, {Name: "(", Match: MatchRegexp}, {Match: "fuzzy"}, {States: []string{"shiny"}}, {States: []string{"!"}}} {
		if _, err := q.Compile(); err == nil {
			t.Errorf("%+v compiled", q)
		}
	}
}

func TestParseStateAction(t *testing.T) {
	for _, s := range []a11y.State{a11y.Disabled, a11y.Checked, a11y.Expanded, a11y.Editable} {
		if got, ok := ParseState(s.String()); !ok || got != s {
			t.Errorf("state %v", s)
		}
	}
	for _, a := range []a11y.Action{a11y.Press, a11y.SetValue, a11y.Dismiss, a11y.ScrollIntoView} {
		if got, ok := ParseAction(a.String()); !ok || got != a {
			t.Errorf("action %v", a)
		}
	}
	if _, ok := ParseAction(""); ok {
		t.Error("empty action")
	}
}

func TestQueryString(t *testing.T) {
	v := "1"
	for q, want := range map[*Query]string{
		{Role: "button", Name: "OK"}:                                `button "OK"`,
		{Name: "mail", Match: MatchContains}:                        `node containing "mail"`,
		{Role: "row", Name: "^a", Match: MatchRegexp, Nth: 2}:       `row matching /^a/ #2`,
		{Role: "checkbox", States: []string{"!checked"}, Value: &v}: `checkbox [!checked] value="1"`,
	} {
		if got := q.String(); got != want {
			t.Errorf("%q, want %q", got, want)
		}
	}
}
