//go:build darwin && !ios

package darwin

import (
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// App-level events (contract round 2, HostEx): deep links, opened files,
// Dock reopen, quit interception, recent documents.

func (b *backend) hostEx() (platform.HostEx, bool) {
	hx, ok := b.h.(platform.HostEx)
	return hx, ok
}

// shouldQuit asks HostEx.ShouldQuit about a quit the user started. A second
// request while the first is still being answered (Cmd+Q pressed again
// under a confirmation dialog) is refused.
func (b *backend) shouldQuit() bool {
	hx, ok := b.hostEx()
	if !ok {
		return true
	}
	if b.askingQuit {
		return false
	}
	b.askingQuit = true
	defer func() { b.askingQuit = false }()
	return hx.ShouldQuit()
}

func (b *backend) appEvent(ev platform.AppEvent) {
	if hx, ok := b.hostEx(); ok {
		hx.OnAppEvent(ev)
	}
}

// disableArgumentOpen switches off AppKit's habit of treating unrecognised
// command-line arguments as documents (delivered via application:openURLs:).
// The core already parses os.Args on every desktop platform; left on, a file
// argument would be reported twice and a stray flag value could be "opened".
func disableArgumentOpen() {
	withPool(func() {
		key, off := nsString("NSTreatUnknownArgumentsAsOpen"), nsString("NO")
		store := send(class("NSUserDefaults"), "standardUserDefaults")
		send(store, "setObject:forKey:", uintptr(off), uintptr(key))
	})
}

// partitionOpenURLs separates file URLs (returned as paths) from every other
// URL (returned as absolute strings), preserving order within each group.
func partitionOpenURLs(urls id) (paths, links []string) {
	for _, u := range arrayItems(urls) {
		if !sendBool(u, "isFileURL") {
			links = append(links, goString(send(u, "absoluteString")))
		} else if p := goString(send(u, "path")); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, links
}

func appEventMethods() []objc.MethodDef {
	// NSApplicationDelegate application:openURLs: (macOS 10.13+) receives
	// both custom-scheme deep links (CFBundleURLTypes) and documents (Open
	// With, drops on the Dock icon, file associations). Once it exists
	// AppKit answers the kAEGetURL / kAEOpenDocuments Apple events by itself
	// and no longer calls application:openFiles:. URLs that launched the app
	// arrive before applicationDidFinishLaunching:; the core queues them.
	openURLs := func(_ id, _ objc.SEL, _ id, urls id) {
		b := theBackend
		paths, links := partitionOpenURLs(urls)
		for _, link := range links {
			b.appEvent(platform.AppEvent{Kind: platform.AppOpenURL, URL: link})
		}
		if len(paths) != 0 {
			b.appEvent(platform.AppEvent{Kind: platform.AppOpenFiles, Paths: paths})
		}
	}
	// A click on the Dock icon. The core only hears about it when no window
	// is visible; returning YES keeps AppKit's own behaviour of restoring a
	// minimized window.
	reopen := func(_ id, _ objc.SEL, _ id, hasVisible bool) bool {
		if !hasVisible {
			theBackend.appEvent(platform.AppEvent{Kind: platform.AppReopen})
		}
		return true
	}
	return []objc.MethodDef{
		method("application:openURLs:", openURLs),
		method("applicationShouldHandleReopen:hasVisibleWindows:", reopen),
		// Dock menu set with platform.AppShortcuts (sysx_dock.go).
		dockMenuMethod(),
	}
}

var _ platform.RecentDocuments = (*backend)(nil)

// NoteRecentDocument adds path to the app's Recent Documents (Dock menu,
// File > Open Recent if the app menu has one); platform.RecentDocuments.
func (b *backend) NoteRecentDocument(path string) {
	withPool(func() {
		dc := send(class("NSDocumentController"), "sharedDocumentController")
		send(dc, "noteNewRecentDocumentURL:", uintptr(fileURL(path)))
	})
}

// ClearRecentDocuments empties the app's Recent Documents.
func (b *backend) ClearRecentDocuments() {
	withPool(func() {
		dc := send(class("NSDocumentController"), "sharedDocumentController")
		send(dc, "clearRecentDocuments:", 0)
	})
}
