//go:build tango_dev

package devfeat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devfeat"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// devStateApp starts an App on a fresh fake backend with
// TANGO_DEV_WINDOW_STATE=path.
func devStateApp(t *testing.T, path string) (*tango.App, *fake.Backend) {
	t.Helper()
	t.Setenv("TANGO_DEV_WINDOW_STATE", path)
	a, fb := faketest.NewApp(tango.Options{Name: "Test"})
	faketest.Start(t, a, fb)
	return a, fb
}

func shortDevStateDelay(t *testing.T) {
	old := *devfeat.DevStateDelay
	*devfeat.DevStateDelay = 10 * time.Millisecond
	t.Cleanup(func() { *devfeat.DevStateDelay = old })
}

// waitDevState polls the file until key satisfies ok.
func waitDevState(t *testing.T, path, key string, ok func(devfeat.DevWin) bool) devfeat.DevWin {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for {
		var m map[string]devfeat.DevWin
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("bad file: %v\n%s", err, data)
			}
		}
		if w, found := m[key]; found && ok(w) {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("file never had a matching %q: %v", key, m)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDevStateRestore(t *testing.T) {
	shortDevStateDelay(t)
	path := filepath.Join(t.TempDir(), "dev", "windows.json")

	a, fb := devStateApp(t, path)
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "Main", Width: 800, Height: 600})
	if err != nil {
		t.Fatal(err)
	}
	fb.Move(s.ID(), 200, 220)
	fb.Resize(s.ID(), 900, 700)
	got := waitDevState(t, path, "Main", func(w devfeat.DevWin) bool { return w.Width == 900 && w.X == 200 })
	if got != (devfeat.DevWin{X: 200, Y: 220, Width: 900, Height: 700}) {
		t.Fatalf("saved %+v", got)
	}
	// An untitled window is keyed by creation order.
	u, _ := a.NewSurface(tango.SurfaceOptions{Width: 300, Height: 200})
	fb.Move(u.ID(), 50, 60)
	waitDevState(t, path, "#2", func(w devfeat.DevWin) bool { return w.X == 50 })
	a.Quit()
	<-corehook.Done(a)

	// A new run: the window comes back there, even with explicit X/Y.
	a2, _ := devStateApp(t, path)
	s2, err := a2.NewSurface(tango.SurfaceOptions{Title: "Main", Width: 800, Height: 600, X: 5, Y: 5})
	if err != nil {
		t.Fatal(err)
	}
	if b := s2.Bounds(); b != (tango.Rect{X: 200, Y: 220, Width: 900, Height: 700}) {
		t.Fatalf("restored %+v", b)
	}
	// A second open window with the same title has its own key.
	s3, _ := a2.NewSurface(tango.SurfaceOptions{Title: "Main", Width: 400, Height: 300, X: 7, Y: 8})
	if b := s3.Bounds(); b != (tango.Rect{X: 7, Y: 8, Width: 400, Height: 300}) {
		t.Fatalf("second window %+v", b)
	}
	u2, _ := a2.NewSurface(tango.SurfaceOptions{Width: 300, Height: 200})
	if b := u2.Bounds(); b.X == 50 { // "#3" now: not restored
		t.Fatalf("untitled %+v", b)
	}
}

