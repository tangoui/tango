package uihost

// Following the system "reduce motion" preference (docs/specs/a11y-bridge.md):
// platform.MotionPreference gives the current value, AppReduceMotionChanged
// reports changes (like the theme in follow.go). A window whose
// RunnerOptions.ReduceMotion is set keeps reduced motion whatever the
// system says.

import (
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
)

type motionWin struct {
	r     *ui.Runner
	fixed bool // RunnerOptions.ReduceMotion: always on
}

type appMotion struct {
	list []*motionWin
	last bool // last value applied
}

var motions struct {
	mu   sync.Mutex
	apps map[*tango.App]*appMotion
}

// systemReduceMotion reads the backend's preference (false when it has
// none).
func systemReduceMotion(app *tango.App) (on, ok bool) {
	if app == nil {
		return false, false
	}
	mp, ok := app.Backend().(platform.MotionPreference)
	if !ok {
		return false, false
	}
	return mp.ReduceMotion(), true
}

// followMotion applies the system preference to r now and on every change
// until the returned stop is called. Main thread.
func followMotion(app *tango.App, r *ui.Runner, ropts ui.RunnerOptions) (stop func()) {
	on, ok := systemReduceMotion(app)
	if !ok {
		return func() {}
	}
	if on && !ropts.ReduceMotion {
		r.SetReduceMotion(true)
	}
	w := &motionWin{r: r, fixed: ropts.ReduceMotion}
	motions.mu.Lock()
	if motions.apps == nil {
		motions.apps = map[*tango.App]*appMotion{}
	}
	am := motions.apps[app]
	hook := am == nil
	if hook {
		am = &appMotion{last: on}
		motions.apps[app] = am
	}
	am.list = append(am.list, w)
	motions.mu.Unlock()
	if hook {
		app.OnEventKind(platform.AppReduceMotionChanged, func(tango.AppEvent) { motionChanged(app) })
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			motions.mu.Lock()
			am.list = slices.DeleteFunc(am.list, func(x *motionWin) bool { return x == w })
			motions.mu.Unlock()
		})
	}
}

// motionChanged runs on the main thread after AppReduceMotionChanged.
func motionChanged(app *tango.App) {
	on, ok := systemReduceMotion(app)
	if !ok {
		return
	}
	motions.mu.Lock()
	am := motions.apps[app]
	if am == nil || am.last == on {
		motions.mu.Unlock()
		return
	}
	am.last = on
	list := slices.Clone(am.list)
	motions.mu.Unlock()
	for _, w := range list {
		w.r.SetReduceMotion(on || w.fixed)
	}
}
