package devhost

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

func a11yTestTree(name string, extra bool) *a11y.Tree {
	t := a11y.NewTree(1)
	kids := []a11y.NodeID{2}
	if extra {
		kids = append(kids, 3)
	}
	t.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: kids})
	t.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: name, Actions: a11y.Press | a11y.Focus,
		States: a11y.Focusable, Bounds: a11y.Rect{X: 1, Y: 2, W: 30, H: 10}})
	if extra {
		t.Add(&a11y.Node{ID: 3, Parent: 1, Role: a11y.RoleSlider, HasRange: true, ValueNow: 5, ValueMax: 10})
	}
	t.Focus = 2
	return t
}

func TestA11yMessagesRoundTrip(t *testing.T) {
	a, b := pipe(t)
	full := NewA11yTree(4, 1, a11yTestTree("OK", true), nil, true)
	full.Announcements = []a11y.Announcement{{Text: "saved", Live: a11y.LivePolite}}
	msgs := []any{
		A11yState{Win: 4, On: true, Epoch: 1},
		full,
		A11yAction{Win: 4, ID: 3, Act: a11y.SetValue, Arg: "7"},
	}
	go func() {
		for _, m := range msgs {
			if err := a.Send(m); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i, want := range msgs {
		got, err := b.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("message %d: got %#v, want %#v", i, got, want)
		}
	}
}

func TestA11yTreeFullAndDelta(t *testing.T) {
	t1 := a11yTestTree("OK", true)
	got1, err := NewA11yTree(1, 1, t1, nil, true).Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got1.Dump() != t1.Dump() || got1.Focus != 2 || len(a11y.Diff(t1, got1)) != 0 {
		t.Fatalf("full tree:\n%s\nwant\n%s", got1.Dump(), t1.Dump())
	}

	// Delta: the button renamed, the slider removed.
	t2 := a11yTestTree("Save", false)
	changed := a11y.Diff(t1, t2)
	m := NewA11yTree(1, 1, t2, changed, false)
	if len(m.Nodes) != 2 || len(m.Removed) != 1 || m.Removed[0] != 3 {
		t.Fatalf("delta %+v", m)
	}
	if _, err := m.Apply(nil); err != ErrA11yDelta {
		t.Fatalf("delta without base: %v", err)
	}
	got2, err := m.Apply(got1)
	if err != nil {
		t.Fatal(err)
	}
	if d := a11y.Diff(t2, got2); len(d) != 0 || got2.Len() != 2 {
		t.Fatalf("delta tree differs in %v:\n%s", d, got2.Dump())
	}
	if got1.Node(2).Name != "OK" || got1.Len() != 3 {
		t.Fatal("Apply modified the base tree")
	}

	// A node added back keeps reading order.
	t3 := a11yTestTree("Save", true)
	got3, err := NewA11yTree(1, 1, t3, a11y.Diff(t2, t3), false).Apply(got2)
	if err != nil {
		t.Fatal(err)
	}
	var ids []a11y.NodeID
	for _, n := range got3.Nodes() {
		ids = append(ids, n.ID)
	}
	if !reflect.DeepEqual(ids, []a11y.NodeID{1, 2, 3}) || len(a11y.Diff(t3, got3)) != 0 {
		t.Fatalf("order %v", ids)
	}
	if got3.Node(2) != got2.Node(2) {
		t.Fatal("unchanged node not shared")
	}
}
