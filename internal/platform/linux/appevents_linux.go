//go:build linux && !android

package linux

import "github.com/tangoui/tango/internal/platform"

// askingQuit is set while HostEx.ShouldQuit runs (main thread only).
var askingQuit bool

// userQuit is the Quit menu role (Ctrl+Q): it asks HostEx.ShouldQuit
// first, unlike Backend.Quit (App.Quit from code). A second request while
// the first is being answered is ignored.
func (b *backend) userQuit() {
	if hx, ok := b.host.(platform.HostEx); ok {
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
