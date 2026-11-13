// Package shell hands URLs and files to the operating system: open a link
// in the default browser, a file with its default app, or show a file in
// Finder / Explorer / the file manager.
//
// OpenURL only accepts URLs whose scheme is allowed (http, https and
// mailto by default; see AllowSchemes). Pages usually reach these
// functions through bound methods, and a page must not be able to make
// the app launch arbitrary protocol handlers or local programs (file:,
// custom schemes) just by passing a string. Paths are checked to exist.
//
// On iOS OpenURL goes through UIApplication openURL (Safari, Mail, …),
// OpenPath shows a QuickLook preview (or the "Open in…" menu) and
// RevealPath is ErrUnsupported, as is OpenPath for folders. On Android
// OpenURL starts Intent.ACTION_VIEW, OpenPath does the same with a
// content:// URI of the host's file provider, and RevealPath and folders
// are ErrUnsupported.
//
// Functions are safe from any goroutine once the app runs. They return
// ErrUnsupported when the backend has no shell integration and
// tango.ErrNotRunning after the app quit.
package shell

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

// ErrSchemeNotAllowed is returned by OpenURL for URLs whose scheme is not
// allowed.
var ErrSchemeNotAllowed = errors.New("tango/shell: URL scheme not allowed")

// DefaultSchemes are the schemes OpenURL allows unless changed.
var DefaultSchemes = []string{"http", "https", "mailto"}

var (
	schemesMu sync.RWMutex
	schemes   = slices.Clone(DefaultSchemes)
)

// AllowSchemes replaces the set of schemes OpenURL accepts, for the whole
// process. Schemes are case-insensitive, without the colon ("tel",
// "zoommtg"). With no arguments it restores DefaultSchemes. "file" is
// never allowed: use OpenPath.
func AllowSchemes(s ...string) {
	if len(s) == 0 {
		s = DefaultSchemes
	}
	out := make([]string, 0, len(s))
	for _, v := range s {
		v = strings.ToLower(strings.TrimSuffix(v, ":"))
		if v != "" && v != "file" {
			out = append(out, v)
		}
	}
	schemesMu.Lock()
	schemes = out
	schemesMu.Unlock()
}

// Allowed reports whether OpenURL would accept rawURL.
func Allowed(rawURL string) bool { return checkURL(rawURL) == nil }

func checkURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("tango/shell: bad URL: %w", err)
	}
	sch := strings.ToLower(u.Scheme)
	if sch == "" {
		return fmt.Errorf("%w: %q has no scheme", ErrSchemeNotAllowed, rawURL)
	}
	schemesMu.RLock()
	ok := slices.Contains(schemes, sch)
	schemesMu.RUnlock()
	if !ok || sch == "file" {
		return fmt.Errorf("%w: %s", ErrSchemeNotAllowed, sch)
	}
	if (sch == "http" || sch == "https") && u.Host == "" {
		return fmt.Errorf("tango/shell: %q has no host", rawURL)
	}
	return nil
}

func backend(a *tango.App) (platform.Shell, error) {
	s, ok := a.Backend().(platform.Shell)
	if !ok {
		return nil, ErrUnsupported
	}
	return s, nil
}

type result struct {
	err error
	ran bool
}

func call(a *tango.App, fn func(platform.Shell) error) error {
	s, err := backend(a)
	if err != nil {
		return err
	}
	r := tango.Await(a, func() result { return result{fn(s), true} })
	if !r.ran {
		return tango.ErrNotRunning
	}
	return r.err
}

// OpenURL opens rawURL with the system's handler for its scheme (the
// default browser for http and https, the mail app for mailto).
func OpenURL(a *tango.App, rawURL string) error {
	if _, err := backend(a); err != nil {
		return err
	}
	if err := checkURL(rawURL); err != nil {
		return err
	}
	return call(a, func(s platform.Shell) error { return s.OpenURL(rawURL) })
}

// existing returns the absolute form of path, which must exist.
func existing(path string) (string, error) {
	if path == "" {
		return "", errors.New("tango/shell: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// OpenPath opens a file with its default application, or a directory in
// the file manager.
func OpenPath(a *tango.App, path string) error {
	if _, err := backend(a); err != nil {
		return err
	}
	abs, err := existing(path)
	if err != nil {
		return err
	}
	return call(a, func(s platform.Shell) error { return s.OpenPath(abs) })
}

// RevealPath shows path selected in its folder (Finder, Explorer, the
// file manager). Where the file manager cannot select items, its parent
// folder is opened.
func RevealPath(a *tango.App, path string) error {
	if _, err := backend(a); err != nil {
		return err
	}
	abs, err := existing(path)
	if err != nil {
		return err
	}
	return call(a, func(s platform.Shell) error { return s.RevealPath(abs) })
}