func TestDevStateState(t *testing.T) {
	shortDevStateDelay(t)
	path := filepath.Join(t.TempDir(), "windows.json")
	a, fb := devStateApp(t, path)
	s, _ := a.NewSurface(tango.SurfaceOptions{Title: "W", Width: 800, Height: 600})
	fb.Move(s.ID(), 100, 100)
	waitDevState(t, path, "W", func(w devfeat.DevWin) bool { return w.X == 100 })
	fb.ChangeState(s.ID(), platform.StateMaximized)
	got := waitDevState(t, path, "W", func(w devfeat.DevWin) bool { return w.State == "maximized" })
	if got.X != 100 || got.Width != 800 {
		t.Fatalf("maximizing lost the normal bounds: %+v", got)
	}
	// Minimizing keeps what was saved.
	fb.ChangeState(s.ID(), platform.StateMinimized)
	fb.Move(s.ID(), 1, 1)
	time.Sleep(50 * time.Millisecond)
	waitDevState(t, path, "W", func(w devfeat.DevWin) bool { return w.State == "maximized" && w.X == 100 })
	a.Quit()
	<-corehook.Done(a)

	a2, fb2 := devStateApp(t, path)
	s2, _ := a2.NewSurface(tango.SurfaceOptions{Title: "W", Width: 300, Height: 300})
	if b := s2.Bounds(); b != (tango.Rect{X: 100, Y: 100, Width: 800, Height: 600}) {
		t.Fatalf("bounds %+v", b)
	}
	if st := fb2.Surface(s2.ID()).CurrentState(); st != tango.StateMaximized {
		t.Fatalf("state %v", st)
	}
}

func TestDevStateOffscreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "windows.json")
	data, _ := json.Marshal(map[string]devfeat.DevWin{
		"Gone": {X: 5000, Y: 100, Width: 800, Height: 600}, // display unplugged
		"Here": {X: 300, Y: 200, Width: 640, Height: 480},
	})
	os.WriteFile(path, data, 0o644)
	a, _ := devStateApp(t, path)
	s, _ := a.NewSurface(tango.SurfaceOptions{Title: "Gone", Width: 400, Height: 300, X: 10, Y: 30})
	if b := s.Bounds(); b != (tango.Rect{X: 10, Y: 30, Width: 400, Height: 300}) {
		t.Fatalf("off-screen place restored: %+v", b)
	}
	h, _ := a.NewSurface(tango.SurfaceOptions{Title: "Here", Width: 400, Height: 300})
	if b := h.Bounds(); b != (tango.Rect{X: 300, Y: 200, Width: 640, Height: 480}) {
		t.Fatalf("not restored: %+v", b)
	}
}

func TestDevStateOff(t *testing.T) {
	shortDevStateDelay(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "windows.json")
	data, _ := json.Marshal(map[string]devfeat.DevWin{"W": {X: 300, Y: 200, Width: 640, Height: 480}})
	os.WriteFile(path, data, 0o644)

	// Hot-reload workers leave placement to the dev host.
	t.Setenv("TANGO_DEV_ROLE", "worker")
	a, fb := devStateApp(t, path)
	s, _ := a.NewSurface(tango.SurfaceOptions{Title: "W", Width: 400, Height: 300, X: 10, Y: 30})
	if b := s.Bounds(); b.X != 10 {
		t.Fatalf("worker restored: %+v", b)
	}
	fb.Move(s.ID(), 1, 2)
	time.Sleep(60 * time.Millisecond)
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatalf("worker wrote the file: %s", got)
	}
}

// The saved place goes into the creation options, so the window is
// created there rather than moved after it appeared (no flash at the
// default place).
func TestDevStatePresetBeforeCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "windows.json")
	data, _ := json.Marshal(map[string]devfeat.DevWin{
		"Main": {X: 300, Y: 200, Width: 640, Height: 480, State: "maximized"},
	})
	os.WriteFile(path, data, 0o644)
	a, fb := devStateApp(t, path)
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "Main", Width: 400, Height: 300, Centered: true})
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := fb.Surface(s.ID()).Extras()
	if !ok {
		t.Fatal("no creation extras")
	}
	if ex.Centered || ex.Bounds != (platform.Rect{X: 300, Y: 200, Width: 640, Height: 480}) || ex.State != tango.StateMaximized {
		t.Fatalf("creation extras %+v", ex)
	}
	if st := fb.Surface(s.ID()).CurrentState(); st != tango.StateMaximized {
		t.Fatalf("state %v", st)
	}
	if b := s.Bounds(); b != (tango.Rect{X: 300, Y: 200, Width: 640, Height: 480}) {
		t.Fatalf("bounds %+v", b)
	}
}
