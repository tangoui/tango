//go:build windows && (amd64 || arm64)

package windows

import "github.com/tangoui/tango/internal/platform"

// askingQuit is set while HostEx.ShouldQuit runs (main thread only).
var askingQuit bool

// userQuit is the Quit menu role: it asks HostEx.ShouldQuit first, unlike
// Backend.Quit (App.Quit from code). A second request while the first is
// being answered is ignored.
//
// WM_QUERYENDSESSION (logoff/shutdown) is deliberately not routed here:
// Windows gives the app a few seconds and no chance to show UI, so the
// core's quit handlers, which may ask the user, cannot run there.
func (b *Backend) userQuit() {
	if hx, ok := b.h.(platform.HostEx); ok {
		if askingQuit {
			return
		}
		askingQuit = true
		allow := hx.ShouldQuit()
		askingQuit = false
		if !allow {
			return
		}
	}
	b.Quit()
}
