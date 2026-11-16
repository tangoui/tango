package main

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
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
