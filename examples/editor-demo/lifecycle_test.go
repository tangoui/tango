package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

func testDocumentApp(t *testing.T, path string) (*tango.App, *fake.Backend) {
	t.Helper()
	a, fb := faketest.NewApp(tango.Options{})
	faketest.Start(t, a, fb)
	tango.Await(a, func() bool {
		app, surface, runner = a, nil, nil
		documents = newDocumentController(editor.DefaultRegistry(), theme.Base("vs"), false, false)
		documents.install(newDocumentSession(path, "original", nil))
		st.SetSelections([]editor.Selection{{Anchor: 0, Head: 8}})
		st.Insert("changed")
		return true
	})
	t.Cleanup(func() {
		tango.Await(a, func() bool {
			background.Close()
			st.Close()
			if second != nil {
				second.Close()
			}
			app, surface, runner, documents = nil, nil, nil, nil
			background, st, second, active = nil, nil, nil, nil
			paletteOpen, focused = false, false
			return true
		})
	})
	return a, fb
}

func TestUnsavedDialogButtons(t *testing.T) {
	for _, tt := range []struct {
		name         string
		choice       int
		allow, saved bool
	}{
		{"Save", 0, true, true}, {"DontSave", 1, true, false}, {"Cancel", 2, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "before.txt")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			a, fb := testDocumentApp(t, path)
			fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return tt.choice, nil }
			got := tango.Await(a, confirmDiscard)
			if got != tt.allow {
				t.Errorf("allow=%v want %v", got, tt.allow)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "original"
			if tt.saved {
				want = "changed"
			}
			if string(data) != want {
				t.Errorf("disk=%q want %q", data, want)
			}
		})
	}
}

func TestOpenDialogAsksDiscardOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "next.txt")
	if err := os.WriteFile(path, []byte("next"), 0600); err != nil {
		t.Fatal(err)
	}
	a, fb := testDocumentApp(t, "")
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 1, nil }
	fb.OnOpenDialog = func(_ platform.SurfaceID, _ platform.FileOptions) ([]string, error) { return []string{path}, nil }
	tango.Await(a, func() bool { openDocumentDialog(); return true })
	if prompts != 1 {
		t.Errorf("discard prompts=%d want 1", prompts)
	}
}

func TestSaveSeparatesUndoHistory(t *testing.T) {
	d := newDocumentSession("sample.txt", "", nil)
	v := editor.NewView(d.model, theme.Base("vs"))
	defer v.Close()
	v.Insert("a")
	if err := d.saveTo(filepath.Join(t.TempDir(), "saved.txt")); err != nil {
		t.Fatal(err)
	}
	v.Insert("b")
	v.Undo()
	if d.dirty() || d.text() != "a" {
		t.Fatalf("after undo dirty=%v text=%q, want saved a", d.dirty(), d.text())
	}
}

func TestQuitCancelKeepsDirtyDocument(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	tango.Await(a, func() bool { configureDocumentApp(a); return true })
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 2, nil }
	if fb.RequestQuit() {
		t.Fatal("Quit ignored Cancel")
	}
	if !tango.Await(a, func() bool { return documents.session.dirty() }) {
		t.Fatal("Quit lost current edits")
	}
}

func TestSaveAsCanceledKeepsDocument(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	fb.OnSaveDialog = func(_ platform.SurfaceID, _ platform.FileOptions) (string, error) { return "", platform.ErrCanceled }
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 0, nil }
	if tango.Await(a, confirmDiscard) {
		t.Fatal("canceled Save As allowed discard")
	}
	if !tango.Await(a, func() bool { return documents.session.dirty() && documents.session.path == "" }) {
		t.Fatal("cancellation changed session")
	}
}

func TestWindowDropAndReplacementHighlightNotify(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	a, fb := testDocumentApp(t, "")
	path := filepath.Join(t.TempDir(), "next.txt")
	if err := os.WriteFile(path, []byte("next"), 0600); err != nil {
		t.Fatal(err)
	}
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 1, nil }
	var sf *tango.Surface
	var windowErr error
	tango.Await(a, func() bool { _, sf, windowErr = openDemoWindow("Editor test", ui.RunnerOptions{}); return true })
	if windowErr != nil {
		t.Fatal(windowErr)
	}
	native := fb.Canvas(sf.ID())
	if ex, ok := native.Surface().Extras(); !ok || !ex.AcceptFiles {
		t.Fatal("native window does not accept dropped files")
	}
	if !fb.DropFiles(sf.ID(), []string{path}, 0, 0) {
		t.Fatal("drop not delivered")
	}
	if !tango.Await(a, func() bool { return documents.session.path == path }) {
		t.Fatal("drop did not open file")
	}
	before := native.Invalidates()
	tango.Await(a, func() bool { background.ready(); return true })
	tango.Await(a, func() bool { return true })
	if native.Invalidates() <= before {
		t.Fatal("replacement highlighter did not request repaint")
	}
}
