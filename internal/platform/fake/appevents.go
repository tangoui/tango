package fake

import (
	"slices"

	"github.com/tangoui/tango/internal/platform"
)

// App-level native events (contract round 2: HostEx). The helpers below
// simulate the OS; they are no-ops when the Host is not a HostEx.

func (b *Backend) hostEx() (platform.HostEx, bool) {
	hx, ok := b.h.(platform.HostEx)
	return hx, ok
}

// AppEvent simulates the platform delivering ev (deep link, opened files,
// Dock reopen, ...). It reports false if the loop stopped first.
func (b *Backend) AppEvent(ev platform.AppEvent) bool {
	return b.Do(func() {
		if hx, ok := b.hostEx(); ok {
			hx.OnAppEvent(ev)
		}
	})
}

// DeepLink simulates the OS opening url (a custom scheme) with the app.
func (b *Backend) DeepLink(url string) bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppOpenURL, URL: url})
}

// OpenFiles simulates the OS asking the app to open paths.
func (b *Backend) OpenFiles(paths ...string) bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppOpenFiles, Paths: paths})
}

// Reopen simulates a click on the Dock icon with no visible windows.
func (b *Backend) Reopen() bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppReopen})
}

// RequestQuit simulates the user quitting (Cmd+Q, a Quit menu role): it
// asks HostEx.ShouldQuit and quits if allowed, like the real backends. It
// reports whether the app is quitting.
func (b *Backend) RequestQuit() bool {
	quit := false
	b.Do(func() {
		if hx, ok := b.hostEx(); ok && !hx.ShouldQuit() {
			return
		}
		quit = true
		b.Quit()
	})
	return quit
}

// NotificationOpened simulates a click on a notification an earlier run
// sent (AppNotificationOpened).
func (b *Backend) NotificationOpened(id string) bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppNotificationOpened, Notification: id})
}

// ---- platform.RecentDocuments ----

var _ platform.RecentDocuments = (*Backend)(nil)

// NoteRecentDocument records path as the OS recent-documents list would.
func (b *Backend) NoteRecentDocument(path string) {
	b.mustMain("NoteRecentDocument")
	b.mu.Lock()
	b.recent = append(b.recent, path)
	b.mu.Unlock()
}

// ClearRecentDocuments empties the simulated OS list.
func (b *Backend) ClearRecentDocuments() {
	b.mustMain("ClearRecentDocuments")
	b.mu.Lock()
	b.recent = nil
	b.mu.Unlock()
}

// RecentDocuments returns what NoteRecentDocument recorded.
func (b *Backend) RecentDocuments() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.recent)
}
