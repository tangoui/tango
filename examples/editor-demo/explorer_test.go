package main

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
	"os"
	"path/filepath"
	"testing"
)

func TestExplorerLoadsOneDirectoryInNavigationOrder(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "src"), 0755)
	os.WriteFile(filepath.Join(root, "z.txt"), []byte("z"), 0600)
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main"), 0600)
	os.WriteFile(filepath.Join(root, "src", "deep.go"), []byte("package deep"), 0600)
	var e fileExplorer
	if err := e.openFolder(root); err != nil {
		t.Fatal(err)
	}
	if len(e.nodes) != 3 || e.nodes[0].Title != "src" || e.nodes[1].Title != "a.go" {
		t.Fatalf("bad folder navigation: %+v", e.nodes)
	}
	if len(e.nodes[0].Children) != 0 || e.nodes[0].IsLeaf {
		t.Fatal("folder must be lazy expandable")
	}
	e.load(filepath.Join(root, "src"))
	if len(e.nodes[0].Children) != 1 || e.nodes[0].Children[0].Title != "deep.go" {
		t.Fatal("expansion failed")
	}
	if err := e.openFolder(filepath.Join(root, "missing")); err == nil || e.root != root {
		t.Fatal("failed folder open replaced navigation")
	}
}

func TestExplorerDoesNotFollowDirectorySymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skip(err)
	}
	var e fileExplorer
	if err := e.openFolder(root); err != nil {
		t.Fatal(err)
	}
	if len(e.nodes) != 1 || !e.nodes[0].IsLeaf {
		t.Fatal("symlink directory must not recursively expand")
	}
}

func TestExplorerFileSelectionHonorsCancel(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	path := filepath.Join(t.TempDir(), "other.go")
	if err := os.WriteFile(path, []byte("package other"), 0600); err != nil {
		t.Fatal(err)
	}
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 2, nil }
	var e fileExplorer
	result := tango.Await(a, func() bool { return e.selectFile(path) })
	if result {
		t.Fatal("cancel accepted file replacement")
	}
	if got := tango.Await(a, func() string { return st.Snapshot().String() }); got != "changed" {
		t.Fatal("navigation discarded edits")
	}
}

func TestExplorerKeepsCurrentDirtyFileWithoutPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.go")
	os.WriteFile(path, []byte("original"), 0600)
	a, fb := testDocumentApp(t, path)
	prompts := 0
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { prompts++; return 1, nil }
	var e fileExplorer
	if !tango.Await(a, func() bool { return e.selectFile(path) }) {
		t.Fatal("current file selection failed")
	}
	if prompts != 0 {
		t.Fatalf("current file prompted %d times", prompts)
	}
	if got := tango.Await(a, func() string { return st.Snapshot().String() }); got != "changed" {
		t.Fatal("current file was reloaded")
	}
}

func TestExplorerReopenFolderReloadsExpandedDirectories(t *testing.T) {
	d := shellDriver(t, 1200)
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "src"), 0755)
	os.WriteFile(filepath.Join(root, "src", "child.go"), []byte("package child"), 0600)
	for range 2 {
		if err := shellUI.explorer.openFolder(root); err != nil {
			t.Fatal(err)
		}
		d.Frame()
		d.Frame()
		shellUI.explorer.expanded = []string{filepath.Join(root, "src")}
		d.Frame()
		d.Frame()
		if _, ok := d.A11yFind(a11y.RoleTreeItem, "child.go"); !ok {
			t.Fatal("folder reopen retained stale tree load state")
		}
	}
}

func TestExplorerRefreshRetriesFailedDirectory(t *testing.T) {
	d := shellDriver(t, 1200)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.Mkdir(src, 0755)
	if err := shellUI.explorer.openFolder(root); err != nil {
		t.Fatal(err)
	}
	os.Remove(src)
	shellUI.explorer.expanded = []string{src}
	d.Frame()
	d.Frame()
	if shellUI.explorer.err == "" {
		t.Fatal("directory failure not reported")
	}
	os.Mkdir(src, 0755)
	os.WriteFile(filepath.Join(src, "recovered.go"), []byte("package recovered"), 0600)
	shellUI.explorer.refresh()
	d.Frame()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleTreeItem, "recovered.go"); !ok {
		t.Fatal("refresh did not retry failed directory")
	}
}

func TestExplorerMissingFileRestoresActualDocumentSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.go")
	a, _ := testDocumentApp(t, path)
	e := fileExplorer{selected: []string{"missing"}}
	tango.Await(a, func() bool { e.selectFile(filepath.Join(t.TempDir(), "missing.go")); return true })
	if len(e.selected) != 1 || e.selected[0] != path {
		t.Fatalf("missing file left selection=%v", e.selected)
	}
}
