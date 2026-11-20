package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

func testDocumentApp(t *testing.T, path string) (*tango.App, *fake.Backend) {
	t.Helper()
	// Exercise explicit quit requests even after the last window closes,
	// regardless of the host OS's default last-window policy.
	keepOpen := false
	a, fb := faketest.NewApp(tango.Options{QuitOnLastClose: &keepOpen})
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

// Without a window the main thread is the UI thread: the quit request is
// answered at once from the user's choice.
func TestQuitCancelKeepsDirtyDocument(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	tango.Await(a, func() bool { configureDocumentApp(a); return true })
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 2, nil }
	if fb.RequestQuit() {
		t.Fatal("Quit ignored Cancel")
	}
	if prompts != 1 {
		t.Fatalf("unsaved-changes prompts=%d want 1", prompts)
	}
	if !tango.Await(a, func() bool { return documents.session.dirty() }) {
		t.Fatal("Quit lost current edits")
	}
}

func TestQuitDontSaveQuits(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	t.Cleanup(resetAfterQuit)
	tango.Await(a, func() bool { configureDocumentApp(a); return true })
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 1, nil }
	if !fb.RequestQuit() {
		t.Fatal("Don't Save did not quit")
	}
}

// openEditorWindow opens the demo window on the fake backend and runs a
// frame, so the document state is published for the main thread.
func openEditorWindow(t *testing.T, a *tango.App, fb *fake.Backend) *tango.Surface {
	t.Helper()
	t.Setenv("TANGO_RENDERER", "cpu")
	var sf *tango.Surface
	var err error
	tango.Await(a, func() bool {
		configureDocumentApp(a)
		_, sf, err = openDemoWindow("Editor test", ui.RunnerOptions{})
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	cv := fb.Canvas(sf.ID())
	cv.Invalidate()
	cv.FlushFrame()
	tango.Await(a, func() bool { return true })
	return sf
}

// resetAfterQuit clears the demo's globals once the app has quit (the
// testDocumentApp cleanup cannot reach the main thread any more).
func resetAfterQuit() {
	app, surface, runner, documents = nil, nil, nil, nil
	background, st, second, active = nil, nil, nil, nil
	paletteOpen, focused = false, false
	docDirty.Store(false)
	windowClosed.Store(false)
}

// With a window the request is refused at first, asked on the window's UI
// thread, and "Don't Save" then quits.
func TestWindowQuitDontSaveQuits(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	t.Cleanup(resetAfterQuit)
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 1, nil }
	openEditorWindow(t, a, fb)
	if !docDirty.Load() {
		t.Fatal("dirty document not published")
	}
	if fb.RequestQuit() {
		t.Fatal("quit with unsaved edits did not wait for the answer")
	}
	select {
	case <-corehook.Done(a):
	case <-time.After(5 * time.Second):
		t.Fatalf("Don't Save did not quit (prompts=%d)", prompts)
	}
	if prompts != 1 {
		t.Fatalf("prompts=%d want 1", prompts)
	}
}

// Cancel keeps the app running; once the window has closed, quitting is
// no longer refused (Runner.Post would drop the question).
func TestWindowQuitCancelThenClosedWindowQuits(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	t.Cleanup(resetAfterQuit)
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 2, nil }
	sf := openEditorWindow(t, a, fb)
	if fb.RequestQuit() {
		t.Fatal("quit with unsaved edits did not wait for the answer")
	}
	for deadline := time.Now().Add(5 * time.Second); tango.Await(a, func() bool { return prompts == 0 }) && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if !tango.Await(a, func() bool { return prompts == 1 && documents.session.dirty() }) {
		t.Fatal("Cancel was not asked or lost the edits")
	}
	tango.Await(a, func() bool { sf.Close(); return true })
	if !fb.RequestQuit() {
		t.Fatal("quit refused after the window closed")
	}
}

// Typing into a clean document marks it dirty in the same frame: a quit
// right after that frame asks instead of quitting.
func TestTypingThenQuitAsks(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	t.Cleanup(func() { docDirty.Store(false); windowClosed.Store(false) }) // the app keeps running (Cancel)
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 2, nil }
	tango.Await(a, func() bool { documents.install(newDocumentSession("", "clean", nil)); return true })
	sf := openEditorWindow(t, a, fb)
	if docDirty.Load() {
		t.Fatal("clean document published as dirty")
	}
	cv := fb.Canvas(sf.ID())
	cv.Input(input.TextInput{Text: "x"})
	cv.FlushFrame()
	tango.Await(a, func() bool { return true })
	if fb.RequestQuit() {
		t.Fatal("quit right after typing did not ask about the edit")
	}
	for deadline := time.Now().Add(5 * time.Second); tango.Await(a, func() bool { return prompts == 0 }) && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if !tango.Await(a, func() bool { return prompts == 1 && documents.session.dirty() }) {
		t.Fatal("typed edit was not asked about")
	}
}

// A clean document closes and quits at once, without a prompt.
func TestCleanDocumentAnswersAtOnce(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	t.Cleanup(resetAfterQuit)
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) {
		t.Error("clean document asked about unsaved changes")
		return 2, nil
	}
	tango.Await(a, func() bool { documents.install(newDocumentSession("", "clean", nil)); return true })
	openEditorWindow(t, a, fb)
	if docDirty.Load() {
		t.Fatal("clean document published as dirty")
	}
	if !fb.RequestQuit() {
		t.Fatal("clean document refused to quit")
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
	// The dirty document makes the drop ask confirmDiscard first; that
	// dialog runs a nested wait on the fake backend's main loop, so an
	// Await from this goroutine can run before the file is open: poll.
	opened := func() bool { return tango.Await(a, func() bool { return documents.session.path == path }) }
	for deadline := time.Now().Add(5 * time.Second); !opened() && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if !opened() {
		t.Fatal("drop did not open file")
	}
	before := native.Invalidates()
	tango.Await(a, func() bool { background.ready(); return true })
	for deadline := time.Now().Add(5 * time.Second); native.Invalidates() <= before && time.Now().Before(deadline); {
		tango.Await(a, func() bool { return true }) // the notify reaches the window through Runner.Post
	}
	if native.Invalidates() <= before {
		t.Fatal("replacement highlighter did not request repaint")
	}
}
