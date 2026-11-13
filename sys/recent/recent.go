// Package recent keeps the app's list of recently opened documents.
//
// The list lives in recent.json in the app's config directory
// (service/paths: Config(app ID)), newest first, at most Max entries. On
// macOS each Add is also reported to NSDocumentController, so the Dock
// menu's recent items follow it (macOS lists only document types the app
// declares in its Info.plist).
//
// Pair it with App.OnOpenFiles to record what the OS asks the app to open:
//
//	app.OnOpenFiles(func(paths []string) {
//		for _, p := range paths {
//			open(p)
//			recent.Add(app, p)
//		}
//	})
package recent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/service/paths"
)

// Max is how many entries the list keeps.
const Max = 10

var (
	mu        sync.Mutex
	configDir = paths.Config // replaced by tests
)

type file struct {
	Paths []string `json:"paths"`
}

func storePath(a *tango.App) (string, error) {
	id := a.ID()
	if id == "" {
		return "", errors.New("recent: the app needs an ID or Name")
	}
	d, err := configDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "recent.json"), nil
}

func load(p string) ([]string, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil // a corrupt list is not worth failing over
	}
	return f.Paths, nil
}

func save(p string, list []string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(file{Paths: list}, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func update(a *tango.App, fn func([]string) []string) error {
	p, err := storePath(a)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	list, err := load(p)
	if err != nil {
		return err
	}
	return save(p, fn(list))
}

// Add puts path at the top of the list (made absolute; a repeat moves up).
// Safe from any goroutine.
func Add(a *tango.App, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	err = update(a, func(list []string) []string {
		list = slices.DeleteFunc(list, func(s string) bool { return s == abs })
		list = append([]string{abs}, list...)
		if len(list) > Max {
			list = list[:Max]
		}
		return list
	})
	if err != nil {
		return err
	}
	if l, ok := a.Backend().(platform.RecentDocuments); ok {
		a.Post(func() { l.NoteRecentDocument(abs) })
	}
	return nil
}

// List returns the list, newest first, leaving out files that no longer
// exist.
func List(a *tango.App) ([]string, error) {
	p, err := storePath(a)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	list, err := load(p)
	mu.Unlock()
	return slices.DeleteFunc(list, func(s string) bool {
		_, err := os.Stat(s)
		return err != nil
	}), err
}

// Remove drops path from the list. The macOS list is not changed: AppKit
// has no call to remove one entry.
func Remove(a *tango.App, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return update(a, func(list []string) []string {
		return slices.DeleteFunc(list, func(s string) bool { return s == abs })
	})
}

// Clear empties the list, and the system's on macOS.
func Clear(a *tango.App) error {
	if err := update(a, func([]string) []string { return nil }); err != nil {
		return err
	}
	if l, ok := a.Backend().(platform.RecentDocuments); ok {
		a.Post(l.ClearRecentDocuments)
	}
	return nil
}
