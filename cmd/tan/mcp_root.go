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
func (s *mcpServer) resolve(p string) (string, error) {
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.root, abs)
	}
	abs = filepath.Clean(abs)
	real, err := evalExisting(abs)
	if err != nil {
		return "", fmt.Errorf("path %q: %w", p, err)
	}
	if real != s.root && !strings.HasPrefix(real, s.root+string(filepath.Separator)) {
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
