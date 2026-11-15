package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

func TestDocumentLoadAndSavePreservesEOL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte("one\r\ntwo\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := loadDocument(src, editor.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}

	if got := d.text(); got != "one\ntwo\n" {
		t.Fatalf("normalised text = %q", got)
	}
	dst := filepath.Join(dir, "saved.txt")
	if err := d.saveTo(dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one\r\ntwo\r\n" {
		t.Fatalf("saved bytes = %q", got)
	}
	if d.path != dst || d.dirty() {
		t.Fatalf("after save path=%q dirty=%v", d.path, d.dirty())
	}
}

func TestDocumentDirtyTracksUndo(t *testing.T) {
	d := newDocumentSession("untitled.txt", "hello", editor.DefaultRegistry())
	view := editor.NewView(d.model, theme.Base("vs"))
	defer view.Close()

	if d.dirty() {
		t.Fatal("new document is dirty")
	}
	view.SetSelections([]editor.Selection{{Anchor: 5, Head: 5}})
	view.Insert("!")
	if !d.dirty() {
		t.Fatal("edit did not mark document dirty")
	}
	if !view.Undo() {
		t.Fatal("undo did not run")
	}
	if d.dirty() {
		t.Fatal("undo back to saved content stayed dirty")
	}
}

func TestDocumentSaveFailureKeepsPath(t *testing.T) {
	d := newDocumentSession("before.txt", "hello", editor.DefaultRegistry())

	err := d.saveTo(filepath.Join(t.TempDir(), "missing", "after.txt"))
	if err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if d.path != "before.txt" {
		t.Fatalf("failed save changed path to %q", d.path)
	}
}

func TestReplaceAllRejectsEmptyQuery(t *testing.T) {
	d := newDocumentSession("sample.txt", "hello", editor.DefaultRegistry())
	view := editor.NewView(d.model, theme.Base("vs"))
	defer view.Close()

	_, err := replaceAll(view, "", "x", false)
	if !errors.Is(err, errEmptySearch) {
		t.Fatalf("replaceAll error = %v, want %v", err, errEmptySearch)
	}
}

func TestResolveDiscard(t *testing.T) {
	tests := []struct {
		name   string
		dirty  bool
		action discardAction
		want   bool
	}{
		{name: "clean cancel is harmless", action: discardCancel, want: true},
		{name: "dirty save allows close", dirty: true, action: discardSave, want: true},
		{name: "dirty discard allows close", dirty: true, action: discardDontSave, want: true},
		{name: "dirty cancel vetoes close", dirty: true, action: discardCancel, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDiscard(tt.dirty, tt.action); got != tt.want {
				t.Fatalf("resolveDiscard(%v, %v) = %v, want %v", tt.dirty, tt.action, got, tt.want)
			}
		})
	}
}

func TestDocumentControllerTitleMarksDirty(t *testing.T) {
	c := newDocumentController(editor.DefaultRegistry(), theme.Base("vs"), false, false)
	c.install(newDocumentSession("/tmp/main.go", "package main\n", c.registry))
	defer func() {
		if background != nil {
			background.Close()
		}
		if st != nil {
			st.Close()
		}
	}()

	if got := c.title(); got != "Tango Editor — main.go" {
		t.Fatalf("clean title = %q", got)
	}
	st.SetSelections([]editor.Selection{{Anchor: 0, Head: 0}})
	st.Insert("// comment\n")
	if got := c.title(); got != "Tango Editor — *main.go" {
		t.Fatalf("dirty title = %q", got)
	}
}
