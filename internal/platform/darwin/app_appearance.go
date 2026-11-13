//go:build darwin && !ios

package darwin

import "github.com/tangoui/tango/internal/platform"

var _ platform.AppAppearance = (*backend)(nil)

// SetAppAppearance implements platform.AppAppearance by setting
// NSApp.appearance (macOS 10.14+; ignored before): windows, menus and
// WKWebView's prefers-color-scheme follow it, and the KVO on
// NSApp.effectiveAppearance (integration.go) reports the change as a theme
// change. nil — mode "" — goes back to the system appearance.
func (b *backend) SetAppAppearance(mode string) {
	if b == nil || b.app == 0 {
		return
	}
	withPool(func() {
		if !respondsTo(b.app, "setAppearance:") {
			return
		}
		var look id
		switch mode {
		case "light":
			look = send(class("NSAppearance"), "appearanceNamed:", uintptr(nsString(appearanceLight)))
		case "dark":
			look = send(class("NSAppearance"), "appearanceNamed:", uintptr(nsString(appearanceDark)))
		}
		send(b.app, "setAppearance:", uintptr(look))
	})
}
