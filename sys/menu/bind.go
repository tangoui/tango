package menu

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// An attachment is a persistent place a menu is shown (menu bar, window
// menu, tray icon). Update rebuilds and re-applies every attachment whose
// tree contains the updated menu.
type attachment struct {
	app   *tango.App
	key   any
	root  *Menu
	apply func(items []platform.MenuItem, onClick func(id uint32)) // main thread
	gen   uint64
}

var attachments = map[any]*attachment{} // guarded by mu

func init() {
	corehook.AttachMenu = func(app any, key any, m any, apply func([]platform.MenuItem, func(uint32))) error {
		a := app.(*tango.App)
		root, _ := m.(*Menu)
		if root == nil {
			detach(key)
			if !tango.Await(a, func() bool { apply(nil, nil); return true }) {
				return tango.ErrNotRunning
			}
			return nil
		}
		return attach(a, key, root, apply)
	}
	corehook.DetachMenu = detach
}

func attach(a *tango.App, key any, m *Menu, apply func([]platform.MenuItem, func(uint32))) error {
	t := &attachment{app: a, key: key, root: m, apply: apply}
	mu.Lock()
	attachments[key] = t
	mu.Unlock()
	return t.push()
}

func detach(key any) {
	mu.Lock()
	delete(attachments, key)
	mu.Unlock()
}

// push rebuilds t and applies it on the main thread unless a newer push or
// a replacement overtook it.
func (t *attachment) push() error {
	mu.Lock()
	t.gen++
	gen := t.gen
	mu.Unlock()
	items, onClick := convert(t.app, t.root)
	ran := tango.Await(t.app, func() bool {
		mu.Lock()
		current := attachments[t.key] == t && t.gen == gen
		mu.Unlock()
		if current {
			t.apply(items, onClick)
		}
		return true
	})
	if !ran {
		detach(t.key) // the app has quit
		return tango.ErrNotRunning
	}
	return nil
}

// Update sends the current state of m to every place it is shown: menus
// set with SetAppMenu / SetSurfaceMenu or on a tray icon whose tree
// contains m. Safe from any goroutine; waits until the native menus are
// updated. Context menus need no update: they are built when they open.
func (m *Menu) Update() error {
	mu.Lock()
	var ts []*attachment
	for _, t := range attachments {
		if contains(t.root, m, map[*Menu]bool{}) {
			ts = append(ts, t)
		}
	}
	mu.Unlock()
	var first error
	for _, t := range ts {
		if err := t.push(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// contains reports whether m is root or one of its submenus. mu held.
func contains(root, m *Menu, seen map[*Menu]bool) bool {
	if root == nil || seen[root] {
		return false
	}
	if root == m {
		return true
	}
	seen[root] = true
	for _, it := range root.Items {
		if it != nil && contains(it.Submenu, m, seen) {
			return true
		}
	}
	return false
}
