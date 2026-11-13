// Package theme reads the system's light/dark appearance and accent color
// and follows their changes.
//
// System and OnChange report what the operating system uses; the WebView
// already applies prefers-color-scheme to pages. SyncPage additionally
// mirrors the theme into every page as data attributes and a CSS variable,
// for styles that need the accent color or a selector to hang on:
//
//	<html data-tango-theme="dark" style="--tango-accent: #0a84ffff">
//
// Functions are safe from any goroutine once the app runs. System returns
// ErrUnsupported when the backend cannot read the theme and
// tango.ErrNotRunning after the app quit.
package theme

import (
	"fmt"
	"strconv"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

// Theme is the system appearance. Accent is 0xRRGGBBAA, 0 when the
// platform has none or does not say.
type Theme = platform.Theme

type result struct {
	t   Theme
	ran bool
}

// System returns the current system theme.
func System(a *tango.App) (Theme, error) {
	st, ok := a.Backend().(platform.SystemTheme)
	if !ok {
		return Theme{}, ErrUnsupported
	}
	r := tango.Await(a, func() result { return result{st.SystemTheme(), true} })
	if !r.ran {
		return Theme{}, tango.ErrNotRunning
	}
	return r.t, nil
}

// OnChange calls fn on the main thread with the new theme whenever the
// user switches between light and dark or changes the accent color.
func OnChange(a *tango.App, fn func(Theme)) {
	a.OnEvent(func(ev tango.AppEvent) {
		if ev.Kind == tango.EventThemeChanged {
			fn(ev.Theme)
		}
	})
}

// SyncPage keeps document.documentElement of every page in step with the
// system theme: the attribute data-tango-theme is "dark" or "light", and
// the CSS variable --tango-accent holds the accent color (#rrggbbaa) when
// known. Pages can also listen for the "tangotheme" event on window, whose
// detail is {dark, accent}.
//
// Call it before creating surfaces (it adds a bridge script). Pages that
// exist are updated on every change; every page that loads later — in a
// new surface or after a navigation — gets the current theme, accent
// included, when it has loaded (until then light/dark follows
// prefers-color-scheme). It returns ErrUnsupported when the backend cannot
// read the theme; the attribute then still follows prefers-color-scheme.
func SyncPage(a *tango.App) error {
	a.AddBridgeScript(pageScript)
	st, ok := a.Backend().(platform.SystemTheme)
	if !ok {
		return ErrUnsupported
	}
	// current is only touched on the main thread (OnChange, OnReady and
	// the surface callbacks all run there).
	var current Theme
	known := false
	push := func(t Theme) {
		current, known = t, true
		js := applyCall(t)
		for _, s := range a.Surfaces() {
			s.Eval(js)
		}
	}
	OnChange(a, push)
	a.OnReady(func() {
		if !known {
			push(st.SystemTheme())
		}
	})
	a.OnSurfaceCreated(func(s *tango.Surface) {
		s.OnLoaded(func() {
			if !known {
				current, known = st.SystemTheme(), true
			}
			s.Eval(applyCall(current))
		})
	})
	return nil
}

// applyCall is the JS that applies t to a page.
func applyCall(t Theme) string {
	return fmt.Sprintf("window.__tangoTheme&&window.__tangoTheme(%s,%s);", strconv.FormatBool(t.Dark), strconv.Quote(AccentCSS(t.Accent)))
}

// AccentCSS formats a 0xRRGGBBAA color as #rrggbbaa; "" for 0.
func AccentCSS(c uint32) string {
	if c == 0 {
		return ""
	}
	return fmt.Sprintf("#%08x", c)
}

// pageScript installs window.__tangoTheme and the prefers-color-scheme
// fallback. It runs at document start, before <html> may exist, so the
// attribute is set once the element is there.
const pageScript = `(function(){
if (window.__tangoTheme) return;
var state = {dark: null, accent: ""};
var mq = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
function apply(){
  var el = document.documentElement;
  if (!el) return;
  var dark = state.dark === null ? !!(mq && mq.matches) : state.dark;
  el.dataset.tangoTheme = dark ? "dark" : "light";
  if (state.accent) el.style.setProperty("--tango-accent", state.accent);
  else el.style.removeProperty("--tango-accent");
  try { window.dispatchEvent(new CustomEvent("tangotheme", {detail: {dark: dark, accent: state.accent}})); } catch (e) {}
}
window.__tangoTheme = function(dark, accent){ state.dark = dark; state.accent = accent || ""; apply(); };
if (mq && mq.addEventListener) mq.addEventListener("change", function(){ state.dark = null; apply(); });
if (document.documentElement) apply();
else document.addEventListener("readystatechange", apply, {once: true});
})();`
