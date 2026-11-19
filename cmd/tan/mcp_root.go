package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// resolve maps a path a tool received to an absolute path inside the MCP
// root. Relative paths are taken from the root. Symlinks are resolved, and a
// path that does not exist yet is resolved through its nearest existing
// parent, so neither ".." nor a symlink can lead a tool outside the root.
//
// Callers must use the returned path, never the original argument: the check
// is a lexical Clean followed by EvalSymlinks, and holds only for the path it
// returns. On a case-insensitive filesystem a path spelled with a different
// case than the root fails closed (it is reported as outside).
func (s *mcpServer) resolve(p string) (string, error) {
	var abs string
	switch {
	case filepath.IsAbs(p):
		abs = filepath.Clean(p)
	case filepath.VolumeName(p) != "" || (p != "" && os.IsPathSeparator(p[0])):
		// Windows only: \foo (current drive) and C:foo (drive-relative)
		// are not relative to the root; take them where they really point.
		a, err := filepath.Abs(p)
		if err != nil {
			return "", fmt.Errorf("path %q: %w", p, err)
		}
		abs = a
	default:
		abs = filepath.Join(s.root, p)
	}
	real, err := evalExisting(abs)
	if err != nil {
		return "", fmt.Errorf("path %q: %w", p, err)
	}
	prefix := s.root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		// A filesystem or volume root already ends with a separator.
		prefix += string(filepath.Separator)
	}
	if real != s.root && !strings.HasPrefix(real, prefix) {
		return "", fmt.Errorf("path %q is outside the MCP root %s (resolves to %s)", p, s.root, real)
	}
	return real, nil
}

// evalExisting resolves the symlinks in the clean absolute path abs. When the
// path does not exist, it resolves the nearest existing parent and joins the
// missing rest back on.
func evalExisting(abs string) (string, error) {
	var rest []string
	dir := abs
	for {
		real, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if _, lerr := os.Lstat(dir); lerr == nil {
			// dir exists but does not resolve: a dangling symlink, whose
			// target could lie anywhere once it is created.
			return "", fmt.Errorf("%s is a dangling symlink", dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
		dir = parent
	}
}
