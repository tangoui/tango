// Package jumplist sets the app's shortcut menu: the menu of the Dock icon
// on macOS and the "Tasks" of the taskbar Jump List on Windows.
//
//	jumplist.OnClick(a, func(id string) {
//		switch id {
//		case "new":  newWindow()
//		case "sync": syncNow()
//		}
//	})
//	err := jumplist.Set(a, []jumplist.Item{
//		{ID: "new", Label: "New Window"},
//		jumplist.Separator(),
//		{ID: "sync", Label: "Sync Now", Description: "Fetch new mail"},
//	})
//
// How clicks arrive differs, and OnClick hides it:
//
//   - macOS: the Dock menu is part of the running app; a click calls the
//     handlers directly. The items appear above the system's own (Open
//     Recent, Options, Quit).
//   - Windows: a task is a shortcut that starts the executable again with
//     the argument "--tango-shortcut=<id>". With sys/single the new
//     process forwards it to the running instance, whose handlers run;
//     without it the new process sees its own argument and its handlers
//     run once it is ready. The list is kept by the shell across runs and
//     is tied to the app's AppUserModelID (the app's ID option).
//   - Linux has no runtime API: shortcuts are Desktop Actions in the
//     app's .desktop file, written when the app is packaged (tan package
//     reads them from the project; each action's Exec line passes
//     "--tango-shortcut=<id>"). Set returns ErrUnsupported; OnClick still
//     works for launches through those actions.
//
// Set and Clear are safe from any goroutine once the app runs; OnClick may
// be called before Run. Handlers run on the main thread.
package jumplist

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
	"github.com/tangoui/tango/sys/single"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// ErrBadItem is returned by Set for an item without an ID or label, a
// duplicate ID, or an ID that cannot travel on a command line.
var ErrBadItem = errors.New("tango/jumplist: bad item")

// Item is one entry of the shortcut menu.
type Item struct {
	ID          string // reported to OnClick; letters, digits and "-_.:" only
	Label       string
	Description string // Windows tooltip
	Icon        string // Windows: icon resource path, optionally ",index"; empty = the app's icon
	Disabled    bool   // macOS only
	Separator   bool
}

// Separator returns a separator item.
func Separator() Item { return Item{Separator: true} }

// Arg is the command-line argument a Windows task (or a Linux Desktop
// Action) passes to start the shortcut id.
func Arg(id string) string { return platform.ShortcutArgPrefix + id }

// IDFromArgs returns the shortcut id among command-line arguments, if any.
func IDFromArgs(args []string) (string, bool) {
	for _, s := range args {
		if id, ok := strings.CutPrefix(s, platform.ShortcutArgPrefix); ok && id != "" {
			return id, true
		}
	}
	return "", false
}

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:", r)
		if !ok {
			return false
		}
	}
	return true
}

// convert validates items and turns them into the platform form.
func convert(items []Item) ([]platform.ShortcutItem, error) {
	out := make([]platform.ShortcutItem, 0, len(items))
	seen := map[string]bool{}
	for i, it := range items {
		if it.Separator {
			out = append(out, platform.ShortcutItem{Separator: true})
			continue
		}
		if !validID(it.ID) {
			return nil, fmt.Errorf("%w: item %d: id %q", ErrBadItem, i, it.ID)
		}
		if it.Label == "" {
			return nil, fmt.Errorf("%w: item %q has no label", ErrBadItem, it.ID)
		}
		if seen[it.ID] {
			return nil, fmt.Errorf("%w: duplicate id %q", ErrBadItem, it.ID)
		}
		seen[it.ID] = true
		out = append(out, platform.ShortcutItem{
			ID: it.ID, Label: it.Label, Description: it.Description,
			IconPath: it.Icon, Disabled: it.Disabled,
		})
	}
	return out, nil
}

type appState struct {
	a        *tango.App
	mu       sync.Mutex
	handlers []func(string)
	hooked   bool
}

var states sync.Map // *tango.App -> *appState

func state(a *tango.App) *appState {
	v, _ := states.LoadOrStore(a, &appState{a: a})
	return v.(*appState)
}

func (st *appState) clicked(id string) {
	st.mu.Lock()
	fns := slices.Clone(st.handlers)
	st.mu.Unlock()
	// The backend calls clicked directly (SetAppShortcuts): guard each
	// handler on its own so a panic in one does not skip the rest.
	for _, fn := range fns {
		corehook.Guard(st.a, "event handler", func() { fn(id) })
	}
}

// launchArgs is replaced by tests.
var launchArgs = func() []string { return os.Args[1:] }

// Set replaces the shortcut menu with items; nil or empty clears it.
func Set(a *tango.App, items []Item) error {
	s, err := capcall.Backend[platform.AppShortcuts](a)
	if err != nil {
		return err
	}
	pi, err := convert(items)
	if err != nil {
		return err
	}
	if len(pi) == 0 {
		pi = nil
	}
	st := state(a)
	_, err = capcall.Sync(a, func() (struct{}, error) {
		return struct{}{}, s.SetAppShortcuts(pi, st.clicked)
	})
	return err
}

// Clear removes the shortcut menu.
func Clear(a *tango.App) error { return Set(a, nil) }

// OnClick registers fn for chosen shortcuts, on every platform: Dock menu
// clicks on macOS, and launches carrying "--tango-shortcut=<id>" (Windows
// tasks, Linux Desktop Actions) — this process's own launch arguments
// once, and later launches forwarded by sys/single.
func OnClick(a *tango.App, fn func(id string)) {
	st := state(a)
	st.mu.Lock()
	st.handlers = append(st.handlers, fn)
	hook := !st.hooked
	st.hooked = true
	st.mu.Unlock()
	if !hook {
		return
	}
	single.OnSecondInstance(a, func(args []string, _ string) {
		if id, ok := IDFromArgs(args); ok {
			st.clicked(id)
		}
	})
	if id, ok := IDFromArgs(launchArgs()); ok {
		a.OnReady(func() { st.clicked(id) })
	}
}
