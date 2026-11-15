package main

import (
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
