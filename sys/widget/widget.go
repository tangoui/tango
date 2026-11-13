// Package widget updates the app's home-screen widget.
//
//	widget.Update(a, widget.Content{Title: "Inbox", Text: "3 unread"})
//
// Android: the host's AppWidget (org.tango.host.TangoWidget), added to the
// manifest when tan.json "permissions" lists "widget". It shows a title and
// a text; tapping it opens the app. The content is stored and survives
// restarts and reboots, so the widget shows the last value even while the
// app is not running.
//
// iOS widgets are WidgetKit extensions written in SwiftUI and are not built
// by tan yet (see docs/specs/mobile-extensions.md): Update returns
// ErrUnsupported there, as on desktop.
package widget

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported   = platform.ErrUnsupported
	ErrNotConfigured = platform.ErrNotConfigured // the app does not declare the widget
)

type Content = platform.WidgetContent

// Update replaces what the widget shows.
func Update(a *tango.App, c Content) error {
	w, err := capcall.Backend[platform.Widgets](a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, w.UpdateWidget(c) })
	return err
}
