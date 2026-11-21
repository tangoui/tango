//go:build linux && !android

package linux

import (
	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/platform"
)

func (b *backend) installActivityMonitor() {
	var setHandler func(callback, data, notify ptr)
	var eventType func(ptr) int32
	var eventWidget, toplevel func(ptr) ptr
	var dispatch func(ptr)
	var bd binder
	bd.req(&setHandler, "gdk_event_handler_set", libGTK)
	bd.req(&eventType, "gdk_event_get_event_type", libGTK)
	bd.req(&eventWidget, "gtk_get_event_widget", libGTK)
	bd.req(&toplevel, "gtk_widget_get_toplevel", libGTK)
	bd.req(&dispatch, "gtk_main_do_event", libGTK)
	if len(bd.missing) != 0 {
		return // keep GTK's dispatcher and rely on canvas and WebView inputs
	}
	setHandler(purego.NewCallback(func(event, _ ptr) {
		// GdkEventType: motion, buttons, keys, scroll, touch, touchpad gestures.
		switch eventType(event) {
		case 3, 4, 5, 6, 7, 8, 9, 31, 37, 38, 39, 41, 42:
			if w := eventWidget(event); w != 0 {
				win := toplevel(w)
				for _, s := range b.surfaces {
					if s.win == win && !s.closed {
						platform.ReportActivity(b.host, s.id)
						break
					}
				}
			}
		}
		dispatch(event)
	}), 0, 0)
}
