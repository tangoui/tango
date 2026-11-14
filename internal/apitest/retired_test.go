// Package apitest holds repository-wide API guards that run with go test
// (docs/specs/api-stability.md §4.3).
package apitest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// retired lists module-relative directories that used to be public
// packages and were moved (for example ui/gpu -> internal/render/gpu in
// Phase 1). A rebased branch can recreate a file at the old path and
// silently bring the public package back; this guard fails instead. An
// entry ending in "/..." covers the directory and everything below it.
var retired = []string{}

// moduleRoot returns the repository root (two levels above this file).
func moduleRoot(t *testing.T) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// goFilesUnder returns the .go files in dir, or in its whole tree when
// recursive is set. A missing directory has none.
func goFilesUnder(dir string, recursive bool) ([]string, error) {
	var found []string
	if !recursive {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			return nil, nil
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				found = append(found, filepath.Join(dir, e.Name()))
			}
		}
		return found, err
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) && path == dir {
			return filepath.SkipAll
		}
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

// violations returns the .go files found under the retired paths.
func violations(root string, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		rel, recursive := strings.CutSuffix(p, "/...")
		files, err := goFilesUnder(filepath.Join(root, filepath.FromSlash(rel)), recursive)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			r, _ := filepath.Rel(root, f)
			out = append(out, filepath.ToSlash(r)+" is under the retired path "+p)
		}
	}
	return out, nil
}

func TestRetiredPaths(t *testing.T) {
	v, err := violations(moduleRoot(t), retired)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range v {
		t.Errorf("%s; move it to the package's new location", msg)
	}
}

// TestRetiredGuard checks the guard itself on a temporary tree.
func TestRetiredGuard(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("old/keep.txt")
	write("tree/sub/x.go")
	write("flat/sub/y.go")

	for _, tc := range []struct {
		paths []string
		fail  bool
	}{
		{[]string{"missing", "missing/..."}, false},
		{[]string{"old"}, false},     // no .go files
		{[]string{"flat"}, false},    // .go only in a subdirectory
		{[]string{"flat/..."}, true}, // ... covers it
		{[]string{"tree/sub"}, true},
	} {
		v, err := violations(root, tc.paths)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(v) > 0; got != tc.fail {
			t.Errorf("violations(%v) = %v, want failure %v", tc.paths, v, tc.fail)
		}
	}
}
