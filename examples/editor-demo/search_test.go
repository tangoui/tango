package main

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

func TestReplaceAllCaseSensitive(t *testing.T) {
	d := newDocumentSession("sample.txt", "foo Foo foo", editor.DefaultRegistry())
	view := editor.NewView(d.model, theme.Base("vs"))
	defer view.Close()

	count, err := replaceAll(view, "foo", "bar", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || view.Snapshot().String() != "bar Foo bar" {
		t.Fatalf("replaceAll count=%d text=%q", count, view.Snapshot().String())
	}
}

func TestReplaceAllCaseInsensitive(t *testing.T) {
	d := newDocumentSession("sample.txt", "foo Foo FOO", editor.DefaultRegistry())
	view := editor.NewView(d.model, theme.Base("vs"))
	defer view.Close()

	count, err := replaceAll(view, "foo", "bar", false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || view.Snapshot().String() != "bar bar bar" {
		t.Fatalf("replaceAll count=%d text=%q", count, view.Snapshot().String())
	}
}

func TestReplaceAllNoMatchLeavesTextUnchanged(t *testing.T) {
	d := newDocumentSession("sample.txt", "hello", editor.DefaultRegistry())
	view := editor.NewView(d.model, theme.Base("vs"))
	defer view.Close()

	count, err := replaceAll(view, "missing", "bar", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || view.Snapshot().String() != "hello" {
		t.Fatalf("replaceAll count=%d text=%q", count, view.Snapshot().String())
	}
}

func TestFindMatchWrapsForwardAndBackward(t *testing.T) {
	text := "one two one"
	if a, b, ok := findMatch(text, "one", true, len(text), true); !ok || a != 0 || b != 3 {
		t.Fatalf("forward wrap = %d,%d,%v", a, b, ok)
	}
	if a, b, ok := findMatch(text, "one", true, 0, false); !ok || a != 8 || b != 11 {
		t.Fatalf("backward wrap = %d,%d,%v", a, b, ok)
	}
}

func TestFindMatchCaseInsensitive(t *testing.T) {
	if a, b, ok := findMatch("Foo foo", "FOO", false, 0, true); !ok || a != 0 || b != 3 {
		t.Fatalf("case-insensitive match = %d,%d,%v", a, b, ok)
	}
}

func TestPreviousSearchMovesPastCurrentSelection(t *testing.T) {
	v := editor.NewState("one two one", "sample.txt", theme.Base("vs"))
	defer v.Close()
	p := searchPanel{query: "one", caseSensitive: true}
	v.SetSelections([]editor.Selection{{Anchor: 8, Head: 11}})
	p.find(v, false)
	if sel := v.Primary(); sel.Anchor != 0 || sel.Head != 3 {
		t.Fatalf("previous selected %v", sel)
	}
}

func TestReplaceMissingReportsNoMatches(t *testing.T) {
	v := editor.NewState("hello", "sample.txt", theme.Base("vs"))
	defer v.Close()
	p := searchPanel{query: "absent", replacement: "new"}
	p.replace(v)
	if p.status != "No matches" {
		t.Fatalf("status=%q", p.status)
	}
}

func TestSearchPanelIsInlineAndRestoresEditorFocus(t *testing.T) {
	d := shellDriver(t, 1200)
	st.Insert("// one two one\n")
	showSearch(false)
	d.Frame()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleDialog, "Find and Replace"); ok {
		t.Fatal("search still uses a blocking modal")
	}
	query, ok := d.A11yFind(a11y.RoleTextbox, "Find text")
	if !ok {
		t.Fatal("inline query missing")
	}
	if query.Bounds.W < 160 || query.Bounds.Y > 180 {
		t.Fatalf("query outside editor header: %+v", query.Bounds)
	}
	d.Type("one")
	d.Key(input.KeyEnter, 0)
	d.Frame()
	if st.Snapshot().Slice(st.Primary().Anchor, st.Primary().Head) != "one" {
		t.Fatal("Enter did not select a match")
	}
	if err := d.ClickLabel("Previous match"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if err := d.ClickLabel("Close search"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	d.Frame()
	if search.open {
		t.Fatal("search did not close")
	}
	if n := d.A11yFocused(); n == nil || n.Name != "main.go" {
		t.Fatalf("focus not restored to editor: %+v", n)
	}
}

func TestInlineReplaceWorksWithoutCoveringEditor(t *testing.T) {
	d := shellDriver(t, 480)
	showSearch(true)
	search.query = "Hello"
	search.replacement = "Hi"
	d.Frame()
	d.Frame()
	if err := d.ClickLabel("Replace all"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if !strings.Contains(st.Snapshot().String(), "Hi, Tango") {
		t.Fatal("inline replace all did not apply")
	}
	if n, ok := d.A11yFind(a11y.RoleTextbox, "main.go"); !ok || n.Bounds.H < 300 {
		t.Fatal("search hid the code viewport")
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if search.open {
		t.Fatal("Escape did not close inline search")
	}
}
