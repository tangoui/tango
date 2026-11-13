// Package windowstate remembers where a window was: its position, size and
// whether it was maximized, saved as it changes and restored the next time
// the app starts.
//
//	s, _ := app.NewSurface(tango.SurfaceOptions{Hidden: true, Resizable: true})
//	windowstate.Track(s, "main", nil) // nil: <UserConfigDir>/<App.ID>/windows.json
//	s.Show()
//
// Track before showing the window so it does not jump. A saved position
// that is no longer on any display (a monitor was unplugged) is not
// restored; the window keeps its size and is centered instead.
package windowstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// State is what is remembered of one window. X, Y, Width, Height are the
// normal (not maximized) bounds, as tango.Surface.Bounds returns them.
type State struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized,omitempty"`
}

// Store keeps states by key. Implementations must be safe for concurrent
// use.
type Store interface {
	Load(key string) (State, bool)
	Save(key string, st State) error
}

// SaveDelay is how long Track waits after the last change before saving.
var SaveDelay = 400 * time.Millisecond

// DefaultPath returns <os.UserConfigDir>/<appID>/windows.json. An empty
// appID uses the executable's name.
func DefaultPath(appID string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if appID == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		appID = filepath.Base(exe)
	}
	return filepath.Join(dir, appID, "windows.json"), nil
}

// FileStore is a Store backed by one JSON file holding every key.
type FileStore struct {
	path string

	mu     sync.Mutex
	loaded bool
	m      map[string]State
}

var (
	storesMu sync.Mutex
	stores   = map[string]*FileStore{}
)

// NewFileStore returns the store for the JSON file at path. Stores for the
// same path are shared, so windows tracked separately do not overwrite
// each other's entries.
func NewFileStore(path string) *FileStore {
	path = filepath.Clean(path)
	storesMu.Lock()
	defer storesMu.Unlock()
	if fs := stores[path]; fs != nil {
		return fs
	}
	fs := &FileStore{path: path}
	stores[path] = fs
	return fs
}

// Path returns the file's path.
func (fs *FileStore) Path() string { return fs.path }

func (fs *FileStore) load() { // fs.mu held
	if fs.loaded {
		return
	}
	fs.loaded = true
	fs.m = map[string]State{}
	if data, err := os.ReadFile(fs.path); err == nil {
		_ = json.Unmarshal(data, &fs.m) // a corrupt file starts over
		if fs.m == nil {
			fs.m = map[string]State{}
		}
	}
}

func (fs *FileStore) Load(key string) (State, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.load()
	st, ok := fs.m[key]
	return st, ok
}

// Save records st under key and rewrites the file atomically.
func (fs *FileStore) Save(key string, st State) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.load()
	if old, ok := fs.m[key]; ok && old == st {
		return nil
	}
	fs.m[key] = st
	data, err := json.MarshalIndent(fs.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fs.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(fs.path), ".windows-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), fs.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Track restores s from store under key, then saves its state whenever it
// is moved, resized, maximized or restored, and when it closes or the app
// terminates. A nil store is the shared file at DefaultPath(app ID). It
// returns tango.ErrUnsupported when the platform cannot place windows.
func Track(s *tango.Surface, key string, store Store) error {
	if store == nil {
		path, err := DefaultPath(s.App().ID())
		if err != nil {
			return err
		}
		store = NewFileStore(path)
	}
	cur := s.Bounds()
	if cur.Width <= 0 || cur.Height <= 0 {
		return tango.ErrUnsupported
	}
	t := &tracker{s: s, key: key, store: store}
	if st, ok := store.Load(key); ok {
		t.restore(st, displays(s.App()))
	}
	t.mu.Lock()
	t.st = State{X: cur.X, Y: cur.Y, Width: cur.Width, Height: cur.Height}
	t.mu.Unlock()
	tango.Await(s.App(), func() struct{} { t.record(); return struct{}{} })

	s.OnResized(func(int, int) { t.changed() })
	s.OnMoved(func(int, int) { t.changed() })
	s.OnStateChanged(func(tango.WindowState) { t.changed() })
	s.OnClose(func() bool { t.record(); t.flush(); return true })
	s.OnClosed(func() { t.flush() })
	s.App().OnLifecycle(func(ev tango.Lifecycle) {
		if ev == tango.Terminating {
			t.flush()
		}
	})
	return nil
}

type tracker struct {
	s     *tango.Surface
	key   string
	store Store

	mu    sync.Mutex
	st    State
	dirty bool
	timer *time.Timer
	done  bool
}

// displays returns the work areas of the app's displays, or nil when the
// platform cannot list them.
func displays(a *tango.App) []platform.Rect {
	d, ok := a.Backend().(platform.Displays)
	if !ok {
		return nil
	}
	return tango.Await(a, func() []platform.Rect {
		var rs []platform.Rect
		for _, disp := range d.Displays() {
			r := disp.WorkArea
			if r.Width <= 0 || r.Height <= 0 {
				r = disp.Bounds
			}
			rs = append(rs, r)
		}
		return rs
	})
}

// onScreen reports whether enough of st's top edge (where a title bar is)
// lies on one of the screens to grab the window. Without screens only the
// size is checked.
func onScreen(st State, screens []platform.Rect) bool {
	if st.Width <= 0 || st.Height <= 0 {
		return false
	}
	if len(screens) == 0 {
		return true
	}
	const grab = 48 // px of title strip that must be reachable
	top := platform.Rect{X: st.X, Y: st.Y, Width: st.Width, Height: min(st.Height, grab)}
	for _, r := range screens {
		w := min(top.X+top.Width, r.X+r.Width) - max(top.X, r.X)
		h := min(top.Y+top.Height, r.Y+r.Height) - max(top.Y, r.Y)
		if w >= min(grab, top.Width) && h >= min(grab/2, top.Height) {
			return true
		}
	}
	return false
}

func (t *tracker) restore(st State, screens []platform.Rect) {
	if st.Width <= 0 || st.Height <= 0 {
		return
	}
	if onScreen(st, screens) {
		t.s.SetBounds(tango.Rect{X: st.X, Y: st.Y, Width: st.Width, Height: st.Height})
	} else {
		t.s.SetSize(st.Width, st.Height)
		t.s.Center()
	}
	if st.Maximized {
		t.s.Maximize()
	}
}

// record reads the window's current state. Main thread.
func (t *tracker) record() {
	state := t.s.State()
	b := t.s.Bounds()
	t.mu.Lock()
	defer t.mu.Unlock()
	switch state {
	case tango.StateNormal:
		if b.Width > 0 && b.Height > 0 {
			t.st = State{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}
		}
	case tango.StateMaximized:
		t.st.Maximized = true
	}
	// Minimized and fullscreen keep the last normal bounds and flag.
	t.dirty = true
}

// changed records and schedules a save. Main thread.
func (t *tracker) changed() {
	t.record()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return
	}
	if t.timer == nil {
		t.timer = time.AfterFunc(SaveDelay, t.flush)
	} else {
		t.timer.Reset(SaveDelay)
	}
}

// flush saves now if something changed. Any goroutine.
func (t *tracker) flush() {
	t.mu.Lock()
	if !t.dirty {
		t.mu.Unlock()
		return
	}
	t.dirty = false
	st := t.st
	t.mu.Unlock()
	_ = t.store.Save(t.key, st)
}
