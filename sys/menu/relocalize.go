package menu

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
)

// The labels of standard items (Role set, Label empty) are translated by
// the backend when it builds the native menu, in the locale of that
// moment (internal/platform/rolenames). When the effective locale changes
// — the system language on macOS/Windows, which i18n watches, or
// i18n.SetLocale — every attached menu (menu bar, window menus, tray
// menus) is rebuilt so those labels follow. Context menus are built when
// they open and need nothing.
//
// i18n does not import this package, so there is no cycle; registering a
// listener has no side effect until i18n reads the system languages.
func init() {
	i18n.OnChange(func(string) {
		// OnChange runs on the goroutine that caused the change, possibly
		// the main thread or one holding locks of the caller: push waits
		// for the main thread, so do it from a goroutine of its own.
		go relocalize()
	})
}

// relocalize re-applies every attachment.
func relocalize() {
	mu.Lock()
	ts := make([]*attachment, 0, len(attachments))
	for _, t := range attachments {
		ts = append(ts, t)
	}
	mu.Unlock()
	for _, t := range ts {
		t.pushOnMain()
	}
}

// pushOnMain is push for a rebuild nobody asked for: unlike Update, whose
// caller owns the menu tree while it converts it, this runs from a
// background goroutine, so the tree is read on the main thread (where apps
// normally edit their menus) instead of racing with the app. An app that
// quit detaches itself.
func (t *attachment) pushOnMain() {
	mu.Lock()
	t.gen++
	gen := t.gen
	mu.Unlock()
	ran := tango.Await(t.app, func() bool {
		mu.Lock()
		current := attachments[t.key] == t && t.gen == gen
		mu.Unlock()
		if current {
			items, onClick := convert(t.app, t.root)
			t.apply(items, onClick)
		}
		return true
	})
	if !ran {
		detach(t.key)
	}
}
