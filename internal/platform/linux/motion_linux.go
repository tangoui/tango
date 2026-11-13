//go:build linux && !android

package linux

// platform.MotionPreference: the GTK "enable animations" setting
// (GtkSettings:gtk-enable-animations, which GNOME's
// org.gnome.desktop.interface enable-animations and the Wayland settings
// portal feed). Animations off means "reduce motion". A change is reported
// as AppReduceMotionChanged, the way installSystemWatch reports theme
// changes (integration_linux.go).

import (
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.MotionPreference = (*backend)(nil)

var motion struct {
	reduce  atomic.Bool
	watched bool // main thread
}

// ReduceMotion implements platform.MotionPreference. Any goroutine: it
// reads the value the main thread last saw (Init reads it first).
func (b *backend) ReduceMotion() bool { return motion.reduce.Load() }

// installMotionWatch reads gtk-enable-animations and follows its changes.
// Called from Init after gtk_init (main thread).
func (b *backend) installMotionWatch() {
	if motion.watched || !bindIntegration() {
		return
	}
	st := ix.settingsGetDefault()
	if st == 0 {
		return
	}
	motion.watched = true
	motion.reduce.Store(readReduceMotion(st))
	connect(st, "notify::gtk-enable-animations", 1, func(_, _ ptr) ptr {
		b.motionMaybeChanged(st)
		return 0
	})
}

func readReduceMotion(st ptr) bool {
	on, _ := gtkSetting(st, "gtk-enable-animations", gTypeBoolean)
	return !on
}

func (b *backend) motionMaybeChanged(st ptr) {
	v := readReduceMotion(st)
	if motion.reduce.Swap(v) == v {
		return
	}
	if hx, ok := b.host.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppReduceMotionChanged})
	}
}
