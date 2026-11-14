// Package fileserver serves a local directory over HTTP, for showing user
// files (downloads, generated images, a documents folder) in a page. Mount
// it under a prefix of the app's asset handler with Mount.
//
// Requests cannot leave the directory: paths containing ".." are refused,
// and files are opened through an os.Root, so symlinks pointing outside the
// directory are not followed either. Directory listings and dotfiles are
// not served unless enabled.
package fileserver

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// Options configures New.
type Options struct {
	_ [0]func()

	// List enables directory listings. Without it a directory is served
	// only through its index.html.
	List bool
	// Dotfiles allows serving entries whose name starts with ".".
	Dotfiles bool
}

// Server is an http.Handler serving one directory. Close releases it.
type Server struct {
	root *os.Root
	opts Options
	fs   http.Handler
}

// New returns a Server for dir, which must exist.
func New(dir string, opts Options) (*Server, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	s := &Server{root: root, opts: opts}
	s.fs = http.FileServerFS(guardFS{s})
	return s, nil
}

// Close closes the directory. Later requests fail.
func (s *Server) Close() error { return s.root.Close() }

// ServeHTTP serves the file at r.URL.Path relative to the directory.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	if strings.Contains(p, "\x00") || strings.Contains(p, `\`) || hasDotDot(p) {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	s.fs.ServeHTTP(w, r)
}

func hasDotDot(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// Mount returns a handler that serves srv under prefix (e.g. "/files/")
// and passes every other request to next (nil: 404). The prefix is
// stripped before srv sees the path.
func Mount(prefix string, srv http.Handler, next http.Handler) http.Handler {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if next == nil {
		next = http.NotFoundHandler()
	}
	inner := http.StripPrefix(strings.TrimSuffix(prefix, "/"), srv)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, prefix) {
			inner.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guardFS is the root's file system with the Options' restrictions.
type guardFS struct{ s *Server }

func (g guardFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if !g.s.opts.Dotfiles && name != "." {
		for seg := range strings.SplitSeq(name, "/") {
			if strings.HasPrefix(seg, ".") {
				return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
			}
		}
	}
	f, err := g.s.root.FS().Open(name)
	if err != nil {
		return nil, err
	}
	if g.s.opts.List {
		return f, nil
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if fi.IsDir() {
		// Only serve a directory that has an index.html, which
		// http.FileServerFS then serves in its place.
		if _, err := fs.Stat(g.s.root.FS(), path.Join(name, "index.html")); err != nil {
			f.Close()
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	return f, nil
}
