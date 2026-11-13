package uihost

// Following the system (docs/specs/system-follow.md): uihost windows use a
// theme generated from the system's light/dark appearance (and accent
// color, and high-contrast preference: contrast.go) and switch it when the
// system does; the built-in texts of components follow i18n.Locale().
//
// All windows of an app share one subscription to each source. The theme
// listener (App.OnEvent) cannot be unregistered, so once the last window
// closed it stays registered but does nothing; the i18n listener is
// cancelled and registered again by the next window.

import (
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
	systheme "github.com/tangoui/tango/sys/theme"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// ThemeMode says how a window's theme relates to the system appearance.
type ThemeMode int

const (
	// ThemeSystem follows the system: light or dark, switching when the
	// user does. The default.
	ThemeSystem ThemeMode = iota
	// ThemeLight always uses the light variant of the generated theme
	// (Flavor, AccentFromSystem and Customize still apply; with
	// AccentFromSystem the accent color is still followed).
	ThemeLight
	// ThemeDark always uses the dark variant of the generated theme.
	ThemeDark
	// ThemeFixed uses RunnerOptions.Theme (theme.AntDesign() when nil)
	// and never changes it.
	ThemeFixed
)

// LocaleMode says whether component texts follow the app's locale.
type LocaleMode int

const (
	// LocaleSystem sets ui.SetDefaultLocale to the pack matching
	// i18n.Locale() (the system languages, or the app's i18n.SetLocale)
	// when a window opens and whenever that locale changes, and redraws
	// the windows. Locales without a ui pack fall back to English.
	LocaleSystem LocaleMode = iota
	// LocaleFixed leaves ui's default locale alone.
	LocaleFixed
)

// ThemeFor returns the theme a window with these options draws while the
// system appearance is sys: nil for ThemeFixed. Pages of the same app can
// use it to get the same tokens (see sys/theme/uivars):
//
//	uivars.SyncPage(app, opts.ThemeFor)
//
// With ContrastHigh the result is the high-contrast variant. ThemeFor does
// not read the system high-contrast preference (ContrastSystem gives the
// standard variant); ThemeForContrast does.
func (o Options) ThemeFor(sys systheme.Theme) *theme.Theme {
	return o.themeForContrast(sys, o.highContrast(false))
}

// ThemeForContrast is ThemeFor while the system high-contrast preference
// is highContrast (see SystemHighContrast): the theme a window with these
// options draws then, the high-contrast variant included.
func (o Options) ThemeForContrast(sys systheme.Theme, highContrast bool) *theme.Theme {
	return o.themeForContrast(sys, o.highContrast(highContrast))
}

// baseTheme is the generated theme before the contrast decision.
func (o Options) baseTheme(sys systheme.Theme) *theme.Theme {
	var dark bool
	switch o.Theme {
	case ThemeFixed:
		return nil
	case ThemeLight:
	case ThemeDark:
		dark = true
	default:
		dark = sys.Dark
	}
	var accent *theme.Color
	if o.AccentFromSystem && sys.Accent != 0 {
		c := accentColor(sys.Accent)
		accent = &c
	}
	var th *theme.Theme
	if o.Flavor != nil {
		th = o.Flavor(dark, accent)
	}
	if th == nil {
		var opts []theme.Option
		if dark {
			opts = append(opts, theme.Dark())
		}
		if accent != nil {
			opts = append(opts, theme.WithPrimary(*accent))
		}
		th = theme.AntDesign(opts...)
	}
	if o.Customize != nil {
		o.Customize(th)
	}
	return th
}

// accentColor converts 0xRRGGBBAA to an opaque color.
func accentColor(c uint32) theme.Color {
	return theme.Color{R: uint8(c >> 24), G: uint8(c >> 16), B: uint8(c >> 8), A: 0xff}
}

// FollowSystemTheme turns following the system theme on or off for r, a
// runner returned by Open or Attach. Turning it on applies the theme for
// the current system appearance at once (a window opened with ThemeFixed
// then follows as with ThemeSystem). Main thread; other runners are
// ignored.
//
// A runner also stops following by itself when its theme was changed by
// someone else: if, when the system appearance changes, the runner's
// theme is not the one uihost set last (the app called Runner.SetTheme,
// or a ConfigProvider with theme settings replaced it), uihost leaves it
// alone from then on. Call FollowSystemTheme(r, false) before setting a
// theme by hand to make that explicit, and FollowSystemTheme(r, true) to
// go back to following.
func FollowSystemTheme(r *ui.Runner, on bool) {
	f := followerOf(r)
	if f == nil {
		return
	}
	f.theme = on
	if !on {
		return
	}
	sys, err := systheme.System(f.app)
	if err != nil {
		sys = systheme.Theme{}
	}
	f.apply(sys)
}

// FollowingSystemTheme reports whether r currently follows the system
// theme. Main thread.
func FollowingSystemTheme(r *ui.Runner) bool {
	f := followerOf(r)
	return f != nil && f.theme && f.r.Context().Theme == f.last
}

// follower is one window's following state. Main thread, except that the
// registry (follows) is locked when lists change.
type follower struct {
	app    *tango.App
	r      *ui.Runner
	opts   Options // Theme is never ThemeFixed here
	theme  bool    // following the system theme
	last   *theme.Theme
	sys    systheme.Theme // appearance last applied
	hc     bool           // last is the high-contrast variant
	locale bool           // following i18n.Locale
}

// apply sets the theme for sys and the current system contrast. Main
// thread.
func (f *follower) apply(sys systheme.Theme) {
	on, _ := systemHighContrast(f.app)
	f.applyContrast(sys, f.opts.highContrast(on))
}

// applyContrast sets the theme for sys, high-contrast or not. Main
// thread.
func (f *follower) applyContrast(sys systheme.Theme, hc bool) {
	th := f.opts.themeForContrast(sys, hc)
	f.last, f.sys, f.hc = th, sys, hc
	f.r.SetTheme(th)
}

// appFollow is the shared subscription of one app's windows.
type appFollow struct {
	app        *tango.App
	list       []*follower
	hooked     bool   // theme and contrast listeners registered (for good)
	cancelI18n func() // nil: not subscribed
}

var follows struct {
	mu         sync.Mutex
	apps       map[*tango.App]*appFollow
	byRunner   map[*ui.Runner]*follower
	lastLocale *ui.Locale // the default locale uihost set last
}

func followerOf(r *ui.Runner) *follower {
	follows.mu.Lock()
	defer follows.mu.Unlock()
	return follows.byRunner[r]
}

// pendingFollow is what followSetup decided, applied by start once the
// runner exists.
type pendingFollow struct {
	app    *tango.App
	opts   Options
	theme  bool
	last   *theme.Theme
	sys    systheme.Theme
	hc     bool
	locale bool
}

// followSetup resolves the options for a new window: it returns the runner
// options to use (with the generated theme) and the follower to start.
// Main thread.
func followSetup(app *tango.App, opts Options, ropts ui.RunnerOptions) (ui.RunnerOptions, *pendingFollow) {
	p := &pendingFollow{app: app, opts: opts, locale: opts.Locale == LocaleSystem}
	if opts.Theme == ThemeSystem && ropts.Theme != nil {
		p.opts.Theme = ThemeFixed // an explicit theme keeps the old behavior
	}
	if p.opts.Theme == ThemeFixed {
		p.opts.Theme = ThemeSystem // what FollowSystemTheme(r, true) uses
	} else {
		sys, err := systheme.System(app)
		if err != nil {
			sys = systheme.Theme{}
		}
		p.theme = true
		on, _ := systemHighContrast(app)
		p.sys, p.hc = sys, p.opts.highContrast(on)
		p.last = p.opts.themeForContrast(sys, p.hc)
		ropts.Theme = p.last
	}
	if p.locale {
		l := localeFor(i18n.Locale())
		ui.SetDefaultLocale(l)
		follows.mu.Lock()
		follows.lastLocale = l
		follows.mu.Unlock()
	}
	return ropts, p
}

// localeFor returns the ui pack for an i18n locale, English when there is
// none.
func localeFor(code string) *ui.Locale {
	if l, ok := ui.LocaleByCode(code); ok {
		return l
	}
	l, _ := ui.LocaleByCode(i18n.DefaultLocale)
	return l
}

// start registers the window's follower and returns the function that
// removes it; it also runs when s closes (s may be nil). Main thread.
func (p *pendingFollow) start(r *ui.Runner, s *tango.Surface) (stop func()) {
	f := &follower{app: p.app, r: r, opts: p.opts, theme: p.theme, last: p.last, sys: p.sys, hc: p.hc, locale: p.locale}
	follows.mu.Lock()
	if follows.apps == nil {
		follows.apps = map[*tango.App]*appFollow{}
		follows.byRunner = map[*ui.Runner]*follower{}
	}
	af := follows.apps[p.app]
	if af == nil {
		af = &appFollow{app: p.app}
		follows.apps[p.app] = af
	}
	af.list = append(af.list, f)
	follows.byRunner[r] = f
	hook := !af.hooked
	af.hooked = true
	if af.cancelI18n == nil {
		af.cancelI18n = i18n.OnChange(func(code string) {
			// Any goroutine: hop to the app's main thread.
			p.app.Post(func() { af.localeChanged(code) })
		})
	}
	follows.mu.Unlock()
	if hook {
		systheme.OnChange(p.app, af.themeChanged)
		p.app.OnEventKind(platform.AppContrastChanged, func(tango.AppEvent) { af.contrastChanged() }) // contrast.go
	}
	var once sync.Once
	stop = func() { once.Do(func() { af.remove(f) }) }
	if s != nil {
		s.OnClosed(stop)
	}
	return stop
}

// remove drops f; the last window of the app cancels the i18n listener.
func (af *appFollow) remove(f *follower) {
	follows.mu.Lock()
	af.list = slices.DeleteFunc(af.list, func(x *follower) bool { return x == f })
	delete(follows.byRunner, f.r)
	var cancel func()
	if len(af.list) == 0 {
		cancel, af.cancelI18n = af.cancelI18n, nil
	}
	follows.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (af *appFollow) snapshot() []*follower {
	follows.mu.Lock()
	defer follows.mu.Unlock()
	return slices.Clone(af.list)
}

// themeChanged runs on the main thread when the system theme changed.
func (af *appFollow) themeChanged(sys systheme.Theme) {
	for _, f := range af.snapshot() {
		if !f.theme {
			continue
		}
		if f.r.Context().Theme != f.last {
			f.theme = false // the app set its own theme: leave it alone
			continue
		}
		if f.opts.Theme != ThemeSystem && !f.opts.AccentFromSystem {
			continue // nothing to follow
		}
		f.apply(sys)
	}
}

// localeChanged runs on the main thread when i18n.Locale changed.
func (af *appFollow) localeChanged(code string) {
	list := af.snapshot()
	var first *follower
	for _, f := range list {
		if f.locale {
			first = f
			break
		}
	}
	if first == nil {
		return
	}
	l := localeFor(code)
	follows.mu.Lock()
	last := follows.lastLocale
	follows.mu.Unlock()
	if cur := first.r.Context().Locale(); last != nil && cur != last && cur != l {
		// The app called ui.SetDefaultLocale itself: stop following.
		for _, f := range list {
			f.locale = false
		}
		return
	}
	ui.SetDefaultLocale(l)
	follows.mu.Lock()
	follows.lastLocale = l
	follows.mu.Unlock()
	for _, f := range list {
		f.r.Invalidate()
	}
}
