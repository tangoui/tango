//go:build tango_dev

package devfeat

// Window placement across `tan dev` restarts (docs/specs/ui-hotreload.md,
// "Window placement across restarts"). tan dev sets TANGO_DEV_WINDOW_STATE
// to a JSON file; every App started with it restores each window's last
// position, size and state from that file and writes them back as they
// change. Without the variable, and in production builds (without -tags
// tango_dev; release.go), nothing happens.
//
// It is deliberately self-contained (package windowstate imports package
// tango, so it cannot be used here) and only meant for development: the
// saved place wins over SurfaceOptions.X/Y/Centered, so a window comes back
// where the developer left it.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// windowSurface is the part of *tango.Surface's exported API window
// placement uses.
type windowSurface interface {
	Title() string
	OnClosed(fn func())
	Bounds() platform.Rect
	SetBounds(r platform.Rect) error
	State() platform.WindowState
	SetState(st platform.WindowState) error
	OnMoved(fn func(x, y int))
	OnResized(fn func(w, h int))
	OnStateChanged(fn func(platform.WindowState))
}

// devStateEnv names the state file; set by tan dev.
const devStateEnv = "TANGO_DEV_WINDOW_STATE"

// devStateDelay is how long after the last change the file is written
// (a variable for tests).
var devStateDelay = 300 * time.Millisecond

// devWin is one window's saved place. X, Y, Width, Height are the normal
// (not maximized) bounds as Surface.Bounds reports them.
type devWin struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	State  string `json:"state,omitempty"` // "", "maximized" or "fullscreen"
}

// devStateFile is the state file of one App.
type devStateFile struct {
	path string

	mu    sync.Mutex // guards m, dirty, timer
	m     map[string]devWin
	dirty bool
	timer *time.Timer

	wmu sync.Mutex // serializes writes
}

// AttachWindowState hooks a (a *tango.App) into the state file named by
// TANGO_DEV_WINDOW_STATE. Called by package tango for every new App.
func AttachWindowState(a App) {
	path := os.Getenv(devStateEnv)
	// Hot-reload workers only have hidden stand-ins for windows the dev
	// host owns; the host restores the real ones.
	if path == "" || os.Getenv("TANGO_DEV_ROLE") == "worker" {
		return
	}
	f := &devStateFile{path: path, m: map[string]devWin{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &f.m) // a corrupt file starts over
		if f.m == nil {
			f.m = map[string]devWin{}
		}
	}
	// Keys of open windows; main thread only.
	live := map[string]bool{}
	seq := 0
	keyFor := func(title string, seq int) string {
		key := title
		if key == "" {
			key = "#" + strconv.Itoa(seq)
		}
		// Two open windows with one title: the second is "Title#2".
		for n := 2; live[key]; n++ {
			key = title + "#" + strconv.Itoa(n)
		}
		return key
	}
	// Before creation: put the saved place into the options, so the
	// window is created there instead of being moved after it appeared
	// (which could flash it at the default place). preset remembers the
	// key whose place went into the options of the surface being created.
	preset := ""
	corehook.OnPrepareSurface(a, func(o *corehook.SurfacePlacement) {
		preset = ""
		key := keyFor(o.Title, seq+1)
		if f.preset(a, o, key) {
			preset = key
		}
	})
	corehook.OnSurfaceCreated(a, func(surface any) {
		s := surface.(windowSurface)
		seq++
		key := keyFor(s.Title(), seq)
		live[key] = true
		s.OnClosed(func() { delete(live, key) })
		applied := preset == key
		preset = ""
		f.track(a, s, key, applied)
	})
	a.OnLifecycle(func(ev platform.LifecycleEvent) {
		if ev == platform.LifecycleTerminating {
			f.flush()
		}
	})
}

