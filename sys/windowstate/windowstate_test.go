package windowstate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

type memStore struct {
	mu    sync.Mutex
	m     map[string]State
	saves int
}

func (m *memStore) Load(k string) (State, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.m[k]
	return st, ok
}

func (m *memStore) Save(k string, st State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[k] = st
	m.saves++
	return nil
}

func (m *memStore) get(k string) State { st, _ := m.Load(k); return st }

func TestOnScreen(t *testing.T) {
	screens := []platform.Rect{{X: 0, Y: 0, Width: 1920, Height: 1080}, {X: 1920, Y: 0, Width: 1280, Height: 1024}}
	for _, c := range []struct {
		st   State
		want bool
	}{
		{State{X: 100, Y: 100, Width: 800, Height: 600}, true},
		{State{X: 2000, Y: 100, Width: 800, Height: 600}, true},  // second screen
		{State{X: 5000, Y: 100, Width: 800, Height: 600}, false}, // unplugged monitor
		{State{X: -790, Y: 100, Width: 800, Height: 600}, false}, // only 10px visible
		{State{X: 100, Y: -500, Width: 800, Height: 600}, false}, // title bar above the screen
		{State{X: 100, Y: 100, Width: 0, Height: 600}, false},    // bad size
		{State{X: 1900, Y: 500, Width: 800, Height: 600}, true},  // straddles both
	} {
		if got := onScreen(c.st, screens); got != c.want {
			t.Errorf("%+v: %v", c.st, got)
		}
	}
	if !onScreen(State{X: 99999, Y: 99999, Width: 10, Height: 10}, nil) {
		t.Error("without displays only the size counts")
	}
}

func TestFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "windows.json")
	fs := NewFileStore(path)
	if NewFileStore(path) != fs {
		t.Fatal("stores for one path not shared")
	}
	if _, ok := fs.Load("main"); ok {
		t.Fatal("empty store has a key")
	}
	if err := fs.Save("main", State{X: 1, Y: 2, Width: 3, Height: 4, Maximized: true}); err != nil {
		t.Fatal(err)
	}
	fs.Save("other", State{Width: 5, Height: 6})
	// A fresh store (another run) reads both back.
	fresh := &FileStore{path: path}
	if st, ok := fresh.Load("main"); !ok || st != (State{X: 1, Y: 2, Width: 3, Height: 4, Maximized: true}) {
		t.Fatalf("main: %+v %v", st, ok)
	}
	if st, _ := fresh.Load("other"); st.Width != 5 {
		t.Fatal("other lost")
	}
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, ok := (&FileStore{path: path}).Load("main"); ok {
		t.Fatal("corrupt file")
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath("com.example.app")
	if err != nil {
		t.Skip(err)
	}
	if filepath.Base(p) != "windows.json" || filepath.Base(filepath.Dir(p)) != "com.example.app" {
		t.Fatal(p)
	}
}

func TestTrackRestoresAndSaves(t *testing.T) {
	old := SaveDelay
	SaveDelay = 10 * time.Millisecond
	t.Cleanup(func() { SaveDelay = old })

	a, fb := faketest.Run(t, tango.Options{ID: "test"})
	store := &memStore{m: map[string]State{"main": {X: 50, Y: 60, Width: 700, Height: 500, Maximized: true}}}
	s, err := a.NewSurface(tango.SurfaceOptions{Hidden: true, Width: 300, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	if err := Track(s, "main", store); err != nil {
		t.Fatal(err)
	}
	if b := s.Bounds(); b != (tango.Rect{X: 50, Y: 60, Width: 700, Height: 500}) {
		t.Fatalf("restored bounds %+v", b)
	}
	if s.State() != tango.StateMaximized {
		t.Fatal("not maximized")
	}

	// User restores and moves the window: saved after the delay.
	fb.ChangeState(s.ID(), tango.StateNormal)
	fb.Move(s.ID(), 80, 90)
	deadline := time.Now().Add(faketest.Timeout)
	for store.get("main") != (State{X: 80, Y: 90, Width: 700, Height: 500}) {
		if time.Now().After(deadline) {
			t.Fatalf("saved %+v", store.get("main"))
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Maximize: the normal bounds stay, the flag is set; minimizing keeps it.
	fb.ChangeState(s.ID(), tango.StateMaximized)
	fb.Resize(s.ID(), 1920, 1080)
	fb.ChangeState(s.ID(), tango.StateMinimized)
	fb.RequestClose(s.ID()) // saves synchronously
	if st := store.get("main"); st != (State{X: 80, Y: 90, Width: 700, Height: 500, Maximized: true}) {
		t.Fatalf("after close %+v", st)
	}
}

func TestTrackDefaultStoreAndUnsupported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	keep := false // closing the last surface must not quit (default on Linux/Windows)
	a, fb := faketest.Run(t, tango.Options{ID: "com.example.track", QuitOnLastClose: &keep})
	s, _ := a.NewSurface(tango.SurfaceOptions{Width: 300, Height: 200})
	if err := Track(s, "w", nil); err != nil {
		t.Fatal(err)
	}
	fb.RequestClose(s.ID())
	path, _ := DefaultPath("com.example.track")
	if st, ok := (&FileStore{path: path}).Load("w"); !ok || st.Width != 300 {
		t.Fatalf("%s: %+v %v", path, st, ok)
	}
	closed, _ := a.NewSurface(tango.SurfaceOptions{})
	closed.Close()
	if err := Track(closed, "x", &memStore{m: map[string]State{}}); err == nil {
		t.Fatal("tracking a closed surface")
	}
}
