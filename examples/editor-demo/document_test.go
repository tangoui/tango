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