// preset writes the place saved under key into o before the surface is
// created and reports whether it did. A place at (0, 0) is left to track:
// X = Y = 0 in SurfaceOptions means "centered". Main thread.
func (f *devStateFile) preset(a App, o *corehook.SurfacePlacement, key string) bool {
	f.mu.Lock()
	w, ok := f.m[key]
	f.mu.Unlock()
	if !ok || w.Width <= 0 || w.Height <= 0 || (w.X == 0 && w.Y == 0) {
		return false
	}
	r := platform.Rect{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height}
	if !devVisible(a, r) {
		return false
	}
	o.X, o.Y, o.Width, o.Height = r.X, r.Y, r.Width, r.Height
	o.Centered = false
	switch w.State {
	case "maximized":
		o.State = platform.StateMaximized
	case "fullscreen":
		o.State = platform.StateFullscreen
	}
	return true
}

// track restores s from the file (unless preset already put the saved
// place into its creation options) and records its changes. Main thread.
func (f *devStateFile) track(a App, s windowSurface, key string, preset bool) {
	f.mu.Lock()
	w, ok := f.m[key]
	f.mu.Unlock()
	if ok && !preset {
		r := platform.Rect{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height}
		if r.Width > 0 && r.Height > 0 && devVisible(a, r) {
			s.SetBounds(r)
		}
		switch w.State {
		case "maximized":
			s.SetState(platform.StateMaximized)
		case "fullscreen":
			s.SetState(platform.StateFullscreen)
		}
	}
	rec := func() { f.record(s, key) }
	s.OnMoved(func(int, int) { rec() })
	s.OnResized(func(int, int) { rec() })
	s.OnStateChanged(func(platform.WindowState) { rec() })
}

// record saves s's current place under key and schedules a write. Main
// thread.
func (f *devStateFile) record(s windowSurface, key string) {
	st := s.State()
	if st == platform.StateMinimized { // keep the place it was minimized from
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.m[key]
	switch st {
	case platform.StateMaximized:
		w.State = "maximized"
	case platform.StateFullscreen:
		w.State = "fullscreen"
	default:
		b := s.Bounds()
		if b.Width <= 0 || b.Height <= 0 {
			return
		}
		w = devWin{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}
	}
	if old, ok := f.m[key]; ok && old == w {
		return
	}
	f.m[key] = w
	f.dirty = true
	if f.timer == nil {
		f.timer = time.AfterFunc(devStateDelay, f.flush)
	} else {
		f.timer.Reset(devStateDelay)
	}
}

// flush writes the file if anything changed. Any goroutine.
func (f *devStateFile) flush() {
	f.wmu.Lock()
	defer f.wmu.Unlock()
	f.mu.Lock()
	if !f.dirty {
		f.mu.Unlock()
		return
	}
	f.dirty = false
	data, err := json.MarshalIndent(f.m, "", "  ")
	f.mu.Unlock()
	if err == nil {
		err = devWriteFile(f.path, append(data, '\n'))
	}
	if err != nil {
		os.Stderr.WriteString("tango: saving window placement: " + err.Error() + "\n")
	}
}

// devWriteFile replaces path atomically: a temporary file renamed over it.
func devWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".windows-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// devVisible reports whether enough of r's title strip lies on a display
// to grab the window. True when the platform cannot list displays. Main
// thread.
func devVisible(a App, r platform.Rect) bool {
	d, ok := a.Backend().(platform.Displays)
	if !ok {
		return true
	}
	ds := d.Displays()
	if len(ds) == 0 {
		return true
	}
	const strip = 40 // px of the top edge that must be reachable
	top := platform.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: min(r.Height, strip)}
	for _, disp := range ds {
		area := disp.WorkArea
		if area.Width <= 0 || area.Height <= 0 {
			area = disp.Bounds
		}
		w := min(top.X+top.Width, area.X+area.Width) - max(top.X, area.X)
		h := min(top.Y+top.Height, area.Y+area.Height) - max(top.Y, area.Y)
		if w >= min(strip, top.Width) && h >= min(strip/2, top.Height) {
			return true
		}
	}
	return false
}
